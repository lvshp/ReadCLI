//go:build windows

package lib

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func helperProcessStarted(process windows.Handle) (uint64, error) {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(process, &created, &exited, &kernel, &user); err != nil {
		return 0, err
	}
	return uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime), nil
}

func helperProcessExited(process windows.Handle) (bool, error) {
	status, err := windows.WaitForSingleObject(process, 0)
	if err != nil {
		return false, err
	}
	return status == windows.WAIT_OBJECT_0, nil
}

func helperProcessImage(process windows.Handle) (string, error) {
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil {
		return "", err
	}
	return helperCanonicalPath(windows.UTF16ToString(buffer[:size]))
}

func helperCopySelf(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	if copyErr == nil {
		copyErr = out.Sync()
	}
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// nil means prepared and accepted, not installed. The caller must retain dir
// and exit normally before the helper can replace this process's executable.
func startWindowsDeferredUpdate(binaryPath, executablePath, tempDir string) (result error) {
	binaryPath, err := helperCanonicalPath(binaryPath)
	if err != nil {
		return err
	}
	executablePath, err = helperCanonicalPath(executablePath)
	if err != nil {
		return err
	}
	tempDir, err = helperCanonicalPath(tempDir)
	if err != nil {
		return err
	}
	image, err := helperProcessImage(windows.CurrentProcess())
	if err != nil {
		return err
	}
	if !helperSamePath(image, executablePath) {
		return errors.New("更新目标不是当前运行的程序")
	}
	started, err := helperProcessStarted(windows.CurrentProcess())
	if err != nil {
		return err
	}
	req, digest, err := helperCreateRequest(binaryPath, executablePath, tempDir, uint32(os.Getpid()), started)
	if err != nil {
		return err
	}
	defer func() {
		if result != nil {
			_ = helperWriteJSON(tempDir, "cancel.json", updateHelperState{Nonce: req.Nonce})
		}
	}()
	helper := filepath.Join(tempDir, updateHelperName)
	if err := helperCopySelf(executablePath, helper); err != nil {
		return fmt.Errorf("准备更新助手失败: %w", err)
	}
	if err := helperVerifyHash(helper, req.CurrentHash); err != nil {
		return err
	}
	cmd := exec.Command(helper, updateHelperFlag, filepath.Join(tempDir, updateRequestName), digest)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	var diagnostic bytes.Buffer
	cmd.Stderr = &diagnostic
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新助手失败: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	exited := false
	result = helperAwaitPrepared(tempDir, req, func() (bool, error) {
		if exited {
			return true, nil
		}
		select {
		case err := <-done:
			exited = true
			if err != nil {
				return true, fmt.Errorf("更新助手退出: %w\n%s", err, diagnostic.String())
			}
			return true, nil
		default:
			return false, nil
		}
	})
	if result != nil {
		_ = helperWriteJSON(tempDir, "cancel.json", updateHelperState{Nonce: req.Nonce})
		if !exited {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				// An elevated child may outlive this supervisor, but without the
				// commit marker it cannot perform the final replacement.
				_ = cmd.Process.Kill()
			}
		}
	}
	return result
}

// RunUpdateHelper must run before normal flags and UI initialization. Only the
// current executable copied by the parent handles this private entry point.
func RunUpdateHelper(args []string) (bool, int) {
	if len(args) == 0 || args[0] != updateHelperFlag {
		return false, 0
	}
	if len(args) != 3 && (len(args) != 4 || args[3] != "--elevated") {
		fmt.Fprintln(os.Stderr, "更新助手参数无效")
		return true, 1
	}
	req, dir, err := helperLoadRequest(args[1], args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return true, 1
	}
	staged, delegated, err := runWindowsUpdateHelper(dir, req, args[2], len(args) == 4)
	if delegated {
		if err != nil {
			return true, 1
		}
		return true, 0
	}
	if err != nil {
		helperRecordFailure(dir, req, staged, err)
		_, committed, _ := helperReadState(dir, "commit.json", req.Nonce)
		if committed && !errors.Is(err, errUpdateHelperCancelled) {
			helperFailureDialog(fmt.Sprintf("更新未完成：%s\n\n原始下载与可恢复文件已保留。\n完整日志：%s", err, filepath.Join(dir, "update.log")))
		}
		return true, 1
	}
	_ = helperWriteJSON(dir, "installed.json", updateHelperState{Nonce: req.Nonce})
	_ = os.WriteFile(filepath.Join(dir, "update.log"), []byte("更新已安装。请自行重新启动 ReadCLI。\n运行中的助手副本保留在此临时目录，可在退出后删除。\n"), 0600)
	for _, name := range []string{updateRequestName, "ready.json", "accepted.json", "confirmed.json", "commit.json", "cancel.json", "readcli.exe", "readcli-update.zip"} {
		_ = os.Remove(filepath.Join(dir, name))
	}
	// Windows keeps this helper image mapped until process exit. Do not launch a
	// shell or restart the app as administrator merely to delete that small file.
	return true, 0
}

func runWindowsUpdateHelper(dir string, req updateHelperRequest, digest string, elevated bool) (staged string, delegated bool, result error) {
	self, err := os.Executable()
	if err != nil {
		return "", false, err
	}
	self, err = helperCanonicalPath(self)
	if err != nil {
		return "", false, err
	}
	if !helperSamePath(self, filepath.Join(dir, updateHelperName)) {
		return "", false, errors.New("更新助手不在本次临时目录")
	}
	if err := helperVerifyHash(self, req.CurrentHash); err != nil {
		return "", false, err
	}
	parent, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, req.ParentPID)
	if err != nil {
		return "", false, fmt.Errorf("打开待退出程序失败: %w", err)
	}
	defer windows.CloseHandle(parent)
	started, err := helperProcessStarted(parent)
	if err != nil {
		return "", false, err
	}
	image, err := helperProcessImage(parent)
	if err != nil {
		return "", false, err
	}
	if started != req.ParentStart || !helperSamePath(image, req.Target) {
		return "", false, errors.New("父进程身份或安装目标不匹配")
	}
	if helperCancelled(dir, req) {
		return "", false, errUpdateHelperCancelled
	}
	if err := helperVerifyHash(req.Target, req.CurrentHash); err != nil {
		return "", false, err
	}
	staged, err = helperStageDownload(dir, req)
	if err != nil && helperNeedsElevation(err, req.Target) && !elevated {
		child, launchErr := helperLaunchElevated(self, []string{updateHelperFlag, filepath.Join(dir, updateRequestName), digest, "--elevated"})
		if launchErr != nil {
			if errors.Is(launchErr, windows.ERROR_CANCELLED) {
				return "", false, errors.New("管理员授权已取消，原程序未替换")
			}
			return "", false, fmt.Errorf("请求管理员授权失败: %w", launchErr)
		}
		defer windows.CloseHandle(child)
		for {
			status, waitErr := windows.WaitForSingleObject(child, 100)
			if waitErr != nil {
				return "", false, waitErr
			}
			if status == windows.WAIT_OBJECT_0 {
				var code uint32
				if err := windows.GetExitCodeProcess(child, &code); err != nil {
					return "", false, err
				}
				if code != 0 {
					if _, exists, _ := helperReadState(dir, "failed.json", req.Nonce); !exists {
						return "", false, fmt.Errorf("管理员更新助手退出，状态码 %d", code)
					}
					return "", true, errors.New("管理员更新助手未完成安装")
				}
				return "", true, nil
			}
		}
	}
	if err != nil {
		return staged, false, err
	}
	if time.Now().UnixNano() >= req.Deadline {
		return staged, false, errors.New("更新暂存完成时准备请求已过期")
	}
	result = helperFinishUpdate(dir, req, staged, func() (bool, error) { return helperProcessExited(parent) }, func(staged, target string) error {
		return replaceStagedExecutable(staged, target, true, os.Rename)
	})
	return staged, false, result
}

func helperNeedsElevation(err error, target string) bool {
	var pathErr *os.PathError
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) && errors.As(err, &pathErr) && pathErr.Op == "open" &&
		helperSamePath(filepath.Dir(pathErr.Path), filepath.Dir(target)) &&
		strings.HasPrefix(filepath.Base(pathErr.Path), "."+filepath.Base(target)+".update-")
}

func helperStageDownload(dir string, req updateHelperRequest) (string, error) {
	source := filepath.Join(dir, "readcli.exe")
	info, err := os.Lstat(source)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("下载的更新不是普通文件")
	}
	name, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return "", err
	}
	// Deny writes/deletes while verifying and copying, including across the UAC
	// boundary. The exact verified bytes are copied using this same handle.
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return "", err
	}
	f := os.NewFile(uintptr(handle), source)
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	if hex.EncodeToString(hash.Sum(nil)) != req.SourceHash {
		return "", errors.New("下载文件摘要校验失败")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	if helperCancelled(dir, req) {
		return "", errUpdateHelperCancelled
	}
	return stageExecutable(f, req.Target, os.FileMode(req.Mode)&0777)
}

// ShellExecuteEx uses a separate executable field and Win32 argv quoting. No
// cmd.exe/PowerShell command is constructed, even for Unicode or spaced paths.
type updateShellExecuteInfo struct {
	Size       uint32
	Mask       uint32
	Window     windows.Handle
	Verb       *uint16
	File       *uint16
	Parameters *uint16
	Directory  *uint16
	Show       int32
	Instance   windows.Handle
	IDList     unsafe.Pointer
	Class      *uint16
	ClassKey   windows.Handle
	HotKey     uint32
	Icon       windows.Handle
	Process    windows.Handle
}

func helperLaunchElevated(path string, args []string) (windows.Handle, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err != nil && err != syscall.Errno(1) {
		return 0, fmt.Errorf("初始化管理员授权界面失败: %w", err)
	}
	defer windows.CoUninitialize()
	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	params, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(args))
	if err != nil {
		return 0, err
	}
	info := updateShellExecuteInfo{Mask: 0x40 | 0x100, Verb: verb, File: file, Parameters: params, Show: windows.SW_HIDE}
	info.Size = uint32(unsafe.Sizeof(info))
	result, _, callErr := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW").Call(uintptr(unsafe.Pointer(&info)))
	if result == 0 {
		return 0, callErr
	}
	if info.Process == 0 {
		return 0, errors.New("管理员更新助手没有返回进程句柄")
	}
	return info.Process, nil
}

func helperFailureDialog(message string) {
	text, _ := windows.UTF16PtrFromString(message)
	caption, _ := windows.UTF16PtrFromString("ReadCLI 更新失败")
	_, _ = windows.MessageBox(0, text, caption, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SETFOREGROUND)
}
