package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var validPDFDocumentKinds = map[string]struct{}{
	"unknown":      {},
	"text":         {},
	"scanned_book": {},
	"comic":        {},
	"mixed":        {},
}

var validPDFOCRModes = map[string]struct{}{
	"auto":       {},
	"off":        {},
	"on_demand":  {},
	"background": {},
	"full":       {},
}

type PDFClassificationInput struct {
	PageCount          int           `json:"page_count"`
	Title              string        `json:"title"`
	Author             string        `json:"author"`
	Subject            string        `json:"subject"`
	Language           string        `json:"language"`
	PublicationYear    *int          `json:"publication_year,omitempty"`
	TextLayer          string        `json:"text_layer"`
	DocumentKind       string        `json:"document_kind"`
	RecommendedOCRMode string        `json:"recommended_ocr_mode"`
	SampledPages       int           `json:"sampled_pages"`
	Pages              []PDFPageText `json:"pages"`
}

type PDFPageTextInput struct {
	PageNumber int     `json:"page_number"`
	Text       string  `json:"text"`
	Source     string  `json:"source"`
	Quality    float64 `json:"quality"`
}

func (s *Service) SavePDFClassification(ctx context.Context, bookID int64, input PDFClassificationInput) error {
	if input.PageCount < 1 || input.PageCount > 100_000 || input.SampledPages < 0 || input.SampledPages > input.PageCount {
		return ErrInvalidBook
	}
	if _, ok := validPDFDocumentKinds[input.DocumentKind]; !ok {
		return ErrInvalidBook
	}
	if _, ok := validPDFOCRModes[input.RecommendedOCRMode]; !ok || input.RecommendedOCRMode == "auto" {
		return ErrInvalidBook
	}
	switch input.TextLayer {
	case "text", "scanned", "mixed", "corrupt":
	default:
		return ErrInvalidBook
	}
	if len(input.Pages) > input.SampledPages || len(input.Pages) > 32 {
		return ErrInvalidBook
	}
	if input.PublicationYear != nil && (*input.PublicationYear < 0 || *input.PublicationYear > 9999) {
		return ErrInvalidBook
	}

	seen := make(map[int]struct{}, len(input.Pages))
	for index := range input.Pages {
		page := &input.Pages[index]
		if page.PageNumber < 0 || page.PageNumber >= input.PageCount {
			return ErrInvalidBook
		}
		if _, exists := seen[page.PageNumber]; exists {
			return ErrInvalidBook
		}
		seen[page.PageNumber] = struct{}{}
		page.Text = normalizePDFText(page.Text)
		if page.Source != "pdf" && page.Source != "ocr" && page.Source != "none" {
			return ErrInvalidBook
		}
		if page.Quality < 0 || page.Quality > 1 || len(page.Text) > maxPDFPageTextBytes {
			return ErrInvalidBook
		}
		if (page.Source == "none") != (page.Text == "") {
			return ErrInvalidBook
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin pdf classification: %w", err)
	}
	defer tx.Rollback()

	var format, currentMode, currentTitle, originalFilename, currentWriter, currentSummary, currentLanguage string
	var currentYear sql.NullInt64
	if err := tx.QueryRowContext(ctx, `
		SELECT format, pdf_ocr_mode, title, original_filename, writer, summary, language, publication_year
		FROM books WHERE id = ? AND status = 'ready'
	`, bookID).Scan(&format, &currentMode, &currentTitle, &originalFilename, &currentWriter, &currentSummary, &currentLanguage, &currentYear); errors.Is(err, sql.ErrNoRows) {
		return ErrBookNotFound
	} else if err != nil {
		return fmt.Errorf("find pdf for classification: %w", err)
	}
	if format != "pdf" {
		return ErrUnsupportedFormat
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM pdf_pages WHERE book_id = ?`, bookID); err != nil {
		return fmt.Errorf("clear preliminary pdf index: %w", err)
	}
	for _, page := range input.Pages {
		if page.Source == "none" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO pdf_pages(book_id, page_number, text_content, search_content, search_compact, text_source, text_quality)
			VALUES (?, ?, ?, ?, '', ?, ?)
		`, bookID, page.PageNumber, page.Text, normalizePDFSearchText(page.Text), page.Source, page.Quality); err != nil {
			return fmt.Errorf("store sampled pdf page: %w", err)
		}
	}

	mode := currentMode
	if mode == "" || mode == "auto" {
		mode = input.RecommendedOCRMode
	}
	ocrStatus := ocrStatusForMode(mode, input.DocumentKind, len(input.Pages), input.PageCount)

	sets := []string{
		"page_count = ?",
		"pdf_analysis_status = 'classified'",
		"pdf_classification_status = 'complete'",
		"pdf_text_layer = ?",
		"pdf_document_kind = ?",
		"pdf_ocr_mode = ?",
		"pdf_ocr_status = ?",
		"pdf_sampled_pages = ?",
		"pdf_indexed_pages = ?",
		"pdf_invalid_pages = 0",
		"pdf_ocr_pages = ?",
		"pdf_analyzed_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')",
		"updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')",
	}
	indexedPages, ocrPages := countIndexedSamplePages(input.Pages)
	args := []any{input.PageCount, input.TextLayer, input.DocumentKind, mode, ocrStatus, input.SampledPages, indexedPages, ocrPages}
	appendPDFMetadataUpdates(&sets, &args, input.Title, input.Author, input.Subject, input.Language, input.PublicationYear, currentTitle, originalFilename, currentWriter, currentSummary, currentLanguage, currentYear)
	args = append(args, bookID)
	if _, err := tx.ExecContext(ctx, `UPDATE books SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return fmt.Errorf("update pdf classification: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pdf classification: %w", err)
	}
	return nil
}

func (s *Service) UpdatePDFOCRMode(ctx context.Context, bookID int64, mode string) error {
	mode = strings.TrimSpace(mode)
	if _, ok := validPDFOCRModes[mode]; !ok {
		return ErrInvalidBook
	}
	var format, kind string
	var pageCount, indexedPages int
	if err := s.db.QueryRowContext(ctx, `
		SELECT format, pdf_document_kind, page_count, pdf_indexed_pages
		FROM books WHERE id = ? AND status = 'ready'
	`, bookID).Scan(&format, &kind, &pageCount, &indexedPages); errors.Is(err, sql.ErrNoRows) {
		return ErrBookNotFound
	} else if err != nil {
		return fmt.Errorf("find pdf for ocr mode: %w", err)
	}
	if format != "pdf" {
		return ErrUnsupportedFormat
	}
	resolved := mode
	if mode == "auto" {
		resolved = recommendedOCRMode(kind)
	}
	status := ocrStatusForMode(resolved, kind, indexedPages, pageCount)
	if _, err := s.db.ExecContext(ctx, `
		UPDATE books
		SET pdf_ocr_mode = ?, pdf_ocr_status = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?
	`, mode, status, bookID); err != nil {
		return fmt.Errorf("update pdf ocr mode: %w", err)
	}
	return nil
}

func (s *Service) SavePDFPageText(ctx context.Context, bookID int64, input PDFPageTextInput) error {
	if input.PageNumber < 0 || input.Quality < 0 || input.Quality > 1 {
		return ErrInvalidBook
	}
	input.Text = normalizePDFText(input.Text)
	if input.Source != "pdf" && input.Source != "ocr" {
		return ErrInvalidBook
	}
	if input.Text == "" || len(input.Text) > maxPDFPageTextBytes {
		return ErrInvalidBook
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin pdf page text: %w", err)
	}
	defer tx.Rollback()
	var format, mode, kind string
	var pageCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT format, page_count, pdf_ocr_mode, pdf_document_kind
		FROM books WHERE id = ? AND status = 'ready'
	`, bookID).Scan(&format, &pageCount, &mode, &kind); errors.Is(err, sql.ErrNoRows) {
		return ErrBookNotFound
	} else if err != nil {
		return fmt.Errorf("find pdf page text book: %w", err)
	}
	if format != "pdf" {
		return ErrUnsupportedFormat
	}
	if input.PageNumber >= pageCount {
		return ErrPageOutOfRange
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO pdf_pages(book_id, page_number, text_content, search_content, search_compact, text_source, text_quality)
		VALUES (?, ?, ?, ?, '', ?, ?)
		ON CONFLICT(book_id, page_number) DO UPDATE SET
			text_content = excluded.text_content,
			search_content = excluded.search_content,
			search_compact = '',
			text_source = excluded.text_source,
			text_quality = excluded.text_quality,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
	`, bookID, input.PageNumber, input.Text, normalizePDFSearchText(input.Text), input.Source, input.Quality); err != nil {
		return fmt.Errorf("upsert pdf page text: %w", err)
	}
	var indexedPages, ocrPages int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(CASE WHEN text_source = 'ocr' THEN 1 ELSE 0 END), 0)
		FROM pdf_pages WHERE book_id = ? AND text_content <> ''
	`, bookID).Scan(&indexedPages, &ocrPages); err != nil {
		return fmt.Errorf("count pdf page text: %w", err)
	}
	status := ocrStatusForMode(mode, kind, indexedPages, pageCount)
	if indexedPages >= pageCount {
		status = "complete"
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE books
		SET pdf_indexed_pages = ?, pdf_ocr_pages = ?, pdf_ocr_status = ?,
			pdf_analysis_status = CASE WHEN ? >= page_count THEN 'complete' ELSE 'classified' END,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?
	`, indexedPages, ocrPages, status, indexedPages, bookID); err != nil {
		return fmt.Errorf("update pdf page text counters: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pdf page text: %w", err)
	}
	return nil
}

func countIndexedSamplePages(pages []PDFPageText) (indexed, ocr int) {
	for _, page := range pages {
		if page.Source == "none" || page.Text == "" {
			continue
		}
		indexed++
		if page.Source == "ocr" {
			ocr++
		}
	}
	return indexed, ocr
}

func recommendedOCRMode(kind string) string {
	switch kind {
	case "text", "comic":
		return "off"
	case "scanned_book", "mixed":
		return "on_demand"
	default:
		return "on_demand"
	}
}

func ocrStatusForMode(mode, kind string, indexedPages, pageCount int) string {
	if pageCount > 0 && indexedPages >= pageCount {
		return "complete"
	}
	switch mode {
	case "off":
		if kind == "text" {
			return "not_needed"
		}
		return "disabled"
	case "background", "full":
		if indexedPages > 0 {
			return "partial"
		}
		return "needed"
	case "on_demand", "auto":
		if indexedPages > 0 {
			return "partial"
		}
		return "idle"
	default:
		return "idle"
	}
}

func appendPDFMetadataUpdates(
	sets *[]string,
	args *[]any,
	title, author, subject, language string,
	publicationYear *int,
	currentTitle, originalFilename, currentWriter, currentSummary, currentLanguage string,
	currentYear sql.NullInt64,
) {
	title = limitPDFMetadata(title, 240)
	author = limitPDFMetadata(author, 2_000)
	subject = limitPDFMetadata(subject, 20_000)
	language = limitPDFMetadata(language, 64)
	filenameTitle := strings.TrimSpace(strings.TrimSuffix(filepath.Base(originalFilename), filepath.Ext(originalFilename)))
	if title != "" && (strings.TrimSpace(currentTitle) == "" || strings.EqualFold(strings.TrimSpace(currentTitle), filenameTitle)) {
		*sets = append(*sets, "title = ?")
		*args = append(*args, title)
	}
	if author != "" && strings.TrimSpace(currentWriter) == "" {
		*sets = append(*sets, "writer = ?")
		*args = append(*args, author)
	}
	if subject != "" && strings.TrimSpace(currentSummary) == "" {
		*sets = append(*sets, "summary = ?")
		*args = append(*args, subject)
	}
	if language != "" && strings.TrimSpace(currentLanguage) == "" {
		*sets = append(*sets, "language = ?")
		*args = append(*args, language)
	}
	if publicationYear != nil && !currentYear.Valid {
		*sets = append(*sets, "publication_year = ?")
		*args = append(*args, *publicationYear)
	}
}
