package core

import (
	"strings"
	"testing"

	"github.com/lvshp/ReadCLI/lib"
)

func TestReadingVisibleSourceLinesUsesDisplayLinesDirectly(t *testing.T) {
	app = &appState{
		displayLines: 6,
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
		displayLines: 20,
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
		settingsIndex: 0,
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
