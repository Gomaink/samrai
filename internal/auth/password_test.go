package auth

import (
	"errors"
	"strings"
	"testing"
)

func testPasswordParams() PasswordParams {
	return PasswordParams{MemoryKiB: 64, Iterations: 1, Parallelism: 1, SaltLength: 8, KeyLength: 16}
}

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple", testPasswordParams(), strings.NewReader("12345678"))
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	valid, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if !valid {
		t.Fatal("password should be valid")
	}

	valid, err = VerifyPassword("wrong password", hash)
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if valid {
		t.Fatal("wrong password should not be valid")
	}
}

func TestRejectMalformedPasswordHash(t *testing.T) {
	if _, err := VerifyPassword("password", "$argon2id$broken"); !errors.Is(err, ErrInvalidPasswordHash) {
		t.Fatalf("error = %v, want ErrInvalidPasswordHash", err)
	}
}
