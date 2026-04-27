package core

func queueUIUpdate(fn func()) {
	if fn == nil {
		return
	}
	if tApp == nil {
		fn()
		return
	}
	tApp.QueueUpdateDraw(fn)
}

// queueRedraw schedules a screen refresh from a goroutine.
func queueRedraw() {
	queueUIUpdate(func() {})
}
