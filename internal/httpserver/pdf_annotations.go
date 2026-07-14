package httpserver

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"samrai/internal/auth"
	"samrai/internal/catalog"
)

type pdfMetadataRequest struct {
	PageCount int `json:"page_count"`
}

type annotationRequest struct {
	PageNumber   int                      `json:"page_number"`
	Kind         string                   `json:"kind"`
	Color        string                   `json:"color"`
	SelectedText string                   `json:"selected_text"`
	Note         string                   `json:"note"`
	Anchor       catalog.AnnotationAnchor `json:"anchor"`
}

type updateAnnotationRequest struct {
	Color *string `json:"color"`
	Note  *string `json:"note"`
}

type annotationsResponse struct {
	Items []catalog.Annotation `json:"items"`
}

func (s *Server) bookFile(w http.ResponseWriter, r *http.Request) {
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	asset, err := s.catalog.OpenBookFile(r.Context(), bookID)
	switch {
	case errors.Is(err, catalog.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
		return
	case errors.Is(err, catalog.ErrUnsupportedFormat):
		writeError(w, http.StatusUnprocessableEntity, "unsupported_format", "This book does not have a PDF file.")
		return
	case err != nil:
		s.logger.Error("open pdf book", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not open the PDF.")
		return
	}
	defer asset.File.Close()

	if r.Header.Get("If-None-Match") == asset.ETag && r.Header.Get("Range") == "" {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	filename := strings.ReplaceAll(asset.Filename, `"`, "")
	w.Header().Set("Content-Type", asset.MediaType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("ETag", asset.ETag)
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, filename, asset.ModTime, asset.File)
}

func (s *Server) updatePDFMetadata(w http.ResponseWriter, r *http.Request) {
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	var request pdfMetadataRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Provide the PDF page count.")
		return
	}
	err := s.catalog.UpdatePDFPageCount(r.Context(), bookID, request.PageCount)
	switch {
	case errors.Is(err, catalog.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
		return
	case errors.Is(err, catalog.ErrUnsupportedFormat):
		writeError(w, http.StatusUnprocessableEntity, "unsupported_format", "This book is not a PDF.")
		return
	case errors.Is(err, catalog.ErrInvalidBook):
		writeError(w, http.StatusUnprocessableEntity, "invalid_page_count", "Invalid page count.")
		return
	case err != nil:
		s.logger.Error("update pdf metadata", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not update PDF data.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listAnnotations(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	items, err := s.catalog.ListAnnotations(r.Context(), principal.User.ID, bookID)
	if errors.Is(err, catalog.ErrBookNotFound) {
		writeError(w, http.StatusNotFound, "book_not_found", "PDF not found.")
		return
	}
	if err != nil {
		s.logger.Error("list annotations", "book_id", bookID, "user_id", principal.User.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not load highlights.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, annotationsResponse{Items: items})
}

func (s *Server) createAnnotation(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	var request annotationRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid highlight data.")
		return
	}
	annotation, err := s.catalog.CreateAnnotation(r.Context(), principal.User.ID, bookID, catalog.CreateAnnotationInput{
		PageNumber: request.PageNumber, Kind: request.Kind, Color: request.Color,
		SelectedText: request.SelectedText, Note: request.Note, Anchor: request.Anchor,
	})
	switch {
	case errors.Is(err, catalog.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book_not_found", "PDF not found.")
		return
	case errors.Is(err, catalog.ErrUnsupportedFormat):
		writeError(w, http.StatusUnprocessableEntity, "unsupported_format", "Text highlights are available only for PDF files.")
		return
	case errors.Is(err, catalog.ErrPageOutOfRange), errors.Is(err, catalog.ErrInvalidAnnotation):
		writeError(w, http.StatusUnprocessableEntity, "invalid_annotation", "The highlight is invalid.")
		return
	case err != nil:
		s.logger.Error("create annotation", "book_id", bookID, "user_id", principal.User.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not save the highlight.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, annotation)
}

func (s *Server) updateAnnotation(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	annotationID, ok := parsePositivePathID(w, r, "annotationID", "invalid_annotation_id", "Invalid highlight ID.")
	if !ok {
		return
	}
	var request updateAnnotationRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid highlight data.")
		return
	}
	annotation, err := s.catalog.UpdateAnnotation(r.Context(), principal.User.ID, annotationID, catalog.UpdateAnnotationInput{Color: request.Color, Note: request.Note})
	switch {
	case errors.Is(err, catalog.ErrAnnotationNotFound):
		writeError(w, http.StatusNotFound, "annotation_not_found", "Highlight not found.")
		return
	case errors.Is(err, catalog.ErrInvalidAnnotation):
		writeError(w, http.StatusUnprocessableEntity, "invalid_annotation", "The highlight is invalid.")
		return
	case err != nil:
		s.logger.Error("update annotation", "annotation_id", annotationID, "user_id", principal.User.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not update the highlight.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, annotation)
}

func (s *Server) deleteAnnotation(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	annotationID, ok := parsePositivePathID(w, r, "annotationID", "invalid_annotation_id", "Invalid highlight ID.")
	if !ok {
		return
	}
	err := s.catalog.DeleteAnnotation(r.Context(), principal.User.ID, annotationID)
	if errors.Is(err, catalog.ErrAnnotationNotFound) {
		writeError(w, http.StatusNotFound, "annotation_not_found", "Highlight not found.")
		return
	}
	if err != nil {
		s.logger.Error("delete annotation", "annotation_id", annotationID, "user_id", principal.User.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not delete the highlight.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) exportAnnotations(w http.ResponseWriter, r *http.Request) {
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
	if err != nil || book.Format != "pdf" {
		writeError(w, http.StatusNotFound, "book_not_found", "PDF not found.")
		return
	}
	items, err := s.catalog.ListAnnotations(r.Context(), principal.User.ID, bookID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not export highlights.")
		return
	}
	filename := strings.TrimSuffix(book.OriginalFilename, ".pdf") + "-annotations.md"
	filename = strings.ReplaceAll(filename, `"`, "")
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = fmt.Fprintf(w, "# %s\n\n", book.Title)
	for _, item := range items {
		_, _ = fmt.Fprintf(w, "## Page %d\n\n", item.PageNumber+1)
		if item.SelectedText != "" {
			for _, line := range strings.Split(item.SelectedText, "\n") {
				_, _ = fmt.Fprintf(w, "> %s\n", line)
			}
			_, _ = fmt.Fprintln(w)
		} else {
			_, _ = fmt.Fprint(w, "_Area mark._\n\n")
		}
		if item.Note != "" {
			_, _ = fmt.Fprintf(w, "**Note:** %s\n\n", item.Note)
		}
	}
}
