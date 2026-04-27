package core

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/lvshp/ReadCLI/lib"
	"github.com/lvshp/ReadCLI/reader"
)

func openBook(path string) error {
	path = normalizeBookPath(path)

	r, _, err := cachedReaderForPath(path)
	if err != nil {
		return err
	}

	applyLoadedBook(path, r)
	return nil
}

func applyLoadedBook(path string, r reader.Reader) {
	// Normalize existing bookshelf entries whose path resolves to the same file.
	// On Windows, paths may differ only by case (e.g. C:\A\B vs c:\a\b).
	for i := range app.bookshelf.Books {
		if app.bookshelf.Books[i].Path == path {
			break
		}
		resolved, err := filepath.Abs(app.bookshelf.Books[i].Path)
		if err == nil && strings.EqualFold(resolved, path) {
			app.bookshelf.Books[i].Path = path
		}
	}

	app.reader = r
	app.currentFile = path
	app.currentBook = nil
	app.showHelp = false
	app.showProgress = false
	app.rowNumber = ""
	app.searchQuery = ""
	app.inputValue = ""
	app.inputCursor = 0
	app.inputHints = nil
	app.inputHintIndex = 0
	app.lastSearchIndex = -1
	app.tocNumber = ""
	app.mode = modeReading

	var savedAnchor reader.ProgressAnchor
	hasSavedAnchor := false
	legacyChapterTitle := ""

	if book, ok := lib.FindBookshelfBook(app.bookshelf, path); ok {
		app.currentBook = &book
		legacyChapterTitle = strings.TrimSpace(book.CurrentChapter)
		if book.ChapterIndex > 0 || book.ChapterOffset > 0 || (book.ProgressPos == 0 && legacyChapterTitle == "") {
			savedAnchor = reader.ProgressAnchor{
				Pos:           book.ProgressPos,
				ChapterIndex:  book.ChapterIndex,
				ChapterOffset: book.ChapterOffset,
			}
			hasSavedAnchor = true
		}
	}
	if !hasSavedAnchor {
		if anchor, ok := app.progress.Anchors[path]; ok {
			savedAnchor = reader.ProgressAnchor{
				Pos:           anchor.Pos,
				ChapterIndex:  anchor.ChapterIndex,
				ChapterOffset: anchor.ChapterOffset,
				OverallRatio:  anchor.OverallRatio,
			}
			hasSavedAnchor = true
		} else if pos, ok := app.progress.Books[path]; ok {
			savedAnchor = reader.ProgressAnchor{Pos: pos}
			hasSavedAnchor = true
		}
	}
	main.SetTitle(" editor: " + filepath.Base(path) + " ")
	applyLayoutFromApp()
	if hasSavedAnchor {
		reader.RestoreFromAnchor(app.reader, savedAnchor)
	} else if legacyChapterTitle != "" {
		if chapterIndex := reader.FindChapterIndexByTitle(app.reader, legacyChapterTitle); chapterIndex >= 0 {
			reader.RestoreFromAnchor(app.reader, reader.ProgressAnchor{
				ChapterIndex:  chapterIndex,
				ChapterOffset: 0,
				Pos:           0,
			})
		}
	}
	app.currentBook = upsertCurrentBook(path)
	app.statusMessage = "已打开 " + filepath.Base(path)
}

func upsertCurrentBook(path string) *lib.BookshelfBook {
	anchor := reader.AnchorFromReader(app.reader)
	book := lib.BookshelfBook{
		Path:            path,
		Title:           bookTitleForPath(path, app.reader.BookTitle()),
		Format:          strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."),
		ProgressPos:     anchor.Pos,
		ProgressTotal:   app.reader.Total(),
		ProgressPercent: progressPercent(anchor.Pos, app.reader.Total()),
		CurrentChapter:  app.reader.CurrentChapterTitle(),
		ChapterIndex:    anchor.ChapterIndex,
		ChapterOffset:   anchor.ChapterOffset,
		LastReadAt:      time.Now().Format(time.RFC3339),
	}
	if existing, ok := lib.FindBookshelfBook(app.bookshelf, path); ok {
		book.ImportedAt = existing.ImportedAt
	}
	lib.UpsertBookshelfBook(app.bookshelf, book)
	saveBookshelf("保存书架")
	for i := range app.bookshelf.Books {
		if app.bookshelf.Books[i].Path == path {
			return &app.bookshelf.Books[i]
		}
	}
	return nil
}

func syncCurrentBookState() {
	if app == nil || app.reader == nil || app.currentFile == "" {
		return
	}

	anchor := reader.AnchorFromReader(app.reader)
	book := lib.BookshelfBook{
		Path:            app.currentFile,
		Title:           bookTitleForPath(app.currentFile, app.reader.BookTitle()),
		Format:          strings.TrimPrefix(strings.ToLower(filepath.Ext(app.currentFile)), "."),
		ProgressPos:     anchor.Pos,
		ProgressTotal:   app.reader.Total(),
		ProgressPercent: progressPercent(anchor.Pos, app.reader.Total()),
		CurrentChapter:  app.reader.CurrentChapterTitle(),
		ChapterIndex:    anchor.ChapterIndex,
		ChapterOffset:   anchor.ChapterOffset,
		LastReadAt:      time.Now().Format(time.RFC3339),
	}
	if existing, ok := lib.FindBookshelfBook(app.bookshelf, app.currentFile); ok {
		book.ImportedAt = existing.ImportedAt
	}
	lib.UpsertBookshelfBook(app.bookshelf, book)
	app.progress.Books[app.currentFile] = anchor.Pos
	if app.progress.Anchors == nil {
		app.progress.Anchors = map[string]lib.ProgressAnchor{}
	}
	app.progress.Anchors[app.currentFile] = lib.ProgressAnchor{
		Pos:           anchor.Pos,
		ChapterIndex:  anchor.ChapterIndex,
		ChapterOffset: anchor.ChapterOffset,
		OverallRatio:  anchor.OverallRatio,
	}
	saveBookshelf("保存书架")
	saveProgress("保存进度")
}

func openSelectedBook() {
	book := selectedBook()
	if book == nil {
		app.statusMessage = "书架为空"
		return
	}
	path := normalizeBookPath(book.Path)
	if r, cached, err := cachedReaderIfFresh(path); err != nil {
		app.statusMessage = err.Error()
		return
	} else if cached {
		applyLoadedBook(path, r)
		refreshChrome()
		return
	}

	app.loadingBookPath = path
	app.statusMessage = "正在打开 " + shorten(filepath.Base(path), 24)
	refreshChrome()

	go func(requestedPath string) {
		r, size, modTime, err := loadFreshReader(requestedPath)
		queueUIUpdate(func() {
			if app.loadingBookPath != requestedPath {
				return
			}
			app.loadingBookPath = ""
			if err != nil {
				app.statusMessage = err.Error()
				refreshChrome()
				return
			}
			storeCachedReader(requestedPath, r, size, modTime)
			applyLoadedBook(requestedPath, r)
			refreshChrome()
		})
	}(path)
}
