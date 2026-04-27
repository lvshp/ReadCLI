package core

import (
	"os"

	"github.com/lvshp/ReadCLI/lib"
)

func removeSelectedBook(deleteFile bool) {
	path := app.deleteTargetPath
	if path == "" {
		return
	}
	if deleteFile {
		if err := os.Remove(path); err != nil {
			app.statusMessage = "删除本地文件失败: " + shorten(err.Error(), 96)
			return
		}
	}
	removeBookState(path)
	app.mode = modeHome
	app.deleteTargetPath = ""
	app.deleteTargetTitle = ""
	app.statusMessage = "已移出书架"
	if deleteFile {
		app.statusMessage = "已删除本地文件并移出书架"
	}
}

func removeBookState(path string) {
	// Kept small so delete flow and future cleanup commands share the same state pruning.
	delete(app.readerCache, path)
	delete(app.progress.Books, path)
	delete(app.bookmarks.Books, path)
	lib.RemoveBookshelfBook(app.bookshelf, path)
	saveBookshelf("保存书架")
	saveProgress("保存进度")
	saveBookmarks("保存书签")
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
