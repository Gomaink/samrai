package httpserver

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"samrai/internal/auth"
	"samrai/internal/catalog"
)

type favoriteResponse struct {
	Favorite bool `json:"favorite"`
}

type historyResponse struct {
	Items []catalog.ReadingSession `json:"items"`
}

type allAnnotationsResponse struct {
	Items []catalog.AnnotationSummary `json:"items"`
	Total int                         `json:"total"`
}

type endSessionRequest struct {
	BookID    int64  `json:"book_id"`
	SessionID string `json:"session_id"`
}

func (s *Server) favorites(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	result, err := s.catalog.Favorites(r.Context(), principal.User.ID)
	if err != nil {
		s.logger.Error("load favorites", "user_id", principal.User.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not load favorites.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) favoriteBook(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	favorite := r.Method == http.MethodPut
	err := s.catalog.SetBookFavorite(r.Context(), principal.User.ID, bookID, favorite)
	if errors.Is(err, catalog.ErrBookNotFound) {
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
		return
	}
	if err != nil {
		s.logger.Error("set book favorite", "book_id", bookID, "user_id", principal.User.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not update favorite.")
		return
	}
	writeJSON(w, http.StatusOK, favoriteResponse{Favorite: favorite})
}

func (s *Server) favoriteSeries(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	seriesID, ok := parsePositivePathID(w, r, "seriesID", "invalid_series_id", "Invalid series ID.")
	if !ok {
		return
	}
	favorite := r.Method == http.MethodPut
	err := s.catalog.SetSeriesFavorite(r.Context(), principal.User.ID, seriesID, favorite)
	if errors.Is(err, catalog.ErrSeriesNotFound) {
		writeError(w, http.StatusNotFound, "series_not_found", "Series not found.")
		return
	}
	if err != nil {
		s.logger.Error("set series favorite", "series_id", seriesID, "user_id", principal.User.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not update favorite.")
		return
	}
	writeJSON(w, http.StatusOK, favoriteResponse{Favorite: favorite})
}

func (s *Server) readingHistory(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	items, err := s.catalog.ListReadingHistory(r.Context(), principal.User.ID, queryInt(r, "limit", 100), queryInt(r, "offset", 0))
	if err != nil {
		s.logger.Error("load reading history", "user_id", principal.User.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not load reading history.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, historyResponse{Items: items})
}

func (s *Server) clearReadingHistory(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	if err := s.catalog.ClearReadingHistory(r.Context(), principal.User.ID); err != nil {
		s.logger.Error("clear reading history", "user_id", principal.User.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not clear reading history.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) endReadingSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	var request endSessionRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid reading session.")
		return
	}
	if err := s.catalog.EndReadingSession(r.Context(), principal.User.ID, request.BookID, request.SessionID); err != nil {
		s.logger.Warn("end reading session", "user_id", principal.User.ID, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) allAnnotations(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	items, total, err := s.catalog.ListAllAnnotations(r.Context(), principal.User.ID, catalog.AnnotationListOptions{
		Search: r.URL.Query().Get("search"),
		Color:  r.URL.Query().Get("color"),
		Source: r.URL.Query().Get("source"),
		Limit:  queryInt(r, "limit", 100),
		Offset: queryInt(r, "offset", 0),
	})
	if err != nil {
		s.logger.Error("load all annotations", "user_id", principal.User.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not load annotations.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, allAnnotationsResponse{Items: items, Total: total})
}

func (s *Server) exportAllAnnotations(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	const pageSize = 500
	items := make([]catalog.AnnotationSummary, 0, pageSize)
	for offset := 0; ; offset += pageSize {
		page, total, err := s.catalog.ListAllAnnotations(r.Context(), principal.User.ID, catalog.AnnotationListOptions{Limit: pageSize, Offset: offset})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Could not export annotations.")
			return
		}
		items = append(items, page...)
		if len(items) >= total || len(page) == 0 {
			break
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].BookTitle != items[j].BookTitle {
			return strings.ToLower(items[i].BookTitle) < strings.ToLower(items[j].BookTitle)
		}
		if items[i].BookID != items[j].BookID {
			return items[i].BookID < items[j].BookID
		}
		if items[i].Position != items[j].Position {
			return items[i].Position < items[j].Position
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="samrai-annotations.md"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = fmt.Fprintln(w, "# samrai annotations")
	currentBook := int64(0)
	for _, item := range items {
		if item.BookID != currentBook {
			currentBook = item.BookID
			_, _ = fmt.Fprintf(w, "\n## %s\n\n", item.BookTitle)
		}
		_, _ = fmt.Fprintf(w, "### %s\n\n", item.Location)
		if strings.TrimSpace(item.SelectedText) != "" {
			for _, line := range strings.Split(item.SelectedText, "\n") {
				_, _ = fmt.Fprintf(w, "> %s\n", line)
			}
			_, _ = fmt.Fprintln(w)
		}
		if strings.TrimSpace(item.Note) != "" {
			_, _ = fmt.Fprintf(w, "**Note:** %s\n\n", item.Note)
		}
	}
}

func (s *Server) purgeImageCache(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	if s.images == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := s.images.PurgeAll(); err != nil {
		s.logger.Error("purge image cache", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not clear the image cache.")
		return
	}
	if principal.User.ID > 0 {
		if _, err := s.db.ExecContext(r.Context(), `INSERT INTO audit_events(actor_user_id,event_type,subject_type,subject_id,detail) VALUES(?,'cache.purged','cache','images','manual cleanup from web interface')`, principal.User.ID); err != nil {
			s.logger.Warn("record cache purge audit event", "error", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
