package core

func applyLayoutFromApp() {
	applyLayoutFromAppWithReflow(true)
}

func applyLayoutFromAppWithoutReflow() {
	applyLayoutFromAppWithReflow(false)
}

func applyLayoutFromAppWithReflow(reflow bool) {
	w, h := lastTermWidth, lastTermHeight
	if w <= 0 {
		w = fixedWidth
	}
	if h <= 0 {
		h = 24
	}
	applyLayoutWithReflow(w, h, reflow)
}

func applyLayout(termWidth, termHeight int) {
	applyLayoutWithReflow(termWidth, termHeight, true)
}

func applyLayoutWithReflow(termWidth, termHeight int, reflow bool) {
	if termHeight <= 0 {
		termHeight = 3
	}
	width := termWidth
	if width <= 0 {
		width = fixedWidth
	}

	leftWidth := 24
	rightWidth := 28
	headerHeight := 4
	footerHeight := 4
	compact := compactReadingUI()
	if compact {
		leftWidth = 0
		rightWidth = 0
		headerHeight = 0
		footerHeight = 1
	}
	if !compact && width < 120 {
		leftWidth = 20
		rightWidth = 22
	}
	if !compact && width < 90 {
		leftWidth = 18
		rightWidth = 0
	}
	mainWidth := width - leftWidth - rightWidth
	if mainWidth < 30 {
		mainWidth = width
		leftWidth = 0
		rightWidth = 0
	}

	if !compact && termHeight < 16 {
		headerHeight = 4
		footerHeight = 3
	}

	contentHeight := termHeight - headerHeight - footerHeight
	if contentHeight < 1 {
		contentHeight = 1
	}

	mainContentWidth = mainWidth - 2
	mainContentHeight = contentHeight - 2

	nextContentWidth := readingContentWidth(mainWidth)
	if reflow {
		app.contentWidth = nextContentWidth
	}
	if reflow && app.reader != nil {
		app.reader.Reflow(app.contentWidth)
	}

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

func compactReadingUI() bool {
	return app != nil && app.compactMode && app.mode == modeReading && !app.bossKey
}
