package core

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lvshp/ReadCLI/lib"
)

func buildBookshelfPanel() string {
	books := visibleBooks()
	var lines []string
	th := currentTheme()
	lines = append(lines, "["+titleCase(th.HomeName)+"](fg:cyan,mod:bold)")
	lines = append(lines, "")
	if app.loadingBookPath != "" {
		bookName := strings.TrimSuffix(filepath.Base(app.loadingBookPath), filepath.Ext(app.loadingBookPath))
		lines = append(lines,
			"正在打开：",
			"",
			"  "+shortenDisplay(bookName, bookshelfTitleWidth()),
			"",
			"请稍等，正在加载正文和目录…",
		)
		return strings.Join(lines, "\n")
	}
	if len(books) == 0 {
		lines = append(lines,
			"还没有导入任何书。",
			"",
			"开始方式：",
			"  1. 按 i 导入本地 txt / epub",
			"  2. 或直接运行 readcli /path/to/book.epub",
			"",
			"导入后会自动记录：",
			"  - 阅读进度",
			"  - 最后阅读时间",
			"  - 当前章节信息",
		)
		return strings.Join(lines, "\n")
	}

	pageSize := bookshelfPageSize()
	start := (app.shelfIndex / pageSize) * pageSize
	end := start + pageSize
	if end > len(books) {
		end = len(books)
	}
	lines = append(lines, fmt.Sprintf("共 %d 本  |  排序 %s  |  过滤 %s  |  第 %d/%d 页", len(books), readableSort(app.sortMode), readableFilter(app.filterMode), start/pageSize+1, (len(books)+pageSize-1)/pageSize))
	lines = append(lines, bookshelfStatsLine(books))
	lines = append(lines, "")
	titleWidth := bookshelfTitleWidth()
	lines = append(lines, "  书名")
	lines = append(lines, "  "+strings.Repeat("─", max(24, titleWidth)))
	for i := start; i < end; i++ {
		book := books[i]
		prefix := "  "
		if i == app.shelfIndex {
			prefix = "> "
		}
		lines = append(lines, prefix+shortenDisplay(book.Title, titleWidth))
	}
	return strings.Join(lines, "\n")
}

func bookshelfTitleWidth() int {
	titleWidth := 28
	if mainContentWidth > 0 {
		available := mainContentWidth - 4
		if available > 18 {
			titleWidth = available
		}
	}
	if titleWidth < 18 {
		titleWidth = 18
	}
	return titleWidth
}

func bookshelfStatsLine(books []lib.BookshelfBook) string {
	unread := 0
	reading := 0
	finished := 0
	for _, book := range books {
		switch {
		case book.ProgressPercent >= 100:
			finished++
		case book.ProgressPos > 0:
			reading++
		default:
			unread++
		}
	}
	return fmt.Sprintf("未读 %d  |  在读 %d  |  已读 %d", unread, reading, finished)
}

func bookshelfPageSize() int {
	if mainContentHeight > 4 {
		reservedLines := 8
		available := mainContentHeight - reservedLines
		if available < 3 {
			return 3
		}
		return available
	}
	return 10
}
