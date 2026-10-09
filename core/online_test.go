package core

import (
	"context"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/lvshp/ReadCLI/booksource"
	"github.com/lvshp/ReadCLI/lib"
	"github.com/lvshp/ReadCLI/reader"
)

func TestOnlineReadingPersistsMetadataAndBookmarks(t *testing.T) {
	t.Setenv("READCLI_DATA_DIR", t.TempDir())
	initTestUIState()
	book := booksource.Book{Name: "网络书", URL: "https://example.test/book", SourceURL: "https://example.test", SourceName: "示例"}
	chapters := []booksource.Chapter{{Name: "第一章", URL: "https://example.test/1"}, {Name: "第二章", URL: "https://example.test/2"}}
	r := reader.NewOnlineReader(book, chapters, 1, strings.Repeat("中文正文一行\n", 60))
	applyOnlineReader(r, 0.5)
	saved, err := lib.LoadBookshelf()
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Books) != 1 || saved.Books[0].Online == nil || saved.Books[0].Format != "online" {
		t.Fatalf("shelf=%+v", saved)
	}
	b := saved.Books[0]
	if b.ChapterIndex != 1 || b.ChapterURL != chapters[1].URL || b.ProgressPercent < 70 || b.ProgressPercent > 80 {
		t.Fatalf("progress=%+v", b)
	}
	saveBookmark()
	marks := app.bookmarks.Books[app.currentFile]
	if len(marks) != 1 || marks[0].ChapterURL != chapters[1].URL || marks[0].ChapterOffset < 0.45 {
		t.Fatalf("bookmark=%+v", marks)
	}
	syncCurrentBookState()
	if len(app.bookshelf.Books) != 1 || app.bookshelf.Books[0].Online == nil {
		t.Fatal("book metadata lost on sync")
	}
	app.bookshelfState.deleteTargetPath = app.currentFile
	removeSelectedBook(false)
	if len(app.bookshelf.Books) != 0 || len(app.progress.Anchors) != 0 {
		t.Fatal("online shelf cleanup incomplete")
	}
}
func TestSourceManagementNavigationAndPasswordMask(t *testing.T) {
	t.Setenv("READCLI_DATA_DIR", t.TempDir())
	initTestUIState()
	app.online.sources = []booksource.Source{{URL: "https://example.test", Name: "测试源", SearchURL: "/search"}}
	dispatchEvent("S")
	if app.mode != modeSources {
		t.Fatal(app.mode)
	}
	dispatchEvent("<Space>")
	if app.online.sources[0].IsEnabled() {
		t.Fatal("source not disabled")
	}
	dispatchEvent("<Space>")
	if !app.online.sources[0].IsEnabled() {
		t.Fatal("source not enabled")
	}
	dispatchEvent("/")
	if app.mode != modeOnlineSearchInput || app.online.scopeURL != "https://example.test" {
		t.Fatal("selected-source search missing")
	}
	dispatchEvent("<Escape>")
	dispatchEvent("S")
	app.online.loginFields = []booksource.LoginField{{Name: "密码", Secret: true}}
	app.online.loginValues = map[string]string{}
	app.online.loginIndex = 0
	app.mode = modeSourceLogin
	app.uiState.input.value = "test-secret"
	app.uiState.input.cursor = len("test-secret")
	if strings.Contains(buildSourceLoginPanel(), "test-secret") {
		t.Fatal("password exposed")
	}
	dispatchEvent("<Escape>")
	if app.online.loginValues != nil || app.uiState.input.value != "" || app.mode != modeSources {
		t.Fatal("login input not cleared")
	}
	dispatchEvent("d")
	if app.mode != modeSourceConfirm || len(app.sourceManager.pending.keys) != 1 || app.sourceManager.pending.keys[0] != "https://example.test" {
		t.Fatal("delete confirmation missing")
	}
	dispatchEvent("y")
	if len(app.online.sources) != 0 {
		t.Fatal("source not removed")
	}
}
func TestOnlinePanelsRenderOnSmallTerminal(t *testing.T) {
	initTestUIState()
	app.online.sources = []booksource.Source{{URL: "https://example.test", Name: "[red]远程书源", SearchURL: "/s"}}
	app.online.results = []booksource.Book{{Name: "[red]远程书名", Author: "作者", SourceName: "源"}}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	for _, m := range []mode{modeSources, modeOnlineResults, modeSourceImport, modeOnlineSearchInput} {
		app.mode = m
		for _, size := range [][2]int{{80, 24}, {48, 16}, {140, 40}} {
			screen.SetSize(size[0], size[1])
			root.SetRect(0, 0, size[0], size[1])
			applyLayout(size[0], size[1])
			root.Draw(screen)
			if strings.Contains(buildOnlinePanel(), "[red]") {
				t.Fatal("metadata markup not escaped")
			}
		}
	}
}

func TestOnlineLoadCancelsOnNavigationAndLocalOpen(t *testing.T) {
	initTestUIState()
	for _, m := range []mode{modeHome, modeReading, modeTOC, modeBookmarks} {
		app.mode = m
		ctx, cancel := context.WithCancel(context.Background())
		app.online.cancel = cancel
		app.online.busy = "loading"
		dispatchEvent("<Escape>")
		if ctx.Err() == nil || app.online.busy != "" {
			t.Fatalf("Esc failed in %s", m)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	app.online.cancel = cancel
	app.online.busy = "loading"
	app.mode = modeTOC
	transitionTo(modeHome)
	if ctx.Err() == nil {
		t.Fatal("navigation failed to cancel download")
	}
	ctx, cancel = context.WithCancel(context.Background())
	app.online.cancel = cancel
	app.online.busy = "loading"
	openSelectedBook()
	if ctx.Err() == nil {
		t.Fatal("switching book failed to cancel download")
	}
}
func TestLastOnlinePageCanFinishBook(t *testing.T) {
	t.Setenv("READCLI_DATA_DIR", t.TempDir())
	initTestUIState()
	r := reader.NewOnlineReader(booksource.Book{Name: "在线书", URL: "https://example.test/b", SourceURL: "https://example.test"}, []booksource.Chapter{{Name: "末章", URL: "https://example.test/1"}}, 0, "一\n二\n三")
	applyOnlineReader(r, 0)
	moveReading(8)
	if r.CurrentPos() != r.Total()-1 || app.bookshelf.Books[0].ProgressPercent != 100 {
		t.Fatal("last page did not finish")
	}
	if currentDisplayName() != "在线书" {
		t.Fatal(currentDisplayName())
	}
}

func TestCachedOnlineChapterLoadsWithoutSource(t *testing.T) {
	dir := t.TempDir()
	book := booksource.Book{URL: "https://example.test/book", SourceURL: "source"}
	chapter := booksource.Chapter{URL: "https://example.test/1"}
	if err := booksource.SaveCachedContent(dir, book, chapter, "缓存正文"); err != nil {
		t.Fatal(err)
	}
	text, err := loadOnlineContent(context.Background(), booksource.NewClient(), dir, booksource.Source{}, false, book, chapter)
	if err != nil || text != "缓存正文" {
		t.Fatalf("offline content=%q, %v", text, err)
	}
}

func TestCachedOnlineHTMLIsReadableWithoutSource(t *testing.T) {
	dir := t.TempDir()
	book := booksource.Book{URL: "https://example.test/book", SourceURL: "source"}
	chapter := booksource.Chapter{URL: "https://example.test/1"}
	raw := "<p>第一段<b>正文</b></p><p>第二段<br>下一行</p>"
	if err := booksource.SaveCachedContent(dir, book, chapter, raw); err != nil {
		t.Fatal(err)
	}
	text, err := loadOnlineContent(context.Background(), booksource.NewClient(), dir, booksource.Source{}, false, book, chapter)
	if err != nil || strings.Contains(text, "<p>") || strings.Contains(text, "<br>") || !strings.Contains(text, "第一段正文") || !strings.Contains(text, "\n") {
		t.Fatalf("cached HTML=%q, %v", text, err)
	}
	// Keep the original cache so later replacement-rule edits remain reversible.
	stored, err := booksource.LoadCachedContent(dir, book, chapter)
	if err != nil || stored != raw {
		t.Fatalf("raw cache modified: %q, %v", stored, err)
	}
}

func TestOnlineChapterStartCountsAsReading(t *testing.T) {
	b := lib.BookshelfBook{Format: "online", Online: &booksource.Book{}, ChapterIndex: 2, ProgressPos: 0, ProgressPercent: 5}
	if len(lib.FilterBooks([]lib.BookshelfBook{b}, "reading")) != 1 || len(lib.FilterBooks([]lib.BookshelfBook{b}, "unread")) != 0 {
		t.Fatal("chapter-start progress was treated as unread")
	}
}

func TestSmallTerminalKeepsStatusAndPanelHeaders(t *testing.T) {
	initTestUIState()
	app.mode = modeSources
	app.online.sources = []booksource.Source{{URL: "https://example.test", Name: "测试源", SearchURL: "/s"}}
	setStatus(statusProgress, "加载状态测试")
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)
	root.SetRect(0, 0, 80, 24)
	lastTermWidth = 80
	lastTermHeight = 24
	applyLayout(80, 24)
	root.Draw(screen)
	var rendered strings.Builder
	for y := 0; y < 24; y++ {
		for x := 0; x < 80; x++ {
			r, _, _, _ := screen.GetContent(x, y)
			rendered.WriteRune(r)
		}
	}
	if !strings.Contains(rendered.String(), "ReadCLI") {
		t.Fatal("status line scrolled out of view")
	}
}
