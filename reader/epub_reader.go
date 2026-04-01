package reader

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	reBlockTags  = regexp.MustCompile(`(?is)</?(p|div|section|article|h[1-6]|li|blockquote|tr|td|br)[^>]*>`)
	reAllTags    = regexp.MustCompile(`(?is)<[^>]+>`)
	reWhitespace = regexp.MustCompile(`[\t\f\v ]+`)
)

type EpubReader struct {
	contentReader
	chapters    []epubChapter
	toc         string
	chapterText []string
	title       string
}

func (epub *EpubReader) Reflow(width int) {
	if width <= 0 {
		width = defaultLineWidth
	}

	progress := epub.progressRatio()
	epub.lineWidth = width
	epub.rebuildContent(width)
	epub.restoreProgress(progress)
}

type epubContainer struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

type epubPackage struct {
	Title    string
	Manifest []struct {
		ID         string `xml:"id,attr"`
		Href       string `xml:"href,attr"`
		MediaType  string `xml:"media-type,attr"`
		Properties string `xml:"properties,attr"`
	} `xml:"manifest>item"`
	Spine []struct {
		IDRef string `xml:"idref,attr"`
	} `xml:"spine>itemref"`
	TOC string `xml:"spine,toc,attr"`
}

type epubChapter struct {
	Title string
	Start int
}

type epubNCX struct {
	NavMap []epubNCXNavPoint `xml:"navMap>navPoint"`
}

type epubNCXNavPoint struct {
	Label struct {
		Text string `xml:"text"`
	} `xml:"navLabel"`
	Content struct {
		Src string `xml:"src,attr"`
	} `xml:"content"`
	Children []epubNCXNavPoint `xml:"navPoint"`
}

func NewEpubReader() *EpubReader {
	return &EpubReader{}
}

func (epub *EpubReader) Load(filePath string) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return err
	}

	zr, err := zip.OpenReader(filePath)
	if err != nil {
		return err
	}
	defer zr.Close()

	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[f.Name] = f
	}

	opfPath, err := findPackagePath(files)
	if err != nil {
		return err
	}

	pkg, err := readPackage(files[opfPath])
	if err != nil {
		return err
	}
	epub.title = strings.TrimSpace(pkg.Title)

	manifest := make(map[string]string, len(pkg.Manifest))
	baseDir := path.Dir(opfPath)
	tocMap := make(map[string]string)
	var navPath string
	var ncxPath string
	for _, item := range pkg.Manifest {
		joinedPath := path.Clean(path.Join(baseDir, item.Href))
		manifest[item.ID] = joinedPath

		if strings.Contains(item.Properties, "nav") {
			navPath = joinedPath
		}

		if item.ID == pkg.TOC || item.MediaType == "application/x-dtbncx+xml" {
			ncxPath = joinedPath
		}
	}

	if navPath != "" {
		if navTOC, err := readNavTOC(files[navPath], navPath); err == nil {
			tocMap = navTOC
		}
	}

	if len(tocMap) == 0 && ncxPath != "" {
		if ncxTOC, err := readNCXTOC(files[ncxPath], ncxPath); err == nil {
			tocMap = ncxTOC
		}
	}

	chapterTexts := make([]string, 0, len(pkg.Spine))
	chapterTitles := make([]string, 0, len(pkg.Spine))
	for _, item := range pkg.Spine {
		chapterPath, ok := manifest[item.IDRef]
		if !ok {
			continue
		}

		chapterFile, ok := files[chapterPath]
		if !ok {
			continue
		}

		chapterText, err := readChapterText(chapterFile)
		if err != nil {
			return err
		}

		if strings.TrimSpace(chapterText) == "" {
			continue
		}

		title := chapterTitleForPath(chapterPath, tocMap)
		if title == "" {
			title = inferChapterTitle(chapterText, chapterPath)
		}

		if shouldSkipChapter(chapterPath, title, chapterText) {
			continue
		}

		chapterTexts = append(chapterTexts, chapterText)
		chapterTitles = append(chapterTitles, title)
	}

	if len(chapterTexts) == 0 {
		return errors.New("no readable content found in epub")
	}

	epub.chapterText = chapterTexts
	epub.lineWidth = defaultLineWidth
	epub.rebuildChapters(chapterTitles, epub.lineWidth)
	epub.pos = startingChapterPosition(epub.chapters)
	return nil
}

func (epub *EpubReader) BookTitle() string {
	return strings.TrimSpace(epub.title)
}

func (epub *EpubReader) CurrentChapterTitle() string {
	if len(epub.chapters) == 0 {
		return ""
	}

	return epub.chapters[epub.currentChapterIndex()].Title
}

func (epub *EpubReader) CurrentChapterIndex() int {
	return epub.currentChapterIndex()
}

func (epub *EpubReader) NextChapter() string {
	if len(epub.chapters) == 0 {
		return epub.Current()
	}

	index := epub.currentChapterIndex()
	if index < len(epub.chapters)-1 {
		epub.pos = epub.chapters[index+1].Start
	}

	return epub.Current()
}

func (epub *EpubReader) PrevChapter() string {
	if len(epub.chapters) == 0 {
		return epub.Current()
	}

	index := epub.currentChapterIndex()
	if index > 0 {
		epub.pos = epub.chapters[index-1].Start
	} else {
		epub.pos = epub.chapters[0].Start
	}

	return epub.Current()
}

func (epub *EpubReader) GetTOC() string {
	if epub.toc == "" {
		return "没有可用目录。"
	}

	return epub.toc
}

func (epub *EpubReader) GetTOCWithSelection(selected, pageSize int) string {
	if len(epub.chapters) == 0 {
		return epub.GetTOC()
	}

	if selected < 0 {
		selected = 0
	}

	if selected >= len(epub.chapters) {
		selected = len(epub.chapters) - 1
	}

	if pageSize <= 0 {
		pageSize = len(epub.chapters)
	}

	current := epub.CurrentChapterIndex()
	start := (selected / pageSize) * pageSize
	end := start + pageSize
	if end > len(epub.chapters) {
		end = len(epub.chapters)
	}

	var builder strings.Builder
	builder.WriteString("目录\n")
	builder.WriteString("j/k 移动，数字 + 回车跳转，m 返回\n")
	builder.WriteString(fmt.Sprintf("第 %d/%d 页\n", start/pageSize+1, (len(epub.chapters)+pageSize-1)/pageSize))
	for i := start; i < end; i++ {
		chapter := epub.chapters[i]
		prefix := "  "
		switch {
		case i == selected && i == current:
			prefix = "*>"
		case i == selected:
			prefix = "> "
		case i == current:
			prefix = "* "
		}

		builder.WriteString(fmt.Sprintf("%s %s\n", prefix, chapter.Title))
	}

	return strings.TrimRight(builder.String(), "\n")
}

func (epub *EpubReader) GotoChapter(index int) string {
	if len(epub.chapters) == 0 {
		return epub.Current()
	}

	if index < 0 {
		index = 0
	}

	if index >= len(epub.chapters) {
		index = len(epub.chapters) - 1
	}

	epub.pos = epub.chapters[index].Start
	return epub.Current()
}

func (epub *EpubReader) GetProgress() string {
	if len(epub.content) == 0 {
		return "(0 / 0)"
	}

	chapter := epub.CurrentChapterTitle()
	if chapter == "" {
		return epub.contentReader.GetProgress()
	}

	return fmt.Sprintf("%s %s", chapter, epub.contentReader.GetProgress())
}

func (epub *EpubReader) currentChapterIndex() int {
	if len(epub.chapters) == 0 {
		return 0
	}

	for i := len(epub.chapters) - 1; i >= 0; i-- {
		if epub.pos >= epub.chapters[i].Start {
			return i
		}
	}

	return 0
}

func (epub *EpubReader) rebuildContent(width int) {
	titles := make([]string, 0, len(epub.chapters))
	for _, chapter := range epub.chapters {
		titles = append(titles, chapter.Title)
	}

	if len(titles) == 0 && len(epub.chapterText) > 0 {
		for _, chapterText := range epub.chapterText {
			titles = append(titles, inferChapterTitle(chapterText, ""))
		}
	}

	epub.rebuildChapters(titles, width)
}

func (epub *EpubReader) rebuildChapters(titles []string, width int) {
	contentLines := make([]string, 0)
	chapters := make([]epubChapter, 0, len(epub.chapterText))

	for i, chapterText := range epub.chapterText {
		title := ""
		if i < len(titles) {
			title = titles[i]
		}
		chapterText = dedupeLeadingEpubTitle(chapterText, title)
		chapterLines := paginateContent(chapterText, width)
		if len(chapterLines) == 0 {
			continue
		}

		chapters = append(chapters, epubChapter{
			Title: title,
			Start: len(contentLines),
		})
		contentLines = append(contentLines, chapterLines...)
	}

	epub.content = contentLines
	epub.chapters = chapters
	epub.toc = buildTOC(chapters)
}

func findPackagePath(files map[string]*zip.File) (string, error) {
	containerFile, ok := files["META-INF/container.xml"]
	if !ok {
		return "", errors.New("invalid epub: META-INF/container.xml not found")
	}

	data, err := readZipFile(containerFile)
	if err != nil {
		return "", err
	}

	var container epubContainer
	if err := xml.Unmarshal(data, &container); err != nil {
		return "", err
	}

	if len(container.Rootfiles) == 0 || container.Rootfiles[0].FullPath == "" {
		return "", errors.New("invalid epub: package document not found")
	}

	return container.Rootfiles[0].FullPath, nil
}

func readPackage(file *zip.File) (*epubPackage, error) {
	data, err := readZipFile(file)
	if err != nil {
		return nil, err
	}

	var pkg epubPackage
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}
	pkg.Title = extractEPUBTitle(data)

	return &pkg, nil
}

func extractEPUBTitle(data []byte) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)<dc:title[^>]*>(.*?)</dc:title>`),
		regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`),
	}
	for _, pattern := range patterns {
		match := pattern.FindSubmatch(data)
		if len(match) < 2 {
			continue
		}
		title := strings.TrimSpace(stripMarkup(string(match[1])))
		if title != "" {
			return title
		}
	}
	return ""
}

func readNavTOC(file *zip.File, navPath string) (map[string]string, error) {
	if file == nil {
		return nil, errors.New("nav document not found")
	}

	data, err := readZipFile(file)
	if err != nil {
		return nil, err
	}

	reNav := regexp.MustCompile(`(?is)<nav[^>]*?(?:epub:type|type)=["'][^"']*toc[^"']*["'][^>]*>(.*?)</nav>`)
	reLink := regexp.MustCompile(`(?is)<a[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	navMatch := reNav.FindSubmatch(data)
	if len(navMatch) < 2 {
		return nil, errors.New("toc nav not found")
	}

	matches := reLink.FindAllSubmatch(navMatch[1], -1)
	toc := make(map[string]string, len(matches))
	for _, match := range matches {
		href := normalizeHref(resolveRelative(navPath, string(match[1])))
		title := strings.TrimSpace(stripMarkup(string(match[2])))
		if href == "" || title == "" {
			continue
		}
		toc[href] = title
	}

	if len(toc) == 0 {
		return nil, errors.New("toc nav links not found")
	}

	return toc, nil
}

func readNCXTOC(file *zip.File, ncxPath string) (map[string]string, error) {
	if file == nil {
		return nil, errors.New("ncx document not found")
	}

	data, err := readZipFile(file)
	if err != nil {
		return nil, err
	}

	var ncx epubNCX
	if err := xml.Unmarshal(data, &ncx); err != nil {
		return nil, err
	}

	toc := make(map[string]string)
	flattenNCXPoints(ncx.NavMap, ncxPath, toc)

	if len(toc) == 0 {
		return nil, errors.New("ncx nav points not found")
	}

	return toc, nil
}

func readChapterText(file *zip.File) (string, error) {
	data, err := readZipFile(file)
	if err != nil {
		return "", err
	}

	text := stripMarkup(string(data))

	lines := strings.Split(text, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = reWhitespace.ReplaceAllString(line, " ")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		cleaned = append(cleaned, line)
	}

	return strings.Join(cleaned, "\n"), nil
}

func stripMarkup(text string) string {
	text = reBlockTags.ReplaceAllString(text, "\n")
	text = reAllTags.ReplaceAllString(text, "")
	return html.UnescapeString(text)
}

func flattenNCXPoints(points []epubNCXNavPoint, basePath string, toc map[string]string) {
	for _, point := range points {
		if href := normalizeHref(resolveRelative(basePath, point.Content.Src)); href != "" && point.Label.Text != "" {
			toc[href] = strings.TrimSpace(point.Label.Text)
		}

		if len(point.Children) > 0 {
			flattenNCXPoints(point.Children, basePath, toc)
		}
	}
}

func chapterTitleForPath(chapterPath string, toc map[string]string) string {
	if len(toc) == 0 {
		return ""
	}

	if title, ok := toc[normalizeHref(chapterPath)]; ok {
		return title
	}

	base := normalizeHref(filepath.Base(chapterPath))
	return toc[base]
}

func inferChapterTitle(chapterText, chapterPath string) string {
	lines := strings.Split(chapterText, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}

	name := strings.TrimSuffix(filepath.Base(chapterPath), filepath.Ext(chapterPath))
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	return strings.TrimSpace(name)
}

func normalizeHref(href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}

	if idx := strings.Index(href, "#"); idx >= 0 {
		href = href[:idx]
	}

	return path.Clean(strings.TrimPrefix(href, "./"))
}

func resolveRelative(basePath, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}

	if strings.Contains(href, "://") {
		return href
	}

	return path.Clean(path.Join(path.Dir(basePath), href))
}

func buildTOC(chapters []epubChapter) string {
	if len(chapters) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("目录\n")
	for _, chapter := range chapters {
		builder.WriteString(chapter.Title)
		builder.WriteString("\n")
	}

	return strings.TrimRight(builder.String(), "\n")
}

func dedupeLeadingEpubTitle(chapterText, title string) string {
	if strings.TrimSpace(chapterText) == "" || strings.TrimSpace(title) == "" {
		return chapterText
	}

	lines := strings.Split(chapterText, "\n")
	normalizedTitle := normalizeChapterText(title)
	if normalizedTitle == "" {
		return chapterText
	}

	consumed := 0
	for consumed < len(lines) && consumed < 6 {
		line := strings.TrimSpace(lines[consumed])
		if line == "" {
			consumed++
			continue
		}
		if !looksLikeTitleFragment(line, normalizedTitle) {
			break
		}
		consumed++
	}

	if consumed == 0 {
		return chapterText
	}

	result := []string{title}
	for i := consumed; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

func looksLikeTitleFragment(line, normalizedTitle string) bool {
	normalizedLine := normalizeChapterText(line)
	if normalizedLine == "" {
		return true
	}
	if normalizedLine == normalizedTitle {
		return true
	}
	if len([]rune(line)) > 40 {
		return false
	}
	return strings.Contains(normalizedTitle, normalizedLine)
}

func shouldSkipChapter(chapterPath, title, chapterText string) bool {
	base := strings.ToLower(filepath.Base(chapterPath))
	normalizedTitle := normalizeChapterText(title)
	normalizedText := normalizeChapterText(chapterText)

	if normalizedText == "" {
		return true
	}

	if isCoverLike(base, normalizedTitle, normalizedText) {
		return true
	}

	return false
}

func normalizeChapterText(text string) string {
	text = strings.ToLower(text)
	text = strings.ReplaceAll(text, "\n", " ")
	text = reWhitespace.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

func isCoverLike(base, title, text string) bool {
	shortText := len(strings.Fields(text)) <= 4
	coverName := strings.Contains(base, "cover") || strings.Contains(base, "titlepage") || strings.Contains(base, "title-page")
	if coverName && shortText {
		return true
	}

	coverWords := map[string]struct{}{
		"cover":             {},
		"book cover":        {},
		"title page":        {},
		"copyright":         {},
		"contents":          {},
		"table of contents": {},
	}

	if _, ok := coverWords[title]; ok && shortText {
		return true
	}

	if _, ok := coverWords[text]; ok && shortText {
		return true
	}

	return false
}

func startingChapterPosition(chapters []epubChapter) int {
	if len(chapters) == 0 {
		return 0
	}

	for _, chapter := range chapters {
		if looksLikeReadableChapter(chapter.Title) {
			return chapter.Start
		}
	}

	return chapters[0].Start
}

func looksLikeReadableChapter(title string) bool {
	title = normalizeChapterText(title)
	if title == "" {
		return false
	}

	if strings.HasPrefix(title, "chapter ") {
		return true
	}

	if strings.HasPrefix(title, "part ") {
		return true
	}

	if _, err := strconv.Atoi(strings.TrimPrefix(title, "chapter")); err == nil {
		return true
	}

	return title != "cover" && title != "title page"
}

func readZipFile(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	buf := bytes.NewBuffer(nil)
	if _, err := io.Copy(buf, rc); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
