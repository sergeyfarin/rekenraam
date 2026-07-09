package app

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rekenraam/backend/internal/db"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePasswordHashRejectsOutOfRangeArgonParameters(t *testing.T) {
	t.Parallel()

	validHash := "$argon2id$v=19$m=19456,t=2,p=1$yFKjVHLDHsBTRk6lkk88Zg$dJ6u65HlcVuyRD4M7ArLq5QvFzwFgceJMsq/DIucXd0"

	cases := []struct {
		name        string
		replacement string
		message     string
	}{
		{
			name:        "memory overflows uint32",
			replacement: "m=4294967296,t=2,p=1",
			message:     "argon2 memory parameter out of range",
		},
		{
			name:        "iterations overflows uint32",
			replacement: "m=19456,t=4294967296,p=1",
			message:     "argon2 iterations parameter out of range",
		},
		{
			name:        "parallelism overflows uint8",
			replacement: "m=19456,t=2,p=256",
			message:     "argon2 parallelism parameter out of range",
		},
		{
			name:        "parallelism is zero",
			replacement: "m=19456,t=2,p=0",
			message:     "argon2 parallelism parameter out of range",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			encodedHash := strings.Replace(validHash, "m=19456,t=2,p=1", tc.replacement, 1)
			_, err := parsePasswordHash(encodedHash)

			require.Error(t, err)
			assert.Equal(t, tc.message, err.Error())
		})
	}
}

func TestParsePasswordHashAcceptsConfiguredArgonParameters(t *testing.T) {
	t.Parallel()

	parsed, err := parsePasswordHash("$argon2id$v=19$m=19456,t=2,p=1$yFKjVHLDHsBTRk6lkk88Zg$dJ6u65HlcVuyRD4M7ArLq5QvFzwFgceJMsq/DIucXd0")

	require.NoError(t, err)
	assert.Equal(t, 19, parsed.Version)
	assert.Equal(t, argon2MemoryKiB, parsed.MemoryKiB)
	assert.Equal(t, argon2Iterations, parsed.Iterations)
	assert.Equal(t, argon2Parallelism, parsed.Parallelism)
	assert.Len(t, parsed.Salt, argon2SaltLength)
	assert.Len(t, parsed.Hash, argon2KeyLength)
}

func TestSessionExpiresAtUsesConfiguredLifetime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)

	assert.Equal(t, "2026-07-08T18:00:00Z", sessionExpiresAt(now, 6*time.Hour))
}

func TestCleanupExpiredAndRevokedSessionsDeletesOnlyInactiveRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "test.sqlite"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	require.NoError(t, db.Migrate(ctx, database))

	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	insertedAt := now.Add(-time.Hour).Format(time.RFC3339)
	expiredAt := now.Add(-time.Minute).Format(time.RFC3339)
	activeExpiresAt := now.Add(time.Hour).Format(time.RFC3339)
	revokedAt := now.Add(-30 * time.Minute).Format(time.RFC3339)

	_, err = database.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, is_owner, created_at, updated_at)
		VALUES (1, 'owner', 'hash', 1, ?, ?)
	`, insertedAt, insertedAt)
	require.NoError(t, err)

	_, err = database.ExecContext(ctx, `
		INSERT INTO auth_sessions (id, user_id, token_hash, created_at, expires_at, revoked_at)
		VALUES
			(1, 1, 'active', ?, ?, NULL),
			(2, 1, 'expired', ?, ?, NULL),
			(3, 1, 'revoked', ?, ?, ?)
	`, insertedAt, activeExpiresAt, insertedAt, expiredAt, insertedAt, activeExpiresAt, revokedAt)
	require.NoError(t, err)

	service := NewAuthService(db.NewAuthRepository(database), slog.New(slog.NewTextHandler(io.Discard, nil)))
	service.now = func() time.Time { return now }

	deleted, err := service.CleanupExpiredAndRevokedSessions(ctx)

	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)

	var remainingTokenHash string
	err = database.QueryRowContext(ctx, `SELECT token_hash FROM auth_sessions`).Scan(&remainingTokenHash)
	require.NoError(t, err)
	assert.Equal(t, "active", remainingTokenHash)
}
