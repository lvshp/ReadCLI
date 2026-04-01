package reader

import "errors"

type Snapshot struct {
	Kind          string   `json:"kind"`
	Title         string   `json:"title,omitempty"`
	RawText       string   `json:"raw_text,omitempty"`
	ChapterText   []string `json:"chapter_text,omitempty"`
	ChapterTitles []string `json:"chapter_titles,omitempty"`
}

func SnapshotFromReader(r Reader) (*Snapshot, bool) {
	switch v := r.(type) {
	case *TxtReader:
		return &Snapshot{
			Kind:    "txt",
			RawText: v.rawText,
		}, true
	case *EpubReader:
		titles := make([]string, 0, len(v.chapters))
		for _, chapter := range v.chapters {
			titles = append(titles, chapter.Title)
		}
		chapterText := append([]string(nil), v.chapterText...)
		return &Snapshot{
			Kind:          "epub",
			Title:         v.title,
			ChapterText:   chapterText,
			ChapterTitles: titles,
		}, true
	default:
		return nil, false
	}
}

func ReaderFromSnapshot(snapshot Snapshot, width int) (Reader, error) {
	switch snapshot.Kind {
	case "txt":
		r := NewTxtReader()
		r.setContent(snapshot.RawText)
		r.buildChapters()
		if width > 0 {
			r.Reflow(width)
		}
		return r, nil
	case "epub":
		if len(snapshot.ChapterText) == 0 {
			return nil, errors.New("empty epub cache")
		}
		r := NewEpubReader()
		r.title = snapshot.Title
		r.chapterText = append([]string(nil), snapshot.ChapterText...)
		if width <= 0 {
			width = defaultLineWidth
		}
		r.lineWidth = width
		r.rebuildChapters(snapshot.ChapterTitles, width)
		r.pos = startingChapterPosition(r.chapters)
		return r, nil
	default:
		return nil, errors.New("unsupported cache kind")
	}
}
