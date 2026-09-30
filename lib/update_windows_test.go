//go:build windows

package lib

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// This test runs only on Windows. Cross-compiling it does not validate Windows
// image locks; the child keeps the original installed executable running.
func TestWindowsReplaceRunningExecutableTwice(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	executable := filepath.Join(dir, "运行中 readcli.exe")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, data, 0755); err != nil {
		t.Fatal(err)
	}
	fixedBackup := executable + ".old"
	if err := os.WriteFile(fixedBackup, []byte("locked previous backup"), 0600); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(fixedBackup)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(lock) })
	if err := os.Remove(fixedBackup); err == nil {
		t.Fatal("test fixture did not hold a Windows delete lock")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestWindowsUpdateProcessFixture$")
	cmd.Env = append(os.Environ(), "READCLI_UPDATE_PROCESS_FIXTURE=1")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = input.Close()
		if err := cmd.Wait(); err != nil {
			t.Errorf("running executable fixture: %v", err)
		}
	})
	scanner := bufio.NewScanner(output)
	if !scanner.Scan() || scanner.Text() != "ready" {
		t.Fatalf("fixture failed to start: %q, %v", scanner.Text(), scanner.Err())
	}
	for _, payload := range []string{"first new executable", "second new executable"} {
		download := filepath.Join(t.TempDir(), "download.exe")
		if err := os.WriteFile(download, []byte(payload), 0755); err != nil {
			t.Fatal(err)
		}
		if err := replaceExecutable(download, executable); err != nil {
			t.Fatal(err)
		}
		assertUpdateFile(t, executable, payload)
		assertUpdateFile(t, download, payload)
		assertUpdateFile(t, fixedBackup, "locked previous backup")
	}
}

func TestWindowsUpdateProcessFixture(t *testing.T) {
	if os.Getenv("READCLI_UPDATE_PROCESS_FIXTURE") != "1" {
		return
	}
	fmt.Fprintln(os.Stdout, "ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}
