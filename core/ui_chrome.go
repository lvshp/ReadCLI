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
	main.SetText(termuiStyleToTview(buildMainPanel()))
	footer.SetText(termuiStyleToTview(buildFooter()))

	main.SetTitle(buildMainTitle())
	left.SetTitle(" " + th.LeftName + " ")
	right.SetTitle(" " + th.RightName + " ")
	footer.SetTitle(" " + strings.ToLower(th.FooterTag) + " ")
	if app.mode == modeHome || app.mode == modeImportInput || app.mode == modeDeleteConfirm || app.mode == modeBookshelfSearchInput {
		main.SetTitle(" " + th.HomeName + " ")
	}

	showBorder := app.showBorder
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
	case modeUpdatePrompt:
		main.SetScrollable(true)
		main.ScrollToBeginning()
	default:
		main.SetScrollable(false)
	}
}

func updateStatusMessageLifecycle() {
	now := time.Now()
	if app.statusMessage != app.lastStatusMessage || app.statusMessageKind != app.lastStatusMessageKind {
		app.lastStatusMessage = app.statusMessage
		app.lastStatusMessageKind = app.statusMessageKind
		app.statusMessageGeneration++
		scheduleStatusMessageClear(app.statusMessage, app.statusMessageGeneration, app.statusMessageUntil)
	}
	if !app.statusMessageUntil.IsZero() && !now.Before(app.statusMessageUntil) {
		clearStatus()
		app.lastStatusMessage = ""
		app.lastStatusMessageKind = statusInfo
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
			if app == nil || app.statusMessageGeneration != generation || app.statusMessage != message {
				return
			}
			if !app.statusMessageUntil.IsZero() && !time.Now().Before(app.statusMessageUntil) {
				clearStatus()
				app.lastStatusMessage = ""
				app.lastStatusMessageKind = statusInfo
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

	showBorder := app.showBorder
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
