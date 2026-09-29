package core

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/lvshp/ReadCLI/booksource"
	"github.com/lvshp/ReadCLI/lib"
	"github.com/lvshp/ReadCLI/reader"
)

func initSourceSwitchUIFixture() {
	initTestUIState()
	oldBook := booksource.Book{Name: "原书", SourceName: "旧聚合", URL: "https://old.example/book", SourceURL: "https://old.example", Author: "原作者", LastChapter: "旧站 最新九章"}
	original := lib.BookshelfBook{Path: "online:old", Title: oldBook.Name, Format: "online", Online: &oldBook, ChapterIndex: 1, CurrentChapter: "第二章", ChapterOffset: 0.35}
	app.bookshelf.Books = []lib.BookshelfBook{original}
	app.currentFile = original.Path
	app.reader = reader.NewOnlineReader(oldBook, []booksource.Chapter{{Name: "第一章", URL: "/1"}, {Name: "第二章", URL: "/2"}}, 1, "原正文\n原第二行")
	app.sourceSwitch = sourceSwitchState{original: original, returnMode: modeReading, page: 1}
	app.mode = modeSourceSwitch
}

func drawSourceSwitchUI(t *testing.T, width, height int) string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(width, height)
	root.SetRect(0, 0, width, height)
	applyLayout(width, height)
	root.Draw(screen)
	return simulationText(screen, width, height)
}

func TestSourceSwitchUICandidateMetadataFitsSmallTerminal(t *testing.T) {
	for _, size := range [][2]int{{48, 16}, {80, 24}, {140, 40}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			initSourceSwitchUIFixture()
			for i := 0; i < 7; i++ {
				app.sourceSwitch.results = append(app.sourceSwitch.results, booksource.Book{Name: fmt.Sprintf("版本%d", i), SourceName: "新聚合", Author: "[red]作者", LastChapter: "顶点 最新十章"})
			}
			app.sourceSwitch.index = 6
			got := drawSourceSwitchUI(t, size[0], size[1])
			for _, want := range []string{"> 版本6", "新聚合", "［red］作者", "顶点 最新十章"} {
				if !strings.Contains(got, want) {
					t.Fatalf("missing %q at %v:\n%s", want, size, got)
				}
			}
			if strings.Contains(got, "[red]") || strings.Contains(got, "fg:cyan") {
				t.Fatalf("unescaped metadata or raw style:\n%s", got)
			}
		})
	}
}

func TestSourceSwitchUIConfirmationKeepsWarningAndProgressVisible(t *testing.T) {
	for _, size := range [][2]int{{48, 16}, {80, 24}, {140, 40}} {
		for _, matched := range []bool{true, false} {
			t.Run(fmt.Sprint(size, matched), func(t *testing.T) {
				initSourceSwitchUIFixture()
				app.mode = modeSourceSwitchConfirm
				app.sourceSwitch.prepared = &sourceSwitchPlan{book: booksource.Book{Name: "新书", SourceName: "新聚合", LastChapter: "顶点 最新十章"}, chapters: []booksource.Chapter{{Name: "第二章", URL: "/2"}}, index: 0, offset: 0.35, matched: matched, text: "新正文"}
				got := drawSourceSwitchUI(t, size[0], size[1])
				for _, want := range []string{"旧聚合", "新聚合", "第二章", "35%", "Enter/y", "Esc/q"} {
					if !strings.Contains(got, want) {
						t.Fatalf("missing %q:\n%s", want, got)
					}
				}
				if !matched && (!strings.Contains(got, "未匹配") || !strings.Contains(got, "近似定位")) {
					t.Fatalf("approximate chapter warning is hidden:\n%s", got)
				}
			})
		}
	}
}

func TestSourceSwitchUIBusyCancelRestoresOriginalScene(t *testing.T) {
	for _, origin := range []mode{modeHome, modeReading} {
		for _, key := range []string{"<Escape>", "q", "<C-c>"} {
			t.Run(string(origin)+key, func(t *testing.T) {
				initSourceSwitchUIFixture()
				app.sourceSwitch.returnMode = origin
				before := app.bookshelf.Books[0]
				previousReader := app.reader
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				app.online.cancel = cancel
				app.online.busy = "正在加载候选章节"
				dispatchEvent(key)
				if ctx.Err() == nil || app.online.busy != "" || app.mode != origin {
					t.Fatalf("cancel failed: mode=%s busy=%q context=%v", app.mode, app.online.busy, ctx.Err())
				}
				if app.reader != previousReader || len(app.bookshelf.Books) != 1 || !reflect.DeepEqual(app.bookshelf.Books[0], before) {
					t.Fatal("cancel changed original book or reader")
				}
			})
		}
	}
}

func TestSourceSwitchUIConfirmationBackAndCandidateNavigation(t *testing.T) {
	initSourceSwitchUIFixture()
	app.sourceSwitch.results = []booksource.Book{{Name: "一"}, {Name: "二"}}
	dispatchEvent("<Down>")
	dispatchEvent("<Down>")
	if app.sourceSwitch.index != 1 {
		t.Fatal("candidate selection did not clamp to the last result")
	}
	app.sourceSwitch.prepared = &sourceSwitchPlan{book: app.sourceSwitch.results[1]}
	app.mode = modeSourceSwitchConfirm
	dispatchEvent("q")
	if app.mode != modeSourceSwitch || app.sourceSwitch.prepared != nil || app.sourceSwitch.index != 1 || len(app.sourceSwitch.results) != 2 {
		t.Fatal("confirmation cancel did not retain the candidate list")
	}
	dispatchEvent("<Up>")
	if app.sourceSwitch.index != 0 {
		t.Fatal("candidate selection did not move up")
	}
}

func TestSourceSwitchUIBookshelfShowsCurrentBackend(t *testing.T) {
	initSourceSwitchUIFixture()
	app.mode = modeHome
	got := buildRightPanel(currentTheme())
	for _, want := range []string{"旧聚合", "原作者", "旧站 最新九章", "C 更换书源"} {
		if !strings.Contains(got, want) {
			t.Fatalf("bookshelf details missing %q: %s", want, got)
		}
	}
}
