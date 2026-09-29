// Package purification applies Legado-compatible reading replacement rules.
package purification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/dlclark/regexp2"
	"github.com/dop251/goja"
)

const (
	maxTextBytes = 8 << 20
	maxRuleBytes = 128 << 10
	maxMatches   = 100000
	matchTimeout = 500 * time.Millisecond
	applyTimeout = 5 * time.Second
)

// Rule retains the original pattern and replacement for editing and export.
type Rule struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Group              string `json:"group,omitempty"`
	Pattern            string `json:"pattern"`
	Replacement        string `json:"replacement"`
	Scope              string `json:"scope,omitempty"`
	ExcludeScope       string `json:"excludeScope,omitempty"`
	ScopeTitle         bool   `json:"scopeTitle"`
	ScopeContent       bool   `json:"scopeContent"`
	IsEnabled          bool   `json:"isEnabled"`
	IsRegex            bool   `json:"isRegex"`
	TimeoutMillisecond int64  `json:"timeoutMillisecond,omitempty"`
	Order              int    `json:"order"`
}

func (r *Rule) UnmarshalJSON(data []byte) error {
	type plain Rule
	value := plain{IsEnabled: true, IsRegex: true, ScopeContent: true, TimeoutMillisecond: 3000, Order: math.MinInt32}
	if strings.TrimSpace(string(data)) == "null" {
		return errors.New("净化规则必须是 JSON 对象")
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*r = Rule(value)
	return nil
}

// Target selects chapter-title or body rules for a book and its source URL.
type Target struct {
	BookName  string
	SourceURL string
	Title     bool
}

type compiledRule struct {
	rule        Rule
	regex       *regexp2.Regexp
	groups      map[string]int
	replacement []replacementPart
	script      *goja.Program
	timeout     time.Duration
}

type Engine struct{ rules []compiledRule }

// Compile validates every rule, including disabled rules, without changing the
// input. It preserves input order; the loader orders rules within their files.
func Compile(rules []Rule) (*Engine, error) {
	if len(rules) > 2000 {
		return nil, errors.New("净化规则超过 2000 条限制")
	}
	engine := &Engine{}
	for i, rule := range rules {
		compiled, err := compileRule(rule)
		if err != nil {
			return nil, fmt.Errorf("净化规则 %d（%s）: %w", i+1, rule.Name, err)
		}
		engine.rules = append(engine.rules, compiled)
	}
	return engine, nil
}

func compileRule(rule Rule) (compiledRule, error) {
	result := compiledRule{rule: rule}
	if rule.Pattern == "" {
		return result, errors.New("匹配内容不能为空")
	}
	if len(rule.Pattern) > maxRuleBytes || len(rule.Replacement) > maxRuleBytes {
		return result, errors.New("规则内容过长")
	}
	ms := rule.TimeoutMillisecond
	if ms <= 0 {
		ms = 3000
	}
	result.timeout = time.Duration(min(ms, int64(3000))) * time.Millisecond
	if !rule.IsRegex {
		return result, nil
	}
	pattern, names, err := javaPattern(rule.Pattern)
	if err != nil {
		return result, err
	}
	result.regex, err = regexp2.Compile(pattern, regexp2.None)
	if err != nil {
		return result, errors.New("正则表达式语法错误或不支持")
	}
	result.regex.MatchTimeout = min(result.timeout, matchTimeout)
	result.groups = names
	if strings.HasPrefix(rule.Replacement, "@js:") {
		result.script, err = goja.Compile("purification.js", strings.TrimPrefix(rule.Replacement, "@js:"), false)
		if err != nil {
			return result, errors.New("JavaScript 替换表达式语法错误")
		}
	} else {
		result.replacement, err = parseReplacement(rule.Replacement, len(result.regex.GetGroupNumbers())-1, names)
	}
	return result, err
}

// Apply is safe for concurrent use. On any error it returns the original text,
// never a partially rewritten chapter. No filesystem or network APIs are exposed
// to replacement scripts. Both Go and JavaScript regexes have bounded matches.
func (e *Engine) Apply(ctx context.Context, target Target, text string) (string, error) {
	if err := ctx.Err(); err != nil {
		return text, err
	}
	if len(text) > maxTextBytes {
		return text, errors.New("净化文本超过 8 MiB 限制")
	}
	if e == nil {
		return text, nil
	}
	ctx, cancel := context.WithTimeout(ctx, applyTimeout)
	defer cancel()
	output := text
	for _, rule := range e.rules {
		if !rule.applies(target) {
			continue
		}
		ruleCtx, stop := context.WithTimeout(ctx, rule.timeout)
		next, err := rule.apply(ruleCtx, output)
		if err == nil {
			err = ruleCtx.Err()
		}
		stop()
		if err != nil {
			return text, fmt.Errorf("净化规则“%s”: %w", rule.rule.Name, err)
		}
		output = next
	}
	if err := ctx.Err(); err != nil {
		return text, err
	}
	return output, nil
}

func (r compiledRule) applies(target Target) bool {
	if !r.rule.IsEnabled || (target.Title && !r.rule.ScopeTitle) || (!target.Title && !r.rule.ScopeContent) {
		return false
	}
	matches := func(scope string) bool {
		// Legado tests whether the stored scope contains the book's name or
		// origin. Empty target fields must not accidentally match every scope.
		return (target.BookName != "" && strings.Contains(scope, target.BookName)) || (target.SourceURL != "" && strings.Contains(scope, target.SourceURL))
	}
	return (r.rule.Scope == "" || matches(r.rule.Scope)) && (r.rule.ExcludeScope == "" || !matches(r.rule.ExcludeScope))
}

func (r compiledRule) apply(ctx context.Context, text string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !r.rule.IsRegex {
		count := strings.Count(text, r.rule.Pattern)
		if len(r.rule.Replacement) > len(r.rule.Pattern) && count > (maxTextBytes-len(text))/(len(r.rule.Replacement)-len(r.rule.Pattern)) {
			return "", errors.New("替换结果超过 8 MiB 限制")
		}
		return strings.ReplaceAll(text, r.rule.Pattern, r.rule.Replacement), nil
	}
	runes := []rune(text)
	match, err := r.regex.FindRunesMatch(runes)
	var output strings.Builder
	position, count := 0, 0
	for match != nil && err == nil {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		count++
		if count > maxMatches {
			return "", errors.New("单条规则匹配次数过多")
		}
		replacement := ""
		if r.script != nil {
			replacement, err = runReplacementJS(ctx, r.script, match.String())
			if err != nil {
				return "", err
			}
		} else {
			var expanded strings.Builder
			for _, part := range r.replacement {
				fragment := part.literal
				if part.group < 0 {
					fragment = part.literal
				} else if group := match.GroupByNumber(part.group); group != nil {
					fragment = group.String()
				}
				if expanded.Len()+len(fragment) > maxTextBytes {
					return "", errors.New("替换结果超过 8 MiB 限制")
				}
				expanded.WriteString(fragment)
			}
			replacement = expanded.String()
		}
		prefix := string(runes[position:match.Index])
		if output.Len()+len(prefix)+len(replacement) > maxTextBytes {
			return "", errors.New("替换结果超过 8 MiB 限制")
		}
		output.WriteString(prefix)
		output.WriteString(replacement)
		position = match.Index + match.Length
		match, err = r.regex.FindNextMatch(match)
	}
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("正则匹配超时: %w", context.DeadlineExceeded)
	}
	tail := string(runes[position:])
	if output.Len()+len(tail) > maxTextBytes {
		return "", errors.New("替换结果超过 8 MiB 限制")
	}
	output.WriteString(tail)
	return output.String(), nil
}

func runReplacementJS(ctx context.Context, program *goja.Program, match string) (result string, err error) {
	defer func() {
		if recover() != nil {
			result, err = "", errors.New("JavaScript 替换执行失败")
			if ctx.Err() != nil {
				err = ctx.Err()
			}
		}
	}()
	vm := goja.New()
	vm.SetMaxCallStackSize(256)
	stop := context.AfterFunc(ctx, func() { vm.Interrupt(ctx.Err()) })
	defer stop()
	if err := installBoundedJSRegex(vm, ctx); err != nil {
		return "", err
	}
	if err := vm.Set("result", match); err != nil {
		return "", err
	}
	value, err := vm.RunProgram(program)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", errors.New("JavaScript 替换执行失败")
	}
	// Legado quotes the evaluated string before appendReplacement: $ and
	// backslashes returned by JavaScript are literal, not capture references.
	result = value.String()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if len(result) > maxTextBytes {
		return "", errors.New("JavaScript 替换结果过长")
	}
	return result, nil
}
