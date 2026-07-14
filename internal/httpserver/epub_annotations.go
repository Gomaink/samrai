package httpserver

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"samrai/internal/auth"
	"samrai/internal/catalog"
)

type epubAnnotationRequest struct {
	SpineIndex   int                          `json:"spine_index"`
	ResourcePath string                       `json:"resource_path"`
	Kind         string                       `json:"kind"`
	Color        string                       `json:"color"`
	SelectedText string                       `json:"selected_text"`
	Note         string                       `json:"note"`
	Anchor       catalog.EPUBAnnotationAnchor `json:"anchor"`
}

type updateEPUBAnnotationRequest struct {
	Color *string `json:"color"`
	Note  *string `json:"note"`
}

type epubAnnotationsResponse struct {
	Items []catalog.EPUBAnnotation `json:"items"`
}

func (s *Server) listEPUBAnnotations(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	items, err := s.catalog.ListEPUBAnnotations(r.Context(), principal.User.ID, bookID)
	switch {
	case errors.Is(err, catalog.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book_not_found", "EPUB not found.")
	case errors.Is(err, catalog.ErrUnsupportedFormat):
		writeError(w, http.StatusUnprocessableEntity, "unsupported_format", "EPUB annotations are available only for EPUB files.")
	case err != nil:
		s.logger.Error("list epub annotations", "book_id", bookID, "user_id", principal.User.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not load EPUB highlights.")
	default:
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, epubAnnotationsResponse{Items: items})
	}
}

func (s *Server) createEPUBAnnotation(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	var request epubAnnotationRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid EPUB highlight data.")
		return
	}
	annotation, err := s.catalog.CreateEPUBAnnotation(r.Context(), principal.User.ID, bookID, catalog.CreateEPUBAnnotationInput{
		SpineIndex: request.SpineIndex, ResourcePath: request.ResourcePath, Kind: request.Kind,
		Color: request.Color, SelectedText: request.SelectedText, Note: request.Note, Anchor: request.Anchor,
	})
	switch {
	case errors.Is(err, catalog.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book_not_found", "EPUB not found.")
	case errors.Is(err, catalog.ErrUnsupportedFormat):
		writeError(w, http.StatusUnprocessableEntity, "unsupported_format", "This book is not an EPUB.")
	case errors.Is(err, catalog.ErrInvalidAnnotation):
		writeError(w, http.StatusUnprocessableEntity, "invalid_annotation", "The EPUB highlight is invalid.")
	case err != nil:
		s.logger.Error("create epub annotation", "book_id", bookID, "user_id", principal.User.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not save the EPUB highlight.")
	default:
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, annotation)
	}
}

func (s *Server) updateEPUBAnnotation(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	annotationID, ok := parsePositivePathID(w, r, "annotationID", "invalid_annotation_id", "Invalid highlight ID.")
	if !ok {
		return
	}
	var request updateEPUBAnnotationRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid EPUB highlight data.")
		return
	}
	annotation, err := s.catalog.UpdateEPUBAnnotation(r.Context(), principal.User.ID, annotationID, catalog.UpdateEPUBAnnotationInput{Color: request.Color, Note: request.Note})
	switch {
	case errors.Is(err, catalog.ErrEPUBAnnotationNotFound):
		writeError(w, http.StatusNotFound, "annotation_not_found", "EPUB highlight not found.")
	case errors.Is(err, catalog.ErrInvalidAnnotation):
		writeError(w, http.StatusUnprocessableEntity, "invalid_annotation", "The EPUB highlight is invalid.")
	case err != nil:
		s.logger.Error("update epub annotation", "annotation_id", annotationID, "user_id", principal.User.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not update the EPUB highlight.")
	default:
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, annotation)
	}
}

func (s *Server) deleteEPUBAnnotation(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	annotationID, ok := parsePositivePathID(w, r, "annotationID", "invalid_annotation_id", "Invalid highlight ID.")
	if !ok {
		return
	}
	err := s.catalog.DeleteEPUBAnnotation(r.Context(), principal.User.ID, annotationID)
	if errors.Is(err, catalog.ErrEPUBAnnotationNotFound) {
		writeError(w, http.StatusNotFound, "annotation_not_found", "EPUB highlight not found.")
		return
	}
	if err != nil {
		s.logger.Error("delete epub annotation", "annotation_id", annotationID, "user_id", principal.User.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not delete the EPUB highlight.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) exportEPUBAnnotations(w http.ResponseWriter, r *http.Request) {
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
	if err != nil || book.Format != "epub" {
		writeError(w, http.StatusNotFound, "book_not_found", "EPUB not found.")
		return
	}
	items, err := s.catalog.ListEPUBAnnotations(r.Context(), principal.User.ID, bookID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not export EPUB highlights.")
		return
	}
	filename := strings.TrimSuffix(book.OriginalFilename, ".epub") + "-annotations.md"
	filename = strings.ReplaceAll(filename, `"`, "")
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = fmt.Fprintf(w, "# %s\n\n", book.Title)
	for _, item := range items {
		_, _ = fmt.Fprintf(w, "## Section %d\n\n", item.SpineIndex+1)
		if item.SelectedText != "" {
			for _, line := range strings.Split(item.SelectedText, "\n") {
				_, _ = fmt.Fprintf(w, "> %s\n", line)
			}
			_, _ = fmt.Fprintln(w)
		}
		if item.Note != "" {
			_, _ = fmt.Fprintf(w, "**Note:** %s\n\n", item.Note)
		}
	}
}
