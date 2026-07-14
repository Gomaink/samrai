package catalog

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxPDFAnalysisTextBytes = 32 << 20
	maxPDFPageTextBytes     = 2 << 20
	maxPDFCoverBytes        = 6 << 20
)

type PDFPageText struct {
	PageNumber int     `json:"page_number"`
	Text       string  `json:"text"`
	Source     string  `json:"source"`
	Quality    float64 `json:"quality"`
}

type PDFAnalysisInput struct {
	PageCount       int           `json:"page_count"`
	Title           string        `json:"title"`
	Author          string        `json:"author"`
	Subject         string        `json:"subject"`
	Language        string        `json:"language"`
	PublicationYear *int          `json:"publication_year,omitempty"`
	TextLayer       string        `json:"text_layer"`
	Pages           []PDFPageText `json:"pages"`
}

type PDFSearchResult struct {
	PageNumber int    `json:"page_number"`
	Excerpt    string `json:"excerpt"`
	Matches    int    `json:"matches"`
	MatchType  string `json:"match_type"`
	TextSource string `json:"text_source"`
}

type PDFCoverAsset struct {
	Stream    io.ReadCloser
	MediaType string
	Size      int64
	Width     int
	Height    int
	ETag      string
	UpdatedAt time.Time
}

func (s *Service) SavePDFAnalysis(ctx context.Context, bookID int64, input PDFAnalysisInput) error {
	if input.PageCount < 1 || input.PageCount > 100_000 || len(input.Pages) != input.PageCount {
		return ErrInvalidBook
	}
	switch input.TextLayer {
	case "text", "scanned", "mixed", "corrupt", "ocr", "hybrid":
	default:
		return ErrInvalidBook
	}

	seen := make(map[int]struct{}, len(input.Pages))
	totalBytes := 0
	ocrPages := 0
	invalidPages := 0
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
		if page.Source == "" {
			if page.Text == "" {
				page.Source = "none"
			} else {
				page.Source = "pdf"
			}
		}
		switch page.Source {
		case "pdf":
		case "ocr":
			ocrPages++
		case "none":
			invalidPages++
		default:
			return ErrInvalidBook
		}
		if page.Quality < 0 || page.Quality > 1 {
			return ErrInvalidBook
		}
		if page.Source == "none" && page.Text != "" {
			return ErrInvalidBook
		}
		if page.Source != "none" && page.Text == "" {
			return ErrInvalidBook
		}
		if len(page.Text) > maxPDFPageTextBytes {
			return ErrInvalidBook
		}
		totalBytes += len(page.Text)
		if totalBytes > maxPDFAnalysisTextBytes {
			return ErrInvalidBook
		}
	}

	input.Title = limitPDFMetadata(input.Title, 240)
	input.Author = limitPDFMetadata(input.Author, 2_000)
	input.Subject = limitPDFMetadata(input.Subject, 20_000)
	input.Language = limitPDFMetadata(input.Language, 64)
	if input.PublicationYear != nil && (*input.PublicationYear < 0 || *input.PublicationYear > 9999) {
		return ErrInvalidBook
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin pdf analysis: %w", err)
	}
	defer tx.Rollback()

	var format, currentTitle, originalFilename, currentWriter, currentSummary, currentLanguage, currentOCRMode, currentDocumentKind string
	var currentYear sql.NullInt64
	if err := tx.QueryRowContext(ctx, `
		SELECT format, title, original_filename, writer, summary, language, publication_year, pdf_ocr_mode, pdf_document_kind
		FROM books WHERE id = ? AND status = 'ready'
	`, bookID).Scan(&format, &currentTitle, &originalFilename, &currentWriter, &currentSummary, &currentLanguage, &currentYear, &currentOCRMode, &currentDocumentKind); errors.Is(err, sql.ErrNoRows) {
		return ErrBookNotFound
	} else if err != nil {
		return fmt.Errorf("find pdf for analysis: %w", err)
	}
	if format != "pdf" {
		return ErrUnsupportedFormat
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM pdf_pages WHERE book_id = ?`, bookID); err != nil {
		return fmt.Errorf("clear pdf text index: %w", err)
	}
	statement, err := tx.PrepareContext(ctx, `
		INSERT INTO pdf_pages(book_id, page_number, text_content, search_content, search_compact, text_source, text_quality)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare pdf text index: %w", err)
	}
	defer statement.Close()
	for _, page := range input.Pages {
		searchContent := normalizePDFSearchText(page.Text)
		// Kept for schema compatibility. Search is evaluated token by token so
		// whitespace from unrelated words can never create a match.
		searchCompact := ""
		if _, err := statement.ExecContext(ctx, bookID, page.PageNumber, page.Text, searchContent, searchCompact, page.Source, page.Quality); err != nil {
			return fmt.Errorf("store pdf page text: %w", err)
		}
	}

	indexedPages := input.PageCount - invalidPages
	ocrStatus := ocrStatusForMode(currentOCRMode, currentDocumentKind, indexedPages, input.PageCount)
	if invalidPages == 0 {
		if ocrPages > 0 {
			ocrStatus = "complete"
		} else {
			ocrStatus = "not_needed"
		}
	} else if ocrPages > 0 {
		ocrStatus = "partial"
	}
	sets := []string{
		"page_count = ?",
		"pdf_analysis_status = 'complete'",
		"pdf_classification_status = 'complete'",
		"pdf_text_layer = ?",
		"pdf_ocr_status = ?",
		"pdf_invalid_pages = ?",
		"pdf_ocr_pages = ?",
		"pdf_indexed_pages = ?",
		"pdf_analyzed_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')",
		"updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')",
	}
	args := []any{input.PageCount, input.TextLayer, ocrStatus, invalidPages, ocrPages, indexedPages}

	filenameTitle := strings.TrimSpace(strings.TrimSuffix(filepath.Base(originalFilename), filepath.Ext(originalFilename)))
	if input.Title != "" && (strings.TrimSpace(currentTitle) == "" || strings.EqualFold(strings.TrimSpace(currentTitle), filenameTitle)) {
		sets = append(sets, "title = ?")
		args = append(args, input.Title)
	}
	if input.Author != "" && strings.TrimSpace(currentWriter) == "" {
		sets = append(sets, "writer = ?")
		args = append(args, input.Author)
	}
	if input.Subject != "" && strings.TrimSpace(currentSummary) == "" {
		sets = append(sets, "summary = ?")
		args = append(args, input.Subject)
	}
	if input.Language != "" && strings.TrimSpace(currentLanguage) == "" {
		sets = append(sets, "language = ?")
		args = append(args, input.Language)
	}
	if input.PublicationYear != nil && !currentYear.Valid {
		sets = append(sets, "publication_year = ?")
		args = append(args, *input.PublicationYear)
	}
	args = append(args, bookID)
	if _, err := tx.ExecContext(ctx, `UPDATE books SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return fmt.Errorf("update pdf analysis: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pdf analysis: %w", err)
	}
	return nil
}

func (s *Service) MarkPDFAnalysisFailed(ctx context.Context, bookID int64) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE books
		SET pdf_analysis_status = 'failed', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ? AND status = 'ready' AND format = 'pdf'
	`, bookID)
	if err != nil {
		return fmt.Errorf("mark pdf analysis failed: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read pdf analysis rows: %w", err)
	}
	if rows == 0 {
		return ErrBookNotFound
	}
	return nil
}

func (s *Service) SavePDFCover(ctx context.Context, bookID int64, mediaType string, width, height int, data []byte) error {
	if width < 1 || height < 1 || width > 8_192 || height > 12_288 || len(data) < 32 || len(data) > maxPDFCoverBytes {
		return ErrInvalidBook
	}
	if mediaType != "image/webp" && mediaType != "image/jpeg" && mediaType != "image/png" {
		return ErrInvalidBook
	}
	var format string
	if err := s.db.QueryRowContext(ctx, `SELECT format FROM books WHERE id = ? AND status = 'ready'`, bookID).Scan(&format); errors.Is(err, sql.ErrNoRows) {
		return ErrBookNotFound
	} else if err != nil {
		return fmt.Errorf("find pdf cover book: %w", err)
	}
	if format != "pdf" {
		return ErrUnsupportedFormat
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin pdf cover: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO pdf_covers(book_id, media_type, width, height, image_data)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(book_id) DO UPDATE SET
			media_type = excluded.media_type,
			width = excluded.width,
			height = excluded.height,
			image_data = excluded.image_data,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
	`, bookID, mediaType, width, height, data); err != nil {
		return fmt.Errorf("store pdf cover: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE books SET updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, bookID); err != nil {
		return fmt.Errorf("touch pdf cover book: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pdf cover: %w", err)
	}
	return nil
}

func (s *Service) OpenPDFCover(ctx context.Context, bookID int64) (PDFCoverAsset, error) {
	var mediaType, updatedRaw string
	var width, height int
	var data []byte
	if err := s.db.QueryRowContext(ctx, `
		SELECT media_type, width, height, image_data, updated_at
		FROM pdf_covers WHERE book_id = ?
	`, bookID).Scan(&mediaType, &width, &height, &data, &updatedRaw); errors.Is(err, sql.ErrNoRows) {
		return PDFCoverAsset{}, ErrPageNotFound
	} else if err != nil {
		return PDFCoverAsset{}, fmt.Errorf("open pdf cover: %w", err)
	}
	updated := parseSQLiteTime(updatedRaw)
	return PDFCoverAsset{
		Stream: io.NopCloser(bytes.NewReader(data)), MediaType: mediaType, Size: int64(len(data)),
		Width: width, Height: height, UpdatedAt: updated,
		ETag: fmt.Sprintf(`"pdf-cover-%d-%d-%d"`, bookID, len(data), updated.UnixNano()),
	}, nil
}

func (s *Service) SearchPDF(ctx context.Context, bookID int64, query string, limit int) ([]PDFSearchResult, error) {
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) < 2 || utf8.RuneCountInString(query) > 200 {
		return nil, ErrInvalidBook
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	var format, analysisStatus string
	var indexedPages int
	if err := s.db.QueryRowContext(ctx, `SELECT format, pdf_analysis_status, pdf_indexed_pages FROM books WHERE id = ? AND status = 'ready'`, bookID).Scan(&format, &analysisStatus, &indexedPages); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBookNotFound
	} else if err != nil {
		return nil, fmt.Errorf("find pdf search book: %w", err)
	}
	if format != "pdf" {
		return nil, ErrUnsupportedFormat
	}
	if analysisStatus != "complete" && indexedPages == 0 {
		return []PDFSearchResult{}, nil
	}
	normalizedQuery := normalizePDFSearchText(query)
	if normalizedQuery == "" || len(tokenizePDFSearch(query)) == 0 {
		return nil, ErrInvalidBook
	}

	// Candidate pages are narrowed with exact normalized boundaries. Native PDF
	// pages are also included because a visible word may be split into two text
	// tokens and needs the conservative repair pass below. Matching in Go is
	// intentional: SQLite's instr() cannot express Unicode-aware word boundaries,
	// and the old page-wide compact index could turn "poet died" into a false
	// match for "love".
	rows, err := s.db.QueryContext(ctx, `
		SELECT page_number, text_content, text_source
		FROM pdf_pages
		WHERE book_id = ?
		  AND (instr(' ' || search_content || ' ', ' ' || ? || ' ') > 0 OR text_source = 'pdf')
		ORDER BY page_number
	`, bookID, normalizedQuery)
	if err != nil {
		return nil, fmt.Errorf("search pdf text: %w", err)
	}
	defer rows.Close()

	results := make([]PDFSearchResult, 0, min(limit, 16))
	for rows.Next() {
		var pageNumber int
		var text, source string
		if err := rows.Scan(&pageNumber, &text, &source); err != nil {
			return nil, fmt.Errorf("scan pdf search result: %w", err)
		}
		matches := findPDFSearchMatches(text, query, source)
		if len(matches) == 0 {
			continue
		}
		best := matches[0]
		matchType := "repaired"
		for _, match := range matches {
			if match.Kind == "exact" {
				matchType = "exact"
				best = match
				break
			}
		}
		results = append(results, PDFSearchResult{
			PageNumber: pageNumber,
			Excerpt:    pdfSearchExcerptAt(text, best),
			Matches:    len(matches),
			MatchType:  matchType,
			TextSource: source,
		})
		if len(results) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pdf search results: %w", err)
	}
	return results, nil
}

type pdfSearchToken struct {
	Value string
	Start int // rune offset in the original text
	End   int // exclusive rune offset in the original text
}

type pdfSearchMatch struct {
	Start int
	End   int
	Kind  string
}

func tokenizePDFSearch(value string) []pdfSearchToken {
	runes := []rune(strings.ReplaceAll(value, "\x00", ""))
	tokens := make([]pdfSearchToken, 0, len(runes)/5)
	var builder strings.Builder
	start := -1
	flush := func(end int) {
		if start < 0 || builder.Len() == 0 {
			start = -1
			builder.Reset()
			return
		}
		tokens = append(tokens, pdfSearchToken{Value: builder.String(), Start: start, End: end})
		start = -1
		builder.Reset()
	}
	for index, character := range runes {
		character = unicode.ToLower(character)
		folded := foldPDFSearchRune(character)
		if folded != "" {
			if start < 0 {
				start = index
			}
			builder.WriteString(folded)
			continue
		}
		if unicode.IsLetter(character) || unicode.IsNumber(character) {
			if start < 0 {
				start = index
			}
			builder.WriteRune(character)
			continue
		}
		flush(index)
	}
	flush(len(runes))
	return tokens
}

func findPDFSearchMatches(text, query, source string) []pdfSearchMatch {
	textTokens := tokenizePDFSearch(text)
	queryTokens := tokenizePDFSearch(query)
	if len(textTokens) == 0 || len(queryTokens) == 0 {
		return nil
	}
	matches := make([]pdfSearchMatch, 0)
	seen := make(map[[2]int]struct{})
	for start := 0; start < len(textTokens); start++ {
		if match, ok := matchPDFTokenSequence(textTokens, queryTokens, start, false); ok {
			key := [2]int{match.Start, match.End}
			seen[key] = struct{}{}
			matches = append(matches, match)
			continue
		}
		// OCR already produces words. Repairing its token boundaries would hide
		// recognition errors and create false positives. The fallback exists only
		// for native PDF layers that split one visible word into two text items.
		if source != "pdf" {
			continue
		}
		if match, ok := matchPDFTokenSequence(textTokens, queryTokens, start, true); ok {
			key := [2]int{match.Start, match.End}
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				matches = append(matches, match)
			}
		}
	}
	return matches
}

func matchPDFTokenSequence(textTokens, queryTokens []pdfSearchToken, start int, allowRepair bool) (pdfSearchMatch, bool) {
	cursor := start
	repaired := false
	for _, queryToken := range queryTokens {
		if cursor >= len(textTokens) {
			return pdfSearchMatch{}, false
		}
		if textTokens[cursor].Value == queryToken.Value {
			cursor++
			continue
		}
		if allowRepair && cursor+1 < len(textTokens) && plausiblePDFTokenRepair(textTokens[cursor].Value, textTokens[cursor+1].Value, queryToken.Value) {
			cursor += 2
			repaired = true
			continue
		}
		return pdfSearchMatch{}, false
	}
	kind := "exact"
	if repaired {
		kind = "repaired"
	}
	return pdfSearchMatch{Start: textTokens[start].Start, End: textTokens[cursor-1].End, Kind: kind}, true
}

func plausiblePDFTokenRepair(left, right, query string) bool {
	// Require a full-token equality. We never look for the query as a substring
	// of two adjacent words, which is what caused "poet died" => "love".
	if utf8.RuneCountInString(query) < 5 || utf8.RuneCountInString(left) < 2 || utf8.RuneCountInString(right) < 2 {
		return false
	}
	return left+right == query
}

func pdfSearchExcerptAt(text string, match pdfSearchMatch) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return ""
	}
	match.Start = max(0, min(match.Start, len(runes)))
	match.End = max(match.Start, min(match.End, len(runes)))
	start := max(0, match.Start-95)
	end := min(len(runes), match.End+135)
	for start > 0 && !unicode.IsSpace(runes[start-1]) && match.Start-start < 125 {
		start--
	}
	for end < len(runes) && !unicode.IsSpace(runes[end]) && end-match.End < 165 {
		end++
	}
	excerpt := strings.TrimSpace(string(runes[start:end]))
	if start > 0 {
		excerpt = "…" + excerpt
	}
	if end < len(runes) {
		excerpt += "…"
	}
	return excerpt
}

func normalizePDFSearchText(value string) string {
	value = strings.ToLower(strings.ReplaceAll(value, "\x00", ""))
	var builder strings.Builder
	builder.Grow(len(value))
	lastWasSpace := true
	for _, character := range value {
		folded := foldPDFSearchRune(character)
		if folded != "" {
			builder.WriteString(folded)
			lastWasSpace = false
			continue
		}
		if unicode.IsLetter(character) || unicode.IsNumber(character) {
			builder.WriteRune(character)
			lastWasSpace = false
			continue
		}
		if !lastWasSpace {
			builder.WriteByte(' ')
			lastWasSpace = true
		}
	}
	return strings.TrimSpace(builder.String())
}

func foldPDFSearchRune(character rune) string {
	switch character {
	case 'à', 'á', 'â', 'ã', 'ä', 'å', 'ā', 'ă', 'ą':
		return "a"
	case 'æ':
		return "ae"
	case 'ç', 'ć', 'č':
		return "c"
	case 'ď', 'đ':
		return "d"
	case 'è', 'é', 'ê', 'ë', 'ē', 'ė', 'ę':
		return "e"
	case 'ì', 'í', 'î', 'ï', 'ī', 'į':
		return "i"
	case 'ł':
		return "l"
	case 'ñ', 'ń':
		return "n"
	case 'ò', 'ó', 'ô', 'õ', 'ö', 'ø', 'ō', 'ő':
		return "o"
	case 'œ':
		return "oe"
	case 'ř':
		return "r"
	case 'ś', 'š', 'ş':
		return "s"
	case 'ß':
		return "ss"
	case 'ť':
		return "t"
	case 'ù', 'ú', 'û', 'ü', 'ū', 'ů', 'ű':
		return "u"
	case 'ý', 'ÿ':
		return "y"
	case 'ž', 'ź', 'ż':
		return "z"
	default:
		return ""
	}
}

func countPDFMatches(text, query string) int {
	return len(findPDFSearchMatches(text, query, "pdf"))
}

func pdfSearchExcerpt(text, query string) string {
	matches := findPDFSearchMatches(text, query, "pdf")
	if len(matches) > 0 {
		return pdfSearchExcerptAt(text, matches[0])
	}
	runes := []rune(normalizePDFText(text))
	if len(runes) > 220 {
		return string(runes[:220]) + "…"
	}
	return string(runes)
}

func normalizePDFText(value string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(value, "\x00", "")), " ")
}

func limitPDFMetadata(value string, maxRunes int) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\x00", ""))
	runes := []rune(value)
	if len(runes) > maxRunes {
		value = string(runes[:maxRunes])
	}
	return value
}

func countFold(text, query string) int {
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	count := 0
	for {
		index := strings.Index(lowerText, lowerQuery)
		if index < 0 {
			break
		}
		count++
		lowerText = lowerText[index+len(lowerQuery):]
	}
	return count
}
