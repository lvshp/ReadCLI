package core

import (
	"fmt"
	"strings"
)

func isPurificationMode(m mode) bool {
	return m == modePurification || m == modePurificationImport
}

func handlePurificationEvent(id string) {
	if app.mode == modePurificationImport {
		if id == "<Escape>" || id == "<C-c>" {
			cancelOnlineRequest()
			transitionTo(modePurification)
			setStatus(statusInfo, "已取消净化规则导入")
			return
		}
		if app.online.busy == "" {
			handleTextInputEvent(id, importPurification)
		}
		return
	}
	if app.purification.showErrors {
		switch id {
		case "<Escape>", "q", "e":
			app.purification.showErrors = false
			app.purification.errorScroll = 0
		case "<C-c>":
			closePurification()
		case "j", "<Down>":
			movePurificationErrors(1)
		case "k", "<Up>":
			movePurificationErrors(-1)
		case "<Space>":
			movePurificationErrors(purificationErrorPageSize())
		case "<Home>":
			app.purification.errorScroll = 0
		case "<End>":
			movePurificationErrors(len(purificationErrorLines()))
		}
		return
	}
	if id == "<Escape>" || id == "q" || id == "<C-c>" {
		closePurification()
		return
	}
	if app.online.busy != "" {
		return
	}
	switch id {
	case "j", "<Down>":
		app.purification.index = clamp(app.purification.index+1, 0, max(0, len(app.purification.files)-1))
	case "k", "<Up>":
		app.purification.index = clamp(app.purification.index-1, 0, max(0, len(app.purification.files)-1))
	case "i":
		transitionTo(modePurificationImport)
	case "r":
		reloadPurification()
	case "e":
		if len(app.purification.errors) > 0 {
			app.purification.showErrors = true
			app.purification.errorScroll = 0
		} else {
			setStatus(statusInfo, "暂无净化错误")
		}
	}
}

func buildPurificationPanel() string {
	if app.mode == modePurificationImport {
		lines := []string{
			"[导入净化规则](fg:cyan,mod:bold)",
			"JSON 文件路径或下载链接：",
			renderInputWithCursor(app.uiState.input.value, app.uiState.input.cursor),
			"Enter 导入 · Esc 返回配置页",
		}
		if app.online.busy != "" {
			lines = append(lines, remoteLabel(app.online.busy))
		} else if mainContentHeight >= 10 {
			lines = append(lines, "", "支持规则数组或单条规则。", "导入后保存在 imports 目录中，原始 JSON 保留。", "本地追加规则可编辑 main.json，按 r 重新载入。")
		}
		return strings.Join(lines, "\n")
	}
	if app.purification.showErrors {
		return buildPurificationErrorsPanel()
	}
	width := max(8, mainContentWidth)
	state := app.purification
	lines := []string{
		fmt.Sprintf("[净化配置](fg:cyan,mod:bold)  启用 %d / %d", state.enabledCount, state.totalCount),
		shortenDisplay("main.json: "+remoteLabel(state.configPath), width),
		shortenDisplay("imports: "+remoteLabel(state.importsPath), width),
	}
	if app.online.busy != "" {
		return strings.Join(append(lines, remoteLabel(app.online.busy), "Esc 取消并返回"), "\n")
	}
	if mainContentHeight >= 10 {
		lines = append(lines, "i 导入 · r 重载 · ↑/↓ 选择文件", "")
	}
	if len(state.files) == 0 {
		lines = append(lines, "暂无规则文件，按 i 导入或 r 重新载入。")
	} else {
		headerLines := len(lines) + max(0, 10-mainContentHeight)
		if len(state.errors) > 0 {
			headerLines++
		}
		start, end := onlineListBounds(state.index, len(state.files), headerLines, 1)
		for i := start; i < end; i++ {
			file := state.files[i]
			prefix := "  "
			if i == state.index {
				prefix = "> "
			}
			count := fmt.Sprintf(" %d/%d", file.EnabledCount, file.RuleCount)
			lines = append(lines, prefix+shortenDisplay(remoteLabel(file.Name), max(6, width-2-len(count)))+count)
		}
	}
	if len(state.errors) > 0 {
		lines = append(lines, fmt.Sprintf("[配置错误](fg:yellow,mod:bold) %d 条 · e 查看详情", len(state.errors)))
	}
	return strings.Join(lines, "\n")
}

func purificationErrorLines() []string {
	if len(app.purification.errors) == 0 {
		return []string{"暂无净化错误。"}
	}
	var lines []string
	for i, message := range app.purification.errors {
		if i > 0 {
			lines = append(lines, "")
		}
		label := fmt.Sprintf("%d. %s", i+1, remoteLabel(message))
		lines = append(lines, wrapDisplayLines(label, max(8, mainContentWidth))...)
	}
	return lines
}

func purificationErrorPageSize() int {
	return max(1, mainContentHeight-2)
}

func movePurificationErrors(delta int) {
	last := max(0, len(purificationErrorLines())-purificationErrorPageSize())
	app.purification.errorScroll = clamp(app.purification.errorScroll+delta, 0, last)
}

func buildPurificationErrorsPanel() string {
	wrapped := purificationErrorLines()
	size := purificationErrorPageSize()
	start := clamp(app.purification.errorScroll, 0, max(0, len(wrapped)-size))
	app.purification.errorScroll = start
	end := min(start+size, len(wrapped))
	lines := []string{
		fmt.Sprintf("[错误详情](fg:yellow,mod:bold) %d/%d", start+1, len(wrapped)),
		"↑/↓ 滚动 · Esc/q 返回",
	}
	return strings.Join(append(lines, wrapped[start:end]...), "\n")
}

func buildPurificationLeftPanel() string {
	if app.purification.showErrors {
		return "[错误详情](fg:yellow,mod:bold)\n\n  ↑/↓  滚动一行\n  空格 下一页\n  Home 开头\n  End  末尾\n  Esc/q 返回文件列表\n\n显示完整文件路径、\n规则名称和错误原因。"
	}
	return "[净化替换](fg:cyan,mod:bold)\n\n  ↑/↓  选择规则文件\n  i    导入 JSON\n  r    重新载入\n  e    查看完整错误\n  Esc/q 返回原页面\n\n用于在线正文和章节标题。\n修改 JSON 后按 r 重载；\n返回阅读后按原进度重开。\n\n[执行顺序](fg:yellow,mod:bold)\n  imports 按文件名\n  每组规则按 order\n  main.json 最后应用\n\n原始章节缓存保留。"
}

func buildPurificationRightPanel() string {
	state := app.purification
	lines := []string{"[规则文件](fg:cyan,mod:bold)", ""}
	if len(state.files) > 0 {
		file := state.files[clamp(state.index, 0, len(state.files)-1)]
		kind := "导入规则"
		if file.Local {
			kind = "本地追加规则"
		}
		lines = append(lines,
			remoteLabel(file.Name), kind,
			fmt.Sprintf("启用 %d / 总数 %d", file.EnabledCount, file.RuleCount),
			"", "完整路径：", remoteLabel(file.Path),
		)
	}
	lines = append(lines, "", "[配置位置](fg:yellow,mod:bold)", "main.json:", remoteLabel(state.configPath), "", "imports:", remoteLabel(state.importsPath))
	for _, err := range state.errors {
		lines = append(lines, "", remoteLabel(err))
	}
	return strings.Join(lines, "\n")
}
