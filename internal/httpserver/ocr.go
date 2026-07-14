package httpserver

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"samrai/internal/ocr"
)

const maxOCRImageBytes = 12 << 20

func (s *Server) ocrStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.ocr == nil {
		writeJSON(w, http.StatusOK, ocr.Status{Available: false, Languages: []string{}, Message: "OCR is not configured for this installation."})
		return
	}
	writeJSON(w, http.StatusOK, s.ocr.Status())
}

func (s *Server) recognizePDFPage(w http.ResponseWriter, r *http.Request) {
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	var format, status string
	if err := s.db.QueryRowContext(r.Context(), `SELECT format, status FROM books WHERE id = ?`, bookID).Scan(&format, &status); err != nil || status != "ready" {
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
		return
	}
	if format != "pdf" {
		writeError(w, http.StatusUnprocessableEntity, "unsupported_format", "OCR is available only for PDF files.")
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || (mediaType != "image/png" && mediaType != "image/jpeg" && mediaType != "image/webp") {
		writeError(w, http.StatusUnsupportedMediaType, "invalid_ocr_image", "Send the page as PNG, JPEG, or WebP.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxOCRImageBytes)
	image, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "ocr_image_too_large", "The image sent to OCR exceeds the allowed limit.")
		return
	}
	language := strings.TrimSpace(r.URL.Query().Get("language"))
	if s.ocr == nil {
		writeError(w, http.StatusServiceUnavailable, "ocr_unavailable", "Tesseract OCR is not available in this installation.")
		return
	}
	result, err := s.ocr.Recognize(r.Context(), image, language)
	switch {
	case errors.Is(err, ocr.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, "ocr_unavailable", "Tesseract OCR is not available in this installation.")
	case errors.Is(err, ocr.ErrUnsupportedLanguage):
		writeError(w, http.StatusUnprocessableEntity, "ocr_language_unavailable", "The requested OCR language is not installed.")
	case errors.Is(err, ocr.ErrInvalidImage):
		writeError(w, http.StatusUnprocessableEntity, "invalid_ocr_image", "The image sent to OCR is invalid.")
	case err != nil:
		s.logger.Error("recognize pdf page", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "ocr_failed", "Could not recognize text on this page.")
	default:
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, result)
	}
}
