package core

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/lvshp/ReadCLI/lib"
	"github.com/rivo/tview"
)

func buildUpdatePromptPanel() string {
	if app.updateState.release == nil {
		return "没有可用更新。"
	}
	body := strings.TrimSpace(app.updateState.release.Body)
	if body == "" {
		body = "本次版本未提供额外说明。"
	}
	question := "是否现在下载并替换当前程序？"
	nextStep := "更新完成后退出，再重新启动即可生效。"
	if runtime.GOOS == "windows" {
		question = "是否现在下载并准备更新？"
		nextStep = "准备完成后退出，由更新助手自动替换。受保护目录需要管理员授权。"
	}
	lines := []string{
		"发现新版本",
		"",
		fmt.Sprintf("当前版本：%s", emptyFallback(strings.TrimSpace(app.currentVersion), "未知")),
		fmt.Sprintf("最新版本：%s", app.updateState.release.TagName),
		fmt.Sprintf("当前二进制：%s", emptyFallback(shortenDisplay(lib.CurrentExecutablePath(), 56), "未知")),
		"",
		question,
		nextStep,
		"",
		"更新说明：",
	}
	for _, line := range wrapDisplayLines(body, max(28, mainContentWidth-4)) {
		lines = append(lines, "  "+line)
	}
	lines = append(lines, "", "j/k 上下翻页，y/Enter 开始更新，n/Esc 稍后再说。")
	if !app.updateState.promptManual {
		lines = append(lines, "如果这次选择不更新，之后启动时不会再提醒这个版本。")
	} else {
		lines = append(lines, "这是手动检查更新，不会受之前的忽略记录影响。")
	}
	return strings.Join(lines, "\n")
}

func buildUpdatingPanel() string {
	version := "最新版本"
	if app.updateState.release != nil && app.updateState.release.TagName != "" {
		version = app.updateState.release.TagName
	}
	progress := app.updateState.progress
	lines := []string{
		"正在安装更新",
		"",
		"目标版本：" + version,
		"",
		updateProgressLabel(progress),
	}
	if progress.Stage == lib.UpdateStageDownload {
		lines = append(lines, "", updateProgressBar(progress, max(18, min(48, mainContentWidth-8))))
		if progress.Total > 0 {
			lines = append(lines, fmt.Sprintf("%s / %s  %d%%", formatBytes(progress.Downloaded), formatBytes(progress.Total), updateProgressPercent(progress)))
		} else if progress.Downloaded > 0 {
			lines = append(lines, "已下载："+formatBytes(progress.Downloaded))
		}
	}
	nextStep := "更新完成后会提示你退出并重新启动生效。"
	if runtime.GOOS == "windows" {
		nextStep = "准备完成后会提示退出，再由更新助手安装。请留意系统管理员授权窗口。"
	}
	lines = append(lines,
		"",
		"ReadCLI 正在从 GitHub Releases 下载并替换当前程序。",
		nextStep,
	)
	return strings.Join(lines, "\n")
}

func buildUpdateRestartPanel() string {
	version := "新版本"
	if app.updateState.release != nil && app.updateState.release.TagName != "" {
		version = app.updateState.release.TagName
	}
	if app.updateState.pendingExit {
		return strings.Join([]string{
			"更新文件已准备",
			"",
			"目标版本：" + version,
			"",
			"按 Enter 退出 ReadCLI，更新助手会自动替换程序。",
			"稍等片刻后，再启动 ReadCLI。",
			"若安装失败，系统会弹出完整错误及恢复文件位置。",
		}, "\n")
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

func updateProgressLabel(progress lib.UpdateProgress) string {
	switch progress.Stage {
	case lib.UpdateStageDownload:
		if progress.Total > 0 {
			return fmt.Sprintf("下载更新包：%d%%", updateProgressPercent(progress))
		}
		if progress.Downloaded > 0 {
			return "下载更新包：" + formatBytes(progress.Downloaded)
		}
		return "下载更新包：准备中"
	case lib.UpdateStageExtract:
		return "解压更新包..."
	case lib.UpdateStageReplace:
		return "替换当前程序..."
	case lib.UpdateStagePrepare:
		return "准备更新文件，可能需要 Windows 管理员授权..."
	default:
		return "准备下载更新包..."
	}
}

func updateErrorLines() []string {
	message := "未提供具体错误。"
	if app.updateState.failure != nil {
		message = app.updateState.failure.Error()
	}
	paragraphs := []string{
		"失败原因：", message,
		"当前程序：", emptyFallback(lib.CurrentExecutablePath(), "未知"),
		"处理方法：",
		"若提示权限不足，请允许 Windows 管理员授权，或将 ReadCLI 安装到当前用户可写的目录。",
		"若更新文件已保留，也可退出 ReadCLI 后，以有写入权限的账户将新程序复制到上方路径。",
		"请根据完整错误检查目录权限、磁盘空间或被占用的文件，返回后按 u 重新检查更新。",
	}
	var lines []string
	for _, paragraph := range paragraphs {
		lines = append(lines, wrapDisplayLines(paragraph, max(8, mainContentWidth))...)
		lines = append(lines, "")
	}
	return lines
}

func updateErrorPageSize() int {
	return max(1, mainContentHeight-2)
}

func moveUpdateErrors(delta int) {
	last := max(0, len(updateErrorLines())-updateErrorPageSize())
	app.updateState.errorScroll = clamp(app.updateState.errorScroll+delta, 0, last)
}

func buildUpdateErrorPanel() string {
	lines := updateErrorLines()
	size := updateErrorPageSize()
	start := clamp(app.updateState.errorScroll, 0, max(0, len(lines)-size))
	app.updateState.errorScroll = start
	page := []string{fmt.Sprintf("更新失败 %d/%d", start+1, len(lines)), "↑/↓ 滚动 · Esc/Enter 返回"}
	page = append(page, lines[start:min(start+size, len(lines))]...)
	// Errors can contain bracketed directory names. Keep paths literal instead
	// of interpreting them as tview colors or the legacy termui markup.
	return tview.Escape(strings.Join(page, "\n"))
}

func updateProgressPercent(progress lib.UpdateProgress) int {
	if progress.Total <= 0 {
		return 0
	}
	percent := int(progress.Downloaded * 100 / progress.Total)
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}

func updateProgressBar(progress lib.UpdateProgress, width int) string {
	if width < 8 {
		width = 8
	}
	innerWidth := width - 2
	filled := 0
	if progress.Total > 0 {
		filled = int(progress.Downloaded * int64(innerWidth) / progress.Total)
	}
	if filled < 0 {
		filled = 0
	}
	if filled > innerWidth {
		filled = innerWidth
	}
	return "[" + strings.Repeat("#", filled) + strings.Repeat(".", innerWidth-filled) + "]"
}

func formatBytes(size int64) string {
	if size < 0 {
		size = 0
	}
	units := []string{"B", "KB", "MB", "GB"}
	value := float64(size)
	unit := units[0]
	for i := 1; i < len(units) && value >= 1024; i++ {
		value /= 1024
		unit = units[i]
	}
	if unit == "B" {
		return fmt.Sprintf("%d %s", size, unit)
	}
	return fmt.Sprintf("%.1f %s", value, unit)
}
