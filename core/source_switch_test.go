package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lvshp/ReadCLI/booksource"
	"github.com/lvshp/ReadCLI/lib"
	"github.com/lvshp/ReadCLI/reader"
)

type switchTransport func(*http.Request) (*http.Response, error)

func (f switchTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func switchFixture(t *testing.T, failure string) (string, lib.BookshelfBook, booksource.Source, booksource.Book) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("READCLI_DATA_DIR", dir)
	initTestUIState()
	initOnline()
	old := booksource.Book{SourceURL: "https://old.example.test", SourceName: "原书源", URL: "https://old.example.test/book", Name: "测试小说", Author: "作者"}
	chapters := []booksource.Chapter{{Name: "第一章 初见", URL: old.SourceURL + "/1"}, {Name: "第二章 重逢", URL: old.SourceURL + "/2"}}
	applyOnlineReader(reader.NewOnlineReader(old, chapters, 1, strings.Repeat("旧正文\n", 120)), 0.5)
	app.bookshelf.Books[0].ImportedAt = "2026-01-01T00:00:00Z"
	saveBookmark()
	app.bookmarks.Books[app.currentFile] = append(app.bookmarks.Books[app.currentFile], lib.Bookmark{Path: app.currentFile, Chapter: "缺失的番外", ChapterURL: old.SourceURL + "/extra", CreatedAt: "2026-01-02T00:00:00Z", Snippet: "旧源番外"})
	persistState()
	original := app.bookshelf.Books[0]
	source := booksource.Source{URL: "https://new.example.test", Name: "新书源", SearchURL: "/search", BookInfo: booksource.BookRule{Name: "$.name", TOCURL: "$.toc"}, TOC: booksource.TOCRule{ChapterList: "$.chapters[*]", ChapterName: "$.name", ChapterURL: "$.url"}, Content: booksource.ContentRule{Content: "@js: result"}}
	if failure == "empty after cleanup" {
		source.Content.ReplaceRegex = "##(?s).*##"
	}
	candidate := booksource.Book{SourceURL: source.URL, SourceName: source.Name, Name: old.Name, Author: old.Author, URL: source.URL + "/book", LastChapter: "顶点 第三章 归途"}
	app.online.sources = []booksource.Source{source}
	app.online.client.HTTPClient.Transport = switchTransport(func(req *http.Request) (*http.Response, error) {
		body := ""
		status := 200
		switch req.URL.Path {
		case "/book":
			if failure == "details" {
				status = 503
			}
			body = `{"name":"测试小说","toc":"/toc"}`
		case "/toc":
			body = `{"chapters":[{"name":"前言","url":"/intro"},{"name":"第1章 初见","url":"/1"},{"name":"第2章 重逢","url":"/2"},{"name":"第3章 归途","url":"/3"}]}`
			if failure == "catalog" {
				body = `{"chapters":[]}`
			}
		case "/2":
			if failure == "content" {
				return nil, fmt.Errorf("chapter unavailable")
			}
			body = strings.Repeat("新来源正文\n", 160)
		default:
			return nil, fmt.Errorf("unexpected request %s", req.URL.Path)
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})
	return dir, original, source, candidate
}

func TestSourceSwitchPrepareAndConfirmPersists(t *testing.T) {
	dir, original, source, candidate := switchFixture(t, "")
	oldShelf, err := os.ReadFile(filepath.Join(dir, "bookshelf.json"))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := prepareAlternative(context.Background(), app.online.client, dir, source, original, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.matched || plan.index != 2 || plan.offset < 0.45 {
		t.Fatalf("wrong alignment: %+v", plan)
	}
	unchanged, _ := os.ReadFile(filepath.Join(dir, "bookshelf.json"))
	if string(oldShelf) != string(unchanged) || app.currentFile != original.Path {
		t.Fatal("preparing a source changed the current book")
	}
	newID := onlineBookID(plan.book)
	// Switching to a version already on the shelf must merge its bookmarks,
	// replace the selected entry and preserve unrelated local books.
	app.bookshelf.Books = append(app.bookshelf.Books, lib.BookshelfBook{Path: newID, Online: &candidate}, lib.BookshelfBook{Path: "/local.epub", Title: "本地书", Format: "epub"})
	app.bookmarks.Books[newID] = []lib.Bookmark{{Path: newID, Chapter: "第1章 初见", ChapterURL: source.URL + "/1", CreatedAt: "2026-02-01T00:00:00Z", Snippet: "目标原有书签"}}
	app.sourceSwitch = sourceSwitchState{original: original, returnMode: modeHome, prepared: plan}
	app.mode = modeSourceSwitchConfirm
	confirmSourceSwitch()
	if app.mode != modeReading || app.currentFile != newID || len(app.bookshelf.Books) != 2 {
		t.Fatalf("switch not installed: mode=%s, shelf=%+v", app.mode, app.bookshelf)
	}
	if _, found := lib.FindBookshelfBook(app.bookshelf, original.Path); found {
		t.Fatal("old source remains as a duplicate shelf entry")
	}
	saved, err := lib.LoadBookshelf()
	if err != nil {
		t.Fatal(err)
	}
	book, exists := lib.FindBookshelfBook(saved, newID)
	if !exists || book.ImportedAt != original.ImportedAt || book.ChapterIndex != 2 || book.ChapterURL != source.URL+"/2" || book.ChapterOffset < 0.45 || book.Online == nil || book.Online.SourceURL != source.URL {
		t.Fatalf("wrong restored shelf entry: %+v", book)
	}
	progress, err := lib.LoadProgress()
	if err != nil || progress.Anchors[newID].ChapterIndex != 2 {
		t.Fatalf("new progress missing: %v", err)
	}
	marks, err := lib.LoadBookmarks()
	if err != nil || len(marks.Books[newID]) != 3 || len(marks.Books[original.Path]) != 2 {
		t.Fatalf("bookmarks lost: %+v %v", marks, err)
	}
	if marks.Books[newID][1].ChapterURL != source.URL+"/2" || marks.Books[newID][2].ChapterURL != original.Online.SourceURL+"/extra" {
		t.Fatal("matched bookmark not migrated or unmatched record destroyed")
	}
	chapters, err := booksource.LoadCachedChapters(dir, *book.Online)
	if err != nil || len(chapters) != 4 {
		t.Fatalf("new catalog not persisted: %v", err)
	}
	if _, err := booksource.LoadCachedContent(dir, *book.Online, chapters[book.ChapterIndex]); err != nil {
		t.Fatalf("switched chapter not cached: %v", err)
	}
}

func TestSourceSwitchPreparationFailureRetainsOriginal(t *testing.T) {
	for _, failure := range []string{"details", "catalog", "content", "empty after cleanup"} {
		t.Run(failure, func(t *testing.T) {
			dir, original, source, candidate := switchFixture(t, failure)
			before, _ := json.Marshal(app.bookshelf)
			oldReader := app.reader
			if _, err := prepareAlternative(context.Background(), app.online.client, dir, source, original, candidate); err == nil {
				t.Fatal("invalid source was ready for confirmation")
			}
			after, _ := json.Marshal(app.bookshelf)
			if string(before) != string(after) || app.reader != oldReader || app.currentFile != original.Path {
				t.Fatal("failed preparation changed the original book")
			}
		})
	}
}

func TestSourceSwitchSaveFailureRestoresReaderAndShelf(t *testing.T) {
	dir, original, source, candidate := switchFixture(t, "")
	plan, err := prepareAlternative(context.Background(), app.online.client, dir, source, original, candidate)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "bookshelf.json"))
	oldReader, oldShelf := app.reader, app.bookshelf
	// Fail after bookmarks.json has been replaced but before the shelf commit.
	if err := os.Rename(filepath.Join(dir, "progress.json"), filepath.Join(dir, "progress.before.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "progress.json"), 0700); err != nil {
		t.Fatal(err)
	}
	app.sourceSwitch = sourceSwitchState{original: original, returnMode: modeHome, prepared: plan}
	app.mode = modeSourceSwitchConfirm
	confirmSourceSwitch()
	after, _ := os.ReadFile(filepath.Join(dir, "bookshelf.json"))
	if string(before) != string(after) || app.reader != oldReader || app.bookshelf != oldShelf || app.currentFile != original.Path || app.mode != modeSourceSwitchConfirm || app.uiState.statusMessageKind != statusError {
		t.Fatal("failed save changed the shelf/reader or reported success")
	}
	marks, err := lib.LoadBookmarks()
	if err != nil || !reflect.DeepEqual(marks.Books[original.Path], app.bookmarks.Books[original.Path]) {
		t.Fatal("partial commit lost the old book's bookmarks")
	}
}

func TestSourceSwitchCancelAndLocalBook(t *testing.T) {
	_, original, _, _ := switchFixture(t, "")
	oldReader := app.reader
	ctx, cancel := context.WithCancel(context.Background())
	app.online.cancel, app.online.busy = cancel, "loading"
	app.sourceSwitch = sourceSwitchState{original: original, returnMode: modeReading}
	app.mode = modeSourceSwitch
	dispatchEvent("<Escape>")
	if ctx.Err() == nil || app.mode != modeReading || app.reader != oldReader || app.currentFile != original.Path || app.sourceSwitch.original.Online != nil {
		t.Fatal("cancel did not restore the original scene")
	}
	app.mode = modeHome
	app.bookshelf.Books = []lib.BookshelfBook{{Title: "本地书", Path: "/local.txt", Format: "txt"}}
	dispatchEvent("C")
	if app.mode != modeHome || app.online.busy != "" {
		t.Fatal("local file incorrectly entered source switching")
	}
}

func TestSourceSwitchBookmarkRoundTrip(t *testing.T) {
	_, original, _, _ := switchFixture(t, "")
	oldReader := app.reader.(*reader.OnlineReader)
	book := booksource.Book{Name: original.Title, URL: "https://b.test/book", SourceURL: "https://b.test"}
	plan := &sourceSwitchPlan{book: book, chapters: []booksource.Chapter{{Name: "第1章 初见", URL: "https://b.test/1"}, {Name: "第2章 重逢", URL: "https://b.test/2"}}, index: 1}
	r := reader.NewOnlineReader(book, plan.chapters, 1, "正文")
	shelf, progress, marks := switchedLibrary(original, plan, r)
	app.bookshelf, app.progress, app.bookmarks = shelf, progress, marks
	second := shelf.Books[0]
	back := &sourceSwitchPlan{book: *original.Online, chapters: oldReader.Chapters, index: original.ChapterIndex}
	_, _, restored := switchedLibrary(second, back, oldReader)
	if len(restored.Books[original.Path]) != 2 || restored.Books[original.Path][0].ChapterURL != original.ChapterURL {
		t.Fatalf("round trip duplicated or lost original bookmarks: %+v", restored.Books[original.Path])
	}
}
