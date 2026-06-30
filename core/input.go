package core

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func resetInputState() {
	app.uiState.input.value = ""
	app.uiState.input.cursor = 0
	app.uiState.input.hints = nil
	app.uiState.input.hintIndex = 0
}

func insertInputText(text string) {
	runes := []rune(app.uiState.input.value)
	if app.uiState.input.cursor < 0 {
		app.uiState.input.cursor = 0
	}
	if app.uiState.input.cursor > len(runes) {
		app.uiState.input.cursor = len(runes)
	}
	insert := []rune(text)
	runes = append(runes[:app.uiState.input.cursor], append(insert, runes[app.uiState.input.cursor:]...)...)
	app.uiState.input.value = string(runes)
	app.uiState.input.cursor += len(insert)
	app.uiState.input.hints = nil
	app.uiState.input.hintIndex = 0
}

func deleteInputBackward() {
	runes := []rune(app.uiState.input.value)
	if app.uiState.input.cursor <= 0 || len(runes) == 0 {
		return
	}
	runes = append(runes[:app.uiState.input.cursor-1], runes[app.uiState.input.cursor:]...)
	app.uiState.input.value = string(runes)
	app.uiState.input.cursor--
	app.uiState.input.hints = nil
	app.uiState.input.hintIndex = 0
}

func deleteInputForward() {
	runes := []rune(app.uiState.input.value)
	if app.uiState.input.cursor < 0 || app.uiState.input.cursor >= len(runes) {
		return
	}
	runes = append(runes[:app.uiState.input.cursor], runes[app.uiState.input.cursor+1:]...)
	app.uiState.input.value = string(runes)
	app.uiState.input.hints = nil
	app.uiState.input.hintIndex = 0
}

func moveInputCursor(delta int) {
	runes := []rune(app.uiState.input.value)
	app.uiState.input.cursor += delta
	if app.uiState.input.cursor < 0 {
		app.uiState.input.cursor = 0
	}
	if app.uiState.input.cursor > len(runes) {
		app.uiState.input.cursor = len(runes)
	}
}

func setInputCursor(pos int) {
	runes := []rune(app.uiState.input.value)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	app.uiState.input.cursor = pos
}

func completeImportPath() {
	current := strings.TrimSpace(app.uiState.input.value)
	if current == "" {
		current = "."
	}

	resolved, useTilde := resolveImportPath(current)
	dir, prefix := splitImportPath(resolved)
	entries, err := os.ReadDir(dir)
	if err != nil {
		app.uiState.input.hints = nil
		app.uiState.input.hintIndex = 0
		setStatus(statusError, "无法读取目录: "+shorten(dir, 36))
		return
	}

	type match struct {
		resolved string
		display  string
	}
	var matches []match
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		resolvedMatch := filepath.Join(dir, name)
		if entry.IsDir() {
			resolvedMatch += string(os.PathSeparator)
		}
		displayMatch := renderImportPath(resolvedMatch, useTilde)
		matches = append(matches, match{resolved: resolvedMatch, display: displayMatch})
	}
	if len(matches) == 0 {
		app.uiState.input.hints = nil
		app.uiState.input.hintIndex = 0
		setStatus(statusError, "没有匹配的路径")
		return
	}

	common := matches[0].display
	for _, item := range matches[1:] {
		common = commonPathPrefix(common, item.display)
	}
	if len([]rune(common)) > len([]rune(app.uiState.input.value)) {
		app.uiState.input.value = common
		app.uiState.input.cursor = len([]rune(common))
	}

	previousHints := append([]string(nil), app.uiState.input.hints...)
	app.uiState.input.hints = nil
	for _, item := range matches {
		app.uiState.input.hints = append(app.uiState.input.hints, item.display)
	}

	if len(matches) == 1 {
		app.uiState.input.value = matches[0].display
		app.uiState.input.cursor = len([]rune(app.uiState.input.value))
		app.uiState.input.hints = nil
		app.uiState.input.hintIndex = 0
		setStatus(statusInfo, "已补全路径")
		return
	}
	if sameStringSlice(previousHints, app.uiState.input.hints) {
		app.uiState.input.hintIndex = (app.uiState.input.hintIndex + 1) % len(app.uiState.input.hints)
	} else {
		app.uiState.input.hintIndex = 0
	}
	setStatusf(statusInfo, "找到 %d 个候选", len(matches))
}

func moveInputHint(delta int) {
	if len(app.uiState.input.hints) == 0 {
		return
	}
	app.uiState.input.hintIndex += delta
	if app.uiState.input.hintIndex < 0 {
		app.uiState.input.hintIndex = len(app.uiState.input.hints) - 1
	}
	if app.uiState.input.hintIndex >= len(app.uiState.input.hints) {
		app.uiState.input.hintIndex = 0
	}
}

func importHintPageSize() int {
	if mainContentHeight > 4 {
		available := mainContentHeight - 10
		if available < 3 {
			return 3
		}
		return available
	}
	return 8
}

func importHintPageBounds(pageSize int) (start, end, page, totalPages int) {
	if pageSize < 1 {
		pageSize = 1
	}
	total := len(app.uiState.input.hints)
	if total == 0 {
		return 0, 0, 1, 1
	}
	start = (app.uiState.input.hintIndex / pageSize) * pageSize
	end = start + pageSize
	if end > total {
		end = total
	}
	page = start/pageSize + 1
	totalPages = (total + pageSize - 1) / pageSize
	return start, end, page, totalPages
}

func acceptSelectedImportHint() bool {
	if len(app.uiState.input.hints) == 0 {
		return false
	}
	if app.uiState.input.hintIndex < 0 || app.uiState.input.hintIndex >= len(app.uiState.input.hints) {
		app.uiState.input.hintIndex = 0
	}
	app.uiState.input.value = app.uiState.input.hints[app.uiState.input.hintIndex]
	app.uiState.input.cursor = len([]rune(app.uiState.input.value))
	app.uiState.input.hints = nil
	app.uiState.input.hintIndex = 0
	setStatus(statusInfo, "已填入候选路径")
	return true
}

func resolveImportPath(value string) (string, bool) {
	value = normalizeImportInputPath(value)
	if strings.HasPrefix(value, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			if value == "~" {
				value = home
			} else {
				value = filepath.Join(home, strings.TrimPrefix(value, "~"+string(os.PathSeparator)))
			}
		}
	}
	abs, err := filepath.Abs(value)
	if err == nil {
		return abs, true
	}
	return value, false
}

func normalizeImportInputPath(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			if unquoted, err := strconv.Unquote(value); err == nil {
				return unquoted
			}
			value = strings.Trim(value, "\"")
		}
		if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
			return strings.Trim(value, "'")
		}
	}

	var b strings.Builder
	escaped := false
	for _, r := range value {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		b.WriteRune(r)
	}
	if escaped {
		b.WriteRune('\\')
	}
	return b.String()
}

func renderImportPath(resolved string, useTilde bool) string {
	if !useTilde {
		return resolved
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return resolved
	}
	if resolved == home {
		return "~"
	}
	if strings.HasPrefix(resolved, home+string(os.PathSeparator)) {
		return "~" + string(os.PathSeparator) + strings.TrimPrefix(resolved, home+string(os.PathSeparator))
	}
	return resolved
}

func splitImportPath(value string) (string, string) {
	clean := value
	if strings.HasSuffix(clean, string(os.PathSeparator)) {
		return clean, ""
	}
	dir := filepath.Dir(clean)
	if dir == "." && !strings.HasPrefix(clean, ".") && !strings.HasPrefix(clean, string(os.PathSeparator)) {
		dir = "."
	}
	base := filepath.Base(clean)
	if dir == "" {
		dir = "."
	}
	return dir, base
}

func commonPathPrefix(a, b string) string {
	ar := []rune(a)
	br := []rune(b)
	n := min(len(ar), len(br))
	i := 0
	for i < n && ar[i] == br[i] {
		i++
	}
	return string(ar[:i])
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func toggleImportRecursive() {
	app.uiState.input.importRecursive = !app.uiState.input.importRecursive
	if app.uiState.input.importRecursive {
		setStatus(statusInfo, "目录导入已切换为递归子目录")
		return
	}
	setStatus(statusInfo, "目录导入已切换为仅当前层")
}

func importModeLabel() string {
	return importModeLabelFor(app.uiState.input.importRecursive)
}

func importModeLabelFor(recursive bool) string {
	if recursive {
		return "递归"
	}
	return "当前层"
}
