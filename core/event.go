package core

import (
	"strings"

	"github.com/lvshp/ReadCLI/lib"
)

func handleHomeEvent(id string) {
	switch id {
	case "q", "<C-c>":
		app.quit = true
	case "<Escape>":
		cancelBookshelfSearch()
	case "j", "<C-n>", "<Down>":
		moveShelf(1)
	case "k", "<C-p>", "<Up>":
		moveShelf(-1)
	case "<Enter>", "<Right>":
		openSelectedBook()
	case "i":
		transitionTo(modeImportInput)
	case "/":
		startBookshelfSearch()
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
		transitionTo(modeHome)
		setStatus(statusInfo, "已回到书架")
	case "?":
		displayReadingQuickHelp()
	case "p":
		displayProgress()
	case "m":
		displayTOC()
	case "f":
		toggleBorder()
	case "z":
		toggleCompactMode()
	case "b":
		displayBossKey()
	case "<C-n>", "j", "<Space>", "<Enter>", "<Down>":
		if app.readingState.rowNumber == "" {
			moveReading(pageStep())
		} else {
			if num, err := lib.ParseRowNum(app.readingState.rowNumber); err != nil {
				setStatus(statusError, err.Error())
			} else {
				moveReading(num)
			}
			app.readingState.rowNumber = ""
		}
	case "<C-p>", "k", "<Up>":
		if app.readingState.rowNumber == "" {
			moveReading(-pageStep())
		} else {
			if num, err := lib.ParseRowNum(app.readingState.rowNumber); err != nil {
				setStatus(statusError, err.Error())
			} else {
				moveReading(1 - num)
			}
			app.readingState.rowNumber = ""
		}
	case "[", "<Left>":
		app.reader.PrevChapter()
		syncCurrentBookState()
	case "]", "<Right>":
		app.reader.NextChapter()
		syncCurrentBookState()
	case "+", "=":
		setDisplayLines(app.readingState.displayLines + 1)
	case "-", "_":
		setDisplayLines(app.readingState.displayLines - 1)
	case "c":
		cycleReadingColorPreset()
	case "a":
		cycleReadingAlignment(1)
	case "t":
		toggleTimer()
	case "/":
		transitionTo(modeSearchInput)
	case "g":
		startReadingJumpInput()
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
		app.readingState.rowNumber += id
		setStatus(statusInfo, "跳转输入: "+app.readingState.rowNumber)
	}
}

func handleReadingSettingsEvent(id string) {
	switch id {
	case "<Escape>", "q":
		transitionTo(modeReading)
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
		transitionTo(modeHome)
	case "m", "<Left>":
		transitionTo(modeReading)
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
		transitionTo(modeReading)
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
	originalMode := app.mode
	switch id {
	case "<Escape>":
		if app.mode == modeReadingColorInput {
			transitionTo(modeReadingSettings)
		} else if app.mode == modeBookshelfSearchInput {
			app.bookshelfState.query = ""
			app.bookshelfState.shelfIndex = 0
			transitionTo(modeHome)
			setStatus(statusInfo, "书架搜索已取消")
		} else if app.mode == modeReadingJumpInput {
			transitionTo(modeReading)
			setStatus(statusInfo, "已取消跳转")
		} else if app.currentFile != "" {
			transitionTo(modeReading)
		} else {
			transitionTo(modeHome)
		}
		if originalMode != modeBookshelfSearchInput && originalMode != modeReadingJumpInput {
			setStatus(statusInfo, "已取消输入")
		}
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
		setInputCursor(len([]rune(app.uiState.input.value)))
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
		transitionTo(modeHome)
		app.bookshelfState.deleteTargetPath = ""
		app.bookshelfState.deleteTargetTitle = ""
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
		if !app.updateState.promptManual && app.updateState.release != nil && app.config != nil {
			app.config.SkippedUpdateVersion = strings.TrimSpace(app.updateState.release.TagName)
			saveConfig("保存配置")
		}
		transitionTo(app.updateState.returnMode)
		if app.updateState.promptManual {
			setStatus(statusInfo, "已取消本次更新")
		} else {
			setStatus(statusInfo, "该版本已忽略，之后将不再自动提醒")
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
		setStatus(statusProgress, "更新进行中，请稍候")
	}
}

func handleUpdateRestartEvent(id string) {
	switch id {
	case "<Enter>", "q", "<C-c>":
		app.quit = true
	}
}

func dispatchEvent(id string) {
	switch {
	case isInputMode(app.mode):
		dispatchInputEvent(id)
	case isUpdateMode(app.mode):
		dispatchUpdateEvent(id)
	default:
		dispatchSceneEvent(id)
	}
}

func dispatchSceneEvent(id string) {
	switch app.mode {
	case modeHome:
		handleHomeEvent(id)
	case modeReading:
		handleReadingEvent(id)
	case modeTOC:
		handleTOCEvent(id)
	case modeBookmarks:
		handleBookmarkEvent(id)
	case modeReadingSettings:
		handleReadingSettingsEvent(id)
	case modeDeleteConfirm:
		handleDeleteConfirmEvent(id)
	}
}

func dispatchInputEvent(id string) {
	switch app.mode {
	case modeSearchInput:
		handleTextInputEvent(id, runSearch)
	case modeBookshelfSearchInput:
		handleTextInputEvent(id, runBookshelfSearch)
	case modeReadingJumpInput:
		handleTextInputEvent(id, runReadingJump)
	case modeImportInput:
		handleTextInputEvent(id, importBook)
	case modeReadingColorInput:
		handleTextInputEvent(id, applyReadingTextColorInput)
	}
}

func dispatchUpdateEvent(id string) {
	switch app.mode {
	case modeUpdatePrompt:
		if !scrollUpdatePrompt(id) {
			handleUpdatePromptEvent(id)
		}
	case modeUpdating:
		handleUpdatingEvent(id)
	case modeUpdateRestart:
		handleUpdateRestartEvent(id)
	case modeUpdateError:
		handleUpdateErrorEvent(id)
	}
}

func isInputMode(m mode) bool {
	switch m {
	case modeSearchInput, modeBookshelfSearchInput, modeReadingJumpInput, modeImportInput, modeReadingColorInput:
		return true
	default:
		return false
	}
}

func isUpdateMode(m mode) bool {
	switch m {
	case modeUpdatePrompt, modeUpdating, modeUpdateRestart, modeUpdateError:
		return true
	default:
		return false
	}
}

func handleUpdateErrorEvent(id string) {
	switch id {
	case "<Escape>", "q", "<Enter>":
		transitionTo(app.updateState.returnMode)
		clearStatus()
	case "<C-c>":
		app.quit = true
	case "j", "<Down>", "<C-n>":
		moveUpdateErrors(1)
	case "k", "<Up>", "<C-p>":
		moveUpdateErrors(-1)
	case "<Space>", "<PageDown>":
		moveUpdateErrors(updateErrorPageSize())
	case "<PageUp>":
		moveUpdateErrors(-updateErrorPageSize())
	case "g", "<Home>":
		app.updateState.errorScroll = 0
	case "G", "<End>":
		moveUpdateErrors(len(updateErrorLines()))
	}
}

func isPrintableInput(id string) bool {
	if strings.HasPrefix(id, "<") {
		return false
	}
	return len([]rune(id)) == 1
}
