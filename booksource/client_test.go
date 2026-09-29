package booksource

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestHTMLSourceEndToEnd(t *testing.T) {
	var tocRequests, contentRequests atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "中文 &小说" || r.URL.Query().Get("page") != "2" {
			t.Errorf("query: %s", r.URL.RawQuery)
		}
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "ok"})
		fmt.Fprint(w, `<ul><li class="book"><a href="/book/1">测试小说</a><b>作者</b></li></ul>`)
	})
	mux.HandleFunc("/book/1", func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie("session"); err != nil || cookie.Value != "ok" {
			t.Errorf("cookie missing: %v", err)
		}
		fmt.Fprint(w, `<h1>测试小说</h1><a class="toc" href="/toc/1">目录</a><div class="intro">介绍</div>`)
	})
	mux.HandleFunc("/toc/1", func(w http.ResponseWriter, r *http.Request) {
		tocRequests.Add(1)
		fmt.Fprint(w, `<ul><li><a href="/chapter/1">第一章</a></li></ul><a class="next" href="2">下一页</a>`)
	})
	mux.HandleFunc("/toc/2", func(w http.ResponseWriter, r *http.Request) {
		tocRequests.Add(1)
		fmt.Fprint(w, `<ul><li><a href="/chapter/1">重复章节</a></li><li><a href="/chapter/2">第二章</a></li></ul><a class="next" href="1">循环</a>`)
	})
	mux.HandleFunc("/chapter/1", func(w http.ResponseWriter, r *http.Request) {
		contentRequests.Add(1)
		fmt.Fprint(w, `<div id="content"><p>第一段。</p><p>第二段。<br>第三段。</p></div><a class="next" href="1b">下一页</a>`)
	})
	mux.HandleFunc("/chapter/1b", func(w http.ResponseWriter, r *http.Request) {
		contentRequests.Add(1)
		fmt.Fprint(w, `<div id="content">第四段。广告</div><a class="next" href="1">循环</a>`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	source := Source{URL: server.URL, Name: "HTML测试", SearchURL: "/search?q={{key}}&page={{page}}", Search: BookRule{BookList: "class.book", Name: "tag.a@text", Author: "tag.b@text", BookURL: "tag.a@href"}, BookInfo: BookRule{Name: "tag.h1@text", TOCURL: "class.toc@href", Intro: "class.intro@text"}, TOC: TOCRule{ChapterList: "tag.li", ChapterName: "tag.a@text", ChapterURL: "tag.a@href", NextTOCURL: "class.next@href"}, Content: ContentRule{Content: "id.content@text", NextContentURL: "class.next@href", ReplaceRegex: "##广告##"}}
	client := NewClient()
	books, err := client.Search(context.Background(), source, "中文 &小说", 2)
	if err != nil || len(books) != 1 {
		t.Fatalf("Search: %+v %v", books, err)
	}
	if books[0].Name != "测试小说" || books[0].URL != server.URL+"/book/1" {
		t.Fatalf("book: %+v", books[0])
	}
	book, err := client.BookInfo(context.Background(), source, books[0])
	if err != nil || book.TOCURL != server.URL+"/toc/1" || book.Intro != "介绍" {
		t.Fatalf("BookInfo: %+v %v", book, err)
	}
	chapters, err := client.Chapters(context.Background(), source, book)
	if err != nil || len(chapters) != 2 || tocRequests.Load() != 2 {
		t.Fatalf("Chapters: %+v requests=%d %v", chapters, tocRequests.Load(), err)
	}
	content, err := client.Content(context.Background(), source, book, chapters[0])
	if err != nil || content != "第一段。\n第二段。\n第三段。\n\n第四段。" || contentRequests.Load() != 2 {
		t.Fatalf("Content: %q requests=%d %v", content, contentRequests.Load(), err)
	}
}

func TestJSONSourcePOSTEndToEnd(t *testing.T) {
	keyword := "中文 \"&"
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("X-Test") != "ok" || r.Header.Get("X-Source") != "yes" {
			t.Errorf("request: %s %v", r.Method, r.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["q"] != keyword || body["page"] != "3" {
			t.Errorf("body: %v %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"name":"JSON小说","url":"/info"}]}`)
	})
	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"name":"JSON小说","toc":"/toc"}`) })
	mux.HandleFunc("/toc", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"chapters":[{"title":"第一章","url":"/content"}]}`)
	})
	mux.HandleFunc("/content", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"text":"段落一\n段落二"}`) })
	server := httptest.NewServer(mux)
	defer server.Close()
	source := Source{URL: server.URL, Name: "JSON测试", Header: json.RawMessage(`"{\"X-Source\":\"yes\"}"`), SearchURL: `/search,{"method":"POST","body":{"q":"{{key}}","page":"{{page}}"},"headers":"{\"X-Test\":\"ok\"}"}`, Search: BookRule{BookList: "@json:$.data[*]", Name: "name", BookURL: "url"}, BookInfo: BookRule{Name: "name", TOCURL: "toc"}, TOC: TOCRule{ChapterList: "$.chapters[*]", ChapterName: "title", ChapterURL: "url"}, Content: ContentRule{Content: "$.text"}}
	client := NewClient()
	books, err := client.Search(context.Background(), source, keyword, 3)
	if err != nil || len(books) != 1 {
		t.Fatalf("Search: %+v %v", books, err)
	}
	book, err := client.BookInfo(context.Background(), source, books[0])
	if err != nil {
		t.Fatal(err)
	}
	chapters, err := client.Chapters(context.Background(), source, book)
	if err != nil || len(chapters) != 1 {
		t.Fatalf("Chapters: %+v %v", chapters, err)
	}
	content, err := client.Content(context.Background(), source, book, chapters[0])
	if err != nil || content != "段落一\n段落二" {
		t.Fatalf("Content: %q %v", content, err)
	}
}

func TestRequestErrorsLimitsAndEncoding(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "failure", http.StatusBadGateway) })
	mux.HandleFunc("/large", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, strings.Repeat("x", 101)) })
	mux.HandleFunc("/gbk", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=gbk")
		data, _ := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("<div>中文内容</div>"))
		w.Write(data)
	})
	mux.HandleFunc("/form", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("q") != "中文 &" {
			t.Errorf("form=%v", r.Form)
		}
		fmt.Fprint(w, "ok")
	})
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	server := httptest.NewServer(mux)
	defer server.Close()
	client := NewClient()
	if _, _, err := client.fetch(context.Background(), Source{}, server.URL, "/status", nil, 0); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("status error: %v", err)
	}
	if _, _, err := client.fetch(context.Background(), Source{}, server.URL, "/large", nil, 100); err == nil {
		t.Fatal("missing size limit")
	}
	body, _, err := client.fetch(context.Background(), Source{}, server.URL, "/gbk", nil, 0)
	if err != nil || body != "<div>中文内容</div>" {
		t.Fatalf("gbk: %q %v", body, err)
	}
	_, _, err = client.fetch(context.Background(), Source{}, server.URL, `/form,{"method":"POST","body":"q={{key}}"}`, map[string]any{"key": "中文 &"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, _, err = client.fetch(ctx, Source{}, server.URL, "/slow", nil, 0)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestImportStoreAndCache(t *testing.T) {
	dataDir := t.TempDir()
	data := `[{"bookSourceUrl":"https://example.test","bookSourceName":"示例","searchUrl":"/s","ruleSearch":"{\"bookList\":\"tag.li\",\"name\":\"text\"}"}]`
	path := filepath.Join(dataDir, "input.json")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	sources, err := Import(context.Background(), path)
	if err != nil || len(sources) != 1 || sources[0].Search.BookList != "tag.li" {
		t.Fatalf("import: %+v %v", sources, err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, data) }))
	defer server.Close()
	remote, err := Import(context.Background(), server.URL)
	if err != nil || len(remote) != 1 {
		t.Fatalf("remote import: %+v %v", remote, err)
	}
	replacement := sources[0]
	replacement.Name = "更新"
	merged := MergeSources(sources, []Source{replacement})
	if len(merged) != 1 || merged[0].Name != "更新" {
		t.Fatalf("merged: %+v", merged)
	}
	if err := SaveSources(dataDir, merged); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSources(dataDir)
	if err != nil || len(loaded) != 1 || loaded[0].Name != "更新" {
		t.Fatalf("loaded: %+v %v", loaded, err)
	}
	book := Book{URL: "https://example.test/b", SourceURL: "https://example.test"}
	chapters := []Chapter{{Name: "一", URL: "https://example.test/c"}}
	if err := SaveCachedChapters(dataDir, book, chapters); err != nil {
		t.Fatal(err)
	}
	cached, err := LoadCachedChapters(dataDir, book)
	if err != nil || len(cached) != 1 {
		t.Fatalf("cached chapters: %+v %v", cached, err)
	}
	if err := SaveCachedContent(dataDir, book, chapters[0], "段落一\n段落二"); err != nil {
		t.Fatal(err)
	}
	content, err := LoadCachedContent(dataDir, book, chapters[0])
	if err != nil || content != "段落一\n段落二" {
		t.Fatalf("cached content: %q %v", content, err)
	}
	mixed, err := decodeSources([]byte(`[{"bookSourceUrl":"x","bookSourceName":"x","searchUrl":"/"},{"bookSourceUrl":"y","bookSourceName":"y","searchUrl":"/","bookSourceType":1}]`))
	if err != nil || len(mixed) != 2 || mixed[1].IsEnabled() || ValidateSource(mixed[1]) == nil {
		t.Fatalf("mixed import: %+v %v", mixed, err)
	}
	if _, err := decodeSources([]byte(`[{"bookSourceUrl":"x","bookSourceName":"x","searchUrl":"/"},null]`)); err == nil || !strings.Contains(err.Error(), "第 2") {
		t.Fatalf("invalid import: %v", err)
	}
	if _, err := decodeSources([]byte(`<html>source list</html>`)); err == nil {
		t.Fatal("HTML must fail")
	}
}

func TestPaginationBound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("p")
		if page == "" {
			page = "1"
		}
		fmt.Fprintf(w, `<li><a href="/chapter/%s">章</a></li><a class="next" href="?p=2">next</a>`, page)
	}))
	defer server.Close()
	source := Source{URL: server.URL, TOC: TOCRule{ChapterList: "tag.li", ChapterName: "text", ChapterURL: "tag.a@href", NextTOCURL: "class.next@href"}}
	client := NewClient()
	client.MaxPages = 1
	if _, err := client.Chapters(context.Background(), source, Book{URL: server.URL}); err == nil || !strings.Contains(err.Error(), "分页超过") {
		t.Fatalf("pagination: %v", err)
	}
}

func TestDataURIAndUnknownFieldRoundTrip(t *testing.T) {
	payload := `{"id":"123","name":"中文"}`
	uri := "data:;base64," + base64.StdEncoding.EncodeToString([]byte(payload)) + `,{"type":"qingtian"}`
	c := NewClient()
	body, base, err := c.fetch(context.Background(), Source{URL: "大灰狼融合VIP"}, "大灰狼融合VIP", uri, nil, 0)
	if err != nil || body != hex.EncodeToString([]byte(payload)) || base != uri {
		t.Fatalf("data URI: %q %q %v", body, base, err)
	}
	resolved, err := resolveURL("大灰狼融合VIP", uri)
	if err != nil || resolved != uri {
		t.Fatalf("resolve data URI: %q %v", resolved, err)
	}
	var source Source
	err = json.Unmarshal([]byte(`{"bookSourceUrl":"大灰狼融合VIP","bookSourceName":"示例","searchUrl":"/s","loginUrl":"<js>login()</js>","loginUi":"[{\"name\":\"邮箱\"}]","customField":{"a":1},"ruleToc":{"customToc":true,"chapterList":"tag.li"}}`), &source)
	if err != nil {
		t.Fatal(err)
	}
	source.Name = "更新"
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["bookSourceName"] != "更新" || saved["customField"].(map[string]any)["a"] != float64(1) || saved["ruleToc"].(map[string]any)["customToc"] != true {
		t.Fatalf("roundtrip: %s", data)
	}
}

func TestContentStopsBeforeNextChapterAndReplacesAfterMerge(t *testing.T) {
	var nextChapterRequests atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/part1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<div>第一页</div><a href="/part2">next</a>`)
	})
	mux.HandleFunc("/part2", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<div>第二页</div><a href="/next-chapter">next</a>`)
	})
	mux.HandleFunc("/next-chapter", func(w http.ResponseWriter, r *http.Request) {
		nextChapterRequests.Add(1)
		fmt.Fprint(w, `<div>下一章</div>`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	source := Source{URL: server.URL, Content: ContentRule{Content: "tag.div@text", NextContentURL: "tag.a@href", ReplaceRegex: `@js:result.replace("第一页\n\n第二页", "合并成功")`}}
	content, err := NewClient().Content(context.Background(), source, Book{URL: server.URL}, Chapter{URL: server.URL + "/part1", NextURL: server.URL + "/next-chapter"})
	if err != nil || content != "合并成功" || nextChapterRequests.Load() != 0 {
		t.Fatalf("content %q nextRequests=%d err=%v", content, nextChapterRequests.Load(), err)
	}
}

func TestJavaScriptURLQueryEncoding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, ch := range []byte(r.RequestURI) {
			if ch >= 0x80 || ch <= 0x20 {
				t.Errorf("unescaped request URI %q", r.RequestURI)
				break
			}
		}
		if r.URL.Query().Get("title") != "中文 小说" || r.URL.Query().Get("encoded") != "中文" || r.URL.Query().Get("filter") != "a/b+c" || r.URL.Query().Get("page") != "2" {
			t.Errorf("query changed: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()
	source := Source{URL: server.URL}
	rule := `@js:baseUrl + '/search?title=' + key + '&encoded=%E4%B8%AD%E6%96%87&filter=a%2Fb%2Bc&page=' + page`
	body, _, err := NewClient().fetch(context.Background(), source, server.URL, rule, map[string]any{"key": "中文 小说", "page": 2}, 0)
	if err != nil || body != "ok" {
		t.Fatalf("request: %q %v", body, err)
	}
}
