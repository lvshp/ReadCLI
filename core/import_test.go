package core

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/lvshp/ReadCLI/lib"
)

func TestImportBooksFromDirectory(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("READCLI_DATA_DIR", filepath.Join(tempDir, "readcli-test-data"))
	txtPath := filepath.Join(tempDir, "one.txt")
	epubPath := filepath.Join(tempDir, "two.epub")
	otherPath := filepath.Join(tempDir, "ignore.md")
	nestedDir := filepath.Join(tempDir, "nested")
	nestedTxt := filepath.Join(nestedDir, "three.txt")

	if err := os.WriteFile(txtPath, []byte("第1章 开始\n正文"), 0644); err != nil {
		t.Fatalf("write txt: %v", err)
	}
	if err := writeTestZip(epubPath, map[string]string{
		"META-INF/container.xml": `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`,
		"OEBPS/content.opf": `<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf">
  <manifest>
    <item id="chapter-1" href="text/chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter-1"/>
  </spine>
</package>`,
		"OEBPS/text/chapter1.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>Chapter 1</h1><p>Body</p></body></html>`,
	}); err != nil {
		t.Fatalf("write epub: %v", err)
	}
	if err := os.WriteFile(otherPath, []byte("ignore"), 0644); err != nil {
		t.Fatalf("write other: %v", err)
	}
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(nestedTxt, []byte("第2章 继续\n正文"), 0644); err != nil {
		t.Fatalf("write nested txt: %v", err)
	}

	books, err := importBooksFromDirectory(tempDir, false, nil)
	if err != nil {
		t.Fatalf("importBooksFromDirectory() error = %v", err)
	}
	if len(books) != 2 {
		t.Fatalf("len(books) = %d, want 2", len(books))
	}
	app = &appState{bookshelf: &lib.BookshelfStore{}}
	for _, book := range books {
		lib.UpsertBookshelfBook(app.bookshelf, book)
	}
	if got := len(app.bookshelf.Books); got != 2 {
		t.Fatalf("bookshelf len = %d, want 2", got)
	}
}

func TestImportBooksFromDirectoryRecursive(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("READCLI_DATA_DIR", filepath.Join(tempDir, "readcli-test-data"))
	topTxt := filepath.Join(tempDir, "one.txt")
	nestedDir := filepath.Join(tempDir, "nested")
	nestedTxt := filepath.Join(nestedDir, "two.txt")

	if err := os.WriteFile(topTxt, []byte("第1章 开始\n正文"), 0644); err != nil {
		t.Fatalf("write top txt: %v", err)
	}
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(nestedTxt, []byte("第2章 继续\n正文"), 0644); err != nil {
		t.Fatalf("write nested txt: %v", err)
	}

	books, err := importBooksFromDirectory(tempDir, true, nil)
	if err != nil {
		t.Fatalf("importBooksFromDirectory() error = %v", err)
	}
	if len(books) != 2 {
		t.Fatalf("len(books) = %d, want 2", len(books))
	}
	app = &appState{bookshelf: &lib.BookshelfStore{}}
	for _, book := range books {
		lib.UpsertBookshelfBook(app.bookshelf, book)
	}
	if got := len(app.bookshelf.Books); got != 2 {
		t.Fatalf("bookshelf len = %d, want 2", got)
	}
}

func TestCollectImportCandidatesNonRecursive(t *testing.T) {
	tempDir := t.TempDir()
	topTxt := filepath.Join(tempDir, "one.txt")
	nestedDir := filepath.Join(tempDir, "nested")
	nestedTxt := filepath.Join(nestedDir, "two.txt")

	if err := os.WriteFile(topTxt, []byte("第1章 开始\n正文"), 0644); err != nil {
		t.Fatalf("write top txt: %v", err)
	}
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(nestedTxt, []byte("第2章 继续\n正文"), 0644); err != nil {
		t.Fatalf("write nested txt: %v", err)
	}

	paths, err := collectImportCandidates(tempDir, false)
	if err != nil {
		t.Fatalf("collectImportCandidates() error = %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("len(paths) = %d, want 1", len(paths))
	}
	if paths[0] != topTxt {
		t.Fatalf("paths[0] = %q, want %q", paths[0], topTxt)
	}
}

func TestNormalizeImportInputPath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain", input: "/Users/lvsp/Books/demo.epub", want: "/Users/lvsp/Books/demo.epub"},
		{name: "double quoted", input: "\"/Users/lvsp/Books/My Book.epub\"", want: "/Users/lvsp/Books/My Book.epub"},
		{name: "single quoted", input: "'/Users/lvsp/Books/My Book.epub'", want: "/Users/lvsp/Books/My Book.epub"},
		{name: "escaped spaces", input: "/Users/lvsp/My\\ Books/demo\\ file.txt", want: "/Users/lvsp/My Books/demo file.txt"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeImportInputPath(tc.input); got != tc.want {
				t.Fatalf("normalizeImportInputPath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func writeTestZip(path string, files map[string]string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	zw := zip.NewWriter(file)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			_ = zw.Close()
			return err
		}
		if _, err := w.Write([]byte(content)); err != nil {
			_ = zw.Close()
			return err
		}
	}
	return zw.Close()
}
