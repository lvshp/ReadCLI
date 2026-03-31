package core

import (
	"strings"

	"github.com/TimothyYe/glance/lib"
	ui "github.com/gizak/termui/v3"
)

func handleEvents() {
	uiEvents := ui.PollEvents()
	for !app.quit {
		e := <-uiEvents

		if e.ID == "<Resize>" {
			payload := e.Payload.(ui.Resize)
			applyLayout(payload.Width, payload.Height)
			renderUI()
			continue
		}

		switch app.mode {
		case modeHome:
			handleHomeEvent(e.ID)
		case modeReading:
			handleReadingEvent(e.ID)
		case modeTOC:
			handleTOCEvent(e.ID)
		case modeBookmarks:
			handleBookmarkEvent(e.ID)
		case modeSearchInput:
			handleTextInputEvent(e.ID, runSearch)
		case modeImportInput:
			handleTextInputEvent(e.ID, importBook)
		case modeDeleteConfirm:
			handleDeleteConfirmEvent(e.ID)
		}

		renderUI()
	}
}

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
	case "G":
		if app.rowNumber == "" {
			app.reader.Last()
		} else {
			if num, err := lib.ParseRowNum(app.rowNumber); err == nil {
				app.reader.Goto(num)
			}
			app.rowNumber = ""
		}
		syncCurrentBookState()
	case "g":
		if app.rowNumber == "g" {
			app.reader.First()
			app.rowNumber = ""
			syncCurrentBookState()
		} else {
			app.rowNumber = "g"
		}
	case "+", "=":
		setDisplayLines(app.displayLines + 1)
	case "-", "_":
		setDisplayLines(app.displayLines - 1)
	case "c":
		app.color++
	case "t":
		toggleTimer()
	case "/":
		setMode(modeSearchInput)
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
		if app.currentFile != "" {
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

func isPrintableInput(id string) bool {
	if strings.HasPrefix(id, "<") {
		return false
	}
	return len([]rune(id)) == 1
}
