package core

import (
	"strings"
	"time"
)

func refreshChrome() {
	if app == nil || app.config == nil {
		return
	}
	updateStatusMessageLifecycle()
	th := currentTheme()
	if app.bossKey {
		applyBossChrome(th)
		return
	}

	header.SetText(termuiStyleToTview(buildHeader(th)))
	left.SetText(termuiStyleToTview(buildLeftPanel(th)))
	right.SetText(termuiStyleToTview(buildRightPanel(th)))
	mainText := buildMainPanel()
	if app.mode == modeUpdateError {
		main.SetText(mainText)
	} else {
		main.SetText(termuiStyleToTview(mainText))
	}
	footer.SetText(termuiStyleToTview(buildFooter()))

	main.SetTitle(buildMainTitle())
	left.SetTitle(" " + th.LeftName + " ")
	right.SetTitle(" " + th.RightName + " ")
	footer.SetTitle(" " + strings.ToLower(th.FooterTag) + " ")
	if app.mode == modeHome || app.mode == modeImportInput || app.mode == modeDeleteConfirm || app.mode == modeBookshelfSearchInput {
		main.SetTitle(" " + th.HomeName + " ")
	}

	showBorder := app.uiState.showBorder
	if compactReadingUI() {
		header.SetBorder(false)
		left.SetBorder(false)
		right.SetBorder(false)
		footer.SetBorder(false)
		main.SetBorder(false)
		main.SetTitle("")
		footer.SetTitle("")
	} else {
		header.SetBorder(showBorder)
		left.SetBorder(showBorder)
		right.SetBorder(showBorder)
		footer.SetBorder(showBorder)
	}

	header.SetBorderColor(th.HeaderTint)
	main.SetBorderColor(th.Accent)
	left.SetBorderColor(th.SideAccent)
	right.SetBorderColor(th.SideAccent)
	footer.SetBorderColor(th.HeaderTint)

	header.SetTitleColor(th.HeaderTint)
	left.SetTitleColor(th.SideAccent)
	main.SetTitleColor(th.Accent)
	right.SetTitleColor(th.SideAccent)
	footer.SetTitleColor(th.HeaderTint)

	main.SetTextColor(currentReadingTextColor())

	switch app.mode {
	case modeUpdatePrompt, modeUpdateError:
		main.SetScrollable(true)
		main.ScrollToBeginning()
	default:
		main.SetScrollable(false)
	}
}

func updateStatusMessageLifecycle() {
	now := time.Now()
	if app.uiState.statusMessage != app.uiState.lastStatusMessage || app.uiState.statusMessageKind != app.uiState.lastStatusMessageKind {
		app.uiState.lastStatusMessage = app.uiState.statusMessage
		app.uiState.lastStatusMessageKind = app.uiState.statusMessageKind
		app.uiState.statusMessageGeneration++
		scheduleStatusMessageClear(app.uiState.statusMessage, app.uiState.statusMessageGeneration, app.uiState.statusMessageUntil)
	}
	if !app.uiState.statusMessageUntil.IsZero() && !now.Before(app.uiState.statusMessageUntil) {
		clearStatus()
		app.uiState.lastStatusMessage = ""
		app.uiState.lastStatusMessageKind = statusInfo
	}
}

func scheduleStatusMessageClear(message string, generation int, until time.Time) {
	if until.IsZero() || tApp == nil {
		return
	}
	delay := time.Until(until)
	if delay < 0 {
		delay = 0
	}
	go func() {
		time.Sleep(delay)
		queueUIUpdate(func() {
			if app == nil || app.uiState.statusMessageGeneration != generation || app.uiState.statusMessage != message {
				return
			}
			if !app.uiState.statusMessageUntil.IsZero() && !time.Now().Before(app.uiState.statusMessageUntil) {
				clearStatus()
				app.uiState.lastStatusMessage = ""
				app.uiState.lastStatusMessageKind = statusInfo
				refreshChrome()
			}
		})
	}()
}

func applyBossChrome(th theme) {
	header.SetText(termuiStyleToTview(buildBossHeader(th)))
	left.SetText(termuiStyleToTview(buildBossLeftPanel()))
	main.SetText(termuiStyleToTview(buildBossMainPanel()))
	right.SetText(termuiStyleToTview(buildBossRightPanel()))
	footer.SetText(termuiStyleToTview(buildBossFooter()))

	showBorder := app.uiState.showBorder
	header.SetBorder(showBorder)
	main.SetBorder(showBorder)
	left.SetBorder(showBorder)
	right.SetBorder(showBorder)
	footer.SetBorder(showBorder)

	left.SetTitle(" processes ")
	main.SetTitle(" runtime ")
	right.SetTitle(" metrics ")
	footer.SetTitle(" monitor ")

	header.SetBorderColor(th.HeaderTint)
	main.SetBorderColor(th.Accent)
	left.SetBorderColor(th.SideAccent)
	right.SetBorderColor(th.SideAccent)
	footer.SetBorderColor(th.HeaderTint)

	header.SetTitleColor(th.HeaderTint)
	left.SetTitleColor(th.SideAccent)
	main.SetTitleColor(th.Accent)
	right.SetTitleColor(th.SideAccent)
	footer.SetTitleColor(th.HeaderTint)
}

func renderUI() {
	refreshChrome()
}

func renderUIIfReady() {
	if tApp == nil || main == nil {
		return
	}
	refreshChrome()
}

func runConfiguredBossProgram() bool {
	if app == nil || app.config == nil {
		return false
	}
	command := strings.TrimSpace(app.config.BossKeyCommand)
	if command == "" {
		return false
	}

	result := make(chan error, 1)
	tApp.Suspend(func() {
		result <- runBossCommand(command)
	})

	err := <-result
	if err != nil {
		setStatus(statusError, "老板键程序退出: "+err.Error())
	} else {
		setStatus(statusInfo, "已返回阅读界面")
	}
	applyLayoutFromApp()
	return true
}
