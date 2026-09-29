package core

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/lvshp/ReadCLI/reader"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

func buildHeader(th theme) string {
	now := time.Now().Format("15:04")
	modeLabel := strings.ToUpper(string(app.mode))
	if isPurificationMode(app.mode) {
		modeLabel = "净化配置"
	}
	if isOnlineMode(app.mode) {
		modeLabel = "在线搜书"
		if app.mode == modeSources || app.mode == modeSourceImport || app.mode == modeSourceDelete {
			modeLabel = "书源管理"
		}
		if app.mode == modeSourceLogin {
			modeLabel = "书源登录"
		}
		if app.mode == modeSourceSwitch || app.mode == modeSourceSwitchConfirm {
			modeLabel = "更换书源"
		}
	}
	switch th.Name {
	case "jetbrains":
		line1 := fmt.Sprintf("[%s](fg:yellow,mod:bold)  [%s](fg:white)  run [%s](fg:cyan)  branch [%s](fg:magenta)  [%s](fg:yellow)",
			th.HeaderName,
			th.RepoName,
			shorten(currentDisplayName(), 24),
			th.Branch,
			now,
		)
		line2 := fmt.Sprintf("[%s](fg:black,bg:yellow,mod:bold)  [ project ] [ structure ] [ services ] [ problems ]  inspections [0](fg:green)  theme [%s](fg:cyan)",
			modeLabel,
			th.Name,
		)
		return line1 + "\n" + line2
	case "ops-console":
		line1 := fmt.Sprintf("[%s](fg:green,mod:bold)  cluster [%s](fg:cyan)  lane [%s](fg:yellow)  target [%s](fg:white,mod:bold)  [%s](fg:green)",
			th.HeaderName,
			th.RepoName,
			th.Branch,
			shorten(currentDisplayName(), 22),
			now,
		)
		line2 := fmt.Sprintf("[%s](fg:black,bg:green,mod:bold)  [ queue ] [ alerts ] [ jobs ] [ audit ]  incidents [0](fg:green)  theme [%s](fg:cyan)",
			modeLabel,
			th.Name,
		)
		return line1 + "\n" + line2
	default:
		line1 := fmt.Sprintf("[%s](fg:cyan,mod:bold)  [%s](fg:green)  branch [%s](fg:yellow)  [%s](fg:white,mod:bold)  [%s](fg:cyan)",
			th.HeaderName,
			th.RepoName,
			th.Branch,
			shorten(currentDisplayName(), 28),
			now,
		)
		line2 := fmt.Sprintf("[%s](fg:black,bg:green,mod:bold)  [ 书架 ] [ S 书源 ] [ s 搜书 ] [ reader ]  diagnostics [0](fg:green)  theme [%s](fg:cyan)",
			modeLabel,
			th.Name,
		)
		return line1 + "\n" + line2
	}
}

func buildLeftPanel(th theme) string {
	if isPurificationMode(app.mode) {
		return buildPurificationLeftPanel()
	}
	if isOnlineMode(app.mode) {
		return buildOnlineLeftPanel()
	}
	if app.mode == modeHome || app.mode == modeImportInput || app.mode == modeDeleteConfirm || app.mode == modeBookshelfSearchInput {
		return strings.Join([]string{
			"[Bookshelf](fg:cyan,mod:bold)",
			"",
			"[Actions](fg:yellow,mod:bold)",
			"  Enter   打开书籍",
			"  i       导入文件",
			"  S       书源管理",
			"  s       在线搜书",
			"  C       更换书源",
			"  P       净化配置",
			"  /       搜索书架",
			"  o       排序视图",
			"  r       过滤视图",
			"  x       移出书架",
			"  T       切换主题",
			"  u       检查更新",
			"",
			"[Sort](fg:yellow,mod:bold)",
			"  " + readableSort(app.bookshelfState.sortMode),
			"",
			"[Filter](fg:green,mod:bold)",
			"  " + readableFilter(app.bookshelfState.filterMode),
			"",
			"[Theme](fg:cyan,mod:bold)",
			"  " + th.Name,
		}, "\n")
	}

	currentChapter := ""
	if app.reader != nil {
		currentChapter = app.reader.CurrentChapterTitle()
	}
	if currentChapter == "" {
		currentChapter = "Inbox"
	}
	return strings.Join([]string{
		"[Explorer](fg:cyan,mod:bold)",
		"",
		"  bookshelf/",
		"    core/",
		"    reader/",
		"    themes/",
		fmt.Sprintf("    > %s", shorten(currentDisplayName(), 14)),
		"",
		"[Actions](fg:yellow,mod:bold)",
		"  / 搜索",
		"  s 保存书签",
		"  B 打开书签",
		"  m 目录",
		"  c 切换颜色",
		"  C 更换书源",
		"  P 净化配置",
		"  , 阅读设置",
		"  T 切主题",
		"  u 检查更新",
		"",
		"[Current Focus](fg:green,mod:bold)",
		"  " + shorten(currentChapter, 14),
	}, "\n")
}

func buildRightPanel(th theme) string {
	if isPurificationMode(app.mode) {
		return buildPurificationRightPanel()
	}
	if isOnlineMode(app.mode) {
		return buildOnlineRightPanel()
	}
	if app.mode == modeHome || app.mode == modeImportInput || app.mode == modeDeleteConfirm || app.mode == modeBookshelfSearchInput {
		book := selectedBook()
		lines := []string{fmt.Sprintf("[%s](fg:cyan,mod:bold)", titleCase(th.RightName)), ""}
		if book == nil {
			lines = append(lines, "  书架为空", "  按 i 导入本地书籍")
		} else {
			lastRead := "未开始"
			if book.LastReadAt != "" {
				lastRead = formatStamp(book.LastReadAt)
			}
			status := "未读"
			if book.ProgressPercent >= 100 {
				status = "已读"
			} else if book.ProgressPos > 0 || (book.Online != nil && book.ChapterIndex > 0) {
				status = "在读"
			}
			continueHint := "  回车从头开始"
			if book.ProgressPercent >= 100 {
				continueHint = "  回车重新打开已读书籍"
			} else if book.ProgressPos > 0 || (book.Online != nil && book.ChapterIndex > 0) {
				continueHint = "  回车继续上次阅读位置"
			}
			lines = append(lines,
				"  标题    "+shorten(remoteLabel(book.Title), 16),
				"  格式    "+strings.ToUpper(book.Format),
			)
			if book.Online != nil {
				lines = append(lines,
					"  书源："+remoteLabel(emptyFallback(book.Online.SourceName, "未命名书源")),
					"  作者："+remoteLabel(emptyFallback(book.Online.Author, "未知作者")),
					"  更新："+remoteLabel(emptyFallback(book.Online.LastChapter, emptyFallback(book.Online.Kind, "暂无章节信息"))),
				)
			}
			lines = append(lines,
				"  状态    "+status,
				fmt.Sprintf("  进度    %d%%", book.ProgressPercent),
				"  章节    "+shorten(remoteLabel(book.CurrentChapter), 16),
				"  最近    "+shorten(lastRead, 16),
			)
			lines = append(lines, "", "[Continue](fg:yellow,mod:bold)", continueHint)
			if book.Online != nil {
				lines = append(lines, "  C 更换书源并迁移进度")
			}
		}
		return strings.Join(lines, "\n")
	}

	progress := ""
	chapter := ""
	total := 0
	current := 0
	if app.reader != nil {
		progress = app.reader.GetProgress()
		chapter = app.reader.CurrentChapterTitle()
		total = app.reader.Total()
		current = app.reader.CurrentPos() + 1
	}
	if chapter == "" {
		chapter = "General"
	}
	width := 16
	if mainContentWidth > 6 {
		width = mainContentWidth - 6
	}
	lines := []string{"[Inspector](fg:cyan,mod:bold)", ""}
	lines = append(lines, buildDetailBlock("章节", chapter, width)...)
	lines = append(lines, "")
	lines = append(lines, buildDetailBlock("进度", formatProgressSummary(current, total, progress), width)...)
	lines = append(lines, "")
	lines = append(lines, buildDetailBlock("总行数", fmt.Sprintf("%d lines", total), width)...)
	lines = append(lines, "", "[Search](fg:yellow,mod:bold)", "")
	lines = append(lines, buildDetailBlock("查询", emptyFallback(app.readingState.searchQuery, "无"), width)...)
	return strings.Join(lines, "\n")
}

func buildFooter() string {
	if compactReadingUI() {
		return compactReadingStatusLine()
	}
	elapsed := time.Since(app.uiState.sessionStart).Round(time.Minute)
	tag := currentTheme().FooterTag
	version := strings.TrimSpace(app.currentVersion)
	if version == "" {
		version = "dev"
	}
	line1 := fmt.Sprintf("[%s](fg:black,bg:green,mod:bold)  utf-8  session [%s](fg:yellow)  theme [%s](fg:cyan)  version [%s](fg:yellow)  [%s](fg:green)",
		tag, elapsed, app.config.Theme, version, app.uiState.statusMessage)
	if lastTermWidth > 0 && lastTermWidth < 110 {
		line1 = shortenDisplay("ReadCLI "+version+" · "+remoteLabel(app.uiState.statusMessage), max(1, lastTermWidth-2))
	}
	switch app.mode {
	case modePurification:
		return line1 + "\n↑/↓ 选择 · i 导入 · r 重载 · Esc/q 返回"
	case modePurificationImport:
		return line1 + "\n输入 JSON 文件路径或下载链接 · Enter 导入 · Esc 取消"
	case modeSourceSwitch:
		return line1 + "\n↑/↓ 选择 · Enter 预览 · n/p 上下页 · r 刷新 · Esc/q 取消换源"
	case modeSourceSwitchConfirm:
		return line1 + "\nEnter/y 确认换源 · Esc/q 返回候选 · 确认前保留原书和进度"
	case modeOnlineErrors:
		return line1 + "\nj/k 切换错误 · r 重试 · q 返回结果"
	case modeSources:
		return line1 + "\ni 导入 · P 净化 · L 登录 · X 退出登录 · 空格 启停 · / 搜当前 · s 搜全部 · d 移除 · q 书架"
	case modeSourceLogin:
		return line1 + "\nTab/↑↓ 切换字段 · Enter 登录 · Esc 取消"
	case modeSourceImport, modeOnlineSearchInput:
		return line1 + "\nEnter 确认 · Esc 取消"
	case modeSourceDelete:
		return line1 + "\ny 确认移除 · Esc 返回"
	case modeOnlineResults:
		return line1 + "\nEnter 阅读 · n/p 上下页 · / 搜索 · S 书源 · r 重试 · e 错误详情 · Esc 取消 · q 书架"
	case modeHome:
		return line1 + "\n[↑/↓](fg:cyan):选择  [→/Enter](fg:cyan):打开  [C](fg:cyan):换源  [P](fg:cyan):净化  [/](fg:cyan):搜书架  [s](fg:cyan):在线搜书  [S](fg:cyan):书源  [i](fg:cyan):导入  [o/r](fg:cyan):排序/过滤  [x](fg:cyan):移除  [T](fg:cyan):主题  [u](fg:cyan):更新  [q](fg:red):退出"
	case modeReading:
		return line1 + "\n[↑/↓](fg:cyan):翻页  [←/→](fg:cyan):切章  [C](fg:cyan):换源  [P](fg:cyan):净化  [g](fg:cyan):跳转  [?](fg:cyan):快捷键  [+/-](fg:cyan):正文行数  [a](fg:cyan):对齐  [c](fg:cyan):颜色  [,](fg:cyan):设置  [/](fg:cyan):搜索  [s/B](fg:cyan):书签  [m](fg:cyan):目录  [z](fg:cyan):精简/全信息  [q](fg:red):书架"
	case modeTOC:
		return line1 + "\n[↑/↓](fg:cyan):移动  [→/Enter](fg:cyan):打开  [←/m](fg:cyan):返回  [0-9](fg:cyan):跳章  [q](fg:red):书架"
	case modeBookmarks:
		return line1 + "\n[↑/↓](fg:cyan):移动  [→/Enter](fg:cyan):打开  [d](fg:cyan):删除  [←/B/q](fg:red):关闭"
	case modeSearchInput:
		return line1 + "\n输入搜索关键字，支持左右移动，Enter 执行，Esc 取消"
	case modeBookshelfSearchInput:
		return line1 + "\n输入书名关键字过滤书架，Enter 应用，Esc 清空并取消"
	case modeReadingJumpInput:
		return line1 + "\n输入章节号或百分比，例如 128 / 50%，Enter 跳转，Esc 取消"
	case modeImportInput:
		scope := "当前层"
		if app.uiState.input.importRecursive {
			scope = "递归"
		}
		return line1 + "\n输入文件或文件夹路径，Tab 补全，Ctrl-r 切换扫描范围(" + scope + ")，Esc 取消"
	case modeReadingSettings:
		return line1 + "\n[↑/↓](fg:cyan):选择  [←/→](fg:cyan):调整  [Enter](fg:cyan):切换/编辑  [Esc](fg:red):返回阅读"
	case modeReadingColorInput:
		return line1 + "\n输入字体颜色，支持 #RRGGBB / #RGB / R,G,B，Enter 保存，Esc 取消"
	case modeDeleteConfirm:
		return line1 + "\n[y](fg:cyan):仅移出书架  [D](fg:red):删除本地文件  [Esc](fg:yellow):取消"
	case modeUpdatePrompt:
		return line1 + "\n[y/Enter](fg:cyan):开始更新  [n/Esc](fg:yellow):稍后再说"
	case modeUpdating:
		return line1 + "\n正在下载安装新版本，请稍候…"
	case modeUpdateRestart:
		return line1 + "\n[Enter](fg:cyan):退出并手动重新启动  [q](fg:red):直接退出"
	default:
		return line1 + "\n[q](fg:red):退出"
	}
}

func compactReadingStatusLine() string {
	chapter := "未命名章节"
	percent := 0
	current := 0
	total := 0
	if app != nil && app.reader != nil {
		if current := strings.TrimSpace(app.reader.CurrentChapterTitle()); current != "" {
			chapter = remoteLabel(current)
		}
		total = app.reader.Total()
		current = app.reader.CurrentPos() + 1
		percent = progressPercent(app.reader.CurrentPos(), total)
		if r, ok := app.reader.(*reader.OnlineReader); ok {
			percent = int(r.OverallProgress() * 100)
		}
	}
	width := max(20, mainContentWidth)
	position := fmt.Sprintf("%d/%d", current, total)
	status := fmt.Sprintf("%d%% · %s", percent, position)
	switchHint := ""
	if _, ok := app.reader.(*reader.OnlineReader); ok {
		switchHint = " · C 换源 · P 净化"
	}
	if app.online.busy != "" {
		return remoteLabel(app.online.busy) + " · Esc 取消"
	}
	if app.uiState.statusMessageKind == statusError {
		return remoteLabel(app.uiState.statusMessage)
	}
	statusWidth := runewidth.StringWidth("  ·  " + status + switchHint)
	chapter = shortenDisplay(chapter, max(8, width-statusWidth))
	return fmt.Sprintf("[%s](fg:white,mod:dim)  [·](fg:cyan,mod:dim)  [%s](fg:yellow)  [%s](fg:white,mod:dim)%s", chapter, fmt.Sprintf("%d%%", percent), position, switchHint)
}

func buildMainTitle() string {
	if isPurificationMode(app.mode) {
		return " 净化替换配置 "
	}
	if app.mode == modeSourceSwitch || app.mode == modeSourceSwitchConfirm {
		return " 更换书源 "
	}
	if isOnlineMode(app.mode) {
		return " 书源 / 在线阅读 "
	}
	switch app.mode {
	case modeHome, modeImportInput, modeDeleteConfirm:
		return " bookshelf "
	case modeBookmarks:
		return " bookmarks "
	case modeTOC:
		return " table of contents "
	case modeReadingSettings:
		return " reading settings "
	case modeReadingColorInput:
		return " reading color "
	case modeBookshelfSearchInput:
		return " bookshelf search "
	case modeReadingJumpInput:
		return " jump "
	case modeUpdatePrompt, modeUpdating, modeUpdateRestart:
		return " update "
	default:
		return " editor: " + currentDisplayName() + " "
	}
}

func buildMainPanel() string {
	if isPurificationMode(app.mode) {
		return buildPurificationPanel()
	}
	if isOnlineMode(app.mode) {
		return buildOnlinePanel()
	}
	if app.readingState.showHelp {
		return buildHelpPanel()
	}
	if app.readingState.showProgress && app.reader != nil {
		return app.reader.GetProgress()
	}
	switch app.mode {
	case modeHome:
		return buildBookshelfPanel()
	case modeImportInput:
		scopeLabel := "当前层"
		if app.uiState.input.importRecursive {
			scopeLabel = "递归子目录"
		}
		lines := []string{
			"导入本地书籍",
			"",
			"请输入 txt / epub 文件路径，或一个文件夹路径：",
			"",
			renderInputWithCursor(app.uiState.input.value, app.uiState.input.cursor),
			"",
			"支持左右移动、删除、Tab 补全、拖入文件/目录，以及目录批量导入。",
			"当前扫描范围：" + scopeLabel,
			"按 Ctrl-r 切换当前层 / 递归子目录。",
		}
		if len(app.uiState.input.hints) > 0 {
			pageSize := importHintPageSize()
			start, end, page, totalPages := importHintPageBounds(pageSize)
			lines = append(lines, "", fmt.Sprintf("候选路径：第 %d/%d 页", page, totalPages))
			for i := start; i < end; i++ {
				hint := app.uiState.input.hints[i]
				prefix := "  "
				if i == app.uiState.input.hintIndex {
					prefix = "> "
				}
				lines = append(lines, prefix+shorten(hint, 72))
			}
			lines = append(lines, "", "Tab/上下键切换候选，Enter 先填入再导入。")
		}
		return strings.Join(lines, "\n")
	case modeDeleteConfirm:
		return fmt.Sprintf("删除确认\n\n目标书籍：%s\n\n按 y 仅从书架移除。\n按 D 从书架移除并删除本地文件。\n按 Esc 取消。", app.bookshelfState.deleteTargetTitle)
	case modeTOC:
		return tocStatusText()
	case modeBookmarks:
		return buildBookmarksPanel()
	case modeSearchInput:
		return "搜索\n\n请输入关键字并回车执行：\n\n" + renderInputWithCursor(app.uiState.input.value, app.uiState.input.cursor)
	case modeBookshelfSearchInput:
		return "书架搜索\n\n请输入书名关键字并回车过滤：\n\n" + renderInputWithCursor(app.uiState.input.value, app.uiState.input.cursor) + "\n\nEsc 清空搜索并返回书架。"
	case modeReadingJumpInput:
		return "跳转\n\n输入章节号或百分比：\n\n" + renderInputWithCursor(app.uiState.input.value, app.uiState.input.cursor) + "\n\n示例：128 或 50%。"
	case modeReadingSettings:
		return buildReadingSettingsPanel()
	case modeReadingColorInput:
		return "阅读颜色\n\n请输入字体颜色：\n\n" + renderInputWithCursor(app.uiState.input.value, app.uiState.input.cursor) + "\n\n支持 #RRGGBB、#RGB 或 R,G,B。"
	case modeUpdatePrompt:
		return buildUpdatePromptPanel()
	case modeUpdating:
		return buildUpdatingPanel()
	case modeUpdateRestart:
		return buildUpdateRestartPanel()
	default:
		if app.reader == nil {
			return "未打开书籍"
		}
		return buildReadingPanel()
	}
}

func buildReadingPanel() string {
	text := app.reader.CurrentView(readingVisibleSourceLines())
	if _, ok := app.reader.(*reader.OnlineReader); ok {
		text = highlightOnlineText(text, app.readingState.searchQuery)
	} else {
		text = highlightSearchMatches(text, app.readingState.searchQuery)
	}
	if app.readingState.showReadingQuickHelp {
		text = withReadingQuickHelp(text)
	}
	return formatReadingPanel(text)
}

// Escape each original text segment before adding trusted tview highlight tags.
// Search offsets must refer to the original text, not tview's escape notation.
func highlightOnlineText(text, query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return tview.Escape(text)
	}
	pattern := regexp.MustCompile("(?i)" + regexp.QuoteMeta(query))
	var b strings.Builder
	previous := 0
	for _, match := range pattern.FindAllStringIndex(text, -1) {
		b.WriteString(tview.Escape(text[previous:match[0]]))
		b.WriteString("[black:yellow:b]")
		b.WriteString(tview.Escape(text[match[0]:match[1]]))
		b.WriteString("[-:-:-]")
		previous = match[1]
	}
	b.WriteString(tview.Escape(text[previous:]))
	return b.String()
}

func withReadingQuickHelp(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	help := []string{
		"[快捷键](fg:green,mod:bold)  j/k 翻页  ←/→ 切章  g 跳转  / 搜索",
		"          s 书签  B 书签列表  m 目录  C 换源  P 净化",
		"          , 阅读设置  a 对齐  c 颜色  t 自动翻页  q 书架",
		"          z 精简/全信息 · 再按 ? 隐藏",
	}
	for i := range help {
		help[i] = termuiStyleToTview(help[i])
	}
	visible := readingVisibleSourceLines()
	reserved := len(help) + 1
	if visible > reserved && len(lines)+reserved > visible {
		lines = lines[:max(0, visible-reserved)]
	}
	lines = append(lines, "")
	lines = append(lines, help...)
	return strings.Join(lines, "\n")
}
