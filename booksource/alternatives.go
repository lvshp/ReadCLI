package booksource

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// AlternativeResult retains successful candidates when individual sources fail.
type AlternativeResult struct {
	Books  []Book
	Errors []string
}

// FindAlternatives searches every enabled, searchable text source. Candidate
// order follows the source order, then the order returned by each source.
func (c *Client) FindAlternatives(ctx context.Context, sources []Source, current Book, page int) (AlternativeResult, error) {
	if err := ctx.Err(); err != nil {
		return AlternativeResult{}, err
	}
	title := normalizeAlternativeName(current.Name)
	if title == "" {
		return AlternativeResult{}, fmt.Errorf("当前书籍缺少书名，无法查找其他书源")
	}
	if page < 1 {
		page = 1
	}
	var searchable []Source
	for _, source := range sources {
		if source.IsEnabled() && source.Type == 0 && strings.TrimSpace(source.SearchURL) != "" && strings.TrimSpace(source.Search.BookList) != "" {
			searchable = append(searchable, source)
		}
	}
	type searchResult struct {
		index int
		books []Book
		err   error
	}
	jobs := make(chan int, len(searchable))
	results := make(chan searchResult, len(searchable))
	for i := range searchable {
		jobs <- i
	}
	close(jobs)
	for worker := 0; worker < min(4, len(searchable)); worker++ {
		go func() {
			for index := range jobs {
				if ctx.Err() != nil {
					return
				}
				sourceCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
				books, err := c.Search(sourceCtx, searchable[index], current.Name, page)
				cancel()
				results <- searchResult{index: index, books: books, err: err}
			}
		}()
	}
	ordered := make([]searchResult, len(searchable))
	for received := 0; received < len(searchable); received++ {
		select {
		case <-ctx.Done():
			return AlternativeResult{}, ctx.Err()
		case result := <-results:
			ordered[result.index] = result
		}
	}
	if err := ctx.Err(); err != nil {
		return AlternativeResult{}, err
	}
	result := AlternativeResult{}
	author := normalizeAlternativeAuthor(current.Author)
	seen := make(map[[2]string]bool)
	seen[[2]string{current.SourceURL, current.URL}] = true
	for i, searched := range ordered {
		if searched.err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", searchable[i].Name, searched.err))
			continue
		}
		for _, book := range searched.books {
			if normalizeAlternativeName(book.Name) != title {
				continue
			}
			candidateAuthor := normalizeAlternativeAuthor(book.Author)
			if author != "" && candidateAuthor != "" && author != candidateAuthor {
				continue
			}
			identity := [2]string{book.SourceURL, book.URL}
			if !seen[identity] {
				seen[identity] = true
				result.Books = append(result.Books, book)
			}
		}
	}
	return result, nil
}

func normalizeAlternativeName(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || strings.ContainsRune("《》〈〉「」『』【】", r) {
			return -1
		}
		return r
	}, name)
}

func normalizeAlternativeAuthor(author string) string {
	author = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, author)
	author = strings.TrimPrefix(strings.TrimPrefix(author, "作者:"), "作者：")
	return strings.TrimSuffix(author, "著")
}

// MatchAlternativeChapter uses only unique matches. When no reliable title or
// chapter number exists, the caller must disclose that the clamped index is an
// approximation rather than an identified chapter.
func MatchAlternativeChapter(chapters []Chapter, previousTitle string, previousIndex int) (index int, matched bool) {
	if len(chapters) == 0 {
		return -1, false
	}
	previous := alternativeChapterSignature(previousTitle)
	signatures := make([]alternativeChapter, len(chapters))
	for i, chapter := range chapters {
		signatures[i] = alternativeChapterSignature(chapter.Name)
	}
	uniqueMatch := func(matches func(alternativeChapter) bool) (int, bool) {
		index := -1
		for i, candidate := range signatures {
			if matches(candidate) {
				if index >= 0 {
					return -1, false
				}
				index = i
			}
		}
		return index, index >= 0
	}
	if meaningfulChapterName(previous.full) {
		if index, ok := uniqueMatch(func(candidate alternativeChapter) bool { return candidate.full == previous.full }); ok {
			return index, true
		}
	}
	compatibleVolume := func(candidate alternativeChapter) bool {
		return !previous.hasVolume || !candidate.hasVolume || previous.volume == candidate.volume
	}
	if meaningfulChapterName(previous.name) {
		if index, ok := uniqueMatch(func(candidate alternativeChapter) bool {
			return candidate.name == previous.name && compatibleVolume(candidate)
		}); ok {
			return index, true
		}
	}
	if previous.hasNumber {
		if index, ok := uniqueMatch(func(candidate alternativeChapter) bool {
			return candidate.hasNumber && candidate.number == previous.number && compatibleVolume(candidate)
		}); ok {
			return index, true
		}
	}
	return min(max(previousIndex, 0), len(chapters)-1), false
}

type alternativeChapter struct {
	full, name           string
	number, volume       int
	hasNumber, hasVolume bool
}

var alternativeVolumePattern = regexp.MustCompile(`^第?([0-9零〇一二三四五六七八九十百千万两]+)卷`)
var alternativeChapterPattern = regexp.MustCompile(`^第?([0-9零〇一二三四五六七八九十百千万两]+)[章节回集](.*)$`)

func alternativeChapterSignature(title string) alternativeChapter {
	title = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			return -1
		}
		if r >= '０' && r <= '９' {
			return r - '０' + '0'
		}
		return unicode.ToLower(r)
	}, title)
	result := alternativeChapter{full: title, name: title}
	remainder := strings.TrimPrefix(title, "正文")
	if volume := alternativeVolumePattern.FindStringSubmatch(remainder); volume != nil {
		result.volume, result.hasVolume = alternativeChapterNumber(volume[1])
		remainder = strings.TrimPrefix(remainder, volume[0])
	}
	if chapter := alternativeChapterPattern.FindStringSubmatch(remainder); chapter != nil {
		result.number, result.hasNumber = alternativeChapterNumber(chapter[1])
		result.name = chapter[2]
	}
	return result
}

func meaningfulChapterName(name string) bool {
	switch name {
	case "", "前言", "序言", "序章", "序", "楔子", "引子", "目录", "正文", "无题", "未命名", "章节", "待更新", "最新章节", "后记", "尾声", "感言", "请假", "请假条", "通知", "公告":
		return false
	default:
		return true
	}
}

func alternativeChapterNumber(value string) (int, bool) {
	if number, err := strconv.Atoi(value); err == nil {
		return number, number >= 0
	}
	digits := map[rune]int{'零': 0, '〇': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	units := map[rune]int{'十': 10, '百': 100, '千': 1000}
	// Digit-only Chinese numbers (for example 一〇二) occur in some catalogs.
	if !strings.ContainsAny(value, "十百千万") {
		number := 0
		for _, r := range value {
			digit, ok := digits[r]
			if !ok || number > 1000000 {
				return 0, false
			}
			number = number*10 + digit
		}
		return number, value != ""
	}
	total, section, digit := 0, 0, 0
	for _, r := range value {
		if number, ok := digits[r]; ok {
			digit = number
		} else if unit, ok := units[r]; ok {
			if digit == 0 {
				digit = 1
			}
			section += digit * unit
			digit = 0
		} else if r == '万' {
			total += (section + digit) * 10000
			section, digit = 0, 0
		} else {
			return 0, false
		}
		if total+section+digit > 10000000 {
			return 0, false
		}
	}
	return total + section + digit, true
}
