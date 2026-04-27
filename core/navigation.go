package core

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lvshp/ReadCLI/lib"
)

func moveShelf(delta int) {
	books := visibleBooks()
	if len(books) == 0 {
		app.shelfIndex = 0
		return
	}
	app.shelfIndex += delta
	if app.shelfIndex < 0 {
		app.shelfIndex = 0
	}
	if app.shelfIndex >= len(books) {
		app.shelfIndex = len(books) - 1
	}
}

func moveBookmarks(delta int) {
	bookmarks := bookmarksForCurrentBook()
	if len(bookmarks) == 0 {
		app.bookmarkIndex = 0
		return
	}
	app.bookmarkIndex += delta
	if app.bookmarkIndex < 0 {
		app.bookmarkIndex = 0
	}
	if app.bookmarkIndex >= len(bookmarks) {
		app.bookmarkIndex = len(bookmarks) - 1
	}
}

func openSelectedBookmark() {
	bookmarks := bookmarksForCurrentBook()
	if len(bookmarks) == 0 {
		return
	}
	if app.bookmarkIndex >= len(bookmarks) {
		app.bookmarkIndex = len(bookmarks) - 1
	}
	app.reader.Goto(bookmarks[app.bookmarkIndex].Position)
	app.mode = modeReading
	syncCurrentBookState()
	app.statusMessage = "已跳转到书签"
}

func deleteSelectedBookmark() {
	bookmarks := bookmarksForCurrentBook()
	if len(bookmarks) == 0 {
		return
	}
	if app.bookmarkIndex >= len(bookmarks) {
		app.bookmarkIndex = len(bookmarks) - 1
	}
	list := append([]lib.Bookmark(nil), bookmarks[:app.bookmarkIndex]...)
	list = append(list, bookmarks[app.bookmarkIndex+1:]...)
	app.bookmarks.Books[app.currentFile] = list
	saveBookmarks("保存书签")
	if app.bookmarkIndex >= len(list) && len(list) > 0 {
		app.bookmarkIndex = len(list) - 1
	}
	app.statusMessage = "书签已删除"
}

func displayHelp() {
	app.showHelp = !app.showHelp
	app.showProgress = false
}

func displayProgress() {
	app.showProgress = !app.showProgress
	app.showHelp = false
}

func displayTOC() {
	if app.reader == nil {
		return
	}
	if app.mode == modeTOC {
		app.mode = modeReading
		app.tocNumber = ""
		return
	}
	app.mode = modeTOC
	app.tocIndex = app.reader.CurrentChapterIndex()
	app.tocNumber = ""
}

func appendTOCNumber(digit string) {
	app.tocNumber += digit
	if index, ok := parseTOCNumber(); ok {
		app.tocIndex = index
	}
}

func parseTOCNumber() (int, bool) {
	if app.tocNumber == "" {
		return 0, false
	}
	num, err := strconv.Atoi(app.tocNumber)
	if err != nil || num <= 0 {
		return 0, false
	}
	return num - 1, true
}

func updateTOCSelection(offset int) {
	app.tocIndex += offset
	pageSize := tocPageSize()
	total := 0
	if app.reader != nil {
		total = app.reader.CurrentChapterIndex()
		_ = total
	}
	if app.tocIndex < 0 {
		app.tocIndex = 0
	}
	if app.tocIndex < 0 {
		app.tocIndex = 0
	}
	if pageSize < 1 {
		pageSize = 1
	}
	app.tocNumber = ""
}

func openSelectedTOCChapter() {
	if app.reader == nil {
		return
	}
	if index, ok := parseTOCNumber(); ok {
		app.tocIndex = index
	}
	app.reader.GotoChapter(app.tocIndex)
	app.mode = modeReading
	app.tocNumber = ""
	app.statusMessage = "已跳转到章节"
	syncCurrentBookState()
}

func moveReading(delta int) {
	if app.reader == nil {
		return
	}
	app.reader.Goto(app.reader.CurrentPos() + delta)
	app.showHelp = false
	app.showProgress = false
	app.statusMessage = fmt.Sprintf("阅读位置 %d/%d", app.reader.CurrentPos()+1, app.reader.Total())
	syncCurrentBookState()
}

func pageStep() int {
	if readingVisibleSourceLines() < 1 {
		return 1
	}
	return readingVisibleSourceLines()
}

func saveBookmark() {
	if app.reader == nil || app.currentFile == "" {
		return
	}
	list := app.bookmarks.Books[app.currentFile]
	mark := lib.Bookmark{
		Path:          app.currentFile,
		Position:      app.reader.CurrentPos(),
		Chapter:       app.reader.CurrentChapterTitle(),
		Snippet:       shorten(app.reader.Current(), 32),
		CreatedAt:     time.Now().Format(time.RFC3339),
		ProgressTotal: app.reader.Total(),
	}
	list = append(list, mark)
	app.bookmarks.Books[app.currentFile] = list
	saveBookmarks("保存书签")
	app.statusMessage = "书签已保存"
}

func openBookmarks() {
	app.mode = modeBookmarks
	app.bookmarkIndex = 0
	app.statusMessage = "已打开书签列表"
}

func runSearch() {
	if app.reader == nil {
		return
	}
	app.searchQuery = strings.TrimSpace(app.inputValue)
	resetInputState()
	app.mode = modeReading
	if app.searchQuery == "" {
		app.statusMessage = "搜索已取消"
		return
	}
	pos, ok := app.reader.Search(app.searchQuery, min(app.reader.CurrentPos()+1, app.reader.Total()-1), true)
	if !ok {
		pos, ok = app.reader.Search(app.searchQuery, 0, true)
	}
	if !ok {
		app.statusMessage = "未找到关键字: " + app.searchQuery
		return
	}
	app.reader.Goto(pos)
	app.lastSearchIndex = pos
	app.statusMessage = "已跳转到搜索结果"
	syncCurrentBookState()
}

func jumpSearch(forward bool) {
	if app.reader == nil || strings.TrimSpace(app.searchQuery) == "" {
		app.statusMessage = "没有可继续跳转的搜索结果"
		return
	}
	start := app.reader.CurrentPos()
	if forward {
		start++
		if start >= app.reader.Total() {
			start = 0
		}
		pos, ok := app.reader.Search(app.searchQuery, start, true)
		if !ok {
			pos, ok = app.reader.Search(app.searchQuery, 0, true)
			if !ok {
				app.statusMessage = "未找到更多结果"
				return
			}
		}
		app.reader.Goto(pos)
	} else {
		start--
		if start < 0 {
			start = app.reader.Total() - 1
		}
		pos, ok := app.reader.Search(app.searchQuery, start, false)
		if !ok {
			pos, ok = app.reader.Search(app.searchQuery, app.reader.Total()-1, false)
			if !ok {
				app.statusMessage = "未找到更多结果"
				return
			}
		}
		app.reader.Goto(pos)
	}
	app.statusMessage = "已跳转到搜索结果"
	syncCurrentBookState()
}

func setMode(m mode) {
	app.mode = m
	resetInputState()
	app.rowNumber = ""
}

func cycleSort() {
	switch app.sortMode {
	case "recent":
		app.sortMode = "imported"
	case "imported":
		app.sortMode = "title"
	default:
		app.sortMode = "recent"
	}
	app.statusMessage = "排序已切换为 " + app.sortMode
}

func cycleFilter() {
	options := []string{"all", "epub", "txt", "unread", "reading", "finished"}
	for i, opt := range options {
		if opt == app.filterMode {
			app.filterMode = options[(i+1)%len(options)]
			app.shelfIndex = 0
			app.statusMessage = "过滤已切换为 " + app.filterMode
			return
		}
	}
	app.filterMode = "all"
}
