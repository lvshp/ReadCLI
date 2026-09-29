package booksource

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type contentRoundTripper func(*http.Request) (*http.Response, error)

func (f contentRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestContentNormalizesAfterRulesAndReplacement(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		rule    ContentRule
		want    string
		wantErr bool
	}{
		{name: "JSON HTML paragraphs", body: `{"text":"<p>第一段。</p><p>第二段。<br>第三行。</p>"}`, rule: ContentRule{Content: "$.text"}, want: "第一段。\n第二段。\n第三行。"},
		{name: "replacement sees original markup", body: `{"text":"<p>原正文</p>"}`, rule: ContentRule{Content: "$.text", ReplaceRegex: `@js:result.replace("<p>原正文</p>", "<p>甲&amp;乙</p><p>第二段</p>")`}, want: "甲&乙\n第二段"},
		{name: "HTML inner fragment", body: `<main><p>正文一</p><p>正文二</p></main>`, rule: ContentRule{Content: "tag.main@html"}, want: "正文一\n正文二"},
		{name: "JavaScript returns markup", body: `{}`, rule: ContentRule{Content: `@js:'<div>甲</div><div>乙</div>'`}, want: "甲\n乙"},
		{name: "escaped HTML", body: `{"text":"&lt;p&gt;甲&amp;amp;乙&lt;/p&gt;&lt;p&gt;丙&lt;/p&gt;"}`, rule: ContentRule{Content: "$.text"}, want: "甲&乙\n丙"},
		{name: "empty markup", body: `{"text":"<p>&nbsp;</p><script>hidden()</script><style>.hidden{}</style>"}`, rule: ContentRule{Content: "$.text"}, wantErr: true},
		{name: "replacement clears body", body: `{"text":"删除"}`, rule: ContentRule{Content: "$.text", ReplaceRegex: "##删除##"}, wantErr: true},
		{name: "plain text unchanged", body: contentJSON("第一段。\n第二段。\n\n　　下一页：a<b && c>d，<T>保持原样。"), rule: ContentRule{Content: "$.text"}, want: "第一段。\n第二段。\n\n　　下一页：a<b && c>d，<T>保持原样。"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient()
			client.HTTPClient.Transport = contentRoundTripper(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tt.body)), Request: req}, nil
			})
			source := Source{URL: "https://content.example", Content: tt.rule}
			got, err := client.Content(context.Background(), source, Book{URL: source.URL + "/book"}, Chapter{URL: source.URL + "/chapter"})
			if tt.wantErr {
				if err == nil || got != "" {
					t.Fatalf("expected empty-content error, got %q, %v", got, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestNormalizeContent(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"blocks and breaks", `<p>第一段</p><p>第二段<br/>第三行</p>`, "第一段\n第二段\n第三行"},
		{"nested inline markup", `<div>甲<b>乙</b><span class="x">丙</span></div><div>丁</div>`, "甲乙丙\n丁"},
		{"attributes containing brackets", `<p title="a > b">甲<a href="/?x=1&amp;y=2">乙</a></p>`, "甲乙"},
		{"scripts styles comments", `<p>甲<script>var x = '<p>hidden</p>';</script><style>.x {color:red}</style>乙</p><!--hidden-->`, "甲乙"},
		{"unclosed script", `正文<script>secret()`, "正文"},
		{"nested markup in text containers", `<textarea><p>甲</p><p>乙</p></textarea>`, "甲\n乙"},
		{"HTML entities", `甲&nbsp;&amp;&#x4e59;&#20057;&quot;丙&quot;`, "甲\u00a0&乙乙\"丙\""},
		{"escaped fragment", `&lt;p&gt;甲&amp;amp;乙&lt;/p&gt;&lt;p&gt;丙&lt;br /&gt;丁&lt;/p&gt;`, "甲&乙\n丙\n丁"},
		{"double escaped fragment", `&amp;lt;p&amp;gt;甲&amp;lt;/p&amp;gt;`, "甲"},
		{"plain paragraphs and indentation", "第一段\n\n　　第二段\n    第三行", "第一段\n\n　　第二段\n    第三行"},
		{"literal angle brackets", `2 < 3，5 > 4；<等级>，<T>，<not-a-tag>甲</not-a-tag>`, `2 < 3，5 > 4；<等级>，<T>，<not-a-tag>甲</not-a-tag>`},
		{"compact comparisons", `a<b && c>d; x<=y && z>=w`, `a<b && c>d; x<=y && z>=w`},
		{"escaped comparisons", `a &lt; b &amp;&amp; c &gt; d; &lt;T&gt;`, `a < b && c > d; <T>`},
		{"unfinished angle text", `正文 <a`, `正文 <a`},
		{"unknown declaration", `甲<!NOTE>乙`, `甲<!NOTE>乙`},
		{"repeated breaks", `甲<br><br>乙`, "甲\n\n乙"},
		{"CRLF", "甲\r\n乙\r丙", "甲\n乙\n丙"},
		{"empty markup", `<p>&nbsp;</p><script>hidden</script><style>hidden</style>`, ""},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeContent(tt.input)
			if got != tt.want {
				t.Fatalf("got %q; want %q", got, tt.want)
			}
			if second := NormalizeContent(got); second != got {
				t.Fatalf("normalizing cached text changed it again: %q -> %q", got, second)
			}
		})
	}
}

func contentJSON(value string) string {
	data, _ := json.Marshal(map[string]string{"text": value})
	return string(data)
}
