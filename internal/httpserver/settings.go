package httpserver

import (
	"errors"
	"net/http"
	"samrai/internal/auth"
	instancesettings "samrai/internal/settings"
)

type instanceResponse struct {
	Name                    string `json:"name"`
	DefaultReadingDirection string `json:"default_reading_direction"`
}

func (s *Server) instanceSettings(w http.ResponseWriter, r *http.Request) {
	v, e := s.settings.Get(r.Context())
	if e != nil {
		writeError(w, 500, "internal_error", "Could not load settings.")
		return
	}
	writeJSON(w, 200, makeInstanceResponse(v))
}
func (s *Server) updateInstanceSettings(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	var q instanceResponse
	if e := decodeJSON(w, r, &q); e != nil {
		writeError(w, 400, "invalid_request", e.Error())
		return
	}
	v, e := s.settings.Update(r.Context(), p.User.ID, instancesettings.Instance{Name: q.Name, DefaultReadingDirection: q.DefaultReadingDirection})
	if e != nil {
		if errors.Is(e, instancesettings.ErrInvalidInstanceName) || errors.Is(e, instancesettings.ErrInvalidReadingDirection) {
			writeError(w, 422, "validation_error", e.Error())
			return
		}
		writeError(w, 500, "internal_error", "Could not save settings.")
		return
	}
	writeJSON(w, 200, makeInstanceResponse(v))
}
func makeInstanceResponse(v instancesettings.Instance) instanceResponse {
	return instanceResponse{Name: v.Name, DefaultReadingDirection: v.DefaultReadingDirection}
}
