package core

import (
	"fmt"
	"strings"

	"github.com/lvshp/ReadCLI/lib"
)

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
