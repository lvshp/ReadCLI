package core

import (
	"encoding/json"
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

func TestCachedReaderForPathRestoresEPUBFromDiskSnapshot(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("READCLI_DATA_DIR", filepath.Join(tempDir, ".readcli-test"))
	path := filepath.Join(tempDir, "book.epub")
	writeTestEpub(t, path, "第一章", "正文 A")

	app = &appState{
		readerCache:  map[string]cachedReader{},
		readingState: readingState{contentWidth: 48},
	}

	first, cached, err := cachedReaderForPath(path)
	if err != nil {
		t.Fatalf("cachedReaderForPath() error = %v", err)
	}
	if cached {
		t.Fatalf("first load should not be cached")
	}

	app.readerCache = map[string]cachedReader{}
	second, cached, err := cachedReaderForPath(path)
	if err != nil {
		t.Fatalf("cachedReaderForPath() error = %v", err)
	}
	if !cached {
		t.Fatalf("second load should hit disk snapshot")
	}
	if first == second {
		t.Fatalf("disk snapshot should restore a new reader instance")
	}
	if second.BookTitle() != first.BookTitle() {
		t.Fatalf("restored reader title = %q, want %q", second.BookTitle(), first.BookTitle())
	}
}

func TestCachedReaderForPathInvalidatesEPUBDiskSnapshotOnFileChange(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("READCLI_DATA_DIR", filepath.Join(tempDir, ".readcli-test"))
	path := filepath.Join(tempDir, "book.epub")
	writeTestEpub(t, path, "第一章", "正文 A")

	app = &appState{
		readerCache:  map[string]cachedReader{},
		readingState: readingState{contentWidth: 48},
	}

	first, cached, err := cachedReaderForPath(path)
	if err != nil {
		t.Fatalf("cachedReaderForPath() error = %v", err)
	}
	if cached {
		t.Fatalf("first load should not be cached")
	}

	writeTestEpub(t, path, "第一章", "正文 B")
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes epub: %v", err)
	}

	app.readerCache = map[string]cachedReader{}
	second, cached, err := cachedReaderForPath(path)
	if err != nil {
		t.Fatalf("cachedReaderForPath() error = %v", err)
	}
	if cached {
		t.Fatalf("changed epub should rebuild instead of using stale snapshot")
	}
	if first == second {
		t.Fatalf("changed epub should not reuse previous reader")
	}
}

func TestCachedReaderForPathFallsBackWhenSnapshotCorrupted(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("READCLI_DATA_DIR", filepath.Join(tempDir, ".readcli-test"))
	path := filepath.Join(tempDir, "book.epub")
	writeTestEpub(t, path, "第一章", "正文 A")

	app = &appState{
		readerCache:  map[string]cachedReader{},
		readingState: readingState{contentWidth: 48},
	}

	_, cached, err := cachedReaderForPath(path)
	if err != nil {
		t.Fatalf("cachedReaderForPath() error = %v", err)
	}
	if cached {
		t.Fatalf("first load should not be cached")
	}

	cachePath, err := diskSnapshotPath(path)
	if err != nil {
		t.Fatalf("diskSnapshotPath() error = %v", err)
	}
	if err := os.WriteFile(cachePath, []byte("{broken"), 0644); err != nil {
		t.Fatalf("write broken snapshot: %v", err)
	}

	app.readerCache = map[string]cachedReader{}
	second, cached, err := cachedReaderForPath(path)
	if err != nil {
		t.Fatalf("cachedReaderForPath() error = %v", err)
	}
	if cached {
		t.Fatalf("corrupted snapshot should fall back to fresh parse")
	}
	if second == nil {
		t.Fatalf("expected fallback reader")
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read repaired snapshot: %v", err)
	}
	var entry diskSnapshot
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("repaired snapshot should be valid json: %v", err)
	}
	if entry.Version != epubSnapshotCacheVersion {
		t.Fatalf("snapshot version = %d, want %d", entry.Version, epubSnapshotCacheVersion)
	}
}

func writeTestEpub(t *testing.T, path, chapterTitle, body string) {
	t.Helper()
	if err := writeTestZip(path, map[string]string{
		"META-INF/container.xml": `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`,
		"OEBPS/content.opf": `<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <metadata>
    <dc:title>缓存测试</dc:title>
  </metadata>
  <manifest>
    <item id="chapter-1" href="text/chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter-1"/>
  </spine>
</package>`,
		"OEBPS/text/chapter1.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>` + chapterTitle + `</h1><p>` + body + `</p></body></html>`,
	}); err != nil {
		t.Fatalf("write epub: %v", err)
	}
}
