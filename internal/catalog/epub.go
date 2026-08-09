package catalog

import (
	"archive/zip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode"
)

type EPUBSpineItem struct {
	Index      int    `json:"index"`
	ItemID     string `json:"item_id"`
	Path       string `json:"path"`
	MediaType  string `json:"media_type"`
	Properties string `json:"properties"`
	Linear     bool   `json:"linear"`
	Title      string `json:"title"`
	ContentURL string `json:"content_url"`
	Searchable bool   `json:"searchable"`
}

type EPUBTOCItem struct {
	Position   int    `json:"position"`
	Label      string `json:"label"`
	Path       string `json:"path"`
	Fragment   string `json:"fragment"`
	SpineIndex *int   `json:"spine_index,omitempty"`
	Depth      int    `json:"depth"`
}

type EPUBPublication struct {
	BookID           int64           `json:"book_id"`
	Layout           string          `json:"layout"`
	Version          string          `json:"version"`
	ReadingDirection string          `json:"reading_direction"`
	Spine            []EPUBSpineItem `json:"spine"`
	TOC              []EPUBTOCItem   `json:"toc"`
}

type EPUBResourceAsset struct {
	Stream       io.ReadCloser
	MediaType    string
	Size         int64
	ETag         string
	ResourcePath string
	FileHash     string
}

type EPUBSearchResult struct {
	SpineIndex int    `json:"spine_index"`
	Title      string `json:"title"`
	Excerpt    string `json:"excerpt"`
	Matches    int    `json:"matches"`
}

func (s *Service) GetEPUBPublication(ctx context.Context, bookID int64) (EPUBPublication, error) {
	var publication EPUBPublication
	var direction sql.NullString
	var format string
	publication.BookID = bookID
	if err := s.db.QueryRowContext(ctx, `
		SELECT format, epub_layout, epub_version, reading_direction
		FROM books WHERE id = ? AND status = 'ready'
	`, bookID).Scan(&format, &publication.Layout, &publication.Version, &direction); errors.Is(err, sql.ErrNoRows) {
		return EPUBPublication{}, ErrBookNotFound
	} else if err != nil {
		return EPUBPublication{}, fmt.Errorf("get epub publication: %w", err)
	}
	if format != "epub" {
		return EPUBPublication{}, ErrUnsupportedFormat
	}
	publication.ReadingDirection = "ltr"
	if direction.Valid && direction.String == "rtl" {
		publication.ReadingDirection = "rtl"
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT spine_index, item_id, resource_path, media_type, properties, linear, title,
			CASE WHEN trim(text_content) = '' THEN 0 ELSE 1 END
		FROM epub_spine WHERE book_id = ? ORDER BY spine_index
	`, bookID)
	if err != nil {
		return EPUBPublication{}, fmt.Errorf("list epub spine: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item EPUBSpineItem
		var linear, searchable int
		if err := rows.Scan(&item.Index, &item.ItemID, &item.Path, &item.MediaType, &item.Properties, &linear, &item.Title, &searchable); err != nil {
			return EPUBPublication{}, fmt.Errorf("scan epub spine: %w", err)
		}
		item.Linear = linear == 1
		item.Searchable = searchable == 1
		item.ContentURL = epubContentURL(bookID, item.Path)
		publication.Spine = append(publication.Spine, item)
	}
	if err := rows.Err(); err != nil {
		return EPUBPublication{}, fmt.Errorf("iterate epub spine: %w", err)
	}
	if len(publication.Spine) == 0 {
		return EPUBPublication{}, ErrInvalidBook
	}

	tocRows, err := s.db.QueryContext(ctx, `
		SELECT position, label, resource_path, fragment, spine_index, depth
		FROM epub_toc WHERE book_id = ? ORDER BY position
	`, bookID)
	if err != nil {
		return EPUBPublication{}, fmt.Errorf("list epub toc: %w", err)
	}
	defer tocRows.Close()
	for tocRows.Next() {
		var item EPUBTOCItem
		var spineIndex sql.NullInt64
		if err := tocRows.Scan(&item.Position, &item.Label, &item.Path, &item.Fragment, &spineIndex, &item.Depth); err != nil {
			return EPUBPublication{}, fmt.Errorf("scan epub toc: %w", err)
		}
		if spineIndex.Valid {
			value := int(spineIndex.Int64)
			item.SpineIndex = &value
		}
		publication.TOC = append(publication.TOC, item)
	}
	if err := tocRows.Err(); err != nil {
		return EPUBPublication{}, fmt.Errorf("iterate epub toc: %w", err)
	}
	return publication, nil
}

func (s *Service) OpenEPUBResource(ctx context.Context, bookID int64, resourcePath string) (EPUBResourceAsset, error) {
	resourcePath, err := cleanEPUBResourcePath(resourcePath)
	if err != nil {
		return EPUBResourceAsset{}, ErrPageNotFound
	}
	var filePath, fileHash, format, mediaType string
	var size int64
	err = s.db.QueryRowContext(ctx, `
		SELECT b.file_path, b.file_hash, b.format, r.media_type, r.file_size
		FROM books b
		JOIN epub_resources r ON r.book_id = b.id AND r.resource_path = ?
		WHERE b.id = ? AND b.status = 'ready'
	`, resourcePath, bookID).Scan(&filePath, &fileHash, &format, &mediaType, &size)
	if errors.Is(err, sql.ErrNoRows) {
		var exists int
		if checkErr := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM books WHERE id = ? AND status = 'ready')`, bookID).Scan(&exists); checkErr != nil {
			return EPUBResourceAsset{}, fmt.Errorf("check epub book: %w", checkErr)
		}
		if exists == 0 {
			return EPUBResourceAsset{}, ErrBookNotFound
		}
		return EPUBResourceAsset{}, ErrPageNotFound
	}
	if err != nil {
		return EPUBResourceAsset{}, fmt.Errorf("find epub resource: %w", err)
	}
	if format != "epub" {
		return EPUBResourceAsset{}, ErrUnsupportedFormat
	}

	archive, err := zip.OpenReader(filePath)
	if err != nil {
		return EPUBResourceAsset{}, fmt.Errorf("open epub archive: %w", err)
	}
	for _, entry := range archive.File {
		if path.Clean(entry.Name) != resourcePath {
			continue
		}
		stream, err := entry.Open()
		if err != nil {
			archive.Close()
			return EPUBResourceAsset{}, fmt.Errorf("open epub resource: %w", err)
		}
		return EPUBResourceAsset{
			Stream:       &archiveEntryStream{ReadCloser: stream, archive: archive},
			MediaType:    mediaType,
			Size:         size,
			ETag:         `"` + fileHash + "-epub-" + strconv.FormatUint(uint64(entry.CRC32), 16) + `"`,
			ResourcePath: resourcePath,
			FileHash:     fileHash,
		}, nil
	}
	archive.Close()
	return EPUBResourceAsset{}, ErrPageNotFound
}

func (s *Service) OpenEPUBCover(ctx context.Context, bookID int64) (EPUBResourceAsset, error) {
	var coverPath, format string
	err := s.db.QueryRowContext(ctx, `SELECT epub_cover_path, format FROM books WHERE id = ? AND status = 'ready'`, bookID).Scan(&coverPath, &format)
	if errors.Is(err, sql.ErrNoRows) {
		return EPUBResourceAsset{}, ErrBookNotFound
	}
	if err != nil {
		return EPUBResourceAsset{}, fmt.Errorf("find epub cover: %w", err)
	}
	if format != "epub" {
		return EPUBResourceAsset{}, ErrUnsupportedFormat
	}
	if strings.TrimSpace(coverPath) == "" {
		return EPUBResourceAsset{}, ErrPageNotFound
	}
	return s.OpenEPUBResource(ctx, bookID, coverPath)
}

func (s *Service) SearchEPUB(ctx context.Context, bookID int64, query string, limit int) ([]EPUBSearchResult, error) {
	queryTokens := tokenizeEPUBSearch(query)
	if len(queryTokens) == 0 || len([]rune(strings.TrimSpace(query))) < 2 {
		return nil, ErrInvalidBook
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	var format string
	if err := s.db.QueryRowContext(ctx, `SELECT format FROM books WHERE id = ? AND status = 'ready'`, bookID).Scan(&format); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBookNotFound
	} else if err != nil {
		return nil, fmt.Errorf("find epub for search: %w", err)
	}
	if format != "epub" {
		return nil, ErrUnsupportedFormat
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT spine_index, title, text_content
		FROM epub_spine WHERE book_id = ? AND trim(text_content) <> ''
		ORDER BY spine_index
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("query epub search text: %w", err)
	}
	defer rows.Close()
	results := make([]EPUBSearchResult, 0)
	for rows.Next() {
		var spineIndex int
		var title, text string
		if err := rows.Scan(&spineIndex, &title, &text); err != nil {
			return nil, fmt.Errorf("scan epub search text: %w", err)
		}
		words := epubWordTokens(text)
		positions := findEPUBTokenSequence(words, queryTokens)
		if len(positions) == 0 {
			continue
		}
		results = append(results, EPUBSearchResult{
			SpineIndex: spineIndex,
			Title:      title,
			Excerpt:    epubSearchExcerpt(words, positions[0], len(queryTokens)),
			Matches:    len(positions),
		})
		if len(results) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate epub search: %w", err)
	}
	return results, nil
}

const epubReaderResourceRevision = "4"

func epubContentURL(bookID int64, resourcePath string) string {
	segments := strings.Split(resourcePath, "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	// The revision query invalidates EPUB content documents cached with older
	// frame policies. Relative images, styles and fonts still resolve against
	// the same content directory.
	return fmt.Sprintf("/api/v1/books/%d/epub/content/%s?reader=%s", bookID, strings.Join(segments, "/"), epubReaderResourceRevision)
}

func cleanEPUBResourcePath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || strings.HasPrefix(value, "/") || strings.ContainsRune(value, '\x00') {
		return "", errors.New("invalid resource path")
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", errors.New("invalid resource path")
	}
	return cleaned, nil
}

type epubWord struct {
	Original   string
	Normalized string
}

func tokenizeEPUBSearch(value string) []string {
	words := epubWordTokens(value)
	result := make([]string, 0, len(words))
	for _, word := range words {
		if word.Normalized != "" {
			result = append(result, word.Normalized)
		}
	}
	return result
}

func epubWordTokens(value string) []epubWord {
	words := make([]epubWord, 0)
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		original := string(current)
		normalized := normalizeEPUBWord(original)
		if normalized != "" {
			words = append(words, epubWord{Original: original, Normalized: normalized})
		}
		current = current[:0]
	}
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsNumber(character) || character == '\'' || character == '’' || character == '-' {
			current = append(current, character)
		} else {
			flush()
		}
	}
	flush()
	return words
}

func normalizeEPUBWord(value string) string {
	value = strings.ToLower(value)
	value = strings.Map(func(character rune) rune {
		switch character {
		case 'á', 'à', 'â', 'ã', 'ä':
			return 'a'
		case 'é', 'è', 'ê', 'ë':
			return 'e'
		case 'í', 'ì', 'î', 'ï':
			return 'i'
		case 'ó', 'ò', 'ô', 'õ', 'ö':
			return 'o'
		case 'ú', 'ù', 'û', 'ü':
			return 'u'
		case 'ç':
			return 'c'
		case 'ñ':
			return 'n'
		case 'ý', 'ÿ':
			return 'y'
		case '\'', '’', '-':
			return -1
		default:
			if unicode.IsLetter(character) || unicode.IsNumber(character) {
				return character
			}
			return -1
		}
	}, value)
	return value
}

func findEPUBTokenSequence(words []epubWord, query []string) []int {
	if len(query) == 0 || len(words) < len(query) {
		return nil
	}
	positions := make([]int, 0)
	for start := 0; start+len(query) <= len(words); start++ {
		matched := true
		for offset := range query {
			if words[start+offset].Normalized != query[offset] {
				matched = false
				break
			}
		}
		if matched {
			positions = append(positions, start)
		}
	}
	return positions
}

func epubSearchExcerpt(words []epubWord, start, length int) string {
	from := max(0, start-12)
	to := min(len(words), start+length+16)
	parts := make([]string, 0, to-from)
	for _, word := range words[from:to] {
		parts = append(parts, word.Original)
	}
	excerpt := strings.Join(parts, " ")
	if from > 0 {
		excerpt = "…" + excerpt
	}
	if to < len(words) {
		excerpt += "…"
	}
	return excerpt
}
