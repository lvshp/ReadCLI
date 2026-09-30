package lib

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	updateHelperFlag     = "--readcli-update-helper"
	updateHelperName     = "readcli-update-helper.exe"
	updateRequestName    = "request.json"
	updatePrepareTimeout = 2 * time.Minute
	updateHelperPoll     = 50 * time.Millisecond
	updateCommitGrace    = 30 * time.Second
)

var errUpdateHelperCancelled = errors.New("更新准备已取消，原程序未替换")

// All IPC filenames and the downloaded executable are derived from the request
// directory. No IPC response can choose a destination for an elevated write.
type updateHelperRequest struct {
	Nonce       string `json:"nonce"`
	Target      string `json:"target"`
	SourceHash  string `json:"source_sha256"`
	CurrentHash string `json:"current_sha256"`
	ParentPID   uint32 `json:"parent_pid"`
	ParentStart uint64 `json:"parent_start"`
	Deadline    int64  `json:"prepare_deadline"`
	Mode        uint32 `json:"mode"`
}

type updateHelperState struct {
	Nonce  string `json:"nonce"`
	Staged string `json:"staged,omitempty"`
	Error  string `json:"error,omitempty"`
}

func helperFileHash(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("不是普通文件：%s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func helperCanonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func helperSamePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func helperValidHash(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == sha256.Size && s == strings.ToLower(s)
}

func helperWriteJSON(dir, name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".ipc-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, name))
}

func helperReadState(dir, name, nonce string) (updateHelperState, bool, error) {
	var state updateHelperState
	data, err := os.ReadFile(filepath.Join(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return state, false, nil
	}
	if err != nil {
		return state, false, err
	}
	if len(data) > 64*1024 {
		return state, false, errors.New("更新状态文件过大")
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, false, err
	}
	if state.Nonce != nonce {
		return state, false, errors.New("更新状态校验失败")
	}
	return state, true, nil
}

func helperCancelled(dir string, req updateHelperRequest) bool {
	_, exists, err := helperReadState(dir, "cancel.json", req.Nonce)
	return exists || err != nil
}

func helperCreateRequest(binaryPath, target, dir string, pid uint32, started uint64) (updateHelperRequest, string, error) {
	var req updateHelperRequest
	if !helperSamePath(binaryPath, filepath.Join(dir, "readcli.exe")) || helperSamePath(filepath.Dir(target), dir) {
		return req, "", errors.New("更新文件路径关系无效")
	}
	info, err := os.Stat(target)
	if err != nil {
		return req, "", err
	}
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return req, "", err
	}
	req = updateHelperRequest{Nonce: hex.EncodeToString(nonce), Target: target, ParentPID: pid, ParentStart: started, Deadline: time.Now().Add(updatePrepareTimeout).UnixNano(), Mode: uint32(info.Mode().Perm())}
	if req.SourceHash, err = helperFileHash(binaryPath); err != nil {
		return req, "", err
	}
	if req.CurrentHash, err = helperFileHash(target); err != nil {
		return req, "", err
	}
	if err := helperWriteJSON(dir, updateRequestName, req); err != nil {
		return req, "", err
	}
	digest, err := helperFileHash(filepath.Join(dir, updateRequestName))
	return req, digest, err
}

func helperLoadRequest(path, expectedHash string) (updateHelperRequest, string, error) {
	var req updateHelperRequest
	if !helperValidHash(expectedHash) {
		return req, "", errors.New("更新请求摘要无效")
	}
	canonical, err := helperCanonicalPath(path)
	if err != nil {
		return req, "", err
	}
	if !helperSamePath(path, canonical) || filepath.Base(path) != updateRequestName {
		return req, "", errors.New("更新请求路径无效")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return req, "", err
	}
	if len(data) > 16*1024 {
		return req, "", errors.New("更新请求过大")
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != expectedHash {
		return req, "", errors.New("更新请求已被修改")
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return req, "", err
	}
	if !helperValidHash(req.Nonce) || !helperValidHash(req.SourceHash) || !helperValidHash(req.CurrentHash) || req.ParentPID == 0 || req.ParentStart == 0 {
		return req, "", errors.New("更新请求字段无效")
	}
	dir := filepath.Dir(path)
	target, err := helperCanonicalPath(req.Target)
	if err != nil {
		return req, "", err
	}
	if !filepath.IsAbs(req.Target) || !helperSamePath(target, req.Target) || !strings.EqualFold(filepath.Ext(target), ".exe") || helperSamePath(filepath.Dir(target), dir) {
		return req, "", errors.New("更新目标路径无效")
	}
	if req.Deadline <= time.Now().UnixNano() || req.Deadline > time.Now().Add(updatePrepareTimeout+time.Minute).UnixNano() {
		return req, "", errors.New("更新准备请求已过期")
	}
	return req, dir, nil
}

func helperVerifyHash(path, expected string) error {
	actual, err := helperFileHash(path)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("文件摘要已变化，停止更新：%s", path)
	}
	return nil
}

func helperValidateReady(dir string, req updateHelperRequest, state updateHelperState) error {
	prefix := "." + filepath.Base(req.Target) + ".update-"
	if !helperSamePath(filepath.Dir(state.Staged), filepath.Dir(req.Target)) || !strings.HasPrefix(filepath.Base(state.Staged), prefix) {
		return errors.New("更新暂存路径无效")
	}
	if err := helperVerifyHash(req.Target, req.CurrentHash); err != nil {
		return err
	}
	return helperVerifyHash(state.Staged, req.SourceHash)
}

// accepted/confirmed acknowledge readiness; only commit authorizes installation.
// A failed or timed-out parent never writes commit, so a late UAC/helper cannot
// install later, even if writing the cancellation marker itself fails.
func helperAwaitPrepared(dir string, req updateHelperRequest, childExited func() (bool, error)) error {
	requestedConfirmation := false
	for {
		if helperCancelled(dir, req) {
			return errUpdateHelperCancelled
		}
		if time.Now().UnixNano() >= req.Deadline {
			return errors.New("准备更新超时；如有管理员授权窗口，请取消后重试")
		}
		failure, exists, err := helperReadState(dir, "failed.json", req.Nonce)
		if err != nil {
			return err
		}
		if exists {
			return errors.New(failure.Error)
		}
		if !requestedConfirmation {
			ready, exists, err := helperReadState(dir, "ready.json", req.Nonce)
			if err != nil {
				return err
			}
			if exists {
				if err := helperValidateReady(dir, req, ready); err != nil {
					return err
				}
				if helperCancelled(dir, req) {
					return errUpdateHelperCancelled
				}
				if time.Now().UnixNano() >= req.Deadline {
					return errors.New("校验更新文件时准备超时，原程序未替换")
				}
				if err := helperWriteJSON(dir, "accepted.json", updateHelperState{Nonce: req.Nonce}); err != nil {
					return err
				}
				requestedConfirmation = true
			}
		}
		if requestedConfirmation {
			_, confirmed, err := helperReadState(dir, "confirmed.json", req.Nonce)
			if err != nil {
				return err
			}
			if confirmed {
				if exited, err := childExited(); err != nil {
					return err
				} else if exited {
					return errors.New("更新助手在准备完成前退出")
				}
				if helperCancelled(dir, req) {
					return errUpdateHelperCancelled
				}
				if time.Now().UnixNano() >= req.Deadline {
					return errors.New("等待更新助手确认超时，原程序未替换")
				}
				return helperWriteJSON(dir, "commit.json", updateHelperState{Nonce: req.Nonce})
			}
		}
		if exited, err := childExited(); err != nil {
			return err
		} else if exited {
			return errors.New("更新助手提前退出，请查看更新日志")
		}
		time.Sleep(updateHelperPoll)
	}
}

func helperFinishUpdate(dir string, req updateHelperRequest, staged string, parentExited func() (bool, error), apply func(string, string) error) error {
	if helperCancelled(dir, req) {
		return errUpdateHelperCancelled
	}
	if exited, err := parentExited(); err != nil {
		return err
	} else if exited {
		return errors.New("程序已在准备完成前退出，取消更新")
	}
	if err := helperWriteJSON(dir, "ready.json", updateHelperState{Nonce: req.Nonce, Staged: staged}); err != nil {
		return err
	}
	confirmed, committed := false, false
	for {
		if helperCancelled(dir, req) {
			return errUpdateHelperCancelled
		}
		if !confirmed {
			_, exists, err := helperReadState(dir, "accepted.json", req.Nonce)
			if err != nil {
				return err
			}
			if exists {
				if err := helperWriteJSON(dir, "confirmed.json", updateHelperState{Nonce: req.Nonce}); err != nil {
					return err
				}
				confirmed = true
			}
			if !confirmed && time.Now().UnixNano() >= req.Deadline {
				return errors.New("父程序未确认更新准备，取消安装")
			}
		}
		if confirmed && !committed {
			_, exists, err := helperReadState(dir, "commit.json", req.Nonce)
			if err != nil {
				return err
			}
			committed = exists
		}
		exited, err := parentExited()
		if err != nil {
			return err
		}
		if exited {
			// Read final permission after observing exit: the parent may have
			// written it and exited between our previous filesystem reads.
			_, committed, err := helperReadState(dir, "commit.json", req.Nonce)
			if err != nil {
				return err
			}
			if !confirmed || !committed {
				return errors.New("程序未确认准备完成就已退出，原程序未替换")
			}
			break
		}
		// Allow the parent's final filesystem write to finish after confirmation,
		// but do not leave an uncommitted helper waiting for a live parent forever.
		// Once final permission exists, the user may keep reading until they exit.
		if confirmed && !committed && time.Now().UnixNano() >= req.Deadline+int64(updateCommitGrace) {
			return errors.New("父程序未完成更新确认，取消安装")
		}
		time.Sleep(updateHelperPoll)
	}
	if helperCancelled(dir, req) {
		return errUpdateHelperCancelled
	}
	if err := helperVerifyHash(req.Target, req.CurrentHash); err != nil {
		return err
	}
	if err := helperVerifyHash(staged, req.SourceHash); err != nil {
		return err
	}
	return apply(staged, req.Target)
}

func helperRecordFailure(dir string, req updateHelperRequest, staged string, err error) {
	message := fmt.Sprintf("%s\n目标：%s\n暂存：%s\n下载文件：%s\n", err, req.Target, staged, filepath.Join(dir, "readcli.exe"))
	_ = os.WriteFile(filepath.Join(dir, "update.log"), []byte(message), 0600)
	_ = helperWriteJSON(dir, "failed.json", updateHelperState{Nonce: req.Nonce, Staged: staged, Error: message})
}
