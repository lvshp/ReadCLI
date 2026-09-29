package core

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lvshp/ReadCLI/booksource"
	"github.com/lvshp/ReadCLI/lib"
	"github.com/lvshp/ReadCLI/reader"
)

func initOnline() {
	app.online.client = booksource.NewClient()
	dir, err := lib.DataDirPath()
	if err == nil {
		app.online.sources, err = booksource.LoadSources(dir)
		if err == nil {
			err = app.online.client.LoadSessions(dir)
		}
	}
	if err != nil {
		setStatus(statusError, "读取书源失败: "+err.Error())
	}
}
func onlineClient() *booksource.Client {
	if app.online.client == nil {
		app.online.client = booksource.NewClient()
	}
	return app.online.client
}
func cancelOnlineRequest() {
	if app.online.cancel != nil {
		app.online.cancel()
		app.online.cancel = nil
	}
	app.online.requestID++
	app.online.busy = ""
	app.online.searching = false
}

// Capture the dispatcher on the UI thread so background workers never inspect
// the current app or tview instance while scheduling a state update.
func onlineUpdateQueue() func(func()) {
	if app.online.enqueue != nil {
		return app.online.enqueue
	}
	if ui := tApp; ui != nil {
		return func(fn func()) { ui.QueueUpdateDraw(fn) }
	}
	return func(fn func()) {
		if fn != nil {
			fn()
		}
	}
}

func cancelOnlineWithStatus() {
	searching := app.online.searching
	cancelOnlineRequest()
	if searching {
		setStatusf(statusInfo, "已取消搜索，保留 %d 本 · 已完成 %d/%d 个书源", len(app.online.results), app.online.searchDone, app.online.searchTotal)
	} else {
		setStatus(statusInfo, "已取消加载")
	}
}

// Only this callback installs downloaded data into UI state. Superseded requests
// cannot reopen a book or overwrite a newer search after the user navigates away.
func startOnlineRequest(label string, work func(context.Context) func()) {
	cancelOnlineRequest()
	owner := app
	id := app.online.requestID
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	app.online.cancel = cancel
	app.online.busy = label
	enqueue := onlineUpdateQueue()
	setStatus(statusProgress, label+"（Esc 可取消）")
	go func() {
		done := work(ctx)
		cancel()
		enqueue(func() {
			if app != owner || app.online.requestID != id {
				return
			}
			app.online.cancel = nil
			app.online.busy = ""
			if done != nil {
				done()
			}
			if tApp != nil {
				applyLayoutFromAppWithoutReflow()
			}
		})
	}()
}
func onlineError(action string, err error) func() {
	return func() {
		setStatus(statusError, action+": "+err.Error())
		app.online.errors = []string{action + ": " + err.Error()}
	}
}
func openSources() {
	cancelOnlineRequest()
	transitionTo(modeSources)
	app.readingState.showHelp = false
	app.readingState.showProgress = false
}
func selectedSource() *booksource.Source {
	if len(app.online.sources) == 0 {
		return nil
	}
	app.online.sourceIndex = clamp(app.online.sourceIndex, 0, len(app.online.sources)-1)
	return &app.online.sources[app.online.sourceIndex]
}
func persistSources(sources []booksource.Source) bool {
	dir, err := lib.DataDirPath()
	if err == nil {
		err = booksource.SaveSources(dir, sources)
	}
	if err != nil {
		setStatus(statusError, "保存书源失败: "+err.Error())
		return false
	}
	app.online.sources = sources
	return true
}
func importSources() {
	location := normalizeSourceImportLocation(app.uiState.input.value)
	if location == "" {
		setStatus(statusError, "请输入书源 JSON 文件或直链")
		return
	}
	client := onlineClient()
	startOnlineRequest("正在导入书源", func(ctx context.Context) func() {
		incoming, err := client.Import(ctx, location)
		if err != nil {
			return onlineError("导入失败", err)
		}
		return func() {
			if !persistSources(booksource.MergeSources(app.online.sources, incoming)) {
				return
			}
			transitionTo(modeSources)
			app.online.errors = nil
			setStatusf(statusInfo, "已导入/更新 %d 个书源，共 %d 个", len(incoming), len(app.online.sources))
		}
	})
}

func normalizeSourceImportLocation(value string) string {
	value = strings.TrimSpace(value)
	quoted := len(value) >= 2 && (value[0] == '\'' || value[0] == '"') && value[len(value)-1] == value[0]
	if quoted {
		value = value[1 : len(value)-1]
	}
	// Download links are not shell paths: preserve query escapes and tokens.
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
		return value
	}
	windowsDrive := len(value) >= 2 && value[1] == ':' && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z'))
	// Finder/terminal drops escape spaces and parentheses on POSIX systems.
	// Quoted paths and Windows separators must retain their literal backslashes.
	if !quoted && runtime.GOOS != "windows" && !windowsDrive && !strings.HasPrefix(value, `\\`) {
		value = normalizeImportInputPath(value)
	}
	if value == "~" || strings.HasPrefix(value, "~/") || (runtime.GOOS == "windows" && strings.HasPrefix(value, `~\`)) {
		if home, err := os.UserHomeDir(); err == nil {
			if value == "~" {
				return home
			}
			return filepath.Join(home, value[2:])
		}
	}
	return value
}

func toggleSource() {
	source := selectedSource()
	if source == nil {
		return
	}
	sources := append([]booksource.Source(nil), app.online.sources...)
	enabled := !source.IsEnabled()
	sources[app.online.sourceIndex].Enabled = &enabled
	if persistSources(sources) {
		setStatus(statusInfo, "书源启用状态已保存")
	}
}
func deleteSource() {
	source := selectedSource()
	if source == nil {
		return
	}
	sources := append([]booksource.Source(nil), app.online.sources[:app.online.sourceIndex]...)
	sources = append(sources, app.online.sources[app.online.sourceIndex+1:]...)
	if persistSources(sources) {
		transitionTo(modeSources)
		setStatus(statusInfo, "已移除书源，书架与缓存保留")
	}
}
func startOnlineSearch(selected bool) {
	cancelOnlineRequest()
	app.online.scopeURL = ""
	if selected {
		if source := selectedSource(); source != nil {
			app.online.scopeURL = source.URL
		}
	}
	transitionTo(modeOnlineSearchInput)
	app.readingState.showHelp = false
	app.readingState.showProgress = false
	app.uiState.input.value = app.online.keyword
	app.uiState.input.cursor = len([]rune(app.uiState.input.value))
}
func runOnlineSearch() {
	keyword := strings.TrimSpace(app.uiState.input.value)
	if keyword == "" {
		setStatus(statusError, "请输入书名或作者")
		return
	}
	app.online.keyword = keyword
	app.online.page = 1
	searchOnlinePage(1)
}
func searchOnlinePage(page int) {
	cancelOnlineRequest()
	if app.online.keyword == "" {
		startOnlineSearch(false)
		return
	}
	sources := []booksource.Source{}
	for _, s := range app.online.sources {
		if s.IsEnabled() && (app.online.scopeURL == "" || app.online.scopeURL == s.URL) {
			sources = append(sources, s)
		}
	}
	if len(sources) == 0 {
		setStatus(statusError, "没有已启用的书源，按 S 管理书源")
		return
	}
	client := onlineClient()
	keyword := app.online.keyword
	transitionTo(modeOnlineResults)
	app.online.results = nil
	app.online.errors = nil
	app.online.resultIndex = 0
	app.online.page = max(1, page)
	page = app.online.page
	// Hundreds of sources may legitimately need several batches. Only each
	// individual request has a deadline; the search lives until completion or
	// navigation/cancellation, without the generic request's 90-second cutoff.
	ctx, cancel := context.WithCancel(context.Background())
	app.online.cancel = cancel
	app.online.searching = true
	app.online.searchTotal, app.online.searchDone = len(sources), 0
	app.online.busy = fmt.Sprintf("正在搜索 %d 个书源 · 第 %d 页", len(sources), page)
	setStatus(statusProgress, app.online.busy+"（结果陆续显示，Esc 可取消）")
	owner, id := app, app.online.requestID
	enqueue := onlineUpdateQueue()
	seen := make(map[string]bool)
	go func() {
		defer cancel()
		streamOnlineSearch(ctx, sources, func(requestCtx context.Context, source booksource.Source) ([]booksource.Book, error) {
			return client.Search(requestCtx, source, keyword, page)
		}, func(response onlineSearchResponse) {
			enqueue(func() {
				if app != owner || app.online.requestID != id {
					return
				}
				app.online.searchDone++
				if response.err != nil {
					app.online.errors = append(app.online.errors, response.source.Name+": "+response.err.Error())
				} else {
					for _, book := range response.books {
						key := onlineBookID(book)
						if !seen[key] {
							seen[key] = true
							app.online.results = append(app.online.results, book)
						}
					}
				}
				if app.online.searchDone == app.online.searchTotal {
					app.online.cancel = nil
					app.online.busy = ""
					app.online.searching = false
					setStatusf(statusInfo, "找到 %d 本 · 已完成 %d/%d 个书源 · %d 个失败", len(app.online.results), app.online.searchDone, app.online.searchTotal, len(app.online.errors))
				} else {
					setStatusf(statusProgress, "已找到 %d 本 · 已完成 %d/%d 个书源 · %d 个失败（可直接阅读）", len(app.online.results), app.online.searchDone, app.online.searchTotal, len(app.online.errors))
				}
				if tApp != nil {
					applyLayoutFromAppWithoutReflow()
				}
			})
		})
	}()
}

type onlineSearchResponse struct {
	source booksource.Source
	books  []booksource.Book
	err    error
}

// streamOnlineSearch owns no UI state. A single collector publishes completed
// sources; callers marshal each publication onto their UI queue.
func streamOnlineSearch(ctx context.Context, sources []booksource.Source, search func(context.Context, booksource.Source) ([]booksource.Book, error), publish func(onlineSearchResponse)) {
	jobs := make(chan booksource.Source, len(sources))
	responses := make(chan onlineSearchResponse, min(4, len(sources)))
	for _, source := range sources {
		jobs <- source
	}
	close(jobs)
	for worker := 0; worker < min(4, len(sources)); worker++ {
		go func() {
			for source := range jobs {
				if ctx.Err() != nil {
					return
				}
				requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
				books, err := search(requestCtx, source)
				cancel()
				select {
				case responses <- onlineSearchResponse{source: source, books: books, err: err}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	for completed := 0; completed < len(sources); completed++ {
		select {
		case <-ctx.Done():
			return
		case response := <-responses:
			if ctx.Err() != nil {
				return
			}
			publish(response)
		}
	}
}
func onlineBookID(book booksource.Book) string {
	return fmt.Sprintf("online:%x", sha256.Sum256([]byte(book.SourceURL+"\n"+book.URL)))
}
func sourceForBook(book booksource.Book) (booksource.Source, bool) {
	for _, s := range app.online.sources {
		if s.URL == book.SourceURL {
			return s, true
		}
	}
	return booksource.Source{}, false
}
func openSelectedOnlineBook() {
	if len(app.online.results) == 0 {
		return
	}
	idx := clamp(app.online.resultIndex, 0, len(app.online.results)-1)
	cancelOnlineRequest()
	openOnlineBook(app.online.results[idx])
}
func openOnlineBook(book booksource.Book) {
	source, hasSource := sourceForBook(book)
	client := onlineClient()
	dir, err := lib.DataDirPath()
	if err != nil {
		setStatus(statusError, err.Error())
		return
	}
	saved, hasSaved := lib.FindBookshelfBook(app.bookshelf, onlineBookID(book))
	startOnlineRequest("正在加载目录和正文", func(ctx context.Context) func() {
		chapters, cacheErr := booksource.LoadCachedChapters(dir, book)
		if cacheErr != nil || len(chapters) == 0 {
			if !hasSource {
				return onlineError("打开失败", fmt.Errorf("书源已移除且没有目录缓存，请重新导入书源"))
			}
			book, err = client.BookInfo(ctx, source, book)
			if err != nil {
				return onlineError("获取书籍详情失败", err)
			}
			chapters, err = client.Chapters(ctx, source, book)
			if err != nil {
				return onlineError("获取目录失败", err)
			}
			if err = booksource.SaveCachedChapters(dir, book, chapters); err != nil {
				return onlineError("保存目录缓存失败", err)
			}
		}
		index := 0
		offset := 0.0
		if hasSaved {
			index = clamp(saved.ChapterIndex, 0, len(chapters)-1)
			offset = saved.ChapterOffset
			if saved.ChapterURL != "" {
				for i, c := range chapters {
					if c.URL == saved.ChapterURL {
						index = i
						break
					}
				}
			}
		}
		text, err := loadOnlineContent(ctx, client, dir, source, hasSource, book, chapters[index])
		if err != nil {
			return onlineError("获取正文失败", err)
		}
		text, titles, purificationErr := purifyOnlineChapter(ctx, dir, book, chapters, text)
		return func() {
			r := reader.NewOnlineReader(book, chapters, index, text)
			r.SetDisplayTitles(titles)
			applyOnlineReader(r, offset)
			app.online.errors = nil
			showPurificationWarning(purificationErr)
		}
	})
}
func loadOnlineContent(ctx context.Context, client *booksource.Client, dir string, source booksource.Source, hasSource bool, book booksource.Book, chapter booksource.Chapter) (string, error) {
	if text, err := booksource.LoadCachedContent(dir, book, chapter); err == nil && strings.TrimSpace(text) != "" {
		if cleaned := booksource.NormalizeContent(text); strings.TrimSpace(cleaned) != "" {
			return cleaned, nil
		}
	}
	if !hasSource {
		return "", fmt.Errorf("本章未缓存，需重新导入书源")
	}
	text, err := client.Content(ctx, source, book, chapter)
	if err != nil {
		return "", err
	}
	if err = booksource.SaveCachedContent(dir, book, chapter, text); err != nil {
		return "", fmt.Errorf("保存正文缓存: %w", err)
	}
	return text, nil
}
func applyOnlineReader(r *reader.OnlineReader, offset float64) {
	installOnlineReader(r, offset)
	syncCurrentBookState()
	if b, ok := lib.FindBookshelfBook(app.bookshelf, app.currentFile); ok {
		app.currentBook = &b
	}
}

// Install and reflow the reader without writing the library. Source switching
// uses this to calculate its final position before committing all stores.
func installOnlineReader(r *reader.OnlineReader, offset float64) {
	app.reader = r
	app.currentFile = onlineBookID(r.Book)
	app.currentBook = nil
	app.readingState.showHelp = false
	app.readingState.showProgress = false
	app.readingState.searchQuery = ""
	app.readingState.lastSearchIndex = -1
	app.readingState.tocIndex = r.Index
	app.readingState.tocNumber = ""
	transitionTo(modeReading)
	applyLayoutFromApp()
	r.RestoreOffset(offset)
	if b, ok := lib.FindBookshelfBook(app.bookshelf, app.currentFile); ok {
		app.currentBook = &b
	}
	setStatus(statusInfo, "已打开 "+r.Book.Name+" · "+r.CurrentChapterTitle())
}
func openOnlineChapter(index int, offset float64) {
	r, ok := app.reader.(*reader.OnlineReader)
	if !ok {
		return
	}
	if index < 0 || index >= len(r.Chapters) {
		setStatus(statusInfo, "已到书籍边界")
		return
	}
	source, hasSource := sourceForBook(r.Book)
	client := onlineClient()
	dir, err := lib.DataDirPath()
	if err != nil {
		setStatus(statusError, err.Error())
		return
	}
	book := r.Book
	chapters := append([]booksource.Chapter(nil), r.Chapters...)
	syncCurrentBookState()
	startOnlineRequest("正在加载 "+chapters[index].Name, func(ctx context.Context) func() {
		text, err := loadOnlineContent(ctx, client, dir, source, hasSource, book, chapters[index])
		if err != nil {
			return onlineError("加载章节失败", err)
		}
		text, titles, purificationErr := purifyOnlineChapter(ctx, dir, book, chapters, text)
		return func() {
			r := reader.NewOnlineReader(book, chapters, index, text)
			r.SetDisplayTitles(titles)
			applyOnlineReader(r, offset)
			showPurificationWarning(purificationErr)
		}
	})
}
func refreshOnlineTOC() {
	r, ok := app.reader.(*reader.OnlineReader)
	if !ok {
		return
	}
	source, hasSource := sourceForBook(r.Book)
	if !hasSource {
		setStatus(statusError, "请先重新导入该书源")
		return
	}
	client := onlineClient()
	book := r.Book
	dir, err := lib.DataDirPath()
	if err != nil {
		setStatus(statusError, err.Error())
		return
	}
	currentURL := r.Chapters[r.Index].URL
	startOnlineRequest("正在刷新目录", func(ctx context.Context) func() {
		chapters, err := client.Chapters(ctx, source, book)
		if err != nil {
			return onlineError("刷新目录失败", err)
		}
		index := -1
		for i, c := range chapters {
			if c.URL == currentURL {
				index = i
				break
			}
		}
		if index < 0 {
			return onlineError("刷新目录失败", fmt.Errorf("新目录不含当前章节，已保留原目录"))
		}
		if err := booksource.SaveCachedChapters(dir, book, chapters); err != nil {
			return onlineError("保存目录失败", err)
		}
		titles, purificationErr := loadPurifiedOnlineTitles(ctx, dir, book, chapters)
		return func() {
			r.Chapters = chapters
			r.SetDisplayTitles(titles)
			r.Index = index
			app.readingState.tocIndex = index
			syncCurrentBookState()
			setStatusf(statusInfo, "目录已刷新，共 %d 章", len(chapters))
			if purificationErr != nil {
				showPurificationWarning(purificationErr)
			}
		}
	})
}
