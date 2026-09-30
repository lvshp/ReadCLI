package core

import (
	"fmt"
	"math"
	"strings"

	"github.com/lvshp/ReadCLI/booksource"
)

func isOnlineMode(m mode) bool {
	switch m {
	case modeSourceSwitch, modeSourceSwitchConfirm, modeOnlineErrors, modeSourceLogin, modeSources, modeSourceImport, modeSourceDelete, modeOnlineSearchInput, modeOnlineResults:
		return true
	}
	return false
}

func handleSourceSwitchEvent(id string) {
	if app.online.busy != "" {
		if id == "<Escape>" || id == "q" || id == "<C-c>" {
			closeSourceSwitch()
		}
		return
	}
	if id == "<C-c>" {
		closeSourceSwitch()
		return
	}
	if app.mode == modeSourceSwitchConfirm {
		switch id {
		case "<Enter>", "y":
			confirmSourceSwitch()
		case "<Escape>", "q":
			backToSourceAlternatives()
		}
		return
	}
	switch id {
	case "<Escape>", "q":
		closeSourceSwitch()
	case "j", "<Down>":
		app.sourceSwitch.index = clamp(app.sourceSwitch.index+1, 0, max(0, len(app.sourceSwitch.results)-1))
	case "k", "<Up>":
		app.sourceSwitch.index = clamp(app.sourceSwitch.index-1, 0, max(0, len(app.sourceSwitch.results)-1))
	case "<Enter>":
		prepareSourceSwitch()
	case "n", "<Right>":
		searchSourceAlternatives(max(1, app.sourceSwitch.page) + 1)
	case "p", "<Left>":
		if app.sourceSwitch.page > 1 {
			searchSourceAlternatives(app.sourceSwitch.page - 1)
		}
	case "r":
		searchSourceAlternatives(max(1, app.sourceSwitch.page))
	}
}

func handleOnlineEvent(id string) {
	if id == "<C-c>" {
		cancelOnlineRequest()
		transitionTo(modeHome)
		return
	}
	if id == "<Escape>" && app.online.busy != "" {
		cancelOnlineWithStatus()
		return
	}
	if app.online.busy != "" && !app.online.searching {
		if id == "q" {
			cancelOnlineRequest()
			transitionTo(modeHome)
		}
		return
	}
	switch app.mode {
	case modeOnlineErrors:
		switch id {
		case "q", "<Escape>":
			transitionTo(modeOnlineResults)
		case "j", "<Down>":
			app.online.errorIndex = clamp(app.online.errorIndex+1, 0, max(0, len(app.online.errors)-1))
		case "k", "<Up>":
			app.online.errorIndex = clamp(app.online.errorIndex-1, 0, max(0, len(app.online.errors)-1))
		case "r":
			searchOnlinePage(app.online.page)
		}
	case modeSources:
		switch id {
		case "q", "<Escape>":
			cancelOnlineRequest()
			transitionTo(modeHome)
		case "j", "<Down>":
			app.online.sourceIndex = clamp(app.online.sourceIndex+1, 0, max(0, len(app.online.sources)-1))
		case "k", "<Up>":
			app.online.sourceIndex = clamp(app.online.sourceIndex-1, 0, max(0, len(app.online.sources)-1))
		case "L":
			openSourceLogin()
		case "X":
			logoutSource()
		case "P":
			openPurification()
		case "i":
			transitionTo(modeSourceImport)
		case "e", "<Space>":
			toggleSource()
		case "d":
			if selectedSource() != nil {
				transitionTo(modeSourceDelete)
			}
		case "/", "<Enter>":
			startOnlineSearch(true)
		case "s":
			startOnlineSearch(false)
		}
	case modeSourceDelete:
		switch id {
		case "y":
			deleteSource()
		case "q", "<Escape>":
			transitionTo(modeSources)
		}
	case modeOnlineResults:
		switch id {
		case "e":
			app.online.errorIndex = 0
			transitionTo(modeOnlineErrors)
		case "q", "<Escape>":
			cancelOnlineRequest()
			transitionTo(modeHome)
		case "j", "<Down>":
			app.online.resultIndex = clamp(app.online.resultIndex+1, 0, max(0, len(app.online.results)-1))
		case "k", "<Up>":
			app.online.resultIndex = clamp(app.online.resultIndex-1, 0, max(0, len(app.online.results)-1))
		case "<Enter>":
			openSelectedOnlineBook()
		case "/", "s":
			startOnlineSearch(false)
		case "S":
			openSources()
		case "n", "<Right>":
			searchOnlinePage(app.online.page + 1)
		case "p", "<Left>":
			if app.online.page > 1 {
				searchOnlinePage(app.online.page - 1)
			}
		case "r":
			searchOnlinePage(app.online.page)
		}
	}
}

// Remote metadata is rendered as plain text, never as tview/termui markup.
func remoteLabel(s string) string {
	return strings.NewReplacer("[", "［", "]", "］", "\r", " ", "\n", " ", "\t", " ", "\x1b", "").Replace(s)
}
func onlineListBounds(index, count, headerLines, itemLines int) (int, int) {
	size := max(1, (max(10, mainContentHeight)-headerLines)/itemLines)
	start := max(0, index) / size * size
	return min(start, count), min(start+size, count)
}
func buildOnlinePanel() string {
	switch app.mode {
	case modeSourceSwitch:
		return buildSourceSwitchPanel()
	case modeSourceSwitchConfirm:
		return buildSourceSwitchConfirmPanel()
	case modeOnlineErrors:
		if len(app.online.errors) == 0 {
			return "没有书源错误。按 q 返回搜索结果。"
		}
		index := clamp(app.online.errorIndex, 0, len(app.online.errors)-1)
		progress := ""
		if app.online.searching {
			progress = fmt.Sprintf("\n搜索继续 · 已完成 %d/%d 个书源", app.online.searchDone, app.online.searchTotal)
		}
		return fmt.Sprintf("书源错误 %d/%d%s\n\nj/k 切换 · r 重试 · q 返回\n\n%s", index+1, len(app.online.errors), progress, remoteLabel(app.online.errors[index]))
	case modeSourceLogin:
		return buildSourceLoginPanel()
	case modeSourceImport:
		return "导入书源\n\n粘贴自行准备的 Legado 书源 JSON 文件路径或 JSON 下载链接：\n\n" + renderInputWithCursor(app.uiState.input.value, app.uiState.input.cursor) + "\n\n支持单个书源或书源合集，按书源地址去重更新。\n仅支持 JSON 文件或直链，不支持 HTML 网页。\n导入后可启用、停用或移除书源。\n\n" + remoteLabel(app.online.busy)
	case modeSourceDelete:
		name := ""
		if s := selectedSource(); s != nil {
			name = s.Name
		}
		return "移除书源\n\n" + remoteLabel(name) + "\n\n仅移除书源定义，保留书架和已缓存章节。\n未缓存章节需重新导入书源后才能阅读。\n\n按 y 确认，Esc 返回。"
	case modeOnlineSearchInput:
		scope := "所有已启用书源"
		for _, s := range app.online.sources {
			if s.URL == app.online.scopeURL {
				scope = s.Name
				break
			}
		}
		return "在线搜书\n\n搜索范围：" + remoteLabel(scope) + "\n\n输入书名或作者：\n\n" + renderInputWithCursor(app.uiState.input.value, app.uiState.input.cursor) + "\n\nEnter 搜索 · Esc 返回\n最多同时请求 4 个书源，失败源会单独显示。"
	case modeSources:
		enabled := 0
		for _, s := range app.online.sources {
			if s.IsEnabled() {
				enabled++
			}
		}
		lines := []string{"[书源管理](fg:cyan,mod:bold)", fmt.Sprintf("%d 个书源 · %d 个已启用", len(app.online.sources), enabled), "i 导入 · L 登录 · / 搜索", ""}
		if len(app.online.sources) == 0 {
			return strings.Join(append(lines, "暂无书源。按 i 导入自行准备的 Legado JSON。"), "\n")
		}
		start, end := onlineListBounds(app.online.sourceIndex, len(app.online.sources), 5, 2)
		for i := start; i < end; i++ {
			s := app.online.sources[i]
			prefix := "  "
			if i == app.online.sourceIndex {
				prefix = "> "
			}
			state := "停用"
			if s.IsEnabled() {
				state = "启用"
			}
			if onlineClient().IsLoggedIn(s) {
				state = "已登录"
			}
			lines = append(lines, prefix+"["+state+"] "+shortenDisplay(remoteLabel(s.Name), max(10, mainContentWidth-12)), "    "+shortenDisplay(remoteLabel(emptyFallback(s.Group, s.URL)), max(10, mainContentWidth-6)))
		}
		return strings.Join(lines, "\n")
	default:
		lines := []string{"[在线搜索](fg:cyan,mod:bold)", fmt.Sprintf("%s · 第 %d 页 · %d 本", remoteLabel(app.online.keyword), max(1, app.online.page), len(app.online.results))}
		if app.online.searchTotal > 0 {
			progress := fmt.Sprintf("已完成 %d/%d 个书源 · %d 个失败", app.online.searchDone, app.online.searchTotal, len(app.online.errors))
			if app.online.searching {
				progress += " · 搜索中"
			}
			lines = append(lines, progress)
		}
		lines = append(lines, "Enter 阅读 · n/p 翻页 · e 错误", "")
		if app.online.busy != "" && !app.online.searching {
			return strings.Join(append(lines, remoteLabel(app.online.busy), "Esc 取消加载"), "\n")
		}
		if len(app.online.results) == 0 {
			if app.online.searching {
				lines = append(lines, "等待书源返回，结果将陆续显示。", "Esc 取消搜索并保留已有结果。")
			} else {
				lines = append(lines, "暂无结果，请更换关键词或检查书源。")
			}
		} else {
			headerLines := len(lines) + max(0, 10-mainContentHeight)
			if len(app.online.errors) > 0 {
				headerLines += 2
			}
			start, end := onlineListBounds(app.online.resultIndex, len(app.online.results), headerLines, 2)
			for i := start; i < end; i++ {
				b := app.online.results[i]
				prefix := "  "
				if i == app.online.resultIndex {
					prefix = "> "
				}
				lines = append(lines, prefix+shortenDisplay(remoteLabel(b.Name), max(12, mainContentWidth-4)), "    "+shortenDisplay(remoteLabel(emptyFallback(b.Author, "未知作者")+" · "+b.SourceName), max(12, mainContentWidth-6)))
			}
		}
		if len(app.online.errors) > 0 {
			lines = append(lines, "", fmt.Sprintf("%d 个源失败：%s", len(app.online.errors), shortenDisplay(remoteLabel(app.online.errors[0]), max(12, mainContentWidth-18))))
		}
		return strings.Join(lines, "\n")
	}
}
func buildOnlineLeftPanel() string {
	if app.mode == modeSourceSwitch || app.mode == modeSourceSwitchConfirm {
		return "[更换书源](fg:cyan,mod:bold)\n\n[候选列表](fg:yellow,mod:bold)\n  ↑/↓  选择版本\n  Enter 加载并预览\n  n/p  搜索上下页\n  r    刷新当前页\n  Esc/q 取消换源\n\n[确认换源](fg:yellow,mod:bold)\n  Enter/y 确认替换\n  Esc/q 返回候选\n\n加载中按 Esc 取消，\n返回原来的书架或阅读。\n确认前保留原书和进度。"
	}
	return "[阅读中心](fg:cyan,mod:bold)\n\n  q    返回书架\n  S    书源管理\n\n[书源管理](fg:yellow,mod:bold)\n  i    导入书源\n  L    登录书源\n  X    退出登录\n  P    净化配置\n  空格 启用/停用\n  /    搜索当前源\n  s    搜索全部\n  d    移除书源\n\n[搜索结果](fg:yellow,mod:bold)\n  /    重新搜索\n  n/p  上下页\n  Enter 在线阅读\n  Esc  取消加载"
}
func buildOnlineRightPanel() string {
	if app.mode == modeSourceSwitch || app.mode == modeSourceSwitchConfirm {
		return buildSourceSwitchRightPanel()
	}
	if app.mode == modeSources || app.mode == modeSourceDelete {
		s := selectedSource()
		if s == nil {
			return "[书源信息](fg:cyan,mod:bold)\n\n支持 Legado JSON 书源。\n按 i 导入开始使用。"
		}
		status := "可用规则"
		if err := booksource.ValidateSource(*s); err != nil {
			status = err.Error()
		}
		return "[书源信息](fg:cyan,mod:bold)\n\n" + remoteLabel(s.Name) + "\n\n" + remoteLabel(s.URL) + "\n\n分组：" + remoteLabel(s.Group) + "\n\n" + remoteLabel(status)
	}
	lines := []string{"[搜索详情](fg:cyan,mod:bold)", ""}
	if len(app.online.results) > 0 {
		b := app.online.results[clamp(app.online.resultIndex, 0, len(app.online.results)-1)]
		lines = append(lines, remoteLabel(b.Name), remoteLabel(b.Author), "", remoteLabel(b.SourceName), "", remoteLabel(b.LastChapter), "", remoteLabel(b.Intro))
	}
	if len(app.online.errors) > 0 {
		lines = append(lines, "", "[书源错误](fg:yellow,mod:bold)")
		for _, err := range app.online.errors[:min(6, len(app.online.errors))] {
			lines = append(lines, remoteLabel(err))
		}
	}
	return strings.Join(lines, "\n")
}

func buildSourceSwitchPanel() string {
	lines := []string{
		"[更换书源](fg:cyan,mod:bold)",
		shortenDisplay(fmt.Sprintf("%s · 第 %d 页 · %d 个候选", remoteLabel(app.sourceSwitch.original.Title), max(1, app.sourceSwitch.page), len(app.sourceSwitch.results)), max(8, mainContentWidth)),
	}
	if mainContentHeight >= 10 {
		lines = append(lines, shortenDisplay("Enter 预览 · n/p 翻页 · r 刷新 · Esc/q 取消", max(8, mainContentWidth)), "")
	}
	if app.online.busy != "" {
		return strings.Join(append(lines, remoteLabel(app.online.busy), "Esc 取消换源并返回，原书和进度保留。"), "\n")
	}
	if len(app.sourceSwitch.results) == 0 {
		lines = append(lines, "暂无其他版本。可按 r 重试，或返回书源管理启用更多书源。")
	} else {
		headerLines := len(lines) + max(0, 10-mainContentHeight)
		if len(app.sourceSwitch.errors) > 0 {
			headerLines++
		}
		start, end := onlineListBounds(app.sourceSwitch.index, len(app.sourceSwitch.results), headerLines, 4)
		width := max(8, mainContentWidth-6)
		for i := start; i < end; i++ {
			b := app.sourceSwitch.results[i]
			prefix := "  "
			if i == app.sourceSwitch.index {
				prefix = "> "
			}
			lines = append(lines,
				prefix+shortenDisplay(remoteLabel(b.Name), max(8, mainContentWidth-2)),
				"  源："+shortenDisplay(remoteLabel(emptyFallback(b.SourceName, "未命名书源")), width),
				"作者："+shortenDisplay(remoteLabel(emptyFallback(b.Author, "未知作者")), width),
				"更新："+shortenDisplay(remoteLabel(emptyFallback(b.LastChapter, "暂无章节信息")), width),
			)
		}
	}
	if len(app.sourceSwitch.errors) > 0 {
		lines = append(lines, "[请求失败](fg:yellow,mod:bold) "+shortenDisplay(remoteLabel(strings.Join(app.sourceSwitch.errors, "；")), max(8, mainContentWidth-12)))
	}
	return strings.Join(lines, "\n")
}

func buildSourceSwitchConfirmPanel() string {
	plan := app.sourceSwitch.prepared
	if plan == nil {
		return "换源预览尚未就绪。按 Esc 返回候选列表。"
	}
	original := app.sourceSwitch.original
	oldSource, oldSite := "未命名书源", "暂无章节信息"
	if original.Online != nil {
		oldSource = emptyFallback(original.Online.SourceName, oldSource)
		oldSite = emptyFallback(original.Online.LastChapter, emptyFallback(original.Online.Kind, oldSite))
	}
	chapter := "未知章节"
	if plan.index >= 0 && plan.index < len(plan.chapters) {
		chapter = plan.chapters[plan.index].Name
	}
	if mainContentHeight < 10 {
		width := max(8, mainContentWidth)
		sourceLine := func(label, name, site string) string {
			return shortenDisplay(label+shortenDisplay(remoteLabel(name), max(4, width/3))+" · "+remoteLabel(site), width)
		}
		match := "[已匹配章节名称](fg:green)"
		if !plan.matched {
			match = "[未匹配：按旧序号近似定位](fg:yellow,mod:bold)"
		}
		return strings.Join([]string{
			sourceLine("原：", oldSource, oldSite),
			sourceLine("新：", emptyFallback(plan.book.SourceName, "未命名书源"), emptyFallback(plan.book.LastChapter, plan.book.Kind)),
			shortenDisplay(fmt.Sprintf("第 %d 章：%s", plan.index+1, remoteLabel(chapter)), width),
			fmt.Sprintf("章内进度：%.0f%%", math.Min(1, math.Max(0, plan.offset))*100),
			match,
			"Enter/y 确认 · Esc/q 返回",
		}, "\n")
	}
	lines := []string{
		"[确认换源](fg:cyan,mod:bold) · " + shortenDisplay(remoteLabel(plan.book.Name), max(8, mainContentWidth-13)),
		"Enter/y 确认 · Esc/q 返回候选",
		"原源：" + remoteLabel(oldSource),
		"原站点/更新：" + remoteLabel(oldSite),
		"新源：" + remoteLabel(emptyFallback(plan.book.SourceName, "未命名书源")),
		"新站点/更新：" + remoteLabel(emptyFallback(plan.book.LastChapter, emptyFallback(plan.book.Kind, "暂无章节信息"))),
		fmt.Sprintf("新章节：第 %d 章 · %s", plan.index+1, remoteLabel(chapter)),
		fmt.Sprintf("章内进度：%.0f%%", math.Min(1, math.Max(0, plan.offset))*100),
	}
	for i := 1; i < len(lines); i++ {
		lines[i] = shortenDisplay(lines[i], max(8, mainContentWidth))
	}
	if plan.matched {
		lines = append(lines, "[已按章节名称匹配阅读位置](fg:green)")
	} else {
		lines = append(lines, "[未匹配同名章节：按旧序号近似定位](fg:yellow,mod:bold)")
	}
	if mainContentHeight > len(lines)+2 {
		lines = append(lines,
			"目录和正文已加载；确认后迁移进度与匹配书签。",
			"未匹配书签仍保留，可能需要回原源查看。",
		)
	}
	return strings.Join(lines, "\n")
}

func buildSourceSwitchRightPanel() string {
	original := app.sourceSwitch.original
	lines := []string{"[原书](fg:cyan,mod:bold)", "", remoteLabel(original.Title)}
	if original.Online != nil {
		lines = append(lines,
			"书源："+remoteLabel(original.Online.SourceName),
			"作者："+remoteLabel(emptyFallback(original.Online.Author, "未知作者")),
			"更新："+remoteLabel(emptyFallback(original.Online.LastChapter, original.Online.Kind)),
		)
	}
	lines = append(lines, "章节："+remoteLabel(original.CurrentChapter), fmt.Sprintf("章内：%.0f%%", math.Min(1, math.Max(0, original.ChapterOffset))*100))
	var selected *booksource.Book
	if app.mode == modeSourceSwitchConfirm && app.sourceSwitch.prepared != nil {
		selected = &app.sourceSwitch.prepared.book
	} else if len(app.sourceSwitch.results) > 0 {
		selected = &app.sourceSwitch.results[clamp(app.sourceSwitch.index, 0, len(app.sourceSwitch.results)-1)]
	}
	if selected != nil {
		lines = append(lines,
			"", "[候选版本](fg:yellow,mod:bold)", "",
			remoteLabel(selected.Name),
			"书源："+remoteLabel(selected.SourceName),
			"作者："+remoteLabel(emptyFallback(selected.Author, "未知作者")),
			"更新："+remoteLabel(emptyFallback(selected.LastChapter, "暂无章节信息")),
			remoteLabel(selected.Kind),
		)
	}
	if len(app.sourceSwitch.errors) > 0 {
		lines = append(lines, "", "[请求错误](fg:yellow,mod:bold)")
		for _, err := range app.sourceSwitch.errors {
			lines = append(lines, remoteLabel(err))
		}
	}
	return strings.Join(lines, "\n")
}
