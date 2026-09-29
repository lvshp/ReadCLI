package core

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/lvshp/ReadCLI/booksource"
	"github.com/lvshp/ReadCLI/reader"
	"github.com/rivo/tview"
)

func simulationText(screen tcell.SimulationScreen, width, height int) string {
	var lines []string
	for y := 0; y < height; y++ {
		var line strings.Builder
		for x := 0; x < width; x++ {
			ch, combining, _, cellWidth := screen.GetContent(x, y)
			if ch != 0 {
				line.WriteRune(ch)
				for _, extra := range combining {
					line.WriteRune(extra)
				}
			}
			if cellWidth > 1 {
				x += cellWidth - 1
			}
		}
		lines = append(lines, strings.TrimRight(line.String(), " "))
	}
	return strings.Join(lines, "\n")
}

func TestOnlineHighlightPreservesLiteralMarkup(t *testing.T) {
	const original = "[red]中文 [x](fg:red)"
	for _, query := range []string{"", "red", "]", "[", "中文", "[x](fg:red)"} {
		t.Run(query, func(t *testing.T) {
			screen := tcell.NewSimulationScreen("UTF-8")
			if err := screen.Init(); err != nil {
				t.Fatal(err)
			}
			defer screen.Fini()
			screen.SetSize(100, 1)
			tview.Print(screen, highlightOnlineText(original, query), 0, 0, 100, tview.AlignLeft, tcell.ColorWhite)
			if got := simulationText(screen, 100, 1); got != original {
				t.Fatalf("query %q rendered %q, want %q", query, got, original)
			}
		})
	}
}

func TestOnlineReadingChromePreservesTextAndTrustedHelp(t *testing.T) {
	const original = "[red]中文 [x](fg:red)"
	for _, quickHelp := range []bool{false, true} {
		for _, query := range []string{"", "red", "]", "[", "中文"} {
			t.Run(query+map[bool]string{false: "/plain", true: "/help"}[quickHelp], func(t *testing.T) {
				initTestUIState()
				app.reader = reader.NewOnlineReader(booksource.Book{Name: "测试书"}, []booksource.Chapter{{Name: "第一章", URL: "/1"}}, 0, original)
				app.currentFile = "online:test"
				app.mode = modeReading
				app.readingState.searchQuery = query
				app.readingState.showReadingQuickHelp = quickHelp
				refreshChrome()
				screen := tcell.NewSimulationScreen("UTF-8")
				if err := screen.Init(); err != nil {
					t.Fatal(err)
				}
				defer screen.Fini()
				screen.SetSize(120, 24)
				main.SetBorder(false)
				main.SetRect(0, 0, 120, 24)
				main.Draw(screen)
				got := simulationText(screen, 120, 24)
				if !strings.Contains(got, original) {
					t.Fatalf("body changed by chrome rendering: %q", got)
				}
				if strings.Contains(got, "fg:black") || strings.Contains(got, "[black:yellow") || strings.Contains(got, "fg:green") {
					t.Fatalf("style markup visible: %q", got)
				}
				if quickHelp && !strings.Contains(got, "快捷键") {
					t.Fatalf("trusted help missing: %q", got)
				}
			})
		}
	}
}
