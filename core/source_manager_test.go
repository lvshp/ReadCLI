package core

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/lvshp/ReadCLI/booksource"
)

func initManagerTest(t *testing.T, count int) {
	t.Helper()
	t.Setenv("READCLI_DATA_DIR", t.TempDir())
	initTestUIState()
	app.mode = modeSources
	for i := 0; i < count; i++ {
		s := streamSource(fmt.Sprintf("source-%04d", i))
		s.Name = fmt.Sprintf("测试书源 %04d", i)
		s.Group = "分组甲"
		if i%2 != 0 {
			s.Group = "分组乙"
		}
		app.online.sources = append(app.online.sources, s)
	}
}

func managerType(text string) {
	for _, r := range text {
		dispatchEvent(tcellKeyEventID(tcell.NewEventKey(tcell.KeyRune, r, 0)))
	}
}

func TestManagerThousandSourcesFilterPagingAndStableMutation(t *testing.T) {
	initManagerTest(t, 1200)
	dispatchEvent("<End>")
	if app.online.sourceIndex != 1199 {
		t.Fatal("cannot navigate to last source")
	}
	dispatchEvent("<PageUp>")
	if app.online.sourceIndex >= 1199 {
		t.Fatal("page navigation missing")
	}
	dispatchEvent("f")
	for _, r := range "118" {
		dispatchEvent(string(r))
	}
	if len(managerVisibleIndices()) != 12 {
		t.Fatalf("filter count %d", len(managerVisibleIndices()))
	}
	dispatchEvent("<Enter>")
	dispatchEvent("<Down>")
	key := booksource.SourceKey(*selectedSource())
	dispatchEvent("<Space>")
	if managerSourceByKey(key).IsEnabled() {
		t.Fatal("filtered selection toggled wrong source")
	}
	if booksource.SourceKey(*selectedSource()) != key {
		t.Fatal("toggle jumped away from selected source")
	}
	if !app.online.sources[0].IsEnabled() {
		t.Fatal("unrelated source changed")
	}
	dispatchEvent("*")
	if !managerSourceByKey(key).Management.Favorite || booksource.SourceKey(*selectedSource()) != key {
		t.Fatal("favorite sorting lost selected source")
	}
	loaded, err := booksource.LoadSources(os.Getenv("READCLI_DATA_DIR"))
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1200 {
		t.Fatal("persistence lost sources")
	}
}

func TestManagerBatchScopeDeleteConfirmUndoAndSaveFailure(t *testing.T) {
	initManagerTest(t, 20)
	app.sourceManager.group = "分组乙"
	dispatchEvent("v")
	dispatchEvent("a")
	if managerSelectedCount() != 10 {
		t.Fatal("select all expanded beyond filter")
	}
	dispatchEvent("d")
	if app.mode != modeSourceConfirm || len(app.online.sources) != 20 || len(app.sourceManager.pending.keys) != 10 {
		t.Fatal("delete did not preview exact scope")
	}
	dispatchEvent("<Escape>")
	if len(app.online.sources) != 20 {
		t.Fatal("cancel deleted data")
	}
	dispatchEvent("d")
	dispatchEvent("y")
	if len(app.online.sources) != 10 {
		t.Fatal("wrong delete count")
	}
	for _, s := range app.online.sources {
		if s.Group != "分组甲" {
			t.Fatal("wrong source retained")
		}
	}
	dispatchEvent("u")
	if len(app.online.sources) != 20 {
		t.Fatal("undo did not restore data")
	}
	bad := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(bad, []byte("not a dir"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("READCLI_DATA_DIR", bad)
	before := selectedSource().IsEnabled()
	dispatchEvent("<Space>") // batch mode selects only; exit it before toggling
	dispatchEvent("v")
	dispatchEvent("<Space>")
	if selectedSource().IsEnabled() != before {
		t.Fatal("failed save modified live sources")
	}
}

func TestManagerImportPreviewHasNoSideEffectsAndPreservesSettings(t *testing.T) {
	initManagerTest(t, 1)
	s := &app.online.sources[0]
	off := false
	s.Enabled = &off
	s.Management = &booksource.SourceManagement{Favorite: true, Tags: []string{"常用"}}
	replacement := *s
	replacement.Name = "规则更新"
	replacement.Enabled = nil
	replacement.Management = nil
	added := streamSource("new")
	managerOpenPreview([]booksource.Source{replacement, added}, "collection.json")
	if len(app.online.sources) != 1 || app.online.sources[0].Name == replacement.Name {
		t.Fatal("preview already changed library")
	}
	dispatchEvent("<Escape>")
	if len(app.online.sources) != 1 {
		t.Fatal("cancel imported sources")
	}
	managerOpenPreview([]booksource.Source{replacement, added}, "collection.json")
	dispatchEvent("<Enter>")
	if len(app.online.sources) != 2 || app.online.sources[0].IsEnabled() || app.online.sources[1].IsEnabled() {
		t.Fatal("import enablement policy changed")
	}
	if !app.online.sources[0].Management.Favorite || !managerContains(app.online.sources[0].Management.Tags, "常用") {
		t.Fatal("import lost preferences")
	}
	if len(app.online.sources[0].Management.Collections) != 1 {
		t.Fatal("collection not recorded")
	}
	dispatchEvent("u")
	if len(app.online.sources) != 1 || app.online.sources[0].Name == replacement.Name {
		t.Fatal("import undo failed")
	}
}

func TestManagerImportSelectionAndEnableNew(t *testing.T) {
	initManagerTest(t, 0)
	incoming := []booksource.Source{streamSource("one"), streamSource("two")}
	managerOpenPreview(incoming, "import.json")
	dispatchEvent("A")
	dispatchEvent("<Enter>")
	if app.mode != modeSourceImportPreview || len(app.online.sources) != 0 {
		t.Fatal("empty selection imported")
	}
	dispatchEvent("f")
	managerType("two")
	dispatchEvent("<Enter>")
	dispatchEvent("a")
	dispatchEvent("e")
	dispatchEvent("<Enter>")
	if len(app.online.sources) != 1 || app.online.sources[0].URL != incoming[1].URL || !app.online.sources[0].IsEnabled() {
		t.Fatal("filtered preview selection ignored")
	}
}

func TestManagerCollectionRemovalPreservesSharedAndUndo(t *testing.T) {
	initManagerTest(t, 3)
	for i := 0; i < 2; i++ {
		app.online.sources[i].Management = &booksource.SourceManagement{Collections: []booksource.SourceCollection{{ID: "a", Name: "合集A"}}}
	}
	app.online.sources[1].Management.Collections = append(app.online.sources[1].Management.Collections, booksource.SourceCollection{ID: "b", Name: "合集B"})
	dispatchEvent("g")
	for i, o := range managerGroups() {
		if o.kind == "collection" && o.id == "a" {
			app.sourceManager.groupIndex = i
		}
	}
	dispatchEvent("d")
	if app.mode != modeSourceConfirm || len(app.online.sources) != 3 {
		t.Fatal("collection deletion skipped preview")
	}
	dispatchEvent("y")
	if len(app.online.sources) != 2 || len(app.online.sources[0].Management.Collections) != 1 || app.online.sources[0].Management.Collections[0].ID != "b" {
		t.Fatal("shared membership incorrectly removed")
	}
	dispatchEvent("u")
	if len(app.online.sources) != 3 {
		t.Fatal("collection undo missing")
	}
}

func TestManagerTagsPersistAndFilter(t *testing.T) {
	initManagerTest(t, 2)
	dispatchEvent("v")
	dispatchEvent("x")
	dispatchEvent("b")
	dispatchEvent("5")
	managerType("我的 标签")
	dispatchEvent("<Enter>")
	dispatchEvent("y")
	if !managerContains(booksource.SourceGroups(app.online.sources[0]), "我的 标签") || managerContains(booksource.SourceGroups(app.online.sources[1]), "我的 标签") {
		t.Fatal("tag batch affected wrong source")
	}
	managerApplyGroup(sourceManagerGroup{kind: "group", id: "我的 标签"})
	if len(managerVisibleIndices()) != 1 {
		t.Fatal("custom group filter failed")
	}
	dispatchEvent("b")
	dispatchEvent("6")
	managerType("我的 标签")
	dispatchEvent("<Enter>")
	dispatchEvent("y")
	loaded, err := booksource.LoadSources(os.Getenv("READCLI_DATA_DIR"))
	if err != nil {
		t.Fatal(err)
	}
	if managerContains(booksource.SourceGroups(loaded[0]), "我的 标签") {
		t.Fatal("removed tag returned after load")
	}
}

func TestManagerScopedSearchNeverFallsBackToAll(t *testing.T) {
	updates, client := initStreamingState(t)
	app.mode = modeSources
	app.online.sources = []booksource.Source{streamSource("favorite"), streamSource("outside")}
	app.online.sources[0].Management = &booksource.SourceManagement{Favorite: true}
	client.HTTPClient.Transport = streamRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() != "favorite.example.test" {
			return nil, fmt.Errorf("search escaped scope: %s", req.URL.Host)
		}
		return streamResponse(req, "范围内书籍"), nil
	})
	managerStartSearch("favorite")
	app.uiState.input.value = "书籍"
	runOnlineSearch()
	nextStreamingUpdate(t, updates)()
	if app.online.searchTotal != 1 || len(app.online.results) != 1 || len(app.online.errors) != 0 {
		t.Fatal("favorite search scope failed")
	}
	dispatchEvent("/")
	if app.mode != modeOnlineSearchInput || !app.online.scopeKeys[booksource.SourceKey(app.online.sources[0])] || len(app.online.scopeKeys) != 1 {
		t.Fatal("changing keyword expanded scope")
	}
	app.uiState.input.value = "第二个词"
	runOnlineSearch()
	nextStreamingUpdate(t, updates)()
	if app.online.searchTotal != 1 || len(app.online.errors) != 0 {
		t.Fatal("second search escaped favorite scope")
	}
	app.mode = modeSources
	app.sourceManager.group = "空分组"
	managerStartSearch("view")
	if app.mode != modeSources {
		t.Fatal("empty scope fell back to full search")
	}
	app.online.scopeKeys = map[string]bool{}
	app.online.scopeURL = ""
	searchOnlinePage(1)
	if app.online.searching {
		t.Fatal("explicit empty scope launched all sources")
	}
}

func TestManagerUndoKeepsNewChecksOnlyForSameRules(t *testing.T) {
	initManagerTest(t, 1)
	key := booksource.SourceKey(app.online.sources[0])
	dispatchEvent("*")
	managerSourceByKey(key).Management.Check = &booksource.SourceCheckResult{Status: "ok", Stage: "search", Detail: "新检查"}
	dispatchEvent("u")
	if managerMeta(app.online.sources[0]).Favorite || managerMeta(app.online.sources[0]).Check == nil || managerMeta(app.online.sources[0]).Check.Detail != "新检查" {
		t.Fatal("undo lost newer check for unchanged rules")
	}
	updated := app.online.sources[0]
	updated.SearchURL = "/updated-search"
	managerOpenPreview([]booksource.Source{updated}, "update.json")
	managerConfirmImport()
	if managerMeta(app.online.sources[0]).Check != nil {
		t.Fatal("new rules retained stale check")
	}
	app.online.sources[0].Management.Check = &booksource.SourceCheckResult{Status: "error", Detail: "新规则检查"}
	dispatchEvent("u")
	if app.online.sources[0].SearchURL == updated.SearchURL || managerMeta(app.online.sources[0]).Check.Detail != "新检查" {
		t.Fatal("undo applied new-rule check to old rules")
	}
}

func TestManagerSearchScopeCancelReturnsToEntry(t *testing.T) {
	initManagerTest(t, 1)
	for _, mode := range []mode{modeHome, modeSources, modeOnlineResults} {
		app.mode = mode
		dispatchEvent("s")
		if app.mode != modeSourceSearchScope {
			t.Fatal("search scope menu missing")
		}
		dispatchEvent("<Escape>")
		if app.mode != mode {
			t.Fatalf("scope cancel lost original mode %s", mode)
		}
	}
}

func TestManagerActualKeyMappingAndInputEscape(t *testing.T) {
	initManagerTest(t, 2)
	if tcellKeyEventID(tcell.NewEventKey(tcell.KeyPgDn, 0, 0)) != "<PageDown>" || tcellKeyEventID(tcell.NewEventKey(tcell.KeyPgUp, 0, 0)) != "<PageUp>" {
		t.Fatal("page keys not mapped")
	}
	app.sourceManager.query = "旧"
	dispatchEvent("f")
	dispatchEvent("q")
	if app.mode != modeSourceFilter || !strings.Contains(app.sourceManager.query, "q") {
		t.Fatal("filter q treated as navigation")
	}
	dispatchEvent("<Escape>")
	if app.mode != modeSources || app.sourceManager.query != "旧" {
		t.Fatal("cancel did not restore query")
	}
	// Existing import parsing continues to handle a local file without network.
	path := filepath.Join(t.TempDir(), "sources.json")
	if err := os.WriteFile(path, []byte(`[{"bookSourceUrl":"test-source","bookSourceName":"本地","searchUrl":"/search"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := booksource.Import(context.Background(), path); err != nil {
		t.Fatal(err)
	}
}
