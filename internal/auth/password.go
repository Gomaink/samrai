package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

var ErrInvalidPasswordHash = errors.New("invalid password hash")

type PasswordParams struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

func DefaultPasswordParams() PasswordParams {
	return PasswordParams{
		MemoryKiB:   64 * 1024,
		Iterations:  3,
		Parallelism: 2,
		SaltLength:  16,
		KeyLength:   32,
	}
}

func (p PasswordParams) Validate() error {
	if p.Parallelism < 1 || p.Parallelism > 16 {
		return errors.New("invalid Argon2id parallelism")
	}
	if p.MemoryKiB < 8*uint32(p.Parallelism) || p.MemoryKiB > 1024*1024 {
		return errors.New("invalid Argon2id memory cost")
	}
	if p.Iterations < 1 || p.Iterations > 10 {
		return errors.New("invalid Argon2id iteration cost")
	}
	if p.SaltLength < 8 || p.SaltLength > 64 || p.KeyLength < 16 || p.KeyLength > 64 {
		return errors.New("invalid Argon2id salt or key length")
	}
	return nil
}

func HashPassword(password string, params PasswordParams, random io.Reader) (string, error) {
	if err := params.Validate(); err != nil {
		return "", err
	}
	if random == nil {
		random = rand.Reader
	}

	salt := make([]byte, params.SaltLength)
	if _, err := io.ReadFull(random, salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}

	key := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.MemoryKiB,
		params.Parallelism,
		params.KeyLength,
	)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		params.MemoryKiB,
		params.Iterations,
		params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func VerifyPassword(password, encoded string) (bool, error) {
	params, salt, expected, err := parsePasswordHash(encoded)
	if err != nil {
		return false, err
	}

	actual := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.MemoryKiB,
		params.Parallelism,
		params.KeyLength,
	)

	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func PasswordHashNeedsUpgrade(encoded string, target PasswordParams) bool {
	params, _, _, err := parsePasswordHash(encoded)
	if err != nil {
		return false
	}

	notWeaker := target.MemoryKiB >= params.MemoryKiB &&
		target.Iterations >= params.Iterations &&
		target.Parallelism >= params.Parallelism &&
		target.SaltLength >= params.SaltLength &&
		target.KeyLength >= params.KeyLength
	stronger := target.MemoryKiB > params.MemoryKiB ||
		target.Iterations > params.Iterations ||
		target.Parallelism > params.Parallelism ||
		target.SaltLength > params.SaltLength ||
		target.KeyLength > params.KeyLength

	return notWeaker && stronger
}

func parsePasswordHash(encoded string) (PasswordParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return PasswordParams{}, nil, nil, ErrInvalidPasswordHash
	}

	version, err := parseNamedUint(parts[2], "v")
	if err != nil || version != argon2.Version {
		return PasswordParams{}, nil, nil, ErrInvalidPasswordHash
	}

	costs := strings.Split(parts[3], ",")
	if len(costs) != 3 {
		return PasswordParams{}, nil, nil, ErrInvalidPasswordHash
	}
	memory, err := parseNamedUint(costs[0], "m")
	if err != nil || memory > uint64(^uint32(0)) {
		return PasswordParams{}, nil, nil, ErrInvalidPasswordHash
	}
	iterations, err := parseNamedUint(costs[1], "t")
	if err != nil || iterations > uint64(^uint32(0)) {
		return PasswordParams{}, nil, nil, ErrInvalidPasswordHash
	}
	parallelism, err := parseNamedUint(costs[2], "p")
	if err != nil || parallelism > uint64(^uint8(0)) {
		return PasswordParams{}, nil, nil, ErrInvalidPasswordHash
	}

	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil {
		return PasswordParams{}, nil, nil, ErrInvalidPasswordHash
	}
	key, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil {
		return PasswordParams{}, nil, nil, ErrInvalidPasswordHash
	}

	params := PasswordParams{
		MemoryKiB:   uint32(memory),
		Iterations:  uint32(iterations),
		Parallelism: uint8(parallelism),
		SaltLength:  uint32(len(salt)),
		KeyLength:   uint32(len(key)),
	}
	if err := params.Validate(); err != nil {
		return PasswordParams{}, nil, nil, ErrInvalidPasswordHash
	}

	return params, salt, key, nil
}

func parseNamedUint(value, name string) (uint64, error) {
	prefix := name + "="
	if !strings.HasPrefix(value, prefix) {
		return 0, ErrInvalidPasswordHash
	}
	parsed, err := strconv.ParseUint(strings.TrimPrefix(value, prefix), 10, 64)
	if err != nil {
		return 0, ErrInvalidPasswordHash
	}
	return parsed, nil
}
