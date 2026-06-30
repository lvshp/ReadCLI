package core

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lvshp/ReadCLI/lib"
)

func importBook() {
	path := strings.TrimSpace(app.uiState.input.value)
	resetInputState()
	transitionTo(modeHome)
	if path == "" {
		setStatus(statusInfo, "导入已取消")
		return
	}
	resolved, _ := resolveImportPath(path)
	path = resolved
	info, err := os.Stat(path)
	if err != nil {
		setStatus(statusError, "文件不存在")
		return
	}
	if info.IsDir() {
		recursive := app.uiState.input.importRecursive
		setStatus(statusProgress, importModeLabelFor(recursive)+"正在扫描目录...")
		refreshChrome()
		go runDirectoryImport(path, recursive)
		return
	}

	book, err := loadBookshelfBook(path)
	if err != nil {
		setStatus(statusError, err.Error())
		return
	}
	lib.UpsertBookshelfBook(app.bookshelf, book)
	if !saveBookshelf("保存书架") {
		return
	}
	setStatus(statusInfo, "已导入 "+filepath.Base(path))
}

func runDirectoryImport(root string, recursive bool) {
	label := importModeLabelFor(recursive)
	books, err := importBooksFromDirectory(root, recursive, func(done, total int, path string) {
		queueUIUpdate(func() {
			setStatusf(statusProgress, "%s正在导入 %d/%d: %s", label, done, total, shorten(filepath.Base(path), 24))
			refreshChrome()
		})
	})
	queueUIUpdate(func() {
		switch {
		case err != nil:
			setStatus(statusError, err.Error())
		case len(books) == 0:
			setStatus(statusError, "目录中没有可导入的 txt/epub")
		case len(books) == 1:
			lib.UpsertBookshelfBook(app.bookshelf, books[0])
			if !saveBookshelf("保存书架") {
				break
			}
			setStatus(statusInfo, label+"已导入 1 本书")
		default:
			for _, book := range books {
				lib.UpsertBookshelfBook(app.bookshelf, book)
			}
			if !saveBookshelf("保存书架") {
				break
			}
			setStatusf(statusInfo, "%s已导入 %d 本书", label, len(books))
		}
		refreshChrome()
	})
}

func importBooksFromDirectory(root string, recursive bool, onProgress func(done, total int, path string)) ([]lib.BookshelfBook, error) {
	paths, err := collectImportCandidates(root, recursive)
	if err != nil {
		return nil, err
	}

	total := len(paths)
	books := make([]lib.BookshelfBook, 0, total)
	for i, path := range paths {
		if onProgress != nil {
			onProgress(i+1, total, path)
		}

		book, err := loadBookshelfBook(path)
		if err != nil {
			continue
		}
		books = append(books, book)
	}
	return books, nil
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
