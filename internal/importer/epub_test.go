package importer

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectEPUBReflowableReadsMetadataSpineAndTOC(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "book.epub")
	writeTestEPUB(t, archivePath, map[string]string{
		"mimetype":                "application/epub+zip",
		"META-INF/container.xml":  `<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="EPUB/package.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`,
		"EPUB/package.opf":        `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Test Nights</dc:title><dc:creator>Example Author</dc:creator><dc:language>en</dc:language><meta property="rendition:layout">reflowable</meta></metadata><manifest><item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/><item id="chapter" href="text/chapter.xhtml" media-type="application/xhtml+xml"/><item id="cover" href="images/cover.svg" media-type="image/svg+xml" properties="cover-image"/></manifest><spine page-progression-direction="ltr"><itemref idref="chapter"/></spine></package>`,
		"EPUB/nav.xhtml":          `<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><body><nav epub:type="toc"><ol><li><a href="text/chapter.xhtml#inicio">Opening Chapter</a></li></ol></nav></body></html>`,
		"EPUB/text/chapter.xhtml": `<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><head><title>Chapter</title></head><body><h1 id="inicio">Opening Chapter</h1><p>It was a wonderful night to search inside the book.</p></body></html>`,
		"EPUB/images/cover.svg":   `<svg xmlns="http://www.w3.org/2000/svg" width="600" height="900"><rect width="600" height="900" fill="#222"/></svg>`,
	})

	book, err := inspectEPUB(context.Background(), "fallback.epub", archivePath, testLimits())
	if err != nil {
		t.Fatalf("inspectEPUB() error = %v", err)
	}
	if book.Format != "epub" || book.Title != "Test Nights" || book.Writer != "Example Author" {
		t.Fatalf("unexpected metadata: %+v", book)
	}
	if book.EPUBLayout != "reflowable" || book.EPUBVersion != "3.0" || book.PageCount != 1 {
		t.Fatalf("unexpected publication: %+v", book)
	}
	if book.EPUBCoverPath != "EPUB/images/cover.svg" {
		t.Fatalf("cover = %q", book.EPUBCoverPath)
	}
	if len(book.EPUBSpine) != 1 || book.EPUBSpine[0].Path != "EPUB/text/chapter.xhtml" {
		t.Fatalf("unexpected spine: %+v", book.EPUBSpine)
	}
	if book.EPUBSpine[0].Title != "Opening Chapter" || book.EPUBSpine[0].TextContent == "" {
		t.Fatalf("spine metadata missing: %+v", book.EPUBSpine[0])
	}
	if len(book.EPUBTOC) != 1 || book.EPUBTOC[0].Path != "EPUB/text/chapter.xhtml" || book.EPUBTOC[0].Fragment != "inicio" {
		t.Fatalf("unexpected toc: %+v", book.EPUBTOC)
	}
}

func TestInspectEPUBDetectsFixedLayout(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "manga.epub")
	writeTestEPUB(t, archivePath, map[string]string{
		"mimetype":               "application/epub+zip",
		"META-INF/container.xml": `<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="package.opf"/></rootfiles></container>`,
		"package.opf":            `<package xmlns="http://www.idpf.org/2007/opf" version="3.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Manga</dc:title><meta property="rendition:layout">pre-paginated</meta></metadata><manifest><item id="p1" href="page1.xhtml" media-type="application/xhtml+xml"/></manifest><spine page-progression-direction="rtl"><itemref idref="p1"/></spine></package>`,
		"page1.xhtml":            `<html xmlns="http://www.w3.org/1999/xhtml"><head><meta name="viewport" content="width=1200,height=1800"/></head><body><img src="page1.jpg"/></body></html>`,
	})

	book, err := inspectEPUB(context.Background(), "manga.epub", archivePath, testLimits())
	if err != nil {
		t.Fatalf("inspectEPUB() error = %v", err)
	}
	if book.EPUBLayout != "fixed" {
		t.Fatalf("layout = %q", book.EPUBLayout)
	}
	if book.ReadingDirection == nil || *book.ReadingDirection != "rtl" {
		t.Fatalf("direction = %+v", book.ReadingDirection)
	}
}

func TestInspectEPUBRejectsUnsupportedEncryption(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "drm.epub")
	writeTestEPUB(t, archivePath, map[string]string{
		"mimetype":                "application/epub+zip",
		"META-INF/container.xml":  `<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="package.opf"/></rootfiles></container>`,
		"META-INF/encryption.xml": `<encryption><EncryptedData><EncryptionMethod Algorithm="http://example.com/drm"/></EncryptedData></encryption>`,
		"package.opf":             `<package xmlns="http://www.idpf.org/2007/opf" version="3.0"><metadata/><manifest><item id="p1" href="p1.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="p1"/></spine></package>`,
		"p1.xhtml":                `<html xmlns="http://www.w3.org/1999/xhtml"><body>Texto</body></html>`,
	})

	if _, err := inspectEPUB(context.Background(), "drm.epub", archivePath, testLimits()); err != ErrEPUBDRM {
		t.Fatalf("inspectEPUB() error = %v, want ErrEPUBDRM", err)
	}
}

func writeTestEPUB(t *testing.T, destination string, entries map[string]string) {
	t.Helper()
	file, err := os.Create(destination)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for name, content := range entries {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		if name == "mimetype" {
			header.Method = zip.Store
		}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
