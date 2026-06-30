package core

import (
	"strings"
	"testing"

	"github.com/lvshp/ReadCLI/lib"
)

type fakeReader struct {
	pos         int
	total       int
	chapter     string
	gotoChapter int
	toc         string
}

func (r *fakeReader) Load(path string) error       { return nil }
func (r *fakeReader) Reflow(width int)             {}
func (r *fakeReader) BookTitle() string            { return "Fake Book" }
func (r *fakeReader) Current() string              { return "current" }
func (r *fakeReader) CurrentView(lines int) string { return "line one\nline two\nline three" }
func (r *fakeReader) Total() int                   { return r.total }
func (r *fakeReader) Search(query string, start int, forward bool) (int, bool) {
	return 0, false
}
func (r *fakeReader) Next() string { return "" }
func (r *fakeReader) Prev() string { return "" }
func (r *fakeReader) First() string {
	r.pos = 0
	return ""
}
func (r *fakeReader) Last() string {
	r.pos = max(0, r.total-1)
	return ""
}
func (r *fakeReader) CurrentPos() int { return r.pos }
func (r *fakeReader) Goto(pos int) string {
	if pos < 0 {
		pos = 0
	}
	if r.total > 0 && pos >= r.total {
		pos = r.total - 1
	}
	r.pos = pos
	return ""
}
func (r *fakeReader) GetProgress() string         { return "" }
func (r *fakeReader) CurrentChapterTitle() string { return r.chapter }
func (r *fakeReader) CurrentChapterIndex() int    { return 0 }
func (r *fakeReader) NextChapter() string         { return "" }
func (r *fakeReader) PrevChapter() string         { return "" }
func (r *fakeReader) GetTOC() string              { return r.toc }
func (r *fakeReader) GetTOCWithSelection(selected, pageSize int) string {
	return ""
}
func (r *fakeReader) GotoChapter(index int) string {
	r.gotoChapter = index
	return ""
}

func TestVisibleBooksFiltersByTitleAndKeepsSort(t *testing.T) {
	app = &appState{
		bookshelf: &lib.BookshelfStore{Books: []lib.BookshelfBook{
			{Title: "Z Mars", Path: "/books/z.epub", Format: "epub"},
			{Title: "Earth", Path: "/mars/earth.epub", Format: "epub"},
			{Title: "A Mars", Path: "/books/a.epub", Format: "epub"},
		}},
		bookshelfState: bookshelfState{
			filterMode: "all",
			sortMode:   "title",
			query:      "mars",
		},
	}

	books := visibleBooks()
	if len(books) != 2 {
		t.Fatalf("visibleBooks() returned %d books, want 2", len(books))
	}
	if books[0].Title != "A Mars" || books[1].Title != "Z Mars" {
		t.Fatalf("visibleBooks() order = %q, %q; want A Mars, Z Mars", books[0].Title, books[1].Title)
	}
}

func TestCompactReadingStatusLineIncludesChapterPercentAndPosition(t *testing.T) {
	app = &appState{
		reader: &fakeReader{pos: 4, total: 9, chapter: "第一百二十八章"},
	}
	mainContentWidth = 60

	line := compactReadingStatusLine()
	for _, want := range []string{"第一百二十八章", "50%", "5/9"} {
		if !strings.Contains(line, want) {
			t.Fatalf("compactReadingStatusLine() = %q, missing %q", line, want)
		}
	}
}

func TestHomeInspectorDoesNotExposeCompactMode(t *testing.T) {
	app = &appState{
		mode:   modeHome,
		config: &lib.Config{Theme: "vscode"},
		readingState: readingState{
			compactMode: true,
		},
		bookshelf: &lib.BookshelfStore{Books: []lib.BookshelfBook{
			{
				Title:           "一本书",
				Path:            "/books/demo.epub",
				Format:          "epub",
				ProgressPercent: 12,
				CurrentChapter:  "第一章",
			},
		}},
		bookshelfState: bookshelfState{
			filterMode: "all",
			sortMode:   "title",
		},
	}

	panel := buildRightPanel(currentTheme())
	if strings.Contains(panel, "阅读界面") || strings.Contains(panel, "精简") {
		t.Fatalf("home inspector should not expose compact mode: %q", panel)
	}
}

func TestFormatCompactReadingPanelCentersNarrowTextBlock(t *testing.T) {
	app = &appState{
		mode: modeReading,
		readingState: readingState{
			compactMode:  true,
			contentWidth: 40,
		},
		config: &lib.Config{
			ReadingLineSpacing: 1,
		},
	}
	mainContentWidth = 80
	mainContentHeight = 24

	got := formatReadingPanel("第一行\n第二行")
	want := "\n\n                    第一行\n\n                    第二行"
	if got != want {
		t.Fatalf("formatReadingPanel() = %q, want %q", got, want)
	}
}

func TestRunReadingJumpSupportsPercentAndChapter(t *testing.T) {
	reader := &fakeReader{total: 101, chapter: "开始"}
	app = &appState{
		reader: reader,
		mode:   modeReadingJumpInput,
		uiState: uiState{
			input: inputState{value: "50%"},
		},
	}
	runReadingJump()
	if reader.pos != 50 {
		t.Fatalf("percent jump pos = %d, want 50", reader.pos)
	}

	app.uiState.input.value = "128"
	app.mode = modeReadingJumpInput
	runReadingJump()
	if reader.gotoChapter != 127 {
		t.Fatalf("chapter jump index = %d, want 127", reader.gotoChapter)
	}
}

func TestStatusExpiryUsesKind(t *testing.T) {
	if expiry := statusExpiry(statusProgress, "正在打开"); !expiry.IsZero() {
		t.Fatalf("progress status should not expire, got %v", expiry)
	}
	if expiry := statusExpiry(statusPersistent, "保持显示"); !expiry.IsZero() {
		t.Fatalf("persistent status should not expire, got %v", expiry)
	}
	if expiry := statusExpiry(statusInfo, "普通提示"); expiry.IsZero() {
		t.Fatal("info status should expire")
	}
	if expiry := statusExpiry(statusError, "错误提示"); expiry.IsZero() {
		t.Fatal("error status should expire")
	}
}

func TestEscapeFromBookshelfSearchKeepsSpecificStatus(t *testing.T) {
	app = &appState{
		mode: modeBookshelfSearchInput,
		bookshelfState: bookshelfState{
			query: "mars",
		},
	}

	handleTextInputEvent("<Escape>", nil)
	if app.uiState.statusMessage != "书架搜索已取消" {
		t.Fatalf("statusMessage = %q, want 书架搜索已取消", app.uiState.statusMessage)
	}
}

func TestEscapeFromReadingJumpKeepsSpecificStatus(t *testing.T) {
	app = &appState{
		mode: modeReadingJumpInput,
	}

	handleTextInputEvent("<Escape>", nil)
	if app.uiState.statusMessage != "已取消跳转" {
		t.Fatalf("statusMessage = %q, want 已取消跳转", app.uiState.statusMessage)
	}
}

func TestUpdateTOCSelectionClampsBounds(t *testing.T) {
	app = &appState{
		reader: &fakeReader{toc: "目录\n一\n二\n三"},
		readingState: readingState{
			tocIndex: 1,
		},
	}

	updateTOCSelection(10)
	if app.readingState.tocIndex != 2 {
		t.Fatalf("tocIndex = %d, want 2", app.readingState.tocIndex)
	}

	updateTOCSelection(-10)
	if app.readingState.tocIndex != 0 {
		t.Fatalf("tocIndex = %d, want 0", app.readingState.tocIndex)
	}
}
