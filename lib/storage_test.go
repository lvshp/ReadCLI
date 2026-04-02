package lib

import (
	"path/filepath"
	"testing"
)

func TestConfigRoundTrip(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("READCLI_DATA_DIR", filepath.Join(tempHome, ".readcli-test"))

	cfg := &Config{
		Theme:                    "jetbrains",
		DisplayLines:             12,
		ShowBorder:               false,
		SkippedUpdateVersion:     "v0.1.9",
		BossKeyCommand:           "  /usr/local/bin/genact --stdin  ",
		ForceBasicColor:          true,
		AutoPageIntervalMs:       4200,
		ReadingContentWidthRatio: 0.8,
		ReadingMarginLeft:        3,
		ReadingMarginTop:         2,
		ReadingLineSpacing:       2,
		ReadingTextColor:         "#ABCDEF",
		ReadingHighContrast:      false,
	}
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
	if loaded.SkippedUpdateVersion != "v0.1.9" {
		t.Fatalf("SkippedUpdateVersion = %q, want v0.1.9", loaded.SkippedUpdateVersion)
	}
	if loaded.BossKeyCommand != "/usr/local/bin/genact --stdin" {
		t.Fatalf("BossKeyCommand = %q, want /usr/local/bin/genact --stdin", loaded.BossKeyCommand)
	}
	if !loaded.ForceBasicColor {
		t.Fatalf("ForceBasicColor = false, want true")
	}
	if loaded.AutoPageIntervalMs != 4200 {
		t.Fatalf("AutoPageIntervalMs = %d, want 4200", loaded.AutoPageIntervalMs)
	}
	if loaded.ReadingContentWidthRatio != 0.8 || loaded.ReadingMarginLeft != 3 || loaded.ReadingMarginTop != 2 {
		t.Fatalf("unexpected reading layout config: %#v", loaded)
	}
	if loaded.ReadingLineSpacing != 2 || loaded.ReadingTextColor != "#ABCDEF" || loaded.ReadingHighContrast {
		t.Fatalf("unexpected reading style config: %#v", loaded)
	}
}

func TestLoadConfigSanitizesInvalidReadingOptions(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	dataDir := filepath.Join(tempHome, ".readcli-test")
	t.Setenv("READCLI_DATA_DIR", dataDir)

	cfg := &Config{
		Theme:                    "vscode",
		DisplayLines:             -5,
		ShowBorder:               true,
		AutoPageIntervalMs:       -1,
		ReadingContentWidthRatio: 2,
		ReadingMarginLeft:        -1,
		ReadingMarginRight:       -2,
		ReadingMarginTop:         -3,
		ReadingMarginBottom:      -4,
		ReadingLineSpacing:       -1,
		ReadingTextColor:         "999,999,999",
	}
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.DisplayLines != 8 {
		t.Fatalf("DisplayLines = %d, want 8", loaded.DisplayLines)
	}
	if loaded.ReadingContentWidthRatio != 0.75 {
		t.Fatalf("ReadingContentWidthRatio = %v, want 0.75", loaded.ReadingContentWidthRatio)
	}
	if loaded.AutoPageIntervalMs != 3500 {
		t.Fatalf("AutoPageIntervalMs = %d, want 3500", loaded.AutoPageIntervalMs)
	}
	if loaded.ReadingMarginLeft != 2 || loaded.ReadingMarginRight != 0 || loaded.ReadingMarginTop != 1 || loaded.ReadingMarginBottom != 0 {
		t.Fatalf("unexpected margin defaults: %#v", loaded)
	}
	if loaded.ReadingLineSpacing != 1 {
		t.Fatalf("ReadingLineSpacing = %d, want 1", loaded.ReadingLineSpacing)
	}
	if loaded.ReadingTextColor != "#FFFFFF" {
		t.Fatalf("ReadingTextColor = %q, want #FFFFFF", loaded.ReadingTextColor)
	}
}

func TestNormalizeConfiguredColor(t *testing.T) {
	cases := map[string]string{
		"#abc":      "#ABC",
		"#abcdef":   "#ABCDEF",
		"12, 34,56": "12,34,56",
		"255,255,0": "255,255,0",
		"bad":       "",
		"256,0,0":   "",
		"#12":       "",
		"#GGGGGG":   "",
		"1,2":       "",
	}

	for input, want := range cases {
		if got := normalizeConfiguredColor(input); got != want {
			t.Fatalf("normalizeConfiguredColor(%q) = %q, want %q", input, got, want)
		}
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
