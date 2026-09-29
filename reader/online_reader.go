package reader

import (
	"fmt"
	"strings"

	"github.com/lvshp/ReadCLI/booksource"
)

// OnlineReader holds one downloaded chapter. Networking is performed by the UI
// controller, outside the render/input loop, before installing a new chapter.
type OnlineReader struct {
	contentReader
	Book          booksource.Book
	Chapters      []booksource.Chapter
	Index         int
	displayTitles []string
}

func NewOnlineReader(book booksource.Book, chapters []booksource.Chapter, index int, text string) *OnlineReader {
	r := &OnlineReader{Book: book, Chapters: append([]booksource.Chapter(nil), chapters...), Index: index}
	if r.Index < 0 || r.Index >= len(chapters) {
		r.Index = 0
	}
	r.setContent(text)
	return r
}
func (r *OnlineReader) Load(string) error        { return fmt.Errorf("在线章节需要通过书源加载") }
func (r *OnlineReader) BookTitle() string        { return r.Book.Name }
func (r *OnlineReader) CurrentChapterIndex() int { return r.Index }
func (r *OnlineReader) CurrentChapterTitle() string {
	return r.ChapterTitle(r.Index)
}

// Display titles can be purified without altering the source metadata used for
// requests, matching, and the original chapter cache.
func (r *OnlineReader) SetDisplayTitles(titles []string) {
	r.displayTitles = append([]string(nil), titles...)
}
func (r *OnlineReader) ChapterTitle(index int) string {
	if index < 0 || index >= len(r.Chapters) {
		return ""
	}
	if index < len(r.displayTitles) && r.displayTitles[index] != "" {
		return r.displayTitles[index]
	}
	return r.Chapters[index].Name
}
func (r *OnlineReader) GetProgress() string {
	return fmt.Sprintf("第 %d/%d 章 · 本章 %d/%d 行", r.Index+1, len(r.Chapters), r.CurrentPos()+1, r.Total())
}
func (r *OnlineReader) GetTOC() string {
	lines := []string{"目录"}
	for i := range r.Chapters {
		lines = append(lines, r.ChapterTitle(i))
	}
	return strings.Join(lines, "\n")
}
func (r *OnlineReader) GetTOCWithSelection(selected, pageSize int) string {
	if len(r.Chapters) == 0 {
		return "暂无目录"
	}
	selected = max(0, min(selected, len(r.Chapters)-1))
	pageSize = max(1, pageSize)
	start := selected / pageSize * pageSize
	lines := []string{"在线目录", fmt.Sprintf("第 %d/%d 页 · 共 %d 章", start/pageSize+1, (len(r.Chapters)+pageSize-1)/pageSize, len(r.Chapters)), ""}
	for i := start; i < min(start+pageSize, len(r.Chapters)); i++ {
		prefix := "  "
		if i == selected {
			prefix = "> "
		} else if i == r.Index {
			prefix = "* "
		}
		lines = append(lines, fmt.Sprintf("%s%d. %s", prefix, i+1, r.ChapterTitle(i)))
	}
	return strings.Join(lines, "\n")
}

// Progress is based on chapter index plus progress within the current chapter.
func (r *OnlineReader) OverallProgress() float64 {
	if len(r.Chapters) == 0 {
		return 0
	}
	return (float64(r.Index) + r.progressRatio()) / float64(len(r.Chapters))
}
func (r *OnlineReader) ChapterOffset() float64       { return r.progressRatio() }
func (r *OnlineReader) RestoreOffset(offset float64) { r.restoreProgress(offset) }
