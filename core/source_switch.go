package core

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lvshp/ReadCLI/booksource"
	"github.com/lvshp/ReadCLI/lib"
	"github.com/lvshp/ReadCLI/reader"
)

type sourceSwitchState struct {
	original   lib.BookshelfBook
	returnMode mode
	results    []booksource.Book
	index      int
	page       int
	errors     []string
	prepared   *sourceSwitchPlan
}

type sourceSwitchPlan struct {
	book            booksource.Book
	chapters        []booksource.Chapter
	text            string
	index           int
	offset          float64
	matched         bool
	displayTitles   []string
	purificationErr error
}

func openSourceSwitch() {
	returnMode := app.mode
	var original *lib.BookshelfBook
	if returnMode == modeReading {
		if _, ok := app.reader.(*reader.OnlineReader); ok {
			syncCurrentBookState()
			if saved, exists := lib.FindBookshelfBook(app.bookshelf, app.currentFile); exists {
				original = &saved
			}
		}
	} else if returnMode == modeHome {
		original = selectedBook()
	}
	if original == nil || original.Online == nil {
		setStatus(statusInfo, "请选择在线书籍后按 C 换源")
		return
	}
	cancelOnlineRequest()
	app.sourceSwitch = sourceSwitchState{original: *original, returnMode: returnMode, page: 1}
	app.readingState.showHelp = false
	app.readingState.showProgress = false
	transitionTo(modeSourceSwitch)
	searchSourceAlternatives(1)
}

func closeSourceSwitch() {
	cancelOnlineRequest()
	returnMode := app.sourceSwitch.returnMode
	app.sourceSwitch = sourceSwitchState{}
	if returnMode != modeReading {
		returnMode = modeHome
	}
	transitionTo(returnMode)
	setStatus(statusInfo, "已取消换源，保留原书源")
}

func backToSourceAlternatives() {
	cancelOnlineRequest()
	app.sourceSwitch.prepared = nil
	transitionTo(modeSourceSwitch)
}

func sourceSwitchError(action string, err error) func() {
	return func() {
		message := action + ": " + err.Error()
		app.sourceSwitch.errors = []string{message}
		setStatus(statusError, message)
	}
}

func searchSourceAlternatives(page int) {
	if app.sourceSwitch.original.Online == nil {
		return
	}
	current := *app.sourceSwitch.original.Online
	sources := append([]booksource.Source(nil), app.online.sources...)
	client := onlineClient()
	app.sourceSwitch.prepared = nil
	app.sourceSwitch.results = nil
	app.sourceSwitch.errors = nil
	app.sourceSwitch.index = 0
	app.sourceSwitch.page = max(1, page)
	page = app.sourceSwitch.page
	transitionTo(modeSourceSwitch)
	startOnlineRequest("正在查找同名书籍的其他来源", func(ctx context.Context) func() {
		result, err := client.FindAlternatives(ctx, sources, current, page)
		if err != nil {
			return sourceSwitchError("查找书源失败", err)
		}
		return func() {
			app.sourceSwitch.results = result.Books
			app.sourceSwitch.errors = result.Errors
			setStatusf(statusInfo, "找到 %d 个可选来源 · %d 个书源失败", len(result.Books), len(result.Errors))
		}
	})
}

func prepareSourceSwitch() {
	state := app.sourceSwitch
	if state.index < 0 || state.index >= len(state.results) || state.original.Online == nil {
		return
	}
	candidate := state.results[state.index]
	source, found := sourceForBook(candidate)
	if !found || !source.IsEnabled() {
		setStatus(statusError, "该书源已移除或停用，请刷新候选列表")
		return
	}
	dir, err := lib.DataDirPath()
	if err != nil {
		setStatus(statusError, err.Error())
		return
	}
	client := onlineClient()
	app.sourceSwitch.errors = nil
	app.sourceSwitch.prepared = nil
	startOnlineRequest("正在匹配新来源的目录并验证正文", func(ctx context.Context) func() {
		plan, err := prepareAlternative(ctx, client, dir, source, state.original, candidate)
		if err != nil {
			return sourceSwitchError("换源准备失败，原书保留", err)
		}
		return func() {
			app.sourceSwitch.prepared = plan
			transitionTo(modeSourceSwitchConfirm)
			setStatus(statusInfo, "新来源已就绪，请核对章节后确认")
		}
	})
}

func prepareAlternative(ctx context.Context, client *booksource.Client, dir string, source booksource.Source, original lib.BookshelfBook, candidate booksource.Book) (*sourceSwitchPlan, error) {
	book, err := client.BookInfo(ctx, source, candidate)
	if err != nil {
		return nil, err
	}
	if onlineBookID(book) == original.Path {
		return nil, fmt.Errorf("该结果仍指向当前书源版本")
	}
	chapters, err := client.Chapters(ctx, source, book)
	if err != nil {
		return nil, err
	}
	index, matched := booksource.MatchAlternativeChapter(chapters, original.CurrentChapter, original.ChapterIndex)
	if index < 0 || index >= len(chapters) {
		return nil, fmt.Errorf("新来源没有可用章节")
	}
	for i, chapter := range chapters {
		if original.ChapterURL != "" && chapter.URL == original.ChapterURL {
			index, matched = i, true
			break
		}
	}
	if original.CurrentChapter == "" && original.ChapterIndex == 0 && original.ProgressPercent == 0 {
		index, matched = 0, true
	}
	text, err := loadOnlineContent(ctx, client, dir, source, true, book, chapters[index])
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("新来源正文为空，无法换源")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	text, titles, purificationErr := purifyOnlineChapter(ctx, dir, book, chapters, text)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &sourceSwitchPlan{book: book, chapters: chapters, text: text, index: index, offset: math.Max(0, math.Min(1, original.ChapterOffset)), matched: matched, displayTitles: titles, purificationErr: purificationErr}, nil
}

func confirmSourceSwitch() {
	plan := app.sourceSwitch.prepared
	if app.mode != modeSourceSwitchConfirm || plan == nil {
		return
	}
	original := app.sourceSwitch.original
	if _, exists := lib.FindBookshelfBook(app.bookshelf, original.Path); !exists {
		setStatus(statusError, "原书已不在书架中，请重新选择")
		return
	}
	dir, err := lib.DataDirPath()
	if err == nil {
		err = booksource.SaveCachedChapters(dir, plan.book, plan.chapters)
	}
	if err == nil {
		err = onlineClient().SaveSessions(dir)
	}
	if err != nil {
		setStatus(statusError, "保存新来源失败，原书保留: "+err.Error())
		return
	}
	r := reader.NewOnlineReader(plan.book, plan.chapters, plan.index, plan.text)
	r.SetDisplayTitles(plan.displayTitles)
	// This UI callback does not draw until it returns. Prepare the final layout
	// to persist an accurate anchor, restoring the entire old state on failure.
	before := *app
	installOnlineReader(r, plan.offset)
	shelf, progress, marks := switchedLibrary(original, plan, r)
	if err := saveSwitchedLibrary(dir, shelf, progress, marks); err != nil {
		*app = before
		applyLayoutFromAppWithoutReflow()
		setStatus(statusError, "保存换源结果失败，原书保留: "+err.Error())
		return
	}
	app.bookshelf, app.progress, app.bookmarks = shelf, progress, marks
	if book, ok := lib.FindBookshelfBook(shelf, app.currentFile); ok {
		app.currentBook = &book
	}
	app.sourceSwitch = sourceSwitchState{}
	setStatus(statusInfo, "换源成功 · "+r.CurrentChapterTitle())
	showPurificationWarning(plan.purificationErr)
}

func switchedLibrary(original lib.BookshelfBook, plan *sourceSwitchPlan, r *reader.OnlineReader) (*lib.BookshelfStore, *lib.ProgressStore, *lib.BookmarkStore) {
	newID := onlineBookID(plan.book)
	replacement := original
	book := plan.book
	replacement.Path, replacement.Online, replacement.Format = newID, &book, "online"
	replacement.Title = book.Name
	replacement.ChapterIndex, replacement.ChapterURL, replacement.CurrentChapter = plan.index, plan.chapters[plan.index].URL, plan.chapters[plan.index].Name
	replacement.ChapterOffset = r.ChapterOffset()
	replacement.ProgressPos, replacement.ProgressTotal = r.CurrentPos(), r.Total()
	replacement.ProgressPercent = int(r.OverallProgress() * 100)
	replacement.LastReadAt = time.Now().Format(time.RFC3339)
	shelf := &lib.BookshelfStore{}
	for _, saved := range app.bookshelf.Books {
		if saved.Path == original.Path {
			shelf.Books = append(shelf.Books, replacement)
		} else if saved.Path != newID {
			shelf.Books = append(shelf.Books, saved)
		}
	}
	progress := &lib.ProgressStore{Books: map[string]int{}, Anchors: map[string]lib.ProgressAnchor{}}
	for key, value := range app.progress.Books {
		progress.Books[key] = value
	}
	for key, value := range app.progress.Anchors {
		progress.Anchors[key] = value
	}
	progress.Books[newID] = r.CurrentPos()
	progress.Anchors[newID] = lib.ProgressAnchor{Pos: r.CurrentPos(), ChapterIndex: r.Index, ChapterOffset: r.ChapterOffset(), OverallRatio: r.OverallProgress()}
	marks := &lib.BookmarkStore{Books: map[string][]lib.Bookmark{}}
	for key, values := range app.bookmarks.Books {
		marks.Books[key] = append([]lib.Bookmark(nil), values...)
	}
	for _, mark := range app.bookmarks.Books[original.Path] {
		mark.Path = newID
		index, matched := booksource.MatchAlternativeChapter(plan.chapters, mark.Chapter, -1)
		if matched && index >= 0 {
			mark.ChapterURL, mark.Chapter = plan.chapters[index].URL, plan.chapters[index].Name
			mark.Position, mark.ProgressTotal = 0, 0
		}
		duplicate := false
		for _, existing := range marks.Books[newID] {
			if existing.CreatedAt == mark.CreatedAt && existing.Snippet == mark.Snippet {
				duplicate = true
				break
			}
		}
		if !duplicate {
			marks.Books[newID] = append(marks.Books[newID], mark)
		}
	}
	return shelf, progress, marks
}

// The shelf is the commit point. Auxiliary stores retain the old book's keys,
// so even a partial write leaves its existing progress and bookmarks readable.
func saveSwitchedLibrary(dir string, shelf *lib.BookshelfStore, progress *lib.ProgressStore, marks *lib.BookmarkStore) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	entries := []struct {
		name string
		data any
		temp string
	}{{name: "bookmarks.json", data: marks}, {name: "progress.json", data: progress}, {name: "bookshelf.json", data: shelf}}
	defer func() {
		for _, entry := range entries {
			if entry.temp != "" {
				_ = os.Remove(entry.temp)
			}
		}
	}()
	for i := range entries {
		data, err := json.MarshalIndent(entries[i].data, "", "  ")
		if err != nil {
			return err
		}
		file, err := os.CreateTemp(dir, ".source-switch-*")
		if err != nil {
			return err
		}
		entries[i].temp = file.Name()
		_, err = file.Write(append(data, '\n'))
		if err == nil {
			err = file.Sync()
		}
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
	}
	for _, entry := range entries {
		if err := os.Rename(entry.temp, filepath.Join(dir, entry.name)); err != nil {
			return fmt.Errorf("%s: %w", strings.TrimSuffix(entry.name, ".json"), err)
		}
	}
	return nil
}
