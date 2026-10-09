package core

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lvshp/ReadCLI/booksource"
	"github.com/rivo/tview"
)

func sourceManagerUIFixture(count int) []booksource.Source {
	sources := make([]booksource.Source, count)
	for i := range sources {
		sources[i] = booksource.Source{Name: fmt.Sprintf("源%04d", i), URL: fmt.Sprintf("https://source-%d.invalid", i), SearchURL: "/search"}
	}
	return sources
}

func TestSourceManagerUIPagesLargeListsAndKeepsSelectedRow(t *testing.T) {
	for _, size := range [][2]int{{48, 16}, {80, 24}, {140, 40}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			initTestUIState()
			app.mode = modeSources
			app.online.sources = sourceManagerUIFixture(200)
			app.sourceManager.sort = "name"
			app.sourceManager.selected = map[string]bool{booksource.SourceKey(app.online.sources[0]): true, booksource.SourceKey(app.online.sources[199]): true}
			app.online.sourceIndex = 199
			disabled := false
			app.online.sources[199].Enabled = &disabled
			app.online.sources[199].LoginURL = "/login"
			got := drawSourceSwitchUI(t, size[0], size[1])
			for _, want := range []string{"名称排序", "匹配200", "已选2", "> ☑ ☆ 停 未登 源0199", "g 分组"} {
				if !strings.Contains(got, want) {
					t.Fatalf("missing %q at %v:\n%s", want, size, got)
				}
			}
			panel := buildSourceManagerPanel()
			if strings.Count(panel, "\n") >= mainContentHeight || strings.Contains(panel, "源0000") {
				t.Fatalf("list did not page to its selected row:\n%s", panel)
			}
			pageSize := max(1, mainContentHeight-managerListHeaderLines())
			handleSourceManagerEvent("<PageUp>")
			if app.online.sourceIndex != 199-pageSize {
				t.Fatalf("PageUp moved %d rows, want %d", 199-app.online.sourceIndex, pageSize)
			}
		})
	}
}

func TestSourceManagerUIConfirmationSmallScreenShowsFixedScope(t *testing.T) {
	for _, collection := range []bool{false, true} {
		initTestUIState()
		app.mode = modeSourceConfirm
		app.online.sources = sourceManagerUIFixture(200)
		keys := make([]string, len(app.online.sources))
		for i, s := range app.online.sources {
			keys[i] = booksource.SourceKey(s)
		}
		app.sourceManager.pending = sourceManagerOperation{kind: "delete", label: "删除", keys: keys}
		wants := []string{"200个", "源0000", "书架与缓存保留", "Enter/y", "Esc/q"}
		if collection {
			app.online.sources = app.online.sources[:3]
			for i := range app.online.sources {
				app.online.sources[i].Management = &booksource.SourceManagement{Collections: []booksource.SourceCollection{{ID: "collection-a", Name: "相同合集"}}}
			}
			app.online.sources[1].Management.Collections = append(app.online.sources[1].Management.Collections, booksource.SourceCollection{ID: "collection-b", Name: "相同合集"})
			app.online.sources[2].Management.Standalone = true
			app.sourceManager.pending = sourceManagerOperation{kind: "collection-delete", label: "移除合集：" + strings.Repeat("很长", 30), keys: keys[:3], collectionID: "collection-a"}
			wants = []string{"3个", "源0000", "删除 1 个独有源", "保留 2 个共享/原有源", "书架与缓存保留", "Enter/y", "Esc/q"}
		}
		got := drawSourceSwitchUI(t, 48, 16)
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Fatalf("confirmation hides %q (collection=%v):\n%s", want, collection, got)
			}
		}
	}
}

func TestSourceManagerUIImportPreviewPagesAndPreservesPolicy(t *testing.T) {
	for _, size := range [][2]int{{48, 16}, {80, 24}, {140, 40}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			initTestUIState()
			sources := sourceManagerUIFixture(301)
			app.online.sources = sources[:1]
			sources[300].Type = 1
			managerOpenPreview(sources, "/fixture/sources.json")
			app.sourceManager.importIndex = 300
			got := drawSourceSwitchUI(t, size[0], size[1])
			for _, want := range []string{"导入预览", "新增300 更新1 不支持1", "已选301/301", "新源停用(e)", "> ☑ 新增! 源0300", "Enter导入", "Esc取消"} {
				if !strings.Contains(got, want) {
					t.Fatalf("preview hides %q at %v:\n%s", want, size, got)
				}
			}
			pageSize := max(1, mainContentHeight-managerImportHeaderLines())
			handleSourceManagerEvent("<PageUp>")
			if app.sourceManager.importIndex != 300-pageSize {
				t.Fatal("import PageUp does not match visible page size")
			}
			handleSourceManagerEvent("e")
			if !strings.Contains(buildSourceManagerPanel(), "新源启用(e)") {
				t.Fatal("preview does not reflect new-source enablement choice")
			}
		})
	}
}

func TestSourceManagerUIGroupsAndActionsShowActualScope(t *testing.T) {
	initTestUIState()
	app.online.sources = sourceManagerUIFixture(100)
	for i := range app.online.sources {
		id := "collection-aaaaaaaa"
		if i > 49 {
			id = "collection-bbbbbbbb"
		}
		app.online.sources[i].Management = &booksource.SourceManagement{Collections: []booksource.SourceCollection{{ID: id, Name: "同名合集"}}}
	}
	app.mode = modeSourceGroups
	app.sourceManager.groupIndex = len(managerGroups()) - 1
	got := drawSourceSwitchUI(t, 80, 24)
	for _, want := range []string{"合集", "#aaaaaaaa", "#bbbbbbbb", "(50)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("group disambiguation missing %q:\n%s", want, got)
		}
	}
	app.mode = modeSourceActions
	app.sourceManager.query = "源000"
	app.sourceManager.selected = map[string]bool{booksource.SourceKey(app.online.sources[99]): true}
	got = drawSourceSwitchUI(t, 48, 16)
	for _, want := range []string{"已选 1 个", "1 启用", "2 停用", "3 收藏", "4 取消收藏", "5 加标签", "6 移除本地标签", "7 删除"} {
		if !strings.Contains(got, want) {
			t.Fatalf("batch scope/action missing %q:\n%s", want, got)
		}
	}
	managerClearSelection()
	if !strings.Contains(buildSourceManagerPanel(), "当前筛选 10 个") {
		t.Fatal("unselected batch action scope must be the filtered list")
	}
}

func TestSourceManagerUIDetailsFollowFilteredSortedSource(t *testing.T) {
	initTestUIState()
	app.mode = modeSources
	sources := sourceManagerUIFixture(3)
	sources[0].Name, sources[1].Name = "Z keep", "A [red]keep"
	sources[1].Group = "[blue]分组"
	sources[1].Management = &booksource.SourceManagement{Standalone: true, Tags: []string{"[yellow]本地"}, Collections: []booksource.SourceCollection{{ID: "bundle-12345678", Name: "[green]合集"}}, Check: &booksource.SourceCheckResult{Status: "ok", Stage: "search", DurationMS: 123, CheckedAt: time.Now(), Detail: "[red]检查详情"}}
	app.online.sources = sources
	app.sourceManager.query, app.sourceManager.sort = "keep", "name"
	mainContentHeight = 20
	right.SetRect(0, 0, 64, 22)
	panel := buildSourceManagerRightPanel()
	got := drawSourceManagerText(t, panel, 64, 22)
	for _, want := range []string{"A ［red］keep", "原分组：［blue］分组", "本地标签：［yellow］本地", "历史/独立来源", "［green］合集 #12345678", "最后检查", "123ms", "［red］检查详情"} {
		if !strings.Contains(got, want) {
			t.Fatalf("detail missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Z keep") || strings.Contains(got, "[red]") {
		t.Fatalf("detail selected raw index or interpreted markup:\n%s", got)
	}
}

func TestSourceManagerUILiveFilterAcceptsRealSpaceKey(t *testing.T) {
	initTestUIState()
	app.mode = modeSources
	app.online.sources = sourceManagerUIFixture(2)
	app.online.sources[0].Name, app.online.sources[1].Name = "alpha beta", "alpha"
	dispatchEvent("f")
	for _, r := range "alpha beta" {
		dispatchEvent(tcellKeyEventID(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)))
	}
	if app.sourceManager.query != "alpha beta" || len(managerVisibleIndices()) != 1 {
		t.Fatalf("live filter mismatch: %q", app.sourceManager.query)
	}
	got := drawSourceSwitchUI(t, 48, 16)
	for _, want := range []string{"alpha beta|", "匹配1", "alpha beta"} {
		if !strings.Contains(got, want) {
			t.Fatalf("live filter hides %q:\n%s", want, got)
		}
	}
	app.uiState.input.value, app.uiState.input.cursor = strings.Repeat("prefix", 30)+"[red]", 185
	got = drawSourceManagerText(t, buildSourceManagerPanel(), mainContentWidth, mainContentHeight)
	if !strings.Contains(got, "［red］|") || strings.Contains(got, "[red]") {
		t.Fatalf("long filter lost cursor or interpreted markup:\n%s", got)
	}
}

func drawSourceManagerText(t *testing.T, text string, width, height int) string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(width, height)
	view := tview.NewTextView().SetDynamicColors(true).SetText(termuiStyleToTview(text))
	view.SetRect(0, 0, width, height)
	view.Draw(screen)
	return simulationText(screen, width, height)
}
