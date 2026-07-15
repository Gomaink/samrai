package httpserver

import (
	"errors"
	"net/http"
	"os"

	"samrai/internal/auth"
	"samrai/internal/catalog"
)

type updateBookRequest struct {
	Title                *string `json:"title"`
	Summary              *string `json:"summary"`
	Writer               *string `json:"writer"`
	Publisher            *string `json:"publisher"`
	PublicationYear      *int    `json:"publication_year"`
	ClearPublicationYear bool    `json:"clear_publication_year"`
	Volume               *string `json:"volume"`
	Number               *string `json:"number"`
	Language             *string `json:"language"`
	ReadingDirection     *string `json:"reading_direction"`
	Series               *string `json:"series"`
}

type setReadingCompletionRequest struct {
	BookIDs   []int64 `json:"book_ids"`
	Completed *bool   `json:"completed"`
}

type setReadingCompletionResponse struct {
	Updated   int  `json:"updated"`
	Completed bool `json:"completed"`
}

func (s *Server) updateBook(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	var request updateBookRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "The provided data is invalid.")
		return
	}
	book, err := s.catalog.UpdateBook(r.Context(), principal.User.ID, bookID, catalog.UpdateBookInput{
		Title: request.Title, Summary: request.Summary, Writer: request.Writer,
		Publisher: request.Publisher, PublicationYear: request.PublicationYear, ClearYear: request.ClearPublicationYear,
		Volume: request.Volume, Number: request.Number, Language: request.Language,
		ReadingDirection: request.ReadingDirection, Series: request.Series,
	})
	switch {
	case errors.Is(err, catalog.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
		return
	case errors.Is(err, catalog.ErrInvalidBook):
		writeError(w, http.StatusUnprocessableEntity, "invalid_book", "Review the provided metadata.")
		return
	case err != nil:
		s.logger.Error("update book", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not update the book.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, book)
}

func (s *Server) deleteBook(w http.ResponseWriter, r *http.Request) {
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	deleted, err := s.catalog.DeleteBook(r.Context(), bookID)
	if errors.Is(err, catalog.ErrBookNotFound) {
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
		return
	}
	if err != nil {
		s.logger.Error("delete book", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not delete the book.")
		return
	}
	if err := os.RemoveAll(deleted.PreparedPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.logger.Warn("book record deleted but prepared pages could not be removed", "book_id", bookID, "path", deleted.PreparedPath, "error", err)
	}
	if err := os.Remove(deleted.FilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.logger.Warn("book record deleted but archive could not be removed", "book_id", bookID, "path", deleted.FilePath, "error", err)
	}
	if s.images != nil {
		if err := s.images.PurgeBook(deleted.FileHash); err != nil {
			s.logger.Warn("purge deleted book cache", "book_id", bookID, "error", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setBooksCompletion(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	var request setReadingCompletionRequest
	if err := decodeJSON(w, r, &request); err != nil || request.Completed == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Provide book_ids and completed.")
		return
	}
	updated, err := s.catalog.SetCompletion(r.Context(), principal.User.ID, request.BookIDs, *request.Completed)
	switch {
	case errors.Is(err, catalog.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book_not_found", "One or more books were not found.")
		return
	case errors.Is(err, catalog.ErrInvalidBook):
		writeError(w, http.StatusUnprocessableEntity, "invalid_books", "Provide between 1 and 500 valid book IDs.")
		return
	case err != nil:
		s.logger.Error("set reading completion", "user_id", principal.User.ID, "book_count", len(request.BookIDs), "completed", *request.Completed, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not update reading status.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, setReadingCompletionResponse{Updated: updated, Completed: *request.Completed})
}

func (s *Server) resetBookProgress(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	if err := s.catalog.ResetProgress(r.Context(), principal.User.ID, bookID); errors.Is(err, catalog.ErrBookNotFound) {
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
		return
	} else if err != nil {
		s.logger.Error("reset reading progress", "book_id", bookID, "user_id", principal.User.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not reset reading progress.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
