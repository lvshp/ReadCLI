package booksource

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/andybalholm/cascadia"
	"github.com/antchfx/htmlquery"
	"github.com/antchfx/xpath"
	"github.com/ohler55/ojg/jp"
	"golang.org/x/net/html"
)

// Evaluator implements the portable subset of Legado's source rule language.
// Each JavaScript invocation has its own VM; put/get variables are shared by
// this evaluator. Selectors return detached values or HTML nodes for chaining.
type Evaluator struct {
	baseURL    string
	mu         sync.RWMutex
	variables  map[string]any
	jsLib      string
	jsTimeout  time.Duration
	jsBindings map[string]any
	jsContext  context.Context
}

func NewEvaluator(baseURL string, variables map[string]any) *Evaluator {
	e := &Evaluator{baseURL: baseURL, variables: make(map[string]any), jsTimeout: 500 * time.Millisecond}
	for k, v := range variables {
		e.variables[k] = v
	}
	return e
}

func (e *Evaluator) SetJSLib(code string) *Evaluator {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.jsLib = code
	return e
}

// SetJSBindings explicitly grants scripts trusted application capabilities.
// A java map augments the built-in helpers. Callers must bound any blocking
// native function (for example with HTTP context deadlines); VM interruption
// cannot preempt a Go function. Treat the supplied maps as immutable.
func (e *Evaluator) SetJSBindings(bindings map[string]any) *Evaluator {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.jsBindings = make(map[string]any, len(bindings))
	for key, value := range bindings {
		e.jsBindings[key] = value
	}
	return e
}

func (e *Evaluator) SetJSTimeout(timeout time.Duration) *Evaluator {
	e.mu.Lock()
	defer e.mu.Unlock()
	if timeout > 0 {
		e.jsTimeout = timeout
	}
	return e
}

// SetJSContext binds script execution to the request lifetime. Native bindings
// must use this same request context for their own blocking operations.
func (e *Evaluator) SetJSContext(ctx context.Context) *Evaluator {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.jsContext = ctx
	return e
}

func (e *Evaluator) Strings(input any, rule string) ([]string, error) {
	values, err := e.evaluate(input, strings.TrimSpace(rule), false, 0)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		s := stringify(value)
		if s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

func (e *Evaluator) String(input any, rule string) (string, error) {
	values, err := e.Strings(input, rule)
	return strings.Join(values, "\n"), err
}

func (e *Evaluator) Elements(input any, rule string) ([]any, error) {
	return e.evaluate(input, strings.TrimSpace(rule), true, 0)
}

func (e *Evaluator) evaluate(input any, rule string, elements bool, depth int) ([]any, error) {
	if depth > 32 {
		return nil, fmt.Errorf("规则嵌套超过限制")
	}
	if rule == "" {
		return nil, nil
	}
	// JavaScript operators belong to the script, not the selector combinator.
	if i, tag := scriptStart(rule); i >= 0 {
		result := input
		if before := strings.TrimSpace(rule[:i]); before != "" {
			values, err := e.evaluate(input, before, elements, depth+1)
			if err != nil {
				return nil, err
			}
			if elements {
				result = values
			} else {
				result = joinValues(values)
			}
		}
		code := rule[i+len(tag):]
		tail := ""
		if tag == "<js>" {
			end := strings.Index(strings.ToLower(code), "</js>")
			if end < 0 {
				return nil, fmt.Errorf("JavaScript规则缺少 </js>")
			}
			tail, code = strings.TrimSpace(code[end+5:]), code[:end]
		}
		if strings.Contains(code, "{{") || strings.Contains(strings.ToLower(code), "@get:{") {
			var err error
			code, err = e.expandTemplate(result, code, depth+1)
			if err != nil {
				return nil, err
			}
		}
		value, err := e.EvalJS(code, result)
		if err != nil {
			return nil, err
		}
		if tail != "" {
			return e.evaluate(value, tail, elements, depth+1)
		}
		return flatten(value), nil
	}
	base, replacement := splitReplacement(rule)
	if strings.Contains(base, "{{") || strings.Contains(strings.ToLower(base), "@get:{") {
		value, err := e.expandTemplate(input, base, depth)
		if err != nil {
			return nil, err
		}
		return replaceValues([]any{value}, replacement)
	}
	for _, operator := range []string{"||", "&&", "%%"} {
		parts, err := splitRule(base, operator)
		if err != nil {
			return nil, err
		}
		if len(parts) <= 1 {
			continue
		}
		var groups [][]any
		for _, part := range parts {
			if strings.TrimSpace(part) == "" {
				return nil, fmt.Errorf("%s 两侧规则不可为空", operator)
			}
		}
		for _, part := range parts {
			values, err := e.evaluate(input, strings.TrimSpace(part), elements, depth+1)
			if err != nil {
				return nil, err
			}
			if meaningful(values) {
				groups = append(groups, values)
				if operator == "||" {
					break
				}
			}
		}
		var values []any
		if operator == "%%" {
			for i := 0; ; i++ {
				found := false
				for _, group := range groups {
					if i < len(group) {
						values = append(values, group[i])
						found = true
					}
				}
				if !found {
					break
				}
			}
		} else {
			for _, group := range groups {
				values = append(values, group...)
			}
		}
		return replaceValues(values, replacement)
	}
	var values []any
	var err error
	lower := strings.ToLower(base)
	switch {
	case strings.HasPrefix(lower, "@json:"):
		values, err = jsonValues(input, strings.TrimSpace(base[6:]))
	case strings.HasPrefix(base, "$"):
		values, err = jsonValues(input, base)
	case strings.HasPrefix(lower, "@xpath:"):
		values, err = xpathValues(input, strings.TrimSpace(base[7:]), elements)
	case strings.HasPrefix(base, "/"):
		values, err = xpathValues(input, base, elements)
	case strings.HasPrefix(lower, "@css:") || strings.HasPrefix(base, "@@"):
		if strings.HasPrefix(base, "@@") {
			base = base[2:]
		} else {
			base = base[5:]
		}
		values, err = htmlValues(input, strings.TrimSpace(base), elements, true)
	case base == "":
		values = flatten(input)
	case isJSONValue(input):
		values, err = jsonValues(input, base)
	default:
		values, err = htmlValues(input, base, elements, false)
	}
	if err != nil {
		return nil, fmt.Errorf("规则 %q: %w", base, err)
	}
	return replaceValues(values, replacement)
}

func isJSONValue(input any) bool {
	switch value := input.(type) {
	case map[string]any, []any, json.RawMessage:
		return true
	case string:
		v := strings.TrimSpace(value)
		return (strings.HasPrefix(v, "{") || strings.HasPrefix(v, "[")) && json.Valid([]byte(v))
	}
	return false
}

func jsonValues(input any, rule string) ([]any, error) {
	if rule == "" {
		return nil, fmt.Errorf("JSONPath不可为空")
	}
	var value any
	switch raw := input.(type) {
	case string:
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, fmt.Errorf("JSON内容无效: %w", err)
		}
	case json.RawMessage:
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, fmt.Errorf("JSON内容无效: %w", err)
		}
	default:
		value = input
	}
	if !strings.HasPrefix(rule, "$") && !strings.HasPrefix(rule, "@") {
		rule = "$." + rule
	}
	path, err := jp.ParseString(rule)
	if err != nil {
		return nil, fmt.Errorf("JSONPath无效: %w", err)
	}
	var out []any
	for _, item := range path.Get(value) {
		out = append(out, flatten(item)...)
	}
	return out, nil
}

func htmlNode(input any) (*html.Node, error) {
	switch value := input.(type) {
	case *html.Node:
		return value, nil
	case *goquery.Selection:
		if len(value.Nodes) != 0 {
			return value.Nodes[0], nil
		}
		return html.Parse(strings.NewReader(""))
	case string:
		return html.Parse(strings.NewReader(value))
	default:
		return html.Parse(strings.NewReader(stringify(input)))
	}
}

func xpathValues(input any, rule string, elements bool) ([]any, error) {
	node, err := htmlNode(input)
	if err != nil {
		return nil, err
	}
	expr, err := xpath.Compile(rule)
	if err != nil {
		return nil, fmt.Errorf("XPath无效: %w", err)
	}
	value := expr.Evaluate(htmlquery.CreateXPathNavigator(node))
	if iterator, ok := value.(*xpath.NodeIterator); ok {
		var out []any
		for iterator.MoveNext() {
			nav := iterator.Current()
			if nav.NodeType() == xpath.AttributeNode || nav.NodeType() == xpath.TextNode {
				out = append(out, nav.Value())
				continue
			}
			n := nav.(*htmlquery.NodeNavigator).Current()
			if elements {
				out = append(out, n)
			} else {
				out = append(out, readableText(n, false))
			}
		}
		return out, nil
	}
	return flatten(value), nil
}

func htmlValues(input any, rule string, elements, css bool) ([]any, error) {
	if values, ok := input.([]any); ok {
		var out []any
		for _, item := range values {
			result, err := htmlValues(item, rule, elements, css)
			if err != nil {
				return nil, err
			}
			out = append(out, result...)
		}
		return out, nil
	}
	node, err := htmlNode(input)
	if err != nil {
		return nil, err
	}
	parts, err := splitRule(strings.TrimPrefix(rule, "@"), "@")
	if err != nil {
		return nil, err
	}
	extract := "text"
	if !elements && len(parts) > 1 {
		extract, parts = strings.TrimSpace(parts[len(parts)-1]), parts[:len(parts)-1]
	}
	if !elements && len(parts) == 1 && isExtraction(parts[0]) {
		extract, parts = parts[0], nil
	}
	nodes := []*html.Node{node}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var selected []*html.Node
		for _, current := range nodes {
			items, err := selectNodes(current, part, css)
			if err != nil {
				return nil, err
			}
			selected = append(selected, items...)
		}
		nodes = selected
	}
	var out []any
	seenAttributes := make(map[string]bool)
	for _, current := range nodes {
		if elements {
			out = append(out, current)
			continue
		}
		value := ""
		switch extract {
		case "text":
			value = readableText(current, false)
		case "ownText", "textNodes":
			value = readableText(current, true)
		case "html":
			value = outerHTML(current, true)
		case "all", "outerHtml":
			value = outerHTML(current, false)
		case "innerHtml":
			for child := current.FirstChild; child != nil; child = child.NextSibling {
				value += outerHTML(child, false)
			}
		default:
			for _, attr := range current.Attr {
				if attr.Key == extract {
					value = attr.Val
					break
				}
			}
			if seenAttributes[value] {
				continue
			}
			seenAttributes[value] = true
		}
		if value != "" {
			out = append(out, value)
		}
	}
	return out, nil
}

func isExtraction(rule string) bool {
	switch rule {
	case "text", "ownText", "textNodes", "html", "all", "outerHtml", "innerHtml", "href", "src", "content", "title", "value", "alt", "data-src":
		return true
	}
	return false
}

var legacyIndex = regexp.MustCompile(`^(.*?)([.!])(-?\d+(?::-?\d+)*)$`)
var bracketIndex = regexp.MustCompile(`^(.*)\[(!?[\d,:\s-]+)\]$`)

func selectNodes(root *html.Node, rule string, css bool) ([]*html.Node, error) {
	selector, indices, exclusion, ranged := rule, "", false, false
	if !css {
		if match := bracketIndex.FindStringSubmatch(rule); match != nil {
			selector, indices, exclusion, ranged = match[1], strings.TrimPrefix(match[2], "!"), strings.HasPrefix(match[2], "!"), true
		} else if match := legacyIndex.FindStringSubmatch(rule); match != nil {
			selector, indices, exclusion = match[1], match[3], match[2] == "!"
		}
	}
	var nodes []*html.Node
	if selector == "children" || selector == "" {
		for child := root.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode {
				nodes = append(nodes, child)
			}
		}
	} else {
		if !css {
			for _, prefix := range []string{"class.", "id.", "tag."} {
				if strings.HasPrefix(selector, prefix) {
					name := strings.TrimPrefix(selector, prefix)
					switch prefix {
					case "class.":
						selector = "." + name
					case "id.":
						selector = "#" + name
					case "tag.":
						selector = name
					}
					break
				}
			}
		}
		if !css && strings.HasPrefix(selector, "text.") {
			needle := strings.TrimPrefix(selector, "text.")
			var walk func(*html.Node)
			walk = func(n *html.Node) {
				if n.Type == html.ElementNode && strings.Contains(readableText(n, true), needle) {
					nodes = append(nodes, n)
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
			}
			walk(root)
		} else {
			matcher, err := cascadia.Compile(selector)
			if err != nil {
				return nil, fmt.Errorf("CSS选择器无效: %w", err)
			}
			selection := goquery.NewDocumentFromNode(root).Selection
			if root.Type == html.ElementNode && matcher.Match(root) {
				nodes = append(nodes, root)
			}
			nodes = append(nodes, selection.FindMatcher(matcher).Nodes...)
		}
	}
	if indices == "" {
		return nodes, nil
	}
	selected, err := parseIndices(indices, len(nodes), ranged)
	if err != nil {
		return nil, err
	}
	var out []*html.Node
	if exclusion {
		omit := make(map[int]bool)
		for _, index := range selected {
			omit[index] = true
		}
		for index, n := range nodes {
			if !omit[index] {
				out = append(out, n)
			}
		}
	} else {
		for _, index := range selected {
			out = append(out, nodes[index])
		}
	}
	return out, nil
}

func parseIndices(rule string, length int, ranged bool) ([]int, error) {
	var out []int
	seen := make(map[int]bool)
	appendIndex := func(i int) {
		if i < 0 {
			i += length
		}
		if i >= 0 && i < length && !seen[i] {
			out = append(out, i)
			seen[i] = true
		}
	}
	separator := ":"
	if ranged {
		separator = ","
	}
	for _, item := range strings.Split(rule, separator) {
		parts := strings.Split(strings.TrimSpace(item), ":")
		if len(parts) == 1 {
			index, err := strconv.Atoi(strings.TrimSpace(item))
			if err != nil {
				return nil, fmt.Errorf("索引无效: %q", item)
			}
			appendIndex(index)
			continue
		}
		if len(parts) > 3 {
			return nil, fmt.Errorf("索引区间无效: %q", item)
		}
		bounds := []int{0, length - 1, 1}
		for i, p := range parts {
			if strings.TrimSpace(p) != "" {
				value, err := strconv.Atoi(strings.TrimSpace(p))
				if err != nil {
					return nil, fmt.Errorf("索引区间无效: %q", item)
				}
				bounds[i] = value
			}
		}
		start, end, step := bounds[0], bounds[1], bounds[2]
		if step == 0 {
			return nil, fmt.Errorf("索引区间步长不能为0")
		}
		if length == 0 {
			continue
		}
		if start < 0 {
			start += length
		}
		if end < 0 {
			end += length
		}
		if start < 0 && end < 0 || start >= length && end >= length {
			continue
		}
		start = max(0, min(length-1, start))
		end = max(0, min(length-1, end))
		if step < 0 {
			step += length
			if step <= 0 {
				step = 1
			}
		}
		if start > end {
			for i := start; i >= end; {
				appendIndex(i)
				if i-end < step {
					break
				}
				i -= step
			}
		} else {
			for i := start; i <= end; {
				appendIndex(i)
				if end-i < step {
					break
				}
				i += step
			}
		}
	}
	return out, nil
}

func readableText(root *html.Node, own bool) string {
	var b strings.Builder
	block := func(tag string) bool {
		switch tag {
		case "br", "p", "div", "li", "h1", "h2", "h3", "h4", "section", "article", "tr", "blockquote", "dd", "dt":
			return true
		}
		return false
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			return
		}
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
			return
		}
		if block(n.Data) {
			b.WriteByte('\n')
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if !own || child.Type == html.TextNode {
				walk(child)
			}
		}
		if block(n.Data) {
			b.WriteByte('\n')
		}
	}
	walk(root)
	var lines []string
	for _, line := range strings.Split(b.String(), "\n") {
		if text := strings.Join(strings.Fields(line), " "); text != "" {
			lines = append(lines, text)
		}
	}
	return strings.Join(lines, "\n")
}

func outerHTML(node *html.Node, clean bool) string {
	if clean {
		selection := goquery.NewDocumentFromNode(node).Selection.Clone()
		selection.Find("script,style").Remove()
		if len(selection.Nodes) > 0 {
			node = selection.Nodes[0]
		}
	}
	var b strings.Builder
	_ = html.Render(&b, node)
	return b.String()
}

func stringify(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case *html.Node:
		return outerHTML(v, false)
	case json.Number:
		return string(v)
	case []byte:
		return string(v)
	case bool:
		return strconv.FormatBool(v)
	}
	kind := reflect.TypeOf(value).Kind()
	if kind == reflect.Map || kind == reflect.Slice || kind == reflect.Array {
		if data, err := json.Marshal(value); err == nil {
			return string(data)
		}
	}
	return fmt.Sprint(value)
}

func flatten(value any) []any {
	if value == nil {
		return nil
	}
	if values, ok := value.([]any); ok {
		return values
	}
	if values, ok := value.([]string); ok {
		out := make([]any, len(values))
		for i, v := range values {
			out[i] = v
		}
		return out
	}
	return []any{value}
}

func meaningful(values []any) bool {
	for _, value := range values {
		if value != nil && stringify(value) != "" {
			return true
		}
	}
	return false
}
func joinValues(values []any) string {
	var out []string
	for _, value := range values {
		if value != nil {
			out = append(out, stringify(value))
		}
	}
	return strings.Join(out, "\n")
}

func splitReplacement(rule string) (string, []string) {
	parts := strings.SplitN(rule, "##", 4)
	return strings.TrimSpace(parts[0]), parts[1:]
}

func replaceValues(values []any, parts []string) ([]any, error) {
	if len(parts) == 0 {
		return values, nil
	}
	re, err := regexp.Compile(parts[0])
	if err != nil {
		return nil, fmt.Errorf("替换正则无效（采用Go RE2语法）: %w", err)
	}
	replacement := ""
	if len(parts) > 1 {
		replacement = parts[1]
	}
	// Java-style $1 followed by Chinese text must not become a Go group name.
	replacement = regexp.MustCompile(`\$(\d+)`).ReplaceAllString(replacement, `${$1}`)
	out := make([]any, 0, len(values))
	for _, value := range values {
		s := stringify(value)
		if len(parts) > 2 {
			if loc := re.FindStringSubmatchIndex(s); loc != nil {
				s = string(re.ExpandString(nil, replacement, s, loc))
			} else {
				s = ""
			}
		} else {
			s = re.ReplaceAllString(s, replacement)
		}
		out = append(out, s)
	}
	return out, nil
}

// splitRule keeps operators inside JSON filters, CSS attributes and templates.
func splitRule(rule, separator string) ([]string, error) {
	var parts []string
	var stack []byte
	quote := byte(0)
	start := 0
	for i := 0; i < len(rule); i++ {
		c := rule[i]
		if c == '\\' {
			i++
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			quote = c
			continue
		}
		if len(stack) == 0 && strings.HasPrefix(rule[i:], separator) {
			parts = append(parts, rule[start:i])
			i += len(separator) - 1
			start = i + 1
			continue
		}
		switch c {
		case '[', '(', '{':
			stack = append(stack, c)
		case ']', ')', '}':
			if len(stack) == 0 {
				return nil, fmt.Errorf("规则括号不匹配: %q", rule)
			}
			open := stack[len(stack)-1]
			if (c == ']' && open != '[') || (c == ')' && open != '(') || (c == '}' && open != '{') {
				return nil, fmt.Errorf("规则括号不匹配: %q", rule)
			}
			stack = stack[:len(stack)-1]
		}
	}
	if quote != 0 || len(stack) != 0 {
		return nil, fmt.Errorf("规则引号或括号未闭合: %q", rule)
	}
	return append(parts, rule[start:]), nil
}

func scriptStart(rule string) (int, string) {
	lower := strings.ToLower(rule)
	var stack []byte
	quote := byte(0)
	for i := 0; i < len(rule); i++ {
		c := rule[i]
		if c == '\\' {
			i++
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			quote = c
			continue
		}
		if len(stack) == 0 {
			if strings.HasPrefix(lower[i:], "@js:") {
				return i, "@js:"
			}
			if strings.HasPrefix(lower[i:], "<js>") {
				return i, "<js>"
			}
		}
		switch c {
		case '[', '(', '{':
			stack = append(stack, c)
		case ']', ')', '}':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return -1, ""
}

func (e *Evaluator) expandTemplate(input any, rule string, depth int) (string, error) {
	var out strings.Builder
	for len(rule) > 0 {
		start := strings.Index(rule, "{{")
		get := strings.Index(strings.ToLower(rule), "@get:{")
		if get >= 0 && (start < 0 || get < start) {
			out.WriteString(rule[:get])
			end := strings.Index(rule[get+6:], "}")
			if end < 0 {
				return "", fmt.Errorf("@get变量缺少结束括号")
			}
			key := rule[get+6 : get+6+end]
			e.mu.RLock()
			value := e.variables[key]
			e.mu.RUnlock()
			out.WriteString(stringify(value))
			rule = rule[get+7+end:]
			continue
		}
		if start < 0 {
			out.WriteString(rule)
			break
		}
		out.WriteString(rule[:start])
		end := templateEnd(rule, start+2)
		if end < 0 {
			return "", fmt.Errorf("模板缺少结束标记 }}")
		}
		code := strings.TrimSpace(rule[start+2 : end])
		var value any
		var err error
		if strings.HasPrefix(code, "@") || strings.HasPrefix(code, "$.") || strings.HasPrefix(code, "$[") || strings.HasPrefix(code, "//") {
			var values []any
			values, err = e.evaluate(input, code, false, depth+1)
			value = joinValues(values)
		} else {
			value, err = e.EvalJS(code, input)
		}
		if err != nil {
			return "", err
		}
		out.WriteString(stringify(value))
		rule = rule[end+2:]
	}
	return out.String(), nil
}

func templateEnd(rule string, start int) int {
	depth := 0
	quote := byte(0)
	for i := start; i < len(rule); i++ {
		c := rule[i]
		if c == '\\' {
			i++
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			quote = c
			continue
		}
		if c == '{' {
			depth++
		}
		if c == '}' {
			if depth == 0 && i+1 < len(rule) && rule[i+1] == '}' {
				return i
			}
			depth--
		}
	}
	return -1
}
