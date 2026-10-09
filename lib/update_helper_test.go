package lib

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func helperProtocolFixture(t *testing.T) (string, updateHelperRequest, string, string) {
	t.Helper()
	root, err := helperCanonicalPath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "download")
	install := filepath.Join(root, "安装 目录")
	for _, path := range []string{dir, install} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(install, "readcli.exe")
	source := filepath.Join(dir, "readcli.exe")
	if err := os.WriteFile(target, []byte("old executable"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("new executable"), 0700); err != nil {
		t.Fatal(err)
	}
	req, digest, err := helperCreateRequest(source, target, dir, 42, 123)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	staged, err := stageExecutable(f, target, 0700)
	if err != nil {
		t.Fatal(err)
	}
	return dir, req, digest, staged
}

func helperWaitState(t *testing.T, dir, name, nonce string) updateHelperState {
	t.Helper()
	deadline := time.After(3 * time.Second)
	var lastReadError error
	for {
		state, exists, err := helperReadState(dir, name, nonce)
		if err != nil {
			// Windows may briefly deny reads while an atomic state-file rename
			// completes. Keep waiting within the deadline, but fail other errors.
			if runtime.GOOS != "windows" || (!errors.Is(err, syscall.Errno(32)) && !errors.Is(err, syscall.Errno(33))) {
				t.Fatal(err)
			}
			lastReadError = err
		}
		if exists {
			return state
		}
		select {
		case <-deadline:
			t.Fatalf("missing %s (last read error: %v)", name, lastReadError)
		default:
			time.Sleep(updateHelperPoll)
		}
	}
}

func TestUpdateHelperRequestRejectsTamperingAndPathChanges(t *testing.T) {
	dir, req, digest, _ := helperProtocolFixture(t)
	loaded, loadedDir, err := helperLoadRequest(filepath.Join(dir, updateRequestName), digest)
	if err != nil || loaded != req || loadedDir != dir {
		t.Fatalf("request: %+v %s %v", loaded, loadedDir, err)
	}
	req.Target = filepath.Join(dir, "readcli.exe")
	if err := helperWriteJSON(dir, updateRequestName, req); err != nil {
		t.Fatal(err)
	}
	if _, _, err := helperLoadRequest(filepath.Join(dir, updateRequestName), digest); err == nil {
		t.Fatal("modified request accepted")
	}
	digest, _ = helperFileHash(filepath.Join(dir, updateRequestName))
	if _, _, err := helperLoadRequest(filepath.Join(dir, updateRequestName), digest); err == nil {
		t.Fatal("target inside download directory accepted")
	}
}

func TestUpdateHelperRejectsForgedNonceOrStagedPath(t *testing.T) {
	dir, req, _, staged := helperProtocolFixture(t)
	if err := helperWriteJSON(dir, "ready.json", updateHelperState{Nonce: strings.Repeat("0", 64), Staged: staged}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := helperReadState(dir, "ready.json", req.Nonce); err == nil {
		t.Fatal("wrong nonce accepted")
	}
	if err := helperValidateReady(dir, req, updateHelperState{Staged: filepath.Join(dir, "readcli.exe")}); err == nil {
		t.Fatal("outside staging path accepted")
	}
}

func TestUpdateHelperWaitsForAcceptanceAndProcessExit(t *testing.T) {
	dir, req, _, staged := helperProtocolFixture(t)
	var exited, applied atomic.Bool
	done := make(chan error, 1)
	go func() {
		done <- helperFinishUpdate(dir, req, staged, func() (bool, error) { return exited.Load(), nil }, func(a, b string) error {
			applied.Store(true)
			return replaceStagedExecutable(a, b, true, os.Rename)
		})
	}()
	helperWaitState(t, dir, "ready.json", req.Nonce)
	if err := helperAwaitPrepared(dir, req, func() (bool, error) { return false, nil }); err != nil {
		t.Fatal(err)
	}
	if applied.Load() {
		t.Fatal("replaced the still-running parent")
	}
	if err := helperVerifyHash(req.Target, req.CurrentHash); err != nil {
		t.Fatal(err)
	}
	exited.Store(true)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("helper did not finish")
	}
	if !applied.Load() {
		t.Fatal("accepted update did not apply after exit")
	}
	if err := helperVerifyHash(req.Target, req.SourceHash); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateHelperTimeoutNeverAuthorizesLateInstallation(t *testing.T) {
	dir, req, _, staged := helperProtocolFixture(t)
	req.Deadline = time.Now().Add(-time.Second).UnixNano()
	if err := helperAwaitPrepared(dir, req, func() (bool, error) { return false, nil }); err == nil {
		t.Fatal("expired preparation accepted")
	}
	// Deliberately omit cancel.json: even a failed cancellation write cannot
	// authorize a helper that returns from UAC after the parent timed out.
	if _, exists, _ := helperReadState(dir, "commit.json", req.Nonce); exists {
		t.Fatal("timeout wrote final permission")
	}
	called := false
	err := helperFinishUpdate(dir, req, staged, func() (bool, error) { return true, nil }, func(string, string) error { called = true; return nil })
	if err == nil || called {
		t.Fatal("late helper applied an unaccepted update")
	}
	if err := helperVerifyHash(req.Target, req.CurrentHash); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateHelperConfirmationTimeoutHasNoFinalPermission(t *testing.T) {
	dir, req, _, staged := helperProtocolFixture(t)
	if err := helperWriteJSON(dir, "ready.json", updateHelperState{Nonce: req.Nonce, Staged: staged}); err != nil {
		t.Fatal(err)
	}
	req.Deadline = time.Now().Add(500 * time.Millisecond).UnixNano()
	if err := helperAwaitPrepared(dir, req, func() (bool, error) { return false, nil }); err == nil {
		t.Fatal("missing helper confirmation accepted")
	}
	if _, exists, err := helperReadState(dir, "accepted.json", req.Nonce); err != nil || !exists {
		t.Fatalf("did not reach helper confirmation wait: exists=%v err=%v", exists, err)
	}
	if _, exists, _ := helperReadState(dir, "commit.json", req.Nonce); exists {
		t.Fatal("confirmation timeout authorized installation")
	}
	// The helper comes back after timeout. The parent then exits without a
	// cancel file; the final commit gate must still preserve the old program.
	var calls int
	applied := false
	err := helperFinishUpdate(dir, req, staged, func() (bool, error) { calls++; return calls > 1, nil }, func(string, string) error { applied = true; return nil })
	if err == nil || applied {
		t.Fatal("late confirmation installed without final permission")
	}
}

func TestUpdateHelperUncommittedConfirmationExpires(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "without_final_permission", true: "with_final_permission"}[commit], func(t *testing.T) {
			dir, req, _, staged := helperProtocolFixture(t)
			req.Deadline = time.Now().Add(-updateCommitGrace - time.Second).UnixNano()
			if err := helperWriteJSON(dir, "accepted.json", updateHelperState{Nonce: req.Nonce}); err != nil {
				t.Fatal(err)
			}
			if commit {
				if err := helperWriteJSON(dir, "commit.json", updateHelperState{Nonce: req.Nonce}); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			applied := false
			err := helperFinishUpdate(dir, req, staged, func() (bool, error) {
				calls++
				return commit && calls > 2, nil
			}, func(string, string) error { applied = true; return nil })
			if commit {
				if err != nil || !applied || calls <= 2 {
					t.Fatalf("committed helper did not wait for parent exit: applied=%v calls=%d err=%v", applied, calls, err)
				}
			} else {
				if err == nil || applied || !strings.Contains(err.Error(), "未完成更新确认") {
					t.Fatalf("uncommitted helper did not expire: applied=%v err=%v", applied, err)
				}
				if _, exists, err := helperReadState(dir, "confirmed.json", req.Nonce); err != nil || !exists {
					t.Fatalf("confirmation branch not reached: exists=%v err=%v", exists, err)
				}
			}
			if err := helperVerifyHash(req.Target, req.CurrentHash); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUpdateHelperReadsFinalPermissionAfterObservingParentExit(t *testing.T) {
	dir, req, _, staged := helperProtocolFixture(t)
	if err := helperWriteJSON(dir, "accepted.json", updateHelperState{Nonce: req.Nonce}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	applied := false
	err := helperFinishUpdate(dir, req, staged, func() (bool, error) {
		calls++
		if calls == 1 {
			return false, nil
		}
		// Models the parent committing and exiting between helper polls.
		return true, helperWriteJSON(dir, "commit.json", updateHelperState{Nonce: req.Nonce})
	}, func(string, string) error { applied = true; return nil })
	if err != nil || !applied {
		t.Fatalf("missed final permission at parent exit: %v", err)
	}
}

func TestUpdateHelperCancellationWinsAfterReady(t *testing.T) {
	dir, req, _, staged := helperProtocolFixture(t)
	done := make(chan error, 1)
	var applied atomic.Bool
	go func() {
		done <- helperFinishUpdate(dir, req, staged, func() (bool, error) { return false, nil }, func(string, string) error { applied.Store(true); return nil })
	}()
	helperWaitState(t, dir, "ready.json", req.Nonce)
	if err := helperWriteJSON(dir, "cancel.json", updateHelperState{Nonce: req.Nonce}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, errUpdateHelperCancelled) {
			t.Fatalf("cancel result: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canceled helper hung")
	}
	if applied.Load() {
		t.Fatal("cancellation installed the update")
	}
}

func TestUpdateHelperDetectsConcurrentTargetOrStagingChange(t *testing.T) {
	for _, which := range []string{"target", "staged"} {
		t.Run(which, func(t *testing.T) {
			dir, req, _, staged := helperProtocolFixture(t)
			var exited, applied atomic.Bool
			done := make(chan error, 1)
			go func() {
				done <- helperFinishUpdate(dir, req, staged, func() (bool, error) { return exited.Load(), nil }, func(string, string) error { applied.Store(true); return nil })
			}()
			helperWaitState(t, dir, "ready.json", req.Nonce)
			if err := helperAwaitPrepared(dir, req, func() (bool, error) { return false, nil }); err != nil {
				t.Fatal(err)
			}
			path := req.Target
			if which == "staged" {
				path = staged
			}
			if err := os.WriteFile(path, []byte("changed by another update"), 0700); err != nil {
				t.Fatal(err)
			}
			exited.Store(true)
			select {
			case err := <-done:
				if err == nil || !strings.Contains(err.Error(), "摘要") {
					t.Fatalf("change result: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("helper hung")
			}
			if applied.Load() {
				t.Fatal("helper overwrote concurrent change")
			}
			if _, err := os.Stat(staged); err != nil {
				t.Fatal("failed staging was not retained", err)
			}
		})
	}
}

func TestUpdateHelperFailureKeepsFullCauseAndRecoveryPath(t *testing.T) {
	dir, req, _, staged := helperProtocolFixture(t)
	cause := errors.New(strings.Repeat("long path / ", 40) + "Access is denied")
	helperRecordFailure(dir, req, staged, cause)
	state := helperWaitState(t, dir, "failed.json", req.Nonce)
	log, err := os.ReadFile(filepath.Join(dir, "update.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{cause.Error(), staged, req.Target, filepath.Join(dir, "readcli.exe")} {
		if !strings.Contains(state.Error, text) || !strings.Contains(string(log), text) {
			t.Fatalf("missing recovery detail %q", text)
		}
	}
}
