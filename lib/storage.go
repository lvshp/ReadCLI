package lib

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Config struct {
	Theme             string `json:"theme"`
	DisplayLines      int    `json:"display_lines"`
	ShowBorder        bool   `json:"show_border"`
	SelectedBookshelf int    `json:"selected_bookshelf"`
}

type BookshelfBook struct {
	Path            string `json:"path"`
	Title           string `json:"title"`
	Format          string `json:"format"`
	ProgressPos     int    `json:"progress_pos"`
	ProgressTotal   int    `json:"progress_total"`
	ProgressPercent int    `json:"progress_percent"`
	CurrentChapter  string `json:"current_chapter"`
	LastReadAt      string `json:"last_read_at"`
	ImportedAt      string `json:"imported_at"`
}

type BookshelfStore struct {
	Books []BookshelfBook `json:"books"`
}

type Bookmark struct {
	Path          string `json:"path"`
	Position      int    `json:"position"`
	Chapter       string `json:"chapter"`
	Snippet       string `json:"snippet"`
	CreatedAt     string `json:"created_at"`
	ProgressTotal int    `json:"progress_total"`
}

type BookmarkStore struct {
	Books map[string][]Bookmark `json:"books"`
}

const (
	appName       = "readcli"
	dataDirName   = ".readcli"
	legacyRootDir = "readcli"
	legacyAppName = "glance"
	dataDirEnv    = "READCLI_DATA_DIR"
)

func configFilePath() (string, error) { return glanceDataPath("config.json") }
func bookshelfFilePath() (string, error) {
	return glanceDataPath("bookshelf.json")
}
func bookmarksFilePath() (string, error) {
	return glanceDataPath("bookmarks.json")
}

func glanceDataPath(name string) (string, error) {
	if dir := strings.TrimSpace(os.Getenv(dataDirEnv)); dir != "" {
		return filepath.Join(dir, name), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(homeDir, dataDirName, name), nil
}

func DataDirPath() (string, error) {
	return glanceDataPath("")
}

func legacyRootDataPath(name string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, legacyRootDir, name), nil
}

func legacyReadcliDataPath(name string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, appName, name), nil
}

func legacyGlanceDataPath(name string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, legacyAppName, name), nil
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		Theme:        "vscode",
		DisplayLines: 8,
		ShowBorder:   true,
	}
	if err := loadJSON(configFilePath, cfg); err != nil {
		return nil, err
	}
	if cfg.DisplayLines < 1 {
		cfg.DisplayLines = 8
	}
	if cfg.Theme == "" {
		cfg.Theme = "vscode"
	}
	return cfg, nil
}

func SaveConfig(cfg *Config) error {
	if cfg == nil {
		cfg = &Config{Theme: "vscode", DisplayLines: 8, ShowBorder: true}
	}
	return saveJSON(configFilePath, cfg)
}

func LoadBookshelf() (*BookshelfStore, error) {
	store := &BookshelfStore{Books: []BookshelfBook{}}
	return store, loadJSON(bookshelfFilePath, store)
}

func SaveBookshelf(store *BookshelfStore) error {
	if store == nil {
		store = &BookshelfStore{Books: []BookshelfBook{}}
	}
	return saveJSON(bookshelfFilePath, store)
}

func LoadBookmarks() (*BookmarkStore, error) {
	store := &BookmarkStore{Books: map[string][]Bookmark{}}
	err := loadJSON(bookmarksFilePath, store)
	if store.Books == nil {
		store.Books = map[string][]Bookmark{}
	}
	return store, err
}

func SaveBookmarks(store *BookmarkStore) error {
	if store == nil {
		store = &BookmarkStore{Books: map[string][]Bookmark{}}
	}
	if store.Books == nil {
		store.Books = map[string][]Bookmark{}
	}
	return saveJSON(bookmarksFilePath, store)
}

func loadJSON(pathFn func() (string, error), dest interface{}) error {
	path, err := pathFn()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		legacyPaths := []func(string) (string, error){
			legacyRootDataPath,
			legacyReadcliDataPath,
			legacyGlanceDataPath,
		}
		for _, pathFn := range legacyPaths {
			legacyPath, legacyErr := pathFn(filepath.Base(path))
			if legacyErr != nil {
				continue
			}
			data, err = os.ReadFile(legacyPath)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			break
		}
		if os.IsNotExist(err) {
			return nil
		}
	}
	if err != nil {
		return err
	}

	return json.Unmarshal(data, dest)
}

func saveJSON(pathFn func() (string, error), value interface{}) error {
	path, err := pathFn()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func UpsertBookshelfBook(store *BookshelfStore, book BookshelfBook) {
	for i := range store.Books {
		if store.Books[i].Path == book.Path {
			importedAt := store.Books[i].ImportedAt
			if importedAt == "" {
				importedAt = nowRFC3339()
			}
			book.ImportedAt = importedAt
			store.Books[i] = book
			return
		}
	}
	if book.ImportedAt == "" {
		book.ImportedAt = nowRFC3339()
	}
	store.Books = append(store.Books, book)
}

func RemoveBookshelfBook(store *BookshelfStore, path string) {
	filtered := store.Books[:0]
	for _, book := range store.Books {
		if book.Path != path {
			filtered = append(filtered, book)
		}
	}
	store.Books = filtered
}

func FindBookshelfBook(store *BookshelfStore, path string) (BookshelfBook, bool) {
	for _, book := range store.Books {
		if book.Path == path {
			return book, true
		}
	}
	return BookshelfBook{}, false
}

func SortBooks(books []BookshelfBook, order string) {
	sort.SliceStable(books, func(i, j int) bool {
		switch order {
		case "title":
			return strings.ToLower(books[i].Title) < strings.ToLower(books[j].Title)
		case "imported":
			return books[i].ImportedAt > books[j].ImportedAt
		default:
			return books[i].LastReadAt > books[j].LastReadAt
		}
	})
}

func FilterBooks(books []BookshelfBook, filter string) []BookshelfBook {
	if filter == "" || filter == "all" {
		return append([]BookshelfBook(nil), books...)
	}

	filtered := make([]BookshelfBook, 0, len(books))
	for _, book := range books {
		switch filter {
		case "txt", "epub":
			if strings.EqualFold(book.Format, filter) {
				filtered = append(filtered, book)
			}
		case "unread":
			if book.ProgressPos <= 0 {
				filtered = append(filtered, book)
			}
		case "reading":
			if book.ProgressPos > 0 && book.ProgressPercent < 100 {
				filtered = append(filtered, book)
			}
		case "finished":
			if book.ProgressPercent >= 100 {
				filtered = append(filtered, book)
			}
		}
	}
	return filtered
}

func nowRFC3339() string {
	return time.Now().Format(time.RFC3339)
}
