package booksource

import (
	"context"
	"fmt"
	"strings"
)

func bookVars(book Book) map[string]any {
	vars := make(map[string]any, len(book.Variables)+1)
	for key, value := range book.Variables {
		vars[key] = value
	}
	vars["book"] = map[string]any{"name": book.Name, "author": book.Author, "bookUrl": book.URL, "tocUrl": book.TOCURL, "origin": book.SourceURL}
	return vars
}

func (c *Client) Search(ctx context.Context, source Source, keyword string, page int) ([]Book, error) {
	if err := ValidateSource(source); err != nil {
		return nil, err
	}
	if !source.IsEnabled() {
		return nil, fmt.Errorf("书源 %q 已禁用", source.Name)
	}
	if page < 1 {
		page = 1
	}
	if strings.TrimSpace(source.Search.BookList) == "" {
		return nil, fmt.Errorf("书源 %q 缺少 ruleSearch.bookList", source.Name)
	}
	vars := map[string]any{"key": keyword, "searchKey": keyword, "page": page}
	body, base, err := c.fetch(ctx, source, source.URL, source.SearchURL, vars, 0)
	if err != nil {
		return nil, fmt.Errorf("搜索 %s: %w", source.Name, err)
	}
	e := c.evaluator(ctx, source, base, vars)
	input, err := initialize(e, body, source.Search.Init)
	if err != nil {
		return nil, fmt.Errorf("搜索 init: %w", err)
	}
	items, err := e.Elements(input, source.Search.BookList)
	if err != nil {
		return nil, fmt.Errorf("搜索 bookList: %w", err)
	}
	books := make([]Book, 0, len(items))
	seen := make(map[string]bool)
	for i, item := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		book, err := extractBook(e, item, source.Search, Book{SourceURL: source.URL, SourceName: source.Name}, base)
		if err != nil {
			return nil, fmt.Errorf("搜索结果 %d: %w", i+1, err)
		}
		if book.URL == "" || book.Name == "" {
			return nil, fmt.Errorf("搜索结果 %d 缺少书名或链接，请检查书源规则", i+1)
		}
		if !seen[book.URL] {
			seen[book.URL] = true
			books = append(books, book)
		}
	}
	return books, nil
}

func (c *Client) BookInfo(ctx context.Context, source Source, book Book) (Book, error) {
	vars := bookVars(book)
	body, base, err := c.fetch(ctx, source, source.URL, book.URL, vars, 0)
	if err != nil {
		return book, fmt.Errorf("读取书籍详情: %w", err)
	}
	e := c.evaluator(ctx, source, base, vars)
	input, err := initialize(e, body, source.BookInfo.Init)
	if err != nil {
		return book, fmt.Errorf("详情 init: %w", err)
	}
	book.SourceURL = source.URL
	book.SourceName = source.Name
	book, err = extractBook(e, input, source.BookInfo, book, base)
	if err != nil {
		return book, fmt.Errorf("书籍详情: %w", err)
	}
	if book.TOCURL == "" {
		book.TOCURL = base
	}
	return book, nil
}
func initialize(e *Evaluator, input any, rule string) (any, error) {
	if strings.TrimSpace(rule) == "" {
		return input, nil
	}
	items, err := e.Elements(input, rule)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("初始化规则未匹配内容")
	}
	return items[0], nil
}
func extractBook(e *Evaluator, input any, rules BookRule, book Book, base string) (Book, error) {
	fields := []struct {
		name string
		rule string
		dest *string
		link bool
	}{
		{"name", rules.Name, &book.Name, false}, {"author", rules.Author, &book.Author, false},
		{"bookUrl", rules.BookURL, &book.URL, true}, {"tocUrl", rules.TOCURL, &book.TOCURL, true},
		{"coverUrl", rules.CoverURL, &book.CoverURL, true}, {"intro", rules.Intro, &book.Intro, false},
		{"kind", rules.Kind, &book.Kind, false}, {"lastChapter", rules.LastChapter, &book.LastChapter, false},
	}
	for _, field := range fields {
		if field.rule == "" {
			continue
		}
		value, err := e.String(input, field.rule)
		if err != nil {
			return book, fmt.Errorf("%s: %w", field.name, err)
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if field.link {
			value, err = resolveURL(base, value)
			if err != nil {
				return book, fmt.Errorf("%s: %w", field.name, err)
			}
		}
		*field.dest = value
	}
	return book, nil
}

func (c *Client) Chapters(ctx context.Context, source Source, book Book) ([]Chapter, error) {
	if strings.TrimSpace(source.TOC.ChapterList) == "" || strings.TrimSpace(source.TOC.ChapterURL) == "" {
		return nil, fmt.Errorf("书源缺少目录 chapterList 或 chapterUrl 规则")
	}
	start := book.TOCURL
	if start == "" {
		start = book.URL
	}
	start, err := resolveURL(source.URL, start)
	if err != nil {
		return nil, err
	}
	pending := []string{start}
	scheduled := map[string]bool{start: true}
	visited, chaptersSeen := make(map[string]bool), make(map[string]bool)
	chapters := make([]Chapter, 0)
	vars := bookVars(book)
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		if visited[current] {
			continue
		}
		if len(visited) >= c.pageLimit() {
			return nil, fmt.Errorf("目录分页超过 %d 页限制", c.pageLimit())
		}
		visited[current] = true
		body, base, err := c.fetch(ctx, source, current, current, vars, 0)
		if err != nil {
			return nil, fmt.Errorf("读取目录: %w", err)
		}
		e := c.evaluator(ctx, source, base, vars)
		items, err := e.Elements(body, source.TOC.ChapterList)
		if err != nil {
			return nil, fmt.Errorf("目录 chapterList: %w", err)
		}
		for i, item := range items {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if source.TOC.IsVolume != "" {
				volume, err := e.String(item, source.TOC.IsVolume)
				if err != nil {
					return nil, fmt.Errorf("目录 isVolume: %w", err)
				}
				if volume == "true" || volume == "1" {
					continue
				}
			}
			name, err := e.String(item, source.TOC.ChapterName)
			if err != nil {
				return nil, fmt.Errorf("目录 chapterName: %w", err)
			}
			link, err := e.String(item, source.TOC.ChapterURL)
			if err != nil {
				return nil, fmt.Errorf("目录 chapterUrl: %w", err)
			}
			if strings.TrimSpace(link) == "" {
				return nil, fmt.Errorf("目录第 %d 项链接为空，请检查书源规则", i+1)
			}
			link, err = resolveURL(base, link)
			if err != nil {
				return nil, fmt.Errorf("目录 chapterUrl: %w", err)
			}
			if !chaptersSeen[link] {
				if len(chapters) >= 100000 {
					return nil, fmt.Errorf("章节数超过 100000 条限制")
				}
				chaptersSeen[link] = true
				chapters = append(chapters, Chapter{Name: strings.TrimSpace(name), URL: link})
			}
		}
		next, err := nextPages(e, body, source.TOC.NextTOCURL, base)
		if err != nil {
			return nil, fmt.Errorf("目录 nextTocUrl: %w", err)
		}
		for _, link := range next {
			if !scheduled[link] {
				if len(scheduled) >= c.pageLimit() {
					return nil, fmt.Errorf("目录分页超过 %d 页限制", c.pageLimit())
				}
				scheduled[link] = true
				pending = append(pending, link)
			}
		}
	}
	if len(chapters) == 0 {
		return nil, fmt.Errorf("未提取到章节，请检查目录规则或网站响应")
	}
	for i := 0; i+1 < len(chapters); i++ {
		chapters[i].NextURL = chapters[i+1].URL
	}
	return chapters, nil
}

func (c *Client) Content(ctx context.Context, source Source, book Book, chapter Chapter) (string, error) {
	if strings.TrimSpace(source.Content.Content) == "" {
		return "", fmt.Errorf("书源缺少正文 content 规则")
	}
	start, err := resolveURL(book.URL, chapter.URL)
	if err != nil {
		return "", err
	}
	pending := []string{start}
	scheduled := map[string]bool{start: true}
	visited := make(map[string]bool)
	vars := bookVars(book)
	vars["chapter"] = map[string]any{"title": chapter.Name, "name": chapter.Name, "url": chapter.URL}
	chunks := make([]string, 0)
	totalSize := 0
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		if visited[current] {
			continue
		}
		if len(visited) >= c.pageLimit() {
			return "", fmt.Errorf("正文分页超过 %d 页限制", c.pageLimit())
		}
		visited[current] = true
		body, base, err := c.fetch(ctx, source, current, current, vars, 0)
		if err != nil {
			return "", fmt.Errorf("读取正文: %w", err)
		}
		e := c.evaluator(ctx, source, base, vars)
		parts, err := e.Strings(body, source.Content.Content)
		if err != nil {
			return "", fmt.Errorf("正文 content: %w", err)
		}
		chunk := strings.TrimSpace(strings.Join(parts, "\n"))

		if chunk != "" {
			chunks = append(chunks, chunk)
			totalSize += len(chunk)
		}
		if totalSize > 64<<20 {
			return "", fmt.Errorf("合并正文超过 64 MiB 限制")
		}
		next, err := nextPages(e, body, source.Content.NextContentURL, base)
		if err != nil {
			return "", fmt.Errorf("正文 nextContentUrl: %w", err)
		}
		for _, link := range next {
			if link == chapter.NextURL {
				continue
			}
			if !scheduled[link] {
				if len(scheduled) >= c.pageLimit() {
					return "", fmt.Errorf("正文分页超过 %d 页限制", c.pageLimit())
				}
				scheduled[link] = true
				pending = append(pending, link)
			}
		}
	}
	if len(chunks) == 0 {
		return "", fmt.Errorf("未提取到正文，请检查正文规则或网站响应")
	}
	content := strings.Join(chunks, "\n\n")
	if source.Content.ReplaceRegex != "" {
		rule := strings.TrimSpace(source.Content.ReplaceRegex)
		lower := strings.ToLower(rule)
		if !strings.HasPrefix(rule, "##") && !strings.Contains(lower, "@js:") && !strings.Contains(lower, "<js>") {
			rule = "##" + rule
		}
		e := c.evaluator(ctx, source, start, vars)
		content, err = e.String(content, rule)
		if err != nil {
			return "", fmt.Errorf("正文 replaceRegex: %w", err)
		}
	}
	content = NormalizeContent(content)
	if content == "" {
		return "", fmt.Errorf("正文清理后为空，请检查正文规则或网站响应")
	}
	return content, nil
}
func nextPages(e *Evaluator, input any, rule, base string) ([]string, error) {
	if strings.TrimSpace(rule) == "" {
		return nil, nil
	}
	values, err := e.Strings(input, rule)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || value == "#" || strings.HasPrefix(strings.ToLower(value), "javascript:") {
			continue
		}
		absolute, err := resolveURL(base, value)
		if err != nil {
			return nil, err
		}
		out = append(out, absolute)
	}
	return out, nil
}
