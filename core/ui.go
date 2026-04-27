package core

import (
	"log"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lvshp/ReadCLI/lib"
	"github.com/rivo/tview"
)

func Run(initialFile string, requestedLines int, version string) {
	cfg, _ := lib.LoadConfig()
	shelf, _ := lib.LoadBookshelf()
	marks, _ := lib.LoadBookmarks()
	progress, _ := lib.LoadProgress()

	if cfg == nil {
		cfg = &lib.Config{Theme: "vscode", DisplayLines: 8, ShowBorder: true}
	}
	if requestedLines > 0 {
		cfg.DisplayLines = requestedLines
	}
	if cfg.DisplayLines < 1 {
		cfg.DisplayLines = 8
	}
	if cfg.Theme == "" {
		cfg.Theme = "vscode"
	}

	app = &appState{
		mode:            modeHome,
		config:          cfg,
		bookshelf:       shelf,
		bookmarks:       marks,
		progress:        progress,
		readerCache:     map[string]cachedReader{},
		themeOrder:      []string{"vscode", "jetbrains", "ops-console"},
		sortMode:        "recent",
		filterMode:      "all",
		importRecursive: false,
		shelfIndex:      cfg.SelectedBookshelf,
		lastSearchIndex: -1,
		sessionStart:    time.Now(),
		showBorder:      cfg.ShowBorder,
		compactMode:     cfg.CompactMode,
		displayLines:    cfg.DisplayLines,
		currentVersion:  strings.TrimSpace(version),
		updateMessages:  make(chan updateMessage, 2),
	}

	if app.bookshelf == nil {
		app.bookshelf = &lib.BookshelfStore{}
	}
	if app.bookmarks == nil {
		app.bookmarks = &lib.BookmarkStore{Books: map[string][]lib.Bookmark{}}
	}
	if app.progress == nil {
		app.progress = &lib.ProgressStore{Books: map[string]int{}}
	}
	if app.bookmarks.Books == nil {
		app.bookmarks.Books = map[string][]lib.Bookmark{}
	}

	initWidgets()
	refreshChrome()

	// Build layout: header + mid(row: left + main + right) + footer
	midRow = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(left, 24, 0, false).
		AddItem(main, 0, 1, true).
		AddItem(right, 28, 0, false)

	root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 4, 0, false).
		AddItem(midRow, 0, 1, true).
		AddItem(footer, 4, 0, false)

	// Detect resize via root's DrawFunc
	root.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		if width != lastTermWidth || height != lastTermHeight {
			lastTermWidth = width
			lastTermHeight = height
			applyLayout(width, height)
		}
		return x, y, width, height
	})

	tApp = tview.NewApplication().SetRoot(root, true).EnableMouse(true)
	tApp.SetMouseCapture(func(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
		return nil, 0
	})

	tApp.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		id := tcellKeyEventID(ev)
		if id == "" {
			return nil
		}

		switch app.mode {
		case modeHome:
			handleHomeEvent(id)
		case modeReading:
			handleReadingEvent(id)
		case modeTOC:
			handleTOCEvent(id)
		case modeBookmarks:
			handleBookmarkEvent(id)
		case modeSearchInput:
			handleTextInputEvent(id, runSearch)
		case modeImportInput:
			handleTextInputEvent(id, importBook)
		case modeReadingSettings:
			handleReadingSettingsEvent(id)
		case modeReadingColorInput:
			handleTextInputEvent(id, applyReadingTextColorInput)
		case modeDeleteConfirm:
			handleDeleteConfirmEvent(id)
		case modeUpdatePrompt:
			if !scrollUpdatePrompt(id) {
				handleUpdatePromptEvent(id)
			}
		case modeUpdating:
			handleUpdatingEvent(id)
		case modeUpdateRestart:
			handleUpdateRestartEvent(id)
		}

		if app.quit {
			tApp.Stop()
			return nil
		}

		refreshChrome()
		return nil
	})

	if initialFile != "" {
		if err := openBook(initialFile); err != nil {
			app.statusMessage = err.Error()
			app.mode = modeHome
		}
		refreshChrome()
	}

	startUpdateCheck(false)

	// Start a goroutine to handle update messages
	go func() {
		for msg := range app.updateMessages {
			queueUIUpdate(func() {
				handleUpdateMessage(msg)
				refreshChrome()
			})
		}
	}()

	err := tApp.Run()
	persistState()
	if err != nil {
		log.Fatalf("failed to start application: %v", err)
	}
}
