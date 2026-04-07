package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lvshp/ReadCLI/lib"
	"github.com/lvshp/ReadCLI/reader"
)

func openBook(path string) error {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}

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

	r, _, err := cachedReaderForPath(path)
	if err != nil {
		return err
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
	return nil
}

func cachedReaderForPath(path string) (reader.Reader, bool, error) {
	if app != nil && app.readerCache != nil {
		if cached, ok := app.readerCache[path]; ok && cached.reader != nil {
			return cached.reader, true, nil
		}
	}

	r, err := newReaderForPath(path)
	if err != nil {
		return nil, false, err
	}
	if err := r.Load(path); err != nil {
		return nil, false, err
	}

	if app != nil {
		if app.readerCache == nil {
			app.readerCache = map[string]cachedReader{}
		}
		app.readerCache[path] = cachedReader{reader: r}
	}
	return r, false, nil
}

func newReaderForPath(path string) (reader.Reader, error) {
	switch strings.ToUpper(filepath.Ext(path)) {
	case ".TXT":
		return reader.NewTxtReader(), nil
	case ".EPUB":
		return reader.NewEpubReader(), nil
	default:
		return nil, fmt.Errorf("unsupported file format: %s", filepath.Ext(path))
	}
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
	_ = lib.SaveBookshelf(app.bookshelf)
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
	_ = lib.SaveBookshelf(app.bookshelf)
	_ = lib.SaveProgress(app.progress)
}

func openSelectedBook() {
	book := selectedBook()
	if book == nil {
		app.statusMessage = "书架为空"
		return
	}
	path := book.Path
	app.loadingBookPath = path
	app.statusMessage = "正在打开 " + shorten(filepath.Base(path), 24)
	refreshChrome()

	go func() {
		if err := openBook(path); err != nil {
			tApp.QueueUpdateDraw(func() {
				app.loadingBookPath = ""
				app.statusMessage = err.Error()
				refreshChrome()
			})
			return
		}
		tApp.QueueUpdateDraw(func() {
			app.loadingBookPath = ""
			refreshChrome()
		})
	}()
}

func removeSelectedBook(deleteFile bool) {
	path := app.deleteTargetPath
	if path == "" {
		return
	}
	if deleteFile {
		_ = os.Remove(path)
	}
	lib.RemoveBookshelfBook(app.bookshelf, path)
	delete(app.readerCache, path)
	delete(app.progress.Books, path)
	delete(app.bookmarks.Books, path)
	_ = lib.SaveBookshelf(app.bookshelf)
	_ = lib.SaveProgress(app.progress)
	_ = lib.SaveBookmarks(app.bookmarks)
	app.mode = modeHome
	app.deleteTargetPath = ""
	app.deleteTargetTitle = ""
	app.statusMessage = "已移出书架"
	if deleteFile {
		app.statusMessage = "已删除本地文件并移出书架"
	}
}

func prepareDeleteSelectedBook() {
	book := selectedBook()
	if book == nil {
		app.statusMessage = "没有可删除的书籍"
		return
	}
	app.deleteTargetPath = book.Path
	app.deleteTargetTitle = book.Title
	app.mode = modeDeleteConfirm
}

func importBook() {
	path := strings.TrimSpace(app.inputValue)
	resetInputState()
	app.mode = modeHome
	if path == "" {
		app.statusMessage = "导入已取消"
		return
	}
	resolved, _ := resolveImportPath(path)
	path = resolved
	info, err := os.Stat(path)
	if err != nil {
		app.statusMessage = "文件不存在"
		return
	}
	if info.IsDir() {
		go runDirectoryImport(path, app.importRecursive)
		return
	}

	book, err := loadBookshelfBook(path)
	if err != nil {
		app.statusMessage = err.Error()
		return
	}
	lib.UpsertBookshelfBook(app.bookshelf, book)
	_ = lib.SaveBookshelf(app.bookshelf)
	app.statusMessage = "已导入 " + filepath.Base(path)
}

func runDirectoryImport(root string, recursive bool) {
	app.statusMessage = importModeLabel() + "正在扫描目录..."
	queueRedraw()

	imported, err := importBooksFromDirectory(root, recursive)
	tApp.QueueUpdateDraw(func() {
		switch {
		case err != nil:
			app.statusMessage = err.Error()
		case imported == 0:
			app.statusMessage = "目录中没有可导入的 txt/epub"
		case imported == 1:
			app.statusMessage = importModeLabel() + "已导入 1 本书"
		default:
			app.statusMessage = fmt.Sprintf("%s已导入 %d 本书", importModeLabel(), imported)
		}
		refreshChrome()
	})
}

func importBooksFromDirectory(root string, recursive bool) (int, error) {
	paths, err := collectImportCandidates(root, recursive)
	if err != nil {
		return 0, err
	}

	total := len(paths)
	imported := 0
	for i, path := range paths {
		app.statusMessage = fmt.Sprintf("%s正在导入 %d/%d: %s", importModeLabel(), i+1, total, shorten(filepath.Base(path), 24))
		queueRedraw()

		book, err := loadBookshelfBook(path)
		if err != nil {
			continue
		}
		lib.UpsertBookshelfBook(app.bookshelf, book)
		imported++
	}
	_ = lib.SaveBookshelf(app.bookshelf)
	return imported, nil
}

func collectImportCandidates(root string, recursive bool) ([]string, error) {
	if recursive {
		var paths []string
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			if !isSupportedBookFile(path) {
				return nil
			}
			paths = append(paths, path)
			return nil
		})
		return paths, err
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if !isSupportedBookFile(path) {
			continue
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func loadBookshelfBook(path string) (lib.BookshelfBook, error) {
	r, err := newReaderForPath(path)
	if err != nil {
		return lib.BookshelfBook{}, err
	}
	if err := r.Load(path); err != nil {
		return lib.BookshelfBook{}, err
	}
	return lib.BookshelfBook{
		Path:            path,
		Title:           bookTitleForPath(path, r.BookTitle()),
		Format:          strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."),
		ProgressPos:     0,
		ProgressTotal:   r.Total(),
		ProgressPercent: 0,
		CurrentChapter:  r.CurrentChapterTitle(),
		LastReadAt:      time.Now().Format(time.RFC3339),
	}, nil
}

func isSupportedBookFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".epub":
		return true
	default:
		return false
	}
}
