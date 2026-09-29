package core

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/lvshp/ReadCLI/lib"
	"github.com/lvshp/ReadCLI/reader"
)

func moveShelf(delta int) {
	books := visibleBooks()
	if len(books) == 0 {
		app.bookshelfState.shelfIndex = 0
		return
	}
	app.bookshelfState.shelfIndex += delta
	if app.bookshelfState.shelfIndex < 0 {
		app.bookshelfState.shelfIndex = 0
	}
	if app.bookshelfState.shelfIndex >= len(books) {
		app.bookshelfState.shelfIndex = len(books) - 1
	}
}

func moveBookmarks(delta int) {
	bookmarks := bookmarksForCurrentBook()
	if len(bookmarks) == 0 {
		app.readingState.bookmarkIndex = 0
		return
	}
	app.readingState.bookmarkIndex += delta
	if app.readingState.bookmarkIndex < 0 {
		app.readingState.bookmarkIndex = 0
	}
	if app.readingState.bookmarkIndex >= len(bookmarks) {
		app.readingState.bookmarkIndex = len(bookmarks) - 1
	}
}

func openSelectedBookmark() {
	bookmarks := bookmarksForCurrentBook()
	if len(bookmarks) == 0 {
		return
	}
	if app.readingState.bookmarkIndex >= len(bookmarks) {
		app.readingState.bookmarkIndex = len(bookmarks) - 1
	}
	if r, ok := app.reader.(*reader.OnlineReader); ok {
		mark := bookmarks[app.readingState.bookmarkIndex]
		for i, c := range r.Chapters {
			if c.URL == mark.ChapterURL {
				openOnlineChapter(i, mark.ChapterOffset)
				return
			}
		}
		setStatus(statusError, "书签章节已不在目录中")
		return
	}
	app.reader.Goto(bookmarks[app.readingState.bookmarkIndex].Position)
	transitionTo(modeReading)
	syncCurrentBookState()
	setStatus(statusInfo, "已跳转到书签")
}

func deleteSelectedBookmark() {
	bookmarks := bookmarksForCurrentBook()
	if len(bookmarks) == 0 {
		return
	}
	if app.readingState.bookmarkIndex >= len(bookmarks) {
		app.readingState.bookmarkIndex = len(bookmarks) - 1
	}
	list := append([]lib.Bookmark(nil), bookmarks[:app.readingState.bookmarkIndex]...)
	list = append(list, bookmarks[app.readingState.bookmarkIndex+1:]...)
	app.bookmarks.Books[app.currentFile] = list
	saveBookmarks("保存书签")
	if app.readingState.bookmarkIndex >= len(list) && len(list) > 0 {
		app.readingState.bookmarkIndex = len(list) - 1
	}
	setStatus(statusInfo, "书签已删除")
}

func displayHelp() {
	app.readingState.showHelp = !app.readingState.showHelp
	app.readingState.showProgress = false
	app.readingState.showReadingQuickHelp = false
}

func displayReadingQuickHelp() {
	app.readingState.showReadingQuickHelp = !app.readingState.showReadingQuickHelp
	app.readingState.showHelp = false
	app.readingState.showProgress = false
}

func displayProgress() {
	app.readingState.showProgress = !app.readingState.showProgress
	app.readingState.showHelp = false
}

func displayTOC() {
	if app.reader == nil {
		return
	}
	if app.mode == modeTOC {
		transitionTo(modeReading)
		app.readingState.tocNumber = ""
		return
	}
	transitionTo(modeTOC)
	app.readingState.showReadingQuickHelp = false
	app.readingState.tocIndex = app.reader.CurrentChapterIndex()
	app.readingState.tocNumber = ""
}

func appendTOCNumber(digit string) {
	app.readingState.tocNumber += digit
	if index, ok := parseTOCNumber(); ok {
		app.readingState.tocIndex = index
	}
}

func parseTOCNumber() (int, bool) {
	if app.readingState.tocNumber == "" {
		return 0, false
	}
	num, err := strconv.Atoi(app.readingState.tocNumber)
	if err != nil || num <= 0 {
		return 0, false
	}
	return num - 1, true
}

func updateTOCSelection(offset int) {
	pageSize := tocPageSize()
	if pageSize < 1 {
		pageSize = 1
	}
	total := 1
	if app.reader != nil {
		toc := strings.Split(strings.TrimSpace(app.reader.GetTOC()), "\n")
		if count := len(toc) - 1; count > 0 {
			total = count
		}
	}
	app.readingState.tocIndex = clamp(app.readingState.tocIndex+offset, 0, total-1)
	app.readingState.tocNumber = ""
}

func openSelectedTOCChapter() {
	if app.reader == nil {
		return
	}
	if index, ok := parseTOCNumber(); ok {
		app.readingState.tocIndex = index
	}
	if _, ok := app.reader.(*reader.OnlineReader); ok {
		openOnlineChapter(app.readingState.tocIndex, 0)
		return
	}
	app.reader.GotoChapter(app.readingState.tocIndex)
	transitionTo(modeReading)
	app.readingState.tocNumber = ""
	setStatus(statusInfo, "已跳转到章节")
	syncCurrentBookState()
}

func moveReading(delta int) {
	if app.reader == nil {
		return
	}
	if r, ok := app.reader.(*reader.OnlineReader); ok {
		if app.online.busy != "" {
			return
		}
		if delta > 0 && r.CurrentPos()+delta >= r.Total() && r.Index+1 < len(r.Chapters) {
			openOnlineChapter(r.Index+1, 0)
			return
		}
		if delta < 0 && r.CurrentPos()+delta < 0 && r.Index > 0 {
			openOnlineChapter(r.Index-1, 1)
			return
		}
	}
	app.reader.Goto(app.reader.CurrentPos() + delta)
	app.readingState.showHelp = false
	app.readingState.showProgress = false
	setStatusf(statusInfo, "阅读位置 %d/%d", app.reader.CurrentPos()+1, app.reader.Total())
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
	if r, ok := app.reader.(*reader.OnlineReader); ok {
		mark.ChapterURL = r.Chapters[r.Index].URL
		mark.Chapter = r.Chapters[r.Index].Name
		mark.ChapterOffset = r.ChapterOffset()
	}
	list = append(list, mark)
	app.bookmarks.Books[app.currentFile] = list
	saveBookmarks("保存书签")
	setStatus(statusInfo, "书签已保存")
}

func openBookmarks() {
	transitionTo(modeBookmarks)
	app.readingState.bookmarkIndex = 0
	setStatus(statusInfo, "已打开书签列表")
}

func runSearch() {
	if app.reader == nil {
		return
	}
	app.readingState.searchQuery = strings.TrimSpace(app.uiState.input.value)
	resetInputState()
	transitionTo(modeReading)
	if app.readingState.searchQuery == "" {
		setStatus(statusInfo, "搜索已取消")
		return
	}
	pos, ok := app.reader.Search(app.readingState.searchQuery, min(app.reader.CurrentPos()+1, app.reader.Total()-1), true)
	if !ok {
		pos, ok = app.reader.Search(app.readingState.searchQuery, 0, true)
	}
	if !ok {
		setStatus(statusError, "未找到关键字: "+app.readingState.searchQuery)
		return
	}
	app.reader.Goto(pos)
	app.readingState.lastSearchIndex = pos
	setStatus(statusInfo, "已跳转到搜索结果")
	syncCurrentBookState()
}

func startBookshelfSearch() {
	transitionTo(modeBookshelfSearchInput)
	app.uiState.input.value = app.bookshelfState.query
	app.uiState.input.cursor = len([]rune(app.uiState.input.value))
	setStatus(statusInfo, "输入书名关键字过滤书架")
}

func runBookshelfSearch() {
	app.bookshelfState.query = strings.TrimSpace(app.uiState.input.value)
	resetInputState()
	transitionTo(modeHome)
	app.bookshelfState.shelfIndex = 0
	if app.bookshelfState.query == "" {
		setStatus(statusInfo, "书架搜索已清空")
		return
	}
	setStatus(statusInfo, "书架搜索: "+app.bookshelfState.query)
}

func cancelBookshelfSearch() {
	if strings.TrimSpace(app.bookshelfState.query) == "" {
		return
	}
	app.bookshelfState.query = ""
	app.bookshelfState.shelfIndex = 0
	setStatus(statusInfo, "书架搜索已清空")
}

func startReadingJumpInput() {
	if app.reader == nil {
		return
	}
	transitionTo(modeReadingJumpInput)
	setStatus(statusInfo, "输入章节号或百分比，例如 128 / 50%")
}

func runReadingJump() {
	if app.reader == nil {
		return
	}
	target := strings.TrimSpace(app.uiState.input.value)
	resetInputState()
	transitionTo(modeReading)
	if target == "" {
		setStatus(statusInfo, "跳转已取消")
		return
	}

	if strings.HasSuffix(target, "%") {
		value := strings.TrimSpace(strings.TrimSuffix(target, "%"))
		percent, err := strconv.ParseFloat(value, 64)
		if err != nil || percent < 0 || percent > 100 {
			setStatus(statusError, "百分比需在 0% 到 100% 之间")
			return
		}
		if r, ok := app.reader.(*reader.OnlineReader); ok {
			target := percent / 100 * float64(len(r.Chapters))
			index := min(int(target), len(r.Chapters)-1)
			openOnlineChapter(index, math.Min(1, target-float64(index)))
			return
		}
		total := app.reader.Total()
		pos := 0
		if total > 1 {
			pos = int(math.Round(percent / 100 * float64(total-1)))
		}
		app.reader.Goto(pos)
		app.readingState.showReadingQuickHelp = false
		setStatusf(statusInfo, "已跳转到 %.0f%%", percent)
		syncCurrentBookState()
		return
	}

	chapter, err := strconv.Atoi(target)
	if err != nil || chapter <= 0 {
		setStatus(statusError, "请输入章节号或百分比，例如 128 / 50%")
		return
	}
	if _, ok := app.reader.(*reader.OnlineReader); ok {
		openOnlineChapter(chapter-1, 0)
		return
	}
	app.reader.GotoChapter(chapter - 1)
	app.readingState.showReadingQuickHelp = false
	setStatusf(statusInfo, "已跳转到第 %d 章", chapter)
	syncCurrentBookState()
}

func jumpSearch(forward bool) {
	if app.reader == nil || strings.TrimSpace(app.readingState.searchQuery) == "" {
		setStatus(statusError, "没有可继续跳转的搜索结果")
		return
	}
	start := app.reader.CurrentPos()
	if forward {
		start++
		if start >= app.reader.Total() {
			start = 0
		}
		pos, ok := app.reader.Search(app.readingState.searchQuery, start, true)
		if !ok {
			pos, ok = app.reader.Search(app.readingState.searchQuery, 0, true)
			if !ok {
				setStatus(statusError, "未找到更多结果")
				return
			}
		}
		app.reader.Goto(pos)
	} else {
		start--
		if start < 0 {
			start = app.reader.Total() - 1
		}
		pos, ok := app.reader.Search(app.readingState.searchQuery, start, false)
		if !ok {
			pos, ok = app.reader.Search(app.readingState.searchQuery, app.reader.Total()-1, false)
			if !ok {
				setStatus(statusError, "未找到更多结果")
				return
			}
		}
		app.reader.Goto(pos)
	}
	setStatus(statusInfo, "已跳转到搜索结果")
	syncCurrentBookState()
}

func transitionTo(m mode) {
	withinSearch := app.online.searching &&
		(app.mode == modeOnlineResults || app.mode == modeOnlineErrors) &&
		(m == modeOnlineResults || m == modeOnlineErrors)
	if m != app.mode && app.online.busy != "" && !withinSearch {
		cancelOnlineRequest()
	}
	app.mode = m
	resetInputState()
	app.readingState.rowNumber = ""
	if m != modeReading {
		app.readingState.showReadingQuickHelp = false
	}
}

func cycleSort() {
	switch app.bookshelfState.sortMode {
	case "recent":
		app.bookshelfState.sortMode = "imported"
	case "imported":
		app.bookshelfState.sortMode = "title"
	default:
		app.bookshelfState.sortMode = "recent"
	}
	setStatus(statusInfo, "排序已切换为 "+app.bookshelfState.sortMode)
}

func cycleFilter() {
	options := []string{"all", "epub", "txt", "online", "unread", "reading", "finished"}
	for i, opt := range options {
		if opt == app.bookshelfState.filterMode {
			app.bookshelfState.filterMode = options[(i+1)%len(options)]
			app.bookshelfState.shelfIndex = 0
			setStatus(statusInfo, "过滤已切换为 "+app.bookshelfState.filterMode)
			return
		}
	}
	app.bookshelfState.filterMode = "all"
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
