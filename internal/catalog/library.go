package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type BookListOptions struct {
	Search       string
	Status       string
	Sort         string
	SeriesID     int64
	FavoriteOnly bool
	Limit        int
	Offset       int
}

type BookListResult struct {
	Items []Book
	Total int
}

type LibraryStats struct {
	TotalBooks     int `json:"total_books"`
	UnreadBooks    int `json:"unread_books"`
	ReadingBooks   int `json:"reading_books"`
	CompletedBooks int `json:"completed_books"`
	TotalSeries    int `json:"total_series"`
}

type Series struct {
	ID               int64     `json:"id"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	ReadingDirection *string   `json:"reading_direction,omitempty"`
	BookCount        int       `json:"book_count"`
	StartedCount     int       `json:"started_count"`
	CompletedCount   int       `json:"completed_count"`
	PageCount        int       `json:"page_count"`
	CoverBookID      int64     `json:"cover_book_id"`
	CoverURL         string    `json:"cover_url"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Favorite         bool      `json:"favorite"`
}

type SeriesListOptions struct {
	Search       string
	Sort         string
	FavoriteOnly bool
	Limit        int
	Offset       int
}

type SeriesListResult struct {
	Items []Series
	Total int
}

type SeriesDetail struct {
	Series Series `json:"series"`
	Books  []Book `json:"books"`
}

type Dashboard struct {
	ContinueReading []Book       `json:"continue_reading"`
	RecentlyAdded   []Book       `json:"recently_added"`
	Series          []Series     `json:"series"`
	Stats           LibraryStats `json:"stats"`
}

func normalizeBookListOptions(options BookListOptions) BookListOptions {
	options.Search = strings.TrimSpace(options.Search)
	if options.Limit < 1 || options.Limit > 500 {
		options.Limit = 100
	}
	if options.Offset < 0 {
		options.Offset = 0
	}
	switch options.Status {
	case "unread", "reading", "completed":
	default:
		options.Status = "all"
	}
	switch options.Sort {
	case "title", "series", "progress", "year", "oldest":
	default:
		options.Sort = "recent"
	}
	return options
}

func (s *Service) QueryBooks(ctx context.Context, userID int64, options BookListOptions) (BookListResult, error) {
	options = normalizeBookListOptions(options)
	where, args := bookFilters(userID, options)

	var total int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM books b
		LEFT JOIN series s ON s.id = b.series_id
		LEFT JOIN reading_progress rp ON rp.book_id = b.id AND rp.user_id = ?
		WHERE `+where,
		args...,
	).Scan(&total); err != nil {
		return BookListResult{}, fmt.Errorf("count books: %w", err)
	}

	queryArgs := append(append([]any(nil), args...), options.Limit, options.Offset)
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+bookColumns+`
		FROM books b
		LEFT JOIN series s ON s.id = b.series_id
		LEFT JOIN reading_progress rp ON rp.book_id = b.id AND rp.user_id = ?
		WHERE `+where+`
		ORDER BY `+bookOrder(options.Sort)+`
		LIMIT ? OFFSET ?
	`, queryArgs...)
	if err != nil {
		return BookListResult{}, fmt.Errorf("query books: %w", err)
	}
	defer rows.Close()

	items := make([]Book, 0)
	for rows.Next() {
		book, err := scanBook(rows)
		if err != nil {
			return BookListResult{}, fmt.Errorf("scan queried book: %w", err)
		}
		items = append(items, book)
	}
	if err := rows.Err(); err != nil {
		return BookListResult{}, fmt.Errorf("iterate queried books: %w", err)
	}
	if err := s.markBookFavorites(ctx, userID, items); err != nil {
		return BookListResult{}, err
	}
	return BookListResult{Items: items, Total: total}, nil
}

func bookFilters(userID int64, options BookListOptions) (string, []any) {
	conditions := []string{"b.status = 'ready'"}
	args := []any{userID}
	if options.Search != "" {
		like := "%" + escapeLike(options.Search) + "%"
		conditions = append(conditions, `(
			b.title LIKE ? ESCAPE '\' COLLATE NOCASE OR
			COALESCE(s.title, '') LIKE ? ESCAPE '\' COLLATE NOCASE OR
			b.writer LIKE ? ESCAPE '\' COLLATE NOCASE OR
			b.publisher LIKE ? ESCAPE '\' COLLATE NOCASE OR
			b.original_filename LIKE ? ESCAPE '\' COLLATE NOCASE
		)`)
		for range 5 {
			args = append(args, like)
		}
	}
	if options.SeriesID > 0 {
		conditions = append(conditions, "b.series_id = ?")
		args = append(args, options.SeriesID)
	}
	if options.FavoriteOnly {
		conditions = append(conditions, "EXISTS(SELECT 1 FROM book_favorites bf WHERE bf.user_id = ? AND bf.book_id = b.id)")
		args = append(args, userID)
	}
	switch options.Status {
	case "unread":
		conditions = append(conditions, "rp.user_id IS NULL")
	case "reading":
		conditions = append(conditions, "rp.user_id IS NOT NULL AND rp.completed = 0")
	case "completed":
		conditions = append(conditions, "COALESCE(rp.completed, 0) = 1")
	}
	return strings.Join(conditions, " AND "), args
}

func bookOrder(sort string) string {
	switch sort {
	case "title":
		return "b.title COLLATE NOCASE ASC, b.id ASC"
	case "series":
		return `COALESCE(s.title, b.title) COLLATE NOCASE ASC,
			CASE WHEN trim(b.volume) <> '' AND trim(b.volume) NOT GLOB '*[^0-9.]*' THEN CAST(b.volume AS REAL) ELSE 999999 END ASC,
			CASE WHEN trim(b.number) <> '' AND trim(b.number) NOT GLOB '*[^0-9.]*' THEN CAST(b.number AS REAL) ELSE 999999 END ASC,
			b.title COLLATE NOCASE ASC`
	case "progress":
		return "CASE WHEN rp.user_id IS NULL THEN 1 ELSE 0 END, rp.updated_at DESC, b.title COLLATE NOCASE ASC"
	case "year":
		return "CASE WHEN b.publication_year IS NULL THEN 1 ELSE 0 END, b.publication_year DESC, b.title COLLATE NOCASE ASC"
	case "oldest":
		return "b.created_at ASC, b.id ASC"
	default:
		return "b.created_at DESC, b.id DESC"
	}
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}

func (s *Service) Stats(ctx context.Context, userID int64) (LibraryStats, error) {
	var stats LibraryStats
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(b.id),
			COALESCE(SUM(CASE WHEN rp.user_id IS NULL THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN rp.user_id IS NOT NULL AND rp.completed = 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN COALESCE(rp.completed, 0) = 1 THEN 1 ELSE 0 END), 0),
			COUNT(DISTINCT b.series_id)
		FROM books b
		LEFT JOIN reading_progress rp ON rp.book_id = b.id AND rp.user_id = ?
		WHERE b.status = 'ready'
	`, userID).Scan(&stats.TotalBooks, &stats.UnreadBooks, &stats.ReadingBooks, &stats.CompletedBooks, &stats.TotalSeries)
	if err != nil {
		return LibraryStats{}, fmt.Errorf("read library stats: %w", err)
	}
	return stats, nil
}

func normalizeSeriesListOptions(options SeriesListOptions) SeriesListOptions {
	options.Search = strings.TrimSpace(options.Search)
	if options.Limit < 1 || options.Limit > 500 {
		options.Limit = 100
	}
	if options.Offset < 0 {
		options.Offset = 0
	}
	switch options.Sort {
	case "recent", "count":
	default:
		options.Sort = "title"
	}
	return options
}

func (s *Service) ListSeries(ctx context.Context, userID int64, options SeriesListOptions) (SeriesListResult, error) {
	options = normalizeSeriesListOptions(options)
	where := "b.status = 'ready'"
	args := []any{userID}
	if options.FavoriteOnly {
		where += " AND EXISTS(SELECT 1 FROM series_favorites sf WHERE sf.user_id = ? AND sf.series_id = s.id)"
		args = append(args, userID)
	}
	if options.Search != "" {
		where += ` AND (s.title LIKE ? ESCAPE '\' COLLATE NOCASE OR s.description LIKE ? ESCAPE '\' COLLATE NOCASE)`
		like := "%" + escapeLike(options.Search) + "%"
		args = append(args, like, like)
	}

	var total int
	countArgs := make([]any, 0)
	countWhere := "EXISTS(SELECT 1 FROM books b WHERE b.series_id = s.id AND b.status = 'ready')"
	if options.FavoriteOnly {
		countWhere += " AND EXISTS(SELECT 1 FROM series_favorites sf WHERE sf.user_id = ? AND sf.series_id = s.id)"
		countArgs = append(countArgs, userID)
	}
	if options.Search != "" {
		countWhere += ` AND (s.title LIKE ? ESCAPE '\' COLLATE NOCASE OR s.description LIKE ? ESCAPE '\' COLLATE NOCASE)`
		like := "%" + escapeLike(options.Search) + "%"
		countArgs = append(countArgs, like, like)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM series s WHERE "+countWhere, countArgs...).Scan(&total); err != nil {
		return SeriesListResult{}, fmt.Errorf("count series: %w", err)
	}

	order := "s.title COLLATE NOCASE ASC"
	switch options.Sort {
	case "recent":
		order = "MAX(b.created_at) DESC, s.title COLLATE NOCASE ASC"
	case "count":
		order = "COUNT(b.id) DESC, s.title COLLATE NOCASE ASC"
	}
	queryArgs := append(append([]any(nil), args...), options.Limit, options.Offset)
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			s.id, s.title, s.description, s.reading_direction,
			COUNT(b.id),
			SUM(CASE WHEN rp.user_id IS NOT NULL THEN 1 ELSE 0 END),
			SUM(CASE WHEN COALESCE(rp.completed, 0) = 1 THEN 1 ELSE 0 END),
			COALESCE(SUM(b.page_count), 0),
			COALESCE((
				SELECT selected.id FROM books selected
				WHERE selected.series_id = s.id AND selected.status = 'ready'
				ORDER BY selected.created_at DESC, selected.id DESC LIMIT 1
			), 0),
			s.created_at, MAX(b.updated_at)
		FROM series s
		JOIN books b ON b.series_id = s.id
		LEFT JOIN reading_progress rp ON rp.book_id = b.id AND rp.user_id = ?
		WHERE `+where+`
		GROUP BY s.id
		ORDER BY `+order+`
		LIMIT ? OFFSET ?
	`, queryArgs...)
	if err != nil {
		return SeriesListResult{}, fmt.Errorf("list series: %w", err)
	}
	defer rows.Close()

	items := make([]Series, 0)
	for rows.Next() {
		series, err := scanSeries(rows)
		if err != nil {
			return SeriesListResult{}, fmt.Errorf("scan series: %w", err)
		}
		items = append(items, series)
	}
	if err := rows.Err(); err != nil {
		return SeriesListResult{}, fmt.Errorf("iterate series: %w", err)
	}
	if err := s.markSeriesFavorites(ctx, userID, items); err != nil {
		return SeriesListResult{}, err
	}
	return SeriesListResult{Items: items, Total: total}, nil
}

func (s *Service) GetSeries(ctx context.Context, userID, seriesID int64) (SeriesDetail, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			s.id, s.title, s.description, s.reading_direction,
			COUNT(b.id),
			SUM(CASE WHEN rp.user_id IS NOT NULL THEN 1 ELSE 0 END),
			SUM(CASE WHEN COALESCE(rp.completed, 0) = 1 THEN 1 ELSE 0 END),
			COALESCE(SUM(b.page_count), 0),
			COALESCE((
				SELECT selected.id FROM books selected
				WHERE selected.series_id = s.id AND selected.status = 'ready'
				ORDER BY selected.created_at DESC, selected.id DESC LIMIT 1
			), 0),
			s.created_at, MAX(b.updated_at)
		FROM series s
		JOIN books b ON b.series_id = s.id AND b.status = 'ready'
		LEFT JOIN reading_progress rp ON rp.book_id = b.id AND rp.user_id = ?
		WHERE s.id = ?
		GROUP BY s.id
	`, userID, seriesID)
	series, err := scanSeries(row)
	if errors.Is(err, sql.ErrNoRows) {
		return SeriesDetail{}, ErrSeriesNotFound
	}
	if err != nil {
		return SeriesDetail{}, fmt.Errorf("get series: %w", err)
	}
	seriesItems := []Series{series}
	if err := s.markSeriesFavorites(ctx, userID, seriesItems); err != nil {
		return SeriesDetail{}, err
	}
	series = seriesItems[0]
	books, err := s.QueryBooks(ctx, userID, BookListOptions{SeriesID: seriesID, Sort: "series", Limit: 500})
	if err != nil {
		return SeriesDetail{}, err
	}
	return SeriesDetail{Series: series, Books: books.Items}, nil
}

func scanSeries(scanner bookScanner) (Series, error) {
	var series Series
	var direction sql.NullString
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&series.ID, &series.Title, &series.Description, &direction,
		&series.BookCount, &series.StartedCount, &series.CompletedCount,
		&series.PageCount, &series.CoverBookID, &createdAt, &updatedAt,
	); err != nil {
		return Series{}, err
	}
	if direction.Valid {
		series.ReadingDirection = &direction.String
	}
	series.CreatedAt = parseSQLiteTime(createdAt)
	series.UpdatedAt = parseSQLiteTime(updatedAt)
	if series.CoverBookID > 0 {
		series.CoverURL = fmt.Sprintf("/api/v1/books/%d/cover?width=640&format=webp", series.CoverBookID)
	}
	return series, nil
}

func (s *Service) Dashboard(ctx context.Context, userID int64) (Dashboard, error) {
	continueBooks, err := s.queryBooksWithExtra(ctx, userID, "rp.user_id IS NOT NULL AND rp.completed = 0", "rp.updated_at DESC", 12)
	if err != nil {
		return Dashboard{}, err
	}
	recentBooks, err := s.QueryBooks(ctx, userID, BookListOptions{Sort: "recent", Limit: 12})
	if err != nil {
		return Dashboard{}, err
	}
	series, err := s.ListSeries(ctx, userID, SeriesListOptions{Sort: "recent", Limit: 8})
	if err != nil {
		return Dashboard{}, err
	}
	stats, err := s.Stats(ctx, userID)
	if err != nil {
		return Dashboard{}, err
	}
	return Dashboard{
		ContinueReading: continueBooks,
		RecentlyAdded:   recentBooks.Items,
		Series:          series.Items,
		Stats:           stats,
	}, nil
}

func (s *Service) queryBooksWithExtra(ctx context.Context, userID int64, extraWhere, order string, limit int) ([]Book, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+bookColumns+`
		FROM books b
		LEFT JOIN series s ON s.id = b.series_id
		LEFT JOIN reading_progress rp ON rp.book_id = b.id AND rp.user_id = ?
		WHERE b.status = 'ready' AND `+extraWhere+`
		ORDER BY `+order+`
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("query dashboard books: %w", err)
	}
	defer rows.Close()
	items := make([]Book, 0)
	for rows.Next() {
		book, err := scanBook(rows)
		if err != nil {
			return nil, fmt.Errorf("scan dashboard book: %w", err)
		}
		items = append(items, book)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dashboard books: %w", err)
	}
	if err := s.markBookFavorites(ctx, userID, items); err != nil {
		return nil, err
	}
	return items, nil
}
