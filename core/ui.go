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
		mode:        modeHome,
		config:      cfg,
		bookshelf:   shelf,
		bookmarks:   marks,
		progress:    progress,
		readerCache: map[string]cachedReader{},
		themeOrder:  []string{"vscode", "jetbrains", "ops-console"},
		bookshelfState: bookshelfState{
			sortMode:   "recent",
			filterMode: "all",
			shelfIndex: cfg.SelectedBookshelf,
		},
		readingState: readingState{
			lastSearchIndex: -1,
			compactMode:     cfg.CompactMode,
			displayLines:    cfg.DisplayLines,
		},
		updateState: updateState{
			messages: make(chan updateMessage, 16),
		},
		uiState: uiState{
			sessionStart: time.Now(),
			showBorder:   cfg.ShowBorder,
			input: inputState{
				importRecursive: false,
			},
		},
		currentVersion: strings.TrimSpace(version),
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

	initOnline()
	initPurification()
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

		wasCompact := compactReadingUI()

		dispatchEvent(id)

		if app.quit {
			tApp.Stop()
			return nil
		}

		if wasCompact != compactReadingUI() {
			applyLayoutFromAppWithoutReflow()
			return nil
		}

		refreshChrome()
		return nil
	})

	if initialFile != "" {
		if err := openBook(initialFile); err != nil {
			setStatus(statusError, err.Error())
			transitionTo(modeHome)
		}
		refreshChrome()
	}

	startUpdateCheck(false)

	// Start a goroutine to handle update messages
	go func() {
		for msg := range app.updateState.messages {
			queueUIUpdate(func() {
				handleUpdateMessage(msg)
				refreshChrome()
			})
		}
	}()

	err := tApp.Run()
	cancelOnlineRequest()
	persistState()
	if err != nil {
		log.Fatalf("failed to start application: %v", err)
	}
}
