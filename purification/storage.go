package purification

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

const ruleFileLimit int64 = 16 << 20

// FileInfo identifies a rule file that can be edited directly. Local is true
// only for main.json, which runs after every imported group.
type FileInfo struct {
	Path         string
	Name         string
	Local        bool
	RuleCount    int
	EnabledCount int
}

func ruleDirectory(dataDir string) string {
	return filepath.Join(dataDir, "replace_rules")
}

// Ensure initializes a separate local rule file without replacing user edits.
func Ensure(dataDir string) error {
	dir := ruleDirectory(dataDir)
	if err := os.MkdirAll(filepath.Join(dir, "imports"), 0700); err != nil {
		return fmt.Errorf("初始化净化规则目录 %s: %w", dir, err)
	}
	mainPath := filepath.Join(dir, "main.json")
	if _, err := os.Lstat(mainPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("读取本地净化规则 %s: %w", mainPath, err)
	}
	temp, err := writeRuleTemp(context.Background(), dir, []byte("[]\n"))
	if err != nil {
		return fmt.Errorf("初始化本地净化规则 %s: %w", mainPath, err)
	}
	defer os.Remove(temp)
	// Linking an already-complete temporary file is exclusive: another Ensure
	// or a user's newly-created main.json must never be overwritten.
	if err := os.Link(temp, mainPath); err != nil && !os.IsExist(err) {
		return fmt.Errorf("初始化本地净化规则 %s: %w", mainPath, err)
	}
	return nil
}

// Load reads the current files on every call. It never initializes or rewrites
// files, so replacing a backup directory also replaces the active definitions.
// Imported groups run in filename order; each group's order is stable, and the
// local group is appended last regardless of its individual order values.
func Load(dataDir string) ([]Rule, []FileInfo, error) {
	dir := ruleDirectory(dataDir)
	entries, err := os.ReadDir(filepath.Join(dir, "imports"))
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("读取导入净化规则目录 %s: %w", filepath.Join(dir, "imports"), err)
	}
	paths := make([]string, 0, len(entries)+1)
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			paths = append(paths, filepath.Join(dir, "imports", entry.Name()))
		}
	}
	sort.Strings(paths)
	mainPath := filepath.Join(dir, "main.json")
	if _, err := os.Lstat(mainPath); err == nil {
		paths = append(paths, mainPath)
	} else if !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("读取本地净化规则 %s: %w", mainPath, err)
	}
	rules := make([]Rule, 0)
	files := make([]FileInfo, 0, len(paths))
	for _, filePath := range paths {
		data, err := readRuleFile(context.Background(), filePath)
		if err != nil {
			return nil, files, err
		}
		group, err := decodeRuleFile(data, filePath)
		if err != nil {
			return nil, files, err
		}
		sort.SliceStable(group, func(i, j int) bool { return group[i].Order < group[j].Order })
		rules = append(rules, group...)
		files = append(files, describeRuleFile(filePath, filePath == mainPath, group))
	}
	return rules, files, nil
}

// Import validates a local JSON file or HTTP(S) response before atomically
// replacing that source's own file. Original JSON bytes, including fields the
// engine does not recognize, are retained for future edits and round trips.
func Import(ctx context.Context, dataDir, location string) (FileInfo, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return FileInfo{}, fmt.Errorf("净化规则文件或网址为空")
	}
	if err := ctx.Err(); err != nil {
		return FileInfo{}, err
	}
	data, identity, name, err := readRuleImport(ctx, location)
	if err != nil {
		return FileInfo{}, err
	}
	rules, err := decodeRuleFile(data, location)
	if err != nil {
		return FileInfo{}, err
	}
	if err := ctx.Err(); err != nil {
		return FileInfo{}, err
	}
	if err := Ensure(dataDir); err != nil {
		return FileInfo{}, err
	}
	digest := sha256.Sum256([]byte(identity))
	filePath := filepath.Join(ruleDirectory(dataDir), "imports", importRuleName(name)+"-"+hex.EncodeToString(digest[:])+".json")
	temp, err := writeRuleTemp(ctx, filepath.Dir(filePath), data)
	if err != nil {
		return FileInfo{}, fmt.Errorf("写入净化规则 %s: %w", filePath, err)
	}
	defer os.Remove(temp)
	if err := ctx.Err(); err != nil {
		return FileInfo{}, err
	}
	if err := os.Rename(temp, filePath); err != nil {
		return FileInfo{}, fmt.Errorf("保存净化规则 %s: %w", filePath, err)
	}
	return describeRuleFile(filePath, false, rules), nil
}

func describeRuleFile(filePath string, local bool, rules []Rule) FileInfo {
	info := FileInfo{Path: filePath, Name: filepath.Base(filePath), Local: local, RuleCount: len(rules)}
	for _, rule := range rules {
		if rule.IsEnabled {
			info.EnabledCount++
		}
	}
	return info
}

func decodeRuleFile(data []byte, filePath string) ([]Rule, error) {
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))
	var records []json.RawMessage
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("净化规则 %s: 文件为空，需要 JSON 对象或数组", filePath)
	}
	switch trimmed[0] {
	case '[':
		if err := json.Unmarshal(trimmed, &records); err != nil {
			return nil, fmt.Errorf("净化规则 %s: JSON 数组无效: %w", filePath, err)
		}
	case '{':
		records = []json.RawMessage{trimmed}
	default:
		return nil, fmt.Errorf("净化规则 %s: 需要 JSON 对象或数组", filePath)
	}
	rules := make([]Rule, 0, len(records))
	for i, raw := range records {
		if value := bytes.TrimSpace(raw); len(value) == 0 || value[0] != '{' {
			return nil, fmt.Errorf("净化规则 %s 第 %d 项: 需要 JSON 对象", filePath, i+1)
		}
		var rule Rule
		if err := json.Unmarshal(raw, &rule); err != nil {
			return nil, fmt.Errorf("净化规则 %s 第 %d 项: %w", filePath, i+1, err)
		}
		rules = append(rules, rule)
	}
	if _, err := Compile(rules); err != nil {
		return nil, fmt.Errorf("净化规则 %s: %w", filePath, err)
	}
	return rules, nil
}

func readRuleFile(ctx context.Context, filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取净化规则 %s: %w", filePath, err)
	}
	defer file.Close()
	data, err := readRuleBytes(ctx, file)
	if err != nil {
		return nil, fmt.Errorf("读取净化规则 %s: %w", filePath, err)
	}
	return data, nil
}

func readRuleImport(ctx context.Context, location string) ([]byte, string, string, error) {
	parsed, err := url.Parse(location)
	if err == nil && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) {
		parsed.Scheme = strings.ToLower(parsed.Scheme)
		parsed.Fragment = ""
		identity := parsed.String()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, identity, nil)
		if err != nil {
			return nil, "", "", fmt.Errorf("读取净化规则 %s: %w", location, err)
		}
		client := &http.Client{Timeout: 30 * time.Second}
		response, err := client.Do(request)
		if err != nil {
			return nil, "", "", fmt.Errorf("下载净化规则 %s: %w", location, err)
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, "", "", fmt.Errorf("下载净化规则 %s: HTTP %d", location, response.StatusCode)
		}
		if response.ContentLength > ruleFileLimit {
			return nil, "", "", fmt.Errorf("下载净化规则 %s: 文件超过 %d 字节限制", location, ruleFileLimit)
		}
		data, err := readRuleBytes(ctx, response.Body)
		if err != nil {
			return nil, "", "", fmt.Errorf("下载净化规则 %s: %w", location, err)
		}
		return data, identity, path.Base(parsed.Path), nil
	}
	filePath, err := filepath.Abs(location)
	if err != nil {
		return nil, "", "", fmt.Errorf("净化规则路径 %s: %w", location, err)
	}
	data, err := readRuleFile(ctx, filePath)
	return data, filePath, filepath.Base(filePath), err
}

func readRuleBytes(ctx context.Context, input io.Reader) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(input, ruleFileLimit+1))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if int64(len(data)) > ruleFileLimit {
		return nil, fmt.Errorf("文件超过 %d 字节限制", ruleFileLimit)
	}
	return data, nil
}

func importRuleName(name string) string {
	name = strings.TrimSuffix(name, filepath.Ext(name))
	name = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)
	name = strings.Trim(name, "-")
	if name == "" {
		return "rules"
	}
	runes := []rune(name)
	if len(runes) > 24 {
		name = string(runes[:24])
	}
	return name
}

func writeRuleTemp(ctx context.Context, dir string, data []byte) (name string, err error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(dir, ".rules-*")
	if err != nil {
		return "", err
	}
	name = file.Name()
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(name)
		}
	}()
	if _, err = file.Write(data); err != nil {
		return name, err
	}
	if err = ctx.Err(); err != nil {
		return name, err
	}
	err = file.Sync()
	return name, err
}
