package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMFATestRepository(t *testing.T) (*sql.DB, *AuthRepository) {
	t.Helper()

	database := openTestDatabase(t)
	require.NoError(t, Migrate(context.Background(), database))
	_, err := database.ExecContext(context.Background(), `
		INSERT INTO users (id, username, password_hash, is_owner, created_at, updated_at)
		VALUES (1, 'owner', 'hash', 1, '2026-08-01T00:00:00Z', '2026-08-01T00:00:00Z');
	`)
	require.NoError(t, err)

	return database, NewAuthRepository(database)
}

func enrollPendingMFA(t *testing.T, repository *AuthRepository) {
	t.Helper()

	require.NoError(t, repository.UpsertMFATOTP(context.Background(), UpsertMFATOTPParams{
		UserID:           1,
		SecretCiphertext: "sealed-secret",
		Status:           "pending",
		CreatedAt:        "2026-08-01T00:00:00Z",
	}))
}

func readMFAStatus(t *testing.T, database *sql.DB) string {
	t.Helper()

	var status string
	require.NoError(t, database.QueryRowContext(context.Background(),
		`SELECT status FROM user_mfa_totp WHERE user_id = 1`).Scan(&status))

	return status
}

func countMFARecoveryCodes(t *testing.T, database *sql.DB) int {
	t.Helper()

	var count int
	require.NoError(t, database.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM user_mfa_recovery_codes WHERE user_id = 1`).Scan(&count))

	return count
}

func TestActivateMFATOTPWithRecoveryCodesTurnsMFAOnAndStoresTheCodes(t *testing.T) {
	database, repository := newMFATestRepository(t)
	enrollPendingMFA(t, repository)

	require.NoError(t, repository.ActivateMFATOTPWithRecoveryCodes(
		context.Background(), 1, "2026-08-01T00:05:00Z", 42,
		[]string{"hash-a", "hash-b", "hash-c"},
	))

	assert.Equal(t, "active", readMFAStatus(t, database))
	assert.Equal(t, 3, countMFARecoveryCodes(t, database))
}

// A failed recovery-code insert must take the activation down with it. The
// duplicate hash trips the UNIQUE index on code_hash, standing in for any
// mid-activation failure: the owner must be left with a retryable pending
// enrollment, never an active second factor they hold no recovery codes for.
func TestActivateMFATOTPWithRecoveryCodesLeavesTheEnrollmentPendingWhenCodesFail(t *testing.T) {
	database, repository := newMFATestRepository(t)
	enrollPendingMFA(t, repository)

	err := repository.ActivateMFATOTPWithRecoveryCodes(
		context.Background(), 1, "2026-08-01T00:05:00Z", 42,
		[]string{"hash-a", "hash-a"},
	)

	require.Error(t, err)
	assert.Equal(t, "pending", readMFAStatus(t, database))
	assert.Equal(t, 0, countMFARecoveryCodes(t, database))
}

// The retry after a failed activation must work, which is only true because
// the failure left the enrollment pending.
func TestActivateMFATOTPWithRecoveryCodesCanBeRetriedAfterAFailure(t *testing.T) {
	database, repository := newMFATestRepository(t)
	enrollPendingMFA(t, repository)

	require.Error(t, repository.ActivateMFATOTPWithRecoveryCodes(
		context.Background(), 1, "2026-08-01T00:05:00Z", 42,
		[]string{"hash-a", "hash-a"},
	))
	require.NoError(t, repository.ActivateMFATOTPWithRecoveryCodes(
		context.Background(), 1, "2026-08-01T00:06:00Z", 43,
		[]string{"hash-a", "hash-b"},
	))

	assert.Equal(t, "active", readMFAStatus(t, database))
	assert.Equal(t, 2, countMFARecoveryCodes(t, database))
}

// Activation is only ever a pending -> active promotion; a second activation
// of an already-active enrollment must not silently reissue its codes.
func TestActivateMFATOTPWithRecoveryCodesRejectsAnAlreadyActiveEnrollment(t *testing.T) {
	database, repository := newMFATestRepository(t)
	enrollPendingMFA(t, repository)

	require.NoError(t, repository.ActivateMFATOTPWithRecoveryCodes(
		context.Background(), 1, "2026-08-01T00:05:00Z", 42, []string{"hash-a"},
	))

	err := repository.ActivateMFATOTPWithRecoveryCodes(
		context.Background(), 1, "2026-08-01T00:06:00Z", 43, []string{"hash-b"},
	)

	require.ErrorIs(t, err, ErrNotFound)
	assert.Equal(t, 1, countMFARecoveryCodes(t, database))
}
