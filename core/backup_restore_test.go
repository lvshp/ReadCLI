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

type backupTransport func(*http.Request) (*http.Response, error)

func (f backupTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// Exercise the same files and loaders as shutdown/startup, with no original
// import file, original data directory or live website left after restoration.
func TestDataDirectoryBackupRestoresOnlineLibrary(t *testing.T) {
	originalDir, restoredDir := t.TempDir(), t.TempDir()
	t.Setenv("READCLI_DATA_DIR", originalDir)
	initTestUIState()
	initOnline()
	app.config.ReadingAlignment = "right"
	app.config.ReadingMarginLeft = 3

	importPath := filepath.Join(t.TempDir(), "sources.json")
	definition := `[
		{"bookSourceUrl":"https://backup.example.test", "bookSourceName":"备份测试源", "enabled":true,
		 "searchUrl":"/search?key={{key}}", "customField":{"keep":"完整定义"},
		 "ruleSearch":{"bookList":"$.books[*]", "name":"$.name", "bookUrl":"$.url"},
		 "ruleBookInfo":{"init":"@js: java.put('prefix', '恢复变量：'); result"},
		 "ruleContent":{"content":"@js: (java.get('prefix') || '') + result"}},
		{"bookSourceUrl":"https://disabled.example.test", "bookSourceName":"停用源", "enabled":false, "searchUrl":"/s"}
	]`
	if err := os.WriteFile(importPath, []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	sources, err := booksource.Import(context.Background(), importPath)
	if err != nil {
		t.Fatal(err)
	}
	if !persistSources(sources) {
		t.Fatal("source import was not saved")
	}
	if err := os.Remove(importPath); err != nil {
		t.Fatal(err)
	}
	source := sources[0]
	book := booksource.Book{Name: "备份测试书", URL: source.URL + "/book", SourceURL: source.URL, SourceName: source.Name}
	chapters := []booksource.Chapter{
		{Name: "第一章", URL: source.URL + "/1"},
		{Name: "第二章", URL: source.URL + "/2"},
		{Name: "第三章", URL: source.URL + "/3"},
	}
	requests := 0
	transport := backupTransport(func(req *http.Request) (*http.Response, error) {
		requests++
		cookie, err := req.Cookie("session")
		if err != nil || cookie.Value != "backup-fixture" {
			return nil, fmt.Errorf("saved login cookie missing")
		}
		var body string
		switch req.URL.Path {
		case "/book":
			body = "{}"
		case "/search":
			body = `{"books":[{"name":"备份测试书","url":"/book"}]}`
		case "/3":
			body = "第三章正文"
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})
	app.online.client.HTTPClient.Transport = transport
	if err := app.online.client.Login(context.Background(), source, map[string]string{"Cookie": "session=backup-fixture"}); err != nil {
		t.Fatal(err)
	}
	book, err = app.online.client.BookInfo(context.Background(), source, book)
	if err != nil {
		t.Fatal(err)
	}
	if err := booksource.SaveCachedChapters(originalDir, book, chapters); err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("已缓存的第二章正文\n", 80)
	if err := booksource.SaveCachedContent(originalDir, book, chapters[1], body); err != nil {
		t.Fatal(err)
	}
	applyOnlineReader(reader.NewOnlineReader(book, chapters, 1, body), 0.5)
	saveBookmark()
	persistState()
	bookID := app.currentFile
	wantShelf, wantProgress, wantBookmarks := app.bookshelf, app.progress, app.bookmarks
	wantConfig := *app.config
	wantSources, err := json.Marshal(sources)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config.json", "book_sources.json", "sessions.json", "bookshelf.json", "progress.json", "bookmarks.json", "online_cache"} {
		if _, err := os.Stat(filepath.Join(originalDir, name)); err != nil {
			t.Fatalf("missing backup entry %s: %v", name, err)
		}
	}
	if err := os.CopyFS(restoredDir, os.DirFS(originalDir)); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(originalDir); err != nil {
		t.Fatal(err)
	}

	// A fresh app loads only the restored folder, just as Run does at startup.
	t.Setenv("READCLI_DATA_DIR", restoredDir)
	initTestUIState()
	if app.config, err = lib.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	if app.bookshelf, err = lib.LoadBookshelf(); err != nil {
		t.Fatal(err)
	}
	if app.bookmarks, err = lib.LoadBookmarks(); err != nil {
		t.Fatal(err)
	}
	if app.progress, err = lib.LoadProgress(); err != nil {
		t.Fatal(err)
	}
	initOnline()
	if !reflect.DeepEqual(*app.config, wantConfig) || !reflect.DeepEqual(app.bookshelf, wantShelf) || !reflect.DeepEqual(app.progress, wantProgress) || !reflect.DeepEqual(app.bookmarks, wantBookmarks) {
		t.Fatal("restored settings, shelf, progress or bookmarks differ")
	}
	gotSources, err := json.Marshal(app.online.sources)
	if err != nil || string(gotSources) != string(wantSources) {
		t.Fatalf("source definitions or enabled states changed: %v", err)
	}
	if !app.online.client.IsLoggedIn(source) {
		t.Fatal("restored session was not loaded at startup")
	}
	app.online.client.HTTPClient.Transport = transport
	saved, found := lib.FindBookshelfBook(app.bookshelf, bookID)
	if !found || saved.Online == nil || saved.ChapterURL != chapters[1].URL || saved.ChapterIndex != 1 || saved.ChapterOffset < 0.45 {
		t.Fatal("online book or chapter position not restored")
	}
	gotChapters, err := booksource.LoadCachedChapters(restoredDir, *saved.Online)
	if err != nil || !reflect.DeepEqual(gotChapters, chapters) {
		t.Fatalf("chapter cache not restored: %v", err)
	}
	before := requests
	gotBody, err := loadOnlineContent(context.Background(), app.online.client, restoredDir, source, true, *saved.Online, gotChapters[1])
	if err != nil || gotBody != strings.TrimSpace(body) || requests != before {
		t.Fatalf("cached chapter should load without a request: %v", err)
	}
	marks := app.bookmarks.Books[bookID]
	if len(marks) != 1 || marks[0].ChapterURL != chapters[1].URL || marks[0].ChapterOffset < 0.45 {
		t.Fatal("online bookmark not restored")
	}
	results, err := app.online.client.Search(context.Background(), app.online.sources[0], "备份", 1)
	if err != nil || len(results) != 1 || results[0].URL != book.URL {
		t.Fatalf("restored source/session cannot search: %v", err)
	}
	gotBody, err = loadOnlineContent(context.Background(), app.online.client, restoredDir, app.online.sources[0], true, *saved.Online, gotChapters[2])
	if err != nil || gotBody != "恢复变量：第三章正文" {
		t.Fatalf("uncached chapter lost saved script variables: body=%q, error=%v", gotBody, err)
	}
}
