package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lvshp/ReadCLI/booksource"
	"github.com/lvshp/ReadCLI/lib"
	"github.com/lvshp/ReadCLI/purification"
	"github.com/lvshp/ReadCLI/reader"
)

func TestOnlinePurificationReloadsRawCacheAndKeepsSourceTitles(t *testing.T) {
	dir := t.TempDir()
	if err := purification.Ensure(dir); err != nil {
		t.Fatal(err)
	}
	importPath := filepath.Join(t.TempDir(), "导入规则 (1).json")
	if err := os.WriteFile(importPath, []byte(`[
		{"name":"去广告","isRegex":false,"pattern":"推广链接","replacement":""},
		{"name":"统一称谓","isRegex":false,"pattern":"角色甲","replacement":"角色乙"},
		{"name":"标题","isRegex":false,"pattern":" 第一章 ","replacement":"第1章","scopeTitle":true,"scopeContent":false}
	]`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := purification.Import(context.Background(), dir, importPath); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "replace_rules", "main.json")
	if err := os.WriteFile(mainPath, []byte(`[{"name":"我的称谓","isRegex":false,"pattern":"角色乙","replacement":"主角"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	book := booksource.Book{Name: "测试小说", URL: "https://example.test/book", SourceURL: "https://example.test"}
	chapters := []booksource.Chapter{{Name: " 第一章 ", URL: "https://example.test/1"}}
	raw := "<p>角色甲登场</p><p>推广链接</p><p>原样保留</p>"
	if err := booksource.SaveCachedContent(dir, book, chapters[0], raw); err != nil {
		t.Fatal(err)
	}
	read := func(dataDir string) (string, []string, error) {
		t.Helper()
		body, err := loadOnlineContent(context.Background(), booksource.NewClient(), dataDir, booksource.Source{}, false, book, chapters[0])
		if err != nil {
			t.Fatal(err)
		}
		return purifyOnlineChapter(context.Background(), dataDir, book, chapters, body)
	}
	body, titles, err := read(dir)
	if err != nil || strings.Contains(body, "推广链接") || !strings.Contains(body, "主角登场") || len(titles) != 1 || titles[0] != "第1章" {
		t.Fatalf("purification=%q titles=%v err=%v", body, titles, err)
	}
	r := reader.NewOnlineReader(book, chapters, 0, body)
	r.SetDisplayTitles(titles)
	if r.CurrentChapterTitle() != "第1章" || r.Chapters[0].Name != " 第一章 " || !strings.Contains(r.GetTOCWithSelection(0, 10), "第1章") {
		t.Fatal("display titles must not overwrite source metadata")
	}
	if err := os.WriteFile(mainPath, []byte(`[{"name":"修改后","isRegex":false,"pattern":"角色乙","replacement":"配角"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	body, _, err = read(dir)
	if err != nil || !strings.Contains(body, "配角登场") || strings.Contains(body, "主角") {
		t.Fatalf("local edit was not reloaded: %q, %v", body, err)
	}
	stored, err := booksource.LoadCachedContent(dir, book, chapters[0])
	if err != nil || stored != raw {
		t.Fatal("purification changed the original cache")
	}
	// Restoring the folder is sufficient; imported source files can disappear.
	restored := t.TempDir()
	if err := os.CopyFS(restored, os.DirFS(dir)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(importPath); err != nil {
		t.Fatal(err)
	}
	restoredBody, _, err := read(restored)
	if err != nil || restoredBody != body {
		t.Fatalf("restored rules=%q, %v", restoredBody, err)
	}
}

func TestOnlinePurificationErrorsKeepOriginalText(t *testing.T) {
	for _, rules := range []string{
		`{broken`,
		`[{"name":"错误正则","pattern":"("}]`,
		`[{"name":"脚本错误","pattern":"正文","replacement":"@js:throw new Error('bad')"}]`,
		`[{"name":"全部删除","pattern":"正文","replacement":""}]`,
	} {
		t.Run(rules, func(t *testing.T) {
			dir := t.TempDir()
			if err := purification.Ensure(dir); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "replace_rules", "main.json"), []byte(rules), 0600); err != nil {
				t.Fatal(err)
			}
			body, titles, err := purifyOnlineChapter(context.Background(), dir, booksource.Book{}, nil, "正文")
			if err == nil || body != "正文" || len(titles) != 0 {
				t.Fatalf("failed rule must keep original: %q %v %v", body, titles, err)
			}
		})
	}
}

func TestPurifiedTitlesKeepOriginalNamesForSourceSwitch(t *testing.T) {
	t.Setenv("READCLI_DATA_DIR", t.TempDir())
	initTestUIState()
	book := booksource.Book{Name: "原书", SourceURL: "old", URL: "old/book"}
	chapters := []booksource.Chapter{{Name: "第1章 开始", URL: "old/1"}, {Name: "第2章 重逢", URL: "old/2"}}
	r := reader.NewOnlineReader(book, chapters, 1, strings.Repeat("正文\n", 100))
	r.SetDisplayTitles([]string{"隐藏编号", "同样的显示标题"})
	applyOnlineReader(r, .4)
	saveBookmark()
	original, ok := lib.FindBookshelfBook(app.bookshelf, app.currentFile)
	if !ok || original.CurrentChapter != "第2章 重逢" || app.bookmarks.Books[app.currentFile][0].Chapter != "第2章 重逢" {
		t.Fatal("purified display title overwrote chapter matching metadata")
	}
	newBook := booksource.Book{Name: "原书", SourceURL: "new", URL: "new/book"}
	newChapters := []booksource.Chapter{{Name: "前言", URL: "new/0"}, {Name: "第1章 开始", URL: "new/1"}, {Name: "第2章 重逢", URL: "new/2"}}
	index, matched := booksource.MatchAlternativeChapter(newChapters, original.CurrentChapter, original.ChapterIndex)
	if index != 2 || !matched {
		t.Fatalf("original title did not match reordered chapter: %d %v", index, matched)
	}
	plan := &sourceSwitchPlan{book: newBook, chapters: newChapters, index: index, matched: matched}
	newReader := reader.NewOnlineReader(newBook, newChapters, index, "新正文")
	newReader.SetDisplayTitles([]string{"序", "一", "二"})
	shelf, _, marks := switchedLibrary(original, plan, newReader)
	if shelf.Books[0].CurrentChapter != "第2章 重逢" || marks.Books[onlineBookID(newBook)][0].ChapterURL != "new/2" {
		t.Fatal("source switch did not retain original title and matched bookmark")
	}
}
