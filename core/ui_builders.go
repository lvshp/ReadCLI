package core

import (
	"fmt"
	"strings"
	"time"
)

func buildHeader(th theme) string {
	now := time.Now().Format("15:04")
	modeLabel := strings.ToUpper(string(app.mode))
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
		line2 := fmt.Sprintf("[%s](fg:black,bg:green,mod:bold)  [ bookshelf ] [ search ] [ bookmarks ] [ reader ]  diagnostics [0](fg:green)  theme [%s](fg:cyan)",
			modeLabel,
			th.Name,
		)
		return line1 + "\n" + line2
	}
}

func buildLeftPanel(th theme) string {
	if app.mode == modeHome || app.mode == modeImportInput || app.mode == modeDeleteConfirm {
		return strings.Join([]string{
			"[Bookshelf](fg:cyan,mod:bold)",
			"",
			"[Actions](fg:yellow,mod:bold)",
			"  Enter   打开书籍",
			"  i       导入文件",
			"  o       排序视图",
			"  r       过滤视图",
			"  x       移出书架",
			"  T       切换主题",
			"  u       检查更新",
			"",
			"[Sort](fg:yellow,mod:bold)",
			"  " + readableSort(app.sortMode),
			"",
			"[Filter](fg:green,mod:bold)",
			"  " + readableFilter(app.filterMode),
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
		"  , 阅读设置",
		"  T 切主题",
		"  u 检查更新",
		"",
		"[Current Focus](fg:green,mod:bold)",
		"  " + shorten(currentChapter, 14),
	}, "\n")
}

func buildRightPanel(th theme) string {
	if app.mode == modeHome || app.mode == modeImportInput || app.mode == modeDeleteConfirm {
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
			} else if book.ProgressPos > 0 {
				status = "在读"
			}
			lines = append(lines,
				"  标题    "+shorten(book.Title, 16),
				"  格式    "+strings.ToUpper(book.Format),
				"  状态    "+status,
				fmt.Sprintf("  进度    %d%%", book.ProgressPercent),
				"  章节    "+shorten(book.CurrentChapter, 16),
				"  最近    "+shorten(lastRead, 16),
				"",
				"[Continue](fg:yellow,mod:bold)",
				"  回车继续阅读",
			)
		}
		lines = append(lines, "",
			"[Recent Status](fg:green,mod:bold)",
			"  home ready",
			"  import available",
			"  theme synced",
		)
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
	lines = append(lines, buildDetailBlock("查询", emptyFallback(app.searchQuery, "无"), width)...)
	lines = append(lines, "", "[Recent Logs](fg:green,mod:bold)", "  reader resumed", "  progress synced", "  layout stable")
	return strings.Join(lines, "\n")
}

func buildFooter() string {
	if compactReadingUI() {
		return compactReadingStatusLine()
	}
	elapsed := time.Since(app.sessionStart).Round(time.Minute)
	tag := currentTheme().FooterTag
	version := strings.TrimSpace(app.currentVersion)
	if version == "" {
		version = "dev"
	}
	line1 := fmt.Sprintf("[%s](fg:black,bg:green,mod:bold)  utf-8  session [%s](fg:yellow)  theme [%s](fg:cyan)  version [%s](fg:yellow)  [%s](fg:green)",
		tag, elapsed, app.config.Theme, version, app.statusMessage)
	switch app.mode {
	case modeHome:
		return line1 + "\n[↑/↓](fg:cyan):选择  [→/Enter](fg:cyan):打开  [i](fg:cyan):导入  [o/r](fg:cyan):排序/过滤  [x](fg:cyan):移除  [T](fg:cyan):主题  [u](fg:cyan):更新  [q](fg:red):退出"
	case modeReading:
		return line1 + "\n[↑/↓](fg:cyan):翻页  [←/→](fg:cyan):切章  [+/-](fg:cyan):正文行数  [c](fg:cyan):颜色  [,](fg:cyan):阅读设置  [/](fg:cyan):搜索  [s/B](fg:cyan):书签  [m](fg:cyan):目录  [z](fg:cyan):精简/全信息  [T](fg:cyan):主题  [u](fg:cyan):更新  [q](fg:red):书架"
	case modeTOC:
		return line1 + "\n[↑/↓](fg:cyan):移动  [→/Enter](fg:cyan):打开  [←/m](fg:cyan):返回  [0-9](fg:cyan):跳章  [q](fg:red):书架"
	case modeBookmarks:
		return line1 + "\n[↑/↓](fg:cyan):移动  [→/Enter](fg:cyan):打开  [d](fg:cyan):删除  [←/B/q](fg:red):关闭"
	case modeSearchInput:
		return line1 + "\n输入搜索关键字，支持左右移动，Enter 执行，Esc 取消"
	case modeImportInput:
		scope := "当前层"
		if app.importRecursive {
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
	progress := "(0 / 0)"
	if app != nil && app.reader != nil {
		if current := strings.TrimSpace(app.reader.CurrentChapterTitle()); current != "" {
			chapter = current
		}
		progress = app.reader.GetProgress()
	}
	width := max(20, mainContentWidth)
	return fmt.Sprintf("[章节](fg:cyan) %s  [进度](fg:yellow) %s", shortenDisplay(chapter, max(8, width-24)), progress)
}

func buildMainTitle() string {
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
	case modeUpdatePrompt, modeUpdating, modeUpdateRestart:
		return " update "
	default:
		return " editor: " + currentDisplayName() + " "
	}
}

func buildMainPanel() string {
	if app.showHelp {
		return buildHelpPanel()
	}
	if app.showProgress && app.reader != nil {
		return app.reader.GetProgress()
	}
	switch app.mode {
	case modeHome:
		return buildBookshelfPanel()
	case modeImportInput:
		scopeLabel := "当前层"
		if app.importRecursive {
			scopeLabel = "递归子目录"
		}
		lines := []string{
			"导入本地书籍",
			"",
			"请输入 txt / epub 文件路径，或一个文件夹路径：",
			"",
			renderInputWithCursor(app.inputValue, app.inputCursor),
			"",
			"支持左右移动、删除、Tab 补全、拖入文件/目录，以及目录批量导入。",
			"当前扫描范围：" + scopeLabel,
			"按 Ctrl-r 切换当前层 / 递归子目录。",
		}
		if len(app.inputHints) > 0 {
			pageSize := importHintPageSize()
			start, end, page, totalPages := importHintPageBounds(pageSize)
			lines = append(lines, "", fmt.Sprintf("候选路径：第 %d/%d 页", page, totalPages))
			for i := start; i < end; i++ {
				hint := app.inputHints[i]
				prefix := "  "
				if i == app.inputHintIndex {
					prefix = "> "
				}
				lines = append(lines, prefix+shorten(hint, 72))
			}
			lines = append(lines, "", "Tab/上下键切换候选，Enter 先填入再导入。")
		}
		return strings.Join(lines, "\n")
	case modeDeleteConfirm:
		return fmt.Sprintf("删除确认\n\n目标书籍：%s\n\n按 y 仅从书架移除。\n按 D 从书架移除并删除本地文件。\n按 Esc 取消。", app.deleteTargetTitle)
	case modeTOC:
		return tocStatusText()
	case modeBookmarks:
		return buildBookmarksPanel()
	case modeSearchInput:
		return "搜索\n\n请输入关键字并回车执行：\n\n" + renderInputWithCursor(app.inputValue, app.inputCursor)
	case modeReadingSettings:
		return buildReadingSettingsPanel()
	case modeReadingColorInput:
		return "阅读颜色\n\n请输入字体颜色：\n\n" + renderInputWithCursor(app.inputValue, app.inputCursor) + "\n\n支持 #RRGGBB、#RGB 或 R,G,B。"
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
		return formatReadingPanel(highlightSearchMatches(app.reader.CurrentView(readingVisibleSourceLines()), app.searchQuery))
	}
}
