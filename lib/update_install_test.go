package lib

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallReleaseZip(t *testing.T) {
	t.Parallel()
	executable := filepath.Join(t.TempDir(), "自定义 readcli.exe")
	if err := os.WriteFile(executable, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	archive := updateZip(t, "readcli.exe", "new executable")
	client := updateArchiveClient(archive)
	var stages []string
	err := installReleaseAssetWithClient("v0.3.5", "https://update.invalid/readcli.zip", executable, func(p UpdateProgress) {
		stages = append(stages, p.Stage)
	}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertUpdateFile(t, executable, "new executable")
	if len(stages) == 0 || stages[len(stages)-1] != UpdateStageReplace {
		t.Fatalf("update did not reach replacement: %v", stages)
	}
	entries, err := os.ReadDir(filepath.Dir(executable))
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected install directory files: %v, %v", entries, err)
	}
}

func TestInstallReleaseZipFailureRetainsDownload(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		entry   string
		blocked bool
	}{
		{name: "missing executable", entry: "wrong-name.exe"},
		{name: "staging directory unavailable", entry: "readcli.exe", blocked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			executable := filepath.Join(t.TempDir(), "readcli.exe")
			if err := os.WriteFile(executable, []byte("old"), 0755); err != nil {
				t.Fatal(err)
			}
			oldFile := executable
			if tc.blocked {
				// A regular-file parent provides a deterministic filesystem error
				// on every OS, without depending on administrator/root privileges.
				executable = filepath.Join(executable, "nested.exe")
			}
			archive := updateZip(t, tc.entry, "new executable")
			err := installReleaseAssetWithClient("v0.3.5", "https://update.invalid/readcli.zip", executable, nil, updateArchiveClient(archive), nil)
			var installErr *UpdateInstallError
			if !errors.As(err, &installErr) || installErr.TempDir == "" {
				t.Fatalf("missing recovery directory: %v", err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(installErr.TempDir) })
			assertUpdateFile(t, oldFile, "old")
			data, readErr := os.ReadFile(filepath.Join(installErr.TempDir, "readcli-update.zip"))
			if readErr != nil || !bytes.Equal(data, archive) {
				t.Fatalf("download archive not preserved: %v", readErr)
			}
			if tc.blocked {
				assertUpdateFile(t, filepath.Join(installErr.TempDir, "readcli.exe"), "new executable")
				if !strings.Contains(err.Error(), "创建暂存文件失败") || !strings.Contains(err.Error(), oldFile) {
					t.Fatalf("filesystem failure details lost: %v", err)
				}
			}
		})
	}
}

func TestInstallReleaseDeferredPreparation(t *testing.T) {
	t.Parallel()
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "ready", true: "denied"}[fail], func(t *testing.T) {
			t.Parallel()
			executable := filepath.Join(t.TempDir(), "readcli.exe")
			if err := os.WriteFile(executable, []byte("old"), 0755); err != nil {
				t.Fatal(err)
			}
			var retained string
			var lastStage string
			prepare := func(binary, target, tempDir string) error {
				retained = tempDir
				t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
				if target != executable || lastStage != UpdateStagePrepare {
					t.Fatalf("unexpected handoff: target %q stage %q", target, lastStage)
				}
				assertUpdateFile(t, binary, "new")
				if fail {
					return errors.New("Access is denied")
				}
				return nil
			}
			err := installReleaseAssetWithClient("v0.3.5", "https://update.invalid/readcli.zip", executable, func(p UpdateProgress) {
				lastStage = p.Stage
			}, updateArchiveClient(updateZip(t, "readcli.exe", "new")), prepare)
			if (err != nil) != fail {
				t.Fatalf("prepare returned %v; failure wanted = %v", err, fail)
			}
			assertUpdateFile(t, executable, "old")
			assertUpdateFile(t, filepath.Join(retained, "readcli.exe"), "new")
			if fail && (!strings.Contains(err.Error(), "Access is denied") || !strings.Contains(err.Error(), retained)) {
				t.Fatalf("lost preparation error or recovery directory: %v", err)
			}
		})
	}
}

func updateZip(t *testing.T, name, content string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, err := w.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(f, content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

type updateRoundTripper func(*http.Request) (*http.Response, error)

func (f updateRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func updateArchiveClient(archive []byte) *http.Client {
	return &http.Client{Transport: updateRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Body:          io.NopCloser(bytes.NewReader(archive)),
			ContentLength: int64(len(archive)),
			Header:        make(http.Header),
			Request:       r,
		}, nil
	})}
}
