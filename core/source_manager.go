package core

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lvshp/ReadCLI/booksource"
)

const (
	modeSourceFilter        mode = "source_filter"
	modeSourceGroups        mode = "source_groups"
	modeSourceActions       mode = "source_actions"
	modeSourceConfirm       mode = "source_confirm"
	modeSourceTag           mode = "source_tag"
	modeSourceImportPreview mode = "source_import_preview"
	modeSourceImportFilter  mode = "source_import_filter"
	modeSourceSearchScope   mode = "source_search_scope"
)

type sourceManagerState struct {
	query, statusFilter, group, collection, sort string
	favoriteOnly                                 bool
	selected                                     map[string]bool
	batch                                        bool
	groupIndex                                   int
	preview                                      *booksource.ImportPreview
	importSelected                               map[string]bool
	importIndex                                  int
	importEnable                                 bool
	importQuery                                  string
	pending                                      sourceManagerOperation
	undo                                         []booksource.Source
	undoLabel                                    string
	filterBefore                                 string
	searchReturn                                 mode
}

type sourceManagerOperation struct {
	kind, label, tag, collectionID string
	keys                           []string
}

type sourceManagerGroup struct {
	kind, id, label string
	count           int
}

func isSourceManagerMode(m mode) bool {
	switch m {
	case modeSources, modeSourceFilter, modeSourceGroups, modeSourceActions, modeSourceConfirm, modeSourceTag, modeSourceImportPreview, modeSourceImportFilter, modeSourceSearchScope:
		return true
	}
	return false
}

func managerSourceByKey(key string) *booksource.Source {
	for i := range app.online.sources {
		if booksource.SourceKey(app.online.sources[i]) == key {
			return &app.online.sources[i]
		}
	}
	return nil
}

func managerMeta(s booksource.Source) booksource.SourceManagement {
	if s.Management != nil {
		return *s.Management
	}
	return booksource.SourceManagement{}
}

func managerNeedsLogin(s booksource.Source) bool {
	return strings.TrimSpace(s.LoginURL) != "" || len(s.LoginUI) > 0
}

func managerMatch(s booksource.Source) bool {
	m := &app.sourceManager
	meta := managerMeta(s)
	if m.favoriteOnly && !meta.Favorite {
		return false
	}
	switch m.statusFilter {
	case "enabled":
		if !s.IsEnabled() {
			return false
		}
	case "disabled":
		if s.IsEnabled() {
			return false
		}
	case "login":
		if !managerNeedsLogin(s) {
			return false
		}
	case "issues":
		if meta.Check == nil || meta.Check.Status == "ok" || meta.Check.Status == "empty" || meta.Check.Status == "cancelled" {
			return false
		}
	}
	groups := booksource.SourceGroups(s)
	if m.group != "" && !managerContains(groups, m.group) {
		return false
	}
	if m.collection != "" {
		found := false
		for _, c := range meta.Collections {
			if c.ID == m.collection {
				found = true
				break
			}
		}
		if m.collection == "legacy" {
			found = len(meta.Collections) == 0 || meta.Standalone
		}
		if !found {
			return false
		}
	}
	haystack := strings.ToLower(s.Name + " " + strings.Join(groups, " "))
	for _, word := range strings.Fields(strings.ToLower(m.query)) {
		if !strings.Contains(haystack, word) {
			return false
		}
	}
	return true
}

func managerContains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func managerVisibleIndices() []int {
	indices := make([]int, 0, len(app.online.sources))
	for i, s := range app.online.sources {
		if managerMatch(s) {
			indices = append(indices, i)
		}
	}
	sort.SliceStable(indices, func(i, j int) bool {
		a, b := app.online.sources[indices[i]], app.online.sources[indices[j]]
		am, bm := managerMeta(a), managerMeta(b)
		switch app.sourceManager.sort {
		case "name":
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case "latency":
			av, bv := int64(1<<62), int64(1<<62)
			if am.Check != nil && am.Check.Status == "ok" && am.Check.Stage != "rules" {
				av = am.Check.DurationMS
			}
			if bm.Check != nil && bm.Check.Status == "ok" && bm.Check.Stage != "rules" {
				bv = bm.Check.DurationMS
			}
			return av < bv
		default:
			return am.Favorite && !bm.Favorite
		}
	})
	return indices
}

func managerRawIndex() int {
	indices := managerVisibleIndices()
	if len(indices) == 0 {
		app.online.sourceIndex = 0
		return -1
	}
	app.online.sourceIndex = clamp(app.online.sourceIndex, 0, len(indices)-1)
	return indices[app.online.sourceIndex]
}

func managerTargetKeys() []string {
	keys := []string{}
	for _, s := range app.online.sources {
		if app.sourceManager.selected[booksource.SourceKey(s)] {
			keys = append(keys, booksource.SourceKey(s))
		}
	}
	if len(keys) == 0 {
		for _, i := range managerVisibleIndices() {
			keys = append(keys, booksource.SourceKey(app.online.sources[i]))
		}
	}
	return keys
}

func managerSelectedCount() int {
	count := 0
	for _, s := range app.online.sources {
		if app.sourceManager.selected[booksource.SourceKey(s)] {
			count++
		}
	}
	return count
}

func managerClearSelection() { app.sourceManager.selected = make(map[string]bool) }

func managerMove(id string, index *int, count, header int) {
	step := max(1, mainContentHeight-header)
	switch id {
	case "j", "<Down>":
		*index++
	case "k", "<Up>":
		*index--
	case "<PageDown>", "<Next>", "<C-f>":
		*index += step
	case "<PageUp>", "<Prior>", "<C-b>":
		*index -= step
	case "<Home>":
		*index = 0
	case "<End>":
		*index = count - 1
	}
	*index = clamp(*index, 0, max(0, count-1))
}

func managerGroups() []sourceManagerGroup {
	options := []sourceManagerGroup{{kind: "all", label: "全部"}, {kind: "favorite", label: "常用收藏"}, {kind: "status", id: "enabled", label: "已启用"}, {kind: "status", id: "disabled", label: "已停用"}, {kind: "status", id: "login", label: "有登录配置"}, {kind: "status", id: "issues", label: "检查异常"}}
	groups, collections := map[string]int{}, map[string]sourceManagerGroup{}
	legacy := 0
	for _, s := range app.online.sources {
		meta := managerMeta(s)
		options[0].count++
		if meta.Favorite {
			options[1].count++
		}
		if s.IsEnabled() {
			options[2].count++
		} else {
			options[3].count++
		}
		if managerNeedsLogin(s) {
			options[4].count++
		}
		if meta.Check != nil && meta.Check.Status != "ok" && meta.Check.Status != "empty" && meta.Check.Status != "cancelled" {
			options[5].count++
		}
		for _, g := range booksource.SourceGroups(s) {
			groups[g]++
		}
		if len(meta.Collections) == 0 || meta.Standalone {
			legacy++
		}
		for _, c := range meta.Collections {
			o := collections[c.ID]
			o.kind = "collection"
			o.id = c.ID
			o.label = c.Name
			o.count++
			collections[c.ID] = o
		}
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		options = append(options, sourceManagerGroup{kind: "group", id: name, label: name, count: groups[name]})
	}
	ids := make([]string, 0, len(collections))
	for id := range collections {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return collections[ids[i]].label+ids[i] < collections[ids[j]].label+ids[j] })
	if legacy > 0 {
		options = append(options, sourceManagerGroup{kind: "collection", id: "legacy", label: "历史导入（未记录合集）", count: legacy})
	}
	for _, id := range ids {
		options = append(options, collections[id])
	}
	return options
}

func managerApplyGroup(o sourceManagerGroup) {
	m := &app.sourceManager
	m.group, m.collection, m.statusFilter = "", "", ""
	m.favoriteOnly = false
	switch o.kind {
	case "favorite":
		m.favoriteOnly = true
	case "status":
		m.statusFilter = o.id
	case "group":
		m.group = o.id
	case "collection":
		m.collection = o.id
	}
	app.online.sourceIndex = 0
	managerClearSelection()
	transitionTo(modeSources)
}

func managerViewLabel() string {
	m := &app.sourceManager
	labels := []string{}
	if m.favoriteOnly {
		labels = append(labels, "常用收藏")
	}
	if m.group != "" {
		labels = append(labels, "分组："+m.group)
	}
	if m.statusFilter != "" {
		for _, o := range managerGroups() {
			if o.kind == "status" && o.id == m.statusFilter {
				labels = append(labels, o.label)
				break
			}
		}
	}
	if m.collection != "" {
		for _, o := range managerGroups() {
			if o.kind == "collection" && o.id == m.collection {
				labels = append(labels, "合集："+o.label)
				break
			}
		}
	}
	if len(labels) == 0 {
		return "全部"
	}
	return strings.Join(labels, " · ")
}

func managerCommit(sources []booksource.Source, label string) bool {
	key, index := "", app.online.sourceIndex
	if selected := selectedSource(); selected != nil {
		key = booksource.SourceKey(*selected)
	}
	before := booksource.CloneSources(app.online.sources)
	if !persistSources(sources) {
		return false
	}
	app.sourceManager.undo, app.sourceManager.undoLabel = before, label
	managerClearSelection()
	indices := managerVisibleIndices()
	app.online.sourceIndex = clamp(index, 0, max(0, len(indices)-1))
	for i, raw := range indices {
		if booksource.SourceKey(app.online.sources[raw]) == key {
			app.online.sourceIndex = i
			break
		}
	}
	setStatus(statusInfo, label+"，按 u 可撤销最近一次管理操作")
	return true
}

func managerPrepare(kind, label string, keys []string) {
	if len(keys) == 0 {
		setStatus(statusError, "当前范围没有书源")
		return
	}
	app.sourceManager.pending = sourceManagerOperation{kind: kind, label: label, keys: append([]string(nil), keys...)}
	transitionTo(modeSourceConfirm)
}

func managerApplyPending() {
	op := app.sourceManager.pending
	sources := booksource.CloneSources(app.online.sources)
	keys := make(map[string]bool, len(op.keys))
	for _, k := range op.keys {
		keys[k] = true
	}
	changed, skipped := 0, 0
	if op.kind == "collection-delete" {
		var shared int
		sources, changed, shared = booksource.RemoveCollection(sources, op.collectionID)
		if managerCommit(sources, fmt.Sprintf("移除合集，删除 %d 个独有源，保留 %d 个共享/原有源", changed, shared)) {
			app.sourceManager.collection = ""
			transitionTo(modeSources)
		}
		return
	}
	kept := make([]booksource.Source, 0, len(sources))
	for _, s := range sources {
		if !keys[booksource.SourceKey(s)] {
			kept = append(kept, s)
			continue
		}
		if s.Management == nil {
			s.Management = &booksource.SourceManagement{}
		}
		switch op.kind {
		case "delete":
			changed++
			continue
		case "enable":
			if booksource.ValidateSource(s) != nil {
				skipped++
				kept = append(kept, s)
				continue
			}
			v := true
			s.Enabled = &v
		case "disable":
			v := false
			s.Enabled = &v
		case "favorite":
			s.Management.Favorite = true
		case "unfavorite":
			s.Management.Favorite = false
		case "tag-add":
			if !managerContains(s.Management.Tags, op.tag) {
				s.Management.Tags = append(s.Management.Tags, op.tag)
			}
		case "tag-remove":
			tags := []string{}
			for _, tag := range s.Management.Tags {
				if tag != op.tag {
					tags = append(tags, tag)
				}
			}
			s.Management.Tags = tags
		}
		changed++
		kept = append(kept, s)
	}
	label := fmt.Sprintf("%s %d 个书源", op.label, changed)
	if skipped > 0 {
		label += fmt.Sprintf("（跳过 %d 个不支持的源）", skipped)
	}
	if managerCommit(kept, label) {
		transitionTo(modeSources)
	}
}

func managerToggleFavorite() {
	s := selectedSource()
	if s == nil {
		return
	}
	key := booksource.SourceKey(*s)
	sources := booksource.CloneSources(app.online.sources)
	for i := range sources {
		if booksource.SourceKey(sources[i]) == key {
			if sources[i].Management == nil {
				sources[i].Management = &booksource.SourceManagement{}
			}
			sources[i].Management.Favorite = !sources[i].Management.Favorite
		}
	}
	managerCommit(sources, "已更新常用收藏")
}

func managerUndo() {
	m := &app.sourceManager
	if m.undo == nil {
		setStatus(statusInfo, "没有可撤销的管理操作")
		return
	}
	undo := booksource.CloneSources(m.undo)
	for i := range undo {
		live := managerSourceByKey(booksource.SourceKey(undo[i]))
		if live != nil && live.Management != nil && booksource.SourceDefinitionFingerprint(*live) == booksource.SourceDefinitionFingerprint(undo[i]) {
			if undo[i].Management == nil {
				undo[i].Management = &booksource.SourceManagement{}
			}
			undo[i].Management.Check = nil
			if live.Management.Check != nil {
				check := *live.Management.Check
				undo[i].Management.Check = &check
			}
		}
	}
	if persistSources(undo) {
		label := m.undoLabel
		m.undo = nil
		m.undoLabel = ""
		managerClearSelection()
		app.online.sourceIndex = 0
		setStatus(statusInfo, "已撤销："+label)
	}
}

func managerPreviewIndices() []int {
	indices := []int{}
	p := app.sourceManager.preview
	if p == nil {
		return indices
	}
	for i, s := range p.Sources {
		if strings.Contains(strings.ToLower(s.Name+" "+strings.Join(booksource.SourceGroups(s), " ")), strings.ToLower(strings.TrimSpace(app.sourceManager.importQuery))) {
			indices = append(indices, i)
		}
	}
	return indices
}

func managerOpenPreview(incoming []booksource.Source, location string) {
	p := booksource.BuildImportPreview(app.online.sources, incoming, location)
	m := &app.sourceManager
	m.preview = &p
	m.importSelected = map[string]bool{}
	m.importIndex = 0
	m.importQuery = ""
	m.importEnable = false
	for _, s := range p.Sources {
		m.importSelected[booksource.SourceKey(s)] = true
	}
	transitionTo(modeSourceImportPreview)
}

func managerConfirmImport() {
	m := &app.sourceManager
	if m.preview == nil {
		return
	}
	count := 0
	for _, s := range m.preview.Sources {
		if m.importSelected[booksource.SourceKey(s)] {
			count++
		}
	}
	if count == 0 {
		setStatus(statusError, "请至少选择一个书源")
		return
	}
	sources := booksource.ApplyImport(app.online.sources, *m.preview, m.importSelected, m.importEnable)
	if managerCommit(sources, fmt.Sprintf("已导入/更新 %d 个书源", count)) {
		m.preview = nil
		transitionTo(modeSources)
	}
}

func managerStartSearch(scope string) {
	keys := []string{}
	label := "所有已启用书源"
	switch scope {
	case "favorite":
		label = "常用收藏"
		for _, s := range app.online.sources {
			if managerMeta(s).Favorite {
				keys = append(keys, booksource.SourceKey(s))
			}
		}
	case "view":
		label = "当前筛选：" + managerViewLabel()
		for _, i := range managerVisibleIndices() {
			keys = append(keys, booksource.SourceKey(app.online.sources[i]))
		}
	case "selected":
		label = "已选书源"
		for _, s := range app.online.sources {
			if app.sourceManager.selected[booksource.SourceKey(s)] {
				keys = append(keys, booksource.SourceKey(s))
			}
		}
	}
	if scope != "all" && len(keys) == 0 {
		setStatus(statusError, "所选搜索范围没有书源")
		return
	}
	startOnlineSearch(false)
	if scope != "all" {
		app.online.scopeKeys = map[string]bool{}
		for _, key := range keys {
			app.online.scopeKeys[key] = true
		}
	}
	app.online.scopeLabel = label
}

func openSourceSearchScope() {
	app.sourceManager.searchReturn = app.mode
	transitionTo(modeSourceSearchScope)
}

func repeatScopedOnlineSearch() {
	cancelOnlineRequest()
	transitionTo(modeOnlineSearchInput)
	app.uiState.input.value = app.online.keyword
	app.uiState.input.cursor = len([]rune(app.online.keyword))
}

func handleSourceManagerEvent(id string) {
	m := &app.sourceManager
	if id == "<C-c>" {
		cancelOnlineRequest()
		transitionTo(modeHome)
		return
	}
	switch app.mode {
	case modeSourceFilter, modeSourceImportFilter, modeSourceTag:
		if id == "<Escape>" {
			if app.mode == modeSourceFilter {
				m.query = m.filterBefore
				transitionTo(modeSources)
			} else if app.mode == modeSourceImportFilter {
				m.importQuery = m.filterBefore
				transitionTo(modeSourceImportPreview)
			} else {
				transitionTo(modeSourceActions)
			}
			return
		}
		current := app.mode
		handleTextInputEvent(id, func() {
			switch current {
			case modeSourceFilter:
				transitionTo(modeSources)
			case modeSourceImportFilter:
				transitionTo(modeSourceImportPreview)
			case modeSourceTag:
				tag := strings.TrimSpace(app.uiState.input.value)
				if tag == "" {
					setStatus(statusError, "请输入标签名称")
					return
				}
				m.pending.tag = tag
				transitionTo(modeSourceConfirm)
			}
		})
		if app.mode == current {
			if current == modeSourceFilter {
				m.query = app.uiState.input.value
				app.online.sourceIndex = 0
				managerClearSelection()
			} else if current == modeSourceImportFilter {
				m.importQuery = app.uiState.input.value
				m.importIndex = 0
			}
		}
	case modeSourceGroups:
		options := managerGroups()
		managerMove(id, &m.groupIndex, len(options), 4)
		if id == "q" || id == "<Escape>" {
			transitionTo(modeSources)
			return
		}
		if len(options) == 0 {
			return
		}
		o := options[clamp(m.groupIndex, 0, len(options)-1)]
		if id == "<Enter>" {
			managerApplyGroup(o)
		}
		if id == "d" && o.kind == "collection" && o.id != "legacy" {
			keys := []string{}
			for _, s := range app.online.sources {
				for _, c := range managerMeta(s).Collections {
					if c.ID == o.id {
						keys = append(keys, booksource.SourceKey(s))
						break
					}
				}
			}
			managerPrepare("collection-delete", "移除合集："+o.label, keys)
			m.pending.collectionID = o.id
		}
	case modeSourceActions:
		if id == "q" || id == "<Escape>" {
			transitionTo(modeSources)
			return
		}
		kinds := map[string][2]string{"1": {"enable", "启用"}, "2": {"disable", "停用"}, "3": {"favorite", "收藏"}, "4": {"unfavorite", "取消收藏"}, "5": {"tag-add", "添加标签"}, "6": {"tag-remove", "移除本地标签"}, "7": {"delete", "删除"}}
		if choice, ok := kinds[id]; ok {
			managerPrepare(choice[0], choice[1], managerTargetKeys())
			if app.mode == modeSourceConfirm && (id == "5" || id == "6") {
				transitionTo(modeSourceTag)
			}
		}
	case modeSourceConfirm:
		if id == "y" || id == "<Enter>" {
			managerApplyPending()
		} else if id == "<Escape>" || id == "q" {
			transitionTo(modeSources)
		}
	case modeSourceImportPreview:
		indices := managerPreviewIndices()
		managerMove(id, &m.importIndex, len(indices), managerImportHeaderLines())
		switch id {
		case "<Escape>", "q":
			m.preview = nil
			transitionTo(modeSources)
		case "<Space>", "x":
			if len(indices) > 0 {
				s := m.preview.Sources[indices[m.importIndex]]
				key := booksource.SourceKey(s)
				m.importSelected[key] = !m.importSelected[key]
			}
		case "a":
			for _, i := range indices {
				m.importSelected[booksource.SourceKey(m.preview.Sources[i])] = true
			}
		case "A":
			m.importSelected = map[string]bool{}
		case "e":
			m.importEnable = !m.importEnable
		case "f":
			m.filterBefore = m.importQuery
			transitionTo(modeSourceImportFilter)
			app.uiState.input.value = m.importQuery
			app.uiState.input.cursor = len([]rune(m.importQuery))
		case "<Enter>":
			managerConfirmImport()
		}
	case modeSourceSearchScope:
		switch id {
		case "1":
			managerStartSearch("favorite")
		case "2":
			managerStartSearch("view")
		case "3":
			managerStartSearch("selected")
		case "4":
			managerStartSearch("all")
		case "q", "<Escape>":
			back := m.searchReturn
			if back == "" || back == modeSourceSearchScope {
				back = modeSources
			}
			transitionTo(back)
		}
	case modeSources:
		managerMove(id, &app.online.sourceIndex, len(managerVisibleIndices()), managerListHeaderLines())
		switch id {
		case "q", "<Escape>":
			if m.batch {
				m.batch = false
				managerClearSelection()
			} else if m.query != "" {
				m.query = ""
				app.online.sourceIndex = 0
				managerClearSelection()
			} else {
				cancelOnlineRequest()
				transitionTo(modeHome)
			}
		case "f":
			m.filterBefore = m.query
			transitionTo(modeSourceFilter)
			app.uiState.input.value = m.query
			app.uiState.input.cursor = len([]rune(m.query))
		case "g":
			m.groupIndex = 0
			transitionTo(modeSourceGroups)
		case "r":
			values := []string{"", "enabled", "disabled", "login", "issues"}
			for i, v := range values {
				if m.statusFilter == v {
					m.statusFilter = values[(i+1)%len(values)]
					break
				}
			}
			app.online.sourceIndex = 0
			managerClearSelection()
		case "o":
			if m.sort == "" {
				m.sort = "name"
			} else if m.sort == "name" {
				m.sort = "latency"
			} else {
				m.sort = ""
			}
			app.online.sourceIndex = 0
		case "v":
			m.batch = !m.batch
			if !m.batch {
				managerClearSelection()
			}
		case "x", "<Space>":
			if m.batch {
				if s := selectedSource(); s != nil {
					if m.selected == nil {
						managerClearSelection()
					}
					key := booksource.SourceKey(*s)
					m.selected[key] = !m.selected[key]
				}
			} else if id == "<Space>" {
				toggleSource()
			}
		case "a":
			if m.batch {
				for _, i := range managerVisibleIndices() {
					if m.selected == nil {
						managerClearSelection()
					}
					m.selected[booksource.SourceKey(app.online.sources[i])] = true
				}
			}
		case "A":
			managerClearSelection()
		case "b":
			transitionTo(modeSourceActions)
		case "*":
			managerToggleFavorite()
		case "u":
			managerUndo()
		case "h":
			openSourceHealth()
		case "s":
			openSourceSearchScope()
		case "e":
			toggleSource()
		case "d":
			if m.batch {
				managerPrepare("delete", "删除", managerTargetKeys())
			} else if s := selectedSource(); s != nil {
				managerPrepare("delete", "删除", []string{booksource.SourceKey(*s)})
			}
		case "L":
			openSourceLogin()
		case "X":
			logoutSource()
		case "P":
			openPurification()
		case "i":
			transitionTo(modeSourceImport)
		case "/", "<Enter>":
			startOnlineSearch(true)
		}
	}
}
