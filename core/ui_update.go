package core

import (
	"os"
	"runtime"
	"strings"

	"github.com/lvshp/ReadCLI/lib"
)

func startUpdateCheck(manual bool) {
	if app == nil || app.updateMessages == nil {
		return
	}
	if app.currentVersion == "" || !lib.CurrentPlatformSupported() {
		if manual {
			app.statusMessage = "当前版本不支持自动更新"
		}
		return
	}
	go func(version string, skipped string, manual bool) {
		release, err := lib.FetchLatestRelease(version)
		if err != nil || release == nil {
			if manual {
				app.updateMessages <- updateMessage{
					Kind:   updateFailed,
					Err:    err,
					Manual: true,
				}
			}
			return
		}
		if !lib.ShouldOfferUpdate(version, release.TagName) {
			if manual {
				app.updateMessages <- updateMessage{
					Kind:   updateCurrent,
					Manual: true,
				}
			}
			return
		}
		if !manual && strings.TrimSpace(skipped) == strings.TrimSpace(release.TagName) {
			return
		}
		app.updateMessages <- updateMessage{
			Kind:    updateAvailable,
			Release: release,
			Manual:  manual,
		}
	}(app.currentVersion, app.config.SkippedUpdateVersion, manual)
}

func handleUpdateMessage(message updateMessage) {
	switch message.Kind {
	case updateAvailable:
		if message.Release == nil {
			return
		}
		app.updateRelease = message.Release
		app.updatePromptManual = message.Manual
		if app.mode != modeUpdating && app.mode != modeUpdateRestart {
			app.updateReturnMode = app.mode
			app.mode = modeUpdatePrompt
		}
		app.statusMessage = "发现新版本 " + message.Release.TagName
	case updateInstalled:
		if message.Release != nil {
			app.updateRelease = message.Release
		}
		if app.config != nil {
			app.config.SkippedUpdateVersion = ""
			saveConfig("保存配置")
		}
		app.mode = modeUpdateRestart
		app.statusMessage = "更新已安装，退出后重新启动生效"
	case updateFailed:
		app.mode = app.updateReturnMode
		if message.Err != nil {
			app.statusMessage = "更新失败: " + shorten(message.Err.Error(), 96)
		} else {
			app.statusMessage = "更新失败"
		}
	case updateCurrent:
		app.statusMessage = "当前已经是最新版本"
	}
}

func triggerManualUpdateCheck() {
	if app == nil {
		return
	}
	app.statusMessage = "正在检查更新..."
	renderUIIfReady()
	startUpdateCheck(true)
}

func startUpdateInstall() {
	if app == nil || app.updateRelease == nil || app.updateMessages == nil {
		return
	}
	executablePath, err := os.Executable()
	if err != nil {
		app.statusMessage = "无法定位当前程序"
		return
	}
	asset := lib.SelectReleaseAsset(app.updateRelease, runtime.GOOS, runtime.GOARCH)
	if asset == nil {
		app.statusMessage = "当前平台暂无可用更新包"
		return
	}

	app.mode = modeUpdating
	app.statusMessage = "正在安装更新 " + app.updateRelease.TagName
	renderUIIfReady()

	go func(version string, release *lib.ReleaseInfo, downloadURL, exePath string) {
		err := lib.InstallLatestReleaseAsset(version, downloadURL, exePath)
		if err != nil {
			app.updateMessages <- updateMessage{Kind: updateFailed, Err: err}
			return
		}
		app.updateMessages <- updateMessage{Kind: updateInstalled, Release: release}
	}(app.currentVersion, app.updateRelease, asset.BrowserDownloadURL, executablePath)
}
