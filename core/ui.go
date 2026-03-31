package core

import (
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/TimothyYe/glance/reader"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

var (
	p          *widgets.Paragraph
	header     *widgets.Paragraph
	leftPanel  *widgets.Paragraph
	rightPanel *widgets.Paragraph
	footer     *widgets.Paragraph
	r          reader.Reader
	ticker     *time.Ticker

	showBorder   = true
	showHelp     = false
	showProgress = false
	showTOC      = false
	bossKey      = false
	timer        = false
	rowNumber    = ""
	color        = 0
	tocIndex     = 0
	tocNumber    = ""
	displayLines = 1
	onExit       func(int)
	contentWidth = fixedWidth
	bookTitle    = "workspace.md"
	sessionStart = time.Now()
)

func setTimer() {
	timer = !timer

	if timer {
		ticker = time.NewTicker(interval * time.Millisecond)
		go func() {
			for range ticker.C {
				r.Next()
				p.Text = currentReadingText()
				renderUI()
			}
		}()
	} else {
		ticker.Stop()
	}
}

func updateParagraph(key string) {
	p.Text = key
}

func currentReadingText() string {
	return r.CurrentView(displayLines)
}

func readingWidth(termWidth int) int {
	width := termWidth
	if width <= 0 {
		width = fixedWidth
	}
	if width > 2 {
		return width - 2
	}

	return width
}

func switchColor() {
	p.TextStyle.Fg = ui.Color(color % 8)
}

func displayHelp(current string) {
	showHelp = !showHelp
	if showHelp {
		p.Text = menuText
	} else {
		p.Text = current
	}
}

func displayBorder() {
	showBorder = !showBorder
	p.Border = showBorder
	header.Border = showBorder
	leftPanel.Border = showBorder
	rightPanel.Border = showBorder
	footer.Border = showBorder
}

func displayProgress(current, progress string) {
	showProgress = !showProgress
	showTOC = false
	if showProgress {
		p.Text = progress
	} else {
		p.Text = current
	}
	refreshChrome()
}

func displayTOC() {
	showTOC = !showTOC
	showHelp = false
	showProgress = false

	if showTOC {
		tocIndex = r.CurrentChapterIndex()
		tocNumber = ""
		p.Text = r.GetTOCWithSelection(tocIndex, tocPageSize())
		refreshChrome()
		return
	}

	tocNumber = ""
	p.Text = currentReadingText()
	refreshChrome()
}

func updateTOCSelection(offset int) {
	tocIndex += offset
	tocNumber = ""
	p.Text = r.GetTOCWithSelection(tocIndex, tocPageSize())
	refreshChrome()
}

func appendTOCNumber(digit string) {
	tocNumber += digit
	if index, ok := parseTOCNumber(); ok {
		tocIndex = index
	}
	p.Text = tocStatusText()
	refreshChrome()
}

func openSelectedTOCChapter() {
	if index, ok := parseTOCNumber(); ok {
		tocIndex = index
	}
	showTOC = false
	tocNumber = ""
	r.GotoChapter(tocIndex)
	p.Text = currentReadingText()
	refreshChrome()
}

func tocStatusText() string {
	text := r.GetTOCWithSelection(tocIndex, tocPageSize())
	if tocNumber == "" {
		return text
	}

	return text + "\nOpen chapter: " + tocNumber
}

func parseTOCNumber() (int, bool) {
	if tocNumber == "" {
		return 0, false
	}

	num, err := strconv.Atoi(tocNumber)
	if err != nil || num <= 0 {
		return 0, false
	}

	return num - 1, true
}

func tocPageSize() int {
	if p != nil && p.Inner.Dy() > 0 {
		reservedLines := 4
		if tocNumber != "" {
			reservedLines++
		}

		available := p.Inner.Dy() - reservedLines
		if available > 0 {
			return available
		}
	}

	_, height := ui.TerminalDimensions()
	if height <= 8 {
		return 1
	}

	return height - 8
}

func setDisplayLines(lines int) {
	if lines < 1 {
		lines = 1
	}

	displayLines = lines
	if p != nil && !showTOC && !showHelp && !showProgress && !bossKey {
		p.Text = currentReadingText()
		refreshChrome()
	}
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

	contentWidth = readingWidth(mainWidth)
	r.Reflow(contentWidth)
	headerHeight := 5
	footerHeight := 3
	if termHeight < 16 {
		headerHeight = 4
		footerHeight = 2
	}
	header.SetRect(0, 0, width, headerHeight)
	footer.SetRect(0, termHeight-footerHeight, width, termHeight)

	contentTop := headerHeight
	contentBottom := termHeight - footerHeight
	if contentBottom <= contentTop {
		contentBottom = termHeight
	}

	if leftWidth > 0 {
		leftPanel.SetRect(0, contentTop, leftWidth, contentBottom)
	}
	if rightWidth > 0 {
		rightPanel.SetRect(width-rightWidth, contentTop, width, contentBottom)
	}
	p.SetRect(leftWidth, contentTop, width-rightWidth, contentBottom)

	switch {
	case showTOC:
		p.Text = tocStatusText()
	case showHelp:
		p.Text = menuText
	case showProgress:
		p.Text = r.GetProgress()
	case bossKey:
		p.Text = fakeShell
	default:
		p.Text = currentReadingText()
	}

	refreshChrome()
}

func adjustDisplayLines(delta int) {
	setDisplayLines(displayLines + delta)
}

func pageStep() int {
	if displayLines < 1 {
		return 1
	}

	return displayLines
}

func moveReading(delta int) {
	r.Goto(r.CurrentPos() + delta)
	p.Text = currentReadingText()
	refreshChrome()
}

func displayBossKey(current string) {
	bossKey = !bossKey
	if bossKey {
		p.Border = false
		p.Text = fakeShell
	} else {
		p.Text = current
		p.Border = showBorder
	}
	refreshChrome()
}

func renderUI() {
	widgetsToRender := []ui.Drawable{header, p, footer}
	if leftPanel != nil && leftPanel.Inner.Dx() > 0 {
		widgetsToRender = append(widgetsToRender, leftPanel)
	}
	if rightPanel != nil && rightPanel.Inner.Dx() > 0 {
		widgetsToRender = append(widgetsToRender, rightPanel)
	}
	ui.Render(widgetsToRender...)
}

func refreshChrome() {
	if header == nil || footer == nil || leftPanel == nil || rightPanel == nil {
		return
	}

	header.Text = buildHeader()
	leftPanel.Text = buildLeftPanel()
	rightPanel.Text = buildRightPanel()
	footer.Text = buildFooter()
}

func buildHeader() string {
	now := time.Now().Format("15:04")
	mode := "Reading"
	switch {
	case showTOC:
		mode = "Index"
	case showHelp:
		mode = "Help"
	case showProgress:
		mode = "Progress"
	case timer:
		mode = "Auto Scroll"
	}

	line1 := fmt.Sprintf(
		"[VS Code](fg:cyan,mod:bold)  [ops-console](fg:green)  branch [fish/quiet-mode](fg:yellow)  [%s](fg:white,mod:bold)  [%s](fg:cyan)",
		shorten(bookTitle, 28),
		now,
	)
	line2 := "[ explorer ] [ issues ] [ terminal ] [ notes ] [ deploy ] [ review ]"
	line3 := fmt.Sprintf(
		"[ main.go ] [ reader.go ] [ %s ]  diagnostics: [0](fg:green)  tests: [passing](fg:green)  mode: [%s](fg:yellow)",
		shorten(bookTitle, 18),
		mode,
	)
	return strings.Join([]string{line1, line2, line3}, "\n")
}

func buildLeftPanel() string {
	currentChapter := r.CurrentChapterTitle()
	if currentChapter == "" {
		currentChapter = "Inbox"
	}

	lines := []string{
		"[Explorer](fg:cyan,mod:bold)",
		"",
		"  project/",
		"    cmd/",
		"    core/",
		"    reader/",
		"    docs/",
		"    assets/",
		fmt.Sprintf("    > %s", shorten(bookTitle, 14)),
		"",
		"[Git Changes](fg:yellow,mod:bold)",
		"  M core/ui.go",
		"  M reader/epub_reader.go",
		"  M reader/content_reader.go",
		"",
		"[Current Focus](fg:green,mod:bold)",
		fmt.Sprintf("  %s", shorten(currentChapter, 14)),
		"  context sync",
		"  detail review",
	}

	return strings.Join(lines, "\n")
}

func buildRightPanel() string {
	progress := r.GetProgress()
	chapter := r.CurrentChapterTitle()
	if chapter == "" {
		chapter = "General"
	}

	lines := []string{
		"[Activity Bar](fg:cyan,mod:bold)",
		"",
		"  source control",
		"  search",
		"  extensions",
		"  output",
		"",
		"[Inspector](fg:yellow,mod:bold)",
		fmt.Sprintf("  chapter  %s", shorten(chapter, 16)),
		fmt.Sprintf("  progress %s", progress),
		fmt.Sprintf("  page     %d lines", displayLines),
		"",
		"[Problems](fg:red,mod:bold)",
		"  warnings: 0",
		"  blockers: 0",
		"",
		"[Recent Logs](fg:green,mod:bold)",
		"  10:24 index refreshed",
		"  10:26 reader resumed",
		"  10:27 worktree stable",
	}

	return strings.Join(lines, "\n")
}

func buildFooter() string {
	elapsed := time.Since(sessionStart).Round(time.Minute)
	line1 := fmt.Sprintf(
		"[NORMAL](fg:black,bg:green,mod:bold)  utf-8  fish-mode  branch: fish/quiet-mode  session: [%s](fg:yellow)",
		elapsed,
	)
	line2 := "[j/k](fg:cyan):page  [+/-](fg:cyan):lines  [[/]](fg:cyan):chapter  [m](fg:cyan):index  [p](fg:cyan):progress  [q](fg:red):quit"
	return strings.Join([]string{line1, line2}, "\n")
}

func shorten(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max || max <= 1 {
		return value
	}

	return string(runes[:max-1]) + "…"
}

// Init ui & components
func Init(gr reader.Reader, lines int, exitFn func(int), filePath string) {
	r = gr
	setDisplayLines(lines)
	onExit = exitFn
	bookTitle = filepath.Base(filePath)
	sessionStart = time.Now()

	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize the termui: %v", err)
	}

	termWidth, termHeight := ui.TerminalDimensions()

	header = widgets.NewParagraph()
	leftPanel = widgets.NewParagraph()
	rightPanel = widgets.NewParagraph()
	p = widgets.NewParagraph()
	footer = widgets.NewParagraph()

	header.TextStyle.Fg = ui.ColorWhite
	leftPanel.TextStyle.Fg = ui.ColorWhite
	rightPanel.TextStyle.Fg = ui.ColorWhite
	p.TextStyle.Fg = ui.ColorWhite
	footer.TextStyle.Fg = ui.ColorWhite

	header.BorderStyle.Fg = ui.ColorBlue
	leftPanel.BorderStyle.Fg = ui.ColorBlue
	rightPanel.BorderStyle.Fg = ui.ColorBlue
	p.BorderStyle.Fg = ui.ColorCyan
	footer.BorderStyle.Fg = ui.ColorBlue

	header.Border = showBorder
	leftPanel.Border = showBorder
	rightPanel.Border = showBorder
	p.Border = showBorder
	footer.Border = showBorder
	p.Title = " editor: " + bookTitle + " "
	applyLayout(termWidth, termHeight)

	renderUI()
	handleEvents()
}
