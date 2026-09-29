package booksource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type sessionRoundTripper func(*http.Request) (*http.Response, error)

func (f sessionRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func sessionResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Status: fmt.Sprint(status), Header: http.Header{"Content-Type": {"application/json; charset=utf-8"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}
}

func sessionFixture() Source {
	return Source{URL: "大灰狼测试", Name: "大灰狼测试", LoginURL: "function login(){java.ajax('/login_api')}", JSLib: `var host = ['https://login.example.test','https://backup.example.test'];`}
}

func TestSessionCanceledFetchCannotRestoreLoggedOutCookie(t *testing.T) {
	c := NewClient()
	src := Source{URL: "https://same.example.test", Name: "A"}
	_ = c.setSessionCookie(src, src.URL, "qttoken=old-token")
	started, release := make(chan struct{}), make(chan struct{})
	c.HTTPClient.Transport = sessionRoundTripper(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-release
		response := sessionResponse(req, 200, "ok")
		response.Header.Set("Set-Cookie", "qttoken=late-token; Path=/")
		return response, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, _, err := c.fetch(ctx, src, src.URL, src.URL, nil, 0); done <- err }()
	<-started
	cancel()
	_ = c.Logout(src)
	close(release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("request error %v", err)
	}
	if cookie := c.sessionCookie(src, src.URL); cookie != "" {
		t.Fatal("canceled old request restored cookies after logout")
	}
}

func TestSessionOldScriptCannotMutateNewSession(t *testing.T) {
	c := NewClient()
	src := Source{URL: "https://same.example.test", Name: "A"}
	e := c.evaluator(context.Background(), src, src.URL, nil)
	if err := c.Logout(src); err != nil {
		t.Fatal(err)
	}
	if err := c.Login(context.Background(), src, map[string]string{"Cookie": "session=new"}); err != nil {
		t.Fatal(err)
	}
	_, _ = e.EvalJS(`source.setVariable('{"server":"stale"}');cookie.setCookie(baseUrl,'session=stale');source.putLoginInfo('{"密钥":"stale"}')`, nil)
	if c.sessionCookie(src, src.URL) != "session=new" {
		t.Fatal("old script overwrote new session cookie")
	}
	state := c.sessionState()
	state.mu.Lock()
	variable := state.source(src.URL).Variable
	info := state.source(src.URL).LoginInfo
	state.mu.Unlock()
	if variable != "" || len(info) != 0 {
		t.Fatal("old script mutated new session state")
	}
	value, err := e.EvalJS(`cookie.getCookie(baseUrl)`, nil)
	if err != nil || value != "" {
		t.Fatal("old script read new session cookie")
	}
	_, _ = e.EvalJS(`source.putLoginInfo('{"密钥":"stale"}')`, nil)
	state.mu.Lock()
	infoCount := len(state.source(src.URL).LoginInfo)
	state.mu.Unlock()
	if infoCount != 0 {
		t.Fatal("old script restored login info")
	}
}

func TestSessionRevokedRequestLeaseCannotUseNewSession(t *testing.T) {
	c := NewClient()
	src := Source{URL: "https://same.example.test"}
	state := c.sessionState()
	state.mu.Lock()
	old := state.source(src.URL)
	state.mu.Unlock()
	ctx := context.WithValue(context.Background(), sessionLeaseKey{}, sessionLease{sourceURL: src.URL, session: old})
	_ = c.Logout(src)
	if err := c.Login(context.Background(), src, map[string]string{"Cookie": "session=new"}); err != nil {
		t.Fatal(err)
	}
	c.HTTPClient.Transport = sessionRoundTripper(func(*http.Request) (*http.Response, error) {
		t.Fatal("revoked request reached network")
		return nil, nil
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if _, err := c.httpClientForSource(src, ctx).Do(req); !errors.Is(err, errSessionRevoked) {
		t.Fatalf("stale lease accepted: %v", err)
	}
}

func TestSessionLateLoginCannotUndoLogout(t *testing.T) {
	c := NewClient()
	src := sessionFixture()
	started, release := make(chan struct{}), make(chan struct{})
	c.HTTPClient.Transport = sessionRoundTripper(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-release
		return sessionResponse(req, 200, `{"code":0,"key":"late-token"}`), nil
	})
	done := make(chan error, 1)
	go func() {
		done <- c.Login(context.Background(), src, map[string]string{"邮箱": "offline@example.test", "密码": "fixture"})
	}()
	<-started
	_ = c.Logout(src)
	close(release)
	if err := <-done; !errors.Is(err, errSessionRevoked) {
		t.Fatalf("late login error: %v", err)
	}
	if c.IsLoggedIn(src) || c.sessionCookie(src, "https://login.example.test") != "" {
		t.Fatal("late login restored logged out session")
	}
}

func TestSessionLoginPersistenceAndLogout(t *testing.T) {
	src := sessionFixture()
	c := NewClient()
	requests := 0
	c.HTTPClient.Transport = sessionRoundTripper(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.URL.String() != "https://login.example.test/login_api" || req.Method != "POST" {
			t.Fatalf("unexpected login destination %s %s", req.Method, req.URL)
		}
		var body map[string]string
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["register_email"] != "fixture@example.test" || body["password"] != "password-sentinel" {
			t.Fatal("incorrect login request")
		}
		return sessionResponse(req, 200, `{"code":0,"key":"fixture-token"}`), nil
	})
	if err := c.Login(context.Background(), src, map[string]string{"邮箱": "fixture@example.test", "密码": "password-sentinel"}); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || !c.IsLoggedIn(src) {
		t.Fatal("login did not establish one session")
	}
	for _, origin := range knownLoginHosts(src) {
		if cookie := c.sessionCookie(src, origin); !strings.Contains(cookie, "qttoken=fixture-token") || !strings.Contains(cookie, "deviceId=") {
			t.Fatal("missing shared token/device cookie")
		}
	}
	dir := t.TempDir()
	if err := c.SaveSessions(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "sessions.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "password-sentinel") || strings.Contains(string(data), "fixture@example.test") {
		t.Fatal("credentials persisted")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions %o", info.Mode().Perm())
	}
	restored := NewClient()
	if err := restored.LoadSessions(dir); err != nil {
		t.Fatal(err)
	}
	if !restored.IsLoggedIn(src) || !strings.Contains(restored.sessionCookie(src, "https://login.example.test"), "fixture-token") {
		t.Fatal("session not restored")
	}
	if c.sessionState().DeviceID != restored.sessionState().DeviceID {
		t.Fatal("device identity changed")
	}
	if err := restored.Logout(src); err != nil {
		t.Fatal(err)
	}
	if restored.IsLoggedIn(src) || restored.sessionCookie(src, "https://login.example.test") != "" || restored.sessionCookie(src, "https://backup.example.test") != "" {
		t.Fatal("logout retained cookies")
	}
	if err := restored.SaveSessions(dir); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), "fixture-token") {
		t.Fatal("logout retained saved token")
	}
}

func TestSessionRejectsLoginRedirectAndPlaintextOrigin(t *testing.T) {
	c := NewClient()
	src := sessionFixture()
	calls := 0
	c.HTTPClient.Transport = sessionRoundTripper(func(req *http.Request) (*http.Response, error) {
		calls++
		response := sessionResponse(req, 307, "")
		response.Header.Set("Location", "https://other.example.test/steal")
		return response, nil
	})
	if err := c.Login(context.Background(), src, map[string]string{"邮箱": "a", "密码": "b"}); err == nil {
		t.Fatal("redirect accepted")
	}
	if calls != 1 {
		t.Fatal("credentials redirected")
	}
	src.JSLib = `var host = ['http://insecure.example.test'];`
	if err := c.Login(context.Background(), src, map[string]string{"邮箱": "a", "密码": "b"}); err == nil {
		t.Fatal("plaintext login accepted")
	}
	if calls != 1 {
		t.Fatal("credentials sent to insecure origin")
	}
}

func TestSessionNativeBindingsAndSecretFiltering(t *testing.T) {
	c := NewClient()
	src := Source{URL: "https://fixture.example.test", Name: "fixture"}
	e := c.evaluator(context.Background(), src, src.URL, bookVars(Book{URL: "book-1", Name: "书籍"}))
	result, err := e.EvalJS(`source.setVariable(JSON.stringify({server:'https://fixture.example.test',password:'never-store'}));
source.putLoginInfo(JSON.stringify({'密码':'also-never-store','密钥':'token'}));
book.putVariable('custom','voice');cookie.setCookie(baseUrl,'session=fixture');
[book.getName(),book.getVariable('custom'),cookie.getKey(baseUrl,'session'),java.hexDecodeToString('e4b8ad'),java.base64Decode(java.base64Encode('中文'))].join('|')`, nil)
	if err != nil || result != "书籍|voice|fixture|中|中文" {
		t.Fatalf("native result %v: %v", result, err)
	}
	dir := t.TempDir()
	if err := c.SaveSessions(dir); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "sessions.json"))
	if strings.Contains(string(data), "never-store") {
		t.Fatal("password persisted through script binding")
	}
	next := c.evaluator(context.Background(), src, src.URL, bookVars(Book{URL: "book-1"}))
	result, err = next.EvalJS(`book.getVariable('custom')`, nil)
	if err != nil || result != "voice" {
		t.Fatalf("book variable lost: %v %v", result, err)
	}
	if _, err := next.EvalJS(`java.put('persistentBookKey','value')`, nil); err != nil {
		t.Fatal(err)
	}
	third := c.evaluator(context.Background(), src, src.URL, bookVars(Book{URL: "book-1"}))
	result, err = third.EvalJS(`java.get('persistentBookKey')`, nil)
	if err != nil || result != "value" {
		t.Fatalf("java book variable lost: %v %v", result, err)
	}
}

func TestSessionScriptStateSurvivesReload(t *testing.T) {
	c := NewClient()
	a := Source{URL: "https://source-a.example.test"}
	b := Source{URL: "https://source-b.example.test"}
	loggedOut := Source{URL: "https://logged-out.example.test"}
	eval := func(t *testing.T, client *Client, src Source, bookURL, code string) any {
		t.Helper()
		e := client.evaluator(context.Background(), src, src.URL, bookVars(Book{URL: bookURL}))
		value, err := e.EvalJS(code, nil)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	if err := c.Login(context.Background(), a, map[string]string{"Cookie": "session=fixture-a"}); err != nil {
		t.Fatal(err)
	}
	eval(t, c, a, "shared-book", `source.setVariable('raw-source-token');
source.putLoginInfo(JSON.stringify({'密钥':'login-token',password:'secret-login-password',register_email:'secret-login-account'}));
book.putVariable('token','book-a-token');
book.putVariable('details',[{name:'kept',password:'secret-book-password',nested:[{register_email:'secret-book-account',value:'nested-value'}]}]);
java.put('java-token','java-a-token');`)
	eval(t, c, a, "other-book", `book.putVariable('token','other-book-token');`)
	eval(t, c, b, "shared-book", `source.setVariable(JSON.stringify([{server:'server-b',password:'secret-source-password',nested:[{register_email:'secret-source-account',value:'kept'}]}]));
source.putLoginInfo(JSON.stringify({'密钥':'login-b-token'}));book.putVariable('token','book-b-token');`)
	eval(t, c, loggedOut, "shared-book", `source.setVariable('revoked-source-token');source.putLoginInfo('{"密钥":"revoked-login-token"}');book.putVariable('token','revoked-book-token');`)
	stale := c.evaluator(context.Background(), loggedOut, loggedOut.URL, bookVars(Book{URL: "shared-book"}))
	if err := c.Logout(loggedOut); err != nil {
		t.Fatal(err)
	}
	if _, err := stale.EvalJS(`source.setVariable('late-source-token');source.putLoginInfo('{"密钥":"late-login-token"}');book.putVariable('token','late-book-token');`, nil); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := c.SaveSessions(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"secret-", "revoked-", "late-", loggedOut.URL} {
		if strings.Contains(string(data), secret) {
			t.Errorf("saved session retained %q", secret)
		}
	}
	if value := eval(t, c, a, "shared-book", `book.getVariable('details')[0].password`); value != "secret-book-password" {
		t.Errorf("saving mutated runtime book variables: %v", value)
	}
	restored := NewClient()
	if err := restored.LoadSessions(dir); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, bookURL, code, want string
		source                    Source
	}{
		{"raw source variable", "shared-book", `source.getVariable()`, "raw-source-token", a},
		{"login info", "shared-book", `source.getLoginInfoMap()['密钥']`, "login-token", a},
		{"book variable", "shared-book", `book.getVariable('token')`, "book-a-token", a},
		{"java variable", "shared-book", `java.get('java-token')`, "java-a-token", a},
		{"book isolation", "other-book", `book.getVariable('token')`, "other-book-token", a},
		{"source isolation", "shared-book", `book.getVariable('token')`, "book-b-token", b},
		{"source login isolation", "shared-book", `source.getLoginInfoMap()['密钥']`, "login-b-token", b},
		{"book secret filtering", "shared-book", `let details=book.getVariable('details')[0];[details.name,details.nested[0].value,typeof details.password,typeof details.nested[0].register_email].join('|')`, "kept|nested-value|undefined|undefined", a},
		{"source secret filtering", "shared-book", `source.getVariable()`, `[{"nested":[{"value":"kept"}],"server":"server-b"}]`, b},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if value := eval(t, restored, tc.source, tc.bookURL, tc.code); value != tc.want {
				t.Errorf("restored %v, want %s", value, tc.want)
			}
		})
	}
	if !restored.IsLoggedIn(a) || restored.sessionCookie(a, a.URL) != "session=fixture-a" {
		t.Error("restored session lost login or cookie")
	}
	if restored.IsLoggedIn(loggedOut) || eval(t, restored, loggedOut, "shared-book", `source.getVariable() + source.getLoginInfo() + JSON.stringify(book.getVariableMap())`) != "{}{}" {
		t.Error("logged-out source state was restored")
	}
}

func TestScrubSessionValueDoesNotMutateArrays(t *testing.T) {
	value := []any{map[string]any{"password": "runtime-secret", "nested": []any{map[string]any{"邮箱": "runtime-account", "value": "kept"}}}}
	before, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	filtered, err := json.Marshal(scrubSessionValue(value))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(filtered), "runtime-") {
		t.Fatal("nested secrets were not removed")
	}
	after, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("secret filtering mutated the original array")
	}
}

func TestSessionLegacyStateLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	if err := os.WriteFile(path, []byte(`{"device_id":"legacy-device","sources":{"legacy":{"variable":"{\"server\":\"legacy-server\"}","login_info":{"密钥":"legacy-token","password":"legacy-secret"},"logged_in":true}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	c := NewClient()
	if err := c.LoadSessions(dir); err != nil {
		t.Fatal(err)
	}
	src := Source{URL: "legacy"}
	e := c.evaluator(context.Background(), src, "https://fixture.example.test", bookVars(Book{URL: "legacy-book"}))
	value, err := e.EvalJS(`book.putVariable('token','new-token');[JSON.parse(source.getVariable()).server,book.getVariable('token'),source.getLoginInfoMap()['密钥']].join('|')`, nil)
	if err != nil || value != "legacy-server|new-token|legacy-token" || !c.IsLoggedIn(src) || c.sessionState().DeviceID != "legacy-device" {
		t.Fatalf("legacy session did not load: %v %v", value, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("legacy session permissions were not restricted: %v %v", info, err)
	}
	if err := c.SaveSessions(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), "legacy-secret") || !strings.Contains(string(data), "legacy-token") {
		t.Fatalf("saving legacy login info failed to retain only session values: %v", err)
	}
}

func TestSessionSaveSizeLimitPreservesPriorFile(t *testing.T) {
	const maxSessionBytes = 8 << 20
	dir := t.TempDir()
	c := NewClient()
	state := c.sessionState()
	state.mu.Lock()
	state.source("large-source").Variable = "x"
	state.mu.Unlock()
	if err := c.SaveSessions(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "sessions.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	state.source("large-source").Variable = strings.Repeat("x", maxSessionBytes-len(before)+1)
	state.mu.Unlock()
	if err := c.SaveSessions(dir); err != nil {
		t.Fatalf("session at the byte limit was rejected: %v", err)
	}
	before, err = os.ReadFile(path)
	if err != nil || len(before) != maxSessionBytes {
		t.Fatalf("boundary session has %d bytes: %v", len(before), err)
	}
	if err := NewClient().LoadSessions(dir); err != nil {
		t.Fatalf("saved boundary session does not load: %v", err)
	}
	state.mu.Lock()
	state.source("large-source").Variable += "x"
	state.mu.Unlock()
	if err := c.SaveSessions(dir); err == nil {
		t.Error("saved a session larger than the load limit")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("oversized save replaced the previous session")
	}
	if err := NewClient().LoadSessions(dir); err != nil {
		t.Fatalf("previous session no longer loads: %v", err)
	}
}

func TestSessionVariablePreservesScalarTextAndLargeNumbers(t *testing.T) {
	for _, tc := range []struct{ name, value, want string }{
		{"raw string", "raw-token", "raw-token"},
		{"integer", "  9223372036854775807\n", "  9223372036854775807\n"},
		{"exponent", "1.234567890123456789e123", "1.234567890123456789e123"},
		{"huge number", "1e1000", "1e1000"},
		{"JSON string", ` "\u0061" `, ` "\u0061" `},
		{"null", " null\n", " null\n"},
		{"boolean", " true\n", " true\n"},
		{"object", `{"id":9223372036854775807,"password":"secret","nested":{"id":12345678901234567890}}`, `{"id":9223372036854775807,"nested":{"id":12345678901234567890}}`},
		{"array", `[{"id":12345678901234567890,"密码":"secret"},1e1000]`, `[{"id":12345678901234567890},1e1000]`},
		{"trailing data", `{"id":123,"password":"raw"} trailing`, `{"id":123,"password":"raw"} trailing`},
		{"second JSON value", `{"id":123} {"id":456}`, `{"id":123} {"id":456}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient()
			src := Source{URL: "source"}
			e := c.evaluator(context.Background(), src, "https://fixture.example.test", nil)
			literal, _ := json.Marshal(tc.value)
			if _, err := e.EvalJS("source.setVariable("+string(literal)+")", nil); err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if err := c.SaveSessions(dir); err != nil {
				t.Fatal(err)
			}
			restored := NewClient()
			if err := restored.LoadSessions(dir); err != nil {
				t.Fatal(err)
			}
			e = restored.evaluator(context.Background(), src, "https://fixture.example.test", nil)
			if value, err := e.EvalJS(`source.getVariable()`, nil); err != nil || value != tc.want {
				t.Fatalf("restored %q, want %q: %v", value, tc.want, err)
			}
		})
	}
}

func TestSessionNonFiniteBookVariablesDoNotPreventSaving(t *testing.T) {
	c := NewClient()
	src := Source{URL: "https://variables.example.test"}
	other := Source{URL: "https://login.example.test"}
	if err := c.Login(context.Background(), other, map[string]string{"Cookie": "session=normal-token"}); err != nil {
		t.Fatal(err)
	}
	e := c.evaluator(context.Background(), src, src.URL, bookVars(Book{URL: "book"}))
	if _, err := e.EvalJS(`book.putVariable('infinite',Infinity);book.putVariable('invalid',parseInt('missing'));book.putVariable('values',[Infinity,-Infinity,NaN,42]);`, nil); err != nil {
		t.Fatal(err)
	}
	state := c.sessionState()
	state.mu.Lock()
	state.source(src.URL).Books["book"]["float32"] = float32(math.Inf(1))
	state.mu.Unlock()
	dir := t.TempDir()
	if err := c.SaveSessions(dir); err != nil {
		t.Fatal(err)
	}
	if value, err := e.EvalJS(`book.getVariable('infinite') === Infinity && isNaN(book.getVariable('invalid')) && book.getVariable('values')[0] === Infinity`, nil); err != nil || value != true {
		t.Fatalf("saving changed runtime values: %v %v", value, err)
	}
	restored := NewClient()
	if err := restored.LoadSessions(dir); err != nil {
		t.Fatal(err)
	}
	e = restored.evaluator(context.Background(), src, src.URL, bookVars(Book{URL: "book"}))
	value, err := e.EvalJS(`book.getVariable('infinite') === null && book.getVariable('invalid') === null && book.getVariable('float32') === null && JSON.stringify(book.getVariable('values')) === '[null,null,null,42]'`, nil)
	if err != nil || value != true {
		details, _ := e.EvalJS(`JSON.stringify(book.getVariableMap())`, nil)
		t.Fatalf("non-finite values were not restored as null: %v %v (%v)", value, err, details)
	}
	if !restored.IsLoggedIn(other) || restored.sessionCookie(other, other.URL) != "session=normal-token" {
		t.Fatal("invalid variables prevented another source's login from persisting")
	}
}

type sessionSaveSnapshotProbe struct {
	once   sync.Once
	called chan struct{}
}

func (p *sessionSaveSnapshotProbe) MarshalJSON() ([]byte, error) {
	p.once.Do(func() { close(p.called) })
	return []byte(`"snapshot"`), nil
}

func TestSessionQueuedSavesCannotRestoreLoggedOutState(t *testing.T) {
	c := NewClient()
	src := Source{URL: "https://login.example.test"}
	if err := c.Login(context.Background(), src, map[string]string{"Cookie": "session=old-token"}); err != nil {
		t.Fatal(err)
	}
	probe := &sessionSaveSnapshotProbe{called: make(chan struct{})}
	state := c.sessionState()
	state.mu.Lock()
	state.source(src.URL).Books["book"] = map[string]any{"probe": probe}
	state.mu.Unlock()
	dir := t.TempDir()
	c.sessionSaveMu.Lock()
	locked := true
	defer func() {
		if locked {
			c.sessionSaveMu.Unlock()
		}
	}()
	first := make(chan error, 1)
	go func() { first <- c.SaveSessions(dir) }()
	select {
	case <-probe.called:
		t.Error("save took a snapshot before acquiring the save lock")
	case err := <-first:
		t.Fatalf("save bypassed the save lock: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	if err := c.Logout(src); err != nil {
		t.Fatal(err)
	}
	second := make(chan error, 1)
	go func() { second <- c.SaveSessions(dir) }()
	select {
	case err := <-second:
		t.Fatalf("logout save bypassed the save lock: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	c.sessionSaveMu.Unlock()
	locked = false
	for _, done := range []chan error{first, second} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("queued save deadlocked")
		}
	}
	select {
	case <-probe.called:
		t.Error("a queued save retained the old session snapshot")
	default:
	}
	restored := NewClient()
	if err := restored.LoadSessions(dir); err != nil {
		t.Fatal(err)
	}
	if restored.IsLoggedIn(src) || restored.sessionCookie(src, src.URL) != "" {
		t.Fatal("an older queued save restored the logged-out session")
	}
}

func TestSessionAJAXCancellationAndDepth(t *testing.T) {
	c := NewClient()
	src := Source{URL: "https://fixture.example.test", Name: "fixture"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e := c.evaluator(ctx, src, src.URL, nil)
	if _, err := e.EvalJS(`java.ajax(baseUrl)`, nil); err == nil {
		t.Fatal("canceled ajax succeeded")
	}
	e = c.evaluator(context.WithValue(context.Background(), ajaxDepthKey{}, 8), src, src.URL, nil)
	if _, err := e.EvalJS(`java.ajax(baseUrl)`, nil); err == nil || !strings.Contains(err.Error(), "嵌套") {
		t.Fatalf("missing ajax depth error: %v", err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	e = c.evaluator(ctx, src, src.URL, nil)
	started := time.Now()
	if _, err := e.EvalJS(`while(true){}`, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("request deadline did not interrupt JS: %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("JS ignored request cancellation")
	}
}

func TestSessionBookVariableObjectsAreSnapshots(t *testing.T) {
	c := NewClient()
	src := Source{URL: "fixture"}
	e := c.evaluator(context.Background(), src, "https://fixture.example.test", bookVars(Book{URL: "book-1"}))
	value, err := e.EvalJS(`let value={nested:{name:'original'}};java.put('object',value);value.nested.name='changed';let stored=java.get('object');stored.nested.name='changed-again';java.get('object').nested.name`, nil)
	if err != nil || value != "original" {
		t.Fatalf("java variable exposed mutable shared state: %v %v", value, err)
	}
	value, err = e.EvalJS(`book.putVariable('object',{nested:{name:'original'}});let value=book.getVariableMap();value.object.nested.name='changed';book.getVariable('object').nested.name`, nil)
	if err != nil || value != "original" {
		t.Fatalf("book variable exposed mutable shared state: %v %v", value, err)
	}
}

func TestSessionSourcesCannotReadOrOverwriteEachOtherCookies(t *testing.T) {
	c := NewClient()
	a := Source{URL: "source-a"}
	b := Source{URL: "source-b"}
	origin := "https://shared.example.test"
	if err := c.setSessionCookie(a, origin, "session=A"); err != nil {
		t.Fatal(err)
	}
	if cookie := c.sessionCookie(b, origin); cookie != "" {
		t.Fatal("second source read first source cookies")
	}
	if err := c.setSessionCookie(b, origin, "session=B"); err != nil {
		t.Fatal(err)
	}
	if c.sessionCookie(a, origin) != "session=A" || c.sessionCookie(b, origin) != "session=B" {
		t.Fatal("source cookies collided")
	}
	c.recordSessionCookies(a, origin+"/api/login", []*http.Cookie{{Name: "pathCookie", Value: "private", Path: "/api", Secure: true, MaxAge: 600}})
	if !strings.Contains(c.sessionCookie(a, origin+"/api/read"), "pathCookie=private") {
		t.Fatal("path cookie missing")
	}
	dir := t.TempDir()
	if err := c.SaveSessions(dir); err != nil {
		t.Fatal(err)
	}
	restored := NewClient()
	if err := restored.LoadSessions(dir); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(restored.sessionCookie(a, origin+"/api/read"), "pathCookie=private") {
		t.Fatal("path cookie not restored")
	}
	if err := restored.Logout(a); err != nil {
		t.Fatal(err)
	}
	if restored.sessionCookie(a, origin+"/api/read") != "" {
		t.Fatal("logout retained non-root path cookie")
	}
	if restored.sessionCookie(b, origin) != "session=B" {
		t.Fatal("logout deleted another source's cookies")
	}
}

// Optional local acceptance fixture: source definitions remain outside the
// repository. Every request is handled by this transport; no real login or
// external HTTP request occurs, even when the source chooses another host.
func TestSessionRealSourceOffline(t *testing.T) {
	path := os.Getenv("READCLI_LEGADO_TEST_SOURCE")
	if path == "" {
		t.Skip("set READCLI_LEGADO_TEST_SOURCE for the local source fixture")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var sources []Source
	if err = json.Unmarshal(data, &sources); err != nil {
		var source Source
		if err = json.Unmarshal(data, &source); err != nil {
			t.Fatal(err)
		}
		sources = []Source{source}
	}
	if len(sources) != 1 {
		t.Fatal("expected one source")
	}
	src := sources[0]
	c := NewClient()
	c.HTTPClient.Transport = sessionRoundTripper(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/login_api":
			return sessionResponse(req, 200, `{"code":0,"key":"offline-token"}`), nil
		case "/search":
			return sessionResponse(req, 200, `{"code":0,"data":[{"book_id":"b1","source":"fixture","tab":"小说","book_name":"离线样本","author":"测试作者","toc_url":"","tags":"测试"}]}`), nil
		case "/detail":
			return sessionResponse(req, 200, `{"code":0,"data":{"book_id":"b1","source":"fixture","tab":"小说","book_name":"离线样本","author":"测试作者","toc_url":"","abstract":"测试简介","tags":"测试","status":"连载","score":"0","word_number":"1000"}}`), nil
		case "/catalog":
			return sessionResponse(req, 200, `{"code":0,"data":[{"item_id":"c1","title":"第一章","source":"fixture","tab":"小说","toc_url":""}]}`), nil
		case "/content":
			if cookie, err := req.Cookie("qttoken"); err != nil || cookie.Value != "offline-token" {
				t.Fatal("content request lost authentication")
			}
			return sessionResponse(req, 200, `{"code":0,"content":"第一段离线内容。\n第二段离线内容。"}`), nil
		case "/get_avatar":
			return sessionResponse(req, 200, `{"code":0,"nickname":"离线用户"}`), nil
		default:
			return nil, fmt.Errorf("unexpected fixture endpoint %s", req.URL.Path)
		}
	})
	ctx := context.Background()
	if err := c.Login(ctx, src, map[string]string{"邮箱": "offline@example.test", "密码": "offline-fixture-password"}); err != nil {
		t.Fatal(err)
	}
	books, err := c.Search(ctx, src, "离线", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 {
		t.Fatalf("books %d", len(books))
	}
	book, err := c.BookInfo(ctx, src, books[0])
	if err != nil {
		t.Fatal(err)
	}
	chapters, err := c.Chapters(ctx, src, book)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 1 {
		t.Fatalf("chapters %d", len(chapters))
	}
	content, err := c.Content(ctx, src, book, chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "第二段离线内容") {
		t.Fatalf("unexpected extracted content %q", content)
	}
}
