package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"samrai/internal/auth"
	"samrai/internal/catalog"
	"samrai/internal/imagecache"
)

type saveProgressRequest struct {
	CurrentPage int             `json:"current_page"`
	Location    json.RawMessage `json:"location"`
	SessionID   string          `json:"session_id"`
}

func (s *Server) getBook(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}

	book, err := s.catalog.GetBook(r.Context(), principal.User.ID, bookID)
	if errors.Is(err, catalog.ErrBookNotFound) {
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
		return
	}
	if err != nil {
		s.logger.Error("get book", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not open the book.")
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, book)
}

func (s *Server) bookPage(w http.ResponseWriter, r *http.Request) {
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	pageNumber, err := strconv.Atoi(r.PathValue("pageNumber"))
	if err != nil || pageNumber < 0 {
		writeError(w, http.StatusBadRequest, "invalid_page_number", "Invalid page number.")
		return
	}

	requestedWidth := imagecache.ParseWidth(r.URL.Query().Get("width"), 0, 8192)
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" && requestedWidth > 0 {
		format = "webp"
	}

	var page imagecache.Asset
	if s.images != nil {
		page, err = s.images.Open(r.Context(), bookID, pageNumber, requestedWidth, imagecache.KindPage, format)
	} else {
		original, originalErr := s.catalog.OpenPage(r.Context(), bookID, pageNumber)
		err = originalErr
		if originalErr == nil {
			page = imagecache.Asset{Stream: original.Stream, MediaType: original.MediaType, Size: original.Size, ETag: original.ETag, Width: original.Width, Height: original.Height, PageNumber: original.PageNumber, Cache: "original"}
		}
	}
	switch {
	case errors.Is(err, catalog.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
		return
	case errors.Is(err, catalog.ErrPageNotFound), errors.Is(err, catalog.ErrPageOutOfRange):
		writeError(w, http.StatusNotFound, "page_not_found", "Page not found.")
		return
	case err != nil:
		s.logger.Error("open book page", "book_id", bookID, "page", pageNumber, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not open the page.")
		return
	}
	defer page.Stream.Close()

	if r.Header.Get("If-None-Match") == page.ETag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", page.MediaType)
	w.Header().Set("Content-Length", strconv.FormatInt(page.Size, 10))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="page-%d"`, pageNumber+1))
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("ETag", page.ETag)
	w.Header().Set("X-Page-Width", strconv.Itoa(page.Width))
	w.Header().Set("X-Page-Height", strconv.Itoa(page.Height))
	w.Header().Set("X-Page-Cache", page.Cache)
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	if _, err := io.Copy(w, page.Stream); err != nil && !strings.Contains(strings.ToLower(err.Error()), "broken pipe") {
		s.logger.Warn("stream book page", "book_id", bookID, "page", pageNumber, "error", err)
	}
}

func (s *Server) saveBookProgress(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}

	var request saveProgressRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Provide a valid page number.")
		return
	}
	progress, err := s.catalog.SaveProgress(r.Context(), principal.User.ID, bookID, request.CurrentPage, request.Location)
	switch {
	case errors.Is(err, catalog.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
		return
	case errors.Is(err, catalog.ErrPageOutOfRange):
		writeError(w, http.StatusUnprocessableEntity, "page_out_of_range", "The provided page does not belong to this book.")
		return
	case errors.Is(err, catalog.ErrInvalidBook):
		writeError(w, http.StatusUnprocessableEntity, "invalid_location", "The provided reading position is invalid.")
		return
	case err != nil:
		s.logger.Error("save reading progress", "book_id", bookID, "user_id", principal.User.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not save reading progress.")
		return
	}

	if err := s.catalog.RecordReadingActivity(r.Context(), principal.User.ID, bookID, request.SessionID, request.CurrentPage, request.Location); err != nil {
		s.logger.Warn("record reading activity", "book_id", bookID, "user_id", principal.User.ID, "error", err)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, progress)
}

func parsePositivePathID(w http.ResponseWriter, r *http.Request, name, code, message string) (int64, bool) {
	value, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || value < 1 {
		writeError(w, http.StatusBadRequest, code, message)
		return 0, false
	}
	return value, true
}
