package core

import (
	"regexp"
	"time"

	"github.com/lvshp/ReadCLI/lib"
	"github.com/lvshp/ReadCLI/reader"
	tcell "github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type mode string

const (
	modeHome              mode = "home"
	modeReading           mode = "reading"
	modeTOC               mode = "toc"
	modeBookmarks         mode = "bookmarks"
	modeSearchInput       mode = "search_input"
	modeImportInput       mode = "import_input"
	modeReadingSettings   mode = "reading_settings"
	modeReadingColorInput mode = "reading_color_input"
	modeDeleteConfirm     mode = "delete_confirm"
	modeUpdatePrompt      mode = "update_prompt"
	modeUpdating          mode = "updating"
	modeUpdateRestart     mode = "update_restart"
)

type updateMessageKind string

const (
	updateAvailable updateMessageKind = "available"
	updateInstalled updateMessageKind = "installed"
	updateFailed    updateMessageKind = "failed"
	updateCurrent   updateMessageKind = "current"
)

type updateMessage struct {
	Kind    updateMessageKind
	Release *lib.ReleaseInfo
	Err     error
	Manual  bool
}

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

	themeOrder []string
	sortMode   string
	filterMode string

	shelfIndex    int
	bookmarkIndex int
	tocIndex      int
	tocNumber     string

	inputValue      string
	inputCursor     int
	inputHints      []string
	inputHintIndex  int
	importRecursive bool
	searchQuery     string
	lastSearchIndex int

	statusMessage string
	sessionStart  time.Time
	contentWidth  int
	showBorder    bool
	showProgress  bool
	showHelp      bool
	bossKey       bool
	displayLines  int
	color         int
	timer         bool
	ticker        *time.Ticker
	rowNumber     string
	settingsIndex int

	deleteTargetPath   string
	deleteTargetTitle  string
	loadingBookPath    string
	currentVersion     string
	updateRelease      *lib.ReleaseInfo
	updateReturnMode   mode
	updateMessages     chan updateMessage
	updatePromptManual bool

	lastHomePath string
	quit         bool
}

type cachedReader struct {
	reader reader.Reader
}

var (
	app     *appState
	tApp    *tview.Application
	root    *tview.Flex
	midRow  *tview.Flex
	header  *tview.TextView
	left    *tview.TextView
	main    *tview.TextView
	right   *tview.TextView
	footer  *tview.TextView

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
