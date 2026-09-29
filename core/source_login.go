package core

import (
	"context"
	"strings"

	"github.com/lvshp/ReadCLI/booksource"
	"github.com/lvshp/ReadCLI/lib"
)

func openSourceLogin() {
	s := selectedSource()
	if s == nil {
		return
	}
	transitionTo(modeSourceLogin)
	app.online.loginFields = booksource.LoginFields(*s)
	app.online.loginValues = make(map[string]string)
	app.online.loginIndex = 0
}
func clearLoginInput() {
	app.online.loginValues = nil
	app.online.loginFields = nil
	app.online.loginIndex = 0
	resetInputState()
}
func handleSourceLoginEvent(id string) {
	if id == "<Escape>" || id == "<C-c>" {
		cancelOnlineRequest()
		clearLoginInput()
		transitionTo(modeSources)
		return
	}
	if app.online.busy != "" {
		return
	}
	if len(app.online.loginFields) == 0 {
		return
	}
	switch id {
	case "<Tab>", "<Down>", "<Up>":
		field := app.online.loginFields[app.online.loginIndex]
		app.online.loginValues[field.Name] = app.uiState.input.value
		delta := 1
		if id == "<Up>" {
			delta = -1
		}
		count := len(app.online.loginFields)
		app.online.loginIndex = (app.online.loginIndex + delta + count) % count
		app.uiState.input.value = app.online.loginValues[app.online.loginFields[app.online.loginIndex].Name]
		app.uiState.input.cursor = len([]rune(app.uiState.input.value))
	case "<Enter>":
		loginSource()
	default:
		handleTextInputEvent(id, loginSource)
	}
}
func loginSource() {
	s := selectedSource()
	if s == nil || len(app.online.loginFields) == 0 {
		return
	}
	source := *s
	app.online.loginValues[app.online.loginFields[app.online.loginIndex].Name] = app.uiState.input.value
	values := make(map[string]string)
	for k, v := range app.online.loginValues {
		values[k] = v
	}
	client := onlineClient()
	clearLoginInput()
	transitionTo(modeSources)
	startOnlineRequest("正在登录 "+source.Name, func(ctx context.Context) func() {
		err := client.Login(ctx, source, values)
		for k := range values {
			delete(values, k)
		}
		if err != nil {
			return onlineError("登录失败", err)
		}
		dir, err := lib.DataDirPath()
		if err == nil {
			err = client.SaveSessions(dir)
		}
		if err != nil {
			return onlineError("保存登录会话失败", err)
		}
		return func() { setStatus(statusInfo, "登录成功，可按 / 搜索当前书源"); app.online.errors = nil }
	})
}
func logoutSource() {
	s := selectedSource()
	if s == nil {
		return
	}
	client := onlineClient()
	if err := client.Logout(*s); err != nil {
		setStatus(statusError, "退出登录失败: "+err.Error())
		return
	}
	dir, err := lib.DataDirPath()
	if err == nil {
		err = client.SaveSessions(dir)
	}
	if err != nil {
		setStatus(statusError, "保存会话失败: "+err.Error())
		return
	}
	setStatus(statusInfo, "已退出该书源的登录")
}
func buildSourceLoginPanel() string {
	lines := []string{"书源登录", "", "Tab / ↑↓ 切换字段，Enter 登录。密码输入隐藏。", "仅保存会话令牌，不保存账号密码。", ""}
	for i, f := range app.online.loginFields {
		value := app.online.loginValues[f.Name]
		if i == app.online.loginIndex {
			value = app.uiState.input.value
		}
		if f.Secret {
			value = strings.Repeat("•", len([]rune(value)))
		}
		prefix := "  "
		if i == app.online.loginIndex {
			prefix = "> "
			value = renderInputWithCursor(value, app.uiState.input.cursor)
		} else {
			value = remoteLabel(value)
		}
		lines = append(lines, prefix+remoteLabel(f.Name)+"： "+value, "")
	}
	return strings.Join(lines, "\n")
}
