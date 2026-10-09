package core

import (
	"fmt"
	"strings"

	"github.com/lvshp/ReadCLI/booksource"
	"github.com/mattn/go-runewidth"
)

func buildSourceManagerPanel() string {
	switch app.mode {
	case modeSourceGroups:
		return sourceManagerGroupsPanel()
	case modeSourceActions:
		lines := []string{"批量操作", sourceManagerScope()}
		if mainContentHeight < 10 {
			lines = append(lines, "1 启用   2 停用", "3 收藏   4 取消收藏", "5 加标签 6 移除本地标签", "7 删除 · Esc/q 返回")
		} else {
			lines = append(lines, "1 启用", "2 停用", "3 收藏", "4 取消收藏", "5 添加本地标签", "6 移除本地标签", "7 删除", "Esc/q 返回")
		}
		return sourceManagerFit(lines, mainContentWidth, mainContentHeight)
	case modeSourceConfirm:
		return sourceManagerConfirmPanel()
	case modeSourceTag:
		return sourceManagerFit([]string{app.sourceManager.pending.label, fmt.Sprintf("已固定 %d 个书源", len(app.sourceManager.pending.keys)), sourceManagerInput("标签：", mainContentWidth), "Enter 预览确认 · Esc 返回", "只修改本地标签，原书源分组保留。"}, mainContentWidth, mainContentHeight)
	case modeSourceImportPreview, modeSourceImportFilter:
		return sourceManagerImportPanel()
	case modeSourceSearchScope:
		favorite := 0
		for _, s := range app.online.sources {
			if managerMeta(s).Favorite {
				favorite++
			}
		}
		return sourceManagerFit([]string{"选择搜书范围 · Esc/q 返回", "仅搜索范围内已启用的书源", fmt.Sprintf("1 常用收藏（%d）", favorite), fmt.Sprintf("2 当前筛选（%d）", len(managerVisibleIndices())), fmt.Sprintf("3 已选书源（%d）", managerSelectedCount()), fmt.Sprintf("4 全部书源（%d）", len(app.online.sources))}, mainContentWidth, mainContentHeight)
	default:
		return sourceManagerListPanel()
	}
}

func sourceManagerListPanel() string {
	m := &app.sourceManager
	indices := managerVisibleIndices()
	header := managerListHeaderLines()
	start, end, page, pages := sourceManagerPage(app.online.sourceIndex, len(indices), header)
	lines := []string{
		"书源管理 · " + sourceManagerSortLabel(),
		fmt.Sprintf("%d/%d页 匹配%d 已选%d", page, pages, len(indices), managerSelectedCount()),
		"视图：" + managerViewLabel(),
		"/Enter搜当前 · s选搜索范围",
	}
	if m.query != "" {
		lines[2] = "筛选：" + m.query
	}
	if app.mode == modeSourceFilter {
		lines[2] = sourceManagerInput("筛选：", mainContentWidth)
		lines[3] = "实时匹配 · Enter保留 Esc取消"
	} else if m.batch {
		lines[3] = "多选：Space/x勾选 · b批量操作"
	}
	if header == 5 {
		lines = append(lines, "□选择 ☆收藏 启/停 登录状态 名称")
	}
	lines = lines[:header]
	if len(indices) == 0 {
		lines = append(lines, "没有匹配书源 · f修改筛选 / i导入")
	}
	for i := start; i < end; i++ {
		s := app.online.sources[indices[i]]
		pointer, check, favorite, enabled := "  ", "□", "☆", "停"
		if i == app.online.sourceIndex {
			pointer = "> "
		}
		if m.selected[booksource.SourceKey(s)] {
			check = "☑"
		}
		if managerMeta(s).Favorite {
			favorite = "★"
		}
		if s.IsEnabled() {
			enabled = "启"
		}
		lines = append(lines, fmt.Sprintf("%s%s %s %s %s %s", pointer, check, favorite, enabled, sourceManagerLoginLabel(s), emptyFallback(s.Name, "未命名书源")))
	}
	return sourceManagerFit(lines, mainContentWidth, mainContentHeight)
}

func sourceManagerGroupsPanel() string {
	groups := managerGroups()
	header := min(4, max(1, mainContentHeight-1))
	start, end, page, pages := sourceManagerPage(app.sourceManager.groupIndex, len(groups), header)
	lines := []string{"分组与合集", "当前：" + managerViewLabel(), fmt.Sprintf("%d/%d页 · 共%d项", page, pages, len(groups)), "Enter选择 · d移除合集 · Esc返回"}
	lines = lines[:header]
	for i := start; i < end; i++ {
		g := groups[i]
		pointer := "  "
		if i == app.sourceManager.groupIndex {
			pointer = "> "
		}
		suffix := fmt.Sprintf(" (%d)", g.count)
		if g.kind == "collection" && g.id != "legacy" {
			suffix = " #" + sourceManagerShortID(g.id) + suffix
		}
		lines = append(lines, sourceManagerColumns(pointer+sourceManagerGroupKind(g.kind)+" ", g.label, suffix, mainContentWidth))
	}
	return sourceManagerFit(lines, mainContentWidth, mainContentHeight)
}

func sourceManagerConfirmPanel() string {
	op := app.sourceManager.pending
	lines := []string{sourceManagerColumns("确认", op.label, fmt.Sprintf(" · %d个", len(op.keys)), mainContentWidth), "Enter/y 确认 · Esc/q 取消"}
	if op.kind == "collection-delete" {
		_, removed, shared := booksource.RemoveCollection(app.online.sources, op.collectionID)
		lines = append(lines, fmt.Sprintf("删除 %d 个独有源", removed), fmt.Sprintf("保留 %d 个共享/原有源", shared), "书架与缓存保留")
	} else if op.kind == "delete" {
		lines = append(lines, "仅删除以下书源定义", "书架与缓存保留")
	} else if op.tag != "" {
		lines = append(lines, "本地标签："+op.tag, "原书源分组保留")
	} else {
		lines = append(lines, "操作范围已固定为以下书源")
	}
	remaining := max(0, mainContentHeight-len(lines))
	shown := min(len(op.keys), remaining)
	if shown < len(op.keys) && remaining > 1 {
		shown--
	}
	for _, key := range op.keys[:shown] {
		name := "已移除的书源"
		if s := managerSourceByKey(key); s != nil {
			name = emptyFallback(s.Name, "未命名书源")
		}
		lines = append(lines, "· "+name)
	}
	if shown < len(op.keys) && remaining > 1 {
		lines = append(lines, fmt.Sprintf("…另%d个，共%d个", len(op.keys)-shown, len(op.keys)))
	}
	return sourceManagerFit(lines, mainContentWidth, mainContentHeight)
}

func sourceManagerImportPanel() string {
	m := &app.sourceManager
	if m.preview == nil {
		return "没有待导入的书源\nEsc 返回"
	}
	p := m.preview
	indices := managerPreviewIndices()
	selected := 0
	for _, s := range p.Sources {
		if m.importSelected[booksource.SourceKey(s)] {
			selected++
		}
	}
	header := managerImportHeaderLines()
	start, end, page, pages := sourceManagerPage(m.importIndex, len(indices), header)
	enable := "停用"
	if m.importEnable {
		enable = "启用"
	}
	lines := []string{fmt.Sprintf("导入预览 · %d/%d页", page, pages), fmt.Sprintf("新增%d 更新%d 不支持%d", p.Added, p.Updated, p.Unsupported), fmt.Sprintf("已选%d/%d 新源%s(e)", selected, len(p.Sources), enable), "f筛选 a选筛选 A清空", "Enter导入 · Esc取消"}
	if m.importQuery != "" {
		lines[3] = fmt.Sprintf("匹配%d：%s", len(indices), m.importQuery)
	}
	if app.mode == modeSourceImportFilter {
		lines[3] = sourceManagerInput("筛选：", mainContentWidth)
		lines[4] = "Enter保留筛选 · Esc取消筛选"
	}
	if header == 9 {
		lines = append(lines, "已有书源保持启停，不支持的规则停用", "合集："+p.Collection.Name+" #"+sourceManagerShortID(p.Collection.ID), fmt.Sprintf("当前匹配%d个 · Space/x勾选", len(indices)), "↑/↓选择 · PgUp/PgDn翻页")
	}
	lines = lines[:header]
	if len(indices) == 0 {
		lines = append(lines, "没有匹配书源 · f修改筛选")
	}
	for i := start; i < end; i++ {
		s := p.Sources[indices[i]]
		pointer, check, kind := "  ", "□", "新增"
		if i == m.importIndex {
			pointer = "> "
		}
		if m.importSelected[booksource.SourceKey(s)] {
			check = "☑"
		}
		if managerSourceByKey(booksource.SourceKey(s)) != nil {
			kind = "更新"
		}
		if booksource.ValidateSource(s) != nil {
			kind += "!"
		}
		lines = append(lines, pointer+check+" "+kind+" "+emptyFallback(s.Name, "未命名书源"))
	}
	return sourceManagerFit(lines, mainContentWidth, mainContentHeight)
}

func buildSourceManagerLeftPanel() string {
	width := 22
	if left != nil {
		if _, _, w, _ := left.GetInnerRect(); w > 0 {
			width = w
		}
	}
	lines := []string{"书源管理", "g 分组 / 合集", "当前：" + managerViewLabel(), "f 筛选 · o 排序", "i 导入 · h 检查", "v 多选 · b 批量", "* 收藏 · u 撤销"}
	for _, g := range managerGroups() {
		if g.kind != "group" && g.kind != "collection" {
			continue
		}
		if len(lines) >= mainContentHeight {
			break
		}
		suffix := fmt.Sprintf(" %d", g.count)
		if g.kind == "collection" && g.id != "legacy" {
			suffix = " #" + sourceManagerShortID(g.id) + suffix
		}
		lines = append(lines, sourceManagerColumns(sourceManagerGroupKind(g.kind)+" ", g.label, suffix, width))
	}
	return sourceManagerFit(lines, width, mainContentHeight)
}

func buildSourceManagerRightPanel() string {
	var source *booksource.Source
	m := &app.sourceManager
	preview := app.mode == modeSourceImportPreview || app.mode == modeSourceImportFilter
	if preview && m.preview != nil {
		indices := managerPreviewIndices()
		if len(indices) > 0 {
			source = &m.preview.Sources[indices[clamp(m.importIndex, 0, len(indices)-1)]]
		}
	} else if i := managerRawIndex(); i >= 0 {
		source = &app.online.sources[i]
	}
	if source == nil {
		return "书源详情\n没有匹配书源\nf 修改筛选 · i 导入"
	}
	s := *source
	meta := managerMeta(s)
	enabled := s.IsEnabled()
	if preview {
		enabled = m.importEnable
		if old := managerSourceByKey(booksource.SourceKey(s)); old != nil {
			meta, enabled = managerMeta(*old), old.IsEnabled()
		}
		if booksource.ValidateSource(s) != nil {
			enabled = false
		}
	}
	enabledText := "停用"
	if enabled {
		enabledText = "启用"
	}
	favorite := "否"
	if meta.Favorite {
		favorite = "是"
	}
	lines := []string{"书源详情", emptyFallback(s.Name, "未命名书源"), "启停：" + enabledText + " · 登录：" + sourceManagerLoginLabel(s), "常用收藏：" + favorite, "原分组：" + emptyFallback(s.Group, "无"), "本地标签：" + emptyFallback(strings.Join(meta.Tags, "、"), "无")}
	if len(meta.Collections) == 0 || meta.Standalone {
		lines = append(lines, "归属：历史/独立来源")
	}
	if len(meta.Collections) > 0 {
		labels := make([]string, 0, len(meta.Collections))
		for _, c := range meta.Collections {
			labels = append(labels, c.Name+" #"+sourceManagerShortID(c.ID))
		}
		lines = append(lines, "合集归属："+strings.Join(labels, "；"))
	}
	if meta.Check == nil {
		lines = append(lines, "最后检查：未检查")
	} else {
		check := meta.Check
		lines = append(lines, "最后检查："+check.CheckedAt.Local().Format("01-02 15:04"), sourceHealthStageLabel(check.Stage)+" · "+sourceHealthStatusLabel(check.Status)+fmt.Sprintf(" · %dms", check.DurationMS), check.Detail)
	}
	if preview {
		lines = append(lines, "导入后归属："+m.preview.Collection.Name, "旧启停保持；不支持的规则停用")
	}
	if err := booksource.ValidateSource(s); err != nil {
		lines = append(lines, "规则："+err.Error())
	}
	lines = append(lines, "地址："+s.URL, "L 登录 · X 退出登录", "P 净化配置")
	width := 26
	if right != nil {
		if _, _, w, _ := right.GetInnerRect(); w > 0 {
			width = w
		}
	}
	return sourceManagerFit(lines, width, mainContentHeight)
}

func sourceManagerFooter() string {
	switch app.mode {
	case modeSourceFilter, modeSourceImportFilter:
		return "输入实时筛选 · Enter保留 · Esc取消"
	case modeSourceGroups:
		return "↑/↓选择 · Enter应用 · d移除合集 · Esc返回"
	case modeSourceActions:
		return "1–7选择操作 · Esc/q返回"
	case modeSourceConfirm:
		return "Enter/y确认 · Esc/q取消"
	case modeSourceTag:
		return "输入本地标签 · Enter预览确认 · Esc返回"
	case modeSourceImportPreview:
		if lastTermWidth < 70 {
			return "Space/x勾选 f筛选 e启停 Enter导入 Esc取消"
		}
		return "Space/x勾选 · f筛选 · a选筛选 A清空 · e新源启停 · Enter导入 · Esc取消"
	case modeSourceSearchScope:
		return "1常用 2当前筛选 3已选 4全部 · Esc返回"
	default:
		if app.sourceManager.batch {
			return "Space/x勾选 a选筛选 A清空 b操作 Esc退出多选"
		}
		return "f筛选 g分组 v多选 b批量 s搜索范围"
	}
}

func sourceManagerScope() string {
	if count := managerSelectedCount(); count > 0 {
		return fmt.Sprintf("范围：已选 %d 个（含筛选外）", count)
	}
	return fmt.Sprintf("范围：当前筛选 %d 个", len(managerVisibleIndices()))
}

func sourceManagerSortLabel() string {
	switch app.sourceManager.sort {
	case "name":
		return "名称排序"
	case "latency":
		return "检查耗时排序"
	default:
		return "收藏优先"
	}
}

func sourceManagerLoginLabel(s booksource.Source) string {
	if app.online.client != nil && app.online.client.IsLoggedIn(s) {
		return "已登"
	}
	if managerNeedsLogin(s) {
		return "未登"
	}
	return "—"
}

func sourceManagerGroupKind(kind string) string {
	switch kind {
	case "collection":
		return "合集"
	case "group":
		return "分组"
	default:
		return "视图"
	}
}

func sourceManagerShortID(id string) string {
	runes := []rune(id)
	return string(runes[max(0, len(runes)-8):])
}

func sourceManagerPage(index, count, header int) (start, end, page, pages int) {
	size := max(1, mainContentHeight-header)
	index = clamp(index, 0, max(0, count-1))
	start = index / size * size
	return start, min(start+size, count), index/size + 1, max(1, (count+size-1)/size)
}

func managerListHeaderLines() int {
	header := 5
	if mainContentHeight < 7 {
		header = 4
	}
	return min(header, max(1, mainContentHeight-1))
}

func managerImportHeaderLines() int {
	header := 9
	if mainContentHeight < 12 {
		header = 5
	}
	return min(header, max(1, mainContentHeight-1))
}

func sourceManagerColumns(prefix, name, suffix string, width int) string {
	prefix, name, suffix = remoteLabel(prefix), remoteLabel(name), remoteLabel(suffix)
	room := max(0, width-runewidth.StringWidth(prefix)-runewidth.StringWidth(suffix))
	return prefix + shortenDisplay(name, room) + suffix
}

func sourceManagerInput(label string, width int) string {
	input := app.uiState.input
	runes := []rune(input.value)
	cursor := clamp(input.cursor, 0, len(runes))
	before, after := remoteLabel(string(runes[:cursor])), remoteLabel(string(runes[cursor:]))
	available := max(1, width-runewidth.StringWidth(label)-1)
	left := []rune(before)
	start, used := len(left), 0
	for start > 0 && used+runewidth.RuneWidth(left[start-1]) <= available {
		start--
		used += runewidth.RuneWidth(left[start])
	}
	return label + string(left[start:]) + "|" + shortenDisplay(after, max(0, available-used))
}

func sourceManagerFit(lines []string, width, height int) string {
	lines = lines[:min(len(lines), max(1, height))]
	for i := range lines {
		lines[i] = shortenDisplay(remoteLabel(lines[i]), max(1, width))
	}
	return strings.Join(lines, "\n")
}
