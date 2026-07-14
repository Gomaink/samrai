package httpserver

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"samrai/internal/catalog"
)

type epubSearchResponse struct {
	Items []catalog.EPUBSearchResult `json:"items"`
}

func (s *Server) getEPUBPublication(w http.ResponseWriter, r *http.Request) {
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	publication, err := s.catalog.GetEPUBPublication(r.Context(), bookID)
	switch {
	case errors.Is(err, catalog.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
	case errors.Is(err, catalog.ErrUnsupportedFormat):
		writeError(w, http.StatusUnprocessableEntity, "unsupported_format", "This book is not an EPUB.")
	case errors.Is(err, catalog.ErrInvalidBook):
		writeError(w, http.StatusUnprocessableEntity, "invalid_epub", "The EPUB does not contain a valid reading order.")
	case err != nil:
		s.logger.Error("get epub publication", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not prepare the EPUB.")
	default:
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, publication)
	}
}

func (s *Server) epubResource(w http.ResponseWriter, r *http.Request) {
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	resourcePath := strings.TrimSpace(r.PathValue("resource"))
	asset, err := s.catalog.OpenEPUBResource(r.Context(), bookID, resourcePath)
	switch {
	case errors.Is(err, catalog.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
		return
	case errors.Is(err, catalog.ErrPageNotFound):
		writeError(w, http.StatusNotFound, "resource_not_found", "EPUB resource not found.")
		return
	case errors.Is(err, catalog.ErrUnsupportedFormat):
		writeError(w, http.StatusUnprocessableEntity, "unsupported_format", "This book is not an EPUB.")
		return
	case err != nil:
		s.logger.Error("open epub resource", "book_id", bookID, "resource", resourcePath, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not open this EPUB resource.")
		return
	}
	defer asset.Stream.Close()

	// Set the representation and framing headers before evaluating ETag. The
	// reader prefetches the next spine item. A later iframe navigation will
	// commonly revalidate that cached XHTML and receive 304. If the EPUB
	// headers are only added after the ETag check, the global DENY /
	// frame-ancestors 'none' policy leaks into the 304 response and the browser
	// silently blocks the chapter inside the reader.
	setEPUBResourceHeaders(w, asset)
	if r.Header.Get("If-None-Match") == asset.ETag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Length", strconv.FormatInt(asset.Size, 10))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	if _, err := io.Copy(w, asset.Stream); err != nil && !strings.Contains(strings.ToLower(err.Error()), "broken pipe") {
		s.logger.Warn("stream epub resource", "book_id", bookID, "resource", resourcePath, "error", err)
	}
}

func setEPUBResourceHeaders(w http.ResponseWriter, asset catalog.EPUBResourceAsset) {
	w.Header().Set("Content-Type", asset.MediaType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, strings.ReplaceAll(resourceFilename(asset.ResourcePath), `"`, "")))
	w.Header().Set("ETag", asset.ETag)
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	if isEPUBDocumentMediaType(asset.MediaType) {
		setEPUBDocumentHeaders(w)
	} else {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	}
}

func (s *Server) searchEPUB(w http.ResponseWriter, r *http.Request) {
	bookID, ok := parsePositivePathID(w, r, "bookID", "invalid_book_id", "Invalid book ID.")
	if !ok {
		return
	}
	items, err := s.catalog.SearchEPUB(r.Context(), bookID, strings.TrimSpace(r.URL.Query().Get("q")), queryInt(r, "limit", 50))
	switch {
	case errors.Is(err, catalog.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book_not_found", "Book not found.")
	case errors.Is(err, catalog.ErrUnsupportedFormat):
		writeError(w, http.StatusUnprocessableEntity, "unsupported_format", "EPUB search is available only for EPUB files.")
	case errors.Is(err, catalog.ErrInvalidBook):
		writeError(w, http.StatusBadRequest, "invalid_search", "Enter at least two characters to search.")
	case err != nil:
		s.logger.Error("search epub", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not search this EPUB.")
	default:
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, epubSearchResponse{Items: items})
	}
}

func setEPUBDocumentHeaders(w http.ResponseWriter) {
	// EPUB content documents are intentionally rendered inside the samrai
	// same-origin sandboxed iframe. The global application headers deny all
	// framing, so this endpoint must explicitly narrow that policy to the
	// current origin. Scripts and external resources remain blocked.
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; font-src 'self' data:; media-src 'self' data:; script-src 'none'; connect-src 'none'; frame-src 'none'; frame-ancestors 'self'; object-src 'none'; form-action 'none'; base-uri 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	// XHTML/SVG viewer responses are cheap to revalidate and their security
	// headers can change between samrai releases. Do not pin them in the
	// browser for a year like immutable images, styles and fonts.
	w.Header().Set("Cache-Control", "private, no-cache")
}

func isEPUBDocumentMediaType(mediaType string) bool {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "application/xhtml+xml", "text/html", "image/svg+xml":
		return true
	default:
		return false
	}
}

func resourceFilename(resourcePath string) string {
	parts := strings.Split(strings.TrimSpace(resourcePath), "/")
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return "resource"
	}
	return parts[len(parts)-1]
}
