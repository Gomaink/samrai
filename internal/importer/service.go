package importer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

const importJobType = "import_cbz"

type Service struct {
	db          *sql.DB
	logger      *slog.Logger
	dataDir     string
	uploadDir   string
	libraryDir  string
	preparedDir string
	limits      Limits
	tools       externalArchiveTools
	wake        chan struct{}
	wait        sync.WaitGroup
	queueMu     sync.Mutex
}

func NewService(db *sql.DB, dataDir string, limits Limits, logger *slog.Logger) (*Service, error) {
	if db == nil || logger == nil {
		return nil, errors.New("database and logger are required")
	}
	if limits.MaxUploadBytes < 1 || limits.MaxArchiveBytes < 1 || limits.MaxPageBytes < 1 || limits.MaxArchiveEntries < 1 || limits.MaxPages < 1 {
		return nil, errors.New("all import limits must be positive")
	}

	service := &Service{
		db:          db,
		logger:      logger,
		dataDir:     dataDir,
		uploadDir:   filepath.Join(dataDir, "uploads"),
		libraryDir:  filepath.Join(dataDir, "library"),
		preparedDir: filepath.Join(dataDir, "prepared"),
		limits:      limits,
		tools:       discoverArchiveTools(limits.SevenZipPath, limits.LSARPath, limits.UNARPath),
		wake:        make(chan struct{}, 1),
	}
	if service.tools.SevenZip == "" && (service.tools.LSAR == "" || service.tools.UNAR == "") {
		logger.Warn("CBR and CB7 import tools unavailable", "hint", "install 7-Zip or configure lsar and unar")
	} else {
		logger.Info("comic archive tools detected", "seven_zip", service.tools.SevenZip != "", "unar", service.tools.LSAR != "" && service.tools.UNAR != "")
	}
	return service, nil
}

func (s *Service) Start(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'pending', progress = 0, started_at = NULL,
			error_message = 'Processing was interrupted; the job will resume.'
		WHERE job_type = ? AND status = 'running'
	`, importJobType); err != nil {
		return fmt.Errorf("recover import jobs: %w", err)
	}

	s.wait.Add(1)
	go s.run(ctx)
	s.signal()
	return nil
}

func (s *Service) Wait() {
	s.wait.Wait()
}

func (s *Service) MaxUploadBytes() int64 {
	return s.limits.MaxUploadBytes
}

func (s *Service) QueueUpload(ctx context.Context, originalFilename string, source io.Reader) (Job, error) {
	return s.QueueUploadWithMetadata(ctx, originalFilename, source, UploadMetadata{})
}

func (s *Service) QueueUploadWithMetadata(ctx context.Context, originalFilename string, source io.Reader, metadata UploadMetadata) (Job, error) {
	originalFilename = normalizeOriginalFilename(originalFilename)
	metadata = normalizeUploadMetadata(metadata)
	extension := strings.ToLower(filepath.Ext(originalFilename))
	switch extension {
	case ".cbz", ".zip", ".cbr", ".rar", ".cb7", ".7z", ".cbt", ".tar", ".pdf", ".epub":
	default:
		return Job{}, ErrInvalidExtension
	}

	randomName, err := randomHex(16)
	if err != nil {
		return Job{}, fmt.Errorf("generate upload name: %w", err)
	}
	uploadPath := filepath.Join(s.uploadDir, randomName+".upload")
	file, err := os.OpenFile(uploadPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return Job{}, fmt.Errorf("create upload: %w", err)
	}

	hash := sha256.New()
	limited := &io.LimitedReader{R: source, N: s.limits.MaxUploadBytes + 1}
	written, copyErr := io.Copy(io.MultiWriter(file, hash), limited)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(uploadPath)
		return Job{}, fmt.Errorf("save upload: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(uploadPath)
		return Job{}, fmt.Errorf("close upload: %w", closeErr)
	}
	if written > s.limits.MaxUploadBytes {
		_ = os.Remove(uploadPath)
		return Job{}, ErrUploadTooLarge
	}
	if written == 0 {
		_ = os.Remove(uploadPath)
		return Job{}, fmt.Errorf("%w: empty file", ErrInvalidArchive)
	}

	payload := uploadPayload{
		UploadPath:       uploadPath,
		OriginalFilename: originalFilename,
		FileSize:         written,
		FileHash:         hex.EncodeToString(hash.Sum(nil)),
		Metadata:         metadata,
	}

	// Queueing is serialized so two simultaneous browser uploads of the same
	// file cannot both pass the duplicate check before either job is stored.
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	duplicate, err := s.duplicateHashExists(ctx, payload.FileHash)
	if err != nil {
		_ = os.Remove(uploadPath)
		return Job{}, fmt.Errorf("check duplicate upload: %w", err)
	}
	if duplicate {
		_ = os.Remove(uploadPath)
		return Job{}, ErrDuplicateBook
	}

	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		_ = os.Remove(uploadPath)
		return Job{}, fmt.Errorf("encode job payload: %w", err)
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO jobs(job_type, status, progress, payload)
		VALUES (?, 'pending', 0, ?)
	`, importJobType, string(encodedPayload))
	if err != nil {
		_ = os.Remove(uploadPath)
		return Job{}, fmt.Errorf("create import job: %w", err)
	}
	jobID, err := result.LastInsertId()
	if err != nil {
		_ = os.Remove(uploadPath)
		return Job{}, fmt.Errorf("read import job id: %w", err)
	}

	s.signal()
	return Job{
		ID:               jobID,
		Type:             importJobType,
		Status:           "pending",
		Progress:         0,
		OriginalFilename: originalFilename,
		CreatedAt:        time.Now().UTC(),
	}, nil
}

func (s *Service) duplicateHashExists(ctx context.Context, hash string) (bool, error) {
	var bookID int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM books WHERE file_hash = ? LIMIT 1`, hash).Scan(&bookID)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT payload
		FROM jobs
		WHERE job_type = ? AND status IN ('pending', 'running')
	`, importJobType)
	if err != nil {
		return false, err
	}
	activeDuplicate := false
	for rows.Next() {
		var payloadJSON string
		if err := rows.Scan(&payloadJSON); err != nil {
			_ = rows.Close()
			return false, err
		}
		var payload uploadPayload
		if json.Unmarshal([]byte(payloadJSON), &payload) == nil && payload.FileHash == hash {
			activeDuplicate = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, err
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	if activeDuplicate {
		return true, nil
	}

	// A worker may have committed the book between the first books query and
	// the active-jobs query. Recheck after closing the rows to cover that edge.
	err = s.db.QueryRowContext(ctx, `SELECT id FROM books WHERE file_hash = ? LIMIT 1`, hash).Scan(&bookID)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, err
}

func (s *Service) ListJobs(ctx context.Context, limit int) ([]Job, error) {
	if limit < 1 || limit > 500 {
		limit = 30
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, job_type, status, progress, payload, COALESCE(error_message, ''),
			created_at, started_at, completed_at
		FROM jobs
		WHERE job_type = ?
		ORDER BY id DESC
		LIMIT ?
	`, importJobType, limit)
	if err != nil {
		return nil, fmt.Errorf("list import jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]Job, 0)
	for rows.Next() {
		var job Job
		var payloadJSON, createdAt string
		var startedAt, completedAt sql.NullString
		if err := rows.Scan(&job.ID, &job.Type, &job.Status, &job.Progress, &payloadJSON, &job.ErrorMessage, &createdAt, &startedAt, &completedAt); err != nil {
			return nil, fmt.Errorf("scan import job: %w", err)
		}
		var payload uploadPayload
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err == nil {
			job.OriginalFilename = payload.OriginalFilename
			job.BookID = payload.BookID
		}
		job.CreatedAt = parseSQLiteTime(createdAt)
		if startedAt.Valid {
			value := parseSQLiteTime(startedAt.String)
			job.StartedAt = &value
		}
		if completedAt.Valid {
			value := parseSQLiteTime(completedAt.String)
			job.CompletedAt = &value
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate import jobs: %w", err)
	}
	return jobs, nil
}

func (s *Service) run(ctx context.Context) {
	defer s.wait.Done()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		for {
			jobID, ok, err := s.claimNext(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				s.logger.Error("claim import job", "error", err)
				break
			}
			if !ok {
				break
			}
			s.process(ctx, jobID)
		}

		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-ticker.C:
		}
	}
}

func (s *Service) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) claimNext(ctx context.Context) (int64, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()

	var jobID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM jobs
		WHERE job_type = ? AND status = 'pending'
		ORDER BY id
		LIMIT 1
	`, importJobType).Scan(&jobID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'running', progress = 1, attempts = attempts + 1,
			started_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), error_message = NULL
		WHERE id = ? AND status = 'pending'
	`, jobID)
	if err != nil {
		return 0, false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, false, err
	}
	if rowsAffected != 1 {
		return 0, false, nil
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return jobID, true, nil
}

func (s *Service) process(ctx context.Context, jobID int64) {
	payload, err := s.loadPayload(ctx, jobID)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.failJob(context.WithoutCancel(ctx), jobID, "Could not read import data.", err)
		return
	}

	var existingID int64
	err = s.db.QueryRowContext(ctx, "SELECT id FROM books WHERE file_hash = ?", payload.FileHash).Scan(&existingID)
	if err == nil {
		_ = os.Remove(payload.UploadPath)
		s.failJob(context.WithoutCancel(ctx), jobID, ErrDuplicateBook.Error(), ErrDuplicateBook)
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		if ctx.Err() != nil {
			return
		}
		s.failJob(context.WithoutCancel(ctx), jobID, "Could not inspect the library.", err)
		return
	}

	destinationExtension := managedComicExtension(payload.OriginalFilename)
	destinationPath := filepath.Join(s.libraryDir, payload.FileHash+destinationExtension)
	sourcePath := payload.UploadPath
	alreadyMoved := false
	if _, err := os.Stat(sourcePath); errors.Is(err, os.ErrNotExist) {
		if _, destinationErr := os.Stat(destinationPath); destinationErr == nil {
			sourcePath = destinationPath
			alreadyMoved = true
		} else {
			s.failJob(context.WithoutCancel(ctx), jobID, "The temporary import file was not found.", err)
			return
		}
	} else if err != nil {
		s.failJob(context.WithoutCancel(ctx), jobID, "Could not access the temporary file.", err)
		return
	}

	var book inspectedBook
	extension := strings.ToLower(filepath.Ext(payload.OriginalFilename))
	switch extension {
	case ".pdf":
		book, err = inspectPDF(ctx, payload.OriginalFilename, sourcePath)
	case ".epub":
		book, err = inspectEPUB(ctx, payload.OriginalFilename, sourcePath, s.limits)
	case ".cbr", ".rar":
		book, err = inspectPreparedComic(ctx, payload.OriginalFilename, sourcePath, "cbr", payload.FileHash, s.preparedDir, s.limits, s.tools)
	case ".cb7", ".7z":
		book, err = inspectPreparedComic(ctx, payload.OriginalFilename, sourcePath, "cb7", payload.FileHash, s.preparedDir, s.limits, s.tools)
	case ".cbt", ".tar":
		book, err = inspectPreparedComic(ctx, payload.OriginalFilename, sourcePath, "cbt", payload.FileHash, s.preparedDir, s.limits, s.tools)
	default:
		book, err = inspectArchive(ctx, payload.OriginalFilename, sourcePath, s.limits)
	}
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		_ = os.Remove(sourcePath)
		s.failJob(context.WithoutCancel(ctx), jobID, userFacingImportError(err), err)
		return
	}
	applyUploadMetadata(&book, payload.Metadata)
	if ctx.Err() != nil {
		cleanupPreparedBook(book)
		return
	}
	if err := s.updateProgress(ctx, jobID, 45); err != nil {
		s.logger.Warn("update import progress", "job_id", jobID, "error", err)
	}
	if ctx.Err() != nil {
		cleanupPreparedBook(book)
		return
	}
	if !alreadyMoved {
		if err := os.Rename(sourcePath, destinationPath); err != nil {
			cleanupPreparedBook(book)
			s.failJob(context.WithoutCancel(ctx), jobID, "Could not move the file into the library.", err)
			return
		}
	}
	if err := s.updateProgress(ctx, jobID, 70); err != nil {
		s.logger.Warn("update import progress", "job_id", jobID, "error", err)
	}

	bookID, err := s.persistBook(ctx, jobID, payload, book, destinationPath)
	if err != nil {
		if ctx.Err() != nil {
			cleanupPreparedBook(book)
			if !alreadyMoved {
				_ = os.Rename(destinationPath, payload.UploadPath)
			}
			return
		}
		cleanupPreparedBook(book)
		if !alreadyMoved {
			_ = os.Rename(destinationPath, payload.UploadPath)
		}
		s.failJob(context.WithoutCancel(ctx), jobID, "Could not save the book to the database.", err)
		return
	}

	s.logger.Info("book imported", "job_id", jobID, "book_id", bookID, "title", book.Title, "format", book.Format, "pages", book.PageCount)
}

func normalizeUploadMetadata(metadata UploadMetadata) UploadMetadata {
	return UploadMetadata{
		Title:  cleanMetadata(metadata.Title, 240),
		Series: cleanMetadata(metadata.Series, 240),
		Volume: cleanMetadata(metadata.Volume, 100),
	}
}

func applyUploadMetadata(book *inspectedBook, metadata UploadMetadata) {
	if book == nil {
		return
	}
	if metadata.Title != "" {
		book.Title = metadata.Title
	}
	if metadata.Series != "" {
		book.Series = metadata.Series
	}
	if metadata.Volume != "" {
		book.Volume = metadata.Volume
	}
}

func (s *Service) loadPayload(ctx context.Context, jobID int64) (uploadPayload, error) {
	var payloadJSON string
	if err := s.db.QueryRowContext(ctx, "SELECT payload FROM jobs WHERE id = ?", jobID).Scan(&payloadJSON); err != nil {
		return uploadPayload{}, err
	}
	var payload uploadPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return uploadPayload{}, err
	}
	return payload, nil
}

func (s *Service) persistBook(ctx context.Context, jobID int64, payload uploadPayload, book inspectedBook, destinationPath string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	libraryID, err := ensureDefaultLibrary(ctx, tx, s.libraryDir)
	if err != nil {
		return 0, err
	}
	var seriesID any
	if book.Series != "" {
		resolvedSeriesID, seriesErr := ensureSeries(ctx, tx, libraryID, book.Series)
		if seriesErr != nil {
			return 0, seriesErr
		}
		seriesID = resolvedSeriesID
	}
	readingDirection := book.ReadingDirection
	if readingDirection == nil {
		direction, directionErr := defaultReadingDirection(ctx, tx)
		if directionErr != nil {
			return 0, directionErr
		}
		readingDirection = &direction
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO books(
			library_id, series_id, title, file_path, original_filename, file_size, file_hash,
			page_count, status, summary, writer, publisher, publication_year, volume, number,
			language, reading_direction, format, pdf_analysis_status,
			epub_layout, epub_version, epub_package_path, epub_cover_path
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'processing', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, libraryID, seriesID, book.Title, destinationPath, payload.OriginalFilename, payload.FileSize,
		payload.FileHash, book.PageCount, book.Summary, book.Writer, book.Publisher, nullableYear(book.PublicationYear),
		book.Volume, book.Number, book.Language, nullableDirection(readingDirection), book.Format, pdfAnalysisStatus(book),
		epubLayout(book), book.EPUBVersion, book.EPUBPackagePath, book.EPUBCoverPath)
	if err != nil {
		return 0, err
	}
	bookID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	statement, err := tx.PrepareContext(ctx, `
		INSERT INTO pages(book_id, page_number, archive_path, media_type, width, height, file_size)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, err
	}
	defer statement.Close()
	for pageNumber, page := range book.Pages {
		if _, err := statement.ExecContext(ctx, bookID, pageNumber, page.ArchivePath, page.MediaType, page.Width, page.Height, page.FileSize); err != nil {
			return 0, err
		}
	}
	if err := persistEPUBData(ctx, tx, bookID, book); err != nil {
		return 0, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE books SET status = 'ready', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?
	`, bookID); err != nil {
		return 0, err
	}
	payload.BookID = &bookID
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'completed', progress = 100, payload = ?,
			completed_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), error_message = NULL
		WHERE id = ?
	`, string(encodedPayload), jobID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return bookID, nil
}

func pdfAnalysisStatus(book inspectedBook) string {
	if book.Format != "pdf" {
		return "not_applicable"
	}
	if book.PDFAnalysisStatus == "complete" || book.PDFAnalysisStatus == "failed" {
		return book.PDFAnalysisStatus
	}
	return "pending"
}

func epubLayout(book inspectedBook) string {
	if book.Format != "epub" {
		return "not_applicable"
	}
	if book.EPUBLayout == "fixed" || book.EPUBLayout == "reflowable" {
		return book.EPUBLayout
	}
	return "reflowable"
}

func persistEPUBData(ctx context.Context, tx *sql.Tx, bookID int64, book inspectedBook) error {
	if book.Format != "epub" {
		return nil
	}
	resourceStatement, err := tx.PrepareContext(ctx, `
		INSERT INTO epub_resources(book_id, resource_path, item_id, media_type, properties, file_size)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer resourceStatement.Close()
	for _, resource := range book.EPUBResources {
		if _, err := resourceStatement.ExecContext(ctx, bookID, resource.Path, resource.ItemID, resource.MediaType, resource.Properties, resource.FileSize); err != nil {
			return err
		}
	}

	spineStatement, err := tx.PrepareContext(ctx, `
		INSERT INTO epub_spine(
			book_id, spine_index, item_id, resource_path, media_type, properties,
			linear, title, text_content, search_content
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer spineStatement.Close()
	for _, item := range book.EPUBSpine {
		linear := 0
		if item.Linear {
			linear = 1
		}
		if _, err := spineStatement.ExecContext(ctx, bookID, item.Index, item.ItemID, item.Path, item.MediaType, item.Properties, linear, item.Title, item.TextContent, item.SearchContent); err != nil {
			return err
		}
	}

	tocStatement, err := tx.PrepareContext(ctx, `
		INSERT INTO epub_toc(book_id, position, label, resource_path, fragment, spine_index, depth)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer tocStatement.Close()
	for _, item := range book.EPUBTOC {
		var spineIndex any
		if item.SpineIndex != nil {
			spineIndex = *item.SpineIndex
		}
		if _, err := tocStatement.ExecContext(ctx, bookID, item.Position, item.Label, item.Path, item.Fragment, spineIndex, item.Depth); err != nil {
			return err
		}
	}
	return nil
}

func ensureSeries(ctx context.Context, tx *sql.Tx, libraryID int64, title string) (int64, error) {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO series(library_id, title) VALUES (?, ?)
		ON CONFLICT(library_id, title) DO NOTHING
	`, libraryID, title); err != nil {
		return 0, err
	}
	var seriesID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM series WHERE library_id = ? AND title = ?`, libraryID, title).Scan(&seriesID); err != nil {
		return 0, err
	}
	return seriesID, nil
}

func nullableYear(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func defaultReadingDirection(ctx context.Context, tx *sql.Tx) (string, error) {
	direction := "ltr"
	err := tx.QueryRowContext(ctx, `SELECT value FROM instance_settings WHERE key = 'default_reading_direction'`).Scan(&direction)
	if errors.Is(err, sql.ErrNoRows) {
		return "ltr", nil
	}
	if err != nil {
		return "", fmt.Errorf("load default reading direction: %w", err)
	}
	if direction != "ltr" && direction != "rtl" {
		return "ltr", nil
	}
	return direction, nil
}

func nullableDirection(value *string) any {
	if value == nil || *value == "" {
		return nil
	}
	return *value
}

func ensureDefaultLibrary(ctx context.Context, tx *sql.Tx, path string) (int64, error) {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO libraries(name, path, management_mode)
		VALUES ('Main library', ?, 'managed')
		ON CONFLICT(path) DO NOTHING
	`, path); err != nil {
		return 0, err
	}
	var libraryID int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM libraries WHERE path = ?", path).Scan(&libraryID); err != nil {
		return 0, err
	}
	return libraryID, nil
}

func (s *Service) updateProgress(ctx context.Context, jobID int64, progress int) error {
	_, err := s.db.ExecContext(ctx, "UPDATE jobs SET progress = ? WHERE id = ? AND status = 'running'", progress, jobID)
	return err
}

func (s *Service) failJob(ctx context.Context, jobID int64, message string, cause error) {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'failed', progress = 100, error_message = ?,
			completed_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?
	`, message, jobID); err != nil {
		s.logger.Error("mark import job as failed", "job_id", jobID, "error", err, "cause", cause)
		return
	}
	s.logger.Warn("book import failed", "job_id", jobID, "error", cause)
}

func normalizeOriginalFilename(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = filepath.Base(value)
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return -1
		}
		return character
	}, value)
	value = strings.TrimSpace(value)
	if value == "" || value == "." {
		return "book.cbz"
	}

	runes := []rune(value)
	if len(runes) > 240 {
		extension := filepath.Ext(value)
		baseRunes := []rune(strings.TrimSuffix(value, extension))
		maxBase := 240 - len([]rune(extension))
		if maxBase < 1 {
			maxBase = 1
		}
		if len(baseRunes) > maxBase {
			baseRunes = baseRunes[:maxBase]
		}
		value = string(baseRunes) + extension
	}
	return value
}

func randomHex(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func parseSQLiteTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000Z", "2006-01-02T15:04:05Z"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func userFacingImportError(err error) string {
	switch {
	case errors.Is(err, ErrDuplicateBook):
		return ErrDuplicateBook.Error()
	case errors.Is(err, ErrInvalidArchive):
		return strings.TrimPrefix(err.Error(), ErrInvalidArchive.Error()+": ")
	case errors.Is(err, ErrInvalidPDF):
		return strings.TrimPrefix(err.Error(), ErrInvalidPDF.Error()+": ")
	case errors.Is(err, ErrInvalidEPUB), errors.Is(err, ErrEPUBDRM):
		return strings.TrimPrefix(strings.TrimPrefix(err.Error(), ErrInvalidEPUB.Error()+": "), ErrEPUBDRM.Error()+": ")
	case errors.Is(err, ErrArchiveToolUnavailable):
		return ErrArchiveToolUnavailable.Error()
	default:
		return "Could not import the file."
	}
}

func managedComicExtension(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".pdf":
		return ".pdf"
	case ".epub":
		return ".epub"
	case ".cbr", ".rar":
		return ".cbr"
	case ".cb7", ".7z":
		return ".cb7"
	case ".cbt", ".tar":
		return ".cbt"
	default:
		return ".cbz"
	}
}

func cleanupPreparedBook(book inspectedBook) {
	if strings.TrimSpace(book.PreparedDir) != "" {
		_ = os.RemoveAll(book.PreparedDir)
	}
}
