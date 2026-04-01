package reader

import (
	"path/filepath"
	"testing"
)

func TestEpubReaderUsesNavTitlesAndChapterNavigation(t *testing.T) {
	tempDir := t.TempDir()
	bookPath := filepath.Join(tempDir, "book.epub")

	files := map[string]string{
		"META-INF/container.xml": `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`,
		"OEBPS/content.opf": `<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf">
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="chapter-1" href="text/chapter1.xhtml" media-type="application/xhtml+xml"/>
    <item id="chapter-2" href="text/chapter2.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter-1"/>
    <itemref idref="chapter-2"/>
  </spine>
</package>`,
		"OEBPS/nav.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
  <body>
    <nav epub:type="toc">
      <ol>
        <li><a href="text/chapter1.xhtml">One</a></li>
        <li><a href="text/chapter2.xhtml">Two</a></li>
      </ol>
    </nav>
  </body>
</html>`,
		"OEBPS/text/chapter1.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>Ignored 1</h1><p>Alpha text.</p></body></html>`,
		"OEBPS/text/chapter2.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>Ignored 2</h1><p>Beta text.</p></body></html>`,
	}

	if err := writeTestZip(bookPath, files); err != nil {
		t.Fatalf("write epub: %v", err)
	}

	r := NewEpubReader()
	if err := r.Load(bookPath); err != nil {
		t.Fatalf("load epub: %v", err)
	}

	if got := r.CurrentChapterTitle(); got != "One" {
		t.Fatalf("chapter title = %q, want %q", got, "One")
	}

	if got := r.GetTOC(); got != "目录\nOne\nTwo" {
		t.Fatalf("toc = %q", got)
	}

	if got := r.GetTOCWithSelection(1, 10); got != "目录\nj/k 移动，数字 + 回车跳转，m 返回\n第 1/1 页\n*  One\n>  Two" {
		t.Fatalf("selected toc = %q", got)
	}

	if got := r.NextChapter(); got != "Ignored 2" {
		t.Fatalf("next chapter current line = %q, want %q", got, "Ignored 2")
	}

	if got := r.CurrentChapterTitle(); got != "Two" {
		t.Fatalf("chapter title after next = %q, want %q", got, "Two")
	}

	if got := r.PrevChapter(); got != "Ignored 1" {
		t.Fatalf("prev chapter current line = %q, want %q", got, "Ignored 1")
	}

	if got := r.GotoChapter(1); got != "Ignored 2" {
		t.Fatalf("goto chapter current line = %q, want %q", got, "Ignored 2")
	}

	if got := r.GetTOCWithSelection(1, 1); got != "目录\nj/k 移动，数字 + 回车跳转，m 返回\n第 2/2 页\n*> Two" {
		t.Fatalf("paged toc = %q", got)
	}
}

func TestEpubReaderSkipsCoverFrontMatterOnOpen(t *testing.T) {
	tempDir := t.TempDir()
	bookPath := filepath.Join(tempDir, "cover-book.epub")

	files := map[string]string{
		"META-INF/container.xml": `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`,
		"OEBPS/content.opf": `<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf">
  <manifest>
    <item id="cover" href="cover.xhtml" media-type="application/xhtml+xml"/>
    <item id="chapter-1" href="text/chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="cover"/>
    <itemref idref="chapter-1"/>
  </spine>
</package>`,
		"OEBPS/cover.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body><p>Cover</p></body></html>`,
		"OEBPS/text/chapter1.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>Chapter 1</h1><p>Real content starts here.</p></body></html>`,
	}

	if err := writeTestZip(bookPath, files); err != nil {
		t.Fatalf("write epub: %v", err)
	}

	r := NewEpubReader()
	if err := r.Load(bookPath); err != nil {
		t.Fatalf("load epub: %v", err)
	}

	if got := r.Current(); got != "Chapter 1" {
		t.Fatalf("current line = %q, want %q", got, "Chapter 1")
	}

	if got := r.GetTOC(); got != "目录\nChapter 1" {
		t.Fatalf("toc = %q", got)
	}
}

func TestEpubReaderDeduplicatesLeadingTitleBlock(t *testing.T) {
	tempDir := t.TempDir()
	bookPath := filepath.Join(tempDir, "dup-title.epub")

	files := map[string]string{
		"META-INF/container.xml": `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`,
		"OEBPS/content.opf": `<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf">
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="chapter-1" href="text/chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter-1"/>
  </spine>
</package>`,
		"OEBPS/nav.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
  <body>
    <nav epub:type="toc">
      <ol>
        <li><a href="text/chapter1.xhtml">第04章 暧昧的表姐弟</a></li>
      </ol>
    </nav>
  </body>
</html>`,
		"OEBPS/text/chapter1.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>第04章 暧昧的表姐弟</h1><p>第04章</p><p>暧昧的表姐弟</p><p>正文开始</p></body></html>`,
	}

	if err := writeTestZip(bookPath, files); err != nil {
		t.Fatalf("write epub: %v", err)
	}

	r := NewEpubReader()
	if err := r.Load(bookPath); err != nil {
		t.Fatalf("load epub: %v", err)
	}

	if got := r.CurrentView(3); got != "第04章 暧昧的表姐弟\n正文开始" {
		t.Fatalf("current view = %q", got)
	}
}

func TestEpubReaderUsesMetadataTitle(t *testing.T) {
	tempDir := t.TempDir()
	bookPath := filepath.Join(tempDir, "meta-title.epub")
	if err := writeTestZip(bookPath, map[string]string{
		"META-INF/container.xml": `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`,
		"OEBPS/content.opf": `<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <metadata>
    <dc:title>三国当混蛋</dc:title>
  </metadata>
  <manifest>
    <item id="chapter-1" href="text/chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter-1"/>
  </spine>
</package>`,
		"OEBPS/text/chapter1.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>第一章</h1><p>正文</p></body></html>`,
	}); err != nil {
		t.Fatalf("write epub: %v", err)
	}

	r := NewEpubReader()
	if err := r.Load(bookPath); err != nil {
		t.Fatalf("load epub: %v", err)
	}

	if got := r.BookTitle(); got != "三国当混蛋" {
		t.Fatalf("book title = %q", got)
	}
}
