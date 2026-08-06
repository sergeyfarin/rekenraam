package app

import (
	"context"
	"database/sql"
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

// --- Authentication event visibility (S-07) ---

// newAuthEventTestService seeds one owner ("owner" / "correct-horse-battery")
// and returns a service whose clock is fixed, so event timestamps are exact.
func newAuthEventTestService(t *testing.T, now time.Time) (*sql.DB, *AuthService) {
	t.Helper()
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "test.sqlite"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	require.NoError(t, db.Migrate(ctx, database))

	passwordHash, err := hashPassword("correct-horse-battery")
	require.NoError(t, err)
	_, err = database.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, is_owner, created_at, updated_at)
		VALUES (1, 'owner', ?, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`, passwordHash)
	require.NoError(t, err)

	service := NewAuthService(db.NewAuthRepository(database), slog.New(slog.NewTextHandler(io.Discard, nil)))
	service.now = func() time.Time { return now }
	return database, service
}

func TestLogin_RecordsSuccessAndFailureEventsWithProxyAwareClientIP(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	_, service := newAuthEventTestService(t, now)

	_, err := service.Login(ctx, LoginInput{
		Username: "owner", Password: "wrong", ClientIP: "203.0.113.7", RequestID: "req-fail",
	})
	require.ErrorIs(t, err, ErrInvalidCredentials)

	_, err = service.Login(ctx, LoginInput{
		Username: "ghost", Password: "wrong", ClientIP: "203.0.113.7", RequestID: "req-unknown",
	})
	require.ErrorIs(t, err, ErrInvalidCredentials)

	result, err := service.Login(ctx, LoginInput{
		Username: "owner", Password: "correct-horse-battery", ClientIP: "203.0.113.9", RequestID: "req-ok",
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.SessionToken)

	events, err := service.AuthenticationEvents(ctx, 50)
	require.NoError(t, err)
	require.Len(t, events.Events, 3)
	assert.Equal(t, 2, events.FailedLast24h)
	assert.False(t, events.HasMore)

	// Newest first.
	success := events.Events[0]
	assert.Equal(t, authEventLoginSucceeded, success.EventType)
	assert.Equal(t, "success", success.Outcome)
	assert.Equal(t, "203.0.113.9", success.ClientIP, "the event must carry the proxy-aware client IP")
	assert.Equal(t, "req-ok", success.RequestID)
	require.NotNil(t, success.UserID)
	assert.Equal(t, int64(1), *success.UserID)
	require.NotNil(t, success.AuthSessionID, "a successful login must name the session it created")

	// The two failures are distinguishable: guessing a password on a real
	// account looks different from guessing account names.
	reasons := []string{events.Events[1].FailureReason, events.Events[2].FailureReason}
	assert.ElementsMatch(t, []string{authFailureUnknownUser, authFailureInvalidCredentials}, reasons)
	assert.Equal(t, "ghost", events.Events[1].Username, "the attempted username is what identifies an enumeration run")
}

func TestLogin_RecordsBlockedAttemptsSeparatelyFromFailures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	_, service := newAuthEventTestService(t, now)

	for attempt := 0; attempt < loginThrottleMaxFailures; attempt++ {
		_, err := service.Login(ctx, LoginInput{Username: "owner", Password: "wrong", ClientIP: "203.0.113.7"})
		require.Error(t, err)
	}

	// The next attempt never reaches password verification — it is refused by
	// the throttle, and that is a different operational fact.
	_, err := service.Login(ctx, LoginInput{Username: "owner", Password: "wrong", ClientIP: "203.0.113.7"})
	require.ErrorIs(t, err, ErrRateLimited)

	events, err := service.AuthenticationEvents(ctx, 50)
	require.NoError(t, err)
	assert.Equal(t, authEventLoginBlocked, events.Events[0].EventType)
	assert.Equal(t, authFailureRateLimited, events.Events[0].FailureReason)

	var blocked int
	for _, event := range events.Events {
		if event.EventType == authEventLoginBlocked {
			blocked++
		}
	}
	assert.Equal(t, 1, blocked)
}

func TestLogout_RecordsAnEventAndAnUnknownTokenRecordsNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	_, service := newAuthEventTestService(t, now)

	result, err := service.Login(ctx, LoginInput{
		Username: "owner", Password: "correct-horse-battery", ClientIP: "203.0.113.9",
	})
	require.NoError(t, err)

	require.NoError(t, service.Logout(ctx, LogoutInput{Token: result.SessionToken, ClientIP: "203.0.113.9"}))
	require.NoError(t, service.Logout(ctx, LogoutInput{Token: "not-a-real-token", ClientIP: "203.0.113.9"}))

	events, err := service.AuthenticationEvents(ctx, 50)
	require.NoError(t, err)
	require.Len(t, events.Events, 2, "a logout of an unresolvable token is a no-op and records nothing")
	assert.Equal(t, authEventLogout, events.Events[0].EventType)
	require.NotNil(t, events.Events[0].AuthSessionID)
	assert.Equal(t, "owner", events.Events[0].Username)
}

func TestAuthenticationEvents_NeverStorePasswordOrSessionTokenMaterial(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	database, service := newAuthEventTestService(t, now)

	const password = "correct-horse-battery"
	result, err := service.Login(ctx, LoginInput{Username: "owner", Password: password, ClientIP: "203.0.113.9"})
	require.NoError(t, err)
	_, err = service.Login(ctx, LoginInput{Username: "owner", Password: "wrong-secret-guess", ClientIP: "203.0.113.9"})
	require.Error(t, err)

	rows, err := database.QueryContext(ctx, `
		SELECT occurred_at || '|' || event_type || '|' || outcome || '|' || username
			|| '|' || client_ip || '|' || failure_reason || '|' || request_id
		FROM authentication_events
	`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var row string
		require.NoError(t, rows.Scan(&row))
		assert.NotContains(t, row, password)
		assert.NotContains(t, row, "wrong-secret-guess")
		assert.NotContains(t, row, result.SessionToken)
		assert.NotContains(t, row, hashSessionToken(result.SessionToken))
	}
	require.NoError(t, rows.Err())
}

func TestPruneAuthenticationEvents_KeepsOnlyTheRetentionWindow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	database, service := newAuthEventTestService(t, now)

	_, err := database.ExecContext(ctx, `
		INSERT INTO authentication_events (occurred_at, event_type, outcome, username, client_ip)
		VALUES (?, 'login_failed', 'failure', 'owner', '203.0.113.7'),
		       (?, 'login_failed', 'failure', 'owner', '203.0.113.7')
	`, now.Add(-authEventRetention-time.Hour).Format(time.RFC3339), now.Add(-time.Hour).Format(time.RFC3339))
	require.NoError(t, err)

	service.pruneAuthenticationEvents(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))

	events, err := service.AuthenticationEvents(ctx, 50)
	require.NoError(t, err)
	require.Len(t, events.Events, 1, "events older than the retention window must be pruned")
	assert.Equal(t, now.Add(-time.Hour).Format(time.RFC3339), events.Events[0].OccurredAt)
}
