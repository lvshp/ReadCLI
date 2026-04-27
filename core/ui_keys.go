package core

import "github.com/gdamore/tcell/v2"

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
			if r == ' ' {
				return "<Space>"
			}
			return string(r)
		}
		return ""
	}
}
