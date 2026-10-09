package booksource

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// SourceManagement belongs to this installation, never to a downloaded source.
// It is stored with the source so one atomic write includes rules and settings.
type SourceManagement struct {
	Standalone  bool               `json:"standalone,omitempty"`
	Favorite    bool               `json:"favorite,omitempty"`
	Tags        []string           `json:"tags,omitempty"`
	Collections []SourceCollection `json:"collections,omitempty"`
	Check       *SourceCheckResult `json:"check,omitempty"`
}

type SourceCollection struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	ImportedAt time.Time `json:"imported_at"`
}

type ImportPreview struct {
	Sources     []Source
	Added       int
	Updated     int
	Unsupported int
	Collection  SourceCollection
}

func SourceKey(source Source) string { return strings.TrimSpace(source.URL) }

// Personal preferences and display names do not change the rules being checked.
func SourceDefinitionFingerprint(source Source) string {
	source.Management, source.Enabled = nil, nil
	source.Name, source.Group, source.URL = "", "", SourceKey(source)
	data, err := json.Marshal(source)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// CloneSources preserves nil slices and copies all mutable source data. It does
// not serialize, so unknown fields survive even when their raw JSON is invalid.
func CloneSources(sources []Source) []Source {
	if sources == nil {
		return nil
	}
	result := make([]Source, len(sources))
	for i, source := range sources {
		result[i] = source
		if source.Enabled != nil {
			enabled := *source.Enabled
			result[i].Enabled = &enabled
		}
		result[i].Header = cloneRaw(source.Header)
		result[i].LoginUI = cloneRaw(source.LoginUI)
		if source.Extra != nil {
			result[i].Extra = make(map[string]json.RawMessage, len(source.Extra))
			for key, value := range source.Extra {
				result[i].Extra[key] = cloneRaw(value)
			}
		}
		result[i].Management = cloneManagement(source.Management)
	}
	return result
}
func cloneRaw(value json.RawMessage) json.RawMessage {
	if value == nil {
		return nil
	}
	return append(json.RawMessage{}, value...)
}
func cloneManagement(management *SourceManagement) *SourceManagement {
	if management == nil {
		return nil
	}
	result := *management
	if management.Tags != nil {
		result.Tags = append([]string{}, management.Tags...)
	}
	if management.Collections != nil {
		result.Collections = append([]SourceCollection{}, management.Collections...)
	}
	if management.Check != nil {
		check := *management.Check
		result.Check = &check
	}
	return &result
}

func SourceGroups(source Source) []string {
	groups := strings.FieldsFunc(source.Group, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(",，;；|｜/、", r)
	})
	if source.Management != nil {
		groups = append(groups, source.Management.Tags...)
	}
	result := make([]string, 0, len(groups))
	seen := make(map[string]bool, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group != "" && !seen[group] {
			seen[group] = true
			result = append(result, group)
		}
	}
	return result
}

// BuildImportPreview never modifies either input. Last duplicate URL wins while
// keeping its first position. Imported readcli data is deliberately discarded.
func BuildImportPreview(existing, incoming []Source, location string) ImportPreview {
	preview := ImportPreview{Sources: uniqueSources(incoming, true), Collection: collectionForLocation(location)}
	known := make(map[string]bool, len(existing))
	for _, source := range existing {
		known[SourceKey(source)] = true
		if source.Management == nil {
			continue
		}
		for _, collection := range source.Management.Collections {
			if collection.ID == preview.Collection.ID && !collection.ImportedAt.IsZero() && collection.ImportedAt.Before(preview.Collection.ImportedAt) {
				preview.Collection.ImportedAt = collection.ImportedAt
			}
		}
	}
	for _, source := range preview.Sources {
		if known[SourceKey(source)] {
			preview.Updated++
		} else {
			preview.Added++
		}
		if ValidateSource(source) != nil {
			preview.Unsupported++
		}
	}
	return preview
}

// ApplyImport imports only explicitly selected URL keys; nil selects nothing.
// Existing enablement and local metadata win over downloaded settings.
func ApplyImport(existing []Source, preview ImportPreview, selected map[string]bool, enableNew bool) []Source {
	result := uniqueSources(existing, false)
	index := make(map[string]int, len(result))
	for i, source := range result {
		index[SourceKey(source)] = i
	}
	for _, source := range uniqueSources(preview.Sources, true) {
		key := SourceKey(source)
		if !selected[key] {
			continue
		}
		if i, exists := index[key]; exists {
			independent := result[i].Management == nil || len(result[i].Management.Collections) == 0 || result[i].Management.Standalone
			changed := SourceDefinitionFingerprint(source) != SourceDefinitionFingerprint(result[i])
			source.Enabled = result[i].Enabled
			source.Management = result[i].Management
			if source.Management == nil {
				source.Management = &SourceManagement{}
			}
			source.Management.Standalone = independent
			if changed {
				source.Management.Check = nil
			}
			if ValidateSource(source) != nil {
				enabled := false
				source.Enabled = &enabled
			}
			addCollection(&source, preview.Collection)
			result[i] = source
		} else {
			enabled := enableNew && ValidateSource(source) == nil
			source.Enabled = &enabled
			addCollection(&source, preview.Collection)
			index[key] = len(result)
			result = append(result, source)
		}
	}
	return result
}

func addCollection(source *Source, collection SourceCollection) {
	if collection.ID == "" {
		return
	}
	if source.Management == nil {
		source.Management = &SourceManagement{}
	}
	for _, existing := range source.Management.Collections {
		if existing.ID == collection.ID {
			return
		}
	}
	source.Management.Collections = append(source.Management.Collections, collection)
}

// RemoveCollection removes only sources whose final collection is removed.
// Legacy sources with no collection membership are left untouched.
func RemoveCollection(sources []Source, id string) (remaining []Source, removed int, shared int) {
	cloned := CloneSources(sources)
	if id == "" {
		return cloned, 0, 0
	}
	if cloned != nil {
		remaining = make([]Source, 0, len(cloned))
	}
	for _, source := range cloned {
		if source.Management == nil || len(source.Management.Collections) == 0 {
			remaining = append(remaining, source)
			continue
		}
		kept := make([]SourceCollection, 0, len(source.Management.Collections))
		matched := false
		for _, collection := range source.Management.Collections {
			if collection.ID == id {
				matched = true
			} else {
				kept = append(kept, collection)
			}
		}
		if !matched {
			remaining = append(remaining, source)
			continue
		}
		if len(kept) == 0 && !source.Management.Standalone {
			removed++
			continue
		}
		source.Management.Collections = kept
		shared++
		remaining = append(remaining, source)
	}
	return remaining, removed, shared
}

func uniqueSources(sources []Source, imported bool) []Source {
	cloned := CloneSources(sources)
	if cloned == nil {
		return nil
	}
	result := make([]Source, 0, len(cloned))
	index := make(map[string]int, len(cloned))
	for _, source := range cloned {
		if imported {
			source.Management = nil
			removeSourceManagementFields(source.Extra)
		}
		key := SourceKey(source)
		if i, exists := index[key]; exists {
			result[i] = source
		} else {
			index[key] = len(result)
			result = append(result, source)
		}
	}
	return result
}

func removeSourceManagementFields(fields map[string]json.RawMessage) {
	// Match encoding/json's case-insensitive struct field decoding so no
	// spelling of downloaded metadata can survive import or an Extra round trip.
	for key := range fields {
		if strings.EqualFold(key, "readcli") {
			delete(fields, key)
		}
	}
}

func collectionForLocation(location string) SourceCollection {
	location = strings.TrimSpace(location)
	identity, filename := location, ""
	if parsed, err := url.Parse(location); err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" {
		parsed.Scheme = strings.ToLower(parsed.Scheme)
		parsed.Host = strings.ToLower(parsed.Host)
		parsed.Fragment = ""
		if parsed.Path == "" {
			parsed.Path = "/"
		}
		identity = parsed.String()
		filename = path.Base(parsed.Path)
	} else if location != "" {
		if absolute, err := filepath.Abs(location); err == nil {
			identity = filepath.Clean(absolute)
		}
		filename = path.Base(strings.ReplaceAll(location, "\\", "/"))
	}
	digest := sha256.Sum256([]byte(identity))
	name := "导入合集"
	// Only a bounded JSON basename becomes visible. Hosts, directories, query
	// tokens, credentials and arbitrary download endpoint names never do.
	if strings.EqualFold(path.Ext(filename), ".json") && filename != ".json" {
		safe := strings.Map(func(r rune) rune {
			if unicode.IsControl(r) || strings.ContainsRune("/\\<>:\"|?*", r) {
				return -1
			}
			return r
		}, filename)
		safe = strings.TrimSpace(safe)
		if safe != "" && len([]rune(safe)) <= 80 {
			name = safe
		}
	}
	return SourceCollection{ID: hex.EncodeToString(digest[:]), Name: name, ImportedAt: time.Now().UTC()}
}
