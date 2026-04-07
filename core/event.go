package core

import (
	"strings"

	"github.com/lvshp/ReadCLI/lib"
)

func handleHomeEvent(id string) {
	switch id {
	case "q", "<C-c>":
		app.quit = true
	case "j", "<C-n>", "<Down>":
		moveShelf(1)
	case "k", "<C-p>", "<Up>":
		moveShelf(-1)
	case "<Enter>", "<Right>":
		openSelectedBook()
	case "i":
		setMode(modeImportInput)
	case "o":
		cycleSort()
	case "r":
		cycleFilter()
	case "x":
		prepareDeleteSelectedBook()
	case "T":
		switchTheme()
	case "f":
		toggleBorder()
	case "u":
		triggerManualUpdateCheck()
	case "?":
		displayHelp()
	}
}

func handleReadingEvent(id string) {
	switch id {
	case "q", "<C-c>":
		syncCurrentBookState()
		app.mode = modeHome
		app.statusMessage = "已回到书架"
	case "?":
		displayHelp()
	case "p":
		displayProgress()
	case "m":
		displayTOC()
	case "f":
		toggleBorder()
	case "b":
		displayBossKey()
	case "<C-n>", "j", "<Space>", "<Enter>", "<Down>":
		if app.rowNumber == "" {
			moveReading(pageStep())
		} else {
			if num, err := lib.ParseRowNum(app.rowNumber); err != nil {
				app.statusMessage = err.Error()
			} else {
				moveReading(num)
			}
			app.rowNumber = ""
		}
	case "<C-p>", "k", "<Up>":
		if app.rowNumber == "" {
			moveReading(-pageStep())
		} else {
			if num, err := lib.ParseRowNum(app.rowNumber); err != nil {
				app.statusMessage = err.Error()
			} else {
				moveReading(1 - num)
			}
			app.rowNumber = ""
		}
	case "[", "<Left>":
		app.reader.PrevChapter()
		syncCurrentBookState()
	case "]", "<Right>":
		app.reader.NextChapter()
		syncCurrentBookState()
	case "+", "=":
		setDisplayLines(app.displayLines + 1)
	case "-", "_":
		setDisplayLines(app.displayLines - 1)
	case "c":
		cycleReadingColorPreset()
	case "t":
		toggleTimer()
	case "/":
		setMode(modeSearchInput)
	case ",":
		openReadingSettings()
	case "u":
		triggerManualUpdateCheck()
	case "n":
		jumpSearch(true)
	case "N":
		jumpSearch(false)
	case "s":
		saveBookmark()
	case "B":
		openBookmarks()
	case "T":
		switchTheme()
	case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
		app.rowNumber += id
		app.statusMessage = "跳转输入: " + app.rowNumber
	}
}

func handleReadingSettingsEvent(id string) {
	switch id {
	case "<Escape>", "q":
		app.mode = modeReading
	case "j", "<Down>":
		moveReadingSettings(1)
	case "k", "<Up>":
		moveReadingSettings(-1)
	case "h", "<Left>":
		adjustReadingSetting(-1)
	case "l", "<Right>":
		adjustReadingSetting(1)
	case "<Enter>":
		activateReadingSetting()
	}
}

func handleTOCEvent(id string) {
	switch id {
	case "q", "<C-c>":
		app.mode = modeHome
	case "m", "<Left>":
		app.mode = modeReading
	case "j", "<C-n>", "<Down>":
		updateTOCSelection(1)
	case "k", "<C-p>", "<Up>":
		updateTOCSelection(-1)
	case "<Enter>", "<Right>":
		openSelectedTOCChapter()
	case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
		appendTOCNumber(id)
	}
}

func handleBookmarkEvent(id string) {
	switch id {
	case "q", "B", "<Left>":
		app.mode = modeReading
	case "j", "<C-n>", "<Down>":
		moveBookmarks(1)
	case "k", "<C-p>", "<Up>":
		moveBookmarks(-1)
	case "<Enter>", "<Right>":
		openSelectedBookmark()
	case "d":
		deleteSelectedBookmark()
	}
}

func handleTextInputEvent(id string, onEnter func()) {
	switch id {
	case "<Escape>":
		if app.mode == modeReadingColorInput {
			app.mode = modeReadingSettings
		} else if app.currentFile != "" {
			app.mode = modeReading
		} else {
			app.mode = modeHome
		}
		resetInputState()
		app.statusMessage = "已取消输入"
	case "<Backspace>", "<Backspace2>":
		deleteInputBackward()
	case "<Delete>":
		deleteInputForward()
	case "<Left>":
		moveInputCursor(-1)
	case "<Right>":
		moveInputCursor(1)
	case "<Up>":
		if app.mode == modeImportInput {
			moveInputHint(-1)
		}
	case "<Down>":
		if app.mode == modeImportInput {
			moveInputHint(1)
		}
	case "<Home>":
		setInputCursor(0)
	case "<End>":
		setInputCursor(len([]rune(app.inputValue)))
	case "<Tab>":
		if app.mode == modeImportInput {
			completeImportPath()
		}
	case "<C-r>":
		if app.mode == modeImportInput {
			toggleImportRecursive()
		}
	case "<Enter>":
		if app.mode == modeImportInput && acceptSelectedImportHint() {
			return
		}
		onEnter()
	default:
		if isPrintableInput(id) {
			insertInputText(id)
		}
	}
}

func handleDeleteConfirmEvent(id string) {
	switch id {
	case "<Escape>", "q":
		app.mode = modeHome
		app.deleteTargetPath = ""
		app.deleteTargetTitle = ""
	case "y":
		removeSelectedBook(false)
	case "D":
		removeSelectedBook(true)
	}
}

func handleUpdatePromptEvent(id string) {
	switch id {
	case "y", "<Enter>":
		startUpdateInstall()
	case "n", "q", "<Escape>":
		if !app.updatePromptManual && app.updateRelease != nil && app.config != nil {
			app.config.SkippedUpdateVersion = strings.TrimSpace(app.updateRelease.TagName)
			_ = lib.SaveConfig(app.config)
		}
		app.mode = app.updateReturnMode
		if app.updatePromptManual {
			app.statusMessage = "已取消本次更新"
		} else {
			app.statusMessage = "该版本已忽略，之后将不再自动提醒"
		}
	}
}

func scrollUpdatePrompt(id string) bool {
	switch id {
	case "j", "<Down>", " ", "<Ctrl-n>":
		row, col := main.GetScrollOffset()
		main.ScrollTo(row+1, col)
		return true
	case "k", "<Up>", "<Ctrl-p>":
		row, col := main.GetScrollOffset()
		main.ScrollTo(row-1, col)
		return true
	case "<Ctrl-d>", "<PageDown>":
		row, col := main.GetScrollOffset()
		main.ScrollTo(row+mainContentHeight-2, col)
		return true
	case "<Ctrl-u>", "<PageUp>":
		row, col := main.GetScrollOffset()
		main.ScrollTo(row-mainContentHeight+2, col)
		return true
	case "G":
		main.ScrollToEnd()
		return true
	case "g":
		main.ScrollToBeginning()
		return true
	}
	return false
}

func handleUpdatingEvent(id string) {
	switch id {
	case "q", "<C-c>":
		app.statusMessage = "更新进行中，请稍候"
	}
}

func handleUpdateRestartEvent(id string) {
	switch id {
	case "<Enter>", "q", "<C-c>":
		app.quit = true
	}
}

func isPrintableInput(id string) bool {
	if strings.HasPrefix(id, "<") {
		return false
	}
	return len([]rune(id)) == 1
}
