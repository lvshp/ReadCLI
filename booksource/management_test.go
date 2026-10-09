package booksource

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func managementFixture(key, name string, enabled bool) Source {
	return Source{URL: key, Name: name, Enabled: &enabled, SearchURL: "/search", Search: BookRule{BookList: "tag.li", Name: "text", BookURL: "tag.a@href"}}
}
func managementJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestImportPreviewPreservesInputsAndCountsDistinctURLs(t *testing.T) {
	old := managementFixture("https://a.test", "原名称", false)
	old.Management = &SourceManagement{Favorite: true, Tags: []string{"本地标签"}, Check: &SourceCheckResult{Status: "ok", Detail: "旧检查"}}
	updated := managementFixture(" https://a.test ", "第一次更新", true)
	updated.Management = &SourceManagement{Tags: []string{"不可信标签"}, Collections: []SourceCollection{{ID: "injected"}}}
	updated.Extra = map[string]json.RawMessage{"readcli": json.RawMessage(`{"favorite":true}`)}
	second := managementFixture("https://b.test", "同名书源", true)
	final := updated
	final.Name = "最后一次更新"
	unsupported := managementFixture("https://c.test", "漫画", true)
	unsupported.Type = 2
	existing, incoming := []Source{old}, []Source{updated, second, final, unsupported}
	beforeExisting, beforeIncoming := managementJSON(t, existing), managementJSON(t, incoming)
	preview := BuildImportPreview(existing, incoming, "https://download.test/sources.json?token=sensitive-token")
	if preview.Added != 2 || preview.Updated != 1 || preview.Unsupported != 1 || len(preview.Sources) != 3 {
		t.Fatalf("preview counts: %+v", preview)
	}
	if preview.Sources[0].Name != "最后一次更新" {
		t.Fatalf("duplicate URL did not keep latest rules: %+v", preview.Sources)
	}
	for _, source := range preview.Sources {
		if source.Management != nil || source.Extra["readcli"] != nil {
			t.Fatal("downloaded management metadata accepted")
		}
	}
	if !bytes.Equal(beforeExisting, managementJSON(t, existing)) || !bytes.Equal(beforeIncoming, managementJSON(t, incoming)) {
		t.Fatal("preview changed input")
	}
	*preview.Sources[0].Enabled = false
	if !*incoming[0].Enabled || !*incoming[2].Enabled {
		t.Fatal("preview retained enabled pointer")
	}
	if preview.Collection.Name != "sources.json" || preview.Collection.ID == "" || preview.Collection.ImportedAt.IsZero() {
		t.Fatalf("collection: %+v", preview.Collection)
	}
}

func TestApplyImportPreservesSettingsAndSupportsSelection(t *testing.T) {
	old := managementFixture("https://a.test", "同名", false)
	earlier := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)
	old.Management = &SourceManagement{Favorite: true, Tags: []string{"自建"}, Collections: []SourceCollection{{ID: "old", Name: "原合集", ImportedAt: earlier}}, Check: &SourceCheckResult{Status: "ok", Detail: "保留检查", CheckedAt: earlier}}
	incoming := []Source{managementFixture(old.URL, "更新", true), managementFixture("https://b.test", "同名", true), managementFixture("https://unsupported.test", "非文字", true)}
	incoming[2].Type = 1
	preview := BuildImportPreview([]Source{old}, incoming, "/tmp/source-pack.json")
	inputBefore, previewBefore := managementJSON(t, []Source{old}), managementJSON(t, preview)
	result := ApplyImport([]Source{old}, preview, map[string]bool{old.URL: true, "https://b.test": true, "https://unsupported.test": true}, true)
	if len(result) != 3 || result[0].Name != "更新" || result[0].IsEnabled() || !result[1].IsEnabled() || result[2].IsEnabled() {
		t.Fatalf("import settings: %+v", result)
	}
	if !result[0].Management.Favorite || !reflect.DeepEqual(result[0].Management.Tags, []string{"自建"}) || result[0].Management.Check.Detail != "保留检查" || len(result[0].Management.Collections) != 2 {
		t.Fatalf("local metadata lost: %+v", result[0].Management)
	}
	if result[1].Management.Favorite || len(result[1].Management.Collections) != 1 {
		t.Fatalf("new metadata: %+v", result[1].Management)
	}
	if !bytes.Equal(inputBefore, managementJSON(t, []Source{old})) || !bytes.Equal(previewBefore, managementJSON(t, preview)) {
		t.Fatal("apply mutated inputs")
	}
	result[0].Management.Tags[0] = "修改副本"
	result[0].Management.Check.Detail = "修改副本"
	if old.Management.Tags[0] != "自建" || old.Management.Check.Detail != "保留检查" {
		t.Fatal("apply retained management aliases")
	}
	none := ApplyImport([]Source{old}, preview, nil, true)
	if len(none) != 1 || none[0].Name != old.Name {
		t.Fatalf("nil selection imported sources: %+v", none)
	}
	onlyNew := ApplyImport(nil, preview, map[string]bool{"https://b.test": true}, false)
	if len(onlyNew) != 1 || onlyNew[0].IsEnabled() {
		t.Fatalf("new source enable preference ignored: %+v", onlyNew)
	}
}

func TestRepeatedImportAndLegacyMergePreserveLocalState(t *testing.T) {
	source := managementFixture("https://repeat.test", "原始", true)
	location := "https://download.test/all.json?dataset=1&token=secret"
	preview := BuildImportPreview(nil, []Source{source}, location)
	first := ApplyImport(nil, preview, map[string]bool{source.URL: true}, true)
	*first[0].Enabled = false
	first[0].Management.Favorite = true
	first[0].Management.Tags = []string{"收藏分组"}
	importedAt := first[0].Management.Collections[0].ImportedAt
	incoming := source
	incoming.Name = "上游新版"
	incoming.Management = &SourceManagement{Tags: []string{"伪造"}}
	again := BuildImportPreview(first, []Source{incoming}, location)
	second := ApplyImport(first, again, map[string]bool{source.URL: true}, true)
	if second[0].IsEnabled() || !second[0].Management.Favorite || second[0].Management.Tags[0] != "收藏分组" || second[0].Name != "上游新版" {
		t.Fatalf("repeat lost settings: %+v", second[0])
	}
	if len(second[0].Management.Collections) != 1 || !second[0].Management.Collections[0].ImportedAt.Equal(importedAt) || again.Collection.ID != preview.Collection.ID {
		t.Fatal("repeat duplicated or replaced collection identity")
	}
	legacy := MergeSources(first, []Source{incoming, managementFixture("https://same-name.test", incoming.Name, true)})
	if len(legacy) != 2 || legacy[0].IsEnabled() || !legacy[0].Management.Favorite || legacy[0].Management.Tags[0] != "收藏分组" {
		t.Fatalf("legacy merge lost settings or merged names: %+v", legacy)
	}
	legacy[0].Management.Tags[0] = "副本"
	if first[0].Management.Tags[0] != "收藏分组" {
		t.Fatal("legacy merge aliases existing")
	}
	nilEnabled := source
	nilEnabled.Enabled = nil
	preserved := MergeSources([]Source{nilEnabled}, []Source{managementFixture(source.URL, "更新", false)})
	if preserved[0].Enabled != nil {
		t.Fatal("implicit enabled state was replaced by imported state")
	}
	incoming.Type = 1
	disabled := MergeSources(first, []Source{incoming})
	if disabled[0].IsEnabled() {
		t.Fatal("unsupported updated source enabled")
	}
}

func TestRemoveCollectionKeepsSharedAndLegacySources(t *testing.T) {
	a, b := SourceCollection{ID: "a", Name: "A"}, SourceCollection{ID: "b", Name: "B"}
	shared := managementFixture("https://shared.test", "共享", true)
	shared.Management = &SourceManagement{Favorite: true, Tags: []string{"标签"}, Collections: []SourceCollection{a, b}}
	exclusive := managementFixture("https://exclusive.test", "独占", true)
	exclusive.Management = &SourceManagement{Collections: []SourceCollection{a}}
	legacy := managementFixture("https://legacy.test", "旧源", true)
	tagged := managementFixture("https://tagged.test", "自建标签", true)
	tagged.Management = &SourceManagement{Tags: []string{"手工"}}
	input := []Source{shared, exclusive, legacy, tagged}
	before := managementJSON(t, input)
	result, removed, keptShared := RemoveCollection(input, a.ID)
	if removed != 1 || keptShared != 1 || len(result) != 3 || result[0].URL != shared.URL || result[1].URL != legacy.URL || result[2].URL != tagged.URL {
		t.Fatalf("remove: removed=%d shared=%d sources=%+v", removed, keptShared, result)
	}
	if len(result[0].Management.Collections) != 1 || result[0].Management.Collections[0].ID != "b" || !result[0].Management.Favorite {
		t.Fatal("shared source lost metadata")
	}
	if !bytes.Equal(before, managementJSON(t, input)) {
		t.Fatal("remove mutated input")
	}
	result[0].Management.Tags[0] = "副本"
	if shared.Management.Tags[0] != "标签" {
		t.Fatal("remove aliases input")
	}
	untouched, removed, keptShared := RemoveCollection(input, "missing")
	if removed != 0 || keptShared != 0 || !bytes.Equal(before, managementJSON(t, untouched)) {
		t.Fatal("missing collection altered sources")
	}
	result, removed, keptShared = RemoveCollection(result, b.ID)
	if removed != 1 || keptShared != 0 || len(result) != 2 {
		t.Fatal("last collection did not remove source")
	}
}

func TestCloneSourcesCopiesAllMutableFields(t *testing.T) {
	source := managementFixture("https://clone.test", "源", true)
	source.Header = json.RawMessage(`{"X":"value"}`)
	source.LoginUI = json.RawMessage(`[{"name":"邮箱"}]`)
	source.Extra = map[string]json.RawMessage{"unknown": json.RawMessage(`{"a":1}`)}
	source.Management = &SourceManagement{Favorite: true, Tags: []string{"标签"}, Collections: []SourceCollection{{ID: "a", Name: "合集"}}, Check: &SourceCheckResult{Detail: "检查"}}
	copy := CloneSources([]Source{source})
	*copy[0].Enabled = false
	copy[0].Header[0] = '['
	copy[0].LoginUI[0] = '{'
	copy[0].Extra["unknown"][0] = '['
	copy[0].Extra["new"] = json.RawMessage(`true`)
	copy[0].Management.Favorite = false
	copy[0].Management.Tags[0] = "别名"
	copy[0].Management.Collections[0].Name = "别名"
	copy[0].Management.Check.Detail = "别名"
	if !source.IsEnabled() || source.Header[0] != '{' || source.LoginUI[0] != '[' || source.Extra["unknown"][0] != '{' || source.Extra["new"] != nil || !source.Management.Favorite || source.Management.Tags[0] != "标签" || source.Management.Collections[0].Name != "合集" || source.Management.Check.Detail != "检查" {
		t.Fatal("clone retained mutable aliases")
	}
	if CloneSources(nil) != nil {
		t.Fatal("clone changed nil slice")
	}
}

func TestSourceGroupsCombinesOriginalAndLocalTags(t *testing.T) {
	source := Source{Group: " 精品,玄幻，都市;小说；完结|免费｜通用/其他、默认\n精品\t玄幻 ", Management: &SourceManagement{Tags: []string{"我的标签", " 精品 ", "带 空格标签", ""}}}
	expected := []string{"精品", "玄幻", "都市", "小说", "完结", "免费", "通用", "其他", "默认", "我的标签", "带 空格标签"}
	if groups := SourceGroups(source); !reflect.DeepEqual(groups, expected) {
		t.Fatalf("groups: %#v", groups)
	}
}

func TestImportLegacyOwnershipAndCheckFingerprint(t *testing.T) {
	old := managementFixture("https://legacy.test", "已有源", true)
	old.Management = &SourceManagement{Favorite: true, Check: &SourceCheckResult{Status: "ok", Detail: "原规则检查"}}
	newSource := managementFixture("https://new.test", "新源", true)
	preview := BuildImportPreview([]Source{old}, []Source{old, newSource}, "pack.json")
	merged := ApplyImport([]Source{old}, preview, map[string]bool{old.URL: true, newSource.URL: true}, true)
	if !merged[0].Management.Standalone || merged[1].Management.Standalone {
		t.Fatal("original independent ownership lost")
	}
	remaining, removed, retained := RemoveCollection(merged, preview.Collection.ID)
	if len(remaining) != 1 || remaining[0].URL != old.URL || removed != 1 || retained != 1 || len(remaining[0].Management.Collections) != 0 {
		t.Fatal("removing new collection deleted pre-existing source")
	}
	if remaining[0].Management.Check == nil {
		t.Fatal("identical reimport lost check")
	}
	updated := old
	updated.SearchURL = "/new-rules"
	preview = BuildImportPreview([]Source{old}, []Source{updated}, "pack.json")
	changed := ApplyImport([]Source{old}, preview, map[string]bool{old.URL: true}, true)
	if changed[0].Management.Check != nil {
		t.Fatal("changed rules retained stale check")
	}
	if old.Management.Check == nil {
		t.Fatal("import mutated input check")
	}
	cosmetic := CloneSources([]Source{old})[0]
	cosmetic.Name = "更名"
	cosmetic.Group = "新分组"
	cosmetic.Management.Favorite = false
	off := false
	cosmetic.Enabled = &off
	if SourceDefinitionFingerprint(old) != SourceDefinitionFingerprint(cosmetic) {
		t.Fatal("preferences altered rule fingerprint")
	}
	cosmetic.Extra = map[string]json.RawMessage{"customRule": json.RawMessage(`"changed"`)}
	if SourceDefinitionFingerprint(old) == SourceDefinitionFingerprint(cosmetic) {
		t.Fatal("unknown rule fields omitted from fingerprint")
	}
}

func TestCollectionIdentityNeverPersistsDownloadSecrets(t *testing.T) {
	input := []Source{managementFixture("https://source.test", "源", true)}
	first := BuildImportPreview(nil, input, "https://private-user:private-password@download.test/private-folder/list.json?token=private-token#fragment")
	encoded := string(managementJSON(t, first.Collection))
	for _, secret := range []string{"private-user", "private-password", "download.test", "private-folder", "private-token", "fragment"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("collection persisted location secret %q: %s", secret, encoded)
		}
	}
	if first.Collection.Name != "list.json" {
		t.Fatalf("unsafe display name %q", first.Collection.Name)
	}
	same := BuildImportPreview(nil, input, "https://private-user:private-password@download.test/private-folder/list.json?token=private-token#other-fragment")
	if first.Collection.ID != same.Collection.ID {
		t.Fatal("URL fragment changed download identity")
	}
	a := BuildImportPreview(nil, input, "https://download.test/get?dataset=A")
	b := BuildImportPreview(nil, input, "https://download.test/get?dataset=B")
	if a.Collection.ID == b.Collection.ID || a.Collection.Name != "导入合集" {
		t.Fatal("different query datasets collapsed or endpoint leaked")
	}
	local := BuildImportPreview(nil, input, "/private/user-token/favorites.json")
	if local.Collection.Name != "favorites.json" || strings.Contains(string(managementJSON(t, local.Collection)), "user-token") {
		t.Fatal("local private path leaked")
	}
}

func TestSourceManagementOldArraysBackupAndClearRoundTrip(t *testing.T) {
	original := t.TempDir()
	backup := t.TempDir()
	restored := t.TempDir()
	legacy := `[{"bookSourceUrl":"https://legacy.test","bookSourceName":"旧源","searchUrl":"/s","enabled":false,"header":{"X":"old"},"jsLib":"old()","custom":{"keep":true},"ruleContent":{"content":"tag.p@text","nextContentUrl":"tag.a@href","customRule":true}}]`
	if err := os.WriteFile(filepath.Join(original, sourceFile), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	sources, err := LoadSources(original)
	if err != nil || len(sources) != 1 || sources[0].Management != nil || sources[0].IsEnabled() {
		t.Fatalf("old array: %+v %v", sources, err)
	}
	now := time.Date(2026, 10, 8, 1, 2, 3, 0, time.UTC)
	sources[0].Management = &SourceManagement{Favorite: true, Tags: []string{"标签"}, Collections: []SourceCollection{{ID: "collection", Name: "合集", ImportedAt: now}}, Check: &SourceCheckResult{Status: "ok", Stage: "search", Detail: "正常", CheckedAt: now, DurationMS: 123}}
	if err := SaveSources(original, sources); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(original, sourceFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || data[0] != '[' {
		t.Fatal("storage no longer uses source array")
	}
	files, err := os.ReadDir(original)
	if err != nil || len(files) != 1 {
		t.Fatal("metadata introduced a second storage file")
	}
	// Copying the saved directory's source file is sufficient to restore all
	// management data, with no hidden metadata database or path binding.
	if err := os.WriteFile(filepath.Join(backup, sourceFile), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveSources(original, nil); err != nil {
		t.Fatal(err)
	}
	copied, err := os.ReadFile(filepath.Join(backup, sourceFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(restored, sourceFile), copied, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSources(restored)
	if err != nil || len(loaded) != 1 {
		t.Fatalf("restore: %+v %v", loaded, err)
	}
	if !reflect.DeepEqual(loaded[0].Management, sources[0].Management) || loaded[0].IsEnabled() {
		t.Fatalf("management restore: %+v", loaded[0])
	}
	loaded[0].Management = nil
	loaded[0].Enabled = nil
	loaded[0].Header = nil
	loaded[0].JSLib = ""
	loaded[0].Content.NextContentURL = ""
	if err := SaveSources(restored, loaded); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(restored, sourceFile))
	if err != nil {
		t.Fatal(err)
	}
	var serialized []map[string]json.RawMessage
	if err := json.Unmarshal(data, &serialized); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"readcli", "enabled", "header", "jsLib"} {
		if serialized[0][key] != nil {
			t.Fatalf("cleared field %q revived from Extra", key)
		}
	}
	var content map[string]json.RawMessage
	if err := json.Unmarshal(serialized[0]["ruleContent"], &content); err != nil {
		t.Fatal(err)
	}
	if content["nextContentUrl"] != nil || content["customRule"] == nil || serialized[0]["custom"] == nil {
		t.Fatal("optional clearing or unknown rule preservation failed")
	}
}

func TestImportDiscardsCaseInsensitiveLocalMetadata(t *testing.T) {
	for _, key := range []string{"readcli", "ReadCLI", "READCLI", "rEaDcLi"} {
		for name, raw := range untrustedManagementValues() {
			t.Run(key+"/"+name, func(t *testing.T) {
				incoming := managementFixture("https://source.test", "源", true)
				incoming.Search = BookRule{}
				incoming.Extra = map[string]json.RawMessage{"custom": json.RawMessage(`true`)}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(managementJSON(t, incoming), &fields); err != nil {
					t.Fatal(err)
				}
				fields[key] = raw
				file := filepath.Join(t.TempDir(), "untrusted.json")
				if err := os.WriteFile(file, managementJSON(t, fields), 0600); err != nil {
					t.Fatal(err)
				}
				sources, err := Import(context.Background(), file)
				if err != nil || len(sources) != 1 {
					t.Fatalf("import metadata: %+v %v", sources, err)
				}
				assertNoImportedManagement(t, sources[0])
				assertImportedManagementRoundTrips(t, incoming, sources)
			})
		}
	}
}

func TestImportPreviewDiscardsCaseInsensitiveExtraMetadata(t *testing.T) {
	for _, key := range []string{"readcli", "ReadCLI", "READCLI", "rEaDcLi"} {
		for name, raw := range untrustedManagementValues() {
			t.Run(key+"/"+name, func(t *testing.T) {
				incoming := managementFixture("https://source.test", "源", true)
				incoming.Search = BookRule{}
				incoming.Extra = map[string]json.RawMessage{key: raw, "custom": json.RawMessage(`true`)}
				incoming.Management = &SourceManagement{Tags: []string{"上游标签"}, Check: &SourceCheckResult{Detail: "上游检查"}}
				before := CloneSources([]Source{incoming})
				preview := BuildImportPreview(nil, []Source{incoming}, "untrusted.json")
				if len(preview.Sources) != 1 {
					t.Fatalf("preview: %+v", preview)
				}
				assertNoImportedManagement(t, preview.Sources[0])
				if !reflect.DeepEqual([]Source{incoming}, before) {
					t.Fatal("preview mutated incoming metadata")
				}
				assertImportedManagementRoundTrips(t, incoming, preview.Sources)
			})
		}
	}
}

func untrustedManagementValues() map[string]json.RawMessage {
	return map[string]json.RawMessage{
		"object":  json.RawMessage(`{"favorite":false,"tags":["上游标签"],"check":{"status":"failed","detail":"上游检查"}}`),
		"string":  json.RawMessage(`"untrusted-invalid-metadata"`),
		"array":   json.RawMessage(`[]`),
		"boolean": json.RawMessage(`true`),
		"number":  json.RawMessage(`123`),
	}
}

func assertNoImportedManagement(t *testing.T, source Source) {
	t.Helper()
	if source.Management != nil {
		t.Errorf("downloaded management accepted: %+v", source.Management)
	}
	for key := range source.Extra {
		if strings.EqualFold(key, "readcli") {
			t.Errorf("downloaded management retained in Extra as %q", key)
		}
	}
	if string(source.Extra["custom"]) != `true` {
		t.Error("discarding management lost an unknown source field")
	}
}

func assertImportedManagementRoundTrips(t *testing.T, incoming Source, sanitized []Source) {
	t.Helper()
	local := managementFixture(incoming.URL, incoming.Name, false)
	local.Search = BookRule{}
	local.Extra = map[string]json.RawMessage{"custom": json.RawMessage(`true`)}
	local.Management = &SourceManagement{Favorite: true, Tags: []string{"本地标签"}, Check: &SourceCheckResult{Status: "ok", Detail: "本地检查"}}
	preview := BuildImportPreview([]Source{local}, sanitized, "untrusted.json")
	results := map[string][]Source{
		"new":   ApplyImport(nil, preview, map[string]bool{local.URL: true}, true),
		"apply": ApplyImport([]Source{local}, preview, map[string]bool{local.URL: true}, true),
		"merge": MergeSources([]Source{local}, []Source{incoming}),
	}
	for name, sources := range results {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := SaveSources(dir, sources); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadSources(dir)
			if err != nil || len(loaded) != 1 {
				t.Fatalf("reload imported metadata: %+v %v", loaded, err)
			}
			for key := range loaded[0].Extra {
				if key != "readcli" && strings.EqualFold(key, "readcli") {
					t.Errorf("saved source retained downloaded metadata key %q", key)
				}
			}
			if string(loaded[0].Extra["custom"]) != `true` {
				t.Error("save/reload lost an unknown source field")
			}
			management := loaded[0].Management
			if name == "new" {
				if management == nil || management.Favorite || len(management.Tags) != 0 || management.Check != nil {
					t.Fatalf("new source accepted downloaded preferences after reload: %+v", management)
				}
				return
			}
			if loaded[0].IsEnabled() || management == nil || !management.Favorite || !reflect.DeepEqual(management.Tags, local.Management.Tags) || !reflect.DeepEqual(management.Check, local.Management.Check) {
				t.Fatalf("local preferences changed after reload: enabled=%v management=%+v", loaded[0].IsEnabled(), management)
			}
		})
	}
}
