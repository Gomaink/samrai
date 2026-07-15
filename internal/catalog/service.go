package catalog

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	ErrBookNotFound   = errors.New("book not found")
	ErrPageNotFound   = errors.New("page not found")
	ErrPageOutOfRange = errors.New("page number is out of range")
	ErrInvalidBook    = errors.New("invalid book data")
	ErrSeriesNotFound = errors.New("series not found")
)

type Service struct {
	db *sql.DB
}

type Book struct {
	ID                      int64           `json:"id"`
	Title                   string          `json:"title"`
	OriginalFilename        string          `json:"original_filename"`
	FileSize                int64           `json:"file_size"`
	PageCount               int             `json:"page_count"`
	Status                  string          `json:"status"`
	Format                  string          `json:"format"`
	PDFAnalysisStatus       string          `json:"pdf_analysis_status"`
	PDFTextLayer            string          `json:"pdf_text_layer"`
	PDFOCRStatus            string          `json:"pdf_ocr_status"`
	PDFInvalidPages         int             `json:"pdf_invalid_pages"`
	PDFOCRPages             int             `json:"pdf_ocr_pages"`
	PDFDocumentKind         string          `json:"pdf_document_kind"`
	PDFClassificationStatus string          `json:"pdf_classification_status"`
	PDFOCRMode              string          `json:"pdf_ocr_mode"`
	PDFSampledPages         int             `json:"pdf_sampled_pages"`
	PDFIndexedPages         int             `json:"pdf_indexed_pages"`
	PDFAnalyzedAt           *time.Time      `json:"pdf_analyzed_at,omitempty"`
	EPUBLayout              string          `json:"epub_layout"`
	EPUBVersion             string          `json:"epub_version"`
	SeriesID                *int64          `json:"series_id,omitempty"`
	Series                  string          `json:"series"`
	Summary                 string          `json:"summary"`
	Writer                  string          `json:"writer"`
	Publisher               string          `json:"publisher"`
	PublicationYear         *int            `json:"publication_year,omitempty"`
	Volume                  string          `json:"volume"`
	Number                  string          `json:"number"`
	Language                string          `json:"language"`
	ReadingDirection        *string         `json:"reading_direction,omitempty"`
	CurrentPage             int             `json:"current_page"`
	ReadingLocation         json.RawMessage `json:"reading_location,omitempty"`
	Started                 bool            `json:"started"`
	Completed               bool            `json:"completed"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
	CoverURL                string          `json:"cover_url"`
	Favorite                bool            `json:"favorite"`
}

type PageDescriptor struct {
	BookID      int64
	PageNumber  int
	FilePath    string
	FileHash    string
	ArchivePath string
	MediaType   string
	Size        int64
	Width       int
	Height      int
}

type PageAsset struct {
	Stream      io.ReadCloser
	MediaType   string
	Size        int64
	ETag        string
	ArchivePath string
	Width       int
	Height      int
	PageNumber  int
	FileHash    string
}

type Cover = PageAsset

type ReadingProgress struct {
	BookID      int64           `json:"book_id"`
	CurrentPage int             `json:"current_page"`
	Started     bool            `json:"started"`
	Completed   bool            `json:"completed"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Location    json.RawMessage `json:"location,omitempty"`
}

type UpdateBookInput struct {
	Title            *string
	Summary          *string
	Writer           *string
	Publisher        *string
	PublicationYear  *int
	ClearYear        bool
	Volume           *string
	Number           *string
	Language         *string
	ReadingDirection *string
	Series           *string
}

type DeletedBook struct {
	FilePath     string
	FileHash     string
	PreparedPath string
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

const bookColumns = `
	b.id, b.title, b.original_filename, b.file_size, b.page_count, b.status, b.format,
	b.pdf_analysis_status, b.pdf_text_layer, b.pdf_ocr_status, b.pdf_invalid_pages, b.pdf_ocr_pages,
	b.pdf_document_kind, b.pdf_classification_status, b.pdf_ocr_mode, b.pdf_sampled_pages, b.pdf_indexed_pages, b.pdf_analyzed_at,
	b.epub_layout, b.epub_version,
	s.id, COALESCE(s.title, ''), b.summary, b.writer, b.publisher, b.publication_year,
	b.volume, b.number, b.language, b.reading_direction,
	COALESCE(rp.current_page, 0), COALESCE(rp.location_json, '{}'), CASE WHEN rp.user_id IS NULL THEN 0 ELSE 1 END,
	COALESCE(rp.completed, 0), b.created_at, b.updated_at
`

func (s *Service) ListBooks(ctx context.Context, userID int64) ([]Book, error) {
	result, err := s.QueryBooks(ctx, userID, BookListOptions{Sort: "recent", Limit: 500})
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

func (s *Service) GetBook(ctx context.Context, userID, bookID int64) (Book, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+bookColumns+`
		FROM books b
		LEFT JOIN series s ON s.id = b.series_id
		LEFT JOIN reading_progress rp ON rp.book_id = b.id AND rp.user_id = ?
		WHERE b.id = ? AND b.status = 'ready'
	`, userID, bookID)

	book, err := scanBook(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Book{}, ErrBookNotFound
	}
	if err != nil {
		return Book{}, fmt.Errorf("get book: %w", err)
	}
	books := []Book{book}
	if err := s.markBookFavorites(ctx, userID, books); err != nil {
		return Book{}, err
	}
	return books[0], nil
}

func (s *Service) UpdateBook(ctx context.Context, userID, bookID int64, input UpdateBookInput) (Book, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Book{}, fmt.Errorf("begin update book: %w", err)
	}
	defer tx.Rollback()

	var libraryID int64
	if err := tx.QueryRowContext(ctx, `SELECT library_id FROM books WHERE id = ? AND status = 'ready'`, bookID).Scan(&libraryID); errors.Is(err, sql.ErrNoRows) {
		return Book{}, ErrBookNotFound
	} else if err != nil {
		return Book{}, fmt.Errorf("find book for update: %w", err)
	}

	sets := make([]string, 0, 11)
	args := make([]any, 0, 12)
	if input.Title != nil {
		value := strings.TrimSpace(*input.Title)
		if value == "" || len([]rune(value)) > 240 {
			return Book{}, ErrInvalidBook
		}
		sets = append(sets, "title = ?")
		args = append(args, value)
	}
	for column, value := range map[string]*string{
		"summary": input.Summary, "writer": input.Writer, "publisher": input.Publisher,
		"volume": input.Volume, "number": input.Number, "language": input.Language,
	} {
		if value == nil {
			continue
		}
		trimmed := strings.TrimSpace(*value)
		if len([]rune(trimmed)) > 20_000 {
			return Book{}, ErrInvalidBook
		}
		sets = append(sets, column+" = ?")
		args = append(args, trimmed)
	}
	if input.ClearYear {
		sets = append(sets, "publication_year = NULL")
	} else if input.PublicationYear != nil {
		if *input.PublicationYear < 0 || *input.PublicationYear > 9999 {
			return Book{}, ErrInvalidBook
		}
		sets = append(sets, "publication_year = ?")
		args = append(args, *input.PublicationYear)
	}
	if input.ReadingDirection != nil {
		value := strings.TrimSpace(*input.ReadingDirection)
		if value == "" {
			sets = append(sets, "reading_direction = NULL")
		} else {
			if value != "ltr" && value != "rtl" {
				return Book{}, ErrInvalidBook
			}
			sets = append(sets, "reading_direction = ?")
			args = append(args, value)
		}
	}
	if input.Series != nil {
		value := strings.TrimSpace(*input.Series)
		if len([]rune(value)) > 240 {
			return Book{}, ErrInvalidBook
		}
		if value == "" {
			sets = append(sets, "series_id = NULL")
		} else {
			seriesID, err := ensureCatalogSeries(ctx, tx, libraryID, value)
			if err != nil {
				return Book{}, err
			}
			sets = append(sets, "series_id = ?")
			args = append(args, seriesID)
		}
	}
	if len(sets) > 0 {
		sets = append(sets, "updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')")
		args = append(args, bookID)
		if _, err := tx.ExecContext(ctx, `UPDATE books SET `+strings.Join(sets, ", ")+` WHERE id = ? AND status = 'ready'`, args...); err != nil {
			return Book{}, fmt.Errorf("update book: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Book{}, fmt.Errorf("commit update book: %w", err)
	}
	return s.GetBook(ctx, userID, bookID)
}

func ensureCatalogSeries(ctx context.Context, tx *sql.Tx, libraryID int64, title string) (int64, error) {
	var seriesID int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM series WHERE library_id = ? AND title = ? COLLATE NOCASE LIMIT 1`, libraryID, title).Scan(&seriesID)
	if err == nil {
		return seriesID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("find series: %w", err)
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO series(library_id, title) VALUES (?, ?)`, libraryID, title)
	if err != nil {
		return 0, fmt.Errorf("create series: %w", err)
	}
	seriesID, err = result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read series id: %w", err)
	}
	return seriesID, nil
}

func (s *Service) DeleteBook(ctx context.Context, bookID int64) (DeletedBook, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DeletedBook{}, fmt.Errorf("begin delete book: %w", err)
	}
	defer tx.Rollback()

	var deleted DeletedBook
	if err := tx.QueryRowContext(ctx, `SELECT file_path, file_hash FROM books WHERE id = ?`, bookID).Scan(&deleted.FilePath, &deleted.FileHash); errors.Is(err, sql.ErrNoRows) {
		return DeletedBook{}, ErrBookNotFound
	} else if err != nil {
		return DeletedBook{}, fmt.Errorf("find book for deletion: %w", err)
	}
	deleted.PreparedPath = filepath.Join(filepath.Dir(filepath.Dir(deleted.FilePath)), "prepared", deleted.FileHash)
	if _, err := tx.ExecContext(ctx, `DELETE FROM books WHERE id = ?`, bookID); err != nil {
		return DeletedBook{}, fmt.Errorf("delete book: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return DeletedBook{}, fmt.Errorf("commit delete book: %w", err)
	}
	return deleted, nil
}

func (s *Service) SetCompletion(ctx context.Context, userID int64, bookIDs []int64, completed bool) (int, error) {
	unique := make([]int64, 0, len(bookIDs))
	seen := make(map[int64]struct{}, len(bookIDs))
	for _, bookID := range bookIDs {
		if bookID <= 0 {
			return 0, ErrInvalidBook
		}
		if _, exists := seen[bookID]; exists {
			continue
		}
		seen[bookID] = struct{}{}
		unique = append(unique, bookID)
	}
	if len(unique) == 0 || len(unique) > 500 {
		return 0, ErrInvalidBook
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin reading completion update: %w", err)
	}
	defer tx.Rollback()

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(unique)), ",")
	args := make([]any, len(unique))
	for index, bookID := range unique {
		args[index] = bookID
	}
	var found int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM books WHERE status = 'ready' AND id IN (`+placeholders+`)`, args...).Scan(&found); err != nil {
		return 0, fmt.Errorf("check books for reading completion update: %w", err)
	}
	if found != len(unique) {
		return 0, ErrBookNotFound
	}

	for _, bookID := range unique {
		if completed {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO reading_progress(user_id, book_id, current_page, completed, completed_at, location_json)
				VALUES (?, ?, 0, 1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{}')
				ON CONFLICT(user_id, book_id) DO UPDATE SET
					completed = 1,
					completed_at = COALESCE(reading_progress.completed_at, strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
					updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			`, userID, bookID); err != nil {
				return 0, fmt.Errorf("mark book as read: %w", err)
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE reading_progress
			SET completed = 0,
				completed_at = NULL,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			WHERE user_id = ? AND book_id = ?
		`, userID, bookID); err != nil {
			return 0, fmt.Errorf("mark book as unread: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit reading completion update: %w", err)
	}
	return len(unique), nil
}

func (s *Service) ResetProgress(ctx context.Context, userID, bookID int64) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM books WHERE id = ? AND status = 'ready')`, bookID).Scan(&exists); err != nil {
		return fmt.Errorf("check book for progress reset: %w", err)
	}
	if exists == 0 {
		return ErrBookNotFound
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM reading_progress WHERE user_id = ? AND book_id = ?`, userID, bookID); err != nil {
		return fmt.Errorf("reset reading progress: %w", err)
	}
	return nil
}

func (s *Service) OpenCover(ctx context.Context, bookID int64) (Cover, error) {
	asset, err := s.OpenPage(ctx, bookID, 0)
	if errors.Is(err, ErrPageNotFound) {
		return Cover{}, ErrBookNotFound
	}
	return asset, err
}

func (s *Service) DescribePage(ctx context.Context, bookID int64, pageNumber int) (PageDescriptor, error) {
	if pageNumber < 0 {
		return PageDescriptor{}, ErrPageOutOfRange
	}
	var descriptor PageDescriptor
	descriptor.BookID = bookID
	descriptor.PageNumber = pageNumber
	err := s.db.QueryRowContext(ctx, `
		SELECT b.file_path, b.file_hash, p.archive_path, p.media_type,
			p.file_size, p.width, p.height
		FROM books b
		JOIN pages p ON p.book_id = b.id AND p.page_number = ?
		WHERE b.id = ? AND b.status = 'ready'
	`, pageNumber, bookID).Scan(
		&descriptor.FilePath, &descriptor.FileHash, &descriptor.ArchivePath,
		&descriptor.MediaType, &descriptor.Size, &descriptor.Width, &descriptor.Height,
	)
	if errors.Is(err, sql.ErrNoRows) {
		var bookExists int
		if checkErr := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM books WHERE id = ? AND status = 'ready')`, bookID).Scan(&bookExists); checkErr != nil {
			return PageDescriptor{}, fmt.Errorf("check book for page: %w", checkErr)
		}
		if bookExists == 0 {
			return PageDescriptor{}, ErrBookNotFound
		}
		return PageDescriptor{}, ErrPageNotFound
	}
	if err != nil {
		return PageDescriptor{}, fmt.Errorf("find page: %w", err)
	}
	return descriptor, nil
}

func (s *Service) OpenPage(ctx context.Context, bookID int64, pageNumber int) (PageAsset, error) {
	descriptor, err := s.DescribePage(ctx, bookID, pageNumber)
	if err != nil {
		return PageAsset{}, err
	}
	return s.OpenPageDescriptor(descriptor)
}

func (s *Service) OpenPageDescriptor(descriptor PageDescriptor) (PageAsset, error) {
	if strings.HasPrefix(descriptor.ArchivePath, preparedPathPrefix) {
		return openPreparedPage(descriptor)
	}
	archive, err := zip.OpenReader(descriptor.FilePath)
	if err != nil {
		return PageAsset{}, fmt.Errorf("open book archive: %w", err)
	}
	for _, entry := range archive.File {
		if entry.Name != descriptor.ArchivePath {
			continue
		}
		stream, err := entry.Open()
		if err != nil {
			archive.Close()
			return PageAsset{}, fmt.Errorf("open page entry: %w", err)
		}
		return PageAsset{
			Stream:      &archiveEntryStream{ReadCloser: stream, archive: archive},
			MediaType:   descriptor.MediaType,
			Size:        descriptor.Size,
			ETag:        `"` + descriptor.FileHash + `-page-` + strconv.Itoa(descriptor.PageNumber) + `"`,
			ArchivePath: descriptor.ArchivePath,
			Width:       descriptor.Width,
			Height:      descriptor.Height,
			PageNumber:  descriptor.PageNumber,
			FileHash:    descriptor.FileHash,
		}, nil
	}
	archive.Close()
	return PageAsset{}, ErrPageNotFound
}

func (s *Service) SaveProgress(ctx context.Context, userID, bookID int64, currentPage int, location json.RawMessage) (ReadingProgress, error) {
	var pageCount int
	var format string
	err := s.db.QueryRowContext(ctx, `SELECT page_count, format FROM books WHERE id = ? AND status = 'ready'`, bookID).Scan(&pageCount, &format)
	if errors.Is(err, sql.ErrNoRows) {
		return ReadingProgress{}, ErrBookNotFound
	}
	if err != nil {
		return ReadingProgress{}, fmt.Errorf("find book for progress: %w", err)
	}
	if currentPage < 0 || currentPage >= pageCount {
		return ReadingProgress{}, ErrPageOutOfRange
	}
	locationJSON := "{}"
	if len(location) > 0 {
		if len(location) > 4096 || !json.Valid(location) {
			return ReadingProgress{}, ErrInvalidBook
		}
		var object map[string]any
		if err := json.Unmarshal(location, &object); err != nil || object == nil {
			return ReadingProgress{}, ErrInvalidBook
		}
		encoded, err := json.Marshal(object)
		if err != nil {
			return ReadingProgress{}, ErrInvalidBook
		}
		locationJSON = string(encoded)
	}

	completed := currentPage == pageCount-1
	if completed && format == "epub" {
		completed = epubLocationAtEnd(locationJSON)
	}
	completedValue := 0
	if completed {
		completedValue = 1
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO reading_progress(user_id, book_id, current_page, completed, completed_at, location_json)
		VALUES (?, ?, ?, ?, CASE WHEN ? = 1 THEN strftime('%Y-%m-%dT%H:%M:%fZ', 'now') ELSE NULL END, ?)
		ON CONFLICT(user_id, book_id) DO UPDATE SET
			current_page = excluded.current_page,
			location_json = excluded.location_json,
			completed = CASE WHEN reading_progress.completed = 1 OR excluded.completed = 1 THEN 1 ELSE 0 END,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
			completed_at = CASE
				WHEN reading_progress.completed_at IS NOT NULL THEN reading_progress.completed_at
				WHEN excluded.completed = 1 THEN strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
				ELSE NULL
			END
	`, userID, bookID, currentPage, completedValue, completedValue, locationJSON)
	if err != nil {
		return ReadingProgress{}, fmt.Errorf("save reading progress: %w", err)
	}

	var progress ReadingProgress
	var completedInt int
	var updatedRaw, savedLocation string
	err = s.db.QueryRowContext(ctx, `
		SELECT book_id, current_page, completed, updated_at, location_json
		FROM reading_progress WHERE user_id = ? AND book_id = ?
	`, userID, bookID).Scan(&progress.BookID, &progress.CurrentPage, &completedInt, &updatedRaw, &savedLocation)
	if err != nil {
		return ReadingProgress{}, fmt.Errorf("read saved progress: %w", err)
	}
	progress.Started = true
	progress.Completed = completedInt == 1
	progress.UpdatedAt = parseSQLiteTime(updatedRaw)
	progress.Location = json.RawMessage(savedLocation)
	return progress, nil
}

func epubLocationAtEnd(locationJSON string) bool {
	var location struct {
		ColumnIndex int     `json:"column_index"`
		ColumnCount int     `json:"column_count"`
		Progress    float64 `json:"chapter_progress"`
	}
	if err := json.Unmarshal([]byte(locationJSON), &location); err != nil {
		return false
	}
	if location.ColumnCount <= 1 {
		return true
	}
	return location.ColumnIndex >= location.ColumnCount-1 || location.Progress >= 0.999
}

type bookScanner interface {
	Scan(dest ...any) error
}

func scanBook(scanner bookScanner) (Book, error) {
	var book Book
	var direction sql.NullString
	var seriesID sql.NullInt64
	var year sql.NullInt64
	var started, completed int
	var createdAt, updatedAt string
	var pdfAnalyzedAt sql.NullString
	var readingLocation string
	if err := scanner.Scan(
		&book.ID, &book.Title, &book.OriginalFilename, &book.FileSize, &book.PageCount, &book.Status, &book.Format,
		&book.PDFAnalysisStatus, &book.PDFTextLayer, &book.PDFOCRStatus, &book.PDFInvalidPages, &book.PDFOCRPages,
		&book.PDFDocumentKind, &book.PDFClassificationStatus, &book.PDFOCRMode, &book.PDFSampledPages, &book.PDFIndexedPages, &pdfAnalyzedAt,
		&book.EPUBLayout, &book.EPUBVersion,
		&seriesID, &book.Series, &book.Summary, &book.Writer, &book.Publisher, &year,
		&book.Volume, &book.Number, &book.Language, &direction,
		&book.CurrentPage, &readingLocation, &started, &completed, &createdAt, &updatedAt,
	); err != nil {
		return Book{}, err
	}
	if seriesID.Valid {
		value := seriesID.Int64
		book.SeriesID = &value
	}
	if year.Valid {
		value := int(year.Int64)
		book.PublicationYear = &value
	}
	if direction.Valid {
		book.ReadingDirection = &direction.String
	}
	book.ReadingLocation = json.RawMessage(readingLocation)
	book.Started = started == 1
	book.Completed = completed == 1
	book.CreatedAt = parseSQLiteTime(createdAt)
	book.UpdatedAt = parseSQLiteTime(updatedAt)
	if pdfAnalyzedAt.Valid {
		value := parseSQLiteTime(pdfAnalyzedAt.String)
		book.PDFAnalyzedAt = &value
	}
	book.CoverURL = fmt.Sprintf("/api/v1/books/%d/cover?width=480&format=webp&v=%d", book.ID, book.UpdatedAt.UnixNano())
	return book, nil
}

const preparedPathPrefix = "prepared:"

func openPreparedPage(descriptor PageDescriptor) (PageAsset, error) {
	relative := strings.TrimPrefix(descriptor.ArchivePath, preparedPathPrefix)
	relative = filepath.Clean(filepath.FromSlash(relative))
	if relative == "." || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return PageAsset{}, ErrPageNotFound
	}
	dataDir := filepath.Dir(filepath.Dir(descriptor.FilePath))
	root := filepath.Join(dataDir, "prepared", descriptor.FileHash)
	target := filepath.Join(root, relative)
	resolved, err := filepath.Rel(root, target)
	if err != nil || resolved == ".." || strings.HasPrefix(resolved, ".."+string(filepath.Separator)) {
		return PageAsset{}, ErrPageNotFound
	}
	file, err := os.Open(target)
	if errors.Is(err, os.ErrNotExist) {
		return PageAsset{}, ErrPageNotFound
	}
	if err != nil {
		return PageAsset{}, fmt.Errorf("open prepared page: %w", err)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		if err != nil {
			return PageAsset{}, fmt.Errorf("stat prepared page: %w", err)
		}
		return PageAsset{}, ErrPageNotFound
	}
	return PageAsset{
		Stream: file, MediaType: descriptor.MediaType, Size: info.Size(),
		ETag:        `"` + descriptor.FileHash + `-page-` + strconv.Itoa(descriptor.PageNumber) + `"`,
		ArchivePath: descriptor.ArchivePath, Width: descriptor.Width, Height: descriptor.Height,
		PageNumber: descriptor.PageNumber, FileHash: descriptor.FileHash,
	}, nil
}

type archiveEntryStream struct {
	io.ReadCloser
	archive *zip.ReadCloser
}

func (s *archiveEntryStream) Close() error {
	entryErr := s.ReadCloser.Close()
	archiveErr := s.archive.Close()
	if entryErr != nil {
		return entryErr
	}
	return archiveErr
}

func parseSQLiteTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000Z", "2006-01-02T15:04:05Z"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
