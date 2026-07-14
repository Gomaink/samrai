package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrUnsupportedFormat  = errors.New("unsupported book format")
	ErrAnnotationNotFound = errors.New("annotation not found")
	ErrInvalidAnnotation  = errors.New("invalid annotation")
)

type BookFile struct {
	File      *os.File
	Filename  string
	MediaType string
	Size      int64
	ModTime   time.Time
	ETag      string
}

type AnnotationRect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type AnnotationAnchor struct {
	Rects  []AnnotationRect `json:"rects"`
	Prefix string           `json:"prefix,omitempty"`
	Suffix string           `json:"suffix,omitempty"`
}

type Annotation struct {
	ID           int64            `json:"id"`
	BookID       int64            `json:"book_id"`
	PageNumber   int              `json:"page_number"`
	Kind         string           `json:"kind"`
	Color        string           `json:"color"`
	SelectedText string           `json:"selected_text"`
	Note         string           `json:"note"`
	Anchor       AnnotationAnchor `json:"anchor"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

type CreateAnnotationInput struct {
	PageNumber   int
	Kind         string
	Color        string
	SelectedText string
	Note         string
	Anchor       AnnotationAnchor
}

type UpdateAnnotationInput struct {
	Color *string
	Note  *string
}

func (s *Service) OpenBookFile(ctx context.Context, bookID int64) (BookFile, error) {
	var path, filename, hash, format string
	var size int64
	err := s.db.QueryRowContext(ctx, `
		SELECT file_path, original_filename, file_hash, file_size, format
		FROM books WHERE id = ? AND status = 'ready'
	`, bookID).Scan(&path, &filename, &hash, &size, &format)
	if errors.Is(err, sql.ErrNoRows) {
		return BookFile{}, ErrBookNotFound
	}
	if err != nil {
		return BookFile{}, fmt.Errorf("find book file: %w", err)
	}
	if format != "pdf" {
		return BookFile{}, ErrUnsupportedFormat
	}
	file, err := os.Open(path)
	if err != nil {
		return BookFile{}, fmt.Errorf("open book file: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return BookFile{}, fmt.Errorf("stat book file: %w", err)
	}
	return BookFile{
		File:      file,
		Filename:  filepath.Base(filename),
		MediaType: "application/pdf",
		Size:      size,
		ModTime:   info.ModTime(),
		ETag:      `"` + hash + `"`,
	}, nil
}

func (s *Service) UpdatePDFPageCount(ctx context.Context, bookID int64, pageCount int) error {
	if pageCount < 1 || pageCount > 100_000 {
		return ErrInvalidBook
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE books
		SET page_count = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ? AND status = 'ready' AND format = 'pdf'
	`, pageCount, bookID)
	if err != nil {
		return fmt.Errorf("update pdf page count: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read updated pdf rows: %w", err)
	}
	if rows == 0 {
		var exists int
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM books WHERE id = ? AND status = 'ready')`, bookID).Scan(&exists); err != nil {
			return fmt.Errorf("check pdf book: %w", err)
		}
		if exists == 0 {
			return ErrBookNotFound
		}
		return ErrUnsupportedFormat
	}
	return nil
}

func (s *Service) ListAnnotations(ctx context.Context, userID, bookID int64) ([]Annotation, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM books WHERE id = ? AND status = 'ready' AND format = 'pdf')`, bookID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check annotation book: %w", err)
	}
	if exists == 0 {
		return nil, ErrBookNotFound
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, book_id, page_number, kind, color, selected_text, note, anchor_json, created_at, updated_at
		FROM annotations
		WHERE user_id = ? AND book_id = ?
		ORDER BY page_number, id
	`, userID, bookID)
	if err != nil {
		return nil, fmt.Errorf("list annotations: %w", err)
	}
	defer rows.Close()
	items := make([]Annotation, 0)
	for rows.Next() {
		annotation, err := scanAnnotation(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, annotation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate annotations: %w", err)
	}
	return items, nil
}

func (s *Service) CreateAnnotation(ctx context.Context, userID, bookID int64, input CreateAnnotationInput) (Annotation, error) {
	if err := validateAnnotationInput(input); err != nil {
		return Annotation{}, err
	}
	var pageCount int
	var format string
	err := s.db.QueryRowContext(ctx, `SELECT page_count, format FROM books WHERE id = ? AND status = 'ready'`, bookID).Scan(&pageCount, &format)
	if errors.Is(err, sql.ErrNoRows) {
		return Annotation{}, ErrBookNotFound
	}
	if err != nil {
		return Annotation{}, fmt.Errorf("find annotation book: %w", err)
	}
	if format != "pdf" {
		return Annotation{}, ErrUnsupportedFormat
	}
	if input.PageNumber < 0 || input.PageNumber >= pageCount {
		return Annotation{}, ErrPageOutOfRange
	}
	anchorJSON, err := json.Marshal(input.Anchor)
	if err != nil {
		return Annotation{}, ErrInvalidAnnotation
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO annotations(user_id, book_id, page_number, kind, color, selected_text, note, anchor_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, bookID, input.PageNumber, input.Kind, input.Color, strings.TrimSpace(input.SelectedText), strings.TrimSpace(input.Note), string(anchorJSON))
	if err != nil {
		return Annotation{}, fmt.Errorf("create annotation: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Annotation{}, fmt.Errorf("read annotation id: %w", err)
	}
	return s.getAnnotation(ctx, userID, id)
}

func (s *Service) UpdateAnnotation(ctx context.Context, userID, annotationID int64, input UpdateAnnotationInput) (Annotation, error) {
	sets := make([]string, 0, 3)
	args := make([]any, 0, 4)
	if input.Color != nil {
		color := strings.TrimSpace(*input.Color)
		if !validAnnotationColor(color) {
			return Annotation{}, ErrInvalidAnnotation
		}
		sets = append(sets, "color = ?")
		args = append(args, color)
	}
	if input.Note != nil {
		note := strings.TrimSpace(*input.Note)
		if len([]rune(note)) > 20_000 {
			return Annotation{}, ErrInvalidAnnotation
		}
		sets = append(sets, "note = ?")
		args = append(args, note)
	}
	if len(sets) == 0 {
		return s.getAnnotation(ctx, userID, annotationID)
	}
	sets = append(sets, "updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')")
	args = append(args, annotationID, userID)
	result, err := s.db.ExecContext(ctx, `UPDATE annotations SET `+strings.Join(sets, ", ")+` WHERE id = ? AND user_id = ?`, args...)
	if err != nil {
		return Annotation{}, fmt.Errorf("update annotation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return Annotation{}, fmt.Errorf("read updated annotation rows: %w", err)
	}
	if rows == 0 {
		return Annotation{}, ErrAnnotationNotFound
	}
	return s.getAnnotation(ctx, userID, annotationID)
}

func (s *Service) DeleteAnnotation(ctx context.Context, userID, annotationID int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM annotations WHERE id = ? AND user_id = ?`, annotationID, userID)
	if err != nil {
		return fmt.Errorf("delete annotation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted annotation rows: %w", err)
	}
	if rows == 0 {
		return ErrAnnotationNotFound
	}
	return nil
}

func (s *Service) getAnnotation(ctx context.Context, userID, annotationID int64) (Annotation, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, book_id, page_number, kind, color, selected_text, note, anchor_json, created_at, updated_at
		FROM annotations WHERE id = ? AND user_id = ?
	`, annotationID, userID)
	annotation, err := scanAnnotation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Annotation{}, ErrAnnotationNotFound
	}
	return annotation, err
}

type annotationScanner interface {
	Scan(dest ...any) error
}

func scanAnnotation(scanner annotationScanner) (Annotation, error) {
	var annotation Annotation
	var anchorJSON, createdAt, updatedAt string
	if err := scanner.Scan(&annotation.ID, &annotation.BookID, &annotation.PageNumber, &annotation.Kind, &annotation.Color, &annotation.SelectedText, &annotation.Note, &anchorJSON, &createdAt, &updatedAt); err != nil {
		return Annotation{}, err
	}
	if err := json.Unmarshal([]byte(anchorJSON), &annotation.Anchor); err != nil {
		return Annotation{}, fmt.Errorf("decode annotation anchor: %w", err)
	}
	annotation.CreatedAt = parseSQLiteTime(createdAt)
	annotation.UpdatedAt = parseSQLiteTime(updatedAt)
	return annotation, nil
}

func validateAnnotationInput(input CreateAnnotationInput) error {
	if input.Kind != "highlight" && input.Kind != "area" {
		return ErrInvalidAnnotation
	}
	if !validAnnotationColor(input.Color) || len(input.Anchor.Rects) == 0 || len(input.Anchor.Rects) > 200 {
		return ErrInvalidAnnotation
	}
	if input.Kind == "highlight" && strings.TrimSpace(input.SelectedText) == "" {
		return ErrInvalidAnnotation
	}
	if len([]rune(input.SelectedText)) > 20_000 || len([]rune(input.Note)) > 20_000 || len([]rune(input.Anchor.Prefix)) > 500 || len([]rune(input.Anchor.Suffix)) > 500 {
		return ErrInvalidAnnotation
	}
	for _, rect := range input.Anchor.Rects {
		if rect.X < 0 || rect.Y < 0 || rect.Width <= 0 || rect.Height <= 0 || rect.X+rect.Width > 1.001 || rect.Y+rect.Height > 1.001 {
			return ErrInvalidAnnotation
		}
	}
	return nil
}

func validAnnotationColor(color string) bool {
	switch color {
	case "yellow", "green", "blue", "pink", "orange":
		return true
	default:
		return false
	}
}
