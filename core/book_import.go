package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lvshp/ReadCLI/lib"
)

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
		recursive := app.importRecursive
		app.statusMessage = importModeLabelFor(recursive) + "正在扫描目录..."
		refreshChrome()
		go runDirectoryImport(path, recursive)
		return
	}

	book, err := loadBookshelfBook(path)
	if err != nil {
		app.statusMessage = err.Error()
		return
	}
	lib.UpsertBookshelfBook(app.bookshelf, book)
	if !saveBookshelf("保存书架") {
		return
	}
	app.statusMessage = "已导入 " + filepath.Base(path)
}

func runDirectoryImport(root string, recursive bool) {
	label := importModeLabelFor(recursive)
	books, err := importBooksFromDirectory(root, recursive, func(done, total int, path string) {
		queueUIUpdate(func() {
			app.statusMessage = fmt.Sprintf("%s正在导入 %d/%d: %s", label, done, total, shorten(filepath.Base(path), 24))
			refreshChrome()
		})
	})
	queueUIUpdate(func() {
		switch {
		case err != nil:
			app.statusMessage = err.Error()
		case len(books) == 0:
			app.statusMessage = "目录中没有可导入的 txt/epub"
		case len(books) == 1:
			lib.UpsertBookshelfBook(app.bookshelf, books[0])
			if !saveBookshelf("保存书架") {
				break
			}
			app.statusMessage = label + "已导入 1 本书"
		default:
			for _, book := range books {
				lib.UpsertBookshelfBook(app.bookshelf, book)
			}
			if !saveBookshelf("保存书架") {
				break
			}
			app.statusMessage = fmt.Sprintf("%s已导入 %d 本书", label, len(books))
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
