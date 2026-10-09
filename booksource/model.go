// Package booksource implements a Go reader for Legado-compatible source definitions.
package booksource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

type Source struct {
	Management   *SourceManagement          `json:"readcli,omitempty"`
	LoginURL     string                     `json:"loginUrl,omitempty"`
	LoginUI      json.RawMessage            `json:"loginUi,omitempty"`
	LoginCheckJS string                     `json:"loginCheckJs,omitempty"`
	Extra        map[string]json.RawMessage `json:"-"`
	URL          string                     `json:"bookSourceUrl"`
	Name         string                     `json:"bookSourceName"`
	Group        string                     `json:"bookSourceGroup,omitempty"`
	Type         int                        `json:"bookSourceType"`
	Enabled      *bool                      `json:"enabled,omitempty"`
	Header       json.RawMessage            `json:"header,omitempty"`
	JSLib        string                     `json:"jsLib,omitempty"`
	SearchURL    string                     `json:"searchUrl"`
	Search       BookRule                   `json:"ruleSearch"`
	BookInfo     BookRule                   `json:"ruleBookInfo"`
	TOC          TOCRule                    `json:"ruleToc"`
	Content      ContentRule                `json:"ruleContent"`
}

func (s Source) IsEnabled() bool { return s.Enabled == nil || *s.Enabled }

// Some source exports serialize individual rule objects as JSON strings.
func (s *Source) UnmarshalJSON(data []byte) error {
	type plain Source
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("书源必须是 JSON 对象")
	}
	for _, key := range []string{"ruleSearch", "ruleBookInfo", "ruleToc", "ruleContent"} {
		raw := bytes.TrimSpace(fields[key])
		if len(raw) > 0 && raw[0] == '"' {
			var nested string
			if err := json.Unmarshal(raw, &nested); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			if nested == "" {
				delete(fields, key)
				continue
			}
			fields[key] = json.RawMessage(nested)
		}
	}
	normalized, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("书源规则 JSON: %w", err)
	}
	var decoded plain
	if err := json.Unmarshal(normalized, &decoded); err != nil {
		return err
	}
	*s = Source(decoded)
	s.Extra = fields
	return nil
}

func (s Source) MarshalJSON() ([]byte, error) {
	type plain Source
	data, err := json.Marshal(plain(s))
	if err != nil {
		return nil, err
	}
	var known map[string]json.RawMessage
	if err := json.Unmarshal(data, &known); err != nil {
		return nil, err
	}
	merged := make(map[string]json.RawMessage, len(s.Extra)+len(known))
	for key, value := range s.Extra {
		merged[key] = value
	}
	// Extra retains unknown source fields only at serialization time. Remove
	// every recognized field first, including omitted zero values, so clearing
	// local metadata or an optional rule cannot revive its previously loaded value.
	removeKnownJSONFields(merged, plain{})
	for key, value := range known {
		if key == "ruleSearch" || key == "ruleBookInfo" || key == "ruleToc" || key == "ruleContent" {
			var original, changed map[string]json.RawMessage
			if json.Unmarshal(s.Extra[key], &original) == nil && original != nil && json.Unmarshal(value, &changed) == nil {
				switch key {
				case "ruleSearch", "ruleBookInfo":
					removeKnownJSONFields(original, BookRule{})
				case "ruleToc":
					removeKnownJSONFields(original, TOCRule{})
				case "ruleContent":
					removeKnownJSONFields(original, ContentRule{})
				}
				for field, v := range changed {
					original[field] = v
				}
				value, err = json.Marshal(original)
				if err != nil {
					return nil, err
				}
			}
		}
		merged[key] = value
	}
	return json.Marshal(merged)
}

func removeKnownJSONFields(fields map[string]json.RawMessage, shape any) {
	typ := reflect.TypeOf(shape)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		delete(fields, name)
	}
}

type BookRule struct {
	Init        string `json:"init,omitempty"`
	BookList    string `json:"bookList,omitempty"`
	Name        string `json:"name,omitempty"`
	Author      string `json:"author,omitempty"`
	BookURL     string `json:"bookUrl,omitempty"`
	TOCURL      string `json:"tocUrl,omitempty"`
	CoverURL    string `json:"coverUrl,omitempty"`
	Intro       string `json:"intro,omitempty"`
	Kind        string `json:"kind,omitempty"`
	LastChapter string `json:"lastChapter,omitempty"`
}
type TOCRule struct {
	ChapterList string `json:"chapterList"`
	ChapterName string `json:"chapterName"`
	ChapterURL  string `json:"chapterUrl"`
	NextTOCURL  string `json:"nextTocUrl,omitempty"`
	IsVolume    string `json:"isVolume,omitempty"`
}
type ContentRule struct {
	Content        string `json:"content"`
	NextContentURL string `json:"nextContentUrl,omitempty"`
	ReplaceRegex   string `json:"replaceRegex,omitempty"`
}
type Book struct {
	Variables   map[string]any `json:"variables,omitempty"`
	SourceURL   string         `json:"source_url"`
	SourceName  string         `json:"source_name"`
	URL         string         `json:"url"`
	TOCURL      string         `json:"toc_url,omitempty"`
	Name        string         `json:"name"`
	Author      string         `json:"author,omitempty"`
	Intro       string         `json:"intro,omitempty"`
	CoverURL    string         `json:"cover_url,omitempty"`
	Kind        string         `json:"kind,omitempty"`
	LastChapter string         `json:"last_chapter,omitempty"`
}
type Chapter struct {
	NextURL string `json:"next_url,omitempty"`
	Name    string `json:"name"`
	URL     string `json:"url"`
}
