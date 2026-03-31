package reader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTxtReaderBuildsTOCFromChapterHeadings(t *testing.T) {
	tempDir := t.TempDir()
	bookPath := filepath.Join(tempDir, "book.txt")
	text := "序章\n内容一\n第1章 山中少年\n这是第一章。\n第2章 下山\n这是第二章。"
	if err := os.WriteFile(bookPath, []byte(text), 0644); err != nil {
		t.Fatalf("write txt: %v", err)
	}

	r := NewTxtReader()
	if err := r.Load(bookPath); err != nil {
		t.Fatalf("load txt: %v", err)
	}

	if got := r.GetTOC(); got != "Table of Contents\n1. 第1章 山中少年\n2. 第2章 下山" {
		t.Fatalf("toc = %q", got)
	}

	if got := r.CurrentChapterTitle(); got != "第1章 山中少年" {
		t.Fatalf("chapter title = %q", got)
	}

	if got := r.NextChapter(); got != "第2章 下山" {
		t.Fatalf("next chapter current line = %q", got)
	}
}

func TestTxtReaderTOCSelectionAndGotoChapter(t *testing.T) {
	tempDir := t.TempDir()
	bookPath := filepath.Join(tempDir, "book.txt")
	text := "第1章 开始\n一\n第2章 发展\n二\n第3章 结尾\n三"
	if err := os.WriteFile(bookPath, []byte(text), 0644); err != nil {
		t.Fatalf("write txt: %v", err)
	}

	r := NewTxtReader()
	if err := r.Load(bookPath); err != nil {
		t.Fatalf("load txt: %v", err)
	}

	if got := r.GetTOCWithSelection(1, 2); got != "Table of Contents\nj/k to move, number + Enter to open, m to close\nPage 1/2\n*  1. 第1章 开始\n>  2. 第2章 发展" {
		t.Fatalf("selected toc = %q", got)
	}

	if got := r.GotoChapter(2); got != "第3章 结尾" {
		t.Fatalf("goto chapter current line = %q", got)
	}

	if got := r.GetTOCWithSelection(2, 2); got != "Table of Contents\nj/k to move, number + Enter to open, m to close\nPage 2/2\n*> 3. 第3章 结尾" {
		t.Fatalf("paged toc = %q", got)
	}
}

func TestTxtReaderDeduplicatesRepeatedChapterHeadings(t *testing.T) {
	tempDir := t.TempDir()
	bookPath := filepath.Join(tempDir, "book.txt")
	text := "第七章 清风观\n\n\n  第七章  清风观\n\n正文开始"
	if err := os.WriteFile(bookPath, []byte(text), 0644); err != nil {
		t.Fatalf("write txt: %v", err)
	}

	r := NewTxtReader()
	if err := r.Load(bookPath); err != nil {
		t.Fatalf("load txt: %v", err)
	}

	if got := r.CurrentView(3); got != "第七章 清风观\n正文开始" {
		t.Fatalf("current view = %q", got)
	}

	if got := r.GetTOC(); got != "Table of Contents\n1. 第七章 清风观" {
		t.Fatalf("toc = %q", got)
	}
}

func TestTxtReaderBookTitleFallsBackToFileName(t *testing.T) {
	tempDir := t.TempDir()
	bookPath := filepath.Join(tempDir, "测试小说.txt")
	text := "《测试小说》\n作者：某人\n\n第1章 开始\n正文"
	if err := os.WriteFile(bookPath, []byte(text), 0644); err != nil {
		t.Fatalf("write txt: %v", err)
	}

	r := NewTxtReader()
	if err := r.Load(bookPath); err != nil {
		t.Fatalf("load txt: %v", err)
	}

	if got := r.BookTitle(); got != "" {
		t.Fatalf("BookTitle() = %q, want empty", got)
	}
	}
