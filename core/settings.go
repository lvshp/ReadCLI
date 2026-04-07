package core

import (
	"fmt"
	"time"

	"github.com/lvshp/ReadCLI/lib"
)

func switchTheme() {
	current := app.config.Theme
	for i, name := range app.themeOrder {
		if name == current {
			app.config.Theme = app.themeOrder[(i+1)%len(app.themeOrder)]
			_ = lib.SaveConfig(app.config)
			app.statusMessage = "主题已切换为 " + app.config.Theme
			return
		}
	}
	app.config.Theme = app.themeOrder[0]
	_ = lib.SaveConfig(app.config)
}

func toggleBorder() {
	app.showBorder = !app.showBorder
	app.config.ShowBorder = app.showBorder
	_ = lib.SaveConfig(app.config)
}

func toggleTimer() {
	app.timer = !app.timer
	if app.timer {
		refreshTimerTicker()
		app.statusMessage = "自动翻页已开启"
		return
	}
	if app.ticker != nil {
		app.ticker.Stop()
		app.ticker = nil
	}
	app.statusMessage = "自动翻页已关闭"
}

func refreshTimerTicker() {
	if app == nil || !app.timer {
		return
	}
	if app.ticker != nil {
		app.ticker.Stop()
	}
	intervalMs := 3500
	if app.config != nil && app.config.AutoPageIntervalMs >= 100 {
		intervalMs = app.config.AutoPageIntervalMs
	}
	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	app.ticker = ticker
	go func(local *time.Ticker) {
		for range local.C {
			if app.ticker != local || !app.timer {
				return
			}
			if app.mode == modeReading && app.reader != nil {
				moveReading(pageStep())
				queueRedraw()
			}
		}
	}(ticker)
}

func openReadingSettings() {
	setMode(modeReadingSettings)
	app.settingsIndex = 0
	app.statusMessage = "已打开阅读设置"
}

func moveReadingSettings(delta int) {
	items := readingSettingsItems()
	if len(items) == 0 {
		app.settingsIndex = 0
		return
	}
	app.settingsIndex += delta
	if app.settingsIndex < 0 {
		app.settingsIndex = 0
	}
	if app.settingsIndex >= len(items) {
		app.settingsIndex = len(items) - 1
	}
}

func adjustReadingSetting(delta int) {
	if app == nil || app.config == nil {
		return
	}
	switch app.settingsIndex {
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
	_ = lib.SaveConfig(app.config)
	refreshTimerTicker()
	if app.reader != nil {
		applyLayoutFromApp()
	}
	app.statusMessage = "阅读设置已更新"
}

func activateReadingSetting() {
	if app == nil || app.config == nil {
		return
	}
	switch app.settingsIndex {
	case 7:
		app.mode = modeReadingColorInput
		app.inputValue = app.config.ReadingTextColor
		app.inputCursor = len([]rune(app.inputValue))
	case 8:
		app.config.ReadingHighContrast = !app.config.ReadingHighContrast
		_ = lib.SaveConfig(app.config)
		app.statusMessage = "高对比已切换"
	case 9:
		app.config.ForceBasicColor = !app.config.ForceBasicColor
		_ = lib.SaveConfig(app.config)
		if app.config.ForceBasicColor {
			app.statusMessage = "已切换为基础色模式"
		} else {
			app.statusMessage = "已切换为扩展颜色模式"
		}
	}
}

func applyReadingTextColorInput() {
	value := lib.NormalizeConfiguredColor(app.inputValue)
	if value == "" {
		app.statusMessage = "颜色格式无效"
		return
	}
	app.config.ReadingTextColor = value
	app.mode = modeReadingSettings
	resetInputState()
	_ = lib.SaveConfig(app.config)
	app.statusMessage = "字体颜色已更新"
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
	_ = lib.SaveConfig(app.config)
	app.statusMessage = "字体颜色已切换为 " + app.config.ReadingTextColor
}

func setDisplayLines(lines int) {
	if lines < 1 {
		lines = 1
	}
	app.displayLines = lines
	app.config.DisplayLines = lines
	_ = lib.SaveConfig(app.config)
	visible := readingVisibleSourceLines()
	if visible < app.displayLines {
		app.statusMessage = fmt.Sprintf("每页正文 %d 行（当前窗口最多显示 %d 行）", app.displayLines, visible)
	} else {
		app.statusMessage = fmt.Sprintf("每页正文 %d 行", visible)
	}
	syncCurrentBookState()
}

func displayBossKey() {
	if runConfiguredBossProgram() {
		return
	}
	app.bossKey = !app.bossKey
	if app.bossKey {
		app.showHelp = false
		app.showProgress = false
		app.statusMessage = "Boss Key 已开启"
		return
	}
	app.statusMessage = "Boss Key 已关闭"
}

func persistState() {
	if app == nil {
		return
	}
	app.config.DisplayLines = app.displayLines
	app.config.ShowBorder = app.showBorder
	app.config.SelectedBookshelf = app.shelfIndex
	_ = lib.SaveConfig(app.config)
	_ = lib.SaveBookshelf(app.bookshelf)
	_ = lib.SaveBookmarks(app.bookmarks)
	_ = lib.SaveProgress(app.progress)
}
