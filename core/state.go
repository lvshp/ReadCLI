package core

import (
	"regexp"
	"time"

	tcell "github.com/gdamore/tcell/v2"
	"github.com/lvshp/ReadCLI/lib"
	"github.com/lvshp/ReadCLI/reader"
	"github.com/rivo/tview"
)

type mode string

const (
	modeHome                 mode = "home"
	modeReading              mode = "reading"
	modeTOC                  mode = "toc"
	modeBookmarks            mode = "bookmarks"
	modeSearchInput          mode = "search_input"
	modeBookshelfSearchInput mode = "bookshelf_search_input"
	modeReadingJumpInput     mode = "reading_jump_input"
	modeImportInput          mode = "import_input"
	modeReadingSettings      mode = "reading_settings"
	modeReadingColorInput    mode = "reading_color_input"
	modeDeleteConfirm        mode = "delete_confirm"
	modeUpdatePrompt         mode = "update_prompt"
	modeUpdating             mode = "updating"
	modeUpdateRestart        mode = "update_restart"
)

type updateMessageKind string

const (
	updateAvailable updateMessageKind = "available"
	updateInstalled updateMessageKind = "installed"
	updateFailed    updateMessageKind = "failed"
	updateCurrent   updateMessageKind = "current"
	updateProgress  updateMessageKind = "progress"
)

type updateMessage struct {
	Kind     updateMessageKind
	Release  *lib.ReleaseInfo
	Progress lib.UpdateProgress
	Err      error
	Manual   bool
}

type statusKind string

const (
	statusInfo       statusKind = "info"
	statusError      statusKind = "error"
	statusProgress   statusKind = "progress"
	statusPersistent statusKind = "persistent"
)

type theme struct {
	Name       string
	HeaderName string
	RepoName   string
	Branch     string
	Tabs       []string
	HeaderTint tcell.Color
	Accent     tcell.Color
	SideAccent tcell.Color
	FooterTag  string
	HomeName   string
	LeftName   string
	RightName  string
}

type bookshelfState struct {
	sortMode          string
	filterMode        string
	shelfIndex        int
	query             string
	deleteTargetPath  string
	deleteTargetTitle string
	loadingBookPath   string
	lastHomePath      string
}

type readingState struct {
	bookmarkIndex        int
	tocIndex             int
	tocNumber            string
	searchQuery          string
	lastSearchIndex      int
	showProgress         bool
	showHelp             bool
	showReadingQuickHelp bool
	compactMode          bool
	displayLines         int
	color                int
	timer                bool
	ticker               *time.Ticker
	rowNumber            string
	settingsIndex        int
	contentWidth         int
}

type updateState struct {
	release      *lib.ReleaseInfo
	returnMode   mode
	messages     chan updateMessage
	progress     lib.UpdateProgress
	promptManual bool
}

type inputState struct {
	value           string
	cursor          int
	hints           []string
	hintIndex       int
	importRecursive bool
}

type uiState struct {
	input                   inputState
	statusMessage           string
	statusMessageKind       statusKind
	lastStatusMessage       string
	lastStatusMessageKind   statusKind
	statusMessageUntil      time.Time
	statusMessageGeneration int
	sessionStart            time.Time
	showBorder              bool
}

type appState struct {
	mode mode

	reader      reader.Reader
	currentFile string
	currentBook *lib.BookshelfBook
	readerCache map[string]cachedReader

	config    *lib.Config
	bookshelf *lib.BookshelfStore
	bookmarks *lib.BookmarkStore
	progress  *lib.ProgressStore

	themeOrder     []string
	bookshelfState bookshelfState
	readingState   readingState
	updateState    updateState
	uiState        uiState

	bossKey        bool
	currentVersion string
	quit           bool
}

type cachedReader struct {
	reader  reader.Reader
	size    int64
	modTime time.Time
}

var (
	app    *appState
	tApp   *tview.Application
	root   *tview.Flex
	midRow *tview.Flex
	header *tview.TextView
	left   *tview.TextView
	main   *tview.TextView
	right  *tview.TextView
	footer *tview.TextView

	mainContentWidth  int
	mainContentHeight int
	lastTermWidth     int
	lastTermHeight    int
)

var titleNoiseSuffixPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\s*[\(_\[]\s*z[\s\-_]*library\s*[\)\]_]?\s*$`),
	regexp.MustCompile(`(?i)\s*[\(_\[]\s*来自\s*z[\s\-_]*library\s*[\)\]_]?\s*$`),
	regexp.MustCompile(`(?i)\s*[\(_\[]\s*downloaded\s+from\s+z[\s\-_]*library\s*[\)\]_]?\s*$`),
}
