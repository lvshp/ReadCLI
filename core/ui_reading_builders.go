package core

import (
	"fmt"
	"strings"
)

func buildBookmarksPanel() string {
	bookmarks := bookmarksForCurrentBook()
	if len(bookmarks) == 0 {
		return "当前书没有书签。\n\n按 s 保存一个书签。"
	}
	if app.bookmarkIndex < 0 {
		app.bookmarkIndex = 0
	}
	if app.bookmarkIndex >= len(bookmarks) {
		app.bookmarkIndex = len(bookmarks) - 1
	}
	var lines []string
	lines = append(lines, "书签列表", "")
	for i, mark := range bookmarks {
		prefix := "  "
		if i == app.bookmarkIndex {
			prefix = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%s | %s", prefix, shorten(mark.Chapter, 16), shorten(mark.Snippet, 36)))
		if i < len(bookmarks)-1 {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}

func buildHelpPanel() string {
	if mainContentWidth < 72 {
		return menuText
	}

	leftTitle := "[Vim 风格](fg:cyan,mod:bold)"
	rightTitle := "[方向键 / 普通键](fg:yellow,mod:bold)"

	leftSections := []string{
		"[书架首页](fg:green,mod:bold)\n  j/k 移动  i 导入  o 排序\n  r 过滤  x 移除  ? 帮助  q 退出",
		"[阅读界面](fg:green,mod:bold)\n  j/k 翻页  [/] 切章  / 搜索\n  n/N 搜索跳转  s 书签  B 书签列表\n  m 目录  p 进度  , 阅读设置\n  c 字体颜色  t 自动翻页\n  b Boss Key  z 精简/全信息\n  T 主题  +/- 行数",
		"[目录](fg:green,mod:bold)\n  j/k 移动  Enter 打开  m 返回\n  0-9 页码输入",
		"[书签列表](fg:green,mod:bold)\n  j/k 移动  d 删除  Enter 打开\n  B/q 返回",
		"[通用](fg:green,mod:bold)\n  f 切换边框  T 切换主题\n  z 精简/全信息  u 检查更新\n  q 返回/退出",
	}

	rightSections := []string{
		"[书架首页](fg:magenta,mod:bold)\n  ↑/↓ 选择  →/Enter 打开",
		"[阅读界面](fg:magenta,mod:bold)\n  ↑/↓ 翻页  ←/→ 切章\n  Space/Enter 向下翻页\n  0-9 行号跳转输入",
		"[阅读设置](fg:magenta,mod:bold)\n  ↑/↓ 选择  ←/→ 调整\n  Enter 激活  Esc 返回",
		"[目录](fg:magenta,mod:bold)\n  ↑/↓ 选择  →/Enter 打开  ← 返回",
		"[书签列表](fg:magenta,mod:bold)\n  ↑/↓ 选择  →/Enter 打开  ← 返回",
		"[导入 / 搜索输入](fg:magenta,mod:bold)\n  ←/→ 光标  ↑/↓ 候选  Tab 补全\n  Ctrl-r 递归扫描  Enter 确认\n  Home/End 首尾  Esc 取消",
		"[删除确认](fg:magenta,mod:bold)\n  y 移除  D 删文件并移除  Esc 取消",
	}

	left := leftTitle + "\n\n" + strings.Join(leftSections, "\n\n")
	right := rightTitle + "\n\n" + strings.Join(rightSections, "\n\n")
	return joinColumns(left, right, 38)
}

type readingSettingItem struct {
	Label string
	Value string
}

func buildReadingSettingsPanel() string {
	items := readingSettingsItems()
	if len(items) == 0 {
		return "阅读设置不可用"
	}
	if app.settingsIndex < 0 {
		app.settingsIndex = 0
	}
	if app.settingsIndex >= len(items) {
		app.settingsIndex = len(items) - 1
	}
	lines := []string{
		"阅读设置",
		"",
		"这些设置全局生效，三个主题共用。",
		"",
	}
	for i, item := range items {
		prefix := "  "
		if i == app.settingsIndex {
			prefix = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%-10s %s", prefix, item.Label, item.Value))
	}
	lines = append(lines, "", "提示：字体颜色支持 #RRGGBB、#RGB、R,G,B。")
	return strings.Join(lines, "\n")
}

func readingSettingsItems() []readingSettingItem {
	colorValue := "#FFFFFF"
	if app != nil && app.config != nil && strings.TrimSpace(app.config.ReadingTextColor) != "" {
		colorValue = app.config.ReadingTextColor
	}
	return []readingSettingItem{
		{Label: "正文宽度", Value: fmt.Sprintf("%.0f%%", readingWidthRatio()*100)},
		{Label: "左边距", Value: fmt.Sprintf("%d", readingMarginLeft())},
		{Label: "右边距", Value: fmt.Sprintf("%d", readingMarginRight())},
		{Label: "上边距", Value: fmt.Sprintf("%d", readingMarginTop())},
		{Label: "下边距", Value: fmt.Sprintf("%d", readingMarginBottom())},
		{Label: "行间距", Value: fmt.Sprintf("%d", readingLineSpacing())},
		{Label: "翻页间隔", Value: formatAutoPageInterval()},
		{Label: "字体颜色", Value: colorValue},
		{Label: "高对比", Value: onOffText(app.config != nil && app.config.ReadingHighContrast)},
		{Label: "基础色模式", Value: onOffText(app.config != nil && app.config.ForceBasicColor)},
	}
}

func tocStatusText() string {
	if app.reader == nil {
		return ""
	}
	text := app.reader.GetTOCWithSelection(app.tocIndex, tocPageSize())
	if app.tocNumber == "" {
		return text
	}
	return text + "\nOpen chapter: " + app.tocNumber
}

func tocPageSize() int {
	if mainContentHeight > 0 {
		reservedLines := 4
		if app.tocNumber != "" {
			reservedLines++
		}
		available := mainContentHeight - reservedLines
		if available > 0 {
			return available
		}
	}
	return 10
}

func readingContentWidth(mainWidth int) int {
	width := readingWidth(mainWidth)
	if width <= 0 {
		return 80
	}
	if app != nil && app.reader != nil {
		target := int(float64(width) * readingWidthRatio())
		if target < 28 {
			target = 28
		}
		if target > width {
			target = width
		}
		target -= readingMarginLeft() + readingMarginRight()
		if target < 20 {
			target = 20
		}
		return target
	}
	return width
}

func readingVisibleSourceLines() int {
	if app == nil || app.displayLines < 1 {
		return 1
	}
	maxLines := readingMaxSourceLines()
	if maxLines < 1 {
		maxLines = 1
	}
	if app.displayLines > maxLines {
		return maxLines
	}
	return app.displayLines
}

func readingMaxSourceLines() int {
	if mainContentHeight == 0 {
		return max(1, app.displayLines)
	}
	available := mainContentHeight - readingMarginTop() - readingMarginBottom()
	if available < 1 {
		return 1
	}
	spacing := readingLineSpacing()
	return max(1, (available+spacing)/(spacing+1))
}

func formatReadingPanel(text string) string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	lineGap := readingLineSpacing()
	leftPad := strings.Repeat(" ", readingMarginLeft())
	padded := make([]string, 0, len(lines)*(lineGap+1)+readingMarginTop()+readingMarginBottom())
	for i := 0; i < readingMarginTop(); i++ {
		padded = append(padded, "")
	}
	for i, line := range lines {
		padded = append(padded, leftPad+line)
		if i != len(lines)-1 {
			for gap := 0; gap < lineGap; gap++ {
				padded = append(padded, "")
			}
		}
	}
	for i := 0; i < readingMarginBottom(); i++ {
		padded = append(padded, "")
	}
	return strings.Join(padded, "\n")
}

func highlightSearchMatches(text, query string) string {
	query = strings.TrimSpace(query)
	if text == "" || query == "" {
		return text
	}

	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	if lowerQuery == "" {
		return text
	}

	var b strings.Builder
	start := 0
	for {
		idx := strings.Index(lowerText[start:], lowerQuery)
		if idx < 0 {
			b.WriteString(text[start:])
			break
		}
		idx += start
		end := idx + len(lowerQuery)
		b.WriteString(text[start:idx])
		b.WriteString("[")
		b.WriteString(text[idx:end])
		b.WriteString("](fg:black,bg:yellow,mod:bold)")
		start = end
	}
	return b.String()
}

func readingWidth(termWidth int) int {
	width := termWidth
	if width <= 0 {
		width = fixedWidth
	}
	if width > 2 {
		return width - 2
	}
	return width
}
