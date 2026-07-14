package settings

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"samrai/internal/database"
)

func TestGetAndUpdateInstanceSettings(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "settings.db"), 1)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	result, err := db.ExecContext(ctx, `INSERT INTO users(username, password_hash, role) VALUES ('admin', 'test-only', 'admin')`)
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}
	actorID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("actor id: %v", err)
	}

	service := NewService(db)
	initial, err := service.Get(ctx)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if initial.Name != "samrai" || initial.DefaultReadingDirection != "ltr" {
		t.Fatalf("unexpected initial settings: %+v", initial)
	}

	updated, err := service.Update(ctx, actorID, Instance{Name: " My Library ", DefaultReadingDirection: "RTL"})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != "My Library" || updated.DefaultReadingDirection != "rtl" {
		t.Fatalf("unexpected updated settings: %+v", updated)
	}

	if _, err := service.Update(ctx, actorID, Instance{Name: "", DefaultReadingDirection: "ltr"}); !errors.Is(err, ErrInvalidInstanceName) {
		t.Fatalf("empty name error = %v", err)
	}
	if _, err := service.Update(ctx, actorID, Instance{Name: "Library", DefaultReadingDirection: "vertical"}); !errors.Is(err, ErrInvalidReadingDirection) {
		t.Fatalf("invalid direction error = %v", err)
	}
}
