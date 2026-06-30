package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommonPathPrefix(t *testing.T) {
	got := commonPathPrefix("/tmp/books/novel-a.epub", "/tmp/books/novel-b.epub")
	want := "/tmp/books/novel-"
	if got != want {
		t.Fatalf("commonPathPrefix() = %q, want %q", got, want)
	}
}

func TestRenderImportPathWithTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}
	resolved := filepath.Join(home, "Books", "demo.epub")
	got := renderImportPath(resolved, true)
	want := filepath.Join("~", "Books", "demo.epub")
	if got != want {
		t.Fatalf("renderImportPath() = %q, want %q", got, want)
	}
}

func TestSameStringSlice(t *testing.T) {
	if !sameStringSlice([]string{"a", "b"}, []string{"a", "b"}) {
		t.Fatal("sameStringSlice() = false, want true")
	}
	if sameStringSlice([]string{"a", "b"}, []string{"a", "c"}) {
		t.Fatal("sameStringSlice() = true, want false")
	}
}

func TestImportHintPageBounds(t *testing.T) {
	app = &appState{
		uiState: uiState{
			input: inputState{
				hints:     []string{"a", "b", "c", "d", "e", "f", "g"},
				hintIndex: 5,
			},
		},
	}
	start, end, page, totalPages := importHintPageBounds(3)
	if start != 3 || end != 6 || page != 2 || totalPages != 3 {
		t.Fatalf("importHintPageBounds() = (%d, %d, %d, %d), want (3, 6, 2, 3)", start, end, page, totalPages)
	}
}
