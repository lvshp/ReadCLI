package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/lvshp/ReadCLI/booksource"
)

const (
	modeSourceHealth      mode = "source_health"
	modeSourceHealthInput mode = "source_health_input"
)

type sourceHealthTarget struct {
	key    string
	source booksource.Source
}

type sourceHealthState struct {
	targets    []sourceHealthTarget
	results    map[string]booksource.SourceCheckResult
	index      int
	keyword    string
	deep       bool
	started    bool
	running    bool
	cancelled  bool
	detail     bool
	completed  int
	cancel     context.CancelFunc
	generation uint64
	dirty      bool
	saveFailed bool
	saveError  string
}

func isSourceHealthMode(m mode) bool { return m == modeSourceHealth || m == modeSourceHealthInput }

func openSourceHealth() {
	cancelSourceHealth()
	// Keep unsaved records reachable until a retry succeeds.
	if app.sourceHealth.dirty {
		transitionTo(modeSourceHealth)
		return
	}
	targets := make([]sourceHealthTarget, 0)
	seen := make(map[string]bool)
	for _, key := range managerTargetKeys() {
		if seen[key] {
			continue
		}
		seen[key] = true
		if source := managerSourceByKey(key); source != nil {
			copy := booksource.CloneSources([]booksource.Source{*source})[0]
			targets = append(targets, sourceHealthTarget{key: key, source: copy})
		}
	}
	if len(targets) == 0 {
		setStatus(statusInfo, "当前检查范围没有书源")
		return
	}
	generation := app.sourceHealth.generation + 1
	app.sourceHealth = sourceHealthState{targets: targets, results: make(map[string]booksource.SourceCheckResult), generation: generation}
	transitionTo(modeSourceHealth)
	setStatusf(statusInfo, "已固定 %d 个书源，请选择检查方式", len(targets))
}

func cancelSourceHealth() {
	if app == nil {
		return
	}
	h := &app.sourceHealth
	wasRunning := h.running
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
	h.generation++
	h.running = false
	if wasRunning {
		h.cancelled = true
	}
	if saveSourceHealthResults() && wasRunning {
		setStatusf(statusInfo, "已取消检查，保留已完成 %d/%d 个；其余未检查", h.completed, len(h.targets))
	}
}

func saveSourceHealthResults() bool {
	h := &app.sourceHealth
	if !h.dirty {
		return true
	}
	if !persistSources(booksource.CloneSources(app.online.sources)) {
		h.saveFailed = true
		h.saveError = app.uiState.statusMessage
		setStatus(statusError, h.saveError+"；检查结果仍在内存，按 s 重试保存")
		return false
	}
	h.dirty = false
	h.saveFailed = false
	h.saveError = ""
	return true
}

func startSourceHealth(keyword string, deep bool) {
	h := &app.sourceHealth
	if h.running || len(h.targets) == 0 {
		return
	}
	if h.dirty && !saveSourceHealthResults() {
		return
	}
	h.keyword, h.deep = strings.TrimSpace(keyword), deep
	h.results = make(map[string]booksource.SourceCheckResult, len(h.targets))
	h.completed = 0
	h.started, h.running, h.cancelled, h.detail = true, true, false, false
	h.generation++
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	owner, generation := app, h.generation
	targets := append([]sourceHealthTarget(nil), h.targets...)
	client := onlineClient()
	enqueue := onlineUpdateQueue()
	setStatusf(statusProgress, "正在检查 %d 个书源（Esc 取消，保留已完成结果）", len(targets))
	go func() {
		defer cancel()
		streamSourceHealth(ctx, targets, func(checkCtx context.Context, source booksource.Source) booksource.SourceCheckResult {
			return client.CheckSource(checkCtx, source, keyword, deep)
		}, func(target sourceHealthTarget, result booksource.SourceCheckResult) {
			enqueue(func() {
				if app != owner || app.sourceHealth.generation != generation || !app.sourceHealth.running {
					return
				}
				h := &app.sourceHealth
				h.results[target.key] = result
				h.completed++
				if source := managerSourceByKey(target.key); source != nil {
					management := booksource.SourceManagement{}
					if source.Management != nil {
						management = *source.Management
					}
					copy := result
					management.Check = &copy
					source.Management = &management
					h.dirty = true
				}
				if h.completed == len(h.targets) {
					h.running = false
					h.cancel = nil
					if saveSourceHealthResults() {
						setStatusf(statusInfo, "检查完成 %d/%d 个，结果已保存", h.completed, len(h.targets))
					}
				} else {
					setStatusf(statusProgress, "已完成检查 %d/%d 个（Esc 可取消）", h.completed, len(h.targets))
				}
				if tApp != nil {
					applyLayoutFromAppWithoutReflow()
				}
			})
		})
	}()
}

// Workers never access UI state. The single collector preserves publication
// order so a queued final result can safely persist all earlier results once.
func streamSourceHealth(ctx context.Context, targets []sourceHealthTarget, check func(context.Context, booksource.Source) booksource.SourceCheckResult, publish func(sourceHealthTarget, booksource.SourceCheckResult)) {
	type response struct {
		target sourceHealthTarget
		result booksource.SourceCheckResult
	}
	jobs := make(chan sourceHealthTarget, len(targets))
	results := make(chan response, min(4, len(targets)))
	for _, target := range targets {
		jobs <- target
	}
	close(jobs)
	for worker := 0; worker < min(4, len(targets)); worker++ {
		go func() {
			for target := range jobs {
				if ctx.Err() != nil {
					return
				}
				result := check(ctx, target.source)
				if ctx.Err() != nil {
					return
				}
				select {
				case results <- response{target: target, result: result}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	for completed := 0; completed < len(targets); completed++ {
		select {
		case <-ctx.Done():
			return
		case response := <-results:
			if ctx.Err() != nil {
				return
			}
			publish(response.target, response.result)
		}
	}
}

func handleSourceHealthEvent(id string) {
	h := &app.sourceHealth
	if app.mode == modeSourceHealthInput {
		switch id {
		case "<Escape>", "<C-c>":
			transitionTo(modeSourceHealth)
		default:
			handleTextInputEvent(id, func() {
				keyword := strings.TrimSpace(app.uiState.input.value)
				if keyword == "" {
					setStatus(statusError, "搜索与完整检查需要输入关键词")
					return
				}
				deep := h.deep
				transitionTo(modeSourceHealth)
				startSourceHealth(keyword, deep)
			})
		}
		return
	}
	switch id {
	case "<Escape>":
		if h.running {
			cancelSourceHealth()
			return
		}
		if h.detail {
			h.detail = false
			return
		}
		transitionTo(modeSources)
	case "q", "<C-c>":
		transitionTo(modeSources)
	case "j", "<Down>":
		h.index = clamp(h.index+1, 0, max(0, len(h.targets)-1))
	case "k", "<Up>":
		h.index = clamp(h.index-1, 0, max(0, len(h.targets)-1))
	case "<PageDown>":
		h.index = clamp(h.index+10, 0, max(0, len(h.targets)-1))
	case "<PageUp>":
		h.index = clamp(h.index-10, 0, max(0, len(h.targets)-1))
	case "<Home>":
		h.index = 0
	case "<End>":
		h.index = max(0, len(h.targets)-1)
	case "<Enter>":
		h.detail = !h.detail
	case "s":
		if h.running {
			setStatus(statusInfo, "检查完成或取消时会统一保存")
			return
		}
		if saveSourceHealthResults() {
			setStatus(statusInfo, "检查结果已保存")
		}
	case "1":
		if !h.running {
			startSourceHealth("", false)
		}
	case "2", "3":
		if h.running {
			return
		}
		h.deep = id == "3"
		keyword := h.keyword
		transitionTo(modeSourceHealthInput)
		app.uiState.input.value = keyword
		app.uiState.input.cursor = len([]rune(keyword))
	}
}

func sourceHealthStatusLabel(status string) string {
	switch status {
	case "ok":
		return "通过"
	case "empty":
		return "无结果"
	case "login":
		return "需登录"
	case "timeout":
		return "超时"
	case "error":
		return "错误"
	case "unsupported":
		return "暂不支持"
	case "cancelled":
		return "已取消"
	}
	return "未检查"
}
func sourceHealthStageLabel(stage string) string {
	switch stage {
	case "rules":
		return "静态规则"
	case "search":
		return "搜索"
	case "toc":
		return "详情/目录"
	case "content":
		return "正文"
	}
	return stage
}

func buildSourceHealthPanel() string {
	h := &app.sourceHealth
	if app.mode == modeSourceHealthInput {
		kind := "搜索检查"
		if h.deep {
			kind = "完整链路检查"
		}
		return fmt.Sprintf("%s\n\n本次固定检查 %d 个书源。\n输入用于搜索的书名或作者：\n\n%s\n\nEnter 开始 · Esc 返回\n\n完整链路会继续检查首本书的详情、目录和首章。\n无搜索结果单独记录，不等同于书源失效。", kind, len(h.targets), renderInputWithCursor(app.uiState.input.value, app.uiState.input.cursor))
	}
	if h.detail {
		return buildSourceHealthDetails()
	}
	state := "请选择方式"
	if h.started {
		state = "已完成"
	}
	if h.running {
		state = "检查中"
	}
	if h.cancelled {
		state = "已取消，其余未检查"
	}
	lines := []string{"书源健康检查", fmt.Sprintf("%s · 已完成 %d/%d 个（目标已固定）", state, h.completed, len(h.targets)), "1 静态规则 · 2 搜索检查 · 3 完整链路", "Enter 详情 · j/k 选择 · Esc 取消/返回 · s 重试保存", ""}
	if !h.started {
		lines = append(lines, "静态规则不访问网络；搜索和完整链路需输入关键词。", "不会自动停用或删除书源，单次结果仅供参考。", "")
	}
	if h.saveFailed {
		lines = append(lines, "结果尚未保存："+remoteLabel(h.saveError), "按 s 重试保存；已完成结果仍保留在内存。", "")
	}
	start, end := onlineListBounds(h.index, len(h.targets), 9, 2)
	for index := start; index < end; index++ {
		target := h.targets[index]
		prefix := "  "
		if index == h.index {
			prefix = "> "
		}
		status := "未检查"
		detail := ""
		if result, ok := h.results[target.key]; ok {
			status = sourceHealthStatusLabel(result.Status)
			detail = fmt.Sprintf("%s · %d ms · %s", sourceHealthStageLabel(result.Stage), result.DurationMS, result.CheckedAt.Local().Format("01-02 15:04:05"))
		}
		lines = append(lines, prefix+remoteLabel(target.source.Name)+" · "+status, "    "+detail)
	}
	return strings.Join(lines, "\n")
}

func buildSourceHealthDetails() string {
	h := &app.sourceHealth
	if len(h.targets) == 0 {
		return "没有检查目标"
	}
	target := h.targets[clamp(h.index, 0, len(h.targets)-1)]
	lines := []string{"检查详情", remoteLabel(target.source.Name), remoteLabel(target.source.URL), ""}
	result, ok := h.results[target.key]
	if !ok && target.source.Management != nil && target.source.Management.Check != nil {
		result = *target.source.Management.Check
		ok = true
		lines = append(lines, "本次未检查；以下为上次记录：")
	}
	if !ok {
		return strings.Join(append(lines, "本次未检查。", "", "j/k 切换书源 · Enter / Esc 返回列表"), "\n")
	}
	lines = append(lines, "结果："+sourceHealthStatusLabel(result.Status), "阶段："+sourceHealthStageLabel(result.Stage), fmt.Sprintf("耗时：%d ms", result.DurationMS), "时间："+result.CheckedAt.Local().Format("2006-01-02 15:04:05"), "", remoteLabel(result.Detail), "", "j/k 切换书源 · Enter / Esc 返回列表")
	return strings.Join(lines, "\n")
}

func buildSourceHealthLeftPanel() string {
	return "检查方式\n\n1 静态规则（不联网）\n2 搜索检查\n3 完整链路\n\n完整链路\n搜索 → 首本详情\n→ 目录 → 首章正文\n\n操作\nj/k 选择 · Enter 详情\nEsc 取消/返回\ns 重试保存\nq 返回书源管理\n\n最多同时检查 4 个源"
}
func buildSourceHealthRightPanel() string { return buildSourceHealthDetails() }
