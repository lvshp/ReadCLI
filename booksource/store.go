package booksource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const sourceFile = "book_sources.json"
const importLimit int64 = 16 << 20

func validateSourceIdentity(source Source) error {
	if strings.TrimSpace(source.URL) == "" {
		return fmt.Errorf("缺少 bookSourceUrl")
	}
	if strings.TrimSpace(source.Name) == "" {
		return fmt.Errorf("缺少 bookSourceName")
	}
	return nil
}

func ValidateSource(source Source) error {
	if err := validateSourceIdentity(source); err != nil {
		return err
	}
	if source.Type != 0 {
		return fmt.Errorf("书源 %q 的 bookSourceType=%d 不受支持，仅支持文字书源（0）", source.Name, source.Type)
	}
	if strings.TrimSpace(source.SearchURL) == "" {
		return fmt.Errorf("书源 %q 缺少 searchUrl", source.Name)
	}
	return nil
}

// Import accepts a UTF-8 JSON file or HTTP(S) URL. Malformed entries are
// reported with their array position. Valid unsupported sources are retained
// disabled so an imported collection can round-trip without losing entries.
func Import(ctx context.Context, location string) ([]Source, error) {
	return NewClient().Import(ctx, location)
}
func (c *Client) Import(ctx context.Context, location string) ([]Source, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return nil, fmt.Errorf("书源文件或网址为空")
	}
	var data []byte
	parsed, err := url.Parse(location)
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		dataString, _, fetchErr := c.fetch(ctx, Source{}, location, location, nil, importLimit)
		if fetchErr != nil {
			return nil, fmt.Errorf("下载书源: %w", fetchErr)
		}
		data = []byte(dataString)
	} else {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		f, err := os.Open(location)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		data, err = readBounded(f, importLimit)
		if err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return decodeSourceData(data, false)
}

func decodeSources(data []byte) ([]Source, error) { return decodeSourceData(data, true) }

func decodeSourceData(data []byte, trustManagement bool) ([]Source, error) {
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))
	if len(data) == 0 {
		return nil, fmt.Errorf("书源文件为空")
	}
	var records []json.RawMessage
	switch data[0] {
	case '[':
		if err := json.Unmarshal(data, &records); err != nil {
			return nil, fmt.Errorf("书源 JSON: %w", err)
		}
	case '{':
		records = []json.RawMessage{data}
	default:
		return nil, fmt.Errorf("书源必须是 JSON 对象或数组；网页链接需要使用其 JSON 下载地址")
	}
	sources := make([]Source, 0, len(records))
	for i, record := range records {
		if !trustManagement {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(record, &fields); err != nil || fields == nil {
				return nil, fmt.Errorf("第 %d 个书源必须是 JSON 对象", i+1)
			}
			removeSourceManagementFields(fields)
			var err error
			record, err = json.Marshal(fields)
			if err != nil {
				return nil, fmt.Errorf("第 %d 个书源: %w", i+1, err)
			}
		}
		var source Source
		if err := json.Unmarshal(record, &source); err != nil {
			return nil, fmt.Errorf("第 %d 个书源: %w", i+1, err)
		}
		if err := validateSourceIdentity(source); err != nil {
			return nil, fmt.Errorf("第 %d 个书源: %w", i+1, err)
		}
		if err := ValidateSource(source); err != nil {
			disabled := false
			source.Enabled = &disabled
		}
		sources = append(sources, source)
	}
	return sources, nil
}

func LoadSources(dataDir string) ([]Source, error) {
	f, err := os.Open(filepath.Join(dataDir, sourceFile))
	if os.IsNotExist(err) {
		return []Source{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := readBounded(f, importLimit)
	if err != nil {
		return nil, err
	}
	return decodeSources(data)
}
func SaveSources(dataDir string, sources []Source) error {
	for i, source := range sources {
		if err := validateSourceIdentity(source); err != nil {
			return fmt.Errorf("第 %d 个书源: %w", i+1, err)
		}
	}
	if sources == nil {
		sources = []Source{}
	}
	data, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		return err
	}
	if int64(len(data)) > importLimit {
		return fmt.Errorf("书源配置超过 %d 字节限制", importLimit)
	}
	return atomicWrite(filepath.Join(dataDir, sourceFile), append(data, '\n'))
}

// MergeSources keeps the original API for callers without import collections.
// Downloaded rules replace matching URLs; local preferences remain authoritative.
func MergeSources(existing, incoming []Source) []Source {
	merged := uniqueSources(existing, false)
	index := make(map[string]int, len(merged))
	for i, source := range merged {
		index[SourceKey(source)] = i
	}
	for _, source := range uniqueSources(incoming, true) {
		key := SourceKey(source)
		if i, exists := index[key]; exists {
			changed := SourceDefinitionFingerprint(source) != SourceDefinitionFingerprint(merged[i])
			source.Enabled = merged[i].Enabled
			source.Management = merged[i].Management
			if changed && source.Management != nil {
				source.Management.Check = nil
			}
			if ValidateSource(source) != nil {
				enabled := false
				source.Enabled = &enabled
			}
			merged[i] = source
		} else {
			if ValidateSource(source) != nil {
				enabled := false
				source.Enabled = &enabled
			}
			index[key] = len(merged)
			merged = append(merged, source)
		}
	}
	return merged
}

func cacheBookDir(dataDir string, book Book) string {
	digest := sha256.Sum256([]byte(book.SourceURL + "\x00" + book.URL))
	return filepath.Join(dataDir, "online_cache", hex.EncodeToString(digest[:]))
}
func cacheContentPath(dataDir string, book Book, chapter Chapter) string {
	digest := sha256.Sum256([]byte(chapter.URL))
	return filepath.Join(cacheBookDir(dataDir, book), hex.EncodeToString(digest[:])+".txt")
}
func SaveCachedChapters(dataDir string, book Book, chapters []Chapter) error {
	data, err := json.Marshal(chapters)
	if err != nil {
		return err
	}
	if int64(len(data)) > importLimit {
		return fmt.Errorf("章节缓存过大")
	}
	return atomicWrite(filepath.Join(cacheBookDir(dataDir, book), "chapters.json"), data)
}
func LoadCachedChapters(dataDir string, book Book) ([]Chapter, error) {
	data, err := readBoundedFile(filepath.Join(cacheBookDir(dataDir, book), "chapters.json"), importLimit)
	if err != nil {
		return nil, err
	}
	var chapters []Chapter
	err = json.Unmarshal(data, &chapters)
	return chapters, err
}
func SaveCachedContent(dataDir string, book Book, chapter Chapter, content string) error {
	if len(content) > 64<<20 {
		return fmt.Errorf("正文缓存过大")
	}
	return atomicWrite(cacheContentPath(dataDir, book, chapter), []byte(content))
}
func LoadCachedContent(dataDir string, book Book, chapter Chapter) (string, error) {
	data, err := readBoundedFile(cacheContentPath(dataDir, book, chapter), 64<<20)
	return string(data), err
}
func readBoundedFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readBounded(f, limit)
}
func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("响应或文件超过 %d 字节限制", limit)
	}
	return data, nil
}
func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".readcli-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
