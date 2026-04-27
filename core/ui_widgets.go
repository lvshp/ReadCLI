package core

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

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
