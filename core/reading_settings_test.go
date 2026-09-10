package core

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lvshp/ReadCLI/lib"
)

func TestReadingAlignmentPositionsColumn(t *testing.T) {
	oldApp, oldWidth, oldHeight := app, mainContentWidth, mainContentHeight
	t.Cleanup(func() { app, mainContentWidth, mainContentHeight = oldApp, oldWidth, oldHeight })
	for _, compact := range []bool{false, true} {
		for _, tc := range []struct {
			alignment                                 string
			width, contentWidth, left, right, padding int
		}{
			{"left", 80, 40, 2, 4, 2},
			{"center", 80, 40, 2, 4, 19},
			{"right", 80, 40, 2, 4, 36},
			{"center", 51, 36, 0, 0, 7},
			{"right", 51, 36, 0, 0, 15},
			{"right", 40, 40, 2, 4, 0},
			{"center", 20, 40, 2, 4, 0},
			{"right", 45, 40, 20, 20, 5},
		} {
			t.Run(fmt.Sprintf("compact=%t/%+v", compact, tc), func(t *testing.T) {
				app = &appState{
					mode:         modeReading,
					readingState: readingState{compactMode: compact, contentWidth: tc.contentWidth},
					config:       &lib.Config{ReadingAlignment: tc.alignment, ReadingMarginLeft: tc.left, ReadingMarginRight: tc.right},
				}
				mainContentWidth, mainContentHeight = tc.width, 20
				text := "  中文段落\n[match](fg:black,bg:yellow,mod:bold) short"
				pad := strings.Repeat(" ", tc.padding)
				want := pad + "  中文段落\n" + pad + "[match](fg:black,bg:yellow,mod:bold) short"
				if compact {
					want = "\n" + want
				}
				if got := formatReadingPanel(text); got != want {
					t.Fatalf("formatted column = %q, want %q", got, want)
				}
			})
		}
	}
}

type alignmentTestReader struct {
	fakeReader
	reflows int
}

func (r *alignmentTestReader) Reflow(int) { r.reflows++ }

func TestReadingAlignmentKeyboardAndSettingsPersistWithoutReflow(t *testing.T) {
	t.Setenv("READCLI_DATA_DIR", t.TempDir())
	oldApp := app
	t.Cleanup(func() { app = oldApp })
	r := &alignmentTestReader{fakeReader: fakeReader{pos: 12, total: 100}}
	app = &appState{
		mode:         modeReading,
		config:       &lib.Config{},
		reader:       r,
		readingState: readingState{compactMode: true, contentWidth: 40},
	}
	for _, want := range []string{"left", "right", "center"} {
		dispatchEvent("a")
		cfg, err := lib.LoadConfig()
		if err != nil || cfg.ReadingAlignment != want {
			t.Fatalf("shortcut saved %#v, %v; want %q", cfg, err, want)
		}
	}
	app.mode = modeReadingSettings
	app.readingState.settingsIndex = len(readingSettingsItems()) - 1
	if !strings.Contains(buildReadingSettingsPanel(), "居中") {
		t.Fatal("settings should show the selected alignment")
	}
	for _, step := range []struct{ key, want string }{{"<Left>", "right"}, {"<Right>", "center"}, {"<Enter>", "left"}} {
		dispatchEvent(step.key)
		cfg, err := lib.LoadConfig()
		if err != nil || cfg.ReadingAlignment != step.want {
			t.Fatalf("settings saved %#v, %v; want %q", cfg, err, step.want)
		}
	}
	if r.reflows != 0 || r.pos != 12 || app.readingState.contentWidth != 40 {
		t.Fatalf("alignment changed wrapping or progress: reader=%+v, width=%d", r, app.readingState.contentWidth)
	}
}

func TestReadingVisibleSourceLinesUsesDisplayLinesDirectly(t *testing.T) {
	app = &appState{
		readingState: readingState{
			displayLines: 6,
		},
		config: &lib.Config{
			ReadingMarginTop:    1,
			ReadingMarginBottom: 0,
			ReadingLineSpacing:  1,
		},
	}
	mainContentWidth = 98
	mainContentHeight = 28

	if got := readingVisibleSourceLines(); got != 6 {
		t.Fatalf("readingVisibleSourceLines() = %d, want 6", got)
	}
}

func TestReadingVisibleSourceLinesCapsToAvailableHeight(t *testing.T) {
	app = &appState{
		readingState: readingState{
			displayLines: 20,
		},
		config: &lib.Config{
			ReadingMarginTop:    1,
			ReadingMarginBottom: 1,
			ReadingLineSpacing:  1,
		},
	}
	mainContentWidth = 58
	mainContentHeight = 8

	got := readingVisibleSourceLines()
	if got < 1 || got >= 20 {
		t.Fatalf("readingVisibleSourceLines() = %d, want capped positive value", got)
	}
}

func TestFormatReadingPanelAppliesMarginsAndSpacing(t *testing.T) {
	app = &appState{
		config: &lib.Config{
			ReadingMarginLeft:   2,
			ReadingMarginTop:    1,
			ReadingMarginBottom: 1,
			ReadingLineSpacing:  1,
		},
	}

	got := formatReadingPanel("第一行\n第二行")
	want := "\n  第一行\n\n  第二行\n"
	if got != want {
		t.Fatalf("formatReadingPanel() = %q, want %q", got, want)
	}
}

func TestParseConfiguredUIColorSupportsHexAndRGB(t *testing.T) {
	if _, ok := parseConfiguredUIColor("#ABCDEF"); !ok {
		t.Fatalf("hex color should parse")
	}
	if _, ok := parseConfiguredUIColor("12,34,56"); !ok {
		t.Fatalf("rgb color should parse")
	}
	if _, ok := parseConfiguredUIColor("300,0,0"); ok {
		t.Fatalf("invalid rgb color should fail")
	}
}

func TestBuildReadingSettingsPanelIncludesColorValue(t *testing.T) {
	app = &appState{
		readingState: readingState{
			settingsIndex: 0,
		},
		config: &lib.Config{
			ForceBasicColor:          true,
			ReadingContentWidthRatio: 0.75,
			ReadingMarginLeft:        2,
			ReadingMarginRight:       0,
			ReadingMarginTop:         1,
			ReadingMarginBottom:      0,
			ReadingLineSpacing:       1,
			ReadingTextColor:         "#FFFFFF",
			ReadingHighContrast:      true,
		},
	}

	panel := buildReadingSettingsPanel()
	if !strings.Contains(panel, "字体颜色") || !strings.Contains(panel, "#FFFFFF") {
		t.Fatalf("reading settings panel missing color entry: %q", panel)
	}
	if !strings.Contains(panel, "基础色模式") || !strings.Contains(panel, "开") {
		t.Fatalf("reading settings panel missing basic color toggle: %q", panel)
	}
}
