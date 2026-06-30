package core

import (
	"time"

	"github.com/lvshp/ReadCLI/lib"
)

func switchTheme() {
	current := app.config.Theme
	for i, name := range app.themeOrder {
		if name == current {
			app.config.Theme = app.themeOrder[(i+1)%len(app.themeOrder)]
			saveConfig("保存配置")
			setStatus(statusInfo, "主题已切换为 "+app.config.Theme)
			return
		}
	}
	app.config.Theme = app.themeOrder[0]
	saveConfig("保存配置")
}

func toggleBorder() {
	app.uiState.showBorder = !app.uiState.showBorder
	app.config.ShowBorder = app.uiState.showBorder
	saveConfig("保存配置")
}

func toggleCompactMode() {
	app.readingState.compactMode = !app.readingState.compactMode
	app.config.CompactMode = app.readingState.compactMode
	saveConfig("保存配置")
	if app.readingState.compactMode {
		setStatus(statusInfo, "已切换为精简阅读界面")
	} else {
		setStatus(statusInfo, "已切换为全信息阅读界面")
	}
	applyLayoutFromAppWithoutReflow()
}

func toggleTimer() {
	app.readingState.timer = !app.readingState.timer
	if app.readingState.timer {
		refreshTimerTicker()
		setStatus(statusInfo, "自动翻页已开启")
		return
	}
	if app.readingState.ticker != nil {
		app.readingState.ticker.Stop()
		app.readingState.ticker = nil
	}
	setStatus(statusInfo, "自动翻页已关闭")
}

func refreshTimerTicker() {
	if app == nil || !app.readingState.timer {
		return
	}
	if app.readingState.ticker != nil {
		app.readingState.ticker.Stop()
	}
	intervalMs := 3500
	if app.config != nil && app.config.AutoPageIntervalMs >= 100 {
		intervalMs = app.config.AutoPageIntervalMs
	}
	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	app.readingState.ticker = ticker
	go func(local *time.Ticker) {
		for range local.C {
			if tApp == nil {
				return
			}
			queueUIUpdate(func() {
				if app.readingState.ticker != local || !app.readingState.timer {
					return
				}
				if app.mode == modeReading && app.reader != nil {
					moveReading(pageStep())
					refreshChrome()
				}
			})
		}
	}(ticker)
}

func openReadingSettings() {
	transitionTo(modeReadingSettings)
	app.readingState.settingsIndex = 0
	setStatus(statusInfo, "已打开阅读设置")
}

func moveReadingSettings(delta int) {
	items := readingSettingsItems()
	if len(items) == 0 {
		app.readingState.settingsIndex = 0
		return
	}
	app.readingState.settingsIndex += delta
	if app.readingState.settingsIndex < 0 {
		app.readingState.settingsIndex = 0
	}
	if app.readingState.settingsIndex >= len(items) {
		app.readingState.settingsIndex = len(items) - 1
	}
}

func adjustReadingSetting(delta int) {
	if app == nil || app.config == nil {
		return
	}
	switch app.readingState.settingsIndex {
	case 0:
		app.config.ReadingContentWidthRatio += float64(delta) * 0.05
		if app.config.ReadingContentWidthRatio < 0.4 {
			app.config.ReadingContentWidthRatio = 0.4
		}
		if app.config.ReadingContentWidthRatio > 1 {
			app.config.ReadingContentWidthRatio = 1
		}
	case 1:
		app.config.ReadingMarginLeft = max(0, app.config.ReadingMarginLeft+delta)
	case 2:
		app.config.ReadingMarginRight = max(0, app.config.ReadingMarginRight+delta)
	case 3:
		app.config.ReadingMarginTop = max(0, app.config.ReadingMarginTop+delta)
	case 4:
		app.config.ReadingMarginBottom = max(0, app.config.ReadingMarginBottom+delta)
	case 5:
		app.config.ReadingLineSpacing = max(0, app.config.ReadingLineSpacing+delta)
	case 6:
		app.config.AutoPageIntervalMs = max(500, app.config.AutoPageIntervalMs+delta*500)
	}
	saveConfig("保存配置")
	refreshTimerTicker()
	if app.reader != nil {
		applyLayoutFromApp()
	}
	setStatus(statusInfo, "阅读设置已更新")
}

func activateReadingSetting() {
	if app == nil || app.config == nil {
		return
	}
	switch app.readingState.settingsIndex {
	case 7:
		transitionTo(modeReadingColorInput)
		app.uiState.input.value = app.config.ReadingTextColor
		app.uiState.input.cursor = len([]rune(app.uiState.input.value))
	case 8:
		app.config.ReadingHighContrast = !app.config.ReadingHighContrast
		saveConfig("保存配置")
		setStatus(statusInfo, "高对比已切换")
	case 9:
		app.config.ForceBasicColor = !app.config.ForceBasicColor
		saveConfig("保存配置")
		if app.config.ForceBasicColor {
			setStatus(statusInfo, "已切换为基础色模式")
		} else {
			setStatus(statusInfo, "已切换为扩展颜色模式")
		}
	}
}

func applyReadingTextColorInput() {
	value := lib.NormalizeConfiguredColor(app.uiState.input.value)
	if value == "" {
		setStatus(statusError, "颜色格式无效")
		return
	}
	app.config.ReadingTextColor = value
	transitionTo(modeReadingSettings)
	resetInputState()
	saveConfig("保存配置")
	setStatus(statusInfo, "字体颜色已更新")
}

func cycleReadingColorPreset() {
	if app == nil || app.config == nil {
		return
	}
	palette := []string{"#FFFFFF", "#7FDBFF", "#FFDC00", "#2ECC40", "#F012BE"}
	current := lib.NormalizeConfiguredColor(app.config.ReadingTextColor)
	index := -1
	for i, item := range palette {
		if item == current {
			index = i
			break
		}
	}
	app.config.ReadingTextColor = palette[(index+1+len(palette))%len(palette)]
	saveConfig("保存配置")
	setStatus(statusInfo, "字体颜色已切换为 "+app.config.ReadingTextColor)
}

func setDisplayLines(lines int) {
	if lines < 1 {
		lines = 1
	}
	app.readingState.displayLines = lines
	app.config.DisplayLines = lines
	saveConfig("保存配置")
	visible := readingVisibleSourceLines()
	if visible < app.readingState.displayLines {
		setStatusf(statusInfo, "每页正文 %d 行（当前窗口最多显示 %d 行）", app.readingState.displayLines, visible)
	} else {
		setStatusf(statusInfo, "每页正文 %d 行", visible)
	}
	syncCurrentBookState()
}

func displayBossKey() {
	if runConfiguredBossProgram() {
		return
	}
	app.bossKey = !app.bossKey
	if app.bossKey {
		app.readingState.showHelp = false
		app.readingState.showProgress = false
		setStatus(statusInfo, "Boss Key 已开启")
		return
	}
	setStatus(statusInfo, "Boss Key 已关闭")
}

func persistState() {
	if app == nil {
		return
	}
	if app.readingState.ticker != nil {
		app.readingState.ticker.Stop()
		app.readingState.ticker = nil
	}
	if app.reader != nil && app.currentFile != "" {
		syncCurrentBookState()
	}
	app.config.DisplayLines = app.readingState.displayLines
	app.config.ShowBorder = app.uiState.showBorder
	app.config.CompactMode = app.readingState.compactMode
	app.config.SelectedBookshelf = app.bookshelfState.shelfIndex
	saveConfig("保存配置")
	saveBookshelf("保存书架")
	saveBookmarks("保存书签")
	saveProgress("保存进度")
}
