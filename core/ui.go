package core

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/TimothyYe/glance/lib"
	"github.com/TimothyYe/glance/reader"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/mattn/go-runewidth"
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
	HeaderTint ui.Color
	Accent     ui.Color
	SideAccent ui.Color
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
	reader   reader.Reader
	modStamp int64
}

type persistentReaderCache struct {
	ModStamp int64           `json:"mod_stamp"`
	Snapshot reader.Snapshot `json:"snapshot"`
}

var (
	app        *appState
	header     *widgets.Paragraph
	leftPanel  *widgets.Paragraph
	mainPanel  *widgets.Paragraph
	rightPanel *widgets.Paragraph
	footer     *widgets.Paragraph
)

var titleNoiseSuffixPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\s*[\(_\[]\s*z[\s\-_]*library\s*[\)\]_]?\s*$`),
	regexp.MustCompile(`(?i)\s*[\(_\[]\s*来自\s*z[\s\-_]*library\s*[\)\]_]?\s*$`),
	regexp.MustCompile(`(?i)\s*[\(_\[]\s*downloaded\s+from\s+z[\s\-_]*library\s*[\)\]_]?\s*$`),
}

func availableThemes() map[string]theme {
	return map[string]theme{
		"vscode": {
			Name:       "vscode",
			HeaderName: "VS Code",
			RepoName:   "ops-console",
			Branch:     "main",
			Tabs:       []string{"explorer", "issues", "terminal", "notes", "deploy", "review"},
			HeaderTint: ui.ColorCyan,
			Accent:     ui.ColorYellow,
			SideAccent: ui.ColorGreen,
			FooterTag:  "NORMAL",
			HomeName:   "bookshelf",
			LeftName:   "explorer",
			RightName:  "inspector",
		},
		"jetbrains": {
			Name:       "jetbrains",
			HeaderName: "GoLand",
			RepoName:   "workspace",
			Branch:     "sprint/readflow",
			Tabs:       []string{"project", "structure", "problems", "terminal", "services"},
			HeaderTint: ui.ColorYellow,
			Accent:     ui.ColorCyan,
			SideAccent: ui.ColorMagenta,
			FooterTag:  "SMART",
			HomeName:   "bookshelf",
			LeftName:   "explorer",
			RightName:  "inspector",
		},
		"ops-console": {
			Name:       "ops-console",
			HeaderName: "Control Center",
			RepoName:   "internal-dashboard",
			Branch:     "ops/quiet-shift",
			Tabs:       []string{"queue", "alerts", "output", "jobs", "audit"},
			HeaderTint: ui.ColorGreen,
			Accent:     ui.ColorRed,
			SideAccent: ui.ColorCyan,
			FooterTag:  "WATCH",
			HomeName:   "bookshelf",
			LeftName:   "explorer",
			RightName:  "inspector",
		},
	}
}

func currentTheme() theme {
	themes := availableThemes()
	if t, ok := themes[app.config.Theme]; ok {
		return t
	}
	return themes["vscode"]
}

func Run(initialFile string, requestedLines int, version string) {
	cfg, _ := lib.LoadConfig()
	shelf, _ := lib.LoadBookshelf()
	marks, _ := lib.LoadBookmarks()
	progress, _ := lib.LoadProgress()

	if cfg == nil {
		cfg = &lib.Config{Theme: "vscode", DisplayLines: 8, ShowBorder: true}
	}
	if requestedLines > 0 {
		cfg.DisplayLines = requestedLines
	}
	if cfg.DisplayLines < 1 {
		cfg.DisplayLines = 8
	}
	if cfg.Theme == "" {
		cfg.Theme = "vscode"
	}

	app = &appState{
		mode:            modeHome,
		config:          cfg,
		bookshelf:       shelf,
		bookmarks:       marks,
		progress:        progress,
		readerCache:     map[string]cachedReader{},
		themeOrder:      []string{"vscode", "jetbrains", "ops-console"},
		sortMode:        "recent",
		filterMode:      "all",
		importRecursive: false,
		shelfIndex:      cfg.SelectedBookshelf,
		lastSearchIndex: -1,
		sessionStart:    time.Now(),
		showBorder:      cfg.ShowBorder,
		displayLines:    cfg.DisplayLines,
		currentVersion:  strings.TrimSpace(version),
		updateMessages:  make(chan updateMessage, 2),
	}

	if app.bookshelf == nil {
		app.bookshelf = &lib.BookshelfStore{}
	}
	if app.bookmarks == nil {
		app.bookmarks = &lib.BookmarkStore{Books: map[string][]lib.Bookmark{}}
	}
	if app.progress == nil {
		app.progress = &lib.ProgressStore{Books: map[string]int{}}
	}
	if app.bookmarks.Books == nil {
		app.bookmarks.Books = map[string][]lib.Bookmark{}
	}

	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()
	defer persistState()

	initWidgets()
	applyLayoutFromTerminal()

	if initialFile != "" {
		if err := openBook(initialFile); err != nil {
			app.statusMessage = err.Error()
			app.mode = modeHome
		}
	}

	startUpdateCheck(false)

	renderUI()
	handleEvents()
}

func initWidgets() {
	header = widgets.NewParagraph()
	leftPanel = widgets.NewParagraph()
	mainPanel = widgets.NewParagraph()
	rightPanel = widgets.NewParagraph()
	footer = widgets.NewParagraph()

	for _, w := range []*widgets.Paragraph{header, leftPanel, mainPanel, rightPanel, footer} {
		w.TextStyle.Fg = ui.ColorWhite
		w.Border = app.showBorder
	}
	header.BorderStyle.Fg = ui.ColorBlue
	leftPanel.BorderStyle.Fg = ui.ColorBlue
	mainPanel.BorderStyle.Fg = ui.ColorCyan
	rightPanel.BorderStyle.Fg = ui.ColorBlue
	footer.BorderStyle.Fg = ui.ColorBlue
	leftPanel.TitleStyle.Fg = ui.ColorCyan
	mainPanel.TitleStyle.Fg = ui.ColorCyan
	rightPanel.TitleStyle.Fg = ui.ColorCyan
	footer.TitleStyle.Fg = ui.ColorCyan
}

func currentReadingTextColor() ui.Color {
	if app == nil {
		return ui.ColorWhite
	}
	if app.config != nil {
		if color, ok := parseConfiguredUIColor(app.config.ReadingTextColor); ok {
			return color
		}
		if app.config.ReadingHighContrast {
			return ui.ColorWhite
		}
	}
	return ui.ColorWhite
}

func parseConfiguredUIColor(value string) (ui.Color, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return ui.ColorWhite, false
	}
	if strings.HasPrefix(value, "#") {
		r, g, b, ok := parseHexColor(value)
		if !ok {
			return ui.ColorWhite, false
		}
		return rgbToXtermColor(r, g, b), true
	}
	parts := strings.Split(value, ",")
	if len(parts) != 3 {
		return ui.ColorWhite, false
	}
	values := make([]int, 0, 3)
	for _, part := range parts {
		num, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || num < 0 || num > 255 {
			return ui.ColorWhite, false
		}
		values = append(values, num)
	}
	return rgbToXtermColor(values[0], values[1], values[2]), true
}

func parseHexColor(value string) (int, int, int, bool) {
	hex := strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(hex) == 3 {
		hex = strings.Repeat(string(hex[0]), 2) + strings.Repeat(string(hex[1]), 2) + strings.Repeat(string(hex[2]), 2)
	}
	if len(hex) != 6 {
		return 0, 0, 0, false
	}
	num, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int((num >> 16) & 0xFF), int((num >> 8) & 0xFF), int(num & 0xFF), true
}

func rgbToXtermColor(r, g, b int) ui.Color {
	if !terminalSupports256Color() {
		return rgbToANSIColor(r, g, b)
	}
	if r == g && g == b {
		if r < 8 {
			return ui.Color(16)
		}
		if r > 248 {
			return ui.Color(231)
		}
		return ui.Color(232 + ((r-8)*24)/247)
	}
	rc := int(float64(r) / 255 * 5)
	gc := int(float64(g) / 255 * 5)
	bc := int(float64(b) / 255 * 5)
	return ui.Color(16 + 36*rc + 6*gc + bc)
}

func terminalSupports256Color() bool {
	if app != nil && app.config != nil && app.config.ForceBasicColor {
		return false
	}

	term := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))
	colorTerm := strings.ToLower(strings.TrimSpace(os.Getenv("COLORTERM")))

	if strings.Contains(term, "256color") || strings.Contains(term, "direct") {
		return true
	}
	if strings.Contains(colorTerm, "truecolor") || strings.Contains(colorTerm, "24bit") {
		return true
	}
	switch {
	case strings.Contains(term, "xterm"),
		strings.Contains(term, "screen"),
		strings.Contains(term, "tmux"),
		strings.Contains(term, "rxvt"),
		strings.Contains(term, "linux"):
		return true
	default:
		return false
	}
}

func rgbToANSIColor(r, g, b int) ui.Color {
	type ansiColor struct {
		color ui.Color
		r     int
		g     int
		b     int
	}

	palette := []ansiColor{
		{ui.ColorBlack, 0, 0, 0},
		{ui.ColorRed, 205, 49, 49},
		{ui.ColorGreen, 13, 188, 121},
		{ui.ColorYellow, 229, 229, 16},
		{ui.ColorBlue, 36, 114, 200},
		{ui.ColorMagenta, 188, 63, 188},
		{ui.ColorCyan, 17, 168, 205},
		{ui.ColorWhite, 229, 229, 229},
	}

	best := palette[0].color
	bestDistance := colorDistanceSquared(r, g, b, palette[0].r, palette[0].g, palette[0].b)
	for _, candidate := range palette[1:] {
		distance := colorDistanceSquared(r, g, b, candidate.r, candidate.g, candidate.b)
		if distance < bestDistance {
			bestDistance = distance
			best = candidate.color
		}
	}
	return best
}

func colorDistanceSquared(r1, g1, b1, r2, g2, b2 int) int {
	dr := r1 - r2
	dg := g1 - g2
	db := b1 - b2
	return dr*dr + dg*dg + db*db
}

func readingWidthRatio() float64 {
	if app == nil || app.config == nil || app.config.ReadingContentWidthRatio <= 0 || app.config.ReadingContentWidthRatio > 1 {
		return 0.75
	}
	return app.config.ReadingContentWidthRatio
}

func readingMarginLeft() int {
	if app == nil || app.config == nil || app.config.ReadingMarginLeft < 0 {
		return 2
	}
	return app.config.ReadingMarginLeft
}

func readingMarginRight() int {
	if app == nil || app.config == nil || app.config.ReadingMarginRight < 0 {
		return 0
	}
	return app.config.ReadingMarginRight
}

func readingMarginTop() int {
	if app == nil || app.config == nil || app.config.ReadingMarginTop < 0 {
		return 1
	}
	return app.config.ReadingMarginTop
}

func readingMarginBottom() int {
	if app == nil || app.config == nil || app.config.ReadingMarginBottom < 0 {
		return 0
	}
	return app.config.ReadingMarginBottom
}

func readingLineSpacing() int {
	if app == nil || app.config == nil || app.config.ReadingLineSpacing < 0 {
		return 1
	}
	return app.config.ReadingLineSpacing
}

func persistState() {
	if app == nil {
		return
	}
	app.config.DisplayLines = app.displayLines
	app.config.ShowBorder = app.showBorder
	app.config.SelectedBookshelf = app.shelfIndex
	_ = lib.SaveConfig(app.config)
	_ = lib.SaveBookshelf(app.bookshelf)
	_ = lib.SaveBookmarks(app.bookmarks)
	_ = lib.SaveProgress(app.progress)
}

func openBook(path string) error {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}

	r, _, err := cachedReaderForPath(path)
	if err != nil {
		return err
	}

	app.reader = r
	app.currentFile = path
	app.currentBook = nil
	app.showHelp = false
	app.showProgress = false
	app.rowNumber = ""
	app.searchQuery = ""
	app.inputValue = ""
	app.inputCursor = 0
	app.inputHints = nil
	app.inputHintIndex = 0
	app.lastSearchIndex = -1
	app.tocNumber = ""
	app.mode = modeReading

	var savedAnchor reader.ProgressAnchor
	hasSavedAnchor := false
	legacyChapterTitle := ""

	if book, ok := lib.FindBookshelfBook(app.bookshelf, path); ok {
		app.currentBook = &book
		legacyChapterTitle = strings.TrimSpace(book.CurrentChapter)
		if book.ChapterIndex > 0 || book.ChapterOffset > 0 || (book.ProgressPos == 0 && legacyChapterTitle == "") {
			savedAnchor = reader.ProgressAnchor{
				Pos:           book.ProgressPos,
				ChapterIndex:  book.ChapterIndex,
				ChapterOffset: book.ChapterOffset,
			}
			hasSavedAnchor = true
		}
	}
	if !hasSavedAnchor {
		if anchor, ok := app.progress.Anchors[path]; ok {
			savedAnchor = reader.ProgressAnchor{
				Pos:           anchor.Pos,
				ChapterIndex:  anchor.ChapterIndex,
				ChapterOffset: anchor.ChapterOffset,
				OverallRatio:  anchor.OverallRatio,
			}
			hasSavedAnchor = true
		} else if pos, ok := app.progress.Books[path]; ok {
			savedAnchor = reader.ProgressAnchor{Pos: pos}
			hasSavedAnchor = true
		}
	}
	mainPanel.Title = " editor: " + filepath.Base(path) + " "
	applyLayoutFromTerminal()
	if hasSavedAnchor {
		reader.RestoreFromAnchor(app.reader, savedAnchor)
	} else if legacyChapterTitle != "" {
		if chapterIndex := reader.FindChapterIndexByTitle(app.reader, legacyChapterTitle); chapterIndex >= 0 {
			reader.RestoreFromAnchor(app.reader, reader.ProgressAnchor{
				ChapterIndex:  chapterIndex,
				ChapterOffset: 0,
				Pos:           0,
			})
		}
	}
	app.currentBook = upsertCurrentBook(path)
	app.statusMessage = "已打开 " + filepath.Base(path)
	return nil
}

func cachedReaderForPath(path string) (reader.Reader, bool, error) {
	modStamp := fileModStamp(path)
	if app != nil && app.readerCache != nil {
		if cached, ok := app.readerCache[path]; ok && cached.reader != nil && cached.modStamp == modStamp {
			return cached.reader, true, nil
		}
	}

	if r, ok, err := loadPersistentCachedReader(path, modStamp); err == nil && ok {
		if app != nil {
			if app.readerCache == nil {
				app.readerCache = map[string]cachedReader{}
			}
			app.readerCache[path] = cachedReader{reader: r, modStamp: modStamp}
		}
		return r, true, nil
	}

	r, err := newReaderForPath(path)
	if err != nil {
		return nil, false, err
	}
	if err := r.Load(path); err != nil {
		return nil, false, err
	}

	if app != nil {
		if app.readerCache == nil {
			app.readerCache = map[string]cachedReader{}
		}
		app.readerCache[path] = cachedReader{reader: r, modStamp: modStamp}
	}
	_ = savePersistentReaderCache(path, modStamp, r)
	return r, false, nil
}

func fileModStamp(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixNano()
}

func loadPersistentCachedReader(path string, modStamp int64) (reader.Reader, bool, error) {
	cachePath, err := persistentCachePath(path)
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var cached persistentReaderCache
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, false, err
	}
	if cached.ModStamp != modStamp {
		return nil, false, nil
	}
	width := app.contentWidth
	if width <= 0 {
		width = 80
	}
	r, err := reader.ReaderFromSnapshot(cached.Snapshot, width)
	if err != nil {
		return nil, false, err
	}
	return r, true, nil
}

func savePersistentReaderCache(path string, modStamp int64, r reader.Reader) error {
	snapshot, ok := reader.SnapshotFromReader(r)
	if !ok || snapshot == nil {
		return nil
	}
	cachePath, err := persistentCachePath(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(persistentReaderCache{
		ModStamp: modStamp,
		Snapshot: *snapshot,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(cachePath, data, 0644)
}

func persistentCachePath(path string) (string, error) {
	dataDir, err := lib.DataDirPath()
	if err != nil {
		return "", err
	}
	sum := sha1.Sum([]byte(path))
	name := hex.EncodeToString(sum[:]) + ".json"
	return filepath.Join(dataDir, "cache", name), nil
}

func newReaderForPath(path string) (reader.Reader, error) {
	switch strings.ToUpper(filepath.Ext(path)) {
	case ".TXT":
		return reader.NewTxtReader(), nil
	case ".EPUB":
		return reader.NewEpubReader(), nil
	default:
		return nil, fmt.Errorf("unsupported file format: %s", filepath.Ext(path))
	}
}

func upsertCurrentBook(path string) *lib.BookshelfBook {
	anchor := reader.AnchorFromReader(app.reader)
	book := lib.BookshelfBook{
		Path:            path,
		Title:           bookTitleForPath(path, app.reader.BookTitle()),
		Format:          strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."),
		ProgressPos:     anchor.Pos,
		ProgressTotal:   app.reader.Total(),
		ProgressPercent: progressPercent(anchor.Pos, app.reader.Total()),
		CurrentChapter:  app.reader.CurrentChapterTitle(),
		ChapterIndex:    anchor.ChapterIndex,
		ChapterOffset:   anchor.ChapterOffset,
		LastReadAt:      time.Now().Format(time.RFC3339),
	}
	if existing, ok := lib.FindBookshelfBook(app.bookshelf, path); ok {
		book.ImportedAt = existing.ImportedAt
	}
	lib.UpsertBookshelfBook(app.bookshelf, book)
	_ = lib.SaveBookshelf(app.bookshelf)
	for i := range app.bookshelf.Books {
		if app.bookshelf.Books[i].Path == path {
			return &app.bookshelf.Books[i]
		}
	}
	return nil
}

func syncCurrentBookState() {
	if app == nil || app.reader == nil || app.currentFile == "" {
		return
	}

	anchor := reader.AnchorFromReader(app.reader)
	book := lib.BookshelfBook{
		Path:            app.currentFile,
		Title:           bookTitleForPath(app.currentFile, app.reader.BookTitle()),
		Format:          strings.TrimPrefix(strings.ToLower(filepath.Ext(app.currentFile)), "."),
		ProgressPos:     anchor.Pos,
		ProgressTotal:   app.reader.Total(),
		ProgressPercent: progressPercent(anchor.Pos, app.reader.Total()),
		CurrentChapter:  app.reader.CurrentChapterTitle(),
		ChapterIndex:    anchor.ChapterIndex,
		ChapterOffset:   anchor.ChapterOffset,
		LastReadAt:      time.Now().Format(time.RFC3339),
	}
	if existing, ok := lib.FindBookshelfBook(app.bookshelf, app.currentFile); ok {
		book.ImportedAt = existing.ImportedAt
	}
	lib.UpsertBookshelfBook(app.bookshelf, book)
	app.progress.Books[app.currentFile] = anchor.Pos
	if app.progress.Anchors == nil {
		app.progress.Anchors = map[string]lib.ProgressAnchor{}
	}
	app.progress.Anchors[app.currentFile] = lib.ProgressAnchor{
		Pos:           anchor.Pos,
		ChapterIndex:  anchor.ChapterIndex,
		ChapterOffset: anchor.ChapterOffset,
		OverallRatio:  anchor.OverallRatio,
	}
	_ = lib.SaveBookshelf(app.bookshelf)
	_ = lib.SaveProgress(app.progress)
}

func progressPercent(pos, total int) int {
	if total <= 1 {
		return 0
	}
	if pos >= total-1 {
		return 100
	}
	if pos <= 0 {
		return 0
	}
	return int(float64(pos) * 100 / float64(total-1))
}

func bookTitleForPath(path, preferred string) string {
	preferred = sanitizeBookTitle(preferred)
	if preferred != "" {
		return preferred
	}
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func sanitizeBookTitle(title string) string {
	title = strings.TrimSpace(title)
	title = strings.Join(strings.Fields(title), " ")
	title = stripKnownTitleNoise(title)
	title = strings.TrimSpace(title)
	title = strings.Trim(title, "[]【】")
	title = stripKnownTitleNoise(title)
	if title == "" {
		return ""
	}
	return title
}

func stripKnownTitleNoise(title string) string {
	cleaned := strings.TrimSpace(title)
	for _, pattern := range titleNoiseSuffixPatterns {
		cleaned = pattern.ReplaceAllString(cleaned, "")
		cleaned = strings.TrimSpace(cleaned)
	}
	return cleaned
}

func visibleBooks() []lib.BookshelfBook {
	books := lib.FilterBooks(app.bookshelf.Books, app.filterMode)
	lib.SortBooks(books, app.sortMode)
	return books
}

func selectedBook() *lib.BookshelfBook {
	books := visibleBooks()
	if len(books) == 0 {
		return nil
	}
	if app.shelfIndex < 0 {
		app.shelfIndex = 0
	}
	if app.shelfIndex >= len(books) {
		app.shelfIndex = len(books) - 1
	}
	book := books[app.shelfIndex]
	return &book
}

func bookmarksForCurrentBook() []lib.Bookmark {
	if app.currentFile == "" {
		return nil
	}
	return app.bookmarks.Books[app.currentFile]
}

func applyLayoutFromTerminal() {
	w, h := ui.TerminalDimensions()
	applyLayout(w, h)
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

	headerHeight := 4
	footerHeight := 4
	if termHeight < 16 {
		headerHeight = 4
		footerHeight = 3
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
	} else {
		leftPanel.SetRect(0, 0, 0, 0)
	}
	if rightWidth > 0 {
		rightPanel.SetRect(width-rightWidth, contentTop, width, contentBottom)
	} else {
		rightPanel.SetRect(0, 0, 0, 0)
	}
	mainPanel.SetRect(leftWidth, contentTop, width-rightWidth, contentBottom)

	app.contentWidth = readingContentWidth(mainWidth)
	if app.reader != nil {
		app.reader.Reflow(app.contentWidth)
	}

	refreshChrome()
}

func renderUI() {
	refreshChrome()
	drawables := []ui.Drawable{header, mainPanel, footer}
	if leftPanel != nil && leftPanel.Inner.Dx() > 0 {
		drawables = append(drawables, leftPanel)
	}
	if rightPanel != nil && rightPanel.Inner.Dx() > 0 {
		drawables = append(drawables, rightPanel)
	}
	ui.Render(drawables...)
}

func startUpdateCheck(manual bool) {
	if app == nil || app.updateMessages == nil {
		return
	}
	if app.currentVersion == "" || !lib.CurrentPlatformSupported() {
		if manual {
			app.statusMessage = "当前版本不支持自动更新"
		}
		return
	}
	go func(version string, skipped string, manual bool) {
		release, err := lib.FetchLatestRelease(version)
		if err != nil || release == nil {
			if manual {
				app.updateMessages <- updateMessage{
					Kind:   updateFailed,
					Err:    err,
					Manual: true,
				}
			}
			return
		}
		if !lib.ShouldOfferUpdate(version, release.TagName) {
			if manual {
				app.updateMessages <- updateMessage{
					Kind:   updateCurrent,
					Manual: true,
				}
			}
			return
		}
		if !manual && strings.TrimSpace(skipped) == strings.TrimSpace(release.TagName) {
			return
		}
		app.updateMessages <- updateMessage{
			Kind:    updateAvailable,
			Release: release,
			Manual:  manual,
		}
	}(app.currentVersion, app.config.SkippedUpdateVersion, manual)
}

func handleUpdateMessage(message updateMessage) {
	switch message.Kind {
	case updateAvailable:
		if message.Release == nil {
			return
		}
		app.updateRelease = message.Release
		app.updatePromptManual = message.Manual
		if app.mode != modeUpdating && app.mode != modeUpdateRestart {
			app.updateReturnMode = app.mode
			app.mode = modeUpdatePrompt
		}
		app.statusMessage = "发现新版本 " + message.Release.TagName
	case updateInstalled:
		if message.Release != nil {
			app.updateRelease = message.Release
		}
		if app.config != nil {
			app.config.SkippedUpdateVersion = ""
			_ = lib.SaveConfig(app.config)
		}
		app.mode = modeUpdateRestart
		app.statusMessage = "更新已安装，退出后重新启动生效"
	case updateFailed:
		app.mode = app.updateReturnMode
		if message.Err != nil {
			app.statusMessage = "更新失败: " + shorten(message.Err.Error(), 48)
		} else {
			app.statusMessage = "更新失败"
		}
	case updateCurrent:
		app.statusMessage = "当前已经是最新版本"
	}
}

func triggerManualUpdateCheck() {
	if app == nil {
		return
	}
	app.statusMessage = "正在检查更新..."
	renderUIIfReady()
	startUpdateCheck(true)
}

func startUpdateInstall() {
	if app == nil || app.updateRelease == nil || app.updateMessages == nil {
		return
	}
	executablePath, err := os.Executable()
	if err != nil {
		app.statusMessage = "无法定位当前程序"
		return
	}
	asset := lib.SelectReleaseAsset(app.updateRelease, runtime.GOOS, runtime.GOARCH)
	if asset == nil {
		app.statusMessage = "当前平台暂无可用更新包"
		return
	}

	app.mode = modeUpdating
	app.statusMessage = "正在安装更新 " + app.updateRelease.TagName
	renderUIIfReady()

	go func(version string, release *lib.ReleaseInfo, downloadURL, exePath string) {
		err := lib.InstallLatestReleaseAsset(version, downloadURL, exePath)
		if err != nil {
			app.updateMessages <- updateMessage{Kind: updateFailed, Err: err}
			return
		}
		app.updateMessages <- updateMessage{Kind: updateInstalled, Release: release}
	}(app.currentVersion, app.updateRelease, asset.BrowserDownloadURL, executablePath)
}

func renderUIIfReady() {
	if header == nil || mainPanel == nil || footer == nil {
		return
	}
	renderUI()
}

func refreshChrome() {
	if app == nil {
		return
	}
	th := currentTheme()
	if app.bossKey {
		applyBossChrome(th)
		return
	}
	header.Text = buildHeader(th)
	leftPanel.Text = buildLeftPanel(th)
	rightPanel.Text = buildRightPanel(th)
	footer.Text = buildFooter()
	mainPanel.Text = buildMainPanel()
	mainPanel.Title = buildMainTitle()
	leftPanel.Title = " " + th.LeftName + " "
	rightPanel.Title = " " + th.RightName + " "
	footer.Title = " " + strings.ToLower(th.FooterTag) + " "
	if app.mode == modeHome || app.mode == modeImportInput || app.mode == modeDeleteConfirm {
		mainPanel.Title = " " + th.HomeName + " "
	}

	mainPanel.Border = app.showBorder
	header.Border = app.showBorder
	leftPanel.Border = app.showBorder
	rightPanel.Border = app.showBorder
	footer.Border = app.showBorder
	header.BorderStyle.Fg = th.HeaderTint
	mainPanel.BorderStyle.Fg = th.Accent
	leftPanel.BorderStyle.Fg = th.SideAccent
	rightPanel.BorderStyle.Fg = th.SideAccent
	footer.BorderStyle.Fg = th.HeaderTint
	header.TextStyle.Fg = ui.ColorWhite
	leftPanel.TextStyle.Fg = ui.ColorWhite
	rightPanel.TextStyle.Fg = ui.ColorWhite
	footer.TextStyle.Fg = ui.ColorWhite
	header.TitleStyle.Fg = th.HeaderTint
	leftPanel.TitleStyle.Fg = th.SideAccent
	mainPanel.TitleStyle.Fg = th.Accent
	rightPanel.TitleStyle.Fg = th.SideAccent
	footer.TitleStyle.Fg = th.HeaderTint
	mainPanel.TextStyle.Fg = currentReadingTextColor()
}

func applyBossChrome(th theme) {
	header.Text = buildBossHeader(th)
	leftPanel.Text = buildBossLeftPanel()
	mainPanel.Text = buildBossMainPanel()
	rightPanel.Text = buildBossRightPanel()
	footer.Text = buildBossFooter()

	header.Border = app.showBorder
	mainPanel.Border = app.showBorder
	leftPanel.Border = app.showBorder
	rightPanel.Border = app.showBorder
	footer.Border = app.showBorder

	leftPanel.Title = " processes "
	mainPanel.Title = " runtime "
	rightPanel.Title = " metrics "
	footer.Title = " monitor "

	header.BorderStyle.Fg = th.HeaderTint
	mainPanel.BorderStyle.Fg = th.Accent
	leftPanel.BorderStyle.Fg = th.SideAccent
	rightPanel.BorderStyle.Fg = th.SideAccent
	footer.BorderStyle.Fg = th.HeaderTint

	header.TextStyle.Fg = ui.ColorWhite
	leftPanel.TextStyle.Fg = ui.ColorWhite
	mainPanel.TextStyle.Fg = ui.ColorWhite
	rightPanel.TextStyle.Fg = ui.ColorWhite
	footer.TextStyle.Fg = ui.ColorWhite

	header.TitleStyle.Fg = th.HeaderTint
	leftPanel.TitleStyle.Fg = th.SideAccent
	mainPanel.TitleStyle.Fg = th.Accent
	rightPanel.TitleStyle.Fg = th.SideAccent
	footer.TitleStyle.Fg = th.HeaderTint
}

func buildHeader(th theme) string {
	now := time.Now().Format("15:04")
	modeLabel := strings.ToUpper(string(app.mode))
	switch th.Name {
	case "jetbrains":
		line1 := fmt.Sprintf("[%s](fg:yellow,mod:bold)  [%s](fg:white)  run [%s](fg:cyan)  branch [%s](fg:magenta)  [%s](fg:yellow)",
			th.HeaderName,
			th.RepoName,
			shorten(currentDisplayName(), 24),
			th.Branch,
			now,
		)
		line2 := fmt.Sprintf("[%s](fg:black,bg:yellow,mod:bold)  [ project ] [ structure ] [ services ] [ problems ]  inspections [0](fg:green)  theme [%s](fg:cyan)",
			modeLabel,
			th.Name,
		)
		return line1 + "\n" + line2
	case "ops-console":
		line1 := fmt.Sprintf("[%s](fg:green,mod:bold)  cluster [%s](fg:cyan)  lane [%s](fg:yellow)  target [%s](fg:white,mod:bold)  [%s](fg:green)",
			th.HeaderName,
			th.RepoName,
			th.Branch,
			shorten(currentDisplayName(), 22),
			now,
		)
		line2 := fmt.Sprintf("[%s](fg:black,bg:green,mod:bold)  [ queue ] [ alerts ] [ jobs ] [ audit ]  incidents [0](fg:green)  theme [%s](fg:cyan)",
			modeLabel,
			th.Name,
		)
		return line1 + "\n" + line2
	default:
		line1 := fmt.Sprintf("[%s](fg:cyan,mod:bold)  [%s](fg:green)  branch [%s](fg:yellow)  [%s](fg:white,mod:bold)  [%s](fg:cyan)",
			th.HeaderName,
			th.RepoName,
			th.Branch,
			shorten(currentDisplayName(), 28),
			now,
		)
		line2 := fmt.Sprintf("[%s](fg:black,bg:green,mod:bold)  [ bookshelf ] [ search ] [ bookmarks ] [ reader ]  diagnostics [0](fg:green)  theme [%s](fg:cyan)",
			modeLabel,
			th.Name,
		)
		return line1 + "\n" + line2
	}
}

func buildBossHeader(th theme) string {
	now := time.Now().Format("15:04:05")
	host := "edge-gw-03"
	job := "batch-reconcile"
	switch th.Name {
	case "jetbrains":
		line1 := fmt.Sprintf("[%s](fg:yellow,mod:bold)  workspace [%s](fg:cyan)  run [%s](fg:green)  branch [%s](fg:magenta)  [%s](fg:yellow)",
			th.HeaderName, host, job, "hotfix/runtime", now)
		line2 := "[RUNNING](fg:black,bg:yellow,mod:bold)  [ processes ] [ logs ] [ traces ] [ queues ] [ alerts ]  inspections [0](fg:green)  state [healthy](fg:cyan)"
		return line1 + "\n" + line2
	case "ops-console":
		line1 := fmt.Sprintf("[%s](fg:green,mod:bold)  node [%s](fg:cyan)  task [%s](fg:yellow)  window [%s](fg:white,mod:bold)  [%s](fg:green)",
			th.HeaderName, host, job, "runtime-monitor", now)
		line2 := "[ACTIVE](fg:black,bg:green,mod:bold)  [ ingest ] [ workers ] [ streams ] [ snapshots ] [ audit ]  incidents [0](fg:green)  state [stable](fg:cyan)"
		return line1 + "\n" + line2
	default:
		line1 := fmt.Sprintf("[%s](fg:cyan,mod:bold)  service [%s](fg:yellow)  env [%s](fg:green)  pod [%s](fg:white,mod:bold)  [%s](fg:cyan)",
			th.HeaderName, job, "prod-sh", host, now)
		line2 := "[LIVE](fg:black,bg:cyan,mod:bold)  [ overview ] [ jobs ] [ traces ] [ tasks ] [ deployment ]  diagnostics [0](fg:green)  state [synced](fg:yellow)"
		return line1 + "\n" + line2
	}
}

func buildBossLeftPanel() string {
	return strings.Join([]string{
		"[Process Tree](fg:cyan,mod:bold)",
		"",
		"  supervisor/",
		"    scheduler/",
		"      job-dispatcher",
		"      signal-watcher",
		"    workers/",
		"      parser-01",
		"      parser-02",
		"      merge-queue",
		"    network/",
		"      stream-relay",
		"      rpc-gateway",
		"",
		"[Jobs](fg:yellow,mod:bold)",
		"  pending      03",
		"  running      12",
		"  blocked      00",
		"  retrying     01",
		"",
		"[Focus](fg:green,mod:bold)",
		"  target  nightly sync",
		"  lane    cn-sh-prod",
		"  state   processing",
	}, "\n")
}

func buildBossMainPanel() string {
	now := time.Now()
	lines := []string{
		"[runtime monitor](fg:cyan,mod:bold)",
		"",
		fmt.Sprintf("[%s](fg:green)  bootstrap completed, worker pool online", now.Add(-29*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:green)  queue snapshot refreshed, 124 batches ready", now.Add(-24*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:yellow)  parser-01 processing shard eu-west/14", now.Add(-19*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:yellow)  parser-02 processing shard ap-east/09", now.Add(-16*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:cyan)  merge-queue flushing 18 delta segments", now.Add(-12*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:white)  rpc-gateway heartbeat stable (p95 42ms)", now.Add(-9*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:green)  audit channel synced, no drift detected", now.Add(-6*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:green)  release window healthy, checksum verified", now.Add(-3*time.Second).Format("15:04:05")),
		"",
		"[active tasks](fg:yellow,mod:bold)",
		"  batch-reconcile      running      73%",
		"  metrics-rollup       running      41%",
		"  cold-storage-sync    queued       wait-io",
		"  nightly-report       standby      00:12",
		"",
		"[stream output](fg:magenta,mod:bold)",
		"  shard/eu-west/14     rows 184220   lag 0.3s",
		"  shard/ap-east/09     rows 163871   lag 0.4s",
		"  shard/cn-south/02    rows 201443   lag 0.2s",
		"",
		"[controls](fg:green,mod:bold)",
		"  b  back to reader",
		"  q  exit to shelf",
	}
	return strings.Join(lines, "\n")
}

func buildBossRightPanel() string {
	return strings.Join([]string{
		"[Runtime](fg:cyan,mod:bold)",
		"",
		"  cpu        37%",
		"  memory     2.4 GB",
		"  io wait    1.2%",
		"  net        128 MB/s",
		"",
		"[Workers](fg:yellow,mod:bold)",
		"  online     12",
		"  idle       03",
		"  errors     00",
		"",
		"[Queue](fg:green,mod:bold)",
		"  inflight   124",
		"  retry      01",
		"  backlog    08",
		"",
		"[Recent](fg:magenta,mod:bold)",
		"  parser pool healthy",
		"  storage sync ready",
		"  monitor stable",
	}, "\n")
}

func buildBossFooter() string {
	elapsed := time.Since(app.sessionStart).Round(time.Minute)
	return fmt.Sprintf("[MONITOR](fg:black,bg:green,mod:bold)  uptime [%s](fg:yellow)  state [running](fg:green)  window [runtime](fg:cyan)  [%s](fg:white)\n[Escaped view active](fg:yellow)  [b](fg:cyan):return  [q](fg:red):shelf",
		elapsed, time.Now().Format("2006-01-02 15:04:05"))
}

func buildLeftPanel(th theme) string {
	if app.mode == modeHome || app.mode == modeImportInput || app.mode == modeDeleteConfirm {
		return strings.Join([]string{
			"[Bookshelf](fg:cyan,mod:bold)",
			"",
			"[Actions](fg:yellow,mod:bold)",
			"  Enter   打开书籍",
			"  i       导入文件",
			"  o       排序视图",
			"  r       过滤视图",
			"  x       移出书架",
			"  T       切换主题",
			"  u       检查更新",
			"",
			"[Sort](fg:yellow,mod:bold)",
			"  " + readableSort(app.sortMode),
			"",
			"[Filter](fg:green,mod:bold)",
			"  " + readableFilter(app.filterMode),
			"",
			"[Theme](fg:cyan,mod:bold)",
			"  " + th.Name,
		}, "\n")
	}

	currentChapter := ""
	if app.reader != nil {
		currentChapter = app.reader.CurrentChapterTitle()
	}
	if currentChapter == "" {
		currentChapter = "Inbox"
	}
	return strings.Join([]string{
		"[Explorer](fg:cyan,mod:bold)",
		"",
		"  bookshelf/",
		"    core/",
		"    reader/",
		"    themes/",
		fmt.Sprintf("    > %s", shorten(currentDisplayName(), 14)),
		"",
		"[Actions](fg:yellow,mod:bold)",
		"  / 搜索",
		"  s 保存书签",
		"  B 打开书签",
		"  m 目录",
		"  c 切换颜色",
		"  , 阅读设置",
		"  T 切主题",
		"  u 检查更新",
		"",
		"[Current Focus](fg:green,mod:bold)",
		"  " + shorten(currentChapter, 14),
	}, "\n")
}

func buildRightPanel(th theme) string {
	if app.mode == modeHome || app.mode == modeImportInput || app.mode == modeDeleteConfirm {
		book := selectedBook()
		lines := []string{fmt.Sprintf("[%s](fg:cyan,mod:bold)", titleCase(th.RightName)), ""}
		if book == nil {
			lines = append(lines, "  书架为空", "  按 i 导入本地书籍")
		} else {
			lastRead := "未开始"
			if book.LastReadAt != "" {
				lastRead = formatStamp(book.LastReadAt)
			}
			status := "未读"
			if book.ProgressPercent >= 100 {
				status = "已读"
			} else if book.ProgressPos > 0 {
				status = "在读"
			}
			lines = append(lines,
				"  标题    "+shorten(book.Title, 16),
				"  格式    "+strings.ToUpper(book.Format),
				"  状态    "+status,
				fmt.Sprintf("  进度    %d%%", book.ProgressPercent),
				"  章节    "+shorten(book.CurrentChapter, 16),
				"  最近    "+shorten(lastRead, 16),
				"",
				"[Continue](fg:yellow,mod:bold)",
				"  回车继续阅读",
			)
		}
		lines = append(lines, "",
			"[Recent Status](fg:green,mod:bold)",
			"  home ready",
			"  import available",
			"  theme synced",
		)
		return strings.Join(lines, "\n")
	}

	progress := ""
	chapter := ""
	total := 0
	current := 0
	if app.reader != nil {
		progress = app.reader.GetProgress()
		chapter = app.reader.CurrentChapterTitle()
		total = app.reader.Total()
		current = app.reader.CurrentPos() + 1
	}
	if chapter == "" {
		chapter = "General"
	}
	width := 16
	if rightPanel != nil && rightPanel.Inner.Dx() > 6 {
		width = rightPanel.Inner.Dx() - 6
	}
	lines := []string{"[Inspector](fg:cyan,mod:bold)", ""}
	lines = append(lines, buildDetailBlock("章节", chapter, width)...)
	lines = append(lines, "")
	lines = append(lines, buildDetailBlock("进度", formatProgressSummary(current, total, progress), width)...)
	lines = append(lines, "")
	lines = append(lines, buildDetailBlock("总行数", fmt.Sprintf("%d lines", total), width)...)
	lines = append(lines, "", "[Search](fg:yellow,mod:bold)", "")
	lines = append(lines, buildDetailBlock("查询", emptyFallback(app.searchQuery, "无"), width)...)
	lines = append(lines, "", "[Recent Logs](fg:green,mod:bold)", "  reader resumed", "  progress synced", "  layout stable")
	return strings.Join(lines, "\n")
}

func buildDetailBlock(label, value string, width int) []string {
	lines := []string{"  " + label}
	for _, line := range wrapDisplayLines(value, width) {
		lines = append(lines, "    "+line)
	}
	return lines
}

func wrapDisplayLines(text string, width int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{"-"}
	}
	if width < 6 {
		width = 6
	}
	parts := strings.Split(text, "\n")
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		for runewidth.StringWidth(part) > width {
			segment := runewidth.Truncate(part, width, "")
			lines = append(lines, segment)
			part = strings.TrimSpace(strings.TrimPrefix(part, segment))
			if part == "" {
				break
			}
		}
		if part != "" {
			lines = append(lines, part)
		}
	}
	if len(lines) == 0 {
		return []string{"-"}
	}
	return lines
}

func formatProgressSummary(current, total int, raw string) string {
	if total <= 0 {
		return raw
	}
	percent := 0
	if total > 1 && current > 0 {
		percent = int(float64(current-1) * 100 / float64(total-1))
		if current >= total {
			percent = 100
		}
	}
	return fmt.Sprintf("%d / %d\n%d%%", current, total, percent)
}

func emptyFallback(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func buildFooter() string {
	elapsed := time.Since(app.sessionStart).Round(time.Minute)
	tag := currentTheme().FooterTag
	version := strings.TrimSpace(app.currentVersion)
	if version == "" {
		version = "dev"
	}
	line1 := fmt.Sprintf("[%s](fg:black,bg:green,mod:bold)  utf-8  session [%s](fg:yellow)  theme [%s](fg:cyan)  version [%s](fg:yellow)  status [%s](fg:green)",
		tag, elapsed, app.config.Theme, version, shorten(app.statusMessage, 22))
	line2 := fmt.Sprintf("status: [%s](fg:cyan)", shorten(app.statusMessage, 72))
	switch app.mode {
	case modeHome:
		return line1 + "\n[↑/↓](fg:cyan):选择  [→/Enter](fg:cyan):打开  [i](fg:cyan):导入  [o/r](fg:cyan):排序/过滤  [x](fg:cyan):移除  [T](fg:cyan):主题  [u](fg:cyan):更新  [q](fg:red):退出"
	case modeReading:
		return line1 + "\n[↑/↓](fg:cyan):翻页  [←/→](fg:cyan):切章  [+/-](fg:cyan):正文行数  [c](fg:cyan):颜色  [,](fg:cyan):阅读设置  [/](fg:cyan):搜索  [s/B](fg:cyan):书签  [m](fg:cyan):目录  [T](fg:cyan):主题  [u](fg:cyan):更新  [q](fg:red):书架"
	case modeTOC:
		return line1 + "\n[↑/↓](fg:cyan):移动  [→/Enter](fg:cyan):打开  [←/m](fg:cyan):返回  [0-9](fg:cyan):跳章  [q](fg:red):书架"
	case modeBookmarks:
		return line1 + "\n[↑/↓](fg:cyan):移动  [→/Enter](fg:cyan):打开  [d](fg:cyan):删除  [←/B/q](fg:red):关闭"
	case modeSearchInput:
		return line1 + "\n输入搜索关键字，支持左右移动，Enter 执行，Esc 取消"
	case modeImportInput:
		scope := "当前层"
		if app.importRecursive {
			scope = "递归"
		}
		return line1 + "\n输入文件或文件夹路径，Tab 补全，Ctrl-r 切换扫描范围(" + scope + ")，Esc 取消"
	case modeReadingSettings:
		return line1 + "\n[↑/↓](fg:cyan):选择  [←/→](fg:cyan):调整  [Enter](fg:cyan):切换/编辑  [Esc](fg:red):返回阅读"
	case modeReadingColorInput:
		return line1 + "\n输入字体颜色，支持 #RRGGBB / #RGB / R,G,B，Enter 保存，Esc 取消"
	case modeDeleteConfirm:
		return line1 + "\n[y](fg:cyan):仅移出书架  [D](fg:red):删除本地文件  [Esc](fg:yellow):取消"
	case modeUpdatePrompt:
		return line1 + "\n[y/Enter](fg:cyan):开始更新  [n/Esc](fg:yellow):稍后再说"
	case modeUpdating:
		return line1 + "\n正在下载安装新版本，请稍候…"
	case modeUpdateRestart:
		return line1 + "\n[Enter](fg:cyan):退出并手动重新启动  [q](fg:red):直接退出"
	default:
		return line1 + "\n" + line2
	}
}

func buildMainTitle() string {
	switch app.mode {
	case modeHome, modeImportInput, modeDeleteConfirm:
		return " bookshelf "
	case modeBookmarks:
		return " bookmarks "
	case modeTOC:
		return " table of contents "
	case modeReadingSettings:
		return " reading settings "
	case modeReadingColorInput:
		return " reading color "
	case modeUpdatePrompt, modeUpdating, modeUpdateRestart:
		return " update "
	default:
		return " editor: " + currentDisplayName() + " "
	}
}

func buildMainPanel() string {
	switch app.mode {
	case modeHome:
		return buildBookshelfPanel()
	case modeImportInput:
		scopeLabel := "当前层"
		if app.importRecursive {
			scopeLabel = "递归子目录"
		}
		lines := []string{
			"导入本地书籍",
			"",
			"请输入 txt / epub 文件路径，或一个文件夹路径：",
			"",
			renderInputWithCursor(app.inputValue, app.inputCursor),
			"",
			"支持左右移动、删除、Tab 补全、拖入文件/目录，以及目录批量导入。",
			"当前扫描范围：" + scopeLabel,
			"按 Ctrl-r 切换当前层 / 递归子目录。",
		}
		if len(app.inputHints) > 0 {
			pageSize := importHintPageSize()
			start, end, page, totalPages := importHintPageBounds(pageSize)
			lines = append(lines, "", fmt.Sprintf("候选路径：第 %d/%d 页", page, totalPages))
			for i := start; i < end; i++ {
				hint := app.inputHints[i]
				prefix := "  "
				if i == app.inputHintIndex {
					prefix = "> "
				}
				lines = append(lines, prefix+shorten(hint, 72))
			}
			lines = append(lines, "", "Tab/上下键切换候选，Enter 先填入再导入。")
		}
		return strings.Join(lines, "\n")
	case modeDeleteConfirm:
		return fmt.Sprintf("删除确认\n\n目标书籍：%s\n\n按 y 仅从书架移除。\n按 D 从书架移除并删除本地文件。\n按 Esc 取消。", app.deleteTargetTitle)
	case modeTOC:
		return tocStatusText()
	case modeBookmarks:
		return buildBookmarksPanel()
	case modeSearchInput:
		return "搜索\n\n请输入关键字并回车执行：\n\n" + renderInputWithCursor(app.inputValue, app.inputCursor)
	case modeReadingSettings:
		return buildReadingSettingsPanel()
	case modeReadingColorInput:
		return "阅读颜色\n\n请输入字体颜色：\n\n" + renderInputWithCursor(app.inputValue, app.inputCursor) + "\n\n支持 #RRGGBB、#RGB 或 R,G,B。"
	case modeUpdatePrompt:
		return buildUpdatePromptPanel()
	case modeUpdating:
		return buildUpdatingPanel()
	case modeUpdateRestart:
		return buildUpdateRestartPanel()
	default:
		if app.showHelp {
			return buildHelpPanel()
		}
		if app.showProgress && app.reader != nil {
			return app.reader.GetProgress()
		}
		if app.reader == nil {
			return "未打开书籍"
		}
		return formatReadingPanel(highlightSearchMatches(app.reader.CurrentView(readingVisibleSourceLines()), app.searchQuery))
	}
}

func buildUpdatePromptPanel() string {
	if app.updateRelease == nil {
		return "没有可用更新。"
	}
	body := strings.TrimSpace(app.updateRelease.Body)
	if body == "" {
		body = "本次版本未提供额外说明。"
	}
	lines := []string{
		"发现新版本",
		"",
		fmt.Sprintf("当前版本：%s", emptyFallback(strings.TrimSpace(app.currentVersion), "未知")),
		fmt.Sprintf("最新版本：%s", app.updateRelease.TagName),
		"",
		"是否现在下载并替换当前程序？",
		"更新完成后退出，再重新启动即可生效。",
		"",
		"更新说明：",
	}
	for _, line := range wrapDisplayLines(body, max(28, mainPanel.Inner.Dx()-4)) {
		lines = append(lines, "  "+line)
	}
	lines = append(lines, "", "按 y / Enter 开始更新，按 n / Esc 稍后再说。")
	if !app.updatePromptManual {
		lines = append(lines, "如果这次选择不更新，之后启动时不会再提醒这个版本。")
	} else {
		lines = append(lines, "这是手动检查更新，不会受之前的忽略记录影响。")
	}
	return strings.Join(lines, "\n")
}

func buildUpdatingPanel() string {
	version := "最新版本"
	if app.updateRelease != nil && app.updateRelease.TagName != "" {
		version = app.updateRelease.TagName
	}
	return strings.Join([]string{
		"正在安装更新",
		"",
		"目标版本：" + version,
		"",
		"ReadCLI 正在从 GitHub Releases 下载并替换当前程序。",
		"更新完成后会提示你退出并重新启动生效。",
	}, "\n")
}

func buildUpdateRestartPanel() string {
	version := "新版本"
	if app.updateRelease != nil && app.updateRelease.TagName != "" {
		version = app.updateRelease.TagName
	}
	return strings.Join([]string{
		"更新已安装",
		"",
		"已完成热更新：" + version,
		"",
		"请退出当前程序，然后重新启动 ReadCLI。",
		"",
		"按 Enter 退出。",
	}, "\n")
}

func readingContentWidth(mainWidth int) int {
	width := readingWidth(mainWidth)
	if width <= 0 {
		return 80
	}
	if app != nil && app.reader != nil {
		target := int(float64(width) * readingWidthRatio())
		if target < 28 {
			target = 28
		}
		if target > width {
			target = width
		}
		target -= readingMarginLeft() + readingMarginRight()
		if target < 20 {
			target = 20
		}
		return target
	}
	return width
}

func readingVisibleSourceLines() int {
	if app == nil || app.displayLines < 1 {
		return 1
	}
	maxLines := readingMaxSourceLines()
	if maxLines < 1 {
		maxLines = 1
	}
	if app.displayLines > maxLines {
		return maxLines
	}
	return app.displayLines
}

func formatReadingPanel(text string) string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	lineGap := readingLineSpacing()
	leftPad := strings.Repeat(" ", readingMarginLeft())
	padded := make([]string, 0, len(lines)*(lineGap+1)+readingMarginTop()+readingMarginBottom())
	for i := 0; i < readingMarginTop(); i++ {
		padded = append(padded, "")
	}
	for i, line := range lines {
		padded = append(padded, leftPad+line)
		if i != len(lines)-1 {
			for gap := 0; gap < lineGap; gap++ {
				padded = append(padded, "")
			}
		}
	}
	for i := 0; i < readingMarginBottom(); i++ {
		padded = append(padded, "")
	}
	return strings.Join(padded, "\n")
}

func readingMaxSourceLines() int {
	if mainPanel == nil {
		return max(1, app.displayLines)
	}
	available := mainPanel.Inner.Dy() - readingMarginTop() - readingMarginBottom()
	if available < 1 {
		return 1
	}
	spacing := readingLineSpacing()
	return max(1, (available+spacing)/(spacing+1))
}

func highlightSearchMatches(text, query string) string {
	query = strings.TrimSpace(query)
	if text == "" || query == "" {
		return text
	}

	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	if lowerQuery == "" {
		return text
	}

	var b strings.Builder
	start := 0
	for {
		idx := strings.Index(lowerText[start:], lowerQuery)
		if idx < 0 {
			b.WriteString(text[start:])
			break
		}
		idx += start
		end := idx + len(lowerQuery)
		b.WriteString(text[start:idx])
		b.WriteString("[")
		b.WriteString(text[idx:end])
		b.WriteString("](fg:black,bg:yellow,mod:bold)")
		start = end
	}
	return b.String()
}

func buildHelpPanel() string {
	if mainPanel == nil || mainPanel.Inner.Dx() < 72 {
		return menuText
	}

	leftTitle := "[Vim 风格](fg:cyan,mod:bold)"
	rightTitle := "[方向键 / 普通键](fg:yellow,mod:bold)"

	leftSections := []string{
		"[书架首页](fg:green,mod:bold)\n  j/k 移动\n  Enter 打开\n  i 导入\n  o/r 排序过滤\n  x 移除\n  u 检查更新",
		"[阅读界面](fg:green,mod:bold)\n  j/k 翻页\n  [ / ] 切章\n  / 搜索\n  n/N 结果跳转\n  s/B 书签\n  m 目录\n  , 阅读设置\n  u 检查更新",
		"[目录 / 书签](fg:green,mod:bold)\n  j/k 移动\n  Enter 打开\n  m 或 B 返回",
		"[通用](fg:green,mod:bold)\n  +/- 调整行数\n  T 切换主题\n  q 返回书架或退出",
	}

	rightSections := []string{
		"[书架首页](fg:magenta,mod:bold)\n  ↑/↓ 选择\n  → 或 Enter 打开\n  u 检查更新",
		"[阅读界面](fg:magenta,mod:bold)\n  ↑/↓ 翻页\n  ←/→ 切章\n  , 阅读设置\n  u 检查更新",
		"[目录 / 书签](fg:magenta,mod:bold)\n  ↑/↓ 移动\n  → 或 Enter 打开\n  ← 返回",
		"[导入输入](fg:magenta,mod:bold)\n  ←/→ 移动光标\n  ↑/↓ 选择候选\n  Tab 补全\n  拖入文件/目录自动取路径\n  Ctrl-r 切换递归\n  Enter 填入或导入\n  Esc 取消",
	}

	left := leftTitle + "\n\n" + strings.Join(leftSections, "\n\n")
	right := rightTitle + "\n\n" + strings.Join(rightSections, "\n\n")
	return joinColumns(left, right, 36)
}

func joinColumns(left, right string, leftWidth int) string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")
	rows := max(len(leftLines), len(rightLines))
	lines := make([]string, 0, rows)
	for i := 0; i < rows; i++ {
		l := ""
		r := ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		lines = append(lines, padDisplayText(l, leftWidth)+"  "+r)
	}
	return strings.Join(lines, "\n")
}

type readingSettingItem struct {
	Label string
	Value string
}

func buildReadingSettingsPanel() string {
	items := readingSettingsItems()
	if len(items) == 0 {
		return "阅读设置不可用"
	}
	if app.settingsIndex < 0 {
		app.settingsIndex = 0
	}
	if app.settingsIndex >= len(items) {
		app.settingsIndex = len(items) - 1
	}
	lines := []string{
		"阅读设置",
		"",
		"这些设置全局生效，三个主题共用。",
		"",
	}
	for i, item := range items {
		prefix := "  "
		if i == app.settingsIndex {
			prefix = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%-10s %s", prefix, item.Label, item.Value))
	}
	lines = append(lines, "", "提示：字体颜色支持 #RRGGBB、#RGB、R,G,B。")
	return strings.Join(lines, "\n")
}

func readingSettingsItems() []readingSettingItem {
	colorValue := "#FFFFFF"
	if app != nil && app.config != nil && strings.TrimSpace(app.config.ReadingTextColor) != "" {
		colorValue = app.config.ReadingTextColor
	}
	return []readingSettingItem{
		{Label: "正文宽度", Value: fmt.Sprintf("%.0f%%", readingWidthRatio()*100)},
		{Label: "左边距", Value: fmt.Sprintf("%d", readingMarginLeft())},
		{Label: "右边距", Value: fmt.Sprintf("%d", readingMarginRight())},
		{Label: "上边距", Value: fmt.Sprintf("%d", readingMarginTop())},
		{Label: "下边距", Value: fmt.Sprintf("%d", readingMarginBottom())},
		{Label: "行间距", Value: fmt.Sprintf("%d", readingLineSpacing())},
		{Label: "字体颜色", Value: colorValue},
		{Label: "高对比", Value: onOffText(app.config != nil && app.config.ReadingHighContrast)},
		{Label: "基础色模式", Value: onOffText(app.config != nil && app.config.ForceBasicColor)},
	}
}

func onOffText(value bool) string {
	if value {
		return "开"
	}
	return "关"
}

func padDisplayText(value string, width int) string {
	displayWidth := styledDisplayWidth(value)
	if displayWidth >= width {
		return value
	}
	return value + strings.Repeat(" ", width-displayWidth)
}

func styledDisplayWidth(value string) int {
	plain := stripTermUIStyle(value)
	width := 0
	for _, r := range plain {
		if r > 127 {
			width += 2
			continue
		}
		width++
	}
	return width
}

func stripTermUIStyle(value string) string {
	var b strings.Builder
	runes := []rune(value)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '[' {
			j := i + 1
			for j < len(runes) && runes[j] != ']' {
				j++
			}
			if j < len(runes)-1 && runes[j] == ']' && runes[j+1] == '(' {
				b.WriteString(string(runes[i+1 : j]))
				k := j + 2
				for k < len(runes) && runes[k] != ')' {
					k++
				}
				i = k
				continue
			}
		}
		b.WriteRune(runes[i])
	}
	return b.String()
}

func buildBookshelfPanel() string {
	books := visibleBooks()
	var lines []string
	th := currentTheme()
	lines = append(lines, "["+titleCase(th.HomeName)+"](fg:cyan,mod:bold)")
	lines = append(lines, "")
	if app.loadingBookPath != "" {
		bookName := strings.TrimSuffix(filepath.Base(app.loadingBookPath), filepath.Ext(app.loadingBookPath))
		lines = append(lines,
			"正在打开：",
			"",
			"  "+shortenDisplay(bookName, bookshelfTitleWidth()),
			"",
			"请稍等，正在加载正文和目录…",
		)
		return strings.Join(lines, "\n")
	}
	if len(books) == 0 {
		lines = append(lines,
			"还没有导入任何书。",
			"",
			"开始方式：",
			"  1. 按 i 导入本地 txt / epub",
			"  2. 或直接运行 readcli /path/to/book.epub",
			"",
			"导入后会自动记录：",
			"  - 阅读进度",
			"  - 最后阅读时间",
			"  - 当前章节信息",
		)
		return strings.Join(lines, "\n")
	}

	pageSize := bookshelfPageSize()
	start := (app.shelfIndex / pageSize) * pageSize
	end := start + pageSize
	if end > len(books) {
		end = len(books)
	}
	lines = append(lines, fmt.Sprintf("共 %d 本  |  排序 %s  |  过滤 %s  |  第 %d/%d 页", len(books), readableSort(app.sortMode), readableFilter(app.filterMode), start/pageSize+1, (len(books)+pageSize-1)/pageSize))
	lines = append(lines, bookshelfStatsLine(books))
	lines = append(lines, "")
	titleWidth := bookshelfTitleWidth()
	lines = append(lines, "  书名")
	lines = append(lines, "  "+strings.Repeat("─", max(24, titleWidth)))
	for i := start; i < end; i++ {
		book := books[i]
		prefix := "  "
		if i == app.shelfIndex {
			prefix = "> "
		}
		lines = append(lines, prefix+shortenDisplay(book.Title, titleWidth))
	}
	return strings.Join(lines, "\n")
}

func bookshelfTitleWidth() int {
	titleWidth := 28
	if mainPanel != nil && mainPanel.Inner.Dx() > 0 {
		available := mainPanel.Inner.Dx() - 4
		if available > 18 {
			titleWidth = available
		}
	}
	if titleWidth < 18 {
		titleWidth = 18
	}
	return titleWidth
}

func bookshelfStatsLine(books []lib.BookshelfBook) string {
	unread := 0
	reading := 0
	finished := 0
	for _, book := range books {
		switch {
		case book.ProgressPercent >= 100:
			finished++
		case book.ProgressPos > 0:
			reading++
		default:
			unread++
		}
	}
	return fmt.Sprintf("未读 %d  |  在读 %d  |  已读 %d", unread, reading, finished)
}

func shortenDisplay(text string, width int) string {
	text = strings.TrimSpace(text)
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(text) <= width {
		return text
	}
	if width <= 1 {
		return runewidth.Truncate(text, width, "")
	}
	return runewidth.Truncate(text, width, "…")
}

func padDisplay(text string, width int) string {
	current := runewidth.StringWidth(text)
	if current >= width {
		return text
	}
	return text + strings.Repeat(" ", width-current)
}

func buildBookmarksPanel() string {
	bookmarks := bookmarksForCurrentBook()
	if len(bookmarks) == 0 {
		return "当前书没有书签。\n\n按 s 保存一个书签。"
	}
	if app.bookmarkIndex < 0 {
		app.bookmarkIndex = 0
	}
	if app.bookmarkIndex >= len(bookmarks) {
		app.bookmarkIndex = len(bookmarks) - 1
	}
	var lines []string
	lines = append(lines, "书签列表", "")
	for i, mark := range bookmarks {
		prefix := "  "
		if i == app.bookmarkIndex {
			prefix = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%s | %s", prefix, shorten(mark.Chapter, 16), shorten(mark.Snippet, 36)))
	}
	return strings.Join(lines, "\n")
}

func bookshelfPageSize() int {
	if mainPanel != nil && mainPanel.Inner.Dy() > 4 {
		// title + blank + summary + stats + blank + header + separator + bottom breathing room
		reservedLines := 8
		available := mainPanel.Inner.Dy() - reservedLines
		if available < 3 {
			return 3
		}
		return available
	}
	return 10
}

func currentDisplayName() string {
	if app.currentFile != "" {
		return filepath.Base(app.currentFile)
	}
	if book := selectedBook(); book != nil {
		return filepath.Base(book.Path)
	}
	return "bookshelf"
}

func shorten(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max || max <= 1 {
		return value
	}
	return string(runes[:max-1]) + "…"
}

func readableSort(value string) string {
	switch value {
	case "title":
		return "书名"
	case "imported":
		return "导入时间"
	default:
		return "最近阅读"
	}
}

func readableFilter(value string) string {
	switch value {
	case "epub":
		return "EPUB"
	case "txt":
		return "TXT"
	case "unread":
		return "未读"
	case "reading":
		return "在读"
	case "finished":
		return "已读"
	default:
		return "全部"
	}
}

func formatStamp(value string) string {
	if value == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return t.Format("01-02 15:04")
}

func renderInputWithCursor(value string, cursor int) string {
	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	left := string(runes[:cursor])
	right := string(runes[cursor:])
	return left + "[|](fg:black,bg:cyan,mod:bold)" + right
}

func resetInputState() {
	app.inputValue = ""
	app.inputCursor = 0
	app.inputHints = nil
	app.inputHintIndex = 0
}

func insertInputText(text string) {
	runes := []rune(app.inputValue)
	if app.inputCursor < 0 {
		app.inputCursor = 0
	}
	if app.inputCursor > len(runes) {
		app.inputCursor = len(runes)
	}
	insert := []rune(text)
	runes = append(runes[:app.inputCursor], append(insert, runes[app.inputCursor:]...)...)
	app.inputValue = string(runes)
	app.inputCursor += len(insert)
	app.inputHints = nil
	app.inputHintIndex = 0
}

func deleteInputBackward() {
	runes := []rune(app.inputValue)
	if app.inputCursor <= 0 || len(runes) == 0 {
		return
	}
	runes = append(runes[:app.inputCursor-1], runes[app.inputCursor:]...)
	app.inputValue = string(runes)
	app.inputCursor--
	app.inputHints = nil
	app.inputHintIndex = 0
}

func deleteInputForward() {
	runes := []rune(app.inputValue)
	if app.inputCursor < 0 || app.inputCursor >= len(runes) {
		return
	}
	runes = append(runes[:app.inputCursor], runes[app.inputCursor+1:]...)
	app.inputValue = string(runes)
	app.inputHints = nil
	app.inputHintIndex = 0
}

func moveInputCursor(delta int) {
	runes := []rune(app.inputValue)
	app.inputCursor += delta
	if app.inputCursor < 0 {
		app.inputCursor = 0
	}
	if app.inputCursor > len(runes) {
		app.inputCursor = len(runes)
	}
}

func setInputCursor(pos int) {
	runes := []rune(app.inputValue)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	app.inputCursor = pos
}

func completeImportPath() {
	current := strings.TrimSpace(app.inputValue)
	if current == "" {
		current = "."
	}

	resolved, useTilde := resolveImportPath(current)
	dir, prefix := splitImportPath(resolved)
	entries, err := os.ReadDir(dir)
	if err != nil {
		app.inputHints = nil
		app.inputHintIndex = 0
		app.statusMessage = "无法读取目录: " + shorten(dir, 36)
		return
	}

	type match struct {
		resolved string
		display  string
	}
	var matches []match
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		resolvedMatch := filepath.Join(dir, name)
		if entry.IsDir() {
			resolvedMatch += string(os.PathSeparator)
		}
		displayMatch := renderImportPath(resolvedMatch, useTilde)
		matches = append(matches, match{resolved: resolvedMatch, display: displayMatch})
	}
	if len(matches) == 0 {
		app.inputHints = nil
		app.inputHintIndex = 0
		app.statusMessage = "没有匹配的路径"
		return
	}

	common := matches[0].display
	for _, item := range matches[1:] {
		common = commonPathPrefix(common, item.display)
	}
	if len([]rune(common)) > len([]rune(app.inputValue)) {
		app.inputValue = common
		app.inputCursor = len([]rune(common))
	}

	previousHints := append([]string(nil), app.inputHints...)
	app.inputHints = nil
	for _, item := range matches {
		app.inputHints = append(app.inputHints, item.display)
	}

	if len(matches) == 1 {
		app.inputValue = matches[0].display
		app.inputCursor = len([]rune(app.inputValue))
		app.inputHints = nil
		app.inputHintIndex = 0
		app.statusMessage = "已补全路径"
		return
	}
	if sameStringSlice(previousHints, app.inputHints) {
		app.inputHintIndex = (app.inputHintIndex + 1) % len(app.inputHints)
	} else {
		app.inputHintIndex = 0
	}
	app.statusMessage = fmt.Sprintf("找到 %d 个候选", len(matches))
}

func moveInputHint(delta int) {
	if len(app.inputHints) == 0 {
		return
	}
	app.inputHintIndex += delta
	if app.inputHintIndex < 0 {
		app.inputHintIndex = len(app.inputHints) - 1
	}
	if app.inputHintIndex >= len(app.inputHints) {
		app.inputHintIndex = 0
	}
}

func importHintPageSize() int {
	if mainPanel == nil {
		return 8
	}
	available := mainPanel.Inner.Dy() - 10
	if available < 3 {
		return 3
	}
	return available
}

func importHintPageBounds(pageSize int) (start, end, page, totalPages int) {
	if pageSize < 1 {
		pageSize = 1
	}
	total := len(app.inputHints)
	if total == 0 {
		return 0, 0, 1, 1
	}
	start = (app.inputHintIndex / pageSize) * pageSize
	end = start + pageSize
	if end > total {
		end = total
	}
	page = start/pageSize + 1
	totalPages = (total + pageSize - 1) / pageSize
	return start, end, page, totalPages
}

func acceptSelectedImportHint() bool {
	if len(app.inputHints) == 0 {
		return false
	}
	if app.inputHintIndex < 0 || app.inputHintIndex >= len(app.inputHints) {
		app.inputHintIndex = 0
	}
	app.inputValue = app.inputHints[app.inputHintIndex]
	app.inputCursor = len([]rune(app.inputValue))
	app.inputHints = nil
	app.inputHintIndex = 0
	app.statusMessage = "已填入候选路径"
	return true
}

func resolveImportPath(value string) (string, bool) {
	value = normalizeImportInputPath(value)
	if strings.HasPrefix(value, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			if value == "~" {
				return home, true
			}
			trimmed := strings.TrimPrefix(value, "~"+string(os.PathSeparator))
			return filepath.Join(home, trimmed), true
		}
	}
	return value, false
}

func normalizeImportInputPath(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			if unquoted, err := strconv.Unquote(value); err == nil {
				return unquoted
			}
			value = strings.Trim(value, "\"")
		}
		if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
			return strings.Trim(value, "'")
		}
	}

	var b strings.Builder
	escaped := false
	for _, r := range value {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		b.WriteRune(r)
	}
	if escaped {
		b.WriteRune('\\')
	}
	return b.String()
}

func renderImportPath(resolved string, useTilde bool) string {
	if !useTilde {
		return resolved
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return resolved
	}
	if resolved == home {
		return "~"
	}
	if strings.HasPrefix(resolved, home+string(os.PathSeparator)) {
		return "~" + string(os.PathSeparator) + strings.TrimPrefix(resolved, home+string(os.PathSeparator))
	}
	return resolved
}

func splitImportPath(value string) (string, string) {
	clean := value
	if strings.HasSuffix(clean, string(os.PathSeparator)) {
		return clean, ""
	}
	dir := filepath.Dir(clean)
	if dir == "." && !strings.HasPrefix(clean, ".") && !strings.HasPrefix(clean, string(os.PathSeparator)) {
		dir = "."
	}
	base := filepath.Base(clean)
	if dir == "" {
		dir = "."
	}
	return dir, base
}

func commonPathPrefix(a, b string) string {
	ar := []rune(a)
	br := []rune(b)
	n := min(len(ar), len(br))
	i := 0
	for i < n && ar[i] == br[i] {
		i++
	}
	return string(ar[:i])
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func titleCase(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parts := strings.Fields(value)
	for i, part := range parts {
		runes := []rune(part)
		if len(runes) == 0 {
			continue
		}
		if runes[0] >= 'a' && runes[0] <= 'z' {
			runes[0] = runes[0] - 'a' + 'A'
		}
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
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

func setDisplayLines(lines int) {
	if lines < 1 {
		lines = 1
	}
	app.displayLines = lines
	app.config.DisplayLines = lines
	_ = lib.SaveConfig(app.config)
	visible := readingVisibleSourceLines()
	if visible < app.displayLines {
		app.statusMessage = fmt.Sprintf("每页正文 %d 行（当前窗口最多显示 %d 行）", app.displayLines, visible)
	} else {
		app.statusMessage = fmt.Sprintf("每页正文 %d 行", visible)
	}
	syncCurrentBookState()
}

func displayBossKey() {
	app.bossKey = !app.bossKey
	if app.bossKey {
		app.showHelp = false
		app.showProgress = false
		app.statusMessage = "Boss Key 已开启"
		return
	}
	app.statusMessage = "Boss Key 已关闭"
}

func moveReading(delta int) {
	if app.reader == nil {
		return
	}
	app.reader.Goto(app.reader.CurrentPos() + delta)
	app.showHelp = false
	app.showProgress = false
	app.statusMessage = fmt.Sprintf("阅读位置 %d/%d", app.reader.CurrentPos()+1, app.reader.Total())
	syncCurrentBookState()
}

func pageStep() int {
	if readingVisibleSourceLines() < 1 {
		return 1
	}
	return readingVisibleSourceLines()
}

func openReadingSettings() {
	setMode(modeReadingSettings)
	app.settingsIndex = 0
	app.statusMessage = "已打开阅读设置"
}

func moveReadingSettings(delta int) {
	items := readingSettingsItems()
	if len(items) == 0 {
		app.settingsIndex = 0
		return
	}
	app.settingsIndex += delta
	if app.settingsIndex < 0 {
		app.settingsIndex = 0
	}
	if app.settingsIndex >= len(items) {
		app.settingsIndex = len(items) - 1
	}
}

func adjustReadingSetting(delta int) {
	if app == nil || app.config == nil {
		return
	}
	switch app.settingsIndex {
	case 0:
		app.config.ReadingContentWidthRatio += float64(delta) * 0.05
		if app.config.ReadingContentWidthRatio < 0.4 {
			app.config.ReadingContentWidthRatio = 0.4
		}
		if app.config.ReadingContentWidthRatio > 1 {
			app.config.ReadingContentWidthRatio = 1
		}
	case 1:
		app.config.ReadingMarginLeft = max(0, app.config.ReadingMarginLeft+delta)
	case 2:
		app.config.ReadingMarginRight = max(0, app.config.ReadingMarginRight+delta)
	case 3:
		app.config.ReadingMarginTop = max(0, app.config.ReadingMarginTop+delta)
	case 4:
		app.config.ReadingMarginBottom = max(0, app.config.ReadingMarginBottom+delta)
	case 5:
		app.config.ReadingLineSpacing = max(0, app.config.ReadingLineSpacing+delta)
	}
	_ = lib.SaveConfig(app.config)
	if app.reader != nil {
		applyLayoutFromTerminal()
	}
	app.statusMessage = "阅读设置已更新"
}

func activateReadingSetting() {
	if app == nil || app.config == nil {
		return
	}
	switch app.settingsIndex {
	case 6:
		app.mode = modeReadingColorInput
		app.inputValue = app.config.ReadingTextColor
		app.inputCursor = len([]rune(app.inputValue))
	case 7:
		app.config.ReadingHighContrast = !app.config.ReadingHighContrast
		_ = lib.SaveConfig(app.config)
		app.statusMessage = "高对比已切换"
	case 8:
		app.config.ForceBasicColor = !app.config.ForceBasicColor
		_ = lib.SaveConfig(app.config)
		if app.config.ForceBasicColor {
			app.statusMessage = "已切换为基础色模式"
		} else {
			app.statusMessage = "已切换为扩展颜色模式"
		}
	}
}

func applyReadingTextColorInput() {
	value := lib.NormalizeConfiguredColor(app.inputValue)
	if value == "" {
		app.statusMessage = "颜色格式无效"
		return
	}
	app.config.ReadingTextColor = value
	app.mode = modeReadingSettings
	resetInputState()
	_ = lib.SaveConfig(app.config)
	app.statusMessage = "字体颜色已更新"
}

func cycleReadingColorPreset() {
	if app == nil || app.config == nil {
		return
	}
	palette := []string{"#FFFFFF", "#7FDBFF", "#FFDC00", "#2ECC40", "#F012BE"}
	current := lib.NormalizeConfiguredColor(app.config.ReadingTextColor)
	index := -1
	for i, item := range palette {
		if item == current {
			index = i
			break
		}
	}
	app.config.ReadingTextColor = palette[(index+1+len(palette))%len(palette)]
	_ = lib.SaveConfig(app.config)
	app.statusMessage = "字体颜色已切换为 " + app.config.ReadingTextColor
}

func switchTheme() {
	current := app.config.Theme
	for i, name := range app.themeOrder {
		if name == current {
			app.config.Theme = app.themeOrder[(i+1)%len(app.themeOrder)]
			_ = lib.SaveConfig(app.config)
			app.statusMessage = "主题已切换为 " + app.config.Theme
			return
		}
	}
	app.config.Theme = app.themeOrder[0]
	_ = lib.SaveConfig(app.config)
}

func toggleBorder() {
	app.showBorder = !app.showBorder
	app.config.ShowBorder = app.showBorder
	_ = lib.SaveConfig(app.config)
}

func toggleTimer() {
	app.timer = !app.timer
	if app.timer {
		app.ticker = time.NewTicker(interval * time.Millisecond)
		go func() {
			for range app.ticker.C {
				if app.mode == modeReading && app.reader != nil {
					moveReading(pageStep())
					renderUI()
				}
			}
		}()
		app.statusMessage = "自动翻页已开启"
		return
	}
	if app.ticker != nil {
		app.ticker.Stop()
	}
	app.statusMessage = "自动翻页已关闭"
}

func saveBookmark() {
	if app.reader == nil || app.currentFile == "" {
		return
	}
	list := app.bookmarks.Books[app.currentFile]
	mark := lib.Bookmark{
		Path:          app.currentFile,
		Position:      app.reader.CurrentPos(),
		Chapter:       app.reader.CurrentChapterTitle(),
		Snippet:       shorten(app.reader.Current(), 32),
		CreatedAt:     time.Now().Format(time.RFC3339),
		ProgressTotal: app.reader.Total(),
	}
	list = append(list, mark)
	app.bookmarks.Books[app.currentFile] = list
	_ = lib.SaveBookmarks(app.bookmarks)
	app.statusMessage = "书签已保存"
}

func openBookmarks() {
	app.mode = modeBookmarks
	app.bookmarkIndex = 0
	app.statusMessage = "已打开书签列表"
}

func runSearch() {
	if app.reader == nil {
		return
	}
	app.searchQuery = strings.TrimSpace(app.inputValue)
	resetInputState()
	app.mode = modeReading
	if app.searchQuery == "" {
		app.statusMessage = "搜索已取消"
		return
	}
	pos, ok := app.reader.Search(app.searchQuery, min(app.reader.CurrentPos()+1, app.reader.Total()-1), true)
	if !ok {
		pos, ok = app.reader.Search(app.searchQuery, 0, true)
	}
	if !ok {
		app.statusMessage = "未找到关键字: " + app.searchQuery
		return
	}
	app.reader.Goto(pos)
	app.lastSearchIndex = pos
	app.statusMessage = "已跳转到搜索结果"
	syncCurrentBookState()
}

func jumpSearch(forward bool) {
	if app.reader == nil || strings.TrimSpace(app.searchQuery) == "" {
		app.statusMessage = "没有可继续跳转的搜索结果"
		return
	}
	start := app.reader.CurrentPos()
	if forward {
		start++
		if start >= app.reader.Total() {
			start = 0
		}
		pos, ok := app.reader.Search(app.searchQuery, start, true)
		if !ok {
			pos, ok = app.reader.Search(app.searchQuery, 0, true)
			if !ok {
				app.statusMessage = "未找到更多结果"
				return
			}
		}
		app.reader.Goto(pos)
	} else {
		start--
		if start < 0 {
			start = app.reader.Total() - 1
		}
		pos, ok := app.reader.Search(app.searchQuery, start, false)
		if !ok {
			pos, ok = app.reader.Search(app.searchQuery, app.reader.Total()-1, false)
			if !ok {
				app.statusMessage = "未找到更多结果"
				return
			}
		}
		app.reader.Goto(pos)
	}
	app.statusMessage = "已跳转到搜索结果"
	syncCurrentBookState()
}

func setMode(m mode) {
	app.mode = m
	resetInputState()
	app.rowNumber = ""
}

func cycleSort() {
	switch app.sortMode {
	case "recent":
		app.sortMode = "imported"
	case "imported":
		app.sortMode = "title"
	default:
		app.sortMode = "recent"
	}
	app.statusMessage = "排序已切换为 " + app.sortMode
}

func cycleFilter() {
	options := []string{"all", "epub", "txt", "unread", "reading", "finished"}
	for i, opt := range options {
		if opt == app.filterMode {
			app.filterMode = options[(i+1)%len(options)]
			app.shelfIndex = 0
			app.statusMessage = "过滤已切换为 " + app.filterMode
			return
		}
	}
	app.filterMode = "all"
}

func importBook() {
	path := strings.TrimSpace(app.inputValue)
	resetInputState()
	app.mode = modeHome
	if path == "" {
		app.statusMessage = "导入已取消"
		return
	}
	resolved, _ := resolveImportPath(path)
	path = resolved
	info, err := os.Stat(path)
	if err != nil {
		app.statusMessage = "文件不存在"
		return
	}
	if info.IsDir() {
		app.statusMessage = importModeLabel() + "正在扫描目录..."
		renderUIIfReady()
		imported, err := importBooksFromDirectory(path, app.importRecursive)
		switch {
		case err != nil:
			app.statusMessage = err.Error()
		case imported == 0:
			app.statusMessage = "目录中没有可导入的 txt/epub"
		case imported == 1:
			app.statusMessage = importModeLabel() + "已从目录导入 1 本书"
		default:
			app.statusMessage = fmt.Sprintf("%s已从目录导入 %d 本书", importModeLabel(), imported)
		}
		return
	}

	book, err := loadBookshelfBook(path)
	if err != nil {
		app.statusMessage = err.Error()
		return
	}
	lib.UpsertBookshelfBook(app.bookshelf, book)
	_ = lib.SaveBookshelf(app.bookshelf)
	app.statusMessage = "已导入 " + filepath.Base(path)
}

func importBooksFromDirectory(root string, recursive bool) (int, error) {
	paths, err := collectImportCandidates(root, recursive)
	if err != nil {
		return 0, err
	}

	total := len(paths)
	imported := 0
	for i, path := range paths {
		app.statusMessage = fmt.Sprintf("%s正在导入 %d/%d: %s", importModeLabel(), i+1, total, shorten(filepath.Base(path), 24))
		renderUIIfReady()

		book, err := loadBookshelfBook(path)
		if err != nil {
			continue
		}
		lib.UpsertBookshelfBook(app.bookshelf, book)
		imported++
	}
	_ = lib.SaveBookshelf(app.bookshelf)
	return imported, nil
}

func collectImportCandidates(root string, recursive bool) ([]string, error) {
	if recursive {
		var paths []string
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			if !isSupportedBookFile(path) {
				return nil
			}
			paths = append(paths, path)
			return nil
		})
		return paths, err
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if !isSupportedBookFile(path) {
			continue
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func toggleImportRecursive() {
	app.importRecursive = !app.importRecursive
	if app.importRecursive {
		app.statusMessage = "目录导入已切换为递归子目录"
		return
	}
	app.statusMessage = "目录导入已切换为仅当前层"
}

func importModeLabel() string {
	if app.importRecursive {
		return "[递归] "
	}
	return "[当前层] "
}

func loadBookshelfBook(path string) (lib.BookshelfBook, error) {
	r, err := newReaderForPath(path)
	if err != nil {
		return lib.BookshelfBook{}, err
	}
	if err := r.Load(path); err != nil {
		return lib.BookshelfBook{}, err
	}
	return lib.BookshelfBook{
		Path:            path,
		Title:           bookTitleForPath(path, r.BookTitle()),
		Format:          strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."),
		ProgressPos:     0,
		ProgressTotal:   r.Total(),
		ProgressPercent: 0,
		CurrentChapter:  r.CurrentChapterTitle(),
		LastReadAt:      time.Now().Format(time.RFC3339),
	}, nil
}

func isSupportedBookFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".epub":
		return true
	default:
		return false
	}
}

func openSelectedBook() {
	book := selectedBook()
	if book == nil {
		app.statusMessage = "书架为空"
		return
	}
	app.loadingBookPath = book.Path
	app.statusMessage = "正在打开 " + shorten(filepath.Base(book.Path), 24)
	renderUIIfReady()
	if err := openBook(book.Path); err != nil {
		app.loadingBookPath = ""
		app.statusMessage = err.Error()
		return
	}
	app.loadingBookPath = ""
}

func removeSelectedBook(deleteFile bool) {
	path := app.deleteTargetPath
	if path == "" {
		return
	}
	if deleteFile {
		_ = os.Remove(path)
	}
	lib.RemoveBookshelfBook(app.bookshelf, path)
	delete(app.readerCache, path)
	if cachePath, err := persistentCachePath(path); err == nil {
		_ = os.Remove(cachePath)
	}
	delete(app.progress.Books, path)
	delete(app.bookmarks.Books, path)
	_ = lib.SaveBookshelf(app.bookshelf)
	_ = lib.SaveProgress(app.progress)
	_ = lib.SaveBookmarks(app.bookmarks)
	app.mode = modeHome
	app.deleteTargetPath = ""
	app.deleteTargetTitle = ""
	app.statusMessage = "已移出书架"
	if deleteFile {
		app.statusMessage = "已删除本地文件并移出书架"
	}
}

func prepareDeleteSelectedBook() {
	book := selectedBook()
	if book == nil {
		app.statusMessage = "没有可删除的书籍"
		return
	}
	app.deleteTargetPath = book.Path
	app.deleteTargetTitle = book.Title
	app.mode = modeDeleteConfirm
}

func moveShelf(delta int) {
	books := visibleBooks()
	if len(books) == 0 {
		app.shelfIndex = 0
		return
	}
	app.shelfIndex += delta
	if app.shelfIndex < 0 {
		app.shelfIndex = 0
	}
	if app.shelfIndex >= len(books) {
		app.shelfIndex = len(books) - 1
	}
}

func moveBookmarks(delta int) {
	bookmarks := bookmarksForCurrentBook()
	if len(bookmarks) == 0 {
		app.bookmarkIndex = 0
		return
	}
	app.bookmarkIndex += delta
	if app.bookmarkIndex < 0 {
		app.bookmarkIndex = 0
	}
	if app.bookmarkIndex >= len(bookmarks) {
		app.bookmarkIndex = len(bookmarks) - 1
	}
}

func openSelectedBookmark() {
	bookmarks := bookmarksForCurrentBook()
	if len(bookmarks) == 0 {
		return
	}
	if app.bookmarkIndex >= len(bookmarks) {
		app.bookmarkIndex = len(bookmarks) - 1
	}
	app.reader.Goto(bookmarks[app.bookmarkIndex].Position)
	app.mode = modeReading
	syncCurrentBookState()
	app.statusMessage = "已跳转到书签"
}

func deleteSelectedBookmark() {
	bookmarks := bookmarksForCurrentBook()
	if len(bookmarks) == 0 {
		return
	}
	if app.bookmarkIndex >= len(bookmarks) {
		app.bookmarkIndex = len(bookmarks) - 1
	}
	list := append([]lib.Bookmark(nil), bookmarks[:app.bookmarkIndex]...)
	list = append(list, bookmarks[app.bookmarkIndex+1:]...)
	app.bookmarks.Books[app.currentFile] = list
	_ = lib.SaveBookmarks(app.bookmarks)
	if app.bookmarkIndex >= len(list) && len(list) > 0 {
		app.bookmarkIndex = len(list) - 1
	}
	app.statusMessage = "书签已删除"
}

func displayHelp() {
	app.showHelp = !app.showHelp
	app.showProgress = false
}

func displayProgress() {
	app.showProgress = !app.showProgress
	app.showHelp = false
}

func displayTOC() {
	if app.reader == nil {
		return
	}
	if app.mode == modeTOC {
		app.mode = modeReading
		app.tocNumber = ""
		return
	}
	app.mode = modeTOC
	app.tocIndex = app.reader.CurrentChapterIndex()
	app.tocNumber = ""
}

func appendTOCNumber(digit string) {
	app.tocNumber += digit
	if index, ok := parseTOCNumber(); ok {
		app.tocIndex = index
	}
}

func parseTOCNumber() (int, bool) {
	if app.tocNumber == "" {
		return 0, false
	}
	num, err := strconv.Atoi(app.tocNumber)
	if err != nil || num <= 0 {
		return 0, false
	}
	return num - 1, true
}

func updateTOCSelection(offset int) {
	app.tocIndex += offset
	pageSize := tocPageSize()
	total := 0
	if app.reader != nil {
		total = app.reader.CurrentChapterIndex()
		_ = total
	}
	if app.tocIndex < 0 {
		app.tocIndex = 0
	}
	// chapter count clamp comes from reader text output and goto
	if app.tocIndex < 0 {
		app.tocIndex = 0
	}
	if pageSize < 1 {
		pageSize = 1
	}
	app.tocNumber = ""
}

func openSelectedTOCChapter() {
	if app.reader == nil {
		return
	}
	if index, ok := parseTOCNumber(); ok {
		app.tocIndex = index
	}
	app.reader.GotoChapter(app.tocIndex)
	app.mode = modeReading
	app.tocNumber = ""
	app.statusMessage = "已跳转到章节"
	syncCurrentBookState()
}

func tocStatusText() string {
	if app.reader == nil {
		return ""
	}
	text := app.reader.GetTOCWithSelection(app.tocIndex, tocPageSize())
	if app.tocNumber == "" {
		return text
	}
	return text + "\nOpen chapter: " + app.tocNumber
}

func tocPageSize() int {
	if mainPanel != nil && mainPanel.Inner.Dy() > 0 {
		reservedLines := 4
		if app.tocNumber != "" {
			reservedLines++
		}
		available := mainPanel.Inner.Dy() - reservedLines
		if available > 0 {
			return available
		}
	}
	return 10
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
