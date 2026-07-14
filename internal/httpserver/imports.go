package httpserver

import (
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"

	"samrai/internal/auth"
	"samrai/internal/catalog"
	"samrai/internal/imagecache"
	"samrai/internal/importer"
)

type jobsResponse struct {
	Items []importer.Job `json:"items"`
}

type booksResponse struct {
	Items []catalog.Book `json:"items"`
	Total int            `json:"total"`
}

func (s *Server) uploadBook(w http.ResponseWriter, r *http.Request) {
	maxBody := s.importer.MaxUploadBytes() + 1024*1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)

	multipartReader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_multipart", "Send the file as multipart/form-data.")
		return
	}

	var queued *importer.Job
	for {
		part, err := multipartReader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var maxBytesError *http.MaxBytesError
			if errors.As(err, &maxBytesError) {
				writeError(w, http.StatusRequestEntityTooLarge, "upload_too_large", "The file exceeds the configured limit.")
				return
			}
			writeError(w, http.StatusBadRequest, "invalid_multipart", "Could not read the upload.")
			return
		}

		if part.FormName() != "file" || part.FileName() == "" {
			_ = part.Close()
			continue
		}
		if queued != nil {
			_ = part.Close()
			writeError(w, http.StatusBadRequest, "too_many_files", "Send one file per request.")
			return
		}

		job, queueErr := s.importer.QueueUploadWithMetadata(r.Context(), part.FileName(), part, importer.UploadMetadata{
			Title:  r.URL.Query().Get("title"),
			Series: r.URL.Query().Get("series"),
			Volume: r.URL.Query().Get("volume"),
		})
		_ = part.Close()
		if queueErr != nil {
			switch {
			case errors.Is(queueErr, importer.ErrInvalidExtension):
				writeError(w, http.StatusUnprocessableEntity, "unsupported_file", queueErr.Error())
			case errors.Is(queueErr, importer.ErrUploadTooLarge):
				writeError(w, http.StatusRequestEntityTooLarge, "upload_too_large", queueErr.Error())
			case errors.Is(queueErr, importer.ErrInvalidArchive):
				writeError(w, http.StatusUnprocessableEntity, "invalid_archive", queueErr.Error())
			case errors.Is(queueErr, importer.ErrInvalidPDF):
				writeError(w, http.StatusUnprocessableEntity, "invalid_pdf", queueErr.Error())
			case errors.Is(queueErr, importer.ErrInvalidEPUB), errors.Is(queueErr, importer.ErrEPUBDRM):
				writeError(w, http.StatusUnprocessableEntity, "invalid_epub", queueErr.Error())
			case errors.Is(queueErr, importer.ErrDuplicateBook):
				writeError(w, http.StatusConflict, "duplicate_book", queueErr.Error())
			default:
				s.logger.Error("queue book upload", "error", queueErr)
				writeError(w, http.StatusInternalServerError, "internal_error", "Could not store the upload.")
			}
			return
		}
		queued = &job
	}

	if queued == nil {
		writeError(w, http.StatusBadRequest, "file_required", "Select a CBZ, CBR, CB7, CBT, PDF, or EPUB file.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusAccepted, queued)
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.importer.ListJobs(r.Context(), 250)
	if err != nil {
		s.logger.Error("list import jobs", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not list uploads.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, jobsResponse{Items: jobs})
}

func (s *Server) listBooks(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	options := catalog.BookListOptions{
		Search:       r.URL.Query().Get("search"),
		Status:       r.URL.Query().Get("status"),
		Sort:         r.URL.Query().Get("sort"),
		Limit:        queryInt(r, "limit", 100),
		Offset:       queryInt(r, "offset", 0),
		FavoriteOnly: r.URL.Query().Get("favorite") == "1" || r.URL.Query().Get("favorite") == "true",
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("series_id")); raw != "" {
		seriesID, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || seriesID < 1 {
			writeError(w, http.StatusBadRequest, "invalid_series_id", "Invalid series ID.")
			return
		}
		options.SeriesID = seriesID
	}
	result, err := s.catalog.QueryBooks(r.Context(), principal.User.ID, options)
	if err != nil {
		s.logger.Error("list books", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not load the library.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, booksResponse{Items: result.Items, Total: result.Total})
}

func queryInt(r *http.Request, name string, fallback int) int {
	value := strings.TrimSpace(r.URL.Query().Get(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func (s *Server) bookCover(w http.ResponseWriter, r *http.Request) {
	bookID, err := strconv.ParseInt(r.PathValue("bookID"), 10, 64)
	if err != nil || bookID < 1 {
		writeError(w, http.StatusBadRequest, "invalid_book_id", "Invalid book ID.")
		return
	}
	principal, authenticated := auth.PrincipalFromContext(r.Context())
	if !authenticated {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	book, bookErr := s.catalog.GetBook(r.Context(), principal.User.ID, bookID)
	if errors.Is(bookErr, catalog.ErrBookNotFound) {
		writeError(w, http.StatusNotFound, "book_not_found", "Book or cover not found.")
		return
	}
	if bookErr != nil {
		s.logger.Error("find book cover", "book_id", bookID, "error", bookErr)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not open the cover.")
		return
	}
	if book.Format == "epub" {
		epubCover, coverErr := s.catalog.OpenEPUBCover(r.Context(), bookID)
		if coverErr == nil {
			defer epubCover.Stream.Close()
			if r.Header.Get("If-None-Match") == epubCover.ETag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("Content-Type", epubCover.MediaType)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", epubCover.Size))
			w.Header().Set("Content-Disposition", `inline; filename="cover"`)
			w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
			w.Header().Set("ETag", epubCover.ETag)
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusOK)
				return
			}
			if _, copyErr := io.Copy(w, epubCover.Stream); copyErr != nil && !strings.Contains(strings.ToLower(copyErr.Error()), "broken pipe") {
				s.logger.Warn("stream epub cover", "book_id", bookID, "error", copyErr)
			}
			return
		}
		if !errors.Is(coverErr, catalog.ErrPageNotFound) {
			s.logger.Warn("open epub cover", "book_id", bookID, "error", coverErr)
		}
		serveEPUBPlaceholderCover(w, r, book)
		return
	}

	if book.Format == "pdf" {
		pdfCover, coverErr := s.catalog.OpenPDFCover(r.Context(), bookID)
		if coverErr == nil {
			defer pdfCover.Stream.Close()
			if r.Header.Get("If-None-Match") == pdfCover.ETag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("Content-Type", pdfCover.MediaType)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", pdfCover.Size))
			w.Header().Set("Content-Disposition", `inline; filename="cover"`)
			w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
			w.Header().Set("ETag", pdfCover.ETag)
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusOK)
				return
			}
			if _, copyErr := io.Copy(w, pdfCover.Stream); copyErr != nil && !strings.Contains(strings.ToLower(copyErr.Error()), "broken pipe") {
				s.logger.Warn("stream pdf cover", "book_id", bookID, "error", copyErr)
			}
			return
		}
		if !errors.Is(coverErr, catalog.ErrPageNotFound) {
			s.logger.Warn("open stored pdf cover", "book_id", bookID, "error", coverErr)
		}
		servePDFPlaceholderCover(w, r, book)
		return
	}

	width := imagecache.ParseWidth(r.URL.Query().Get("width"), 480, 2048)
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "webp"
	}

	var cover imagecache.Asset
	if s.images != nil {
		cover, err = s.images.Open(r.Context(), bookID, 0, width, imagecache.KindCover, format)
	} else {
		original, originalErr := s.catalog.OpenCover(r.Context(), bookID)
		err = originalErr
		if originalErr == nil {
			cover = imagecache.Asset{Stream: original.Stream, MediaType: original.MediaType, Size: original.Size, ETag: original.ETag, Width: original.Width, Height: original.Height, PageNumber: original.PageNumber, Cache: "original"}
		}
	}
	if errors.Is(err, catalog.ErrBookNotFound) {
		writeError(w, http.StatusNotFound, "book_not_found", "Book or cover not found.")
		return
	}
	if err != nil {
		s.logger.Error("open book cover", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not open the cover.")
		return
	}
	defer cover.Stream.Close()

	if r.Header.Get("If-None-Match") == cover.ETag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", cover.MediaType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", cover.Size))
	w.Header().Set("Content-Disposition", `inline; filename="cover"`)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("ETag", cover.ETag)
	w.Header().Set("X-Page-Cache", cover.Cache)
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	if _, err := io.Copy(w, cover.Stream); err != nil && !strings.Contains(strings.ToLower(err.Error()), "broken pipe") {
		s.logger.Warn("stream book cover", "book_id", bookID, "error", err)
	}
}

func serveEPUBPlaceholderCover(w http.ResponseWriter, r *http.Request, book catalog.Book) {
	title := []rune(strings.TrimSpace(book.Title))
	if len(title) > 48 {
		title = append(title[:45], '…')
	}
	label := html.EscapeString(string(title))
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="720" height="1000" viewBox="0 0 720 1000"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#101d20"/><stop offset="1" stop-color="#315b53"/></linearGradient></defs><rect width="720" height="1000" rx="36" fill="url(#g)"/><rect x="62" y="70" width="596" height="860" rx="24" fill="#fff" fill-opacity=".07" stroke="#fff" stroke-opacity=".14"/><text x="360" y="365" text-anchor="middle" fill="#fff" font-family="system-ui,sans-serif" font-size="104" font-weight="800">EPUB</text><path d="M230 430h260" stroke="#72e0c3" stroke-width="12" stroke-linecap="round"/><foreignObject x="105" y="520" width="510" height="250"><div xmlns="http://www.w3.org/1999/xhtml" style="color:#fff;font:700 46px/1.18 system-ui,sans-serif;text-align:center;overflow-wrap:anywhere">%s</div></foreignObject><text x="360" y="870" text-anchor="middle" fill="#d5eee8" font-family="system-ui,sans-serif" font-size="24">samrai · EPUB %s</text></svg>`, label, html.EscapeString(book.EPUBLayout))
	etag := fmt.Sprintf(`"epub-cover-%d-%d"`, book.ID, book.UpdatedAt.Unix())
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(svg)))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("ETag", etag)
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = io.WriteString(w, svg)
}

func servePDFPlaceholderCover(w http.ResponseWriter, r *http.Request, book catalog.Book) {
	title := []rune(strings.TrimSpace(book.Title))
	if len(title) > 48 {
		title = append(title[:45], '…')
	}
	label := html.EscapeString(string(title))
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="720" height="1000" viewBox="0 0 720 1000"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#17131f"/><stop offset="1" stop-color="#3a244b"/></linearGradient></defs><rect width="720" height="1000" rx="36" fill="url(#g)"/><rect x="62" y="70" width="596" height="860" rx="24" fill="#fff" fill-opacity=".07" stroke="#fff" stroke-opacity=".14"/><text x="360" y="365" text-anchor="middle" fill="#fff" font-family="system-ui,sans-serif" font-size="118" font-weight="800">PDF</text><path d="M250 430h220" stroke="#c59cff" stroke-width="12" stroke-linecap="round"/><foreignObject x="105" y="520" width="510" height="250"><div xmlns="http://www.w3.org/1999/xhtml" style="color:#fff;font:700 46px/1.18 system-ui,sans-serif;text-align:center;overflow-wrap:anywhere">%s</div></foreignObject><text x="360" y="870" text-anchor="middle" fill="#d8c9e8" font-family="system-ui,sans-serif" font-size="24">samrai · books and annotations</text></svg>`, label)
	etag := fmt.Sprintf(`"pdf-cover-%d-%d"`, book.ID, book.UpdatedAt.Unix())
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(svg)))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("ETag", etag)
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = io.WriteString(w, svg)
}
