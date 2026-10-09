package booksource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/antchfx/xpath"
	"github.com/dop251/goja"
	"github.com/ohler55/ojg/jp"
	"golang.org/x/net/html"
)

// SourceCheckResult records one manual check without changing source settings.
type SourceCheckResult struct {
	Status     string    `json:"status"`
	Stage      string    `json:"stage"`
	Detail     string    `json:"detail"`
	CheckedAt  time.Time `json:"checked_at"`
	DurationMS int64     `json:"duration_ms"`
}

// CheckSource checks a source without enabling, disabling or removing it.
// An empty keyword performs syntax checks only; scripts are never evaluated.
func (c *Client) CheckSource(ctx context.Context, source Source, keyword string, deep bool) (result SourceCheckResult) {
	started := time.Now()
	result = SourceCheckResult{Stage: "rules", CheckedAt: started}
	if ctx == nil {
		ctx = context.Background()
	}
	keyword = strings.TrimSpace(keyword)
	timeout := 20 * time.Second
	if keyword == "" {
		timeout = 5 * time.Second
	} else if deep {
		timeout = 45 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer func() {
		if err := ctx.Err(); err != nil {
			result.Status, result.Detail = sourceCheckStatus(err), err.Error()
		}
		result.DurationMS = time.Since(started).Milliseconds()
	}()
	fail := func(err error) SourceCheckResult {
		result.Status, result.Detail = sourceCheckStatus(err), err.Error()
		return result
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if keyword == "" {
		// Parsers do not accept contexts. The bounded caller can still return
		// promptly when compilation is cancelled; the worker never runs scripts.
		type checked struct {
			limited bool
			err     error
		}
		done := make(chan checked, 1)
		go func() {
			limited, err := checkSourceRules(ctx, source)
			done <- checked{limited, err}
		}()
		select {
		case <-ctx.Done():
			return fail(ctx.Err())
		case check := <-done:
			if check.err != nil {
				return fail(check.err)
			}
			result.Status = "ok"
			result.Detail = "静态规则检查通过；未执行 JavaScript 或联网，实际可用性需填写关键词检查"
			if check.limited {
				result.Detail += "；动态模板及运行时输入仅能有限检查"
			}
			return result
		}
	}
	if err := ValidateSource(source); err != nil {
		return fail(err)
	}
	// Replacing the pointer preserves the caller's disabled source as-is.
	enabled := true
	source.Enabled = &enabled
	result.Stage = "search"
	books, err := c.Search(ctx, source, keyword, 1)
	if err != nil {
		return fail(err)
	}
	if len(books) == 0 {
		result.Status, result.Detail = "empty", "搜索成功但无结果；不代表书源失效，可换关键词重试"
		return result
	}
	result.Status, result.Detail = "ok", fmt.Sprintf("搜索成功，找到 %d 本书", len(books))
	if !deep {
		return result
	}
	result.Stage = "toc"
	book, err := c.BookInfo(ctx, source, books[0])
	if err != nil {
		return fail(err)
	}
	chapters, err := c.Chapters(ctx, source, book)
	if err != nil {
		return fail(err)
	}
	result.Stage = "content"
	if _, err = c.Content(ctx, source, book, chapters[0]); err != nil {
		return fail(err)
	}
	result.Status, result.Detail = "ok", fmt.Sprintf("搜索、首本详情、目录及首章正文检查通过（%d 章）", len(chapters))
	return result
}

var checkHTTPStatus = regexp.MustCompile(`(?i)\bHTTP\s+(\d{3})\b`)

func sourceCheckStatus(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, ErrJSTimeout):
		return "timeout"
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return "timeout"
	}
	message := strings.ToLower(err.Error())
	if match := checkHTTPStatus.FindStringSubmatch(message); match != nil {
		if match[1] == "401" {
			return "login"
		}
		// HTTP 403 also covers bans and access policies unrelated to login.
		return "error"
	}
	for _, marker := range []string{"不支持", "不受支持", "需要浏览器", "java宿主"} {
		if strings.Contains(message, marker) {
			return "unsupported"
		}
	}
	for _, marker := range []string{"请先登录", "需要登录", "尚未登录", "未登录", "登录已过期", "登录过期", "登录失效", "登录失败", "login required", "authentication required", "not logged in", "please log in", "please login"} {
		if strings.Contains(message, marker) {
			return "login"
		}
	}
	return "error"
}

type sourceCheckRule struct {
	name     string
	rule     string
	elements bool
	required bool
}

func checkSourceRules(ctx context.Context, source Source) (bool, error) {
	if err := ValidateSource(source); err != nil {
		return false, err
	}
	rules := []sourceCheckRule{
		{"ruleSearch.bookList", source.Search.BookList, true, true},
		{"ruleSearch.name", source.Search.Name, false, true},
		{"ruleSearch.bookUrl", source.Search.BookURL, false, true},
		{"ruleToc.chapterList", source.TOC.ChapterList, true, true},
		{"ruleToc.chapterName", source.TOC.ChapterName, false, false},
		{"ruleToc.chapterUrl", source.TOC.ChapterURL, false, true},
		{"ruleToc.nextTocUrl", source.TOC.NextTOCURL, false, false},
		{"ruleToc.isVolume", source.TOC.IsVolume, false, false},
		{"ruleContent.content", source.Content.Content, false, true},
		{"ruleContent.nextContentUrl", source.Content.NextContentURL, false, false},
	}
	for _, group := range []struct {
		name string
		rule BookRule
	}{{"ruleSearch", source.Search}, {"ruleBookInfo", source.BookInfo}} {
		r := group.rule
		for _, field := range []sourceCheckRule{
			{"init", r.Init, true, false}, {"name", r.Name, false, false},
			{"author", r.Author, false, false}, {"bookUrl", r.BookURL, false, false},
			{"tocUrl", r.TOCURL, false, false}, {"coverUrl", r.CoverURL, false, false},
			{"intro", r.Intro, false, false}, {"kind", r.Kind, false, false},
			{"lastChapter", r.LastChapter, false, false},
		} {
			// Required search fields were already added above.
			if group.name == "ruleSearch" && (field.name == "name" || field.name == "bookUrl") {
				continue
			}
			field.name = group.name + "." + field.name
			rules = append(rules, field)
		}
	}
	if replacement := strings.TrimSpace(source.Content.ReplaceRegex); replacement != "" {
		if !strings.HasPrefix(replacement, "##") {
			if i, _ := scriptStart(replacement); i < 0 {
				replacement = "##" + replacement
			}
		}
		rules = append(rules, sourceCheckRule{"ruleContent.replaceRegex", replacement, false, false})
	}
	limited := false
	for _, field := range rules {
		if err := ctx.Err(); err != nil {
			return limited, err
		}
		if field.required && strings.TrimSpace(field.rule) == "" {
			return limited, fmt.Errorf("缺少 %s", field.name)
		}
		partial, err := checkRuleSyntax(ctx, field.rule, field.elements, 0)
		limited = limited || partial
		if err != nil {
			return limited, fmt.Errorf("%s: %w", field.name, err)
		}
	}
	if err := compileCheckJS(source.JSLib, false); err != nil {
		return limited, fmt.Errorf("jsLib: %w", err)
	}
	partial, err := checkHeaderSyntax(source.Header)
	limited = limited || partial
	if err != nil {
		return limited, fmt.Errorf("header: %w", err)
	}
	requestURL := strings.TrimSpace(source.SearchURL)
	if strings.HasPrefix(requestURL, "@js:") || strings.HasPrefix(requestURL, "<js>") {
		partial, err := checkRuleSyntax(ctx, requestURL, false, 0)
		return limited || partial, err
	}
	if loc := optionPattern.FindStringIndex(requestURL); loc != nil {
		var options map[string]json.RawMessage
		if err := json.Unmarshal([]byte(requestURL[loc[0]+1:]), &options); err != nil {
			return limited, fmt.Errorf("searchUrl 选项 JSON: %w", err)
		}
		for _, key := range []string{"header", "headers"} {
			partial, err := checkHeaderSyntax(options[key])
			limited = limited || partial
			if err != nil {
				return limited, fmt.Errorf("searchUrl %s: %w", key, err)
			}
		}
		if raw := options["js"]; len(raw) > 0 {
			var code string
			if err := json.Unmarshal(raw, &code); err != nil {
				return limited, fmt.Errorf("searchUrl js: %w", err)
			}
			if err := compileCheckJS(code, true); err != nil {
				return limited, fmt.Errorf("searchUrl js: %w", err)
			}
		}
	}
	return limited || strings.Contains(requestURL, "{{"), ctx.Err()
}

// parseHeaders expands templates and may execute a source script, so static
// checks inspect its JSON shape and compile embedded JavaScript separately.
func checkHeaderSyntax(raw json.RawMessage) (bool, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return false, nil
	}
	if strings.HasPrefix(text, `"`) {
		if err := json.Unmarshal(raw, &text); err != nil {
			return false, err
		}
		if strings.TrimSpace(text) == "" {
			return false, nil
		}
		if strings.HasPrefix(text, "@js:") {
			return false, compileCheckJS(text[4:], true)
		}
	}
	var values map[string]any
	if err := json.Unmarshal([]byte(text), &values); err != nil {
		return false, err
	}
	limited := false
	for key, value := range values {
		switch value := value.(type) {
		case string:
			limited = limited || strings.Contains(value, "{{")
		case float64:
		default:
			return limited, fmt.Errorf("header %q 必须是字符串", key)
		}
	}
	return limited, nil
}

func compileCheckJS(code string, allowReturn bool) error {
	_, err := goja.Compile("source-check.js", code, false)
	if allowReturn && err != nil && strings.Contains(err.Error(), "Illegal return statement") {
		_, err = goja.Compile("source-check.js", "(function(){\n"+code+"\n}).call(this)", false)
	}
	return err
}

// checkRuleSyntax compiles individual syntax fragments without extracting from
// a mock response: an absent node must not hide an invalid later chain segment.
func checkRuleSyntax(ctx context.Context, rule string, elements bool, depth int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if depth > 32 {
		return false, fmt.Errorf("规则嵌套超过限制")
	}
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return false, nil
	}
	if i, tag := scriptStart(rule); i >= 0 {
		limited, err := checkRuleSyntax(ctx, rule[:i], elements, depth+1)
		if err != nil {
			return limited, err
		}
		code, tail := rule[i+len(tag):], ""
		if tag == "<js>" {
			end := strings.Index(strings.ToLower(code), "</js>")
			if end < 0 {
				return limited, fmt.Errorf("JavaScript规则缺少 </js>")
			}
			code, tail = code[:end], code[end+5:]
		}
		if strings.Contains(code, "{{") || strings.Contains(strings.ToLower(code), "@get:{") {
			limited = true
		} else if err := compileCheckJS(code, true); err != nil {
			return limited, err
		}
		partial, err := checkRuleSyntax(ctx, tail, elements, depth+1)
		return limited || partial, err
	}
	base, replacement := splitReplacement(rule)
	if len(replacement) != 0 {
		if _, err := regexp.Compile(replacement[0]); err != nil {
			return false, fmt.Errorf("替换正则无效（采用Go RE2语法）: %w", err)
		}
	}
	if strings.Contains(base, "{{") || strings.Contains(strings.ToLower(base), "@get:{") {
		return true, nil
	}
	for _, operator := range []string{"||", "&&", "%%"} {
		parts, err := splitRule(base, operator)
		if err != nil {
			return false, err
		}
		if len(parts) <= 1 {
			continue
		}
		limited := false
		for _, part := range parts {
			if strings.TrimSpace(part) == "" {
				return limited, fmt.Errorf("%s 两侧规则不可为空", operator)
			}
			partial, err := checkRuleSyntax(ctx, part, elements, depth+1)
			limited = limited || partial
			if err != nil {
				return limited, err
			}
		}
		return limited, nil
	}
	lower := strings.ToLower(base)
	switch {
	case base == "":
		return false, nil
	case strings.HasPrefix(lower, "@json:"):
		path := strings.TrimSpace(base[6:])
		if path == "" {
			return false, fmt.Errorf("JSONPath不可为空")
		}
		if !strings.HasPrefix(path, "$") && !strings.HasPrefix(path, "@") {
			path = "$." + path
		}
		_, err := jp.ParseString(path)
		return false, err
	case strings.HasPrefix(base, "$"):
		_, err := jp.ParseString(base)
		return false, err
	case strings.HasPrefix(lower, "@xpath:"):
		_, err := xpath.Compile(strings.TrimSpace(base[7:]))
		return false, err
	case strings.HasPrefix(base, "/"):
		_, err := xpath.Compile(base)
		return false, err
	}
	css := strings.HasPrefix(lower, "@css:") || strings.HasPrefix(base, "@@")
	if strings.HasPrefix(base, "@@") {
		base = base[2:]
	} else if css {
		base = base[5:]
	}
	parts, err := splitRule(strings.TrimPrefix(strings.TrimSpace(base), "@"), "@")
	if err != nil {
		return false, err
	}
	if !elements && len(parts) > 1 {
		parts = parts[:len(parts)-1]
	} else if !elements && len(parts) == 1 && isExtraction(parts[0]) {
		return false, nil
	}
	for _, part := range parts {
		if _, err := selectNodes(&html.Node{Type: html.DocumentNode}, strings.TrimSpace(part), css); err != nil {
			// Unprefixed paths depend on runtime input type. Accept a valid
			// relative JSONPath too, and disclose the ambiguity to the caller.
			if !css {
				if _, jsonErr := jp.ParseString("$." + base); jsonErr == nil {
					return true, nil
				}
			}
			return false, err
		}
	}
	return false, nil
}
