package purification

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type storageTransport func(*http.Request) (*http.Response, error)

func (f storageTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func writeStorageFixture(t *testing.T, filePath, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filePath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestStorageEnsurePreservesLocalEditsAndLoadDoesNotInitialize(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	rules, files, err := Load(dir)
	if err != nil || len(rules) != 0 || len(files) != 0 {
		t.Fatalf("missing configuration: rules=%v files=%v err=%v", rules, files, err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("Load created the directory: %v", err)
	}
	if err := Ensure(dir); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "replace_rules", "main.json")
	initial, err := os.ReadFile(mainPath)
	if err != nil || strings.TrimSpace(string(initial)) != "[]" {
		t.Fatalf("initial main = %q, %v", initial, err)
	}
	edit := `{"name":"本地修改","pattern":"旧字","replacement":"新字","isRegex":false,"isEnabled":false,"custom":{"keep":true}}`
	writeStorageFixture(t, mainPath, edit)
	if err := Ensure(dir); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(mainPath)
	if string(after) != edit {
		t.Fatal("Ensure changed an existing local file")
	}
	rules, files, err = Load(dir)
	if err != nil || len(rules) != 1 || len(files) != 1 || !files[0].Local || files[0].RuleCount != 1 || files[0].EnabledCount != 0 || files[0].Path != mainPath {
		t.Fatalf("single-object local file: rules=%v files=%+v err=%v", rules, files, err)
	}
	after, _ = os.ReadFile(mainPath)
	if string(after) != edit {
		t.Fatal("Load rewrote unrecognized JSON fields")
	}
}

func TestStorageLocalImportsStaySeparateAndPreserveUnknownFields(t *testing.T) {
	dir, sources := t.TempDir(), t.TempDir()
	firstPath, secondPath := filepath.Join(sources, "first.json"), filepath.Join(sources, "second.json")
	first := "\xef\xbb\xbf" + `[
  {"name":"导入甲","pattern":"甲","replacement":"乙","isRegex":false,"custom":{"script":"throw new Error('must never execute')"}}
]` + "\n"
	writeStorageFixture(t, firstPath, first)
	writeStorageFixture(t, secondPath, `{"name":"导入乙","pattern":"乙","replacement":"丙","isRegex":false,"isEnabled":false}`)
	a, err := Import(context.Background(), dir, firstPath)
	if err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "replace_rules", "main.json")
	local := `[{"name":"本地末处理","pattern":"丁","replacement":"戊","isRegex":false}]`
	writeStorageFixture(t, mainPath, local)
	b, err := Import(context.Background(), dir, secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if a.Path == b.Path || a.Local || b.Local || a.RuleCount != 1 || a.EnabledCount != 1 || b.EnabledCount != 0 {
		t.Fatalf("wrong file identity or statistics: a=%+v b=%+v", a, b)
	}
	stored, _ := os.ReadFile(a.Path)
	if string(stored) != first {
		t.Fatalf("import changed raw JSON: %q", stored)
	}
	updated := `[{"name":"更新甲","pattern":"新甲","replacement":"新乙","isRegex":false,"futureOption":17}]`
	writeStorageFixture(t, firstPath, updated)
	after, err := Import(context.Background(), dir, firstPath)
	if err != nil || after.Path != a.Path {
		t.Fatalf("same-source import did not update its file: %+v %v", after, err)
	}
	stored, _ = os.ReadFile(a.Path)
	if string(stored) != updated {
		t.Fatal("same-source contents were not replaced")
	}
	stored, _ = os.ReadFile(mainPath)
	if string(stored) != local {
		t.Fatal("import overwrote main.json")
	}
	rules, files, err := Load(dir)
	if err != nil || len(rules) != 3 || len(files) != 3 || !files[len(files)-1].Local {
		t.Fatalf("separate groups not loaded: %+v %+v %v", rules, files, err)
	}
	if rules[len(rules)-1].Name != "本地末处理" {
		t.Fatal("local group did not run last")
	}
}

func TestStorageLoadStableGroupOrderingAndReloadAfterDirectoryRestore(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "replace_rules")
	writeStorageFixture(t, filepath.Join(base, "imports", "b.json"), `[{"name":"b-low","pattern":"b","order":-100}]`)
	writeStorageFixture(t, filepath.Join(base, "imports", "a.json"), `[
  {"name":"a-high","pattern":"a","order":9},
  {"name":"a-first","pattern":"a","order":1},
  {"name":"a-second","pattern":"a","order":1}
]`)
	writeStorageFixture(t, filepath.Join(base, "imports", "notes.txt"), "not a rule file")
	writeStorageFixture(t, filepath.Join(base, "main.json"), `{"name":"local-low","pattern":"c","order":-200}`)
	rules, files, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(rules))
	for i, rule := range rules {
		names[i] = rule.Name
	}
	if want := []string{"a-first", "a-second", "a-high", "b-low", "local-low"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("order = %v, want %v", names, want)
	}
	if len(files) != 3 || files[0].Name != "a.json" || files[1].Name != "b.json" || !files[2].Local {
		t.Fatalf("file order = %+v", files)
	}
	if err := os.Rename(base, filepath.Join(dir, "previous-config")); err != nil {
		t.Fatal(err)
	}
	writeStorageFixture(t, filepath.Join(base, "main.json"), `{"name":"恢复配置","pattern":"restore"}`)
	rules, files, err = Load(dir)
	if err != nil || len(rules) != 1 || rules[0].Name != "恢复配置" || len(files) != 1 {
		t.Fatalf("restored directory retained cached rules: %+v %+v %v", rules, files, err)
	}
}

func TestStorageInvalidImportsDoNotReplaceExistingRules(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(t.TempDir(), "rules.json")
	valid := `[{"name":"valid","pattern":"good"}]`
	writeStorageFixture(t, source, valid)
	info, err := Import(context.Background(), dir, source)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		"", "null", "42", "[] trailing", "[null]", `["not a rule"]`,
		`{"name":"broken","pattern":"[","isRegex":true}`,
		`[{"name":"disabled broken","pattern":"[","isRegex":true,"isEnabled":false}]`,
	} {
		t.Run(fmt.Sprintf("invalid_%q", invalid), func(t *testing.T) {
			writeStorageFixture(t, source, invalid)
			if _, err := Import(context.Background(), dir, source); err == nil || !strings.Contains(err.Error(), source) {
				t.Fatalf("missing source-aware validation error: %v", err)
			}
			stored, _ := os.ReadFile(info.Path)
			if string(stored) != valid {
				t.Fatal("invalid import replaced the valid file")
			}
		})
	}
	writeStorageFixture(t, filepath.Join(dir, "replace_rules", "main.json"), `[{"pattern":"["}]`)
	if _, _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "main.json") {
		t.Fatalf("manual edit error did not name its file: %v", err)
	}
}

func TestStorageImportValidatesScriptsWithoutExecutingThem(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(t.TempDir(), "script.json")
	data := `{"name":"只编译脚本","pattern":"test","replacement":"@js:throw new Error('must not run during import');","isRegex":true}`
	writeStorageFixture(t, source, data)
	info, err := Import(context.Background(), dir, source)
	if err != nil {
		t.Fatalf("import executed a replacement script: %v", err)
	}
	if _, _, err := Load(dir); err != nil {
		t.Fatalf("loading executed a replacement script: %v", err)
	}
	stored, _ := os.ReadFile(info.Path)
	if string(stored) != data {
		t.Fatal("script contents were changed")
	}
}

func TestStorageImportHTTPStatusContextAndLimits(t *testing.T) {
	var body atomic.Value
	body.Store(`[{"name":"HTTP规则","pattern":"old","replacement":"new","unknown":true}]`)
	waitStarted := make(chan struct{}, 1)
	previousTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previousTransport })
	http.DefaultTransport = storageTransport(func(r *http.Request) (*http.Response, error) {
		response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body.Load().(string))), Request: r}
		switch r.URL.Path {
		case "/rules.json":
		case "/failure":
			response.StatusCode = http.StatusServiceUnavailable
		case "/large":
			response.ContentLength = ruleFileLimit + 1
		case "/wait":
			waitStarted <- struct{}{}
			<-r.Context().Done()
			return nil, r.Context().Err()
		default:
			return nil, fmt.Errorf("unexpected path %s", r.URL.Path)
		}
		return response, nil
	})
	baseURL := "https://rules.example.test"
	dir := t.TempDir()
	info, err := Import(context.Background(), dir, baseURL+"/rules.json#first")
	if err != nil {
		t.Fatal(err)
	}
	body.Store(`{"name":"HTTP更新","pattern":"new"}`)
	updated, err := Import(context.Background(), dir, baseURL+"/rules.json#second")
	if err != nil || info.Path != updated.Path {
		t.Fatalf("URL fragment changed source identity: %+v %+v %v", info, updated, err)
	}
	for _, endpoint := range []string{"/failure", "/large"} {
		if _, err := Import(context.Background(), dir, baseURL+endpoint); err == nil {
			t.Fatalf("invalid response %s was imported", endpoint)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Import(ctx, dir, baseURL+"/wait"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled import: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := Import(ctx, dir, baseURL+"/wait")
		done <- err
	}()
	select {
	case <-waitStarted:
	case <-time.After(time.Second):
		t.Fatal("HTTP import request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("in-flight canceled import: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP import ignored cancellation")
	}
	if _, err := readRuleBytes(context.Background(), strings.NewReader(strings.Repeat("x", int(ruleFileLimit)+1))); err == nil {
		t.Fatal("streamed response/local file size limit was not enforced")
	}
}

func TestStorageAtomicWriteFailureLeavesOtherFilesIntact(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(t.TempDir(), "rules.json")
	writeStorageFixture(t, source, `{"pattern":"one"}`)
	info, err := Import(context.Background(), dir, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(info.Path, info.Path+".before"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(info.Path, 0700); err != nil {
		t.Fatal(err)
	}
	writeStorageFixture(t, source, `{"pattern":"two"}`)
	if _, err := Import(context.Background(), dir, source); err == nil || !strings.Contains(err.Error(), info.Path) {
		t.Fatalf("expected rename error naming target: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(info.Path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".rules-") {
			t.Fatal("failed atomic write left a temporary rule file")
		}
	}
	stored, _ := os.ReadFile(info.Path + ".before")
	if string(stored) != `{"pattern":"one"}` {
		t.Fatal("failed import changed a separate file")
	}
}
