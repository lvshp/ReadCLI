package core

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"runtime"
	"strings"

	"github.com/TimothyYe/glance/lib"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// termuiStyleToTview converts [text](fg:COLOR,bg:COLOR,mod:bold) to tview [color]text[-] format.
func termuiStyleToTview(text string) string {
	// Pattern: [content](style)
	re := regexp.MustCompile(`\[([^\]]*)\]\(([^)]*)\)`)
	return re.ReplaceAllStringFunc(text, func(match string) string {
		// Extract content between [ and ]
		contentStart := strings.Index(match, "[") + 1
		contentEnd := strings.Index(match, "](")
		if contentEnd < 0 {
			return match
		}
		content := match[contentStart:contentEnd]

		// Extract style between ](
		styleStart := contentEnd + 2
		styleEnd := len(match) - 1
		if styleStart >= styleEnd {
			return match
		}
		style := match[styleStart:styleEnd]

		fg := ""
		bg := ""
		mod := ""

		parts := strings.Split(style, ",")
		for _, part := range parts {
			kv := strings.SplitN(part, ":", 2)
			key := strings.TrimSpace(kv[0])
			if len(kv) == 2 {
				value := strings.TrimSpace(kv[1])
				switch key {
				case "fg":
					fg = value
				case "bg":
					bg = value
				case "mod":
					mod = value
				}
			}
		}

		// Map termui color names to tview color names
		fg = mapColorName(fg)
		bg = mapColorName(bg)
		// Map termui modifier names to tview single-letter flags
		mod = mapModFlag(mod)

		// Build tview tag: [fg]content[-] or [fg:bg]content[-] or [fg:bg:flags]content[-]
		var tag string
		if fg != "" && bg != "" && mod != "" {
			tag = fmt.Sprintf("[%s:%s:%s]", fg, bg, mod)
		} else if fg != "" && bg != "" {
			tag = fmt.Sprintf("[%s:%s]", fg, bg)
		} else if fg != "" && mod != "" {
			tag = fmt.Sprintf("[%s::%s]", fg, mod)
		} else if fg != "" {
			tag = fmt.Sprintf("[%s]", fg)
		} else if bg != "" && mod != "" {
			tag = fmt.Sprintf("[%s:%s]", bg, mod)
		} else if bg != "" {
			tag = fmt.Sprintf("[:%s]", bg)
		} else if mod != "" {
			tag = fmt.Sprintf("[::%s]", mod)
		} else {
			return content
		}

		return tag + content + "[-:-:-]"
	})
}

// mapColorName maps termui-style color names to tview-compatible names.
func mapColorName(name string) string {
	switch name {
	case "cyan":
		return "teal"
	case "magenta":
		return "purple"
	default:
		return name
	}
}

// mapModFlag maps termui modifier names to tview single-letter attribute flags.
func mapModFlag(mod string) string {
	switch mod {
	case "bold":
		return "b"
	case "dim":
		return "d"
	case "italic":
		return "i"
	case "underline":
		return "u"
	case "strikethrough", "strike":
		return "s"
	case "blink":
		return "l"
	case "reverse":
		return "r"
	case "default":
		return ""
	default:
		return mod
	}
}

func refreshChrome() {
	if app == nil || app.config == nil {
		return
	}
	th := currentTheme()
	if app.bossKey {
		applyBossChrome(th)
		return
	}

	headerText := termuiStyleToTview(buildHeader(th))
	leftText := termuiStyleToTview(buildLeftPanel(th))
	rightText := termuiStyleToTview(buildRightPanel(th))
	mainText := termuiStyleToTview(buildMainPanel())
	footerText := termuiStyleToTview(buildFooter())

	header.SetText(headerText)
	left.SetText(leftText)
	right.SetText(rightText)
	main.SetText(mainText)
	footer.SetText(footerText)

	mainTitle := buildMainTitle()
	main.SetTitle(mainTitle)
	left.SetTitle(" " + th.LeftName + " ")
	right.SetTitle(" " + th.RightName + " ")
	footer.SetTitle(" " + strings.ToLower(th.FooterTag) + " ")
	if app.mode == modeHome || app.mode == modeImportInput || app.mode == modeDeleteConfirm {
		main.SetTitle(" " + th.HomeName + " ")
	}

	showBorder := app.showBorder
	header.SetBorder(showBorder)
	main.SetBorder(showBorder)
	left.SetBorder(showBorder)
	right.SetBorder(showBorder)
	footer.SetBorder(showBorder)

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

	textColor := currentReadingTextColor()
	main.SetTextColor(textColor)

	switch app.mode {
	case modeUpdatePrompt:
		main.SetScrollable(true)
		main.ScrollToBeginning()
	default:
		main.SetScrollable(false)
	}
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

func applyLayoutFromApp() {
	w, h := lastTermWidth, lastTermHeight
	if w <= 0 {
		w = fixedWidth
	}
	if h <= 0 {
		h = 24
	}
	applyLayout(w, h)
}

func applyLayout(termWidth, termHeight int) {
	if termHeight <= 0 {
		termHeight = 3
	}
	width := termWidth
	if width <= 0 {
		width = fixedWidth
	}

	leftWidth := 24
	rightWidth := 28
	if width < 120 {
		leftWidth = 20
		rightWidth = 22
	}
	if width < 90 {
		leftWidth = 18
		rightWidth = 0
	}
	mainWidth := width - leftWidth - rightWidth
	if mainWidth < 30 {
		mainWidth = width
		leftWidth = 0
		rightWidth = 0
	}

	headerHeight := 4
	footerHeight := 4
	if termHeight < 16 {
		headerHeight = 4
		footerHeight = 3
	}

	contentHeight := termHeight - headerHeight - footerHeight
	if contentHeight < 1 {
		contentHeight = 1
	}

	mainContentWidth = mainWidth - 2 // minus border
	mainContentHeight = contentHeight - 2 // minus border

	app.contentWidth = readingContentWidth(mainWidth)
	if app.reader != nil {
		app.reader.Reflow(app.contentWidth)
	}

	// Update flex item sizes via ResizeItem
	root.ResizeItem(header, headerHeight, 0)
	root.ResizeItem(midRow, contentHeight, 1)
	root.ResizeItem(footer, footerHeight, 0)

	if leftWidth > 0 {
		midRow.ResizeItem(left, leftWidth, 0)
	} else {
		midRow.ResizeItem(left, 0, 0)
	}
	midRow.ResizeItem(main, 0, 1)
	if rightWidth > 0 {
		midRow.ResizeItem(right, rightWidth, 0)
	} else {
		midRow.ResizeItem(right, 0, 0)
	}

	refreshChrome()
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

// queueRedraw schedules a screen refresh from a goroutine.
func queueRedraw() {
	if tApp != nil {
		tApp.QueueUpdateDraw(func() {})
	}
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
		shell := strings.TrimSpace(os.Getenv("SHELL"))
		if shell == "" {
			shell = "/bin/sh"
		}
		cmd := exec.Command(shell, "-lc", command)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		signal.Ignore(os.Interrupt)
		err := cmd.Run()
		signal.Reset(os.Interrupt)
		result <- err
	})

	err := <-result
	if err != nil {
		app.statusMessage = "老板键程序退出: " + err.Error()
	} else {
		app.statusMessage = "已返回阅读界面"
	}
	applyLayoutFromApp()
	return true
}

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
			_ = lib.SaveConfig(app.config)
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

// tcellKeyEventID converts a tcell key event to a termui-style string ID.
func tcellKeyEventID(ev *tcell.EventKey) string {
	switch ev.Key() {
	case tcell.KeyUp:
		return "<Up>"
	case tcell.KeyDown:
		return "<Down>"
	case tcell.KeyLeft:
		return "<Left>"
	case tcell.KeyRight:
		return "<Right>"
	case tcell.KeyCtrlC:
		return "<C-c>"
	case tcell.KeyCtrlN:
		return "<C-n>"
	case tcell.KeyCtrlP:
		return "<C-p>"
	case tcell.KeyCtrlR:
		return "<C-r>"
	case tcell.KeyEnter:
		return "<Enter>"
	case tcell.KeyEscape:
		return "<Escape>"
	case tcell.KeyBackspace:
		return "<Backspace>"
	case tcell.KeyBackspace2:
		return "<Backspace2>"
	case tcell.KeyDelete:
		return "<Delete>"
	case tcell.KeyTab:
		return "<Tab>"
	case tcell.KeyHome:
		return "<Home>"
	case tcell.KeyEnd:
		return "<End>"
	case tcell.KeyF1, tcell.KeyF2, tcell.KeyF3, tcell.KeyF4,
		tcell.KeyF5, tcell.KeyF6, tcell.KeyF7, tcell.KeyF8,
		tcell.KeyF9, tcell.KeyF10, tcell.KeyF11, tcell.KeyF12:
		return ""
	default:
		r := ev.Rune()
		if r != 0 {
			// Space key arrives as rune ' ' when not Ctrl+Space
			if r == ' ' {
				return "<Space>"
			}
			return string(r)
		}
		return ""
	}
}

func initWidgets() {
	// Use single-line border characters for both focused and unfocused widgets
	// to avoid visual mismatch (║│) between adjacent panels.
	tview.Borders.HorizontalFocus = tview.Borders.Horizontal
	tview.Borders.VerticalFocus = tview.Borders.Vertical
	tview.Borders.TopLeftFocus = tview.Borders.TopLeft
	tview.Borders.TopRightFocus = tview.Borders.TopRight
	tview.Borders.BottomLeftFocus = tview.Borders.BottomLeft
	tview.Borders.BottomRightFocus = tview.Borders.BottomRight

	header = tview.NewTextView()
	left = tview.NewTextView()
	main = tview.NewTextView()
	right = tview.NewTextView()
	footer = tview.NewTextView()

	for _, tv := range []*tview.TextView{header, left, main, right, footer} {
		tv.SetDynamicColors(true)
		tv.SetTextColor(tcell.ColorWhite)
		tv.SetBorder(true)
		tv.SetBackgroundColor(tcell.ColorDefault)
		tv.SetScrollable(false)
	}
}
