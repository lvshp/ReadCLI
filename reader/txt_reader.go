package reader

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

type TxtReader struct {
	contentReader
	chapters []txtChapter
}

type txtChapter struct {
	Title string
	Start int
}

func NewTxtReader() *TxtReader {
	return &TxtReader{}
}

func (txt *TxtReader) Load(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	txt.setContent(normalizeTXTContent(string(data)))
	txt.buildChapters()
	return nil
}

func (txt *TxtReader) BookTitle() string {
	return ""
}

func (txt *TxtReader) Reflow(width int) {
	txt.contentReader.Reflow(width)
	txt.buildChapters()
}

func (txt *TxtReader) CurrentChapterTitle() string {
	if len(txt.chapters) == 0 {
		return ""
	}
	return txt.chapters[txt.currentChapterIndex()].Title
}

func (txt *TxtReader) CurrentChapterIndex() int {
	return txt.currentChapterIndex()
}

func (txt *TxtReader) NextChapter() string {
	if len(txt.chapters) == 0 {
		return txt.Current()
	}
	index := txt.currentChapterIndex()
	if index < len(txt.chapters)-1 {
		txt.pos = txt.chapters[index+1].Start
	}
	return txt.Current()
}

func (txt *TxtReader) PrevChapter() string {
	if len(txt.chapters) == 0 {
		return txt.Current()
	}
	index := txt.currentChapterIndex()
	if index > 0 {
		txt.pos = txt.chapters[index-1].Start
	} else {
		txt.pos = txt.chapters[0].Start
	}
	return txt.Current()
}

func (txt *TxtReader) GetTOC() string {
	if len(txt.chapters) == 0 {
		return "No table of contents available."
	}
	var lines []string
	lines = append(lines, "Table of Contents")
	for i, chapter := range txt.chapters {
		lines = append(lines, formatChapterLine(i+1, chapter.Title))
	}
	return strings.Join(lines, "\n")
}

func (txt *TxtReader) GetTOCWithSelection(selected, pageSize int) string {
	if len(txt.chapters) == 0 {
		return txt.GetTOC()
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= len(txt.chapters) {
		selected = len(txt.chapters) - 1
	}
	if pageSize <= 0 {
		pageSize = len(txt.chapters)
	}

	current := txt.CurrentChapterIndex()
	start := (selected / pageSize) * pageSize
	end := start + pageSize
	if end > len(txt.chapters) {
		end = len(txt.chapters)
	}

	var lines []string
	lines = append(lines, "Table of Contents")
	lines = append(lines, "j/k to move, number + Enter to open, m to close")
	lines = append(lines, pageIndicator(start, pageSize, len(txt.chapters)))
	for i := start; i < end; i++ {
		prefix := "  "
		switch {
		case i == selected && i == current:
			prefix = "*>"
		case i == selected:
			prefix = "> "
		case i == current:
			prefix = "* "
		}
		lines = append(lines, prefix+" "+formatChapterLine(i+1, txt.chapters[i].Title))
	}
	return strings.Join(lines, "\n")
}

func (txt *TxtReader) GotoChapter(index int) string {
	if len(txt.chapters) == 0 {
		return txt.Current()
	}
	if index < 0 {
		index = 0
	}
	if index >= len(txt.chapters) {
		index = len(txt.chapters) - 1
	}
	txt.pos = txt.chapters[index].Start
	return txt.Current()
}

func (txt *TxtReader) buildChapters() {
	txt.chapters = nil
	for i, line := range txt.content {
		if title, ok := inferTXTChapterTitle(line); ok {
			txt.chapters = append(txt.chapters, txtChapter{Title: title, Start: i})
		}
	}
}

func (txt *TxtReader) currentChapterIndex() int {
	if len(txt.chapters) == 0 {
		return 0
	}
	for i := len(txt.chapters) - 1; i >= 0; i-- {
		if txt.pos >= txt.chapters[i].Start {
			return i
		}
	}
	return 0
}

var txtChapterPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^第[0-9零一二三四五六七八九十百千万两]+[章节卷集回部篇][^\n]{0,40}$`),
	regexp.MustCompile(`(?i)^(正文\s*)?第[0-9零一二三四五六七八九十百千万两]+[章节卷集回部篇][^\n]{0,40}$`),
	regexp.MustCompile(`(?i)^chapter\s+[0-9ivxlcdm]+[^\n]{0,40}$`),
	regexp.MustCompile(`(?i)^prologue[^\n]{0,40}$`),
	regexp.MustCompile(`(?i)^epilogue[^\n]{0,40}$`),
}

var looseTXTChapterPattern = regexp.MustCompile(`(?i)^第\s*[0-9零一二三四五六七八九十百千万两]+\s*[章节卷集回部篇].*`)

func inferTXTChapterTitle(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", false
	}
	if len([]rune(line)) > 50 {
		return "", false
	}
	line = strings.Trim(line, "[]【】<>《》")
	for _, pattern := range txtChapterPatterns {
		if pattern.MatchString(line) {
			return line, true
		}
	}
	return "", false
}

func normalizeTXTContent(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		title, ok := inferTXTChapterTitle(line)
		if !ok {
			out = append(out, line)
			continue
		}

		out = append(out, line)
		j := i + 1
		for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
			j++
		}
		if j < len(lines) {
			nextTitle, nextOK := inferTXTChapterTitle(lines[j])
			if nextOK && normalizeChapterTitle(title) == normalizeChapterTitle(nextTitle) {
				i = j
			}
		}
	}
	return strings.Join(out, "\n")
}

func normalizeChapterTitle(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Join(strings.Fields(value), " ")
	return value
}

func formatChapterLine(index int, title string) string {
	return strconv.Itoa(index) + ". " + title
}

func pageIndicator(start, pageSize, total int) string {
	return "Page " + strconv.Itoa(start/pageSize+1) + "/" + strconv.Itoa((total+pageSize-1)/pageSize)
}

func inferTXTBookTitle(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r", "\n"), "\n")
	seenChapter := false
	for i, line := range lines {
		if i >= 20 {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if title := stripTXTBookTitleLabel(line); title != "" {
			return title
		}
		if _, ok := inferTXTChapterTitle(line); ok {
			seenChapter = true
			continue
		}
		if seenChapter {
			return ""
		}
		if looksLikeStandaloneTXTBookTitle(line) {
			return strings.Trim(line, "《》[]【】")
		}
	}
	return ""
}

func stripTXTBookTitleLabel(line string) string {
	line = strings.TrimSpace(line)
	for _, prefix := range []string{"书名:", "书名：", "title:", "title：", "Title:", "Title："} {
		if strings.HasPrefix(line, prefix) {
			title := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			title = strings.Trim(title, "《》[]【】")
			if looksLikeTXTBookTitle(title) {
				return title
			}
		}
	}
	return ""
}

func looksLikeTXTBookTitle(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	if len([]rune(line)) > 40 {
		return false
	}
	lower := strings.ToLower(line)
	for _, prefix := range []string{"作者", "author", "简介", "文案", "版权", "book", "txt"} {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}
	if strings.Contains(line, "http://") || strings.Contains(line, "https://") {
		return false
	}
	return true
}

func looksLikeStandaloneTXTBookTitle(line string) bool {
	line = strings.TrimSpace(line)
	if !looksLikeTXTBookTitle(line) {
		return false
	}
	if len([]rune(line)) > 30 {
		return false
	}
	trimmed := strings.Trim(line, "《》[]【】\"'")
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "www.") || strings.Contains(lower, ".com") || strings.Contains(lower, ".cn") {
		return false
	}
	if looseTXTChapterHeading(trimmed) {
		return false
	}
	if strings.ContainsAny(trimmed, "，。！？；：、,.!?;:=<>/\\|") {
		return false
	}
	if strings.Contains(trimmed, "作者") || strings.Contains(trimmed, "正文") || strings.Contains(trimmed, "校对") {
		return false
	}
	if strings.Contains(trimmed, "  ") {
		return false
	}
	return true
}

func looseTXTChapterHeading(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	line = strings.Trim(line, "《》[]【】\"'")
	return looseTXTChapterPattern.MatchString(line)
}
