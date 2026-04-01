package lib

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveProgress(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("READCLI_DATA_DIR", filepath.Join(tempHome, ".readcli-test"))

	store, err := LoadProgress()
	if err != nil {
		t.Fatalf("load progress: %v", err)
	}

	store.Books["/tmp/book.epub"] = 42
	if err := SaveProgress(store); err != nil {
		t.Fatalf("save progress: %v", err)
	}

	reloaded, err := LoadProgress()
	if err != nil {
		t.Fatalf("reload progress: %v", err)
	}

	if got := reloaded.Books["/tmp/book.epub"]; got != 42 {
		t.Fatalf("saved progress = %d, want 42", got)
	}

	configFile, err := progressFilePath()
	if err != nil {
		t.Fatalf("progress path: %v", err)
	}

	if _, err := os.Stat(configFile); err != nil {
		t.Fatalf("progress file missing: %v", err)
	}
}
