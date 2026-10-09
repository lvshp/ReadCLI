package core

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/lvshp/ReadCLI/lib"
)

func TestUpdateFailureOpensDetailsAndReturnsToPreviousScene(t *testing.T) {
	for _, scene := range []mode{modeHome, modeReading} {
		t.Run(string(scene), func(t *testing.T) {
			initTestUIState()
			app.mode = modeUpdating
			app.updateState.returnMode = scene
			app.updateState.release = &lib.ReleaseInfo{TagName: "v0.3.6"}
			app.config.SkippedUpdateVersion = "v0.3.4"
			handleUpdateMessage(updateMessage{Kind: updateFailed, Err: errors.New("Access is denied")})
			if app.mode != mode("update_error") {
				t.Fatalf("mode = %s; update failure needs a readable details page", app.mode)
			}
			dispatchEvent("<Escape>")
			if app.mode != scene || app.quit || app.config.SkippedUpdateVersion != "v0.3.4" {
				t.Fatal("closing failure details did not preserve the scene/update preference")
			}
		})
	}
}

func TestUpdateCheckFailureReturnsToCurrentScene(t *testing.T) {
	initTestUIState()
	app.mode = modeHome
	app.updateState.returnMode = modeReading // A previous, unrelated update prompt.
	handleUpdateMessage(updateMessage{Kind: updateFailed, Err: errors.New("network unavailable"), Manual: true})
	dispatchEvent("<Enter>")
	if app.mode != modeHome || app.quit {
		t.Fatal("checking failure restored a stale scene")
	}
}

func TestUpdatePreparedDoesNotClaimInstallationSucceeded(t *testing.T) {
	t.Setenv("READCLI_DATA_DIR", t.TempDir())
	initTestUIState()
	app.mode = modeUpdating
	handleUpdateMessage(updateMessage{Kind: updatePrepared, Release: &lib.ReleaseInfo{TagName: "v0.3.6"}})
	if app.mode != modeUpdateRestart || !app.updateState.pendingExit {
		t.Fatal("prepared update must wait for the user to exit")
	}
	panel := buildUpdateRestartPanel()
	if strings.Contains(panel, "已安装") || strings.Contains(panel, "已完成") || !strings.Contains(panel, "更新助手会自动替换") {
		t.Fatalf("misleading preparation result: %s", panel)
	}
	dispatchEvent("<Enter>")
	if !app.quit {
		t.Fatal("Enter did not allow the waiting updater to proceed on exit")
	}
}

func TestUpdateFailureDetailsRemainReadableWhileScrolling(t *testing.T) {
	for _, size := range [][2]int{{48, 16}, {80, 24}, {140, 40}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			initTestUIState()
			app.mode = modeUpdating
			app.updateState.returnMode = modeHome
			longPath := `D:\Program Files\[red]\` + strings.Repeat(`阅读目录\`, 40) + `readcli.exe`
			recovery := `C:\Users\reader\AppData\Local\Temp\readcli-update-2403099109`
			handleUpdateMessage(updateMessage{Kind: updateFailed, Err: &lib.UpdateInstallError{
				Message: "打开程序失败：" + longPath + ": Access is denied.", TempDir: recovery,
			}})
			screen := tcell.NewSimulationScreen("UTF-8")
			if err := screen.Init(); err != nil {
				t.Fatal(err)
			}
			defer screen.Fini()
			screen.SetSize(size[0], size[1])
			root.SetRect(0, 0, size[0], size[1])
			applyLayout(size[0], size[1])
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
					line := app.updateState.errorScroll + row - 2
					seen[line] = strings.TrimSpace(text.String())
					lastLine = max(lastLine, line)
				}
				previous := app.updateState.errorScroll
				dispatchEvent("<Down>")
				if previous == app.updateState.errorScroll {
					break
				}
			}
			var rendered strings.Builder
			for line := 0; line <= lastLine; line++ {
				rendered.WriteString(seen[line])
			}
			withoutSpaces := func(s string) string {
				return strings.Map(func(r rune) rune {
					if unicode.IsSpace(r) {
						return -1
					}
					return r
				}, s)
			}
			got := withoutSpaces(rendered.String())
			for _, want := range []string{longPath, recovery, "Access is denied.", "管理员授权"} {
				if !strings.Contains(got, withoutSpaces(want)) {
					t.Fatalf("scrolling lost %q at %v:\n%s", want, size, rendered.String())
				}
			}
			dispatchEvent("g")
			if app.updateState.errorScroll != 0 {
				t.Fatal("g did not return to the start")
			}
		})
	}
}
