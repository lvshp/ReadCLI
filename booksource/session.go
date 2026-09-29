package booksource

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/publicsuffix"
)

// LoginField describes an input supported by this client. Script buttons that
// register accounts, purchase content or change server settings are not run.
type LoginField struct {
	Name   string
	Secret bool
}

const sessionFileLimit int64 = 8 << 20

type sourceSession struct {
	Variable  string                    `json:"variable,omitempty"`
	LoginInfo map[string]string         `json:"login_info,omitempty"`
	Cookies   map[string][]*http.Cookie `json:"cookies,omitempty"`
	LoggedIn  bool                      `json:"logged_in,omitempty"`
	Books     map[string]map[string]any `json:"books,omitempty"`
	jar       http.CookieJar
	revoked   bool
}

var errSessionRevoked = errors.New("书源会话已注销，请重新操作")

type sessionLeaseKey struct{}
type sessionLease struct {
	sourceURL string
	session   *sourceSession
}
type sessionTransport struct {
	parent http.RoundTripper
	active func() bool
}

func (t sessionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.active() {
		return nil, errSessionRevoked
	}
	return t.parent.RoundTrip(req)
}

type clientSessions struct {
	mu       sync.Mutex
	DeviceID string                    `json:"device_id"`
	Sources  map[string]*sourceSession `json:"sources"`
}

func (c *Client) sessionState() *clientSessions {
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	if c.sessions == nil {
		data := make([]byte, 16)
		if _, err := rand.Read(data); err != nil {
			// The identifier is an opaque per-installation value, not a secret.
			copy(data, fmt.Sprint(time.Now().UnixNano()))
		}
		c.sessions = &clientSessions{DeviceID: hex.EncodeToString(data), Sources: make(map[string]*sourceSession)}
	}
	return c.sessions
}

func (s *clientSessions) source(key string) *sourceSession {
	v := s.Sources[key]
	if v == nil {
		v = &sourceSession{}
		s.Sources[key] = v
	}
	if v.Cookies == nil {
		v.Cookies = make(map[string][]*http.Cookie)
	}
	if v.LoginInfo == nil {
		v.LoginInfo = make(map[string]string)
	}
	if v.Books == nil {
		v.Books = make(map[string]map[string]any)
	}
	if v.jar == nil {
		v.jar, _ = cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
		for origin, cookies := range v.Cookies {
			if u, err := url.Parse(origin); err == nil {
				v.jar.SetCookies(u, cookies)
			}
		}
	}
	return v
}

// A website cookie belongs to the importing source. Sharing a global jar
// would let two source definitions overwrite or inspect each other's login.
func (c *Client) httpClientForSource(src Source, contexts ...context.Context) *http.Client {
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	cloned := *client
	state := c.sessionState()
	state.mu.Lock()
	var bound *sourceSession
	if len(contexts) > 0 {
		if lease, ok := contexts[0].Value(sessionLeaseKey{}).(sessionLease); ok && lease.sourceURL == src.URL {
			bound = lease.session
		}
	}
	if bound == nil {
		bound = state.source(src.URL)
	}
	cloned.Jar = bound.jar
	state.mu.Unlock()
	active := func() bool {
		state.mu.Lock()
		defer state.mu.Unlock()
		return !bound.revoked && state.Sources[src.URL] == bound
	}
	transport := cloned.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	cloned.Transport = sessionTransport{parent: transport, active: active}
	redirectCheck := cloned.CheckRedirect
	cloned.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.Response != nil && req.Response.Request != nil {
			c.recordSessionCookies(src, req.Response.Request.URL.String(), req.Response.Cookies(), cloned.Jar)
		}
		state.mu.Lock()
		current := state.Sources[src.URL]
		valid := current != nil && !current.revoked && current.jar == cloned.Jar
		state.mu.Unlock()
		if !valid {
			return errSessionRevoked
		}
		if redirectCheck != nil {
			return redirectCheck(req, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}
	return &cloned
}

var profileHosts = regexp.MustCompile(`(?s)\b(?:var|let|const)\s+host\s*=\s*\[([^\]]+)\]`)
var quotedHTTP = regexp.MustCompile(`["'](https?://[^"']+)["']`)

func knownLoginHosts(src Source) []string {
	if !strings.Contains(src.URL+src.Name, "大灰狼") || !strings.Contains(src.LoginURL, "/login_api") {
		return nil
	}
	match := profileHosts.FindStringSubmatch(src.JSLib)
	if len(match) < 2 {
		return nil
	}
	var hosts []string
	for _, value := range quotedHTTP.FindAllStringSubmatch(match[1], -1) {
		parsed, err := url.Parse(value[1])
		if err == nil && parsed.Host != "" && parsed.User == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") {
			hosts = append(hosts, strings.TrimRight(parsed.Scheme+"://"+parsed.Host, "/"))
		}
	}
	return hosts
}

func LoginFields(src Source) []LoginField {
	if len(knownLoginHosts(src)) > 0 {
		return []LoginField{{Name: "邮箱"}, {Name: "密码", Secret: true}}
	}
	return []LoginField{{Name: "Cookie", Secret: true}}
}
func LoginUIFields(src Source) []LoginField { return LoginFields(src) }

// Login supports the declared 大灰狼 API profile and manual website cookies.
// Credentials are scoped to one explicitly declared API origin, are never
// passed to source JavaScript and are never written to the session file.
func (c *Client) Login(ctx context.Context, src Source, fields map[string]string) error {
	state := c.sessionState()
	state.mu.Lock()
	initial := state.source(src.URL)
	initialJar := initial.jar
	state.mu.Unlock()
	hosts := knownLoginHosts(src)
	if len(hosts) == 0 {
		value := strings.TrimSpace(fields["Cookie"])
		if value == "" {
			return fmt.Errorf("请填写网站 Cookie；此书源的脚本登录尚未适配")
		}
		origin, err := cookieOrigin(src.URL)
		if err != nil {
			return fmt.Errorf("此书源没有可设置 Cookie 的 HTTP(S) 地址")
		}
		if err := c.setSessionCookie(src, origin, value, initialJar); err != nil {
			return err
		}
		state.mu.Lock()
		if initial.revoked || state.Sources[src.URL] != initial {
			state.mu.Unlock()
			return errSessionRevoked
		}
		initial.LoggedIn = true
		state.mu.Unlock()
		return nil
	}
	if strings.TrimSpace(fields["邮箱"]) == "" || fields["密码"] == "" {
		return fmt.Errorf("请填写邮箱和密码")
	}
	origin := hosts[0]
	state.mu.Lock()
	var settings map[string]any
	_ = json.Unmarshal([]byte(state.source(src.URL).Variable), &settings)
	if configured, ok := settings["server"].(string); ok {
		for _, host := range hosts {
			if configured == host {
				origin = host
				break
			}
		}
	}
	state.mu.Unlock()
	if !strings.HasPrefix(origin, "https://") {
		return fmt.Errorf("账号密码登录需要书源声明的 HTTPS API 地址")
	}
	body, err := json.Marshal(map[string]string{"register_email": strings.TrimSpace(fields["邮箱"]), "password": fields["密码"]})
	if err != nil {
		return fmt.Errorf("构造登录请求失败")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+"/login_api", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("构造登录请求失败")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ReadCLI/1.0")
	client := c.httpClientForSource(src)
	secureClient := *client
	secureClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := secureClient.Do(req)
	if err != nil {
		return fmt.Errorf("登录接口请求失败: %w", err)
	}
	defer response.Body.Close()
	c.recordSessionCookies(src, origin+"/login_api", response.Cookies(), client.Jar)
	if err := ctx.Err(); err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("登录接口 HTTP %d", response.StatusCode)
	}
	data, err := readBounded(response.Body, 1<<20)
	if err != nil {
		return fmt.Errorf("读取登录响应失败")
	}
	var result struct {
		Code *int   `json:"code"`
		Key  string `json:"key"`
	}
	if json.Unmarshal(data, &result) != nil {
		return fmt.Errorf("登录接口没有返回有效 JSON")
	}
	if result.Code == nil || *result.Code != 0 || result.Key == "" {
		// Do not echo arbitrary server errors: they may contain submitted values.
		return fmt.Errorf("登录失败，请核对账号密码或书源账户状态")
	}
	state.mu.Lock()
	if initial.revoked || state.Sources[src.URL] != initial {
		state.mu.Unlock()
		return errSessionRevoked
	}
	deviceID := state.DeviceID
	session := initial
	if settings == nil {
		settings = map[string]any{}
	}
	settings["server"], settings["qttoken"] = origin, result.Key
	if settings["tab"] == nil {
		settings["tab"] = "小说"
	}
	if settings["sources"] == nil {
		settings["sources"] = "全部"
	}
	// Reader-only features do not need remote paragraph comments or shelf writes.
	settings["fqpara"], settings["fqcommunity"], settings["reading"] = "off", "off", "0"
	encoded, _ := json.Marshal(settings)
	session.Variable = string(encoded)
	session.LoginInfo = map[string]string{"密钥": result.Key}
	session.LoggedIn = true
	state.mu.Unlock()
	for _, host := range hosts {
		if err := c.setSessionCookie(src, host, "qttoken="+result.Key+"; deviceId="+deviceID, initialJar); err != nil {
			return err
		}
	}
	return nil
}

func cookieOrigin(raw string) (string, error) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("无效的 Cookie 地址")
	}
	return u.Scheme + "://" + u.Host, nil
}

func (c *Client) setSessionCookie(src Source, rawURL, value string, expectedJar ...http.CookieJar) error {
	origin, err := cookieOrigin(rawURL)
	if err != nil {
		return err
	}
	req := &http.Request{Header: make(http.Header)}
	req.Header.Set("Cookie", value)
	updates := req.Cookies()
	if value != "" && len(updates) == 0 {
		return fmt.Errorf("Cookie 格式应为 name=value")
	}
	for _, item := range updates {
		item.Path = "/"
	}
	if !c.recordSessionCookies(src, origin, updates, expectedJar...) {
		return errSessionRevoked
	}
	return nil
}

func cookieStorageKey(cookie *http.Cookie) string {
	return cookie.Name + "\x00" + cookie.Domain + "\x00" + cookie.Path
}

// recordSessionCookies retains expiry and path data discarded by Jar.Cookies.
// The HTTP layer calls it for Set-Cookie response headers.
func (c *Client) recordSessionCookies(src Source, rawURL string, updates []*http.Cookie, expectedJar ...http.CookieJar) bool {
	if len(updates) == 0 {
		return true
	}
	origin, err := cookieOrigin(rawURL)
	if err != nil {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		u, _ = url.Parse(origin)
	}
	state := c.sessionState()
	state.mu.Lock()
	defer state.mu.Unlock()
	session := state.Sources[src.URL]
	if len(expectedJar) > 0 && (session == nil || session.revoked || session.jar != expectedJar[0]) {
		return false
	}
	if session == nil {
		session = state.source(src.URL)
	}
	byKey := make(map[string]*http.Cookie)
	for _, item := range session.Cookies[origin] {
		if item != nil {
			byKey[cookieStorageKey(item)] = item
		}
	}
	for _, item := range updates {
		if item == nil {
			continue
		}
		copyCookie := *item
		if copyCookie.Path == "" {
			copyCookie.Path = "/"
			if index := strings.LastIndex(u.Path, "/"); index > 0 {
				copyCookie.Path = u.Path[:index]
			}
		}
		if copyCookie.MaxAge > 0 {
			copyCookie.Expires = time.Now().Add(time.Duration(copyCookie.MaxAge) * time.Second)
			copyCookie.MaxAge = 0
		}
		key := cookieStorageKey(&copyCookie)
		if copyCookie.MaxAge < 0 || (!copyCookie.Expires.IsZero() && copyCookie.Expires.Before(time.Now())) {
			delete(byKey, key)
		} else {
			byKey[key] = &copyCookie
		}
	}
	var cookies []*http.Cookie
	for _, item := range byKey {
		cookies = append(cookies, item)
	}
	sort.Slice(cookies, func(i, j int) bool { return cookieStorageKey(cookies[i]) < cookieStorageKey(cookies[j]) })
	session.Cookies[origin] = cookies
	session.jar.SetCookies(u, updates)
	return true
}

func (c *Client) sessionCookie(src Source, rawURL string, expectedJar ...http.CookieJar) string {
	origin, err := cookieOrigin(rawURL)
	if err != nil {
		return ""
	}
	state := c.sessionState()
	state.mu.Lock()
	session := state.Sources[src.URL]
	if len(expectedJar) > 0 && (session == nil || session.revoked || session.jar != expectedJar[0]) {
		state.mu.Unlock()
		return ""
	}
	if session == nil {
		session = state.source(src.URL)
	}
	jar := session.jar
	state.mu.Unlock()
	u, _ := url.Parse(rawURL)
	if u == nil || u.Host == "" {
		u, _ = url.Parse(origin)
	}
	cookies := jar.Cookies(u)
	parts := make([]string, 0, len(cookies))
	for _, item := range cookies {
		if item.MaxAge >= 0 {
			parts = append(parts, item.Name+"="+item.Value)
		}
	}
	return strings.Join(parts, "; ")
}

func (c *Client) removeSessionCookie(src Source, rawURL string, expectedJar ...http.CookieJar) {
	origin, err := cookieOrigin(rawURL)
	if err != nil {
		return
	}
	state := c.sessionState()
	state.mu.Lock()
	session := state.Sources[src.URL]
	if len(expectedJar) > 0 && (session == nil || session.revoked || session.jar != expectedJar[0]) {
		state.mu.Unlock()
		return
	}
	if session == nil {
		state.mu.Unlock()
		return
	}
	delete(session.Cookies, origin)
	// Rebuild from the other origins so all paths for this origin are removed.
	session.jar = nil
	state.source(src.URL)
	state.mu.Unlock()
}

func (c *Client) Logout(src Source) error {
	state := c.sessionState()
	state.mu.Lock()
	if session := state.Sources[src.URL]; session != nil {
		session.revoked = true
	}
	delete(state.Sources, src.URL)
	state.mu.Unlock()
	return nil
}
func (c *Client) IsLoggedIn(src Source) bool {
	state := c.sessionState()
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.source(src.URL).LoggedIn
}
func (c *Client) LoginStatus(src Source) bool { return c.IsLoggedIn(src) }

func sessionCredentialKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "password") || strings.Contains(lower, "passwd") || strings.Contains(lower, "密码") || lower == "邮箱" || lower == "register_email"
}

func scrubSessionValue(value any) any {
	switch data := value.(type) {
	case map[string]any:
		out := make(map[string]any)
		for key, item := range data {
			if sessionCredentialKey(key) {
				continue
			}
			out[key] = scrubSessionValue(item)
		}
		return out
	case map[string]string:
		out := make(map[string]string)
		for key, item := range data {
			if !sessionCredentialKey(key) {
				out[key] = item
			}
		}
		return out
	case []any:
		out := make([]any, len(data))
		for i, item := range data {
			out[i] = scrubSessionValue(item)
		}
		return out
	case float64:
		if math.IsNaN(data) || math.IsInf(data, 0) {
			return nil
		}
	case float32:
		if math.IsNaN(float64(data)) || math.IsInf(float64(data), 0) {
			return nil
		}
	}
	return value
}

func scrubSourceVariable(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') || !json.Valid([]byte(trimmed)) {
		return value
	}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	var variable any
	if err := decoder.Decode(&variable); err != nil {
		return value
	}
	encoded, err := json.Marshal(scrubSessionValue(variable))
	if err != nil {
		return value
	}
	return string(encoded)
}

func (c *Client) SaveSessions(dir string) error {
	// Serialize snapshot creation and replacement so a delayed save cannot
	// overwrite a newer save made after logout.
	c.sessionSaveMu.Lock()
	defer c.sessionSaveMu.Unlock()
	state := c.sessionState()
	state.mu.Lock()
	copyState := &clientSessions{DeviceID: state.DeviceID, Sources: make(map[string]*sourceSession)}
	for key, session := range state.Sources {
		if session == nil || session.revoked {
			continue
		}
		copySession := &sourceSession{
			Variable:  scrubSourceVariable(session.Variable),
			LoginInfo: scrubSessionValue(session.LoginInfo).(map[string]string),
			LoggedIn:  session.LoggedIn,
			Cookies:   make(map[string][]*http.Cookie),
			Books:     make(map[string]map[string]any),
		}
		for bookURL, variables := range session.Books {
			copySession.Books[bookURL] = scrubSessionValue(variables).(map[string]any)
		}
		for origin, cookies := range session.Cookies {
			for _, item := range cookies {
				if item != nil && item.MaxAge >= 0 && (item.Expires.IsZero() || item.Expires.After(time.Now())) {
					copySession.Cookies[origin] = append(copySession.Cookies[origin], item)
				}
			}
		}
		copyState.Sources[key] = copySession
	}
	data, err := json.MarshalIndent(copyState, "", "  ")
	state.mu.Unlock()
	if err != nil {
		return fmt.Errorf("保存会话失败")
	}
	if int64(len(data)) > sessionFileLimit {
		return fmt.Errorf("会话数据超过 %d 字节限制", sessionFileLimit)
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".sessions-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(dir, "sessions.json"))
}

func (c *Client) LoadSessions(dir string) error {
	path := filepath.Join(dir, "sessions.json")
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := readBounded(file, sessionFileLimit)
	if err != nil {
		return err
	}
	var loaded clientSessions
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("会话文件格式无效")
	}
	if loaded.Sources == nil {
		loaded.Sources = make(map[string]*sourceSession)
	}
	if loaded.DeviceID == "" {
		loaded.DeviceID = c.sessionState().DeviceID
	}
	c.sessionMu.Lock()
	if old := c.sessions; old != nil {
		old.mu.Lock()
		for _, session := range old.Sources {
			if session != nil {
				session.revoked = true
			}
		}
		old.mu.Unlock()
	}
	c.sessions = &loaded
	c.sessionMu.Unlock()
	for key := range loaded.Sources {
		loaded.source(key)
	}
	return os.Chmod(path, 0600)
}

type ajaxDepthKey struct{}

func (c *Client) evaluator(ctx context.Context, src Source, base string, vars map[string]any) *Evaluator {
	localVars := make(map[string]any, len(vars)+1)
	for key, value := range vars {
		localVars[key] = value
	}
	if _, exists := localVars["key"]; !exists {
		localVars["key"] = ""
	}
	vars = localVars
	e := baseEvaluator(src, base, vars).SetJSTimeout(40 * time.Second).SetJSContext(ctx)
	stageDeadline := time.Now().Add(40 * time.Second)
	state := c.sessionState()
	state.mu.Lock()
	var bound *sourceSession
	if lease, ok := ctx.Value(sessionLeaseKey{}).(sessionLease); ok && lease.sourceURL == src.URL {
		bound = lease.session
	}
	if bound == nil {
		bound = state.source(src.URL)
	}
	state.mu.Unlock()
	ctx = context.WithValue(ctx, sessionLeaseKey{}, sessionLease{sourceURL: src.URL, session: bound})
	bindingJar := func() (http.CookieJar, error) {
		state.mu.Lock()
		defer state.mu.Unlock()
		if bound.revoked || state.Sources[src.URL] != bound {
			return nil, errSessionRevoked
		}
		return bound.jar, nil
	}
	getVariable := func() string {
		state.mu.Lock()
		defer state.mu.Unlock()
		if bound.revoked || state.Sources[src.URL] != bound {
			return ""
		}
		return bound.Variable
	}
	setVariable := func(value string) {
		state.mu.Lock()
		if !bound.revoked && state.Sources[src.URL] == bound {
			bound.Variable = value
		}
		state.mu.Unlock()
	}
	getLoginInfo := func() map[string]string {
		state.mu.Lock()
		defer state.mu.Unlock()
		out := make(map[string]string)
		if bound.revoked || state.Sources[src.URL] != bound {
			return out
		}
		for key, value := range bound.LoginInfo {
			out[key] = value
		}
		return out
	}
	source := map[string]any{
		"bookSourceUrl": src.URL, "bookSourceName": src.Name,
		"getKey": func() string { return src.URL }, "getTag": func() string { return src.Name },
		"getVariable": getVariable, "setVariable": setVariable,
		"getLoginInfoMap": getLoginInfo,
		"getLoginInfo":    func() string { data, _ := json.Marshal(getLoginInfo()); return string(data) },
		"putLoginInfo": func(value string) bool {
			var info map[string]any
			if json.Unmarshal([]byte(value), &info) != nil {
				return false
			}
			filtered := scrubSessionValue(info).(map[string]any)
			state.mu.Lock()
			if bound.revoked || state.Sources[src.URL] != bound {
				state.mu.Unlock()
				return false
			}
			target := bound
			target.LoginInfo = make(map[string]string)
			for key, value := range filtered {
				if text, ok := value.(string); ok {
					target.LoginInfo[key] = text
				}
			}
			state.mu.Unlock()
			return true
		},
	}
	getCookie := func(raw string) string {
		jar, err := bindingJar()
		if err != nil {
			return ""
		}
		return c.sessionCookie(src, raw, jar)
	}
	cookie := map[string]any{
		"getCookie": getCookie,
		"setCookie": func(raw, value string) error {
			jar, err := bindingJar()
			if err != nil {
				return err
			}
			return c.setSessionCookie(src, raw, value, jar)
		},
		"removeCookie": func(raw string) {
			jar, err := bindingJar()
			if err == nil {
				c.removeSessionCookie(src, raw, jar)
			}
		},
		"getKey": func(raw, name string) string {
			req := &http.Request{Header: make(http.Header)}
			req.Header.Set("Cookie", getCookie(raw))
			value, err := req.Cookie(name)
			if err != nil {
				return ""
			}
			return value.Value
		},
	}
	book := map[string]any{}
	if value, ok := vars["book"].(map[string]any); ok {
		for key, item := range value {
			book[key] = item
		}
	}
	bookKey, _ := book["bookUrl"].(string)
	bookGet := func(key string) any {
		state.mu.Lock()
		defer state.mu.Unlock()
		if bound.revoked || state.Sources[src.URL] != bound {
			return nil
		}
		return jsData(bound.Books[bookKey][key])
	}
	bookPut := func(key string, value any) any {
		value = jsData(value)
		state.mu.Lock()
		defer state.mu.Unlock()
		if bound.revoked || state.Sources[src.URL] != bound {
			return nil
		}
		session := bound
		if session.Books[bookKey] == nil {
			session.Books[bookKey] = make(map[string]any)
		}
		session.Books[bookKey][key] = value
		return jsData(value)
	}
	book["getVariable"], book["putVariable"] = bookGet, bookPut
	book["getVariableMap"] = func() map[string]any {
		state.mu.Lock()
		defer state.mu.Unlock()
		out := make(map[string]any)
		if bound.revoked || state.Sources[src.URL] != bound {
			return out
		}
		for key, value := range bound.Books[bookKey] {
			out[key] = jsData(value)
		}
		return out
	}
	book["getName"] = func() any { return book["name"] }
	book["getAuthor"] = func() any { return book["author"] }
	book["getBookUrl"] = func() any { return book["bookUrl"] }
	book["readConfig"] = map[string]any{"useReplaceRule": false}
	book["setUseReplaceRule"] = func(bool) {}
	state.mu.Lock()
	device := state.DeviceID
	state.mu.Unlock()
	java := map[string]any{
		"put": func(key string, value any) any {
			value = jsData(value)
			e.mu.Lock()
			e.variables[key] = value
			e.mu.Unlock()
			if bookKey != "" {
				bookPut(key, value)
			}
			return jsData(value)
		},
		"get": func(key string) any {
			e.mu.RLock()
			value, exists := e.variables[key]
			e.mu.RUnlock()
			if exists {
				return jsData(value)
			}
			if bookKey != "" {
				return bookGet(key)
			}
			return ""
		},
		"ajax": func(raw string) (string, error) {
			if _, err := bindingJar(); err != nil {
				return "", err
			}
			depth, _ := ctx.Value(ajaxDepthKey{}).(int)
			if depth >= 8 {
				return "", fmt.Errorf("JS 网络请求嵌套超过 8 层")
			}
			deadline := time.Now().Add(30 * time.Second)
			if stageDeadline.Before(deadline) {
				deadline = stageDeadline
			}
			nested, cancel := context.WithDeadline(context.WithValue(ctx, ajaxDepthKey{}, depth+1), deadline)
			defer cancel()
			body, _, err := c.fetch(nested, src, base, raw, vars, 0)
			return body, err
		},
		"base64Encode": func(value string) string { return base64.StdEncoding.EncodeToString([]byte(value)) },
		"base64Decode": func(value string) (string, error) {
			data, err := base64.StdEncoding.DecodeString(value)
			if err != nil {
				data, err = base64.RawStdEncoding.DecodeString(value)
			}
			return string(data), err
		},
		"hexDecodeToString": func(value string) (string, error) { data, err := hex.DecodeString(value); return string(data), err },
		"deviceID":          func() string { return device }, "getDeviceID": func() string { return device }, "androidId": func() string { return device },
		"getWebViewUA": func() string { return "Mozilla/5.0 ReadCLI/1.0" },
		"toast":        func(any) {}, "longToast": func(any) {}, "log": func(any) {},
		"getCookie": getCookie,
	}
	e.SetJSBindings(map[string]any{"source": source, "cookie": cookie, "book": book, "java": java})
	return e
}
