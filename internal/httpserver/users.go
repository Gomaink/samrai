package httpserver

import (
	"errors"
	"net/http"
	"samrai/internal/auth"
	"strings"
)

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}
type updateUserRequest struct {
	Role     *string `json:"role"`
	Disabled *bool   `json:"disabled"`
}
type passwordRequest struct {
	Password string `json:"password"`
}
type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}
type usersResponse struct {
	Items []userResponse `json:"items"`
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.auth.ListUsers(r.Context())
	if err != nil {
		writeError(w, 500, "internal_error", "Could not load users.")
		return
	}
	items := make([]userResponse, 0, len(users))
	for _, u := range users {
		items = append(items, makeUserResponse(u))
	}
	writeJSON(w, 200, usersResponse{items})
}
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	var q createUserRequest
	if err := decodeJSON(w, r, &q); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(q.Role) == "" {
		q.Role = "reader"
	}
	u, err := s.auth.CreateUser(r.Context(), p.User.ID, q.Username, q.Password, q.Role)
	if err != nil {
		writeUserError(w, err)
		return
	}
	writeJSON(w, 201, makeUserResponse(u))
}
func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePositivePathID(w, r, "userID", "invalid_user_id", "Invalid user ID.")
	if !ok {
		return
	}
	p, _ := auth.PrincipalFromContext(r.Context())
	var q updateUserRequest
	if err := decodeJSON(w, r, &q); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	if q.Role == nil && q.Disabled == nil {
		writeError(w, 400, "invalid_request", "Provide a role or account status.")
		return
	}
	u, err := s.auth.UpdateUser(r.Context(), p.User.ID, id, q.Role, q.Disabled)
	if err != nil {
		writeUserError(w, err)
		return
	}
	writeJSON(w, 200, makeUserResponse(u))
}
func (s *Server) resetUserPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePositivePathID(w, r, "userID", "invalid_user_id", "Invalid user ID.")
	if !ok {
		return
	}
	p, _ := auth.PrincipalFromContext(r.Context())
	var q passwordRequest
	if err := decodeJSON(w, r, &q); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	if err := s.auth.ResetPassword(r.Context(), p.User.ID, id, q.Password); err != nil {
		writeUserError(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) changeOwnPassword(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	var q changePasswordRequest
	if err := decodeJSON(w, r, &q); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	if err := s.auth.ChangePassword(r.Context(), p.User.ID, q.CurrentPassword, q.NewPassword); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, 401, "invalid_credentials", "The current password is incorrect.")
			return
		}
		writeUserError(w, err)
		return
	}
	s.clearSessionCookie(w)
	w.WriteHeader(204)
}
func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePositivePathID(w, r, "userID", "invalid_user_id", "Invalid user ID.")
	if !ok {
		return
	}
	p, _ := auth.PrincipalFromContext(r.Context())
	if err := s.auth.DeleteUser(r.Context(), p.User.ID, id); err != nil {
		writeUserError(w, err)
		return
	}
	w.WriteHeader(204)
}
func writeUserError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidUsername), errors.Is(err, auth.ErrInvalidPassword), errors.Is(err, auth.ErrInvalidRole):
		writeError(w, 422, "validation_error", err.Error())
	case errors.Is(err, auth.ErrUsernameTaken):
		writeError(w, 409, "username_taken", "This username is already in use.")
	case errors.Is(err, auth.ErrUserNotFound):
		writeError(w, 404, "user_not_found", "User not found.")
	case errors.Is(err, auth.ErrLastAdmin):
		writeError(w, 409, "last_admin", "At least one active administrator is required.")
	case errors.Is(err, auth.ErrCannotModifySelf):
		writeError(w, 409, "cannot_modify_self", "You cannot disable, demote, or delete your own account.")
	default:
		writeError(w, 500, "internal_error", "Could not update the user.")
	}
}
