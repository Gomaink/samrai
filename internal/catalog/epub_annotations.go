package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrEPUBAnnotationNotFound = errors.New("epub annotation not found")

type EPUBAnnotationAnchor struct {
	StartPath   string `json:"start_path"`
	StartOffset int    `json:"start_offset"`
	EndPath     string `json:"end_path"`
	EndOffset   int    `json:"end_offset"`
	Prefix      string `json:"prefix,omitempty"`
	Suffix      string `json:"suffix,omitempty"`
}

type EPUBAnnotation struct {
	ID           int64                `json:"id"`
	BookID       int64                `json:"book_id"`
	SpineIndex   int                  `json:"spine_index"`
	ResourcePath string               `json:"resource_path"`
	Kind         string               `json:"kind"`
	Color        string               `json:"color"`
	SelectedText string               `json:"selected_text"`
	Note         string               `json:"note"`
	Anchor       EPUBAnnotationAnchor `json:"anchor"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
}

type CreateEPUBAnnotationInput struct {
	SpineIndex   int
	ResourcePath string
	Kind         string
	Color        string
	SelectedText string
	Note         string
	Anchor       EPUBAnnotationAnchor
}

type UpdateEPUBAnnotationInput struct {
	Color *string
	Note  *string
}

func (s *Service) ListEPUBAnnotations(ctx context.Context, userID, bookID int64) ([]EPUBAnnotation, error) {
	var format string
	err := s.db.QueryRowContext(ctx, `SELECT format FROM books WHERE id = ? AND status = 'ready'`, bookID).Scan(&format)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBookNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find epub annotation book: %w", err)
	}
	if format != "epub" {
		return nil, ErrUnsupportedFormat
	}

	rows, err := s.db.QueryContext(ctx, `
        SELECT id, book_id, spine_index, resource_path, kind, color, selected_text, note,
               anchor_json, created_at, updated_at
        FROM epub_annotations
        WHERE user_id = ? AND book_id = ?
        ORDER BY spine_index, id
    `, userID, bookID)
	if err != nil {
		return nil, fmt.Errorf("list epub annotations: %w", err)
	}
	defer rows.Close()

	items := make([]EPUBAnnotation, 0)
	for rows.Next() {
		annotation, err := scanEPUBAnnotation(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, annotation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate epub annotations: %w", err)
	}
	return items, nil
}

func (s *Service) CreateEPUBAnnotation(ctx context.Context, userID, bookID int64, input CreateEPUBAnnotationInput) (EPUBAnnotation, error) {
	input.ResourcePath = strings.TrimSpace(input.ResourcePath)
	input.SelectedText = strings.TrimSpace(input.SelectedText)
	input.Note = strings.TrimSpace(input.Note)
	if err := validateEPUBAnnotationInput(input); err != nil {
		return EPUBAnnotation{}, err
	}

	var format, resourcePath string
	err := s.db.QueryRowContext(ctx, `
        SELECT b.format, COALESCE(es.resource_path, '')
        FROM books b
        LEFT JOIN epub_spine es ON es.book_id = b.id AND es.spine_index = ?
        WHERE b.id = ? AND b.status = 'ready'
    `, input.SpineIndex, bookID).Scan(&format, &resourcePath)
	if errors.Is(err, sql.ErrNoRows) {
		return EPUBAnnotation{}, ErrBookNotFound
	}
	if err != nil {
		return EPUBAnnotation{}, fmt.Errorf("find epub annotation spine: %w", err)
	}
	if format != "epub" {
		return EPUBAnnotation{}, ErrUnsupportedFormat
	}
	if resourcePath == "" || resourcePath != input.ResourcePath {
		return EPUBAnnotation{}, ErrInvalidAnnotation
	}

	anchorJSON, err := json.Marshal(input.Anchor)
	if err != nil {
		return EPUBAnnotation{}, ErrInvalidAnnotation
	}
	result, err := s.db.ExecContext(ctx, `
        INSERT INTO epub_annotations(
            user_id, book_id, spine_index, resource_path, kind, color,
            selected_text, note, anchor_json
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `, userID, bookID, input.SpineIndex, input.ResourcePath, input.Kind, input.Color,
		input.SelectedText, input.Note, string(anchorJSON))
	if err != nil {
		return EPUBAnnotation{}, fmt.Errorf("create epub annotation: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return EPUBAnnotation{}, fmt.Errorf("read epub annotation id: %w", err)
	}
	return s.getEPUBAnnotation(ctx, userID, id)
}

func (s *Service) UpdateEPUBAnnotation(ctx context.Context, userID, annotationID int64, input UpdateEPUBAnnotationInput) (EPUBAnnotation, error) {
	sets := make([]string, 0, 3)
	args := make([]any, 0, 4)
	if input.Color != nil {
		color := strings.TrimSpace(*input.Color)
		if !validAnnotationColor(color) {
			return EPUBAnnotation{}, ErrInvalidAnnotation
		}
		sets = append(sets, "color = ?")
		args = append(args, color)
	}
	if input.Note != nil {
		note := strings.TrimSpace(*input.Note)
		if len([]rune(note)) > 20_000 {
			return EPUBAnnotation{}, ErrInvalidAnnotation
		}
		sets = append(sets, "note = ?")
		args = append(args, note)
	}
	if len(sets) == 0 {
		return s.getEPUBAnnotation(ctx, userID, annotationID)
	}
	sets = append(sets, "updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')")
	args = append(args, annotationID, userID)
	result, err := s.db.ExecContext(ctx, `UPDATE epub_annotations SET `+strings.Join(sets, ", ")+` WHERE id = ? AND user_id = ?`, args...)
	if err != nil {
		return EPUBAnnotation{}, fmt.Errorf("update epub annotation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return EPUBAnnotation{}, fmt.Errorf("read updated epub annotation rows: %w", err)
	}
	if rows == 0 {
		return EPUBAnnotation{}, ErrEPUBAnnotationNotFound
	}
	return s.getEPUBAnnotation(ctx, userID, annotationID)
}

func (s *Service) DeleteEPUBAnnotation(ctx context.Context, userID, annotationID int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM epub_annotations WHERE id = ? AND user_id = ?`, annotationID, userID)
	if err != nil {
		return fmt.Errorf("delete epub annotation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted epub annotation rows: %w", err)
	}
	if rows == 0 {
		return ErrEPUBAnnotationNotFound
	}
	return nil
}

func (s *Service) getEPUBAnnotation(ctx context.Context, userID, annotationID int64) (EPUBAnnotation, error) {
	row := s.db.QueryRowContext(ctx, `
        SELECT id, book_id, spine_index, resource_path, kind, color, selected_text, note,
               anchor_json, created_at, updated_at
        FROM epub_annotations WHERE id = ? AND user_id = ?
    `, annotationID, userID)
	annotation, err := scanEPUBAnnotation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return EPUBAnnotation{}, ErrEPUBAnnotationNotFound
	}
	return annotation, err
}

type epubAnnotationScanner interface {
	Scan(dest ...any) error
}

func scanEPUBAnnotation(scanner epubAnnotationScanner) (EPUBAnnotation, error) {
	var annotation EPUBAnnotation
	var anchorJSON, createdAt, updatedAt string
	if err := scanner.Scan(
		&annotation.ID, &annotation.BookID, &annotation.SpineIndex, &annotation.ResourcePath,
		&annotation.Kind, &annotation.Color, &annotation.SelectedText, &annotation.Note,
		&anchorJSON, &createdAt, &updatedAt,
	); err != nil {
		return EPUBAnnotation{}, err
	}
	if err := json.Unmarshal([]byte(anchorJSON), &annotation.Anchor); err != nil {
		return EPUBAnnotation{}, fmt.Errorf("decode epub annotation anchor: %w", err)
	}
	annotation.CreatedAt = parseSQLiteTime(createdAt)
	annotation.UpdatedAt = parseSQLiteTime(updatedAt)
	return annotation, nil
}

func validateEPUBAnnotationInput(input CreateEPUBAnnotationInput) error {
	if input.SpineIndex < 0 || len(input.ResourcePath) == 0 || len([]rune(input.ResourcePath)) > 4_000 {
		return ErrInvalidAnnotation
	}
	if input.Kind != "highlight" && input.Kind != "note" {
		return ErrInvalidAnnotation
	}
	if !validAnnotationColor(input.Color) {
		return ErrInvalidAnnotation
	}
	if len([]rune(input.SelectedText)) > 20_000 || len([]rune(input.Note)) > 20_000 {
		return ErrInvalidAnnotation
	}
	if input.Kind == "highlight" && strings.TrimSpace(input.SelectedText) == "" {
		return ErrInvalidAnnotation
	}
	if input.Kind == "note" && strings.TrimSpace(input.Note) == "" {
		return ErrInvalidAnnotation
	}
	anchor := input.Anchor
	if anchor.StartPath == "" || anchor.EndPath == "" || len(anchor.StartPath) > 8_000 || len(anchor.EndPath) > 8_000 {
		return ErrInvalidAnnotation
	}
	if anchor.StartOffset < 0 || anchor.EndOffset < 0 || anchor.StartOffset > 10_000_000 || anchor.EndOffset > 10_000_000 {
		return ErrInvalidAnnotation
	}
	if len([]rune(anchor.Prefix)) > 500 || len([]rune(anchor.Suffix)) > 500 {
		return ErrInvalidAnnotation
	}
	return nil
}
