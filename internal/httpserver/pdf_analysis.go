package httpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"samrai/internal/catalog"
)

const maxPDFAnalysisBodyBytes = 36 << 20
const maxPDFCoverBodyBytes = 6 << 20

type pdfSearchResponse struct {
	Items []catalog.PDFSearchResult `json:"items"`
}

func (s *Server) savePDFAnalysis(w http.ResponseWriter, r *http.Request) {
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	var request catalog.PDFAnalysisInput
	if err := decodeLargeJSON(w, r, &request, maxPDFAnalysisBodyBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_pdf_analysis", "The PDF analysis data is invalid or exceeds the allowed limit.")
		return
	}
	err := s.catalog.SavePDFAnalysis(r.Context(), bookID, request)
	switch {
	case errors.Is(err, catalog.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
	case errors.Is(err, catalog.ErrUnsupportedFormat):
		writeError(w, http.StatusUnprocessableEntity, "unsupported_format", "This book is not a PDF.")
	case errors.Is(err, catalog.ErrInvalidBook):
		writeError(w, http.StatusUnprocessableEntity, "invalid_pdf_analysis", "The extracted PDF data is invalid.")
	case err != nil:
		s.logger.Error("save pdf analysis", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not index the PDF.")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) savePDFCover(w http.ResponseWriter, r *http.Request) {
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || (mediaType != "image/webp" && mediaType != "image/jpeg" && mediaType != "image/png") {
		writeError(w, http.StatusUnsupportedMediaType, "invalid_cover_type", "Send a WebP, JPEG, or PNG cover.")
		return
	}
	width, widthErr := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("width")))
	height, heightErr := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("height")))
	if widthErr != nil || heightErr != nil || width < 1 || height < 1 {
		writeError(w, http.StatusBadRequest, "invalid_cover_dimensions", "Provide valid cover dimensions.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPDFCoverBodyBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cover", "Could not read the cover.")
		return
	}
	if !validImageSignature(mediaType, data) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_cover", "The uploaded content does not match the cover format.")
		return
	}
	err = s.catalog.SavePDFCover(r.Context(), bookID, mediaType, width, height, data)
	switch {
	case errors.Is(err, catalog.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
	case errors.Is(err, catalog.ErrUnsupportedFormat):
		writeError(w, http.StatusUnprocessableEntity, "unsupported_format", "This book is not a PDF.")
	case errors.Is(err, catalog.ErrInvalidBook):
		writeError(w, http.StatusUnprocessableEntity, "invalid_cover", "The provided cover is invalid.")
	case err != nil:
		s.logger.Error("save pdf cover", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not save the PDF cover.")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) searchPDF(w http.ResponseWriter, r *http.Request) {
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := queryInt(r, "limit", 50)
	items, err := s.catalog.SearchPDF(r.Context(), bookID, query, limit)
	switch {
	case errors.Is(err, catalog.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
	case errors.Is(err, catalog.ErrUnsupportedFormat):
		writeError(w, http.StatusUnprocessableEntity, "unsupported_format", "In-book search is available only for PDF files.")
	case errors.Is(err, catalog.ErrInvalidBook):
		writeError(w, http.StatusBadRequest, "invalid_search", "Enter at least two characters to search.")
	case err != nil:
		s.logger.Error("search pdf", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not search this PDF.")
	default:
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, pdfSearchResponse{Items: items})
	}
}

func decodeLargeJSON(w http.ResponseWriter, r *http.Request, destination any, limit int64) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("content type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("json body must contain one object")
	}
	return nil
}

func validImageSignature(mediaType string, data []byte) bool {
	switch mediaType {
	case "image/webp":
		return len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP"))
	case "image/jpeg":
		return len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff
	case "image/png":
		return len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})
	default:
		return false
	}
}
