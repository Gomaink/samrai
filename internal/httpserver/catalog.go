package httpserver

import (
	"errors"
	"net/http"

	"samrai/internal/auth"
	"samrai/internal/catalog"
)

type seriesResponse struct {
	Items []catalog.Series `json:"items"`
	Total int              `json:"total"`
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	result, err := s.catalog.Dashboard(r.Context(), principal.User.ID)
	if err != nil {
		s.logger.Error("load dashboard", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not load the dashboard.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) listSeries(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	result, err := s.catalog.ListSeries(r.Context(), principal.User.ID, catalog.SeriesListOptions{
		Search:       r.URL.Query().Get("search"),
		Sort:         r.URL.Query().Get("sort"),
		Limit:        queryInt(r, "limit", 100),
		Offset:       queryInt(r, "offset", 0),
		FavoriteOnly: r.URL.Query().Get("favorite") == "1" || r.URL.Query().Get("favorite") == "true",
	})
	if err != nil {
		s.logger.Error("list series", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not load series.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, seriesResponse{Items: result.Items, Total: result.Total})
}

func (s *Server) getSeries(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	seriesID, ok := parsePositivePathID(w, r, "seriesID", "invalid_series_id", "Invalid series ID.")
	if !ok {
		return
	}
	result, err := s.catalog.GetSeries(r.Context(), principal.User.ID, seriesID)
	if errors.Is(err, catalog.ErrSeriesNotFound) {
		writeError(w, http.StatusNotFound, "series_not_found", "Series not found.")
		return
	}
	if err != nil {
		s.logger.Error("get series", "series_id", seriesID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not open the series.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}
