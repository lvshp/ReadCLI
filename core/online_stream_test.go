package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lvshp/ReadCLI/booksource"
	"github.com/lvshp/ReadCLI/reader"
)

type streamRoundTripper func(*http.Request) (*http.Response, error)

func (f streamRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func streamSource(name string) booksource.Source {
	return booksource.Source{URL: "https://" + name + ".example.test", Name: name, SearchURL: "/search?q={{key}}&page={{page}}", Search: booksource.BookRule{BookList: "$.data[*]", Name: "name", Author: "author", BookURL: "url"}}
}

func streamResponse(req *http.Request, names ...string) *http.Response {
	books := make([]map[string]string, 0, len(names))
	for _, name := range names {
		books = append(books, map[string]string{"name": name, "author": "作者", "url": "/book/" + name})
	}
	data, _ := json.Marshal(map[string]any{"data": books})
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(data))), Request: req}
}

func initStreamingState(t *testing.T) (chan func(), *booksource.Client) {
	t.Helper()
	t.Setenv("READCLI_DATA_DIR", t.TempDir())
	initTestUIState()
	updates := make(chan func(), 64)
	app.online.enqueue = func(fn func()) { updates <- fn }
	client := booksource.NewClient()
	app.online.client = client
	app.online.keyword = "搜索"
	t.Cleanup(cancelOnlineRequest)
	return updates, client
}

func nextStreamingUpdate(t *testing.T, updates <-chan func()) func() {
	t.Helper()
	select {
	case update := <-updates:
		return update
	case <-time.After(2 * time.Second):
		t.Fatal("search did not enqueue an update")
		return nil
	}
}

func waitStreamingSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal("search worker did not reach fixture barrier")
	}
}

func TestStreamingSearchFastSourceVisibleAndSelectionStable(t *testing.T) {
	updates, client := initStreamingState(t)
	fast, slow := streamSource("fast"), streamSource("slow")
	app.online.sources = []booksource.Source{slow, fast}
	release, started := make(chan struct{}), make(chan struct{})
	defer close(release)
	client.HTTPClient.Transport = streamRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() == "slow.example.test" {
			close(started)
			select {
			case <-release:
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
			return streamResponse(req, "第三本"), nil
		}
		return streamResponse(req, "第一本", "第二本"), nil
	})
	searchOnlinePage(1)
	waitStreamingSignal(t, started)
	nextStreamingUpdate(t, updates)()
	if !app.online.searching || app.online.busy == "" || app.online.searchDone != 1 || app.online.searchTotal != 2 || len(app.online.results) != 2 {
		t.Fatalf("fast source not visible while slow waits: %+v", app.online)
	}
	panel := buildOnlinePanel()
	if !strings.Contains(panel, "第一本") || !strings.Contains(panel, "已完成 1/2") || !strings.Contains(panel, "搜索中") {
		t.Fatalf("busy panel hid partial results: %s", panel)
	}
	dispatchEvent("<Down>")
	if app.online.resultIndex != 1 {
		t.Fatal("busy search blocked result navigation")
	}
	// Permit the slow request to finish without closing the cleanup channel.
	release <- struct{}{}
	nextStreamingUpdate(t, updates)()
	if app.online.searching || app.online.busy != "" || app.online.searchDone != 2 || len(app.online.results) != 3 || app.online.resultIndex != 1 || app.online.results[1].Name != "第二本" {
		t.Fatalf("append changed selection or completion: %+v", app.online)
	}
}

func TestStreamingSearchErrorsArriveImmediatelyAndErrorViewKeepsSearch(t *testing.T) {
	updates, client := initStreamingState(t)
	app.online.sources = []booksource.Source{streamSource("failure"), streamSource("slow")}
	release := make(chan struct{})
	defer close(release)
	client.HTTPClient.Transport = streamRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() == "failure.example.test" {
			return nil, fmt.Errorf("fixture failure")
		}
		select {
		case <-release:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		return streamResponse(req, "成功书"), nil
	})
	searchOnlinePage(1)
	nextStreamingUpdate(t, updates)()
	if len(app.online.errors) != 1 || app.online.searchDone != 1 || !app.online.searching {
		t.Fatal("source error did not update during search")
	}
	id := app.online.requestID
	dispatchEvent("e")
	if app.mode != modeOnlineErrors || !app.online.searching || app.online.requestID != id {
		t.Fatal("error details canceled active search")
	}
	if !strings.Contains(buildOnlinePanel(), "搜索继续") {
		t.Fatal("error details did not show live progress")
	}
	dispatchEvent("q")
	if app.mode != modeOnlineResults || !app.online.searching || app.online.requestID != id {
		t.Fatal("returning from errors canceled search")
	}
	release <- struct{}{}
	nextStreamingUpdate(t, updates)()
	if app.online.searching || len(app.online.results) != 1 || len(app.online.errors) != 1 {
		t.Fatal("success after partial error was lost")
	}
}

func TestStreamingSearchCancelRetainsResultsAndRejectsQueuedLateUpdate(t *testing.T) {
	updates, client := initStreamingState(t)
	app.online.sources = []booksource.Source{streamSource("fast"), streamSource("slow")}
	release := make(chan struct{})
	defer close(release)
	client.HTTPClient.Transport = streamRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() == "slow.example.test" {
			select {
			case <-release:
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
			return streamResponse(req, "迟到书"), nil
		}
		return streamResponse(req, "保留一", "保留二"), nil
	})
	searchOnlinePage(1)
	nextStreamingUpdate(t, updates)()
	dispatchEvent("j")
	release <- struct{}{}
	late := nextStreamingUpdate(t, updates)
	dispatchEvent("<Escape>")
	if app.mode != modeOnlineResults || app.online.searching || app.online.busy != "" || len(app.online.results) != 2 || app.online.resultIndex != 1 || !strings.Contains(app.uiState.statusMessage, "保留 2 本") {
		t.Fatalf("Esc discarded partial search: %+v", app.online)
	}
	late()
	if len(app.online.results) != 2 || app.online.searchDone != 1 || app.online.resultIndex != 1 {
		t.Fatal("canceled queued update polluted retained results")
	}
}

func TestStreamingSearchNewPageAndOwnerRejectOldMessages(t *testing.T) {
	updates, client := initStreamingState(t)
	app.online.sources = []booksource.Source{streamSource("source")}
	client.HTTPClient.Transport = streamRoundTripper(func(req *http.Request) (*http.Response, error) {
		return streamResponse(req, req.URL.Query().Get("q")+"第"+req.URL.Query().Get("page")+"页"), nil
	})
	app.online.keyword = "旧搜索"
	searchOnlinePage(1)
	old := nextStreamingUpdate(t, updates)
	app.online.keyword = "新搜索"
	searchOnlinePage(2)
	current := nextStreamingUpdate(t, updates)
	old()
	if len(app.online.results) != 0 || !app.online.searching || app.online.page != 2 {
		t.Fatal("old page callback polluted new search")
	}
	current()
	if len(app.online.results) != 1 || app.online.results[0].Name != "新搜索第2页" || app.online.searchDone != 1 {
		t.Fatal("new search result missing")
	}
	searchOnlinePage(3)
	priorOwnerUpdate := nextStreamingUpdate(t, updates)
	initTestUIState()
	priorOwnerUpdate()
	if len(app.online.results) != 0 || app.mode != modeHome {
		t.Fatal("old owner callback changed replacement app")
	}
}

func TestStreamingSearchNavigationInvalidatesPendingMessages(t *testing.T) {
	for _, key := range []string{"q", "/", "S", "<C-c>"} {
		t.Run(key, func(t *testing.T) {
			updates, client := initStreamingState(t)
			app.online.sources = []booksource.Source{streamSource("source")}
			client.HTTPClient.Transport = streamRoundTripper(func(req *http.Request) (*http.Response, error) { return streamResponse(req, "迟到"), nil })
			searchOnlinePage(1)
			late := nextStreamingUpdate(t, updates)
			id := app.online.requestID
			dispatchEvent(key)
			mode := app.mode
			late()
			if app.online.searching || app.online.requestID == id || len(app.online.results) != 0 || app.mode != mode {
				t.Fatalf("%s did not invalidate old search", key)
			}
		})
	}
}

func TestStreamingSearchOpenAvailableBookFencesLateResults(t *testing.T) {
	updates, client := initStreamingState(t)
	app.online.sources = []booksource.Source{streamSource("fast"), streamSource("slow")}
	release := make(chan struct{})
	defer close(release)
	client.HTTPClient.Transport = streamRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() == "slow.example.test" {
			select {
			case <-release:
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
			return streamResponse(req, "迟到书"), nil
		}
		return streamResponse(req, "可读书"), nil
	})
	searchOnlinePage(1)
	nextStreamingUpdate(t, updates)()
	book := app.online.results[0]
	chapters := []booksource.Chapter{{Name: "第一章", URL: "https://fast.example.test/chapter"}}
	dir := os.Getenv("READCLI_DATA_DIR")
	if err := booksource.SaveCachedChapters(dir, book, chapters); err != nil {
		t.Fatal(err)
	}
	if err := booksource.SaveCachedContent(dir, book, chapters[0], "缓存正文。\n第二段正文。"); err != nil {
		t.Fatal(err)
	}
	release <- struct{}{}
	late := nextStreamingUpdate(t, updates)
	dispatchEvent("<Enter>")
	if app.online.searching || app.online.busy != "正在加载目录和正文" {
		t.Fatal("Enter was blocked by in-flight search")
	}
	late()
	nextStreamingUpdate(t, updates)()
	r, ok := app.reader.(*reader.OnlineReader)
	if !ok || r.Book.Name != "可读书" || app.mode != modeReading || len(app.online.results) != 1 {
		t.Fatal("late search overwrote opened reader")
	}
}

func TestStreamingSearchWorkerLimitAndPerSourceDeadlines(t *testing.T) {
	var active, maximum, completed atomic.Int32
	started, release, done := make(chan struct{}, 4), make(chan struct{}), make(chan struct{})
	var sources []booksource.Source
	for i := 0; i < 12; i++ {
		sources = append(sources, streamSource(fmt.Sprintf("source-%d", i)))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		defer close(done)
		streamOnlineSearch(ctx, sources, func(requestCtx context.Context, source booksource.Source) ([]booksource.Book, error) {
			count := active.Add(1)
			defer active.Add(-1)
			for previous := maximum.Load(); count > previous && !maximum.CompareAndSwap(previous, count); previous = maximum.Load() {
			}
			if deadline, ok := requestCtx.Deadline(); !ok || time.Until(deadline) > 20*time.Second || time.Until(deadline) < 18*time.Second {
				t.Errorf("unexpected per-source deadline %v exists=%v", deadline, ok)
			}
			select {
			case started <- struct{}{}:
			default:
			}
			select {
			case <-release:
			case <-requestCtx.Done():
				return nil, requestCtx.Err()
			}
			return []booksource.Book{{Name: source.Name}}, nil
		}, func(response onlineSearchResponse) {
			if response.err != nil {
				t.Errorf("search failed: %v", response.err)
			}
			completed.Add(1)
		})
	}()
	for i := 0; i < 4; i++ {
		waitStreamingSignal(t, started)
	}
	close(release)
	waitStreamingSignal(t, done)
	if maximum.Load() != 4 || completed.Load() != 12 {
		t.Fatalf("max concurrency=%d completed=%d", maximum.Load(), completed.Load())
	}
}

func TestStreamingSearchDeduplicatesAcrossCompletedSources(t *testing.T) {
	updates, client := initStreamingState(t)
	source := streamSource("duplicate")
	app.online.sources = []booksource.Source{source, source}
	client.HTTPClient.Transport = streamRoundTripper(func(req *http.Request) (*http.Response, error) { return streamResponse(req, "同一本"), nil })
	searchOnlinePage(1)
	nextStreamingUpdate(t, updates)()
	nextStreamingUpdate(t, updates)()
	if len(app.online.results) != 1 || app.online.searchDone != 2 || app.online.searching {
		t.Fatal("duplicate source response added duplicate books")
	}
}

func TestStreamingSearchCancellationStopsActiveAndQueuedSources(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, stopped := make(chan struct{}, 4), make(chan struct{}, 4)
	done := make(chan struct{})
	var calls, published atomic.Int32
	sources := make([]booksource.Source, 100)
	go func() {
		defer close(done)
		streamOnlineSearch(ctx, sources, func(requestCtx context.Context, _ booksource.Source) ([]booksource.Book, error) {
			calls.Add(1)
			started <- struct{}{}
			<-requestCtx.Done()
			stopped <- struct{}{}
			return nil, requestCtx.Err()
		}, func(onlineSearchResponse) { published.Add(1) })
	}()
	for i := 0; i < 4; i++ {
		waitStreamingSignal(t, started)
	}
	cancel()
	waitStreamingSignal(t, done)
	for i := 0; i < 4; i++ {
		waitStreamingSignal(t, stopped)
	}
	if calls.Load() != 4 || published.Load() != 0 {
		t.Fatalf("cancellation launched queued sources or published errors: calls=%d published=%d", calls.Load(), published.Load())
	}
}
