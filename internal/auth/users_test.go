package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUserManagementLifecycle(t *testing.T) {
	now := time.Date(2026, time.July, 12, 12, 0, 0, 0, time.UTC)
	service, closeDB := newTestService(t, func() time.Time { return now })
	defer closeDB()
	ctx := context.Background()

	admin, err := service.Setup(ctx, "samuel", "a-secure-password")
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	reader, err := service.CreateUser(ctx, admin.User.ID, "reader", "another-secure-password", "reader")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if reader.Role != "reader" || reader.Disabled {
		t.Fatalf("unexpected reader: %+v", reader)
	}

	if _, err := service.CreateUser(ctx, admin.User.ID, "READER", "third-secure-password", "reader"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate CreateUser() error = %v, want ErrUsernameTaken", err)
	}

	disabled := true
	updated, err := service.UpdateUser(ctx, admin.User.ID, reader.ID, nil, &disabled)
	if err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}
	if !updated.Disabled {
		t.Fatal("UpdateUser() did not disable reader")
	}
	if _, err := service.Login(ctx, "reader", "another-secure-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled Login() error = %v, want ErrInvalidCredentials", err)
	}

	readerRole := "reader"
	if _, err := service.UpdateUser(ctx, admin.User.ID, admin.User.ID, &readerRole, nil); !errors.Is(err, ErrCannotModifySelf) {
		t.Fatalf("self-demotion error = %v, want ErrCannotModifySelf", err)
	}
	if err := service.DeleteUser(ctx, admin.User.ID, admin.User.ID); !errors.Is(err, ErrCannotModifySelf) {
		t.Fatalf("self-delete error = %v, want ErrCannotModifySelf", err)
	}

	users, err := service.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("ListUsers() length = %d, want 2", len(users))
	}
}

func TestLastEnabledAdministratorIsProtected(t *testing.T) {
	service, closeDB := newTestService(t, time.Now)
	defer closeDB()
	ctx := context.Background()

	first, err := service.Setup(ctx, "admin1", "a-secure-password")
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	second, err := service.CreateUser(ctx, first.User.ID, "admin2", "another-secure-password", "admin")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	disabled := true
	if _, err := service.UpdateUser(ctx, first.User.ID, second.ID, nil, &disabled); err != nil {
		t.Fatalf("disable second administrator: %v", err)
	}

	// Simulate an operation performed by another administrator account. The
	// service must still refuse to remove the final enabled administrator.
	if err := service.DeleteUser(ctx, second.ID, first.User.ID); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("DeleteUser() error = %v, want ErrLastAdmin", err)
	}
}
