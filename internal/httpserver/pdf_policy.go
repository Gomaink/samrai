package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"samrai/internal/catalog"
)

type pdfOCRModeRequest struct {
	Mode string `json:"mode"`
}

func (s *Server) savePDFClassification(w http.ResponseWriter, r *http.Request) {
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	var request catalog.PDFClassificationInput
	if err := decodeLargeJSON(w, r, &request, maxPDFAnalysisBodyBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_pdf_classification", "The provided classification is invalid.")
		return
	}
	err := s.catalog.SavePDFClassification(r.Context(), bookID, request)
	switch {
	case errors.Is(err, catalog.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
	case errors.Is(err, catalog.ErrUnsupportedFormat):
		writeError(w, http.StatusUnprocessableEntity, "unsupported_format", "This book is not a PDF.")
	case errors.Is(err, catalog.ErrInvalidBook):
		writeError(w, http.StatusUnprocessableEntity, "invalid_pdf_classification", "The provided classification is invalid.")
	case err != nil:
		s.logger.Error("save pdf classification", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not classify the PDF.")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) updatePDFOCRMode(w http.ResponseWriter, r *http.Request) {
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	var request pdfOCRModeRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_ocr_mode", "The provided OCR mode is invalid.")
		return
	}
	err := s.catalog.UpdatePDFOCRMode(r.Context(), bookID, strings.TrimSpace(request.Mode))
	switch {
	case errors.Is(err, catalog.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
	case errors.Is(err, catalog.ErrUnsupportedFormat):
		writeError(w, http.StatusUnprocessableEntity, "unsupported_format", "This book is not a PDF.")
	case errors.Is(err, catalog.ErrInvalidBook):
		writeError(w, http.StatusUnprocessableEntity, "invalid_ocr_mode", "The provided OCR mode is invalid.")
	case err != nil:
		s.logger.Error("update pdf ocr mode", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not change the OCR mode.")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) savePDFPageText(w http.ResponseWriter, r *http.Request) {
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	pageNumber, ok := parseNonNegativePathID(w, r, "pageNumber", "invalid_page_number", "Invalid page number.")
	if !ok {
		return
	}
	var request catalog.PDFPageTextInput
	if err := decodeLargeJSON(w, r, &request, (2<<20)+4096); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_pdf_page_text", "The recognized text is invalid.")
		return
	}
	request.PageNumber = pageNumber
	err := s.catalog.SavePDFPageText(r.Context(), bookID, request)
	switch {
	case errors.Is(err, catalog.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
	case errors.Is(err, catalog.ErrPageOutOfRange):
		writeError(w, http.StatusNotFound, "page_not_found", "Page not found.")
	case errors.Is(err, catalog.ErrUnsupportedFormat):
		writeError(w, http.StatusUnprocessableEntity, "unsupported_format", "This book is not a PDF.")
	case errors.Is(err, catalog.ErrInvalidBook):
		writeError(w, http.StatusUnprocessableEntity, "invalid_pdf_page_text", "The recognized text is invalid.")
	case err != nil:
		s.logger.Error("save pdf page text", "book_id", bookID, "page", pageNumber, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not save recognized text.")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func parseNonNegativePathID(w http.ResponseWriter, r *http.Request, name, code, message string) (int, bool) {
	value, err := strconv.Atoi(r.PathValue(name))
	if err != nil || value < 0 {
		writeError(w, http.StatusBadRequest, code, message)
		return 0, false
	}
	return value, true
}
