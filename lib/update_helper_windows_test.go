//go:build windows

package lib

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestMain(m *testing.M) {
	if handled, code := RunUpdateHelper(os.Args[1:]); handled {
		os.Exit(code)
	}
	os.Exit(m.Run())
}

// This fixture is copied and launched as the installed program. Its helper is
// therefore a real separate process with a real parent wait handle. No UAC is
// needed in the writable test directory; UAC acceptance needs manual Windows QA.
func TestWindowsDeferredUpdateParentFixture(t *testing.T) {
	if os.Getenv("READCLI_DEFERRED_UPDATE_FIXTURE") != "1" {
		return
	}
	dir := os.Getenv("READCLI_DEFERRED_UPDATE_DIR")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := startWindowsDeferredUpdate(filepath.Join(dir, "readcli.exe"), exe, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "parent-prepared"), []byte("prepared"), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "allow-parent-exit")); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture was not released")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWindowsDeferredUpdateWaitsForRealParentExit(t *testing.T) {
	root := t.TempDir()
	install := filepath.Join(root, "安装 with spaces")
	dir := filepath.Join(root, "readcli-update-fixture")
	for _, path := range []string{install, dir} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	current, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(install, "readcli.exe")
	source := filepath.Join(dir, "readcli.exe")
	if err := helperCopySelf(current, old); err != nil {
		t.Fatal(err)
	}
	if err := helperCopySelf(current, source); err != nil {
		t.Fatal(err)
	}
	// PE executables permit an overlay. This gives the payload a distinct hash
	// while retaining a runnable test entry point for the post-update smoke check.
	f, err := os.OpenFile(source, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("readcli-new-version-fixture")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	oldHash, _ := helperFileHash(old)
	newHash, _ := helperFileHash(source)
	cmd := exec.Command(old, "-test.run=^TestWindowsDeferredUpdateParentFixture$")
	cmd.Env = append(os.Environ(), "READCLI_DEFERRED_UPDATE_FIXTURE=1", "READCLI_DEFERRED_UPDATE_DIR="+dir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "parent-prepared")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("parent never prepared")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := helperVerifyHash(old, oldHash); err != nil {
		t.Fatal("running exe changed before exit", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "allow-parent-exit"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	// Observe the protocol instead of repeatedly opening the target EXE:
	// os.Open on Windows does not share deletes, so hashing while the helper
	// renames the executable can make this test itself prevent installation.
	deadline = time.Now().Add(15 * time.Second)
	for {
		if failure, err := os.ReadFile(filepath.Join(dir, "failed.json")); err == nil {
			log, _ := os.ReadFile(filepath.Join(dir, "update.log"))
			t.Fatalf("helper failed: %s\n%s", failure, log)
		}
		if _, err := os.Stat(filepath.Join(dir, "installed.json")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			log, _ := os.ReadFile(filepath.Join(dir, "update.log"))
			t.Fatalf("helper did not replace exited program: %s", log)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := helperVerifyHash(old, newHash); err != nil {
		t.Fatal("installed executable differs from downloaded payload", err)
	}
	if output, err := exec.Command(old, "-test.run=^TestUpdateHelperRequestRejectsTamperingAndPathChanges$").CombinedOutput(); err != nil {
		t.Fatalf("updated executable failed: %v\n%s", err, output)
	}
	// Wait for the helper to exit so Windows permits TempDir cleanup.
	for {
		if err := os.Remove(filepath.Join(dir, updateHelperName)); err == nil || os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper remained running after installation")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWindowsHelperUACArgumentEncodingAndLayout(t *testing.T) {
	args := []string{updateHelperFlag, `D:\安装 目录\request.json`, strings.Repeat("a", 64), "--elevated"}
	encoded := windows.ComposeCommandLine(args)
	decoded, err := windows.DecomposeCommandLine(encoded)
	if err != nil || len(decoded) != len(args) {
		t.Fatalf("argv: %v %v", decoded, err)
	}
	for i := range args {
		if args[i] != decoded[i] {
			t.Fatalf("argument %d changed", i)
		}
	}
	if unsafe.Sizeof(uintptr(0)) == 8 && unsafe.Sizeof(updateShellExecuteInfo{}) != 112 {
		t.Fatalf("SHELLEXECUTEINFO size=%d", unsafe.Sizeof(updateShellExecuteInfo{}))
	}
}

func TestWindowsHelperElevatesOnlyForProtectedStagingDirectory(t *testing.T) {
	target := `D:\Program Files\ReadCLI\readcli.exe`
	denied := &os.PathError{Op: "open", Path: `D:\Program Files\ReadCLI\.readcli.exe.update-123`, Err: windows.ERROR_ACCESS_DENIED}
	if !helperNeedsElevation(denied, target) {
		t.Fatal("protected staging did not request elevation")
	}
	denied.Path = `C:\Temp\readcli-update-123\readcli.exe`
	if helperNeedsElevation(denied, target) {
		t.Fatal("source read failure unnecessarily requested elevation")
	}
	denied.Path = `D:\Program Files\ReadCLI\.readcli.exe.update-123`
	denied.Err = windows.ERROR_SHARING_VIOLATION
	if helperNeedsElevation(denied, target) {
		t.Fatal("sharing violation unnecessarily requested elevation")
	}
}
