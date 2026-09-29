package booksource

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

const ruleHTML = `<html><body><div id="catalog"><div class="book"><a href="/书/一">第一本<span>书</span></a><p>作者：张三</p></div><div class="book"><a href="/书/二">第二本书</a><p>作者：李四</p></div><div class="book"><a href="/书/三">第三本书</a><p>作者：王五</p></div></div><div id="content"><p>第一段 <em>中文</em>。</p><p>第二段<br>第三行</p><script>不应显示</script></div></body></html>`

func TestRuleHTMLSelectors(t *testing.T) {
	e := NewEvaluator("https://example.test", nil)
	cases := []struct {
		rule string
		want []string
	}{
		{"@css:.book a@text", []string{"第一本书", "第二本书", "第三本书"}},
		{"class.book.0@tag.a@href", []string{"/书/一"}},
		{"class.book.-1@tag.a@text", []string{"第三本书"}},
		{"class.book.0:2@tag.a@text", []string{"第一本书", "第三本书"}},
		{"class.book[0:1]@tag.a@text", []string{"第一本书", "第二本书"}},
		{"class.book[-1:0]@tag.a@text", []string{"第三本书", "第二本书", "第一本书"}},
		{"class.book[!1]@tag.a@text", []string{"第一本书", "第三本书"}},
		{"class.book!0:2@tag.a@text", []string{"第二本书"}},
		{"id.content@text", []string{"第一段 中文。\n第二段\n第三行"}},
		{"class.missing@text||class.book.1@tag.a@text", []string{"第二本书"}},
		{"class.book.0@tag.a@text&&class.book.2@tag.a@text", []string{"第一本书", "第三本书"}},
		{"class.book.0@tag.a@text##第(.*)书##这是$1小说", []string{"这是一本小说"}},
	}
	for _, test := range cases {
		t.Run(test.rule, func(t *testing.T) {
			got, err := e.Strings(ruleHTML, test.rule)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got %#v, want %#v", got, test.want)
			}
		})
	}
	items, err := e.Elements(ruleHTML, "class.book")
	if err != nil || len(items) != 3 {
		t.Fatalf("elements: %d, %v", len(items), err)
	}
	got, err := e.String(items[1], "tag.a@href")
	if err != nil || got != "/书/二" {
		t.Fatalf("chained input: %q %v", got, err)
	}
}

func TestRuleJSON(t *testing.T) {
	e := NewEvaluator("", nil)
	input := `{"data":[{"name":"中文一","score":1},{"name":"中文二","score":2}],"empty":"","next":"下一页"}`
	cases := []struct {
		rule string
		want []string
	}{
		{"@json:$.data[*].name", []string{"中文一", "中文二"}},
		{"$.data[?(@.score > 1 && @.name == '中文二')].name", []string{"中文二"}},
		{"$.missing||$.next", []string{"下一页"}},
		{"$.empty||$.next", []string{"下一页"}},
		{"$.next&&$.data[0].name", []string{"下一页", "中文一"}},
		{"next", []string{"下一页"}},
	}
	for _, test := range cases {
		t.Run(test.rule, func(t *testing.T) {
			got, err := e.Strings(input, test.rule)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got %#v, want %#v", got, test.want)
			}
		})
	}
	items, err := e.Elements(input, "@json:$.data")
	if err != nil || len(items) != 2 {
		t.Fatalf("JSON elements: %#v, %v", items, err)
	}
	got, err := e.String(items[1], "name")
	if err != nil || got != "中文二" {
		t.Fatalf("nested property: %q %v", got, err)
	}
}

func TestRuleXPath(t *testing.T) {
	e := NewEvaluator("", nil)
	got, err := e.Strings(ruleHTML, "@xpath://div[@class='book']/a/@href")
	if err != nil || !reflect.DeepEqual(got, []string{"/书/一", "/书/二", "/书/三"}) {
		t.Fatalf("XPath attrs: %#v %v", got, err)
	}
	items, err := e.Elements(ruleHTML, "//div[@class='book']")
	if err != nil || len(items) != 3 {
		t.Fatalf("XPath elements: %#v %v", items, err)
	}
	text, err := e.String(items[0], "@xpath:string(.//a)")
	if err != nil || text != "第一本书" {
		t.Fatalf("XPath scalar: %q %v", text, err)
	}
}

func TestRuleRegexAndHTML(t *testing.T) {
	e := NewEvaluator("", nil)
	got, err := e.String("第一章\n广告\n正文", "##广告\\n##")
	if err != nil || got != "第一章\n正文" {
		t.Fatalf("replace: %q %v", got, err)
	}
	got, err = e.String("aaa", "##a##b##")
	if err != nil || got != "b" {
		t.Fatalf("replace first: %q %v", got, err)
	}
	got, err = e.String("aaa", "##z##b###")
	if err != nil || got != "" {
		t.Fatalf("extract missing: %q %v", got, err)
	}
	got, err = e.String(ruleHTML, "id.content@html")
	if err != nil || !strings.Contains(got, "<p>") || strings.Contains(got, "不应显示") {
		t.Fatalf("HTML: %q %v", got, err)
	}
	got, err = e.String(ruleHTML, "id.content@all")
	if err != nil || !strings.Contains(got, "不应显示") {
		t.Fatalf("original DOM changed: %q %v", got, err)
	}
}

func TestRuleSyntaxErrors(t *testing.T) {
	e := NewEvaluator("", nil)
	for _, rule := range []string{"@css:div[", "@css:div > > a@text", "@json:$.data[", "@xpath://div[", "text##[##", "tag.div[0:2:0]@text", "tag.a@text||", "{{broken", "<js>result"} {
		t.Run(rule, func(t *testing.T) {
			if _, err := e.String(ruleHTML, rule); err == nil {
				t.Fatalf("accepted invalid rule %q", rule)
			}
		})
	}
}

func TestRuleJavaScript(t *testing.T) {
	e := NewEvaluator("https://example.test", map[string]any{"key": "中文", "page": 2}).SetJSLib(`function decorate(x) { return "《" + x + "》"; }`)
	cases := []struct {
		input      any
		rule, want string
	}{
		{"内容", "@js:decorate(result)", "《内容》"},
		{ruleHTML, "class.book.0@tag.a@text@js:decorate(result)", "《第一本书》"},
		{nil, "<js>return key + page;</js>", "中文2"},
		{nil, "<js>`/search?page={{page}}`</js>", "/search?page=2"},
		{nil, "{{baseUrl}}/search?q={{encodeURIComponent(key)}}&page={{page}}", "https://example.test/search?q=%E4%B8%AD%E6%96%87&page=2"},
		{`{"title":"书名"}`, "《{{$.title}}》", "《书名》"},
		{nil, "{{({a: '中文'}).a}}", "中文"},
		{nil, "@js:java.base64Decode(java.base64Encode('中文'))", "中文"},
		{nil, "@js:java.put('saved','值'); java.get('saved')", "值"},
		{nil, "@get:{saved}", "值"},
	}
	for _, test := range cases {
		t.Run(test.rule, func(t *testing.T) {
			got, err := e.String(test.input, test.rule)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
	for _, code := range []string{`java.ajax('https://example.test')`, `Packages.java.io.File('file')`, `importClass('java.io.File')`, `function (`} {
		if _, err := e.EvalJS(code, nil); err == nil {
			t.Fatalf("script should fail: %s", code)
		}
	}
	value, err := e.EvalJS(`var scope = new JavaImporter(); with(scope) { "兼容性探测"; }`, nil)
	if err != nil || value != "兼容性探测" {
		t.Fatalf("empty importer: %v %v", value, err)
	}
	value, err = e.EvalJS("typeof require + ',' + typeof process", nil)
	if err != nil || value != "undefined,undefined" {
		t.Fatalf("unexpected host capabilities: %v %v", value, err)
	}
}

func TestRuleTrustedBindings(t *testing.T) {
	stored := ""
	e := NewEvaluator("https://example.test", nil).SetJSBindings(map[string]any{
		"source": map[string]any{"getVariable": func() string { return stored }, "setVariable": func(value string) { stored = value }},
		"cookie": map[string]any{"getCookie": func(host string) string { return "session=fixture" }},
		"java":   map[string]any{"ajax": func(raw string) string { return `{"name":"离线响应"}` }, "base64Encode": func(value string) string { return "override:" + value }},
	}).SetJSLib(`const shared = "公共库"; function read() { return source.getVariable(); }`)
	got, err := e.String(nil, `<js>source.setVariable(shared);JSON.parse(java.ajax("https://fixture.test"))</js>$.name`)
	if err != nil || got != "离线响应" {
		t.Fatalf("native result: %q %v", got, err)
	}
	value, err := e.EvalJS(`read() + ":" + java.base64Encode("中文") + ":" + cookie.getCookie(baseUrl)`, nil)
	if err != nil || value != "公共库:override:中文:session=fixture" {
		t.Fatalf("native state: %v %v", value, err)
	}
	value, err = e.EvalJS(`java.hexDecodeToString("e4b8ade69687")`, nil)
	if err != nil || value != "中文" {
		t.Fatalf("hex decoding: %v %v", value, err)
	}
}

func TestRuleJavaScriptTimeout(t *testing.T) {
	e := NewEvaluator("", nil).SetJSTimeout(20 * time.Millisecond)
	start := time.Now()
	_, err := e.EvalJS(`while(true){}`, nil)
	if !errors.Is(err, ErrJSTimeout) {
		t.Fatalf("expected timeout, got %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("timeout took %v", time.Since(start))
	}
	value, err := e.EvalJS(`"下一次执行"`, nil)
	if err != nil || value != "下一次执行" {
		t.Fatalf("timeout leaked into next VM: %v %v", value, err)
	}
	e.SetJSLib(`for (;;) {}`)
	if _, err = e.EvalJS(`"unreachable"`, nil); !errors.Is(err, ErrJSTimeout) {
		t.Fatalf("library timeout: %v", err)
	}
}

func TestRuleJavaScriptContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e := NewEvaluator("", nil).SetJSContext(ctx)
	if _, err := e.EvalJS(`invalid syntax !`, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("already canceled: %v", err)
	}
	for _, library := range []bool{false, true} {
		name := "script"
		if library {
			name = "library"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started := make(chan struct{})
			finished := make(chan error, 1)
			e := NewEvaluator("", nil).SetJSTimeout(5 * time.Second).SetJSContext(ctx).SetJSBindings(map[string]any{"started": func() { close(started) }})
			code := `started(); for(;;){}`
			if library {
				e.SetJSLib(code)
				code = `"unreachable"`
			}
			go func() { _, err := e.EvalJS(code, nil); finished <- err }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("VM did not start")
			}
			cancel()
			select {
			case err := <-finished:
				if !errors.Is(err, context.Canceled) || errors.Is(err, ErrJSTimeout) {
					t.Fatalf("cancellation: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("VM ignored cancellation")
			}
			e.SetJSContext(context.Background()).SetJSLib("")
			if value, err := e.EvalJS(`"正常"`, nil); err != nil || value != "正常" {
				t.Fatalf("canceled VM leaked: %v %v", value, err)
			}
		})
	}
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	e = NewEvaluator("", nil).SetJSTimeout(5 * time.Second).SetJSContext(ctx)
	if _, err := e.EvalJS(`while(true){}`, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline: %v", err)
	}
}

func TestRuleJavaScriptResultGetter(t *testing.T) {
	for _, code := range []string{`({get value(){throw new Error("getter失败")}})`, `({get value(){while(true){}}})`} {
		t.Run(code, func(t *testing.T) {
			defer func() {
				if value := recover(); value != nil {
					t.Fatalf("result export panicked: %v", value)
				}
			}()
			e := NewEvaluator("", nil).SetJSTimeout(20 * time.Millisecond)
			_, err := e.EvalJS(code, nil)
			if err == nil {
				t.Fatal("getter error must be returned")
			}
			if strings.Contains(code, "while") && !errors.Is(err, ErrJSTimeout) {
				t.Fatalf("getter timeout: %v", err)
			}
		})
	}
}
