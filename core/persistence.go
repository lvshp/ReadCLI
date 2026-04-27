package core

import "github.com/lvshp/ReadCLI/lib"

func notePersistenceError(action string, err error) bool {
	if err == nil {
		return false
	}
	if app != nil {
		app.statusMessage = action + "失败: " + shorten(err.Error(), 96)
	}
	return true
}

func saveConfig(action string) bool {
	if app == nil {
		return false
	}
	return !notePersistenceError(action, lib.SaveConfig(app.config))
}

func saveBookshelf(action string) bool {
	if app == nil {
		return false
	}
	return !notePersistenceError(action, lib.SaveBookshelf(app.bookshelf))
}

func saveBookmarks(action string) bool {
	if app == nil {
		return false
	}
	return !notePersistenceError(action, lib.SaveBookmarks(app.bookmarks))
}

func saveProgress(action string) bool {
	if app == nil {
		return false
	}
	return !notePersistenceError(action, lib.SaveProgress(app.progress))
}
