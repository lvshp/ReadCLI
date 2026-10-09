package booksource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type healthRoundTripper func(*http.Request) (*http.Response, error)

func (f healthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func healthSource() Source {
	return Source{
		URL: "https://health.example.test", Name: "健康检查样本", SearchURL: "/search?q={{key}}",
		Search:   BookRule{BookList: "li.book", Name: "a@text", BookURL: "a@href"},
		BookInfo: BookRule{TOCURL: "a.toc@href"},
		TOC:      TOCRule{ChapterList: "li.chapter", ChapterName: "a@text", ChapterURL: "a@href"},
		Content:  ContentRule{Content: "#content@text"},
	}
}

func healthClient(handler func(*http.Request) (int, string, error)) *Client {
	client := NewClient()
	client.HTTPClient.Transport = healthRoundTripper(func(req *http.Request) (*http.Response, error) {
		status, body, err := handler(req)
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: status, Status: fmt.Sprintf("%d %s", status, http.StatusText(status)),
			Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req,
		}, nil
	})
	return client
}

func TestCheckSourceStaticNeverExecutesScriptOrNetwork(t *testing.T) {
	source := healthSource()
	source.JSLib = `while (true) {} throw new Error("must never run");`
	source.SearchURL = `@js: java.ajax("https://health.example.test/unexpected"); "/search"`
	source.Search.Init = `@js: throw new Error("must never run")`
	source.Header = json.RawMessage(`"@js: java.ajax('https://health.example.test/unexpected')"`)
	var requests atomic.Int32
	client := healthClient(func(*http.Request) (int, string, error) {
		requests.Add(1)
		return 0, "", errors.New("network must never run")
	})
	started := time.Now()
	result := client.CheckSource(context.Background(), source, " \t", true)
	if result.Status != "ok" || result.Stage != "rules" || requests.Load() != 0 {
		t.Fatalf("static check: %+v; requests=%d", result, requests.Load())
	}
	if result.CheckedAt.Before(started) || result.CheckedAt.After(time.Now()) || result.DurationMS < 0 {
		t.Fatalf("check metadata: %+v", result)
	}
	if !strings.Contains(result.Detail, "未执行") {
		t.Fatalf("static scope undisclosed: %s", result.Detail)
	}
	data, err := json.Marshal(result)
	if err != nil || !strings.Contains(string(data), `"checked_at":`) || !strings.Contains(string(data), `"duration_ms":`) {
		t.Fatalf("result JSON: %s %v", data, err)
	}
}

func TestCheckSourceStaticRuleSyntax(t *testing.T) {
	tests := []struct {
		name   string
		edit   func(*Source)
		status string
		detail string
	}{
		{"missing identity", func(s *Source) { s.Name = "" }, "error", "bookSourceName"},
		{"unsupported type", func(s *Source) { s.Type = 1 }, "unsupported", "bookSourceType"},
		{"missing search URL", func(s *Source) { s.SearchURL = "" }, "error", "searchUrl"},
		{"missing list", func(s *Source) { s.Search.BookList = "" }, "error", "ruleSearch.bookList"},
		{"missing name", func(s *Source) { s.Search.Name = "" }, "error", "ruleSearch.name"},
		{"missing book URL", func(s *Source) { s.Search.BookURL = "" }, "error", "ruleSearch.bookUrl"},
		{"missing TOC", func(s *Source) { s.TOC.ChapterList = "" }, "error", "ruleToc.chapterList"},
		{"missing content", func(s *Source) { s.Content.Content = "" }, "error", "ruleContent.content"},
		{"no matching nodes", func(s *Source) { s.Search.BookList = "tag.unmatchable@tag.absent" }, "ok", "静态"},
		{"bad later chain", func(s *Source) { s.Search.BookList = "tag.unmatchable@#" }, "error", "ruleSearch.bookList"},
		{"bad explicit CSS", func(s *Source) { s.Search.Name = "@css:#@text" }, "error", "ruleSearch.name"},
		{"bad JSON", func(s *Source) { s.Search.BookList = "@json:$.items[" }, "error", "ruleSearch.bookList"},
		{"empty JSON path", func(s *Source) { s.Search.BookList = "@json:" }, "error", "JSONPath不可为空"},
		{"valid JSON", func(s *Source) { s.Search.BookList = "@json:$.missing[*]" }, "ok", "静态"},
		{"valid relative JSON", func(s *Source) { s.Search.Name = "items[0].name" }, "ok", "有限"},
		{"bad XPath", func(s *Source) { s.TOC.ChapterList = "@xpath://*[" }, "error", "ruleToc.chapterList"},
		{"valid XPath", func(s *Source) { s.TOC.ChapterList = "@xpath://li[@class='missing']" }, "ok", "静态"},
		{"bad JS", func(s *Source) { s.Search.Name = "@js: function {" }, "error", "ruleSearch.name"},
		{"bad library", func(s *Source) { s.JSLib = "function {" }, "error", "jsLib"},
		{"bad header JSON", func(s *Source) { s.Header = json.RawMessage(`{"broken":`) }, "error", "header"},
		{"bad header script", func(s *Source) { s.Header = json.RawMessage(`"@js: function {"`) }, "error", "header"},
		{"bad request header", func(s *Source) { s.SearchURL = `/search,{"headers":"@js: function {"}` }, "error", "searchUrl headers"},
		{"return JS", func(s *Source) { s.Search.Name = "@js: return result;" }, "ok", "静态"},
		{"missing script end", func(s *Source) { s.Search.Name = "<js>result" }, "error", "</js>"},
		{"bad script tail", func(s *Source) { s.Search.Name = "<js>result</js>@css:#@text" }, "error", "ruleSearch.name"},
		{"bad replacement", func(s *Source) { s.Content.ReplaceRegex = "[##" }, "error", "replaceRegex"},
		{"alternative syntax", func(s *Source) { s.Search.Name = "a@text||@css:#@text" }, "error", "ruleSearch.name"},
		{"bad request script", func(s *Source) { s.SearchURL = `@js: let =` }, "error", "Unexpected"},
		{"bad request option", func(s *Source) { s.SearchURL = `/search,{"method":` }, "error", "选项 JSON"},
		{"bad request JS option", func(s *Source) { s.SearchURL = `/search,{"js":"function {"}` }, "error", "searchUrl js"},
		{"dynamic rule", func(s *Source) { s.Search.Name = `{{unknownInput()}}` }, "ok", "有限"},
		{"dynamic JS", func(s *Source) { s.Search.Name = `@js: {{java.get("code")}}` }, "ok", "有限"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := healthSource()
			test.edit(&source)
			result := NewClient().CheckSource(context.Background(), source, "", false)
			if result.Status != test.status || result.Stage != "rules" || !strings.Contains(result.Detail, test.detail) {
				t.Fatalf("got %+v; want %s containing %q", result, test.status, test.detail)
			}
		})
	}
}

func TestCheckSourceSearchAndDeep(t *testing.T) {
	for _, deep := range []bool{false, true} {
		t.Run(fmt.Sprint(deep), func(t *testing.T) {
			var paths []string
			client := healthClient(func(req *http.Request) (int, string, error) {
				paths = append(paths, req.URL.Path)
				deadline, ok := req.Context().Deadline()
				wantTimeout := 20 * time.Second
				if deep {
					wantTimeout = 45 * time.Second
				}
				// http.Client may further cap its request timeout to 30 seconds.
				if !ok || time.Until(deadline) > wantTimeout || time.Until(deadline) < min(wantTimeout, 30*time.Second)-time.Second {
					t.Errorf("request deadline %v, want bounded by %v", deadline, wantTimeout)
				}
				switch req.URL.Path {
				case "/search":
					if req.URL.Query().Get("q") != "样本" {
						t.Errorf("keyword: %s", req.URL.RawQuery)
					}
					return 200, `<li class="book"><a href="/book">样本</a></li><li class="book"><a href="/never">次本</a></li>`, nil
				case "/book":
					return 200, `<a class="toc" href="/toc">目录</a>`, nil
				case "/toc":
					return 200, `<li class="chapter"><a href="/chapter">首章</a></li><li class="chapter"><a href="/never">末章</a></li>`, nil
				case "/chapter":
					return 200, `<div id="content">正文测试。</div>`, nil
				default:
					return 0, "", fmt.Errorf("unexpected request: %s", req.URL.Path)
				}
			})
			source := healthSource()
			disabled := false
			source.Enabled = &disabled
			before, _ := json.Marshal(source)
			result := client.CheckSource(context.Background(), source, " 样本 ", deep)
			wantStage, wantPaths := "search", []string{"/search"}
			if deep {
				wantStage, wantPaths = "content", []string{"/search", "/book", "/toc", "/chapter"}
			}
			if result.Status != "ok" || result.Stage != wantStage || !reflect.DeepEqual(paths, wantPaths) {
				t.Fatalf("check: %+v; paths=%v", result, paths)
			}
			after, _ := json.Marshal(source)
			if string(after) != string(before) || source.Enabled != &disabled || disabled {
				t.Fatalf("manual check changed disabled source: %s -> %s", before, after)
			}
		})
	}
}

func TestCheckSourceEmptyIsNotFailure(t *testing.T) {
	client := healthClient(func(*http.Request) (int, string, error) { return 200, `<ul></ul>`, nil })
	result := client.CheckSource(context.Background(), healthSource(), "nothing", true)
	if result.Status != "empty" || result.Stage != "search" || !strings.Contains(result.Detail, "不代表书源失效") {
		t.Fatalf("empty search: %+v", result)
	}
}

func TestCheckSourceAllowsMissingChapterName(t *testing.T) {
	for _, deep := range []bool{false, true} {
		t.Run(fmt.Sprint(deep), func(t *testing.T) {
			source := healthSource()
			source.TOC.ChapterName = ""
			var paths []string
			client := healthClient(func(req *http.Request) (int, string, error) {
				paths = append(paths, req.URL.Path)
				switch req.URL.Path {
				case "/search":
					return 200, `<li class="book"><a href="/book">样本</a></li>`, nil
				case "/book":
					return 200, `<a class="toc" href="/toc">目录</a>`, nil
				case "/toc":
					return 200, `<li class="chapter"><a href="/chapter"></a></li>`, nil
				case "/chapter":
					return 200, `<div id="content">没有章节名也可读取正文。</div>`, nil
				default:
					return 0, "", fmt.Errorf("unexpected request: %s", req.URL.Path)
				}
			})
			keyword, stage := "", "rules"
			var wantPaths []string
			if deep {
				keyword, stage = "样本", "content"
				wantPaths = []string{"/search", "/book", "/toc", "/chapter"}
			}
			result := client.CheckSource(context.Background(), source, keyword, deep)
			if result.Status != "ok" || result.Stage != stage || !reflect.DeepEqual(paths, wantPaths) {
				t.Fatalf("missing chapter name: %+v; paths=%v, want stage=%s paths=%v", result, paths, stage, wantPaths)
			}
		})
	}
}

func TestCheckSourceDeepFailureStages(t *testing.T) {
	for _, failure := range []string{"/search", "/book", "/toc", "/chapter"} {
		t.Run(failure, func(t *testing.T) {
			client := healthClient(func(req *http.Request) (int, string, error) {
				if req.URL.Path == failure {
					return 502, "unavailable", nil
				}
				switch req.URL.Path {
				case "/search":
					return 200, `<li class="book"><a href="/book">样本</a></li>`, nil
				case "/book":
					return 200, `<a class="toc" href="/toc">目录</a>`, nil
				case "/toc":
					return 200, `<li class="chapter"><a href="/chapter">首章</a></li>`, nil
				}
				return 200, "", nil
			})
			result := client.CheckSource(context.Background(), healthSource(), "test", true)
			stage := map[string]string{"/search": "search", "/book": "toc", "/toc": "toc", "/chapter": "content"}[failure]
			if result.Status != "error" || result.Stage != stage || !strings.Contains(result.Detail, "502") {
				t.Fatalf("failure stage: %+v", result)
			}
		})
	}
}

func TestCheckSourceClassification(t *testing.T) {
	for _, test := range []struct {
		message string
		status  string
	}{
		{"HTTP 401 Unauthorized", "login"},
		{"HTTP 403 Forbidden", "error"},
		{"HTTP 403 登录失败", "error"},
		{"请求 https://health.example.test/login 失败", "error"},
		{"login.example.test: connection reset by peer", "error"},
		{"JavaScript执行失败: 请先登录", "login"},
		{"authentication required", "login"},
		{"URL 选项 webView 需要浏览器或 Legado 专用能力，暂不支持", "unsupported"},
		{"不支持的Java宿主方法: java.webView", "unsupported"},
		{"JavaScript ReferenceError: missingValue is not defined", "error"},
	} {
		t.Run(test.message, func(t *testing.T) {
			client := healthClient(func(*http.Request) (int, string, error) { return 0, "", errors.New(test.message) })
			result := client.CheckSource(context.Background(), healthSource(), "test", false)
			if result.Status != test.status || result.Stage != "search" {
				t.Fatalf("classification: %+v, want %s", result, test.status)
			}
		})
	}
	for _, test := range []struct {
		err    error
		status string
	}{
		{fmt.Errorf("login required: %w", context.Canceled), "cancelled"},
		{fmt.Errorf("不支持: %w", context.DeadlineExceeded), "timeout"},
		{fmt.Errorf("请先登录: %w", ErrJSTimeout), "timeout"},
	} {
		if got := sourceCheckStatus(test.err); got != test.status {
			t.Errorf("%v = %s, want %s", test.err, got, test.status)
		}
	}
}

func TestCheckSourceCancellationAndTimeout(t *testing.T) {
	for _, keyword := range []string{"", "test"} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result := NewClient().CheckSource(ctx, healthSource(), keyword, true)
		if result.Status != "cancelled" || result.Stage != "rules" {
			t.Fatalf("pre-cancelled check: %+v", result)
		}
	}
	t.Run("request deadline", func(t *testing.T) {
		client := healthClient(func(req *http.Request) (int, string, error) {
			<-req.Context().Done()
			return 0, "", req.Context().Err()
		})
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
		defer cancel()
		result := client.CheckSource(ctx, healthSource(), "test", false)
		if result.Status != "timeout" || result.Stage != "search" {
			t.Fatalf("request timeout: %+v", result)
		}
	})
	t.Run("running script cancellation", func(t *testing.T) {
		source := healthSource()
		source.SearchURL = "@js: while (true) {}"
		ctx, cancel := context.WithCancel(context.Background())
		timer := time.AfterFunc(15*time.Millisecond, cancel)
		defer timer.Stop()
		defer cancel()
		result := NewClient().CheckSource(ctx, source, "test", false)
		if result.Status != "cancelled" || result.Stage != "search" {
			t.Fatalf("script cancellation: %+v", result)
		}
	})
	t.Run("script timeout", func(t *testing.T) {
		source := healthSource()
		source.SearchURL = "@js: while (true) {}"
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
		defer cancel()
		result := NewClient().CheckSource(ctx, source, "test", false)
		if result.Status != "timeout" || result.Stage != "search" {
			t.Fatalf("script timeout: %+v", result)
		}
	})
}
