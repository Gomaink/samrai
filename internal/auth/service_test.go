package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"samrai/internal/database"
)

func newTestService(t *testing.T, now func() time.Time) (*Service, func()) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "auth.db"), 1)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := database.Migrate(ctx, db); err != nil {
		db.Close()
		t.Fatalf("migrate database: %v", err)
	}

	service, err := NewService(db, Options{
		SessionDuration: time.Hour,
		PasswordParams:  testPasswordParams(),
		Now:             now,
	})
	if err != nil {
		db.Close()
		t.Fatalf("NewService() error = %v", err)
	}
	return service, func() { _ = db.Close() }
}

func TestSetupLoginAndLogout(t *testing.T) {
	now := time.Date(2026, time.July, 12, 12, 0, 0, 0, time.UTC)
	service, closeDB := newTestService(t, func() time.Time { return now })
	defer closeDB()
	ctx := context.Background()

	required, err := service.SetupRequired(ctx)
	if err != nil || !required {
		t.Fatalf("SetupRequired() = %v, %v", required, err)
	}

	setup, err := service.Setup(ctx, "samuel", "a-secure-password")
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if setup.User.Role != "admin" || setup.Token == "" {
		t.Fatalf("unexpected setup result: %+v", setup)
	}

	if _, err := service.Setup(ctx, "other", "another-secure-password"); !errors.Is(err, ErrSetupComplete) {
		t.Fatalf("second Setup() error = %v, want ErrSetupComplete", err)
	}

	principal, err := service.Authenticate(ctx, setup.Token)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal.User.Username != "samuel" {
		t.Fatalf("unexpected principal: %+v", principal)
	}

	if _, err := service.Login(ctx, "samuel", "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong Login() error = %v", err)
	}
	login, err := service.Login(ctx, "SAMUEL", "a-secure-password")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if login.Token == "" {
		t.Fatal("Login() returned empty token")
	}

	if err := service.Logout(ctx, login.Token); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := service.Authenticate(ctx, login.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("Authenticate() after logout error = %v", err)
	}
}

func TestSessionExpires(t *testing.T) {
	now := time.Date(2026, time.July, 12, 12, 0, 0, 0, time.UTC)
	service, closeDB := newTestService(t, func() time.Time { return now })
	defer closeDB()

	result, err := service.Setup(context.Background(), "samuel", "a-secure-password")
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	now = now.Add(2 * time.Hour)
	if _, err := service.Authenticate(context.Background(), result.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expired Authenticate() error = %v", err)
	}
}
