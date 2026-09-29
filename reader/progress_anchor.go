package reader

import (
	"math"
	"strings"
)

type ProgressAnchor struct {
	Pos           int
	ChapterIndex  int
	ChapterOffset float64
	OverallRatio  float64
}

func AnchorFromReader(r Reader) ProgressAnchor {
	anchor := ProgressAnchor{
		Pos:          0,
		ChapterIndex: -1,
	}
	if r == nil {
		return anchor
	}

	total := r.Total()
	pos := r.CurrentPos()
	anchor.Pos = pos
	if total > 1 && pos > 0 {
		anchor.OverallRatio = float64(pos) / float64(total-1)
	}

	switch v := r.(type) {
	case *OnlineReader:
		anchor.ChapterIndex = v.Index
		anchor.ChapterOffset = v.ChapterOffset()
		anchor.OverallRatio = v.OverallProgress()
	case *TxtReader:
		anchor.ChapterIndex = v.currentChapterIndex()
		anchor.ChapterOffset = txtChapterOffset(v)
	case *EpubReader:
		anchor.ChapterIndex = v.currentChapterIndex()
		anchor.ChapterOffset = epubChapterOffset(v)
	}

	return anchor
}

func RestoreFromAnchor(r Reader, anchor ProgressAnchor) {
	if r == nil || r.Total() == 0 {
		return
	}

	switch v := r.(type) {
	case *OnlineReader:
		v.RestoreOffset(anchor.ChapterOffset)
		return
	case *TxtReader:
		if restoreTXTFromAnchor(v, anchor) {
			return
		}
	case *EpubReader:
		if restoreEPUBFromAnchor(v, anchor) {
			return
		}
	}

	restoreGenericFromAnchor(r, anchor)
}

func FindChapterIndexByTitle(r Reader, title string) int {
	title = normalizeAnchorTitle(title)
	if r == nil || title == "" {
		return -1
	}

	switch v := r.(type) {
	case *TxtReader:
		for i, chapter := range v.chapters {
			if normalizeAnchorTitle(chapter.Title) == title {
				return i
			}
		}
	case *EpubReader:
		for i, chapter := range v.chapters {
			if normalizeAnchorTitle(chapter.Title) == title {
				return i
			}
		}
	}

	return -1
}

func restoreGenericFromAnchor(r Reader, anchor ProgressAnchor) {
	total := r.Total()
	if total <= 0 {
		return
	}
	if anchor.Pos >= 0 && anchor.Pos < total {
		r.Goto(anchor.Pos)
		return
	}
	if anchor.OverallRatio > 0 {
		target := int(math.Round(anchor.OverallRatio * float64(total-1)))
		r.Goto(target)
		return
	}
	r.Goto(0)
}

func txtChapterOffset(r *TxtReader) float64 {
	if len(r.chapters) == 0 {
		return 0
	}
	idx := r.currentChapterIndex()
	start, end := txtChapterBounds(r, idx)
	return chapterOffsetRatio(r.pos, start, end)
}

func epubChapterOffset(r *EpubReader) float64 {
	if len(r.chapters) == 0 {
		return 0
	}
	idx := r.currentChapterIndex()
	start, end := epubChapterBounds(r, idx)
	return chapterOffsetRatio(r.pos, start, end)
}

func restoreTXTFromAnchor(r *TxtReader, anchor ProgressAnchor) bool {
	if len(r.chapters) == 0 || anchor.ChapterIndex < 0 {
		return false
	}
	if anchor.ChapterIndex >= len(r.chapters) {
		anchor.ChapterIndex = len(r.chapters) - 1
	}
	start, end := txtChapterBounds(r, anchor.ChapterIndex)
	r.Goto(positionFromChapterOffset(start, end, anchor.ChapterOffset))
	return true
}

func restoreEPUBFromAnchor(r *EpubReader, anchor ProgressAnchor) bool {
	if len(r.chapters) == 0 || anchor.ChapterIndex < 0 {
		return false
	}
	if anchor.ChapterIndex >= len(r.chapters) {
		anchor.ChapterIndex = len(r.chapters) - 1
	}
	start, end := epubChapterBounds(r, anchor.ChapterIndex)
	r.Goto(positionFromChapterOffset(start, end, anchor.ChapterOffset))
	return true
}

func txtChapterBounds(r *TxtReader, idx int) (int, int) {
	start := r.chapters[idx].Start
	end := len(r.content) - 1
	if idx+1 < len(r.chapters) {
		end = r.chapters[idx+1].Start - 1
	}
	if end < start {
		end = start
	}
	return start, end
}

func epubChapterBounds(r *EpubReader, idx int) (int, int) {
	start := r.chapters[idx].Start
	end := len(r.content) - 1
	if idx+1 < len(r.chapters) {
		end = r.chapters[idx+1].Start - 1
	}
	if end < start {
		end = start
	}
	return start, end
}

func chapterOffsetRatio(pos, start, end int) float64 {
	if pos < start {
		pos = start
	}
	if pos > end {
		pos = end
	}
	span := end - start
	if span <= 0 {
		return 0
	}
	return float64(pos-start) / float64(span)
}

func positionFromChapterOffset(start, end int, offset float64) int {
	if offset < 0 {
		offset = 0
	}
	if offset > 1 {
		offset = 1
	}
	span := end - start
	if span <= 0 {
		return start
	}
	return start + int(math.Round(offset*float64(span)))
}

func normalizeAnchorTitle(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Join(strings.Fields(value), " ")
	return value
}
