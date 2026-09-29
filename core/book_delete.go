package core

import (
	"os"
	"strings"

	"github.com/lvshp/ReadCLI/lib"
)

func removeSelectedBook(deleteFile bool) {
	path := app.bookshelfState.deleteTargetPath
	if path == "" {
		return
	}
	if deleteFile && !strings.HasPrefix(path, "online:") {
		if err := os.Remove(path); err != nil {
			setStatus(statusError, "删除本地文件失败: "+shorten(err.Error(), 96))
			return
		}
	}
	removeBookState(path)
	transitionTo(modeHome)
	app.bookshelfState.deleteTargetPath = ""
	app.bookshelfState.deleteTargetTitle = ""
	setStatus(statusInfo, "已移出书架")
	if deleteFile {
		setStatus(statusInfo, "已删除本地文件并移出书架")
	}
}

func removeBookState(path string) {
	// Kept small so delete flow and future cleanup commands share the same state pruning.
	delete(app.readerCache, path)
	delete(app.progress.Books, path)
	delete(app.progress.Anchors, path)
	delete(app.bookmarks.Books, path)
	lib.RemoveBookshelfBook(app.bookshelf, path)
	saveBookshelf("保存书架")
	saveProgress("保存进度")
	saveBookmarks("保存书签")
}

func prepareDeleteSelectedBook() {
	book := selectedBook()
	if book == nil {
		setStatus(statusError, "没有可删除的书籍")
		return
	}
	app.bookshelfState.deleteTargetPath = book.Path
	app.bookshelfState.deleteTargetTitle = book.Title
	transitionTo(modeDeleteConfirm)
}
