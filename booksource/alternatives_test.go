package booksource

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func alternativeSource(host string) Source {
	return Source{
		URL: "https://" + host + ".example.test", Name: host,
		SearchURL: "/search?q={{key}}&page={{page}}",
		Search:    BookRule{BookList: "$.data[*]", Name: "name", Author: "author", BookURL: "url", Kind: "kind", LastChapter: "last"},
	}
}

func TestFindAlternativesPreservesAggregateCandidatesAndSourceIdentity(t *testing.T) {
	aggregate, other := alternativeSource("aggregate"), alternativeSource("other")
	disabled := alternativeSource("disabled")
	off := false
	disabled.Enabled = &off
	noSearch, noList, unsupported := alternativeSource("no-search"), alternativeSource("no-list"), alternativeSource("unsupported")
	noSearch.SearchURL, noList.Search.BookList, unsupported.Type = "", "", 1
	current := Book{SourceURL: aggregate.URL, URL: aggregate.URL + "/current", Name: "《星 海》", Author: "作者：张三 著"}
	c := NewClient()
	c.HTTPClient.Transport = sessionRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("q") != current.Name || req.URL.Query().Get("page") != "3" {
			t.Errorf("unexpected search query: %s", req.URL.RawQuery)
		}
		switch req.URL.Hostname() {
		case "aggregate.example.test":
			return sessionResponse(req, 200, `{"data":[
				{"name":"星海","author":"张三","url":"/current"},
				{"name":"星海","author":"张三","url":"/a","kind":"甲","last":"渠道甲 第一章"},
				{"name":"《星 海》","author":"作者:张三著","url":"/b","kind":"乙","last":"渠道乙 第二章"},
				{"name":"星海","author":"张三","url":"/a","kind":"重复"},
				{"name":"星海续集","author":"张三","url":"/sequel"},
				{"name":"星海","author":"李四","url":"/wrong-author"},
				{"name":"星海","url":"/unknown-author"}
			]}`), nil
		case "other.example.test":
			return sessionResponse(req, 200, fmt.Sprintf(`{"data":[{"name":"星海","author":"张三","url":%q}]}`, aggregate.URL+"/a")), nil
		default:
			return nil, fmt.Errorf("unexpected source %s", req.URL.Hostname())
		}
	})
	result, err := c.FindAlternatives(context.Background(), []Source{aggregate, other, aggregate, disabled, noSearch, noList, unsupported}, current, 3)
	if err != nil || len(result.Errors) != 0 {
		t.Fatalf("FindAlternatives: %+v, %v", result, err)
	}
	want := [][2]string{{aggregate.URL, aggregate.URL + "/a"}, {aggregate.URL, aggregate.URL + "/b"}, {aggregate.URL, aggregate.URL + "/unknown-author"}, {other.URL, aggregate.URL + "/a"}}
	var got [][2]string
	for _, book := range result.Books {
		got = append(got, [2]string{book.SourceURL, book.URL})
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
	if result.Books[0].Kind != "甲" || result.Books[1].LastChapter != "渠道乙 第二章" {
		t.Fatalf("source labels were lost: %+v", result.Books)
	}
}

func TestFindAlternativesKeepsSuccessAndOrdersErrors(t *testing.T) {
	c := NewClient()
	sources := []Source{alternativeSource("slow-error"), alternativeSource("success"), alternativeSource("fast-error")}
	fastFinished := make(chan struct{})
	c.HTTPClient.Transport = sessionRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("page") != "1" {
			t.Errorf("page was not clamped: %s", req.URL.RawQuery)
		}
		switch req.URL.Hostname() {
		case "slow-error.example.test":
			select {
			case <-fastFinished:
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
			return sessionResponse(req, http.StatusBadGateway, "failure"), nil
		case "fast-error.example.test":
			close(fastFinished)
			return nil, errors.New("offline failure")
		default:
			return sessionResponse(req, 200, `{"data":[{"name":"星海","author":"任意作者","url":"/book"}]}`), nil
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := c.FindAlternatives(ctx, sources, Book{Name: "星海"}, 0)
	if err != nil || len(result.Books) != 1 || len(result.Errors) != 2 {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if !strings.Contains(result.Errors[0], "slow-error") || !strings.Contains(result.Errors[1], "fast-error") {
		t.Fatalf("errors lost source order: %v", result.Errors)
	}
}

func TestFindAlternativesLimitsConcurrencyAndSetsSourceDeadline(t *testing.T) {
	c := NewClient()
	var active, maximum atomic.Int32
	started, release := make(chan struct{}, 8), make(chan struct{})
	c.HTTPClient.Transport = sessionRoundTripper(func(req *http.Request) (*http.Response, error) {
		count := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); count > old && !maximum.CompareAndSwap(old, count); old = maximum.Load() {
		}
		deadline, ok := req.Context().Deadline()
		if !ok || time.Until(deadline) > 20*time.Second || time.Until(deadline) < 18*time.Second {
			t.Errorf("source deadline = %v, exists = %v", deadline, ok)
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		return sessionResponse(req, 200, `{"data":[{"name":"星海","url":"/book"}]}`), nil
	})
	var sources []Source
	for i := 0; i < 8; i++ {
		sources = append(sources, alternativeSource(fmt.Sprintf("source-%d", i)))
	}
	done := make(chan AlternativeResult, 1)
	go func() {
		result, err := c.FindAlternatives(context.Background(), sources, Book{Name: "星海"}, 1)
		if err != nil {
			t.Errorf("FindAlternatives: %v", err)
		}
		done <- result
	}()
	for i := 0; i < 4; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("four workers did not start")
		}
	}
	close(release)
	select {
	case result := <-done:
		if len(result.Books) != 8 || maximum.Load() != 4 {
			t.Fatalf("books = %d, maximum concurrency = %d", len(result.Books), maximum.Load())
		}
		for i, book := range result.Books {
			if book.SourceURL != sources[i].URL {
				t.Fatalf("candidate %d has unstable source order", i)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("search did not finish")
	}
}

func TestFindAlternativesCancellation(t *testing.T) {
	t.Run("already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := NewClient().FindAlternatives(ctx, nil, Book{Name: "星海"}, 1)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("in flight", func(t *testing.T) {
		c := NewClient()
		started := make(chan struct{})
		c.HTTPClient.Transport = sessionRoundTripper(func(req *http.Request) (*http.Response, error) {
			close(started)
			<-req.Context().Done()
			return nil, req.Context().Err()
		})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := c.FindAlternatives(ctx, []Source{alternativeSource("canceled")}, Book{Name: "星海"}, 1)
			done <- err
		}()
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("search did not start")
		}
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("canceled search did not finish")
		}
	})
}

func TestMatchAlternativeChapter(t *testing.T) {
	tests := []struct {
		name     string
		titles   []string
		previous string
		oldIndex int
		want     int
		matched  bool
	}{
		{"normalized title", []string{"前言", "第 十 章：星 海", "第十一章 出发"}, "第十章 星海", 0, 1, true},
		{"renumbered unique name", []string{"第十二章 归来", "第十三章 星海"}, "第十章 星海", 0, 1, true},
		{"Chinese and Arabic number", []string{"第9章 启程", "第10章 星空之下"}, "第十章", 0, 1, true},
		{"large Chinese number", []string{"第1234章 风起", "第1235章 云涌"}, "第一千二百三十四章", 1, 0, true},
		{"unique unnumbered name", []string{"前言", "星海"}, "第十章 星海", 0, 1, true},
		{"generic preface", []string{"前言", "第十章 旅途"}, "前言", 1, 1, false},
		{"duplicate title", []string{"无题", "无题", "第三章 旅途"}, "无题", 2, 2, false},
		{"duplicate chapter number", []string{"第一章 启程", "第一章 再起", "第二章"}, "第一章", 2, 2, false},
		{"volume mismatch", []string{"第一卷 第一章 启程", "第二卷 第一章 新程"}, "第三卷 第一章", 1, 1, false},
		{"no match", []string{"第一章 起点", "第二章 终点"}, "第五章 未知", 0, 0, false},
		{"clamp high", []string{"第一章 起点", "第二章 终点"}, "第五章 未知", 20, 1, false},
		{"clamp low", []string{"第一章 起点"}, "", -10, 0, false},
		{"empty toc", nil, "第一章", 0, -1, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chapters := make([]Chapter, len(test.titles))
			for i, title := range test.titles {
				chapters[i].Name = title
			}
			index, matched := MatchAlternativeChapter(chapters, test.previous, test.oldIndex)
			if index != test.want || matched != test.matched {
				t.Fatalf("MatchAlternativeChapter = (%d, %v), want (%d, %v)", index, matched, test.want, test.matched)
			}
		})
	}
}
