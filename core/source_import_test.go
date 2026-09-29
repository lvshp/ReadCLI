package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lvshp/ReadCLI/booksource"
)

func TestSourceImportDraggedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "安卓阅读app-大灰狼融合 4.0(vip完全版)(1).json")
	if err := os.WriteFile(path, []byte(`{"bookSourceUrl":"https://example.test","bookSourceName":"拖入测试","searchUrl":"/search"}`), 0600); err != nil {
		t.Fatal(err)
	}
	inputs := []string{path, `"` + path + `"`, "'" + path + "'"}
	if runtime.GOOS != "windows" {
		inputs = append(inputs, strings.NewReplacer(" ", `\ `, "(", `\(`, ")", `\)`).Replace(path)+" ")
	}
	for _, input := range inputs {
		sources, err := booksource.Import(context.Background(), normalizeSourceImportLocation(input))
		if err != nil {
			t.Errorf("importing %q: %v", input, err)
			continue
		}
		if len(sources) != 1 || sources[0].Name != "拖入测试" {
			t.Errorf("unexpected sources: %+v", sources)
		}
	}
}

func TestNormalizeSourceImportLocation(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	url := `https://example.test/source.json?name=a%20b&key=a+b&literal=\foo&quote='`
	tests := []struct{ input, want string }{
		{url, url},
		{`"` + url + `"`, url},
		{"'https://example.test/source.json?name=a%20b&key=a+b'", "https://example.test/source.json?name=a%20b&key=a+b"},
		{`C:\Downloads\新书源(vip)\test.json`, `C:\Downloads\新书源(vip)\test.json`},
		{`"C:\new\test file.json"`, `C:\new\test file.json`},
		{`\\server\share\sources.json`, `\\server\share\sources.json`},
		{`'\\server\share\sources.json'`, `\\server\share\sources.json`},
		{"~/sources.json", filepath.Join(home, "sources.json")},
		{"~", home},
		{"~someone/sources.json", "~someone/sources.json"},
		{"", ""}, {"  ", ""}, {`""`, ""}, {"''", ""},
		{"/tmp/reader's sources.json", "/tmp/reader's sources.json"},
	}
	if runtime.GOOS != "windows" {
		tests = append(tests, struct{ input, want string }{`/tmp/reader\'s\ sources\(1\).json`, "/tmp/reader's sources(1).json"})
	} else {
		tests = append(tests, struct{ input, want string }{`~\sources.json`, filepath.Join(home, "sources.json")})
	}
	for _, tc := range tests {
		if got := normalizeSourceImportLocation(tc.input); got != tc.want {
			t.Errorf("normalize(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
