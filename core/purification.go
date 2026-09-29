package core

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/lvshp/ReadCLI/booksource"
	"github.com/lvshp/ReadCLI/lib"
	"github.com/lvshp/ReadCLI/purification"
	"github.com/lvshp/ReadCLI/reader"
)

type purificationState struct {
	returnMode               mode
	files                    []purification.FileInfo
	index                    int
	configPath, importsPath  string
	enabledCount, totalCount int
	errors                   []string
	lastError                string
	showErrors               bool
	errorScroll              int
}

func initPurification() {
	dir, err := lib.DataDirPath()
	if err == nil {
		err = purification.Ensure(dir)
	}
	if err != nil {
		setStatus(statusError, "初始化净化配置失败: "+err.Error())
	}
}

func openPurification() {
	previous := app.mode
	if previous == modeReading {
		syncCurrentBookState()
	}
	cancelOnlineRequest()
	app.purification = purificationState{returnMode: previous, lastError: app.purification.lastError}
	app.readingState.showHelp = false
	app.readingState.showProgress = false
	transitionTo(modePurification)
	initPurification()
	reloadPurification()
}

func closePurification() {
	cancelOnlineRequest()
	previous := app.purification.returnMode
	if previous != modeReading && previous != modeSources {
		previous = modeHome
	}
	transitionTo(previous)
	if previous == modeReading {
		if r, ok := app.reader.(*reader.OnlineReader); ok {
			openOnlineChapter(r.Index, r.ChapterOffset())
		}
	}
}

func reloadPurification() {
	dir, err := lib.DataDirPath()
	if err != nil {
		setStatus(statusError, err.Error())
		return
	}
	app.purification.configPath = filepath.Join(dir, "replace_rules", "main.json")
	app.purification.importsPath = filepath.Join(dir, "replace_rules", "imports")
	startOnlineRequest("正在读取净化规则", func(ctx context.Context) func() {
		rules, files, err := purification.Load(dir)
		if ctx.Err() != nil {
			return nil
		}
		return func() {
			app.purification.files = files
			app.purification.index = clamp(app.purification.index, 0, max(0, len(files)-1))
			app.purification.totalCount = len(rules)
			app.purification.enabledCount = 0
			for _, rule := range rules {
				if rule.IsEnabled {
					app.purification.enabledCount++
				}
			}
			app.purification.errors = nil
			if app.purification.lastError != "" {
				app.purification.errors = append(app.purification.errors, "上次净化: "+app.purification.lastError)
			}
			if err != nil {
				app.purification.errors = append(app.purification.errors, err.Error())
				setStatus(statusError, "净化配置错误: "+err.Error())
				return
			}
			setStatusf(statusInfo, "已读取 %d 条净化规则，启用 %d 条；返回在线阅读时应用", len(rules), app.purification.enabledCount)
		}
	})
}

func importPurification() {
	location := normalizeSourceImportLocation(app.uiState.input.value)
	if location == "" {
		setStatus(statusError, "请输入净化规则 JSON 文件或直链")
		return
	}
	dir, err := lib.DataDirPath()
	if err != nil {
		setStatus(statusError, err.Error())
		return
	}
	startOnlineRequest("正在导入净化规则", func(ctx context.Context) func() {
		info, err := purification.Import(ctx, dir, location)
		if err != nil {
			return func() {
				app.purification.errors = []string{err.Error()}
				setStatus(statusError, "导入净化规则失败: "+err.Error())
			}
		}
		return func() {
			transitionTo(modePurification)
			reloadPurification()
			setStatusf(statusInfo, "已保存 %d 条规则到 %s", info.RuleCount, info.Name)
		}
	})
}

// Rules are reloaded for each chapter so editing or restoring the data folder
// takes effect without rewriting cached source content or accumulating edits.
func purifyOnlineChapter(ctx context.Context, dir string, book booksource.Book, chapters []booksource.Chapter, text string) (string, []string, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	rules, _, err := purification.Load(dir)
	if err != nil {
		return text, nil, err
	}
	if len(rules) == 0 {
		return text, nil, nil
	}
	engine, err := purification.Compile(rules)
	if err != nil {
		return text, nil, err
	}
	target := purification.Target{BookName: book.Name, SourceURL: book.SourceURL}
	cleaned, err := engine.Apply(ctx, target, text)
	if err != nil {
		return text, nil, err
	}
	// Replacement rules may convert full-width or escaped HTML into markup.
	cleaned = booksource.NormalizeContent(cleaned)
	if strings.TrimSpace(cleaned) == "" && strings.TrimSpace(text) != "" {
		return text, nil, fmt.Errorf("净化后正文为空，已保留原正文")
	}
	titles, err := purifyOnlineTitles(ctx, engine, target, rules, chapters)
	if err != nil {
		return text, nil, err
	}
	return cleaned, titles, nil
}

func purifyOnlineTitles(ctx context.Context, engine *purification.Engine, target purification.Target, rules []purification.Rule, chapters []booksource.Chapter) ([]string, error) {
	hasTitles := false
	for _, rule := range rules {
		if rule.IsEnabled && rule.ScopeTitle {
			hasTitles = true
			break
		}
	}
	if !hasTitles {
		return nil, nil
	}
	target.Title = true
	titles := make([]string, len(chapters))
	for i, chapter := range chapters {
		title, err := engine.Apply(ctx, target, chapter.Name)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(title) == "" {
			title = chapter.Name
		}
		titles[i] = title
	}
	return titles, nil
}

func loadPurifiedOnlineTitles(ctx context.Context, dir string, book booksource.Book, chapters []booksource.Chapter) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	rules, _, err := purification.Load(dir)
	if err != nil || len(rules) == 0 {
		return nil, err
	}
	engine, err := purification.Compile(rules)
	if err != nil {
		return nil, err
	}
	return purifyOnlineTitles(ctx, engine, purification.Target{BookName: book.Name, SourceURL: book.SourceURL}, rules, chapters)
}

func showPurificationWarning(err error) {
	if err == nil {
		app.purification.errors = nil
		app.purification.lastError = ""
		return
	}
	app.purification.lastError = err.Error()
	app.purification.errors = []string{err.Error()}
	setStatus(statusError, "已保留原文；净化失败，按 P 查看: "+err.Error())
}
