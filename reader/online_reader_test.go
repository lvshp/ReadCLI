package reader

import (
	"strings"
	"testing"

	"github.com/lvshp/ReadCLI/booksource"
)

func TestOnlineReaderReflowAndAnchor(t *testing.T) {
	book := booksource.Book{Name: "测试在线书"}
	chapters := []booksource.Chapter{{Name: "一", URL: "/1"}, {Name: "二", URL: "/2"}, {Name: "三", URL: "/3"}}
	r := NewOnlineReader(book, chapters, 1, strings.Repeat("这是一个用于验证分页之后阅读进度仍然稳定的段落。\n", 30))
	r.Reflow(28)
	r.Goto(r.Total() / 2)
	before := AnchorFromReader(r)
	r.Reflow(52)
	RestoreFromAnchor(r, before)
	after := AnchorFromReader(r)
	if before.ChapterIndex != 1 || after.ChapterIndex != 1 {
		t.Fatal("chapter lost after reflow")
	}
	if d := after.ChapterOffset - before.ChapterOffset; d < -0.04 || d > 0.04 {
		t.Fatalf("offset drift: %v -> %v", before, after)
	}
	if r.OverallProgress() < 0.4 || r.OverallProgress() > 0.6 {
		t.Fatalf("overall=%v", r.OverallProgress())
	}
	if !strings.Contains(r.GetTOCWithSelection(2, 2), "> 3. 三") {
		t.Fatal(r.GetTOCWithSelection(2, 2))
	}
	if r.BookTitle() != "测试在线书" {
		t.Fatal(r.BookTitle())
	}
}
