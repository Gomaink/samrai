package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func ValidateRole(role string) error {
	switch role {
	case "admin", "reader":
		return nil
	default:
		return ErrInvalidRole
	}
}

func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, username, role, disabled, created_at, updated_at FROM users ORDER BY username COLLATE NOCASE, id`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	users := make([]User, 0)
	for rows.Next() {
		var user User
		var disabled int
		var createdRaw, updatedRaw string
		if err := rows.Scan(&user.ID, &user.Username, &user.Role, &disabled, &createdRaw, &updatedRaw); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		user.Disabled = disabled == 1
		user.CreatedAt, err = parseDatabaseTime(createdRaw)
		if err != nil {
			return nil, err
		}
		user.UpdatedAt, err = parseDatabaseTime(updatedRaw)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}

func (s *Service) CreateUser(ctx context.Context, actorID int64, username, password, role string) (User, error) {
	username = strings.TrimSpace(username)
	role = strings.ToLower(strings.TrimSpace(role))
	if err := ValidateUsername(username); err != nil {
		return User{}, err
	}
	if err := ValidatePassword(password); err != nil {
		return User{}, err
	}
	if err := ValidateRole(role); err != nil {
		return User{}, err
	}
	passwordHash, err := HashPassword(password, s.passwordParams, s.random)
	if err != nil {
		return User{}, err
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, fmt.Errorf("begin create user transaction: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO users(username, password_hash, role, disabled, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)`, username, passwordHash, role, formatDatabaseTime(now), formatDatabaseTime(now))
	if err != nil {
		if isUniqueUsernameError(err) {
			return User{}, ErrUsernameTaken
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		return User{}, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO user_preferences(user_id) VALUES (?)", userID); err != nil {
		return User{}, err
	}
	if err := writeAudit(ctx, tx, actorID, "user.created", "user", fmt.Sprint(userID), role); err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return User{ID: userID, Username: username, Role: role, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Service) UpdateUser(ctx context.Context, actorID, userID int64, role *string, disabled *bool) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	user, err := loadUserForUpdate(ctx, tx, userID)
	if err != nil {
		return User{}, err
	}
	newRole := user.Role
	if role != nil {
		newRole = strings.ToLower(strings.TrimSpace(*role))
		if err := ValidateRole(newRole); err != nil {
			return User{}, err
		}
	}
	newDisabled := user.Disabled
	if disabled != nil {
		newDisabled = *disabled
	}
	if actorID == userID && (newRole != "admin" || newDisabled) {
		return User{}, ErrCannotModifySelf
	}
	if user.Role == "admin" && !user.Disabled && (newRole != "admin" || newDisabled) {
		if err := ensureAnotherEnabledAdmin(ctx, tx, userID); err != nil {
			return User{}, err
		}
	}
	now := s.now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE users SET role = ?, disabled = ?, updated_at = ? WHERE id = ?`, newRole, boolToInt(newDisabled), formatDatabaseTime(now), userID); err != nil {
		return User{}, err
	}
	if newDisabled {
		if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID); err != nil {
			return User{}, err
		}
	}
	if err := writeAudit(ctx, tx, actorID, "user.updated", "user", fmt.Sprint(userID), fmt.Sprintf("role=%s disabled=%t", newRole, newDisabled)); err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	user.Role, user.Disabled, user.UpdatedAt = newRole, newDisabled, now
	return user, nil
}

func (s *Service) ResetPassword(ctx context.Context, actorID, userID int64, password string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	passwordHash, err := HashPassword(password, s.passwordParams, s.random)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?", passwordHash, formatDatabaseTime(now), userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrUserNotFound
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID); err != nil {
		return err
	}
	if err := writeAudit(ctx, tx, actorID, "user.password_reset", "user", fmt.Sprint(userID), ""); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) error {
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}
	var passwordHash string
	err := s.db.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE id = ? AND disabled = 0", userID).Scan(&passwordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}
	valid, err := VerifyPassword(currentPassword, passwordHash)
	if err != nil {
		return err
	}
	if !valid {
		return ErrInvalidCredentials
	}
	return s.ResetPassword(ctx, userID, userID, newPassword)
}

func (s *Service) DeleteUser(ctx context.Context, actorID, userID int64) error {
	if actorID == userID {
		return ErrCannotModifySelf
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	user, err := loadUserForUpdate(ctx, tx, userID)
	if err != nil {
		return err
	}
	if user.Role == "admin" && !user.Disabled {
		if err := ensureAnotherEnabledAdmin(ctx, tx, userID); err != nil {
			return err
		}
	}
	if err := writeAudit(ctx, tx, actorID, "user.deleted", "user", fmt.Sprint(userID), user.Username); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM users WHERE id = ?", userID); err != nil {
		return err
	}
	return tx.Commit()
}

func loadUserForUpdate(ctx context.Context, tx *sql.Tx, userID int64) (User, error) {
	var user User
	var disabled int
	var createdRaw, updatedRaw string
	err := tx.QueryRowContext(ctx, `SELECT id, username, role, disabled, created_at, updated_at FROM users WHERE id = ?`, userID).Scan(&user.ID, &user.Username, &user.Role, &disabled, &createdRaw, &updatedRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, err
	}
	user.Disabled = disabled == 1
	user.CreatedAt, err = parseDatabaseTime(createdRaw)
	if err != nil {
		return User{}, err
	}
	user.UpdatedAt, err = parseDatabaseTime(updatedRaw)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func ensureAnotherEnabledAdmin(ctx context.Context, tx *sql.Tx, excludedID int64) error {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin' AND disabled = 0 AND id <> ?`, excludedID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return ErrLastAdmin
	}
	return nil
}

func writeAudit(ctx context.Context, tx *sql.Tx, actorID int64, eventType, subjectType, subjectID, detail string) error {
	var actor any
	if actorID > 0 {
		actor = actorID
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_events(actor_user_id, event_type, subject_type, subject_id, detail) VALUES (?, ?, ?, ?, ?)`, actor, eventType, subjectType, subjectID, detail)
	return err
}
func isUniqueUsernameError(err error) bool {
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "unique constraint failed") && strings.Contains(m, "users.username")
}
func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
