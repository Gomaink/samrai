package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	sessionKeyPattern             = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)
	ErrReadingSessionBookMismatch = errors.New("reading session belongs to another book")
)

type Favorites struct {
	Books  []Book   `json:"books"`
	Series []Series `json:"series"`
}

type ReadingSession struct {
	ID              int64           `json:"id"`
	BookID          int64           `json:"book_id"`
	BookTitle       string          `json:"book_title"`
	BookFormat      string          `json:"book_format"`
	CoverURL        string          `json:"cover_url"`
	StartedAt       time.Time       `json:"started_at"`
	LastActivityAt  time.Time       `json:"last_activity_at"`
	EndedAt         *time.Time      `json:"ended_at,omitempty"`
	StartPage       int             `json:"start_page"`
	EndPage         int             `json:"end_page"`
	StartLocation   json.RawMessage `json:"start_location,omitempty"`
	EndLocation     json.RawMessage `json:"end_location,omitempty"`
	DurationSeconds int             `json:"duration_seconds"`
}

type AnnotationSummary struct {
	Source       string    `json:"source"`
	ID           int64     `json:"id"`
	BookID       int64     `json:"book_id"`
	BookTitle    string    `json:"book_title"`
	BookFormat   string    `json:"book_format"`
	CoverURL     string    `json:"cover_url"`
	Position     int       `json:"position"`
	Location     string    `json:"location"`
	Color        string    `json:"color"`
	SelectedText string    `json:"selected_text"`
	Note         string    `json:"note"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type AnnotationListOptions struct {
	Search string
	Color  string
	Source string
	Limit  int
	Offset int
}

func (s *Service) SetBookFavorite(ctx context.Context, userID, bookID int64, favorite bool) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM books WHERE id = ? AND status = 'ready')`, bookID).Scan(&exists); err != nil {
		return fmt.Errorf("check book favorite target: %w", err)
	}
	if exists == 0 {
		return ErrBookNotFound
	}
	if favorite {
		_, err := s.db.ExecContext(ctx, `INSERT INTO book_favorites(user_id, book_id) VALUES (?, ?) ON CONFLICT(user_id, book_id) DO NOTHING`, userID, bookID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM book_favorites WHERE user_id = ? AND book_id = ?`, userID, bookID)
	return err
}

func (s *Service) SetSeriesFavorite(ctx context.Context, userID, seriesID int64, favorite bool) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM series WHERE id = ?)`, seriesID).Scan(&exists); err != nil {
		return fmt.Errorf("check series favorite target: %w", err)
	}
	if exists == 0 {
		return ErrSeriesNotFound
	}
	if favorite {
		_, err := s.db.ExecContext(ctx, `INSERT INTO series_favorites(user_id, series_id) VALUES (?, ?) ON CONFLICT(user_id, series_id) DO NOTHING`, userID, seriesID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM series_favorites WHERE user_id = ? AND series_id = ?`, userID, seriesID)
	return err
}

func (s *Service) Favorites(ctx context.Context, userID int64) (Favorites, error) {
	books, err := s.QueryBooks(ctx, userID, BookListOptions{FavoriteOnly: true, Sort: "title", Limit: 500})
	if err != nil {
		return Favorites{}, err
	}
	series, err := s.ListSeries(ctx, userID, SeriesListOptions{FavoriteOnly: true, Sort: "title", Limit: 500})
	if err != nil {
		return Favorites{}, err
	}
	return Favorites{Books: books.Items, Series: series.Items}, nil
}

func (s *Service) markBookFavorites(ctx context.Context, userID int64, books []Book) error {
	if len(books) == 0 {
		return nil
	}
	ids := make([]any, 0, len(books)+1)
	ids = append(ids, userID)
	placeholders := make([]string, 0, len(books))
	index := make(map[int64]int, len(books))
	for i := range books {
		placeholders = append(placeholders, "?")
		ids = append(ids, books[i].ID)
		index[books[i].ID] = i
	}
	rows, err := s.db.QueryContext(ctx, `SELECT book_id FROM book_favorites WHERE user_id = ? AND book_id IN (`+strings.Join(placeholders, ",")+`)`, ids...)
	if err != nil {
		return fmt.Errorf("load book favorites: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		if i, ok := index[id]; ok {
			books[i].Favorite = true
		}
	}
	return rows.Err()
}

func (s *Service) markSeriesFavorites(ctx context.Context, userID int64, series []Series) error {
	if len(series) == 0 {
		return nil
	}
	args := make([]any, 0, len(series)+1)
	args = append(args, userID)
	placeholders := make([]string, 0, len(series))
	index := make(map[int64]int, len(series))
	for i := range series {
		placeholders = append(placeholders, "?")
		args = append(args, series[i].ID)
		index[series[i].ID] = i
	}
	rows, err := s.db.QueryContext(ctx, `SELECT series_id FROM series_favorites WHERE user_id = ? AND series_id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return fmt.Errorf("load series favorites: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		if i, ok := index[id]; ok {
			series[i].Favorite = true
		}
	}
	return rows.Err()
}

func normalizeSessionLocation(location json.RawMessage) string {
	if len(location) == 0 || !json.Valid(location) || len(location) > 4096 {
		return "{}"
	}
	return string(location)
}

func (s *Service) RecordReadingActivity(ctx context.Context, userID, bookID int64, sessionKey string, page int, location json.RawMessage) error {
	sessionKey = strings.TrimSpace(sessionKey)
	if !sessionKeyPattern.MatchString(sessionKey) {
		return nil
	}
	now := time.Now().UTC()
	locationJSON := normalizeSessionLocation(location)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var id, existingBookID int64
	var lastRaw string
	err = tx.QueryRowContext(ctx, `SELECT id, book_id, last_activity_at FROM reading_sessions WHERE user_id = ? AND session_key = ?`, userID, sessionKey).Scan(&id, &existingBookID, &lastRaw)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `
            INSERT INTO reading_sessions(user_id, book_id, session_key, started_at, last_activity_at, start_page, end_page, start_location_json, end_location_json)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
        `, userID, bookID, sessionKey, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), page, page, locationJSON, locationJSON)
	} else if err == nil {
		if existingBookID != bookID {
			return ErrReadingSessionBookMismatch
		}
		last := parseSQLiteTime(lastRaw)
		delta := int(now.Sub(last).Seconds())
		if delta < 0 {
			delta = 0
		}
		if delta > 300 {
			delta = 300
		}
		_, err = tx.ExecContext(ctx, `
            UPDATE reading_sessions SET
                end_page = ?, end_location_json = ?, last_activity_at = ?, ended_at = NULL,
                duration_seconds = duration_seconds + ?
            WHERE id = ?
        `, page, locationJSON, now.Format(time.RFC3339Nano), delta, id)
	}
	if err != nil {
		return fmt.Errorf("record reading activity: %w", err)
	}
	return tx.Commit()
}

func (s *Service) EndReadingSession(ctx context.Context, userID, bookID int64, sessionKey string) error {
	sessionKey = strings.TrimSpace(sessionKey)
	if !sessionKeyPattern.MatchString(sessionKey) {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if bookID > 0 {
		_, err := s.db.ExecContext(ctx, `UPDATE reading_sessions SET ended_at = COALESCE(ended_at, ?), last_activity_at = ? WHERE user_id = ? AND book_id = ? AND session_key = ?`, now, now, userID, bookID, sessionKey)
		return err
	}
	// Backward compatibility for clients released before book_id was sent.
	_, err := s.db.ExecContext(ctx, `UPDATE reading_sessions SET ended_at = COALESCE(ended_at, ?), last_activity_at = ? WHERE user_id = ? AND session_key = ?`, now, now, userID, sessionKey)
	return err
}

func (s *Service) ListReadingHistory(ctx context.Context, userID int64, limit, offset int) ([]ReadingSession, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx, `
        SELECT rs.id, rs.book_id, b.title, b.format, rs.started_at, rs.last_activity_at, rs.ended_at,
               rs.start_page, rs.end_page, rs.start_location_json, rs.end_location_json, rs.duration_seconds
        FROM reading_sessions rs
        JOIN books b ON b.id = rs.book_id
        WHERE rs.user_id = ?
        ORDER BY rs.last_activity_at DESC, rs.id DESC
        LIMIT ? OFFSET ?
    `, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list reading history: %w", err)
	}
	defer rows.Close()
	items := make([]ReadingSession, 0)
	for rows.Next() {
		var item ReadingSession
		var started, last, ended sql.NullString
		var startLocation, endLocation string
		if err := rows.Scan(&item.ID, &item.BookID, &item.BookTitle, &item.BookFormat, &started, &last, &ended, &item.StartPage, &item.EndPage, &startLocation, &endLocation, &item.DurationSeconds); err != nil {
			return nil, err
		}
		item.StartedAt = parseSQLiteTime(started.String)
		item.LastActivityAt = parseSQLiteTime(last.String)
		if ended.Valid {
			value := parseSQLiteTime(ended.String)
			item.EndedAt = &value
		}
		item.StartLocation = json.RawMessage(startLocation)
		item.EndLocation = json.RawMessage(endLocation)
		item.CoverURL = fmt.Sprintf("/api/v1/books/%d/cover?width=240&format=webp", item.BookID)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) ClearReadingHistory(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM reading_sessions WHERE user_id = ?`, userID)
	return err
}

func (s *Service) ListAllAnnotations(ctx context.Context, userID int64, options AnnotationListOptions) ([]AnnotationSummary, int, error) {
	options.Search = strings.TrimSpace(options.Search)
	if options.Limit < 1 || options.Limit > 500 {
		options.Limit = 100
	}
	if options.Offset < 0 {
		options.Offset = 0
	}
	switch options.Color {
	case "yellow", "green", "blue", "pink", "orange":
	default:
		options.Color = ""
	}
	if options.Source != "pdf" && options.Source != "epub" {
		options.Source = ""
	}

	base := `
        SELECT 'pdf' AS source, a.id, a.book_id, b.title AS book_title, b.format AS book_format, a.page_number AS position,
               'Page ' || (a.page_number + 1) AS location, a.color, a.selected_text, a.note, a.created_at, a.updated_at
        FROM annotations a JOIN books b ON b.id = a.book_id WHERE a.user_id = ?
        UNION ALL
        SELECT 'epub' AS source, e.id, e.book_id, b.title AS book_title, b.format AS book_format, e.spine_index AS position,
               'Section ' || (e.spine_index + 1) AS location, e.color, e.selected_text, e.note, e.created_at, e.updated_at
        FROM epub_annotations e JOIN books b ON b.id = e.book_id WHERE e.user_id = ?
    `
	where := []string{"1=1"}
	args := []any{userID, userID}
	if options.Search != "" {
		like := "%" + escapeLike(options.Search) + "%"
		where = append(where, `(book_title LIKE ? ESCAPE '\' COLLATE NOCASE OR selected_text LIKE ? ESCAPE '\' COLLATE NOCASE OR note LIKE ? ESCAPE '\' COLLATE NOCASE)`)
		args = append(args, like, like, like)
	}
	if options.Color != "" {
		where = append(where, "color = ?")
		args = append(args, options.Color)
	}
	if options.Source != "" {
		where = append(where, "source = ?")
		args = append(args, options.Source)
	}
	var total int
	countQuery := `SELECT COUNT(*) FROM (` + base + `) all_annotations WHERE ` + strings.Join(where, " AND ")
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count annotations: %w", err)
	}
	queryArgs := append(append([]any(nil), args...), options.Limit, options.Offset)
	rows, err := s.db.QueryContext(ctx, `SELECT source, id, book_id, book_title, book_format, position, location, color, selected_text, note, created_at, updated_at FROM (`+base+`) all_annotations WHERE `+strings.Join(where, " AND ")+` ORDER BY updated_at DESC, source ASC, id DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list annotations: %w", err)
	}
	defer rows.Close()
	items := make([]AnnotationSummary, 0)
	for rows.Next() {
		var item AnnotationSummary
		var created, updated string
		if err := rows.Scan(&item.Source, &item.ID, &item.BookID, &item.BookTitle, &item.BookFormat, &item.Position, &item.Location, &item.Color, &item.SelectedText, &item.Note, &created, &updated); err != nil {
			return nil, 0, err
		}
		item.CreatedAt = parseSQLiteTime(created)
		item.UpdatedAt = parseSQLiteTime(updated)
		item.CoverURL = fmt.Sprintf("/api/v1/books/%d/cover?width=240&format=webp", item.BookID)
		items = append(items, item)
	}
	return items, total, rows.Err()
}
