package importer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func inspectPDF(ctx context.Context, filename, pdfPath string) (inspectedBook, error) {
	if err := ctx.Err(); err != nil {
		return inspectedBook{}, err
	}
	file, err := os.Open(pdfPath)
	if err != nil {
		return inspectedBook{}, fmt.Errorf("%w: %v", ErrInvalidPDF, err)
	}
	defer file.Close()
	buffer := make([]byte, 1024)
	n, err := file.Read(buffer)
	if err != nil && n == 0 {
		return inspectedBook{}, fmt.Errorf("%w: could not read the header", ErrInvalidPDF)
	}
	buffer = buffer[:n]
	if !bytes.HasPrefix(buffer, []byte("%PDF-")) {
		return inspectedBook{}, fmt.Errorf("%w: assinatura %%PDF ausente", ErrInvalidPDF)
	}
	info, err := file.Stat()
	if err != nil || info.Size() < 8 {
		return inspectedBook{}, fmt.Errorf("%w: empty or incomplete file", ErrInvalidPDF)
	}
	title := strings.TrimSpace(strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)))
	if title == "" {
		title = "PDF book"
	}
	return inspectedBook{Format: "pdf", PDFAnalysisStatus: "pending", PageCount: 1, Title: title}, nil
}
