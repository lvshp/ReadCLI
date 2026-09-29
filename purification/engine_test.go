package purification

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func testRule(pattern, replacement string) Rule {
	return Rule{Name: "测试规则", Pattern: pattern, Replacement: replacement, IsEnabled: true, IsRegex: true, ScopeContent: true}
}

func TestRuleJSONDefaultsAndPreservation(t *testing.T) {
	var rule Rule
	if err := json.Unmarshal([]byte(`{"name":"默认","pattern":"\\h+","replacement":"$0\\n"}`), &rule); err != nil {
		t.Fatal(err)
	}
	if !rule.IsEnabled || !rule.IsRegex || !rule.ScopeContent || rule.ScopeTitle || rule.TimeoutMillisecond != 3000 {
		t.Fatalf("defaults=%+v", rule)
	}
	before := rule
	if _, err := Compile([]Rule{rule}); err != nil {
		t.Fatal(err)
	}
	if rule != before {
		t.Fatal("Compile mutated original rule")
	}
	if err := json.Unmarshal([]byte(`{"pattern":"x","isEnabled":false,"isRegex":false,"scopeContent":false,"scopeTitle":true}`), &rule); err != nil {
		t.Fatal(err)
	}
	if rule.IsEnabled || rule.IsRegex || rule.ScopeContent || !rule.ScopeTitle {
		t.Fatal("explicit flags changed")
	}
}

func TestJavaRegexAndReplacementSemantics(t *testing.T) {
	tests := []struct{ name, pattern, replacement, input, want string }{
		{"variable lookbehind", `(?<=中\w{0,3})广告`, "", "中文广告内容", "中文内容"},
		{"horizontal whitespace", `[\h]+`, "_", "a \t\u3000b\nc", "a_b\nc"},
		{"character intersection", `[a-z&&[^aeiou]]+`, "_", "abcde", "a_e"},
		{"capture numbering", `(?<first>a)(b)`, `${first}:$1:$2:$0:$10`, "ab", "a:a:b:ab:a0"},
		{"named backreference", `(?<first>a)\k<first>`, `${first}`, "aa", "a"},
		{"java escaping", `(x)`, `\$1|\\|$1\n`, "x", "$1|\\|xn"},
		{"optional unmatched group", `(x)(y)?`, `$2$1`, "x", "x"},
		{"quoted regex", `\Qa+b\E`, "x", "a+b aab", "x aab"},
		{"zero width Unicode", `(?=文)`, "！", "中文", "中！文"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, err := Compile([]Rule{testRule(test.pattern, test.replacement)})
			if err != nil {
				t.Fatal(err)
			}
			got, err := engine.Apply(context.Background(), Target{}, test.input)
			if err != nil || got != test.want {
				t.Fatalf("got %q err=%v, want %q", got, err, test.want)
			}
		})
	}
}

func TestReplacementJavaScriptSemantics(t *testing.T) {
	tests := []struct{ name, pattern, replacement, input, want string }{
		{"per match result", `[a-z]+`, `@js:result.toUpperCase()`, "a bb", "A BB"},
		{"fresh match bindings", `x`, `@js:var count=(typeof count==='undefined'?0:count)+1; count`, "xx", "11"},
		{"literal result", `x`, `@js:'$1\\n'`, "x", "$1\\n"},
		{"native replace and captures", `.+`, `@js:result.replace(/(?<letter>a)(b)/g,'$1-$2')`, "ab ab", "a-b a-b"},
		{"JS Unicode indices", `.+`, `@js:result.replace(/文/g,'字').replace(/😀/gu,'🌟')`, "😀中文😀", "🌟中字🌟"},
		{"JS UTF16 literal pattern", `.+`, `@js:result.replace(/😀/g,'X')`, "甲😀乙", "甲X乙"},
		{"JS unpaired surrogate", `.+`, `@js:JSON.stringify("\uD800".match(/\uD800/u))`, "x", `["\ud800"]`},
		{"JS identity escape", `.+`, `@js:result.replace(/\h/g,'_')`, "h h", "_ _"},
		{"JS invalid exec receiver", `x`, `@js:try { RegExp.prototype.exec.call({}); 'unexpected' } catch(e) { 'caught' }`, "x", "caught"},
		{"JS invalid test receiver", `x`, `@js:try { RegExp.prototype.test.call(undefined); 'unexpected' } catch(e) { 'caught' }`, "x", "caught"},
		{"JS lookbehind", `.+`, `@js:result.replace(/(?<=a+)b/g,'B')`, "aab", "aaB"},
		{"JS search test split", `.+`, `@js:result.search(/a/)+':'+/a/.test(result)+':'+result.split(/,/).join('|')`, "a,b", "0:true:a|b"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, err := Compile([]Rule{testRule(test.pattern, test.replacement)})
			if err != nil {
				t.Fatal(err)
			}
			got, err := engine.Apply(context.Background(), Target{}, test.input)
			if err != nil || got != test.want {
				t.Fatalf("got %q err=%v, want %q", got, err, test.want)
			}
		})
	}
}

func TestRuleScopeEnabledAndInputOrder(t *testing.T) {
	first := testRule("a", "b")
	first.Order = 100
	second := testRule("b", "c")
	second.Order = 1
	second.Scope = "目标小说;https://source.test"
	second.ExcludeScope = "排除小说"
	title := testRule("c", "d")
	title.ScopeContent = false
	title.ScopeTitle = true
	disabled := testRule("c", "坏")
	disabled.IsEnabled = false
	engine, err := Compile([]Rule{first, second, title, disabled})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		target      Target
		input, want string
	}{
		{Target{BookName: "目标小说"}, "a", "c"},
		{Target{BookName: "其他小说", SourceURL: "https://source.test"}, "a", "c"},
		{Target{BookName: "排除小说", SourceURL: "https://source.test"}, "a", "b"},
		{Target{}, "a", "b"},
		{Target{Title: true}, "c", "d"},
	} {
		got, err := engine.Apply(context.Background(), test.target, test.input)
		if err != nil || got != test.want {
			t.Fatalf("target=%+v got=%q err=%v", test.target, got, err)
		}
	}
	literal := testRule("x", `@js:$1\n`)
	literal.IsRegex = false
	engine, err = Compile([]Rule{literal})
	if err != nil {
		t.Fatal(err)
	}
	got, err := engine.Apply(context.Background(), Target{}, "x")
	if err != nil || got != literal.Replacement {
		t.Fatalf("literal replacement=%q %v", got, err)
	}
}

func TestPurificationTimeoutCancellationAndAtomicFailure(t *testing.T) {
	for _, test := range []struct{ name, pattern, replacement, input string }{
		{"JS infinite loop", "x", "@js:while(true){}", "x"},
		{"regex backtracking", `(a+)+$`, "", strings.Repeat("a", 200) + "!"},
		{"JS native regexp backtracking", "x", `@js:/(a+)+b(?=c)/.test('a'.repeat(200))`, "x"},
		{"JS throw", "x", `@js:throw new Error('failure')`, "x"},
		{"JS throwing result conversion", "x", `@js:({toString:function(){throw new Error('conversion')}})`, "x"},
	} {
		t.Run(test.name, func(t *testing.T) {
			rule := testRule(test.pattern, test.replacement)
			rule.TimeoutMillisecond = 30
			engine, err := Compile([]Rule{testRule("before", "after"), rule})
			if err != nil {
				t.Fatal(err)
			}
			original := "before " + test.input
			start := time.Now()
			got, err := engine.Apply(context.Background(), Target{}, original)
			if err == nil || got != original {
				t.Fatalf("failure lost original text: %q %v", got, err)
			}
			if time.Since(start) > time.Second {
				t.Fatalf("timeout took %s", time.Since(start))
			}
		})
	}
	engine, err := Compile([]Rule{testRule("x", "y")})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := engine.Apply(ctx, Target{}, "x"); got != "x" || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %q %v", got, err)
	}
}

func TestPurificationConcurrentUse(t *testing.T) {
	engine, err := Compile([]Rule{testRule(`(?<=中)[a-z]+`, `@js:result.toUpperCase()`)})
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			got, err := engine.Apply(context.Background(), Target{}, "中abc")
			if err != nil || got != "中ABC" {
				t.Errorf("got=%q err=%v", got, err)
			}
		}()
	}
	wait.Wait()
}

// Optional live compatibility fixture; the downloaded public JSON remains out
// of the repository, and this test performs no network requests.
func TestPublicLegadoRulesCompatibility(t *testing.T) {
	path := os.Getenv("READCLI_REPLACE_RULE_FIXTURE")
	if path == "" {
		t.Skip("set READCLI_REPLACE_RULE_FIXTURE to a downloaded Legado rules JSON")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rules []Rule
	if err = json.Unmarshal(data, &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules) != 20 {
		t.Fatalf("expected 20 real rules, got %d", len(rules))
	}
	engine, err := Compile(rules)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		id          int64
		input, want string
		title       bool
	}{
		{1, "第十二章", "第012章", true},
		{2, "ＡＢＣ１２３", "ABC123", false},
		{3, "...", "…", false},
		{4, "十之ꁘꁘ", "十之八九", false},
	} {
		var chosen Rule
		for _, rule := range rules {
			if rule.ID == test.id {
				chosen = rule
				break
			}
		}
		chosen.IsEnabled = true
		single, err := Compile([]Rule{chosen})
		if err != nil {
			t.Fatal(err)
		}
		got, err := single.Apply(context.Background(), Target{Title: test.title}, test.input)
		if err != nil || got != test.want {
			t.Fatalf("real rule %d got=%q err=%v want=%q", test.id, got, err, test.want)
		}
	}
	input := "ＡＢＣ１２３\n这是测试正文。\n他说道“你好。”\n正常段落。"
	output, err := engine.Apply(context.Background(), Target{BookName: "离线测试小说"}, input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "ABC123") || !strings.Contains(output, "正常段落") {
		t.Fatalf("real rules lost representative content: %q", output)
	}
	t.Logf("compiled %d real rules and applied representative JS/title/body samples", len(rules))
}

// Run separately without -race: race instrumentation changes backtracking costs
// enough to measure the detector overhead rather than the production budgets.
func TestPublicLegadoRulesChapterPerformance(t *testing.T) {
	if os.Getenv("READCLI_REPLACE_RULE_CHAPTER") != "1" {
		t.Skip("set READCLI_REPLACE_RULE_CHAPTER=1 for the normal-build chapter performance check")
	}
	path := os.Getenv("READCLI_REPLACE_RULE_FIXTURE")
	if path == "" {
		t.Skip("set READCLI_REPLACE_RULE_FIXTURE to the downloaded rules JSON")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rules []Rule
	if err = json.Unmarshal(data, &rules); err != nil {
		t.Fatal(err)
	}
	engine, err := Compile(rules)
	if err != nil {
		t.Fatal(err)
	}
	chapter := strings.Repeat("山间的风轻轻吹过树林，远处的灯火映照着归途。旅人停下脚步，看着天空。ＬＶ１２３，他说道“明天继续出发。”这是正常的章节内容。\n", 130)
	chapter += "\n请收藏本站：www.example.com\n"
	started := time.Now()
	cleaned, err := engine.Apply(context.Background(), Target{BookName: "合成测试小说"}, chapter)
	if err != nil {
		t.Fatalf("normal chapter purification failed: %v", err)
	}
	if !strings.Contains(cleaned, "LV123") || !strings.Contains(cleaned, "这是正常的章节内容") {
		t.Fatal("normal chapter lost representative content")
	}
	t.Logf("applied 19 enabled rules to %d synthetic characters in %s; output %d characters", len([]rune(chapter)), time.Since(started), len([]rune(cleaned)))
}
