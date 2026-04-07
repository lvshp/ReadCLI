package core

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/lvshp/ReadCLI/lib"
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

func buildBossHeader(th theme) string {
	now := time.Now().Format("15:04:05")
	host := "edge-gw-03"
	job := "batch-reconcile"
	switch th.Name {
	case "jetbrains":
		line1 := fmt.Sprintf("[%s](fg:yellow,mod:bold)  workspace [%s](fg:cyan)  run [%s](fg:green)  branch [%s](fg:magenta)  [%s](fg:yellow)",
			th.HeaderName, host, job, "hotfix/runtime", now)
		line2 := "[RUNNING](fg:black,bg:yellow,mod:bold)  [ processes ] [ logs ] [ traces ] [ queues ] [ alerts ]  inspections [0](fg:green)  state [healthy](fg:cyan)"
		return line1 + "\n" + line2
	case "ops-console":
		line1 := fmt.Sprintf("[%s](fg:green,mod:bold)  node [%s](fg:cyan)  task [%s](fg:yellow)  window [%s](fg:white,mod:bold)  [%s](fg:green)",
			th.HeaderName, host, job, "runtime-monitor", now)
		line2 := "[ACTIVE](fg:black,bg:green,mod:bold)  [ ingest ] [ workers ] [ streams ] [ snapshots ] [ audit ]  incidents [0](fg:green)  state [stable](fg:cyan)"
		return line1 + "\n" + line2
	default:
		line1 := fmt.Sprintf("[%s](fg:cyan,mod:bold)  service [%s](fg:yellow)  env [%s](fg:green)  pod [%s](fg:white,mod:bold)  [%s](fg:cyan)",
			th.HeaderName, job, "prod-sh", host, now)
		line2 := "[LIVE](fg:black,bg:cyan,mod:bold)  [ overview ] [ jobs ] [ traces ] [ tasks ] [ deployment ]  diagnostics [0](fg:green)  state [synced](fg:yellow)"
		return line1 + "\n" + line2
	}
}

func buildBossLeftPanel() string {
	return strings.Join([]string{
		"[Process Tree](fg:cyan,mod:bold)",
		"",
		"  supervisor/",
		"    scheduler/",
		"      job-dispatcher",
		"      signal-watcher",
		"    workers/",
		"      parser-01",
		"      parser-02",
		"      merge-queue",
		"    network/",
		"      stream-relay",
		"      rpc-gateway",
		"",
		"[Jobs](fg:yellow,mod:bold)",
		"  pending      03",
		"  running      12",
		"  blocked      00",
		"  retrying     01",
		"",
		"[Focus](fg:green,mod:bold)",
		"  target  nightly sync",
		"  lane    cn-sh-prod",
		"  state   processing",
	}, "\n")
}

func buildBossMainPanel() string {
	now := time.Now()
	lines := []string{
		"[runtime monitor](fg:cyan,mod:bold)",
		"",
		fmt.Sprintf("[%s](fg:green)  bootstrap completed, worker pool online", now.Add(-29*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:green)  queue snapshot refreshed, 124 batches ready", now.Add(-24*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:yellow)  parser-01 processing shard eu-west/14", now.Add(-19*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:yellow)  parser-02 processing shard ap-east/09", now.Add(-16*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:cyan)  merge-queue flushing 18 delta segments", now.Add(-12*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:white)  rpc-gateway heartbeat stable (p95 42ms)", now.Add(-9*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:green)  audit channel synced, no drift detected", now.Add(-6*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:green)  release window healthy, checksum verified", now.Add(-3*time.Second).Format("15:04:05")),
		"",
		"[active tasks](fg:yellow,mod:bold)",
		"  batch-reconcile      running      73%",
		"  metrics-rollup       running      41%",
		"  cold-storage-sync    queued       wait-io",
		"  nightly-report       standby      00:12",
		"",
		"[stream output](fg:magenta,mod:bold)",
		"  shard/eu-west/14     rows 184220   lag 0.3s",
		"  shard/ap-east/09     rows 163871   lag 0.4s",
		"  shard/cn-south/02    rows 201443   lag 0.2s",
		"",
		"[controls](fg:green,mod:bold)",
		"  b  back to reader",
		"  q  exit to shelf",
	}
	return strings.Join(lines, "\n")
}

func buildBossRightPanel() string {
	return strings.Join([]string{
		"[Runtime](fg:cyan,mod:bold)",
		"",
		"  cpu        37%",
		"  memory     2.4 GB",
		"  io wait    1.2%",
		"  net        128 MB/s",
		"",
		"[Workers](fg:yellow,mod:bold)",
		"  online     12",
		"  idle       03",
		"  errors     00",
		"",
		"[Queue](fg:green,mod:bold)",
		"  inflight   124",
		"  retry      01",
		"  backlog    08",
		"",
		"[Recent](fg:magenta,mod:bold)",
		"  parser pool healthy",
		"  storage sync ready",
		"  monitor stable",
	}, "\n")
}

func buildBossFooter() string {
	elapsed := time.Since(app.sessionStart).Round(time.Minute)
	return fmt.Sprintf("[MONITOR](fg:black,bg:green,mod:bold)  uptime [%s](fg:yellow)  state [running](fg:green)  window [runtime](fg:cyan)  [%s](fg:white)\n[Escaped view active](fg:yellow)  [b](fg:cyan):return  [q](fg:red):shelf",
		elapsed, time.Now().Format("2006-01-02 15:04:05"))
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
		return line1 + "\n[↑/↓](fg:cyan):翻页  [←/→](fg:cyan):切章  [+/-](fg:cyan):正文行数  [c](fg:cyan):颜色  [,](fg:cyan):阅读设置  [/](fg:cyan):搜索  [s/B](fg:cyan):书签  [m](fg:cyan):目录  [T](fg:cyan):主题  [u](fg:cyan):更新  [q](fg:red):书架"
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

func buildUpdatePromptPanel() string {
	if app.updateRelease == nil {
		return "没有可用更新。"
	}
	body := strings.TrimSpace(app.updateRelease.Body)
	if body == "" {
		body = "本次版本未提供额外说明。"
	}
	lines := []string{
		"发现新版本",
		"",
		fmt.Sprintf("当前版本：%s", emptyFallback(strings.TrimSpace(app.currentVersion), "未知")),
		fmt.Sprintf("最新版本：%s", app.updateRelease.TagName),
		fmt.Sprintf("当前二进制：%s", emptyFallback(shortenDisplay(lib.CurrentExecutablePath(), 56), "未知")),
		"",
		"是否现在下载并替换当前程序？",
		"更新完成后退出，再重新启动即可生效。",
		"",
		"更新说明：",
	}
	for _, line := range wrapDisplayLines(body, max(28, mainContentWidth-4)) {
		lines = append(lines, "  "+line)
	}
	lines = append(lines, "", "j/k 上下翻页，y/Enter 开始更新，n/Esc 稍后再说。")
	if !app.updatePromptManual {
		lines = append(lines, "如果这次选择不更新，之后启动时不会再提醒这个版本。")
	} else {
		lines = append(lines, "这是手动检查更新，不会受之前的忽略记录影响。")
	}
	return strings.Join(lines, "\n")
}

func buildUpdatingPanel() string {
	version := "最新版本"
	if app.updateRelease != nil && app.updateRelease.TagName != "" {
		version = app.updateRelease.TagName
	}
	return strings.Join([]string{
		"正在安装更新",
		"",
		"目标版本：" + version,
		"",
		"ReadCLI 正在从 GitHub Releases 下载并替换当前程序。",
		"更新完成后会提示你退出并重新启动生效。",
	}, "\n")
}

func buildUpdateRestartPanel() string {
	version := "新版本"
	if app.updateRelease != nil && app.updateRelease.TagName != "" {
		version = app.updateRelease.TagName
	}
	return strings.Join([]string{
		"更新已安装",
		"",
		"已完成热更新：" + version,
		"",
		"请退出当前程序，然后重新启动 ReadCLI。",
		"",
		"按 Enter 退出。",
	}, "\n")
}

func buildBookshelfPanel() string {
	books := visibleBooks()
	var lines []string
	th := currentTheme()
	lines = append(lines, "["+titleCase(th.HomeName)+"](fg:cyan,mod:bold)")
	lines = append(lines, "")
	if app.loadingBookPath != "" {
		bookName := strings.TrimSuffix(filepath.Base(app.loadingBookPath), filepath.Ext(app.loadingBookPath))
		lines = append(lines,
			"正在打开：",
			"",
			"  "+shortenDisplay(bookName, bookshelfTitleWidth()),
			"",
			"请稍等，正在加载正文和目录…",
		)
		return strings.Join(lines, "\n")
	}
	if len(books) == 0 {
		lines = append(lines,
			"还没有导入任何书。",
			"",
			"开始方式：",
			"  1. 按 i 导入本地 txt / epub",
			"  2. 或直接运行 readcli /path/to/book.epub",
			"",
			"导入后会自动记录：",
			"  - 阅读进度",
			"  - 最后阅读时间",
			"  - 当前章节信息",
		)
		return strings.Join(lines, "\n")
	}

	pageSize := bookshelfPageSize()
	start := (app.shelfIndex / pageSize) * pageSize
	end := start + pageSize
	if end > len(books) {
		end = len(books)
	}
	lines = append(lines, fmt.Sprintf("共 %d 本  |  排序 %s  |  过滤 %s  |  第 %d/%d 页", len(books), readableSort(app.sortMode), readableFilter(app.filterMode), start/pageSize+1, (len(books)+pageSize-1)/pageSize))
	lines = append(lines, bookshelfStatsLine(books))
	lines = append(lines, "")
	titleWidth := bookshelfTitleWidth()
	lines = append(lines, "  书名")
	lines = append(lines, "  "+strings.Repeat("─", max(24, titleWidth)))
	for i := start; i < end; i++ {
		book := books[i]
		prefix := "  "
		if i == app.shelfIndex {
			prefix = "> "
		}
		lines = append(lines, prefix+shortenDisplay(book.Title, titleWidth))
	}
	return strings.Join(lines, "\n")
}

func bookshelfTitleWidth() int {
	titleWidth := 28
	if mainContentWidth > 0 {
		available := mainContentWidth - 4
		if available > 18 {
			titleWidth = available
		}
	}
	if titleWidth < 18 {
		titleWidth = 18
	}
	return titleWidth
}

func bookshelfStatsLine(books []lib.BookshelfBook) string {
	unread := 0
	reading := 0
	finished := 0
	for _, book := range books {
		switch {
		case book.ProgressPercent >= 100:
			finished++
		case book.ProgressPos > 0:
			reading++
		default:
			unread++
		}
	}
	return fmt.Sprintf("未读 %d  |  在读 %d  |  已读 %d", unread, reading, finished)
}

func bookshelfPageSize() int {
	if mainContentHeight > 4 {
		reservedLines := 8
		available := mainContentHeight - reservedLines
		if available < 3 {
			return 3
		}
		return available
	}
	return 10
}

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
		"[阅读界面](fg:green,mod:bold)\n  j/k 翻页  [/] 切章  / 搜索\n  n/N 搜索跳转  s 书签  B 书签列表\n  m 目录  p 进度  , 阅读设置\n  c 字体颜色  t 自动翻页\n  b Boss Key  T 主题  +/- 行数",
		"[目录](fg:green,mod:bold)\n  j/k 移动  Enter 打开  m 返回\n  0-9 页码输入",
		"[书签列表](fg:green,mod:bold)\n  j/k 移动  d 删除  Enter 打开\n  B/q 返回",
		"[通用](fg:green,mod:bold)\n  f 切换边框  T 切换主题\n  u 检查更新  q 返回/退出",
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
