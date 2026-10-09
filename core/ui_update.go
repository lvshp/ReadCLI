package core

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/lvshp/ReadCLI/lib"
)

func startUpdateCheck(manual bool) {
	if app == nil || app.updateState.messages == nil {
		return
	}
	if app.currentVersion == "" || !lib.CurrentPlatformSupported() {
		if manual {
			setStatus(statusError, "当前版本不支持自动更新")
		}
		return
	}
	go func(version string, skipped string, manual bool) {
		release, err := lib.FetchLatestRelease(version)
		if err != nil || release == nil {
			if manual {
				app.updateState.messages <- updateMessage{
					Kind:   updateFailed,
					Err:    err,
					Manual: true,
				}
			}
			return
		}
		if !lib.ShouldOfferUpdate(version, release.TagName) {
			if manual {
				app.updateState.messages <- updateMessage{
					Kind:   updateCurrent,
					Manual: true,
				}
			}
			return
		}
		if !manual && strings.TrimSpace(skipped) == strings.TrimSpace(release.TagName) {
			return
		}
		app.updateState.messages <- updateMessage{
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
		app.updateState.release = message.Release
		app.updateState.promptManual = message.Manual
		if app.mode != modeUpdating && app.mode != modeUpdateRestart {
			if !isUpdateMode(app.mode) {
				app.updateState.returnMode = app.mode
			}
			transitionTo(modeUpdatePrompt)
		}
		setStatus(statusInfo, "发现新版本 "+message.Release.TagName)
	case updateInstalled, updatePrepared:
		app.updateState.pendingExit = message.Kind == updatePrepared
		if message.Release != nil {
			app.updateState.release = message.Release
		}
		if app.config != nil {
			app.config.SkippedUpdateVersion = ""
			saveConfig("保存配置")
		}
		transitionTo(modeUpdateRestart)
		if app.updateState.pendingExit {
			setStatus(statusInfo, "更新文件已准备，退出后自动安装")
		} else {
			setStatus(statusInfo, "更新已安装，退出后重新启动生效")
		}
	case updateFailed:
		if !isUpdateMode(app.mode) {
			app.updateState.returnMode = app.mode
		}
		app.updateState.failure = message.Err
		if app.updateState.failure == nil {
			app.updateState.failure = errors.New("未能完成更新，请稍后重新检查。")
		}
		app.updateState.errorScroll = 0
		transitionTo(modeUpdateError)
		setStatus(statusError, "更新失败，请查看中间面板的完整原因")
	case updateCurrent:
		setStatus(statusInfo, "当前已经是最新版本")
	case updateProgress:
		app.updateState.progress = message.Progress
		setStatus(statusProgress, updateProgressStatus(message.Progress))
	}
}

func triggerManualUpdateCheck() {
	if app == nil {
		return
	}
	setStatus(statusProgress, "正在检查更新...")
	renderUIIfReady()
	startUpdateCheck(true)
}

func startUpdateInstall() {
	if app == nil || app.updateState.release == nil || app.updateState.messages == nil {
		return
	}
	executablePath, err := os.Executable()
	if err != nil {
		setStatus(statusError, "无法定位当前程序")
		return
	}
	asset := lib.SelectReleaseAsset(app.updateState.release, runtime.GOOS, runtime.GOARCH)
	if asset == nil {
		setStatus(statusError, "当前平台暂无可用更新包")
		return
	}

	transitionTo(modeUpdating)
	app.updateState.failure = nil
	app.updateState.pendingExit = false
	app.updateState.progress = lib.UpdateProgress{Stage: lib.UpdateStageDownload}
	setStatus(statusProgress, "正在安装更新 "+app.updateState.release.TagName)
	renderUIIfReady()

	go func(version string, release *lib.ReleaseInfo, downloadURL, exePath string) {
		err := lib.InstallLatestReleaseAssetWithProgress(version, downloadURL, exePath, func(progress lib.UpdateProgress) {
			select {
			case app.updateState.messages <- updateMessage{Kind: updateProgress, Progress: progress}:
			default:
			}
		})
		if err != nil {
			app.updateState.messages <- updateMessage{Kind: updateFailed, Err: err}
			return
		}
		kind := updateInstalled
		if runtime.GOOS == "windows" {
			kind = updatePrepared
		}
		app.updateState.messages <- updateMessage{Kind: kind, Release: release}
	}(app.currentVersion, app.updateState.release, asset.BrowserDownloadURL, executablePath)
}

func updateProgressStatus(progress lib.UpdateProgress) string {
	switch progress.Stage {
	case lib.UpdateStageDownload:
		if progress.Total > 0 {
			return fmt.Sprintf("正在下载更新 %d%%", updateProgressPercent(progress))
		}
		if progress.Downloaded > 0 {
			return "正在下载更新 " + formatBytes(progress.Downloaded)
		}
		return "正在下载更新..."
	case lib.UpdateStageExtract:
		return "正在解压更新包..."
	case lib.UpdateStageReplace:
		return "正在替换当前程序..."
	case lib.UpdateStagePrepare:
		return "正在准备更新，如弹出管理员授权请确认..."
	default:
		return "正在安装更新..."
	}
}
