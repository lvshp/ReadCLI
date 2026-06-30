package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lvshp/ReadCLI/lib"
	"github.com/rivo/tview"
)

func TestOpenBookMoveAndPersistState(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("READCLI_DATA_DIR", filepath.Join(tempDir, ".readcli-test"))
	bookPath := filepath.Join(tempDir, "demo.txt")
	if err := os.WriteFile(bookPath, []byte("第1章 开始\n第一段\n第二段\n第三段"), 0644); err != nil {
		t.Fatalf("write txt: %v", err)
	}

	initTestUIState()
	app.config = &lib.Config{Theme: "vscode", DisplayLines: 3, ShowBorder: true}
	app.readingState.displayLines = 3
	app.progress = &lib.ProgressStore{Books: map[string]int{}, Anchors: map[string]lib.ProgressAnchor{}}

	if err := openBook(bookPath); err != nil {
		t.Fatalf("openBook() error = %v", err)
	}
	if app.mode != modeReading {
		t.Fatalf("mode = %s, want %s", app.mode, modeReading)
	}
	if app.currentFile != normalizeBookPath(bookPath) {
		t.Fatalf("currentFile = %q, want %q", app.currentFile, normalizeBookPath(bookPath))
	}

	moveReading(1)
	persistState()

	if got := app.progress.Books[app.currentFile]; got <= 0 {
		t.Fatalf("progress = %d, want > 0", got)
	}
	if app.currentBook == nil || app.currentBook.LastReadAt == "" {
		t.Fatalf("currentBook should be updated after reading")
	}
}

func TestImportDeleteAndDirectoryImportFlows(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("READCLI_DATA_DIR", filepath.Join(tempDir, ".readcli-test"))
	bookPath := filepath.Join(tempDir, "demo.txt")
	dirPath := filepath.Join(tempDir, "library")
	if err := os.WriteFile(bookPath, []byte("第1章 开始\n正文"), 0644); err != nil {
		t.Fatalf("write txt: %v", err)
	}
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		t.Fatalf("mkdir library: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirPath, "one.txt"), []byte("第1章 一\n正文"), 0644); err != nil {
		t.Fatalf("write dir txt: %v", err)
	}

	initTestUIState()
	app.uiState.input.value = bookPath
	importBook()
	if len(app.bookshelf.Books) != 1 {
		t.Fatalf("bookshelf len = %d, want 1", len(app.bookshelf.Books))
	}

	app.bookshelfState.shelfIndex = 0
	prepareDeleteSelectedBook()
	if app.mode != modeDeleteConfirm {
		t.Fatalf("mode = %s, want %s", app.mode, modeDeleteConfirm)
	}
	removeSelectedBook(false)
	if len(app.bookshelf.Books) != 0 {
		t.Fatalf("bookshelf len = %d, want 0 after delete", len(app.bookshelf.Books))
	}

	runDirectoryImport(dirPath, false)
	if len(app.bookshelf.Books) != 1 {
		t.Fatalf("bookshelf len = %d, want 1 after directory import", len(app.bookshelf.Books))
	}
}

func TestDispatchSettingsAndChromeFlows(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("READCLI_DATA_DIR", filepath.Join(tempDir, ".readcli-test"))

	initTestUIState()
	app.config = &lib.Config{
		Theme:                    "vscode",
		ShowBorder:               true,
		DisplayLines:             4,
		CompactMode:              false,
		ReadingContentWidthRatio: 0.75,
		ReadingMarginLeft:        1,
		ReadingMarginRight:       1,
		ReadingMarginTop:         1,
		ReadingMarginBottom:      0,
		ReadingLineSpacing:       1,
		ReadingTextColor:         "#FFFFFF",
		AutoPageIntervalMs:       3500,
	}
	app.uiState.showBorder = true
	app.uiState.sessionStart = time.Now().Add(-2 * time.Minute)
	app.readingState.displayLines = 4
	app.reader = &fakeReader{total: 20, chapter: "第一章", toc: "目录\n一\n二"}
	app.currentFile = "/tmp/demo.txt"
	app.bookshelf.Books = []lib.BookshelfBook{{Title: "一本书", Path: "/tmp/demo.txt", Format: "txt"}}

	dispatchEvent("o")
	dispatchEvent("r")
	dispatchEvent("/")
	if app.mode != modeBookshelfSearchInput {
		t.Fatalf("mode = %s, want %s", app.mode, modeBookshelfSearchInput)
	}
	app.uiState.input.value = "一本"
	dispatchEvent("<Enter>")

	transitionTo(modeReading)
	dispatchEvent("?")
	dispatchEvent("p")
	dispatchEvent("m")
	if app.mode != modeTOC {
		t.Fatalf("mode = %s, want %s", app.mode, modeTOC)
	}
	dispatchEvent("m")
	openReadingSettings()
	adjustReadingSetting(1)
	app.readingState.settingsIndex = 7
	activateReadingSetting()
	app.uiState.input.value = "#ABCDEF"
	applyReadingTextColorInput()
	cycleReadingColorPreset()
	toggleBorder()
	toggleCompactMode()
	displayBossKey()

	setStatus(statusInfo, "短提示")
	app.uiState.statusMessageUntil = time.Now().Add(-time.Second)
	refreshChrome()
	if app.uiState.statusMessage != "" {
		t.Fatalf("statusMessage = %q, want cleared", app.uiState.statusMessage)
	}
}

func initTestUIState() {
	app = &appState{
		mode:        modeHome,
		config:      &lib.Config{Theme: "vscode", DisplayLines: 4, ShowBorder: true},
		bookshelf:   &lib.BookshelfStore{},
		bookmarks:   &lib.BookmarkStore{Books: map[string][]lib.Bookmark{}},
		progress:    &lib.ProgressStore{Books: map[string]int{}, Anchors: map[string]lib.ProgressAnchor{}},
		readerCache: map[string]cachedReader{},
		themeOrder:  []string{"vscode", "jetbrains", "ops-console"},
		bookshelfState: bookshelfState{
			sortMode:   "recent",
			filterMode: "all",
		},
		readingState: readingState{
			displayLines: 4,
		},
		uiState: uiState{
			showBorder:   true,
			sessionStart: time.Now(),
		},
	}

	initWidgets()
	midRow = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(left, 24, 0, false).
		AddItem(main, 0, 1, true).
		AddItem(right, 28, 0, false)
	root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 4, 0, false).
		AddItem(midRow, 0, 1, true).
		AddItem(footer, 4, 0, false)
	lastTermWidth = 120
	lastTermHeight = 30
	mainContentWidth = 80
	mainContentHeight = 24
}
