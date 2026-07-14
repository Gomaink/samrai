package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrSetupComplete      = errors.New("initial setup is already complete")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidSession     = errors.New("invalid session")
	ErrInvalidUsername    = errors.New("username must contain 3 to 32 letters, numbers, dots, underscores or hyphens")
	ErrInvalidPassword    = errors.New("password must contain 10 to 128 characters")
	ErrUserNotFound       = errors.New("user not found")
	ErrUsernameTaken      = errors.New("username is already in use")
	ErrInvalidRole        = errors.New("role must be admin or reader")
	ErrLastAdmin          = errors.New("the last enabled administrator cannot be changed or removed")
	ErrCannotModifySelf   = errors.New("you cannot disable, demote or delete your own account")
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{2,31}$`)

const (
	maxSessionsPerUser = 20
	sessionTouchAfter  = 15 * time.Minute
)

type User struct {
	ID        int64
	Username  string
	Role      string
	Disabled  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Principal struct {
	User      User
	SessionID string
	ExpiresAt time.Time
}

type AuthResult struct {
	User      User
	Token     string
	ExpiresAt time.Time
}

type Options struct {
	SessionDuration time.Duration
	PasswordParams  PasswordParams
	Random          io.Reader
	Now             func() time.Time
}

type Service struct {
	db              *sql.DB
	sessionDuration time.Duration
	passwordParams  PasswordParams
	random          io.Reader
	now             func() time.Time
	dummyHash       string
}

func NewService(db *sql.DB, options Options) (*Service, error) {
	if db == nil {
		return nil, errors.New("auth database is required")
	}
	if options.SessionDuration <= 0 {
		return nil, errors.New("session duration must be greater than zero")
	}
	if err := options.PasswordParams.Validate(); err != nil {
		return nil, err
	}
	if options.Random == nil {
		options.Random = rand.Reader
	}
	if options.Now == nil {
		options.Now = time.Now
	}

	dummyHash, err := HashPassword("samrai-dummy-password", options.PasswordParams, options.Random)
	if err != nil {
		return nil, fmt.Errorf("create dummy password hash: %w", err)
	}

	return &Service{
		db:              db,
		sessionDuration: options.SessionDuration,
		passwordParams:  options.PasswordParams,
		random:          options.Random,
		now:             options.Now,
		dummyHash:       dummyHash,
	}, nil
}

func ValidateUsername(username string) error {
	if !usernamePattern.MatchString(strings.TrimSpace(username)) {
		return ErrInvalidUsername
	}
	return nil
}

func ValidatePassword(password string) error {
	length := utf8.RuneCountInString(password)
	if length < 10 || length > 128 || strings.ContainsRune(password, '\x00') {
		return ErrInvalidPassword
	}
	return nil
}

func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users)").Scan(&exists); err != nil {
		return false, fmt.Errorf("check initial setup: %w", err)
	}
	return exists == 0, nil
}

func (s *Service) Setup(ctx context.Context, username, password string) (AuthResult, error) {
	required, err := s.SetupRequired(ctx)
	if err != nil {
		return AuthResult{}, err
	}
	if !required {
		return AuthResult{}, ErrSetupComplete
	}

	username = strings.TrimSpace(username)
	if err := ValidateUsername(username); err != nil {
		return AuthResult{}, err
	}
	if err := ValidatePassword(password); err != nil {
		return AuthResult{}, err
	}

	passwordHash, err := HashPassword(password, s.passwordParams, s.random)
	if err != nil {
		return AuthResult{}, err
	}
	now := s.now().UTC()
	session, err := s.newSession(now)
	if err != nil {
		return AuthResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AuthResult{}, fmt.Errorf("begin setup transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO users(username, password_hash, role)
		SELECT ?, ?, 'admin'
		WHERE NOT EXISTS (SELECT 1 FROM users)
	`, username, passwordHash)
	if err != nil {
		return AuthResult{}, fmt.Errorf("create initial administrator: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return AuthResult{}, fmt.Errorf("inspect setup result: %w", err)
	}
	if rows == 0 {
		return AuthResult{}, ErrSetupComplete
	}

	userID, err := result.LastInsertId()
	if err != nil {
		return AuthResult{}, fmt.Errorf("read administrator id: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO user_preferences(user_id) VALUES (?)", userID); err != nil {
		return AuthResult{}, fmt.Errorf("create administrator preferences: %w", err)
	}
	if err := insertSession(ctx, tx, userID, session); err != nil {
		return AuthResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AuthResult{}, fmt.Errorf("commit setup transaction: %w", err)
	}

	user := User{ID: userID, Username: username, Role: "admin", CreatedAt: now, UpdatedAt: now}
	return AuthResult{User: user, Token: session.token, ExpiresAt: session.expiresAt}, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (AuthResult, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		_, _ = VerifyPassword(password, s.dummyHash)
		return AuthResult{}, ErrInvalidCredentials
	}

	var (
		user         User
		passwordHash string
		createdRaw   string
		updatedRaw   string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, role, disabled, created_at, updated_at
		FROM users
		WHERE username = ?
	`, username).Scan(&user.ID, &user.Username, &passwordHash, &user.Role, &user.Disabled, &createdRaw, &updatedRaw)
	if errors.Is(err, sql.ErrNoRows) {
		_, _ = VerifyPassword(password, s.dummyHash)
		return AuthResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return AuthResult{}, fmt.Errorf("find user for login: %w", err)
	}
	if user.Disabled {
		_, _ = VerifyPassword(password, s.dummyHash)
		return AuthResult{}, ErrInvalidCredentials
	}

	valid, err := VerifyPassword(password, passwordHash)
	if err != nil {
		return AuthResult{}, fmt.Errorf("verify stored password hash: %w", err)
	}
	if !valid {
		return AuthResult{}, ErrInvalidCredentials
	}
	user.CreatedAt, err = parseDatabaseTime(createdRaw)
	if err != nil {
		return AuthResult{}, fmt.Errorf("parse user creation time: %w", err)
	}
	user.UpdatedAt, err = parseDatabaseTime(updatedRaw)
	if err != nil {
		return AuthResult{}, fmt.Errorf("parse user update time: %w", err)
	}

	now := s.now().UTC()
	session, err := s.newSession(now)
	if err != nil {
		return AuthResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AuthResult{}, fmt.Errorf("begin login transaction: %w", err)
	}
	defer tx.Rollback()

	if PasswordHashNeedsUpgrade(passwordHash, s.passwordParams) {
		upgradedHash, err := HashPassword(password, s.passwordParams, s.random)
		if err != nil {
			return AuthResult{}, err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?", upgradedHash, formatDatabaseTime(now), user.ID); err != nil {
			return AuthResult{}, fmt.Errorf("upgrade password hash: %w", err)
		}
		user.UpdatedAt = now
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", formatDatabaseTime(now)); err != nil {
		return AuthResult{}, fmt.Errorf("delete expired sessions: %w", err)
	}
	if err := insertSession(ctx, tx, user.ID, session); err != nil {
		return AuthResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM sessions
		WHERE user_id = ?
		  AND id NOT IN (
			SELECT id FROM sessions WHERE user_id = ? ORDER BY created_at DESC LIMIT ?
		  )
	`, user.ID, user.ID, maxSessionsPerUser); err != nil {
		return AuthResult{}, fmt.Errorf("trim old sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return AuthResult{}, fmt.Errorf("commit login transaction: %w", err)
	}

	return AuthResult{User: user, Token: session.token, ExpiresAt: session.expiresAt}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	if token == "" {
		return Principal{}, ErrInvalidSession
	}

	tokenHash := hashToken(token)
	var (
		principal   Principal
		expiresRaw  string
		lastUsedRaw string
		createdRaw  string
		updatedRaw  string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT s.id, s.expires_at, s.last_used_at,
		       u.id, u.username, u.role, u.disabled, u.created_at, u.updated_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ?
	`, tokenHash).Scan(
		&principal.SessionID,
		&expiresRaw,
		&lastUsedRaw,
		&principal.User.ID,
		&principal.User.Username,
		&principal.User.Role,
		&principal.User.Disabled,
		&createdRaw,
		&updatedRaw,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, ErrInvalidSession
	}
	if err != nil {
		return Principal{}, fmt.Errorf("load session: %w", err)
	}

	principal.ExpiresAt, err = parseDatabaseTime(expiresRaw)
	if err != nil {
		return Principal{}, fmt.Errorf("parse session expiry: %w", err)
	}
	lastUsed, err := parseDatabaseTime(lastUsedRaw)
	if err != nil {
		return Principal{}, fmt.Errorf("parse session activity: %w", err)
	}
	principal.User.CreatedAt, err = parseDatabaseTime(createdRaw)
	if err != nil {
		return Principal{}, fmt.Errorf("parse user creation time: %w", err)
	}
	principal.User.UpdatedAt, err = parseDatabaseTime(updatedRaw)
	if err != nil {
		return Principal{}, fmt.Errorf("parse user update time: %w", err)
	}

	now := s.now().UTC()
	if principal.User.Disabled {
		_, _ = s.db.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", principal.SessionID)
		return Principal{}, ErrInvalidSession
	}
	if !principal.ExpiresAt.After(now) {
		_, _ = s.db.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", principal.SessionID)
		return Principal{}, ErrInvalidSession
	}
	if now.Sub(lastUsed) >= sessionTouchAfter {
		_, _ = s.db.ExecContext(ctx, "UPDATE sessions SET last_used_at = ? WHERE id = ?", formatDatabaseTime(now), principal.SessionID)
	}

	return principal, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ?", hashToken(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

type pendingSession struct {
	id        string
	token     string
	tokenHash string
	expiresAt time.Time
	createdAt time.Time
}

func (s *Service) newSession(now time.Time) (pendingSession, error) {
	idBytes := make([]byte, 16)
	if _, err := io.ReadFull(s.random, idBytes); err != nil {
		return pendingSession{}, fmt.Errorf("generate session id: %w", err)
	}
	tokenBytes := make([]byte, 32)
	if _, err := io.ReadFull(s.random, tokenBytes); err != nil {
		return pendingSession{}, fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	return pendingSession{
		id:        hex.EncodeToString(idBytes),
		token:     token,
		tokenHash: hashToken(token),
		expiresAt: now.Add(s.sessionDuration),
		createdAt: now,
	}, nil
}

func insertSession(ctx context.Context, tx *sql.Tx, userID int64, session pendingSession) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sessions(id, user_id, token_hash, expires_at, created_at, last_used_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		session.id,
		userID,
		session.tokenHash,
		formatDatabaseTime(session.expiresAt),
		formatDatabaseTime(session.createdAt),
		formatDatabaseTime(session.createdAt),
	); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func formatDatabaseTime(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

func parseDatabaseTime(value string) (time.Time, error) {
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
	} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported database timestamp %q", value)
}
