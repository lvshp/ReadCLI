package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCachedReaderForPathReusesReaderWhenFileUnchanged(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("READCLI_DATA_DIR", filepath.Join(tempDir, ".readcli-test"))
	path := filepath.Join(tempDir, "book.txt")
	if err := os.WriteFile(path, []byte("第1章 开始\n正文"), 0644); err != nil {
		t.Fatalf("write txt: %v", err)
	}

	app = &appState{readerCache: map[string]cachedReader{}}

	first, cached, err := cachedReaderForPath(path)
	if err != nil {
		t.Fatalf("cachedReaderForPath() error = %v", err)
	}
	if cached {
		t.Fatalf("first load should not be cached")
	}

	second, cached, err := cachedReaderForPath(path)
	if err != nil {
		t.Fatalf("cachedReaderForPath() error = %v", err)
	}
	if !cached {
		t.Fatalf("second load should reuse cache")
	}
	if first != second {
		t.Fatalf("expected cached reader reuse")
	}
}

func TestCachedReaderForPathReloadsWhenFileChanges(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("READCLI_DATA_DIR", filepath.Join(tempDir, ".readcli-test"))
	path := filepath.Join(tempDir, "book.txt")
	if err := os.WriteFile(path, []byte("第1章 开始\n正文"), 0644); err != nil {
		t.Fatalf("write txt: %v", err)
	}

	app = &appState{readerCache: map[string]cachedReader{}}

	first, cached, err := cachedReaderForPath(path)
	if err != nil {
		t.Fatalf("cachedReaderForPath() error = %v", err)
	}
	if cached {
		t.Fatalf("first load should not be cached")
	}

	if err := os.WriteFile(path, []byte("第1章 开始\n正文\n新增内容"), 0644); err != nil {
		t.Fatalf("rewrite txt: %v", err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes txt: %v", err)
	}

	second, cached, err := cachedReaderForPath(path)
	if err != nil {
		t.Fatalf("cachedReaderForPath() error = %v", err)
	}
	if cached {
		t.Fatalf("changed file should not reuse cache")
	}
	if first == second {
		t.Fatalf("expected changed file to be reloaded")
	}
}
