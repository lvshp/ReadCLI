package core

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/lvshp/ReadCLI/purification"
)

func TestPurificationUIPanelsShowConfigAndSelectedFile(t *testing.T) {
	for _, size := range [][2]int{{48, 16}, {80, 24}, {140, 40}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			initTestUIState()
			app.mode = modePurification
			app.purification = purificationState{
				returnMode: modeHome, enabledCount: 3, totalCount: 7,
				configPath: "/Users/test/.readcli/replace_rules/main.json", importsPath: "/Users/test/.readcli/replace_rules/imports",
			}
			for i := 0; i < 7; i++ {
				app.purification.files = append(app.purification.files, purification.FileInfo{Name: fmt.Sprintf("配置%d.json", i), Path: fmt.Sprintf("/Users/test/.readcli/replace_rules/imports/配置%d.json", i), RuleCount: 7, EnabledCount: 3})
			}
			app.purification.index = 6
			screen := tcell.NewSimulationScreen("UTF-8")
			if err := screen.Init(); err != nil {
				t.Fatal(err)
			}
			defer screen.Fini()
			screen.SetSize(size[0], size[1])
			root.SetRect(0, 0, size[0], size[1])
			applyLayout(size[0], size[1])
			root.Draw(screen)
			got := simulationText(screen, size[0], size[1])
			for _, want := range []string{"净化配置", "启用 3 / 7", "main.json:", "imports:", "> 配置6.json 3/7"} {
				if !strings.Contains(got, want) {
					t.Fatalf("missing %q at %v:\n%s", want, size, got)
				}
			}
			if !strings.Contains(buildPurificationRightPanel(), app.purification.files[6].Path) {
				t.Fatal("selected rule file's full path is not available")
			}
		})
	}
}

func TestPurificationUIImportCancelPreservesReturnPage(t *testing.T) {
	initTestUIState()
	app.mode = modePurification
	app.purification.returnMode = modeSources
	dispatchEvent("i")
	if app.mode != modePurificationImport || !isInputMode(app.mode) {
		t.Fatal("import mode does not accept text input")
	}
	dispatchEvent("q")
	if app.uiState.input.value != "q" || app.mode != modePurificationImport {
		t.Fatal("q in an import path must be text")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.online.cancel = cancel
	app.online.busy = "正在导入"
	dispatchEvent("<Escape>")
	if app.mode != modePurification || app.purification.returnMode != modeSources || app.uiState.input.value != "" || ctx.Err() == nil || app.online.busy != "" {
		t.Fatal("Esc did not cancel import and return to configuration")
	}
	dispatchEvent("q")
	if app.mode != modeSources {
		t.Fatal("configuration did not return to its original page")
	}
}

func TestPurificationUIBusyExitAndFileNavigation(t *testing.T) {
	for _, origin := range []mode{modeHome, modeReading, modeSources} {
		t.Run(string(origin), func(t *testing.T) {
			initTestUIState()
			app.reader = &fakeReader{chapter: "原章节", pos: 3, total: 10}
			originalReader := app.reader
			app.mode = modePurification
			app.purification.returnMode = origin
			app.purification.files = []purification.FileInfo{{Name: "a.json"}, {Name: "main.json"}}
			dispatchEvent("<Down>")
			dispatchEvent("<Down>")
			if app.purification.index != 1 {
				t.Fatal("file selection did not clamp")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			app.online.cancel = cancel
			app.online.busy = "正在读取"
			dispatchEvent("<Escape>")
			if ctx.Err() == nil || app.online.busy != "" || app.mode != origin || app.reader != originalReader {
				t.Fatal("busy Esc did not restore the original scene")
			}
		})
	}
}

func TestPurificationUIEscapesImportedNamesAndErrors(t *testing.T) {
	initTestUIState()
	app.mode = modePurification
	app.purification.files = []purification.FileInfo{{Name: "[red].json", Path: "/imports/[red].json"}}
	app.purification.errors = []string{"[green]invalid"}
	for _, panel := range []string{buildPurificationPanel(), buildPurificationRightPanel()} {
		if strings.Contains(panel, "[red]") || strings.Contains(panel, "[green]") {
			t.Fatalf("unescaped imported text: %s", panel)
		}
	}
}

func TestPurificationUIErrorDetailsExposeLongPathsAndCauses(t *testing.T) {
	for _, size := range [][2]int{{48, 16}, {80, 24}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			initTestUIState()
			app.mode = modePurification
			app.purification.returnMode = modeSources
			app.purification.files = []purification.FileInfo{{Name: "import.json"}, {Name: "main.json"}}
			app.purification.index = 1
			longPath := "/Users/reader/Documents/" + strings.Repeat("reading-backup/", 24) + ".readcli/replace_rules/imports/广告净化-" + strings.Repeat("abcdef12", 8) + ".json"
			first := "净化规则 " + longPath + ": 净化规则 5（广告清理）: 正则表达式语法错误或不支持"
			second := "上次净化: 净化规则“[red]脚本清理”: JavaScript 替换执行失败"
			app.purification.errors = []string{first, second}
			screen := tcell.NewSimulationScreen("UTF-8")
			if err := screen.Init(); err != nil {
				t.Fatal(err)
			}
			defer screen.Fini()
			screen.SetSize(size[0], size[1])
			root.SetRect(0, 0, size[0], size[1])
			applyLayout(size[0], size[1])
			root.Draw(screen)
			if !strings.Contains(simulationText(screen, size[0], size[1]), "e 查看详情") {
				t.Fatal("small-terminal list did not expose the error-details shortcut")
			}
			dispatchEvent("e")
			if !app.purification.showErrors {
				t.Fatal("e did not open error details")
			}
			// Read actual rendered cells while scrolling. Reconstruct all visible
			// error rows to prove neither the path nor its trailing cause is lost.
			seen := map[int]string{}
			lastLine := 0
			for step := 0; step < 200; step++ {
				refreshChrome()
				root.Draw(screen)
				x, y, width, height := main.GetInnerRect()
				for row := 2; row < height; row++ {
					var text strings.Builder
					for col := 0; col < width; col++ {
						r, combining, _, cellWidth := screen.GetContent(x+col, y+row)
						if r != 0 {
							text.WriteRune(r)
							for _, extra := range combining {
								text.WriteRune(extra)
							}
						}
						if cellWidth > 1 {
							col += cellWidth - 1
						}
					}
					line := app.purification.errorScroll + row - 2
					seen[line] = strings.TrimSpace(text.String())
					lastLine = max(lastLine, line)
				}
				previous := app.purification.errorScroll
				dispatchEvent("<Down>")
				if app.purification.errorScroll == previous {
					break
				}
			}
			if app.purification.errorScroll == 0 {
				t.Fatal("long error details did not scroll")
			}
			var rendered strings.Builder
			for line := 0; line <= lastLine; line++ {
				rendered.WriteString(seen[line])
			}
			withoutSpaces := func(text string) string {
				return strings.Map(func(r rune) rune {
					if unicode.IsSpace(r) {
						return -1
					}
					return r
				}, text)
			}
			got := withoutSpaces(rendered.String())
			for _, want := range []string{longPath, "净化规则5（广告清理）:正则表达式语法错误或不支持", "［red］脚本清理", "JavaScript替换执行失败"} {
				if !strings.Contains(got, withoutSpaces(want)) {
					t.Fatalf("scrolling hid %q at %v:\n%s", want, size, rendered.String())
				}
			}
			if strings.Contains(got, "[red]") {
				t.Fatal("error metadata was treated as terminal markup")
			}
			dispatchEvent("<Up>")
			dispatchEvent("<Escape>")
			if app.mode != modePurification || app.purification.showErrors || app.purification.index != 1 {
				t.Fatal("first Esc did not return to the same selected file")
			}
			dispatchEvent("<Escape>")
			if app.mode != modeSources {
				t.Fatal("second Esc did not return to the original page")
			}
		})
	}
}
