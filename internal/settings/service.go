package settings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var ErrInvalidInstanceName = errors.New("instance name must contain 1 to 50 characters")
var ErrInvalidReadingDirection = errors.New("default reading direction must be ltr or rtl")

type Instance struct {
	Name                    string `json:"name"`
	DefaultReadingDirection string `json:"default_reading_direction"`
}
type Service struct{ db *sql.DB }

func NewService(db *sql.DB) *Service { return &Service{db: db} }
func (s *Service) Get(ctx context.Context) (Instance, error) {
	result := Instance{Name: "samrai", DefaultReadingDirection: "ltr"}
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM instance_settings WHERE key IN ('instance_name','default_reading_direction')`)
	if err != nil {
		return Instance{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return Instance{}, err
		}
		if k == "instance_name" {
			result.Name = v
		} else if k == "default_reading_direction" {
			result.DefaultReadingDirection = v
		}
	}
	return result, rows.Err()
}
func (s *Service) Update(ctx context.Context, actorID int64, input Instance) (Instance, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 50 || strings.ContainsRune(input.Name, '\x00') {
		return Instance{}, ErrInvalidInstanceName
	}
	input.DefaultReadingDirection = strings.ToLower(strings.TrimSpace(input.DefaultReadingDirection))
	if input.DefaultReadingDirection != "ltr" && input.DefaultReadingDirection != "rtl" {
		return Instance{}, ErrInvalidReadingDirection
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Instance{}, err
	}
	defer tx.Rollback()
	for k, v := range map[string]string{"instance_name": input.Name, "default_reading_direction": input.DefaultReadingDirection} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO instance_settings(key,value,updated_at) VALUES(?,?,strftime('%Y-%m-%dT%H:%M:%fZ','now')) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`, k, v); err != nil {
			return Instance{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(actor_user_id,event_type,subject_type,subject_id,detail) VALUES(?,'settings.updated','instance','global',?)`, actorID, fmt.Sprintf("name=%s direction=%s", input.Name, input.DefaultReadingDirection)); err != nil {
		return Instance{}, err
	}
	if err := tx.Commit(); err != nil {
		return Instance{}, err
	}
	return input, nil
}
