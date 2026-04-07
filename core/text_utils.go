package core

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lvshp/ReadCLI/lib"
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

func availableThemes() map[string]theme {
	return map[string]theme{
		"vscode": {
			Name:       "vscode",
			HeaderName: "VS Code",
			RepoName:   "ops-console",
			Branch:     "main",
			Tabs:       []string{"explorer", "issues", "terminal", "notes", "deploy", "review"},
			HeaderTint: tcell.ColorTeal,
			Accent:     tcell.ColorYellow,
			SideAccent: tcell.ColorGreen,
			FooterTag:  "NORMAL",
			HomeName:   "bookshelf",
			LeftName:   "explorer",
			RightName:  "inspector",
		},
		"jetbrains": {
			Name:       "jetbrains",
			HeaderName: "GoLand",
			RepoName:   "workspace",
			Branch:     "sprint/readflow",
			Tabs:       []string{"project", "structure", "problems", "terminal", "services"},
			HeaderTint: tcell.ColorYellow,
			Accent:     tcell.ColorTeal,
			SideAccent: tcell.ColorPurple,
			FooterTag:  "SMART",
			HomeName:   "bookshelf",
			LeftName:   "explorer",
			RightName:  "inspector",
		},
		"ops-console": {
			Name:       "ops-console",
			HeaderName: "Control Center",
			RepoName:   "internal-dashboard",
			Branch:     "ops/quiet-shift",
			Tabs:       []string{"queue", "alerts", "output", "jobs", "audit"},
			HeaderTint: tcell.ColorGreen,
			Accent:     tcell.ColorRed,
			SideAccent: tcell.ColorTeal,
			FooterTag:  "WATCH",
			HomeName:   "bookshelf",
			LeftName:   "explorer",
			RightName:  "inspector",
		},
	}
}

func currentTheme() theme {
	themes := availableThemes()
	if t, ok := themes[app.config.Theme]; ok {
		return t
	}
	return themes["vscode"]
}

func readingWidthRatio() float64 {
	if app == nil || app.config == nil || app.config.ReadingContentWidthRatio <= 0 || app.config.ReadingContentWidthRatio > 1 {
		return 0.75
	}
	return app.config.ReadingContentWidthRatio
}

func readingMarginLeft() int {
	if app == nil || app.config == nil || app.config.ReadingMarginLeft < 0 {
		return 2
	}
	return app.config.ReadingMarginLeft
}

func readingMarginRight() int {
	if app == nil || app.config == nil || app.config.ReadingMarginRight < 0 {
		return 0
	}
	return app.config.ReadingMarginRight
}

func readingMarginTop() int {
	if app == nil || app.config == nil || app.config.ReadingMarginTop < 0 {
		return 1
	}
	return app.config.ReadingMarginTop
}

func readingMarginBottom() int {
	if app == nil || app.config == nil || app.config.ReadingMarginBottom < 0 {
		return 0
	}
	return app.config.ReadingMarginBottom
}

func readingLineSpacing() int {
	if app == nil || app.config == nil || app.config.ReadingLineSpacing < 0 {
		return 1
	}
	return app.config.ReadingLineSpacing
}

func shorten(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max || max <= 1 {
		return value
	}
	return string(runes[:max-1]) + "…"
}

func shortenDisplay(text string, width int) string {
	text = strings.TrimSpace(text)
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(text) <= width {
		return text
	}
	if width <= 1 {
		return runewidth.Truncate(text, width, "")
	}
	return runewidth.Truncate(text, width, "…")
}

func padDisplay(text string, width int) string {
	current := runewidth.StringWidth(text)
	if current >= width {
		return text
	}
	return text + strings.Repeat(" ", width-current)
}

func currentDisplayName() string {
	if app.currentFile != "" {
		return filepath.Base(app.currentFile)
	}
	if book := selectedBook(); book != nil {
		return filepath.Base(book.Path)
	}
	return "bookshelf"
}

func readableSort(value string) string {
	switch value {
	case "title":
		return "书名"
	case "imported":
		return "导入时间"
	default:
		return "最近阅读"
	}
}

func readableFilter(value string) string {
	switch value {
	case "epub":
		return "EPUB"
	case "txt":
		return "TXT"
	case "unread":
		return "未读"
	case "reading":
		return "在读"
	case "finished":
		return "已读"
	default:
		return "全部"
	}
}

func formatStamp(value string) string {
	if value == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return t.Format("01-02 15:04")
}

func titleCase(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parts := strings.Fields(value)
	for i, part := range parts {
		runes := []rune(part)
		if len(runes) == 0 {
			continue
		}
		if runes[0] >= 'a' && runes[0] <= 'z' {
			runes[0] = runes[0] - 'a' + 'A'
		}
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func progressPercent(pos, total int) int {
	if total <= 1 {
		return 0
	}
	if pos >= total-1 {
		return 100
	}
	if pos <= 0 {
		return 0
	}
	return int(float64(pos) * 100 / float64(total-1))
}

func bookTitleForPath(path, preferred string) string {
	preferred = sanitizeBookTitle(preferred)
	if preferred != "" {
		return preferred
	}
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func sanitizeBookTitle(title string) string {
	title = strings.TrimSpace(title)
	title = strings.Join(strings.Fields(title), " ")
	title = stripKnownTitleNoise(title)
	title = strings.TrimSpace(title)
	title = strings.Trim(title, "[]【】")
	title = stripKnownTitleNoise(title)
	if title == "" {
		return ""
	}
	return title
}

func stripKnownTitleNoise(title string) string {
	cleaned := strings.TrimSpace(title)
	for _, pattern := range titleNoiseSuffixPatterns {
		cleaned = pattern.ReplaceAllString(cleaned, "")
		cleaned = strings.TrimSpace(cleaned)
	}
	return cleaned
}

func visibleBooks() []lib.BookshelfBook {
	books := lib.FilterBooks(app.bookshelf.Books, app.filterMode)
	lib.SortBooks(books, app.sortMode)
	return books
}

func selectedBook() *lib.BookshelfBook {
	books := visibleBooks()
	if len(books) == 0 {
		return nil
	}
	if app.shelfIndex < 0 {
		app.shelfIndex = 0
	}
	if app.shelfIndex >= len(books) {
		app.shelfIndex = len(books) - 1
	}
	book := books[app.shelfIndex]
	return &book
}

func bookmarksForCurrentBook() []lib.Bookmark {
	if app.currentFile == "" {
		return nil
	}
	return app.bookmarks.Books[app.currentFile]
}

func currentReadingTextColor() tcell.Color {
	if app == nil {
		return tcell.ColorWhite
	}
	if app.config != nil {
		if color, ok := parseConfiguredUIColor(app.config.ReadingTextColor); ok {
			return color
		}
		if app.config.ReadingHighContrast {
			return tcell.ColorWhite
		}
	}
	return tcell.ColorWhite
}

func parseConfiguredUIColor(value string) (tcell.Color, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return tcell.ColorWhite, false
	}
	if strings.HasPrefix(value, "#") {
		r, g, b, ok := parseHexColor(value)
		if !ok {
			return tcell.ColorWhite, false
		}
		return tcell.NewRGBColor(int32(r), int32(g), int32(b)), true
	}
	parts := strings.Split(value, ",")
	if len(parts) != 3 {
		return tcell.ColorWhite, false
	}
	values := make([]int, 0, 3)
	for _, part := range parts {
		num, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || num < 0 || num > 255 {
			return tcell.ColorWhite, false
		}
		values = append(values, num)
	}
	return tcell.NewRGBColor(int32(values[0]), int32(values[1]), int32(values[2])), true
}

func parseHexColor(value string) (int, int, int, bool) {
	hex := strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(hex) == 3 {
		hex = strings.Repeat(string(hex[0]), 2) + strings.Repeat(string(hex[1]), 2) + strings.Repeat(string(hex[2]), 2)
	}
	if len(hex) != 6 {
		return 0, 0, 0, false
	}
	num, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int((num >> 16) & 0xFF), int((num >> 8) & 0xFF), int(num & 0xFF), true
}

func formatAutoPageInterval() string {
	if app == nil || app.config == nil {
		return "3.5 秒"
	}
	return fmt.Sprintf("%.1f 秒", float64(app.config.AutoPageIntervalMs)/1000)
}

func onOffText(value bool) string {
	if value {
		return "开"
	}
	return "关"
}

func renderInputWithCursor(value string, cursor int) string {
	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	left := string(runes[:cursor])
	right := string(runes[cursor:])
	return left + "[|](fg:black,bg:cyan,mod:bold)" + right
}

func emptyFallback(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func wrapDisplayLines(text string, width int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{"-"}
	}
	if width < 6 {
		width = 6
	}
	parts := strings.Split(text, "\n")
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		for runewidth.StringWidth(part) > width {
			segment := runewidth.Truncate(part, width, "")
			lines = append(lines, segment)
			part = strings.TrimSpace(strings.TrimPrefix(part, segment))
			if part == "" {
				break
			}
		}
		if part != "" {
			lines = append(lines, part)
		}
	}
	if len(lines) == 0 {
		return []string{"-"}
	}
	return lines
}

func formatProgressSummary(current, total int, raw string) string {
	if total <= 0 {
		return raw
	}
	percent := 0
	if total > 1 && current > 0 {
		percent = int(float64(current-1) * 100 / float64(total-1))
		if current >= total {
			percent = 100
		}
	}
	return fmt.Sprintf("%d / %d\n%d%%", current, total, percent)
}

func buildDetailBlock(label, value string, width int) []string {
	lines := []string{"  " + label}
	for _, line := range wrapDisplayLines(value, width) {
		lines = append(lines, "    "+line)
	}
	return lines
}

func stripTermUIStyle(value string) string {
	var b strings.Builder
	runes := []rune(value)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '[' {
			j := i + 1
			for j < len(runes) && runes[j] != ']' {
				j++
			}
			if j < len(runes)-1 && runes[j] == ']' && runes[j+1] == '(' {
				b.WriteString(string(runes[i+1 : j]))
				k := j + 2
				for k < len(runes) && runes[k] != ')' {
					k++
				}
				i = k
				continue
			}
		}
		b.WriteRune(runes[i])
	}
	return b.String()
}

func styledDisplayWidth(value string) int {
	plain := stripTermUIStyle(value)
	width := 0
	for _, r := range plain {
		if r > 127 {
			width += 2
			continue
		}
		width++
	}
	return width
}

func padDisplayText(value string, width int) string {
	displayWidth := styledDisplayWidth(value)
	if displayWidth >= width {
		return value
	}
	return value + strings.Repeat(" ", width-displayWidth)
}

func joinColumns(left, right string, leftWidth int) string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")
	rows := max(len(leftLines), len(rightLines))
	lines := make([]string, 0, rows)
	for i := 0; i < rows; i++ {
		l := ""
		r := ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		lines = append(lines, padDisplayText(l, leftWidth)+"  "+r)
	}
	return strings.Join(lines, "\n")
}
