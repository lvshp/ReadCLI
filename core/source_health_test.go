package core

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lvshp/ReadCLI/booksource"
)

func healthUISource(name string) booksource.Source {
	source := streamSource(name)
	source.TOC = booksource.TOCRule{ChapterList: "$.chapters[*]", ChapterName: "name", ChapterURL: "url"}
	source.Content = booksource.ContentRule{Content: "$.content"}
	return source
}

func initHealthUI(t *testing.T) (chan func(), *booksource.Client) {
	t.Helper()
	updates, client := initStreamingState(t)
	t.Cleanup(cancelSourceHealth)
	app.mode = modeSources
	return updates, client
}

func TestSourceHealthFixedTargetsPartialCancelAndIndependentSearch(t *testing.T) {
	updates, client := initHealthUI(t)
	fast, slow, excluded := healthUISource("fast"), healthUISource("slow"), healthUISource("excluded")
	app.online.sources = []booksource.Source{fast, slow, excluded}
	app.sourceManager.selected = map[string]bool{booksource.SourceKey(fast): true, booksource.SourceKey(slow): true}
	dir := os.Getenv("READCLI_DATA_DIR")
	if err := booksource.SaveSources(dir, app.online.sources); err != nil {
		t.Fatal(err)
	}
	release, started := make(chan struct{}), make(chan struct{})
	defer close(release)
	var excludedCalls atomic.Int32
	client.HTTPClient.Transport = streamRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() == "excluded.example.test" {
			excludedCalls.Add(1)
		}
		if req.URL.Hostname() == "slow.example.test" {
			close(started)
			select {
			case <-release:
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}
		return streamResponse(req, "测试书"), nil
	})
	openSourceHealth()
	if len(app.sourceHealth.targets) != 2 {
		t.Fatalf("targets=%+v", app.sourceHealth.targets)
	}
	// A later manager selection must not alter the check's fixed source list.
	app.sourceManager.selected = map[string]bool{booksource.SourceKey(excluded): true}
	searchCtx, searchCancel := context.WithCancel(context.Background())
	defer searchCancel()
	app.online.cancel = searchCancel
	searchGeneration := app.online.requestID
	startSourceHealth("测试", false)
	waitStreamingSignal(t, started)
	nextStreamingUpdate(t, updates)()
	if app.sourceHealth.completed != 1 || !app.sourceHealth.running {
		t.Fatalf("partial state=%+v", app.sourceHealth)
	}
	if managerSourceByKey(booksource.SourceKey(fast)).Management.Check == nil {
		t.Fatal("completed result not visible")
	}
	stored, err := booksource.LoadSources(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range stored {
		if source.Management != nil && source.Management.Check != nil {
			t.Fatal("saved entire source library after an individual result")
		}
	}
	release <- struct{}{}
	late := nextStreamingUpdate(t, updates)
	dispatchEvent("<Escape>")
	late()
	if app.sourceHealth.running || app.sourceHealth.completed != 1 || !app.sourceHealth.cancelled {
		t.Fatalf("cancel state=%+v", app.sourceHealth)
	}
	if source := managerSourceByKey(booksource.SourceKey(slow)); source.Management != nil && source.Management.Check != nil {
		t.Fatal("late result modified source")
	}
	if searchCtx.Err() != nil || app.online.requestID != searchGeneration {
		t.Fatal("health cancellation interrupted online search")
	}
	if excludedCalls.Load() != 0 {
		t.Fatal("changed selection leaked into running check")
	}
	stored, err = booksource.LoadSources(dir)
	if err != nil {
		t.Fatal(err)
	}
	checks := 0
	for _, source := range stored {
		if source.Management != nil && source.Management.Check != nil {
			checks++
		}
	}
	if checks != 1 {
		t.Fatalf("saved checks=%d, want only completed source", checks)
	}
	if !strings.Contains(buildSourceHealthPanel(), "其余未检查") {
		t.Fatal("canceled panel misrepresents pending sources")
	}
}

func TestSourceHealthStaticInputDetailsAndSaveRetry(t *testing.T) {
	updates, client := initHealthUI(t)
	disabled := false
	source := healthUISource("disabled")
	source.Enabled = &disabled
	app.online.sources = []booksource.Source{source}
	var requests atomic.Int32
	client.HTTPClient.Transport = streamRoundTripper(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, fmt.Errorf("static check must not request network")
	})
	openSourceHealth()
	handleSourceHealthEvent("2")
	if app.mode != modeSourceHealthInput || !strings.Contains(buildSourceHealthPanel(), "1 个书源") {
		t.Fatal("keyword input lacks explicit target count")
	}
	handleSourceHealthEvent("<Enter>")
	if app.sourceHealth.running || !strings.Contains(app.uiState.statusMessage, "关键词") {
		t.Fatal("empty search keyword accepted")
	}
	handleSourceHealthEvent("<Escape>")
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("READCLI_DATA_DIR", blocked)
	handleSourceHealthEvent("1")
	nextStreamingUpdate(t, updates)()
	if requests.Load() != 0 {
		t.Fatal("static check accessed network")
	}
	if app.online.sources[0].IsEnabled() {
		t.Fatal("health check enabled the source")
	}
	if !app.sourceHealth.saveFailed || !app.sourceHealth.dirty || app.sourceHealth.running {
		t.Fatalf("save failure state=%+v", app.sourceHealth)
	}
	if !strings.Contains(app.uiState.statusMessage, "重试") || !strings.Contains(buildSourceHealthPanel(), "尚未保存") {
		t.Fatal("save failure missing retry guidance")
	}
	handleSourceHealthEvent("<Enter>")
	panel := buildSourceHealthPanel()
	for _, needle := range []string{"静态规则", "耗时：", "时间：", "未执行"} {
		if !strings.Contains(panel, needle) {
			t.Fatalf("detail missing %q: %s", needle, panel)
		}
	}
	goodDir := t.TempDir()
	t.Setenv("READCLI_DATA_DIR", goodDir)
	handleSourceHealthEvent("s")
	if app.sourceHealth.saveFailed || app.sourceHealth.dirty {
		t.Fatal("save retry did not clear dirty state")
	}
	stored, err := booksource.LoadSources(goodDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Management == nil || stored[0].Management.Check == nil || stored[0].Management.Check.Stage != "rules" {
		t.Fatalf("saved result=%+v", stored)
	}
}

func TestSourceHealthOwnerAndNewRunRejectQueuedResults(t *testing.T) {
	updates, _ := initHealthUI(t)
	app.online.sources = []booksource.Source{healthUISource("source")}
	openSourceHealth()
	startSourceHealth("", false)
	old := nextStreamingUpdate(t, updates)
	cancelSourceHealth()
	startSourceHealth("", false)
	current := nextStreamingUpdate(t, updates)
	old()
	if app.sourceHealth.completed != 0 {
		t.Fatal("old generation changed new run")
	}
	current()
	if app.sourceHealth.completed != 1 || app.sourceHealth.running {
		t.Fatal("current result missing")
	}
	startSourceHealth("", false)
	oldOwner := nextStreamingUpdate(t, updates)
	initTestUIState()
	oldOwner()
	if app.sourceHealth.completed != 0 || len(app.online.sources) != 0 {
		t.Fatal("old app callback changed new app")
	}
}

func TestSourceHealthWorkersBoundedAndPendingRemainUnchecked(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	targets := make([]sourceHealthTarget, 8)
	for i := range targets {
		targets[i] = sourceHealthTarget{key: fmt.Sprint(i), source: healthUISource(fmt.Sprintf("source%d", i))}
	}
	started := make(chan struct{}, 8)
	done := make(chan struct{})
	var calls, published atomic.Int32
	go func() {
		streamSourceHealth(ctx, targets, func(ctx context.Context, source booksource.Source) booksource.SourceCheckResult {
			calls.Add(1)
			started <- struct{}{}
			<-ctx.Done()
			return booksource.SourceCheckResult{Status: "cancelled"}
		}, func(sourceHealthTarget, booksource.SourceCheckResult) { published.Add(1) })
		close(done)
	}()
	for i := 0; i < 4; i++ {
		waitStreamingSignal(t, started)
	}
	select {
	case <-started:
		t.Fatal("more than four concurrent source checks")
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	waitStreamingSignal(t, done)
	if calls.Load() != 4 || published.Load() != 0 {
		t.Fatalf("calls=%d publications=%d", calls.Load(), published.Load())
	}
}

func TestSourceHealthNavigationCancelsAndRejectsLateResult(t *testing.T) {
	updates, _ := initHealthUI(t)
	app.online.sources = []booksource.Source{healthUISource("source")}
	openSourceHealth()
	startSourceHealth("", false)
	late := nextStreamingUpdate(t, updates)
	generation := app.sourceHealth.generation
	handleSourceHealthEvent("q")
	if app.mode != modeSources || app.sourceHealth.running || app.sourceHealth.generation == generation {
		t.Fatal("leaving health view did not cancel")
	}
	late()
	if app.sourceHealth.completed != 0 || app.online.sources[0].Management != nil {
		t.Fatal("late result applied after navigation")
	}
}
