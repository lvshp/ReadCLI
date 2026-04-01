package lib

import (
	"path/filepath"
	"testing"
)

func TestConfigRoundTrip(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("READCLI_DATA_DIR", filepath.Join(tempHome, ".readcli-test"))

	cfg := &Config{Theme: "jetbrains", DisplayLines: 12, ShowBorder: false}
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Theme != "jetbrains" || loaded.DisplayLines != 12 || loaded.ShowBorder {
		t.Fatalf("unexpected config: %#v", loaded)
	}
}

func TestBookshelfUpsertAndFind(t *testing.T) {
	store := &BookshelfStore{}
	book := BookshelfBook{
		Path:        filepath.Join(t.TempDir(), "book.epub"),
		Title:       "Book",
		Format:      "epub",
		ProgressPos: 10,
	}
	UpsertBookshelfBook(store, book)
	book.ProgressPos = 20
	UpsertBookshelfBook(store, book)

	if len(store.Books) != 1 {
		t.Fatalf("expected one book, got %d", len(store.Books))
	}

	found, ok := FindBookshelfBook(store, book.Path)
	if !ok || found.ProgressPos != 20 {
		t.Fatalf("unexpected book: %#v, ok=%v", found, ok)
	}
}

func TestFilterBooks(t *testing.T) {
	books := []BookshelfBook{
		{Title: "A", Format: "epub", ProgressPos: 0, ProgressPercent: 0},
		{Title: "B", Format: "txt", ProgressPos: 10, ProgressPercent: 30},
		{Title: "C", Format: "epub", ProgressPos: 99, ProgressPercent: 100},
	}

	if got := len(FilterBooks(books, "epub")); got != 2 {
		t.Fatalf("epub filter count = %d, want 2", got)
	}
	if got := len(FilterBooks(books, "reading")); got != 1 {
		t.Fatalf("reading filter count = %d, want 1", got)
	}
	if got := len(FilterBooks(books, "finished")); got != 1 {
		t.Fatalf("finished filter count = %d, want 1", got)
	}
}
