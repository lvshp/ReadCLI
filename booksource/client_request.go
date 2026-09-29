package booksource

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html/charset"
)

type Client struct {
	sessionMu        sync.Mutex
	sessionSaveMu    sync.Mutex
	sessions         *clientSessions
	HTTPClient       *http.Client
	MaxResponseBytes int64
	MaxPages         int
}

func NewClient() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{HTTPClient: &http.Client{Timeout: 30 * time.Second, Jar: jar}, MaxResponseBytes: 8 << 20, MaxPages: 50}
}
func (c *Client) pageLimit() int {
	if c.MaxPages > 0 {
		return c.MaxPages
	}
	return 50
}
func (c *Client) responseLimit() int64 {
	if c.MaxResponseBytes > 0 {
		return c.MaxResponseBytes
	}
	return 8 << 20
}

var optionPattern = regexp.MustCompile(`,\s*\{`)
var templatePattern = regexp.MustCompile(`\{\{([\s\S]*?)\}\}`)
var pagePattern = regexp.MustCompile(`<([0-9]+(?:,[0-9]+)+)>`)

func baseEvaluator(source Source, base string, vars map[string]any) *Evaluator {
	if vars == nil {
		vars = make(map[string]any)
	}
	vars["source"] = map[string]any{"bookSourceUrl": source.URL, "bookSourceName": source.Name}
	e := NewEvaluator(base, vars)
	e.SetJSLib(source.JSLib)
	return e
}

func (c *Client) fetch(ctx context.Context, source Source, base, rawURL string, vars map[string]any, limit int64) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	e := c.evaluator(ctx, source, base, vars)
	requestURL := strings.TrimSpace(rawURL)
	if strings.HasPrefix(requestURL, "@js:") || strings.HasPrefix(requestURL, "<js>") {
		code := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(requestURL, "@js:"), "<js>"), "</js>")
		result, err := e.EvalJS(code, nil)
		if err != nil {
			return "", "", fmt.Errorf("URL JavaScript: %w", err)
		}
		requestURL = fmt.Sprint(result)
	}
	var options map[string]json.RawMessage
	if loc := optionPattern.FindStringIndex(requestURL); loc != nil {
		rawOptions := requestURL[loc[0]+1:]
		requestURL = requestURL[:loc[0]]
		if err := json.Unmarshal([]byte(rawOptions), &options); err != nil {
			return "", "", fmt.Errorf("URL 选项 JSON: %w", err)
		}
	}
	var err error
	requestURL, err = expandTemplate(requestURL, e, vars, true)
	if err != nil {
		return "", "", err
	}
	if page, ok := vars["page"].(int); ok && page > 0 {
		requestURL = pagePattern.ReplaceAllStringFunc(requestURL, func(match string) string {
			pages := strings.Split(match[1:len(match)-1], ",")
			index := page - 1
			if index >= len(pages) {
				index = len(pages) - 1
			}
			return pages[index]
		})
	}
	if strings.HasPrefix(strings.ToLower(requestURL), "data:") {
		parts := strings.SplitN(requestURL, ",", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("无效 data URI")
		}
		var data []byte
		if strings.HasSuffix(strings.ToLower(parts[0]), ";base64") {
			if len(parts[1]) > 2*int(c.responseLimit()) {
				return "", "", fmt.Errorf("data URI 超过响应限制")
			}
			data, err = base64.StdEncoding.DecodeString(parts[1])
		} else {
			var decoded string
			decoded, err = url.PathUnescape(parts[1])
			data = []byte(decoded)
		}
		if err != nil {
			return "", "", fmt.Errorf("data URI 解码: %w", err)
		}
		if int64(len(data)) > c.responseLimit()/2 {
			return "", "", fmt.Errorf("data URI 超过响应限制")
		}
		if len(options["type"]) > 0 {
			return hex.EncodeToString(data), rawURL, nil
		}
		return string(data), rawURL, nil
	}
	requestURL, err = resolveURL(base, requestURL)
	if err != nil {
		return "", "", err
	}
	for _, unsupported := range []string{"webView", "webJs", "serverID"} {
		if raw := options[unsupported]; len(raw) > 0 && string(raw) != "false" && string(raw) != `"false"` && string(raw) != "0" && string(raw) != "null" && string(raw) != `""` {
			return "", "", fmt.Errorf("URL 选项 %s 需要浏览器或 Legado 专用能力，暂不支持", unsupported)
		}
	}
	if raw := options["js"]; len(raw) > 0 {
		var code string
		if err := json.Unmarshal(raw, &code); err != nil {
			return "", "", fmt.Errorf("URL js: %w", err)
		}
		result, err := e.EvalJS(code, requestURL)
		if err != nil {
			return "", "", err
		}
		requestURL, err = resolveURL(base, fmt.Sprint(result))
		if err != nil {
			return "", "", err
		}
	}
	method := "GET"
	if raw := options["method"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &method); err != nil {
			return "", "", fmt.Errorf("URL method: %w", err)
		}
		method = strings.ToUpper(method)
	}
	if method != "GET" && method != "POST" {
		return "", "", fmt.Errorf("暂不支持请求方法 %q", method)
	}
	headers, err := parseHeaders(source.Header, e, vars)
	if err != nil {
		return "", "", fmt.Errorf("书源 header: %w", err)
	}
	for _, key := range []string{"header", "headers"} {
		extra, err := parseHeaders(options[key], e, vars)
		if err != nil {
			return "", "", fmt.Errorf("URL %s: %w", key, err)
		}
		for name, values := range extra {
			headers[name] = values
		}
	}
	body := ""
	bodyJSON := false
	if raw := options["body"]; len(raw) > 0 && string(raw) != "null" {
		if raw[0] == '"' {
			if err := json.Unmarshal(raw, &body); err != nil {
				return "", "", err
			}
			bodyJSON = json.Valid([]byte(body))
			if bodyJSON {
				body, err = expandJSONTemplate(body, e, vars)
			} else {
				body, err = expandTemplate(body, e, vars, true)
			}
		} else {
			bodyJSON = true
			body, err = expandJSONTemplate(string(raw), e, vars)
		}
		if err != nil {
			return "", "", fmt.Errorf("URL body: %w", err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, strings.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.URL.RawQuery = normalizeRawQuery(req.URL.RawQuery)
	req.Header = headers
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "ReadCLI/1.0")
	}
	if method == "POST" && req.Header.Get("Content-Type") == "" {
		if bodyJSON {
			req.Header.Set("Content-Type", "application/json; charset=utf-8")
		} else {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	}
	client := c.httpClientForSource(source, ctx)
	response, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer response.Body.Close()
	c.recordSessionCookies(source, response.Request.URL.String(), response.Cookies(), client.Jar)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", "", fmt.Errorf("HTTP %d %s", response.StatusCode, response.Status)
	}
	if limit <= 0 {
		limit = c.responseLimit()
	}
	data, err := readBounded(response.Body, limit)
	if err != nil {
		return "", "", err
	}
	contentType := response.Header.Get("Content-Type")
	var forcedCharset string
	if raw := options["charset"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &forcedCharset); err != nil {
			return "", "", err
		}
	}
	var decoded []byte
	if forcedCharset != "" {
		encoding, _ := charset.Lookup(forcedCharset)
		if encoding == nil {
			return "", "", fmt.Errorf("不支持字符编码 %q", forcedCharset)
		}
		decoded, err = readBounded(encoding.NewDecoder().Reader(bytes.NewReader(data)), limit)
	} else {
		encoding, name, _ := charset.DetermineEncoding(data, contentType)
		if name == "windows-1252" && utf8.Valid(data) {
			decoded = data
		} else {
			decoded, err = readBounded(encoding.NewDecoder().Reader(bytes.NewReader(data)), limit)
		}
	}
	if err != nil {
		return "", "", fmt.Errorf("响应字符解码: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	return string(decoded), response.Request.URL.String(), nil
}

func resolveURL(base, reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	if strings.HasPrefix(strings.ToLower(reference), "data:") {
		return reference, nil
	}
	if reference == "" {
		return "", fmt.Errorf("解析得到空链接")
	}
	suffix := ""
	if loc := optionPattern.FindStringIndex(reference); loc != nil {
		suffix = reference[loc[0]:]
		reference = reference[:loc[0]]
	}
	u, err := url.Parse(reference)
	if err != nil {
		return "", err
	}
	if !u.IsAbs() {
		parent, err := url.Parse(base)
		if err != nil {
			return "", err
		}
		u = parent.ResolveReference(u)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("仅支持 HTTP(S) 链接: %q", reference)
	}
	u.Fragment = ""
	return u.String() + suffix, nil
}

func expandTemplate(input string, e *Evaluator, vars map[string]any, escape bool) (string, error) {
	var failure error
	result := templatePattern.ReplaceAllStringFunc(input, func(match string) string {
		expr := strings.TrimSpace(match[2 : len(match)-2])
		value, ok := vars[expr]
		if !ok {
			var err error
			value, err = e.EvalJS(expr, nil)
			if err != nil {
				failure = fmt.Errorf("URL 模板 %q: %w", expr, err)
				return ""
			}
		}
		valueString := fmt.Sprint(value)
		if value == nil {
			valueString = ""
		}
		if escape && (expr == "key" || expr == "searchKey") {
			return url.QueryEscape(valueString)
		}
		return valueString
	})
	return result, failure
}
func expandJSONTemplate(input string, e *Evaluator, vars map[string]any) (string, error) {
	var value any
	if err := json.Unmarshal([]byte(input), &value); err != nil {
		return "", err
	}
	var walk func(any) (any, error)
	walk = func(v any) (any, error) {
		switch item := v.(type) {
		case string:
			return expandTemplate(item, e, vars, false)
		case map[string]any:
			for key, child := range item {
				changed, err := walk(child)
				if err != nil {
					return nil, err
				}
				item[key] = changed
			}
			return item, nil
		case []any:
			for i, child := range item {
				changed, err := walk(child)
				if err != nil {
					return nil, err
				}
				item[i] = changed
			}
			return item, nil
		default:
			return v, nil
		}
	}
	changed, err := walk(value)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(changed)
	return string(data), err
}
func parseHeaders(raw json.RawMessage, e *Evaluator, vars map[string]any) (http.Header, error) {
	headers := make(http.Header)
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return headers, nil
	}
	if raw[0] == '"' {
		var inner string
		if err := json.Unmarshal(raw, &inner); err != nil {
			return nil, err
		}
		if strings.TrimSpace(inner) == "" {
			return headers, nil
		}
		if strings.HasPrefix(inner, "@js:") {
			result, err := e.EvalJS(strings.TrimPrefix(inner, "@js:"), nil)
			if err != nil {
				return nil, err
			}
			if str, ok := result.(string); ok {
				inner = str
			} else {
				value, err := json.Marshal(result)
				if err != nil {
					return nil, err
				}
				inner = string(value)
			}
		}
		raw = []byte(inner)
	}
	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	for key, value := range values {
		var stringValue string
		switch v := value.(type) {
		case string:
			stringValue = v
		case json.Number:
			stringValue = v.String()
		case float64:
			stringValue = strconv.FormatFloat(v, 'f', -1, 64)
		default:
			return nil, fmt.Errorf("header %q 必须是字符串", key)
		}
		expanded, err := expandTemplate(stringValue, e, vars, false)
		if err != nil {
			return nil, err
		}
		headers.Set(key, expanded)
	}
	return headers, nil
}

// Go escapes URL paths but sends RawQuery verbatim. Source JavaScript commonly
// inserts Chinese search text directly, so encode only bytes forbidden in a
// query while preserving delimiters and already escaped sequences.
func normalizeRawQuery(query string) string {
	const hex = "0123456789ABCDEF"
	isHex := func(c byte) bool { return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }
	var out strings.Builder
	out.Grow(len(query))
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if ch == '%' && i+2 < len(query) && isHex(query[i+1]) && isHex(query[i+2]) {
			out.WriteString(query[i : i+3])
			i += 2
			continue
		}
		if ch <= 0x20 || ch >= 0x7f || ch == '%' || strings.ContainsRune("\"<>\\^`{|}", rune(ch)) {
			out.WriteByte('%')
			out.WriteByte(hex[ch>>4])
			out.WriteByte(hex[ch&15])
		} else {
			out.WriteByte(ch)
		}
	}
	return out.String()
}
