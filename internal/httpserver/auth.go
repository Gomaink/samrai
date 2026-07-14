package httpserver

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"samrai/internal/auth"
)

const (
	sessionCookieName       = "samrai_session"
	legacySessionCookieName = "pageturner_session"
)

type credentialsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userResponse struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	Disabled  bool   `json:"disabled"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type authResponse struct {
	User userResponse `json:"user"`
}

type authStatusResponse struct {
	SetupRequired bool             `json:"setup_required"`
	Authenticated bool             `json:"authenticated"`
	User          *userResponse    `json:"user,omitempty"`
	Instance      instanceResponse `json:"instance"`
}

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	setupRequired, err := s.auth.SetupRequired(r.Context())
	if err != nil {
		s.logger.Error("check authentication status", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not read authentication status.")
		return
	}

	instance, err := s.settings.Get(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not read instance settings.")
		return
	}
	response := authStatusResponse{SetupRequired: setupRequired, Instance: makeInstanceResponse(instance)}
	if cookie, legacy, err := requestSessionCookie(r); err == nil {
		principal, err := s.auth.Authenticate(r.Context(), cookie.Value)
		if err == nil {
			user := makeUserResponse(principal.User)
			response.Authenticated = true
			response.User = &user
			if legacy {
				s.setSessionCookie(w, cookie.Value, principal.ExpiresAt)
				s.clearNamedSessionCookie(w, legacySessionCookieName)
			}
		} else if errors.Is(err, auth.ErrInvalidSession) {
			s.clearSessionCookie(w)
		} else {
			s.logger.Error("authenticate status request", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Could not read the session.")
			return
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var request credentialsRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	result, err := s.auth.Setup(r.Context(), request.Username, request.Password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrSetupComplete):
			writeError(w, http.StatusConflict, "setup_complete", "Initial setup has already been completed.")
		case errors.Is(err, auth.ErrInvalidUsername), errors.Is(err, auth.ErrInvalidPassword):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		default:
			s.logger.Error("create initial administrator", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Could not create the administrator account.")
		}
		return
	}

	s.setSessionCookie(w, result.Token, result.ExpiresAt)
	writeJSON(w, http.StatusCreated, authResponse{User: makeUserResponse(result.User)})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var request credentialsRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	key := loginLimitKey(r, request.Username)
	if retry := s.loginLimiter.retryAfter(key); retry > 0 {
		seconds := int(retry.Round(time.Second).Seconds())
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		writeError(w, http.StatusTooManyRequests, "too_many_attempts", "Too many sign-in attempts. Try again in a few moments.")
		return
	}

	result, err := s.auth.Login(r.Context(), request.Username, request.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			block := s.loginLimiter.recordFailure(key)
			if block > 0 {
				seconds := int(block.Round(time.Second).Seconds())
				if seconds < 1 {
					seconds = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(seconds))
			}
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid username or password.")
			return
		}
		s.logger.Error("login failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not sign in.")
		return
	}

	s.loginLimiter.reset(key)
	s.setSessionCookie(w, result.Token, result.ExpiresAt)
	writeJSON(w, http.StatusOK, authResponse{User: makeUserResponse(result.User)})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
		return
	}
	writeJSON(w, http.StatusOK, authResponse{User: makeUserResponse(principal.User)})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	cookie, _, err := requestSessionCookie(r)
	if err == nil {
		if err := s.auth.Logout(r.Context(), cookie.Value); err != nil {
			s.logger.Error("logout failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Could not end the session.")
			return
		}
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) requireAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, legacy, err := requestSessionCookie(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
			return
		}

		principal, err := s.auth.Authenticate(r.Context(), cookie.Value)
		if errors.Is(err, auth.ErrInvalidSession) {
			s.clearSessionCookie(w)
			writeError(w, http.StatusUnauthorized, "authentication_required", "The session is invalid or has expired.")
			return
		}
		if err != nil {
			s.logger.Error("authenticate request", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Could not validate the session.")
			return
		}

		if legacy {
			s.setSessionCookie(w, cookie.Value, principal.ExpiresAt)
			s.clearNamedSessionCookie(w, legacySessionCookieName)
		}

		next.ServeHTTP(w, r.WithContext(auth.ContextWithPrincipal(r.Context(), principal)))
	})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func requestSessionCookie(r *http.Request) (*http.Cookie, bool, error) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		return cookie, false, nil
	}
	cookie, err := r.Cookie(legacySessionCookieName)
	return cookie, err == nil, err
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	s.clearNamedSessionCookie(w, sessionCookieName)
	s.clearNamedSessionCookie(w, legacySessionCookieName)
}

func (s *Server) clearNamedSessionCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(1, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func makeUserResponse(user auth.User) userResponse {
	return userResponse{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		Disabled:  user.Disabled,
		CreatedAt: user.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: user.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func loginLimitKey(r *http.Request, username string) string {
	host := r.RemoteAddr
	if parsed, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = parsed
	}
	return fmt.Sprintf("%s|%s", host, strings.ToLower(strings.TrimSpace(username)))
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required.")
			return
		}
		if principal.User.Role != "admin" {
			writeError(w, http.StatusForbidden, "admin_required", "Only administrators can perform this action.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
