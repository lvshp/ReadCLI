package lib

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReplaceExecutablePreservesDownload(t *testing.T) {
	t.Parallel()
	download := filepath.Join(t.TempDir(), "download.exe")
	executable := filepath.Join(t.TempDir(), "readcli.exe")
	if err := os.WriteFile(download, []byte("new executable"), 0751); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("old executable"), 0751); err != nil {
		t.Fatal(err)
	}
	if err := replaceExecutable(download, executable); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{download, executable} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "new executable" {
			t.Fatalf("read %s = %q, %v; want complete new executable", path, got, err)
		}
	}
}

func TestStageExecutableCompletesBesideInstalledFile(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "程序 安装目录")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(dir, "readcli.exe")
	path, err := stageExecutable(strings.NewReader("complete update"), executable, 0751)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != dir {
		t.Fatalf("staging directory = %q; want %q", filepath.Dir(path), dir)
	}
	assertUpdateFile(t, path, "complete update")
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0751 {
		t.Fatalf("staging mode = %v; want 0751", info.Mode())
	}
	// A rename must be possible immediately; no writer remains open.
	if err := os.Rename(path, executable); err != nil {
		t.Fatalf("rename closed staging file: %v", err)
	}
	assertUpdateFile(t, executable, "complete update")
}

func TestStageExecutableReadFailureLeavesOldFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	executable := filepath.Join(dir, "readcli.exe")
	if err := os.WriteFile(executable, []byte("old executable"), 0755); err != nil {
		t.Fatal(err)
	}
	readErr := errors.New("download read interrupted")
	reader := io.MultiReader(strings.NewReader("incomplete update"), updateErrorReader{readErr})
	if _, err := stageExecutable(reader, executable, 0755); !errors.Is(err, readErr) {
		t.Fatalf("stage error = %v; want read error", err)
	}
	assertUpdateFile(t, executable, "old executable")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "readcli.exe" {
		t.Fatalf("incomplete staging file leaked: %v, %v", entries, err)
	}
}

func TestReplaceStagedExecutableWindowsRecovery(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		failCalls    map[int]error
		wantOld      string
		wantBackup   bool
		wantNewStage bool
	}{
		{name: "success", wantOld: "new", failCalls: map[int]error{}},
		{name: "backup failure", wantOld: "old", wantNewStage: true, failCalls: map[int]error{1: errors.New("sharing violation")}},
		{name: "install failure restored", wantOld: "old", wantNewStage: true, failCalls: map[int]error{2: errors.New("install denied")}},
		{name: "rollback failure retained", wantBackup: true, wantNewStage: true, failCalls: map[int]error{2: errors.New("install denied"), 3: errors.New("rollback denied")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			executable := filepath.Join(dir, "readcli.exe")
			fixedBackup := executable + ".old"
			for path, content := range map[string]string{executable: "old", fixedBackup: "previous update"} {
				if err := os.WriteFile(path, []byte(content), 0755); err != nil {
					t.Fatal(err)
				}
			}
			staged, err := stageExecutable(strings.NewReader("new"), executable, 0755)
			if err != nil {
				t.Fatal(err)
			}
			var backup string
			calls := 0
			rename := func(from, to string) error {
				calls++
				// A different-volume TEMP source must never reach rename.
				if filepath.Dir(from) != dir || filepath.Dir(to) != dir {
					t.Fatalf("rename crosses install directory: %q -> %q", from, to)
				}
				if calls == 1 {
					backup = to
					if backup == fixedBackup {
						t.Fatal("reused fixed .old backup")
					}
				}
				if err := tc.failCalls[calls]; err != nil {
					return err
				}
				return os.Rename(from, to)
			}
			err = replaceStagedExecutable(staged, executable, true, rename)
			if (err != nil) != (len(tc.failCalls) != 0) {
				t.Fatalf("replace error = %v; fail calls = %v", err, tc.failCalls)
			}
			for _, cause := range tc.failCalls {
				if !errors.Is(err, cause) {
					t.Fatalf("missing original error %v in %v", cause, err)
				}
			}
			assertUpdateFile(t, fixedBackup, "previous update")
			if tc.wantOld != "" {
				assertUpdateFile(t, executable, tc.wantOld)
			} else if _, statErr := os.Stat(executable); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("unexpected executable after rollback failure: %v", statErr)
			}
			if tc.wantBackup {
				assertUpdateFile(t, backup, "old")
				if !strings.Contains(err.Error(), backup) {
					t.Fatalf("missing recovery path in %v", err)
				}
			} else if _, statErr := os.Stat(backup); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("unexpected backup after completion: %v", statErr)
			}
			if tc.wantNewStage {
				assertUpdateFile(t, staged, "new")
			} else if _, statErr := os.Stat(staged); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("staging file should have moved: %v", statErr)
			}
		})
	}
}

type updateErrorReader struct{ err error }

func (r updateErrorReader) Read([]byte) (int, error) { return 0, r.err }

func assertUpdateFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != want {
		t.Fatalf("read %s = %q, %v; want %q", path, data, err, want)
	}
}
