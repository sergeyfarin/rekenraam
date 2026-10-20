package db

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/migrations"
)

// Migration 0014 (#288) rebuilds authentication_events. An upgrade keeps
// every sign-in row exactly, defaulting the new columns; a downgrade refuses
// to drop account-security history.
func TestAuthenticationSecurityEventsMigrationKeepsSignInHistoryAndRefusesLossyDowngrade(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, _ := newMFATestRepository(t)
	provider, err := goose.NewProvider(goose.DialectSQLite3, database, migrations.FS)
	require.NoError(t, err)
	_, err = provider.DownTo(ctx, 13)
	require.NoError(t, err)

	_, err = database.ExecContext(ctx, `
		INSERT INTO authentication_events (id, occurred_at, event_type, outcome, username, user_id, client_ip, failure_reason, request_id)
		VALUES (7, '2026-08-01T00:00:00Z', 'login_failed', 'failure', 'owner', 1, '203.0.113.7', 'invalid_credentials', 'req-7')
	`)
	require.NoError(t, err)

	_, err = provider.Up(ctx)
	require.NoError(t, err)
	var eventType, reason, requestID, origin, factor, metadata string
	require.NoError(t, database.QueryRowContext(ctx, `
		SELECT event_type, failure_reason, request_id, origin, second_factor, metadata_json
		FROM authentication_events WHERE id = 7
	`).Scan(&eventType, &reason, &requestID, &origin, &factor, &metadata))
	assert.Equal(t, []string{"login_failed", "invalid_credentials", "req-7", "browser_api", "", "{}"},
		[]string{eventType, reason, requestID, origin, factor, metadata})

	_, err = database.ExecContext(ctx, `
		INSERT INTO authentication_events (occurred_at, event_type, outcome, second_factor)
		VALUES ('2026-08-01T00:00:00Z', 'mfa_disabled', 'success', 'totp')
	`)
	require.Error(t, err, "only a sign-in names the factor it used")
	_, err = database.ExecContext(ctx, `
		INSERT INTO authentication_events (occurred_at, event_type, outcome)
		VALUES ('2026-08-01T00:00:00Z', 'owner_access_recovered', 'success')
	`)
	require.Error(t, err, "owner recovery is only ever recorded from the CLI")

	require.NoError(t, insertAuthenticationEvent(ctx, database, securityTestEvent("mfa_disabled")))
	_, err = provider.DownTo(ctx, 13)
	require.ErrorContains(t, err, "CHECK constraint failed")
	var count int
	require.NoError(t, database.QueryRowContext(ctx, `SELECT count(*) FROM authentication_events WHERE event_type = 'mfa_disabled'`).Scan(&count))
	assert.Equal(t, 1, count)
}

func countAuthenticationEvents(t *testing.T, repository *AuthRepository, eventType string) int {
	t.Helper()

	var count int
	require.NoError(t, repository.database.QueryRowContext(context.Background(),
		`SELECT count(*) FROM authentication_events WHERE event_type = ?`, eventType).Scan(&count))

	return count
}

// A security change and its event commit together: a change whose event
// cannot be written does not happen (#288).
func TestSecurityChangeIsRolledBackWhenItsEventCannotBeWritten(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, repository := newMFATestRepository(t)
	enrollPendingMFA(t, repository)
	require.NoError(t, repository.ActivateMFATOTPWithRecoveryCodes(
		ctx, 1, "sealed-secret", "2026-08-01T00:05:00Z", 42, []string{"hash-a"}, securityTestEvent("mfa_activated"), func() error { return nil },
	))

	invalid := securityTestEvent("not_an_event_type")
	_, err := repository.DeleteMFATOTP(ctx, 1, invalid)
	require.Error(t, err)
	assert.Equal(t, "active", readMFAStatus(t, database), "MFA must stay on when its disable cannot be recorded")
	require.Error(t, repository.ReplaceMFARecoveryCodes(ctx, 1, []string{"hash-b"}, "2026-08-01T00:06:00Z", invalid))
	var code string
	require.NoError(t, database.QueryRowContext(ctx, `SELECT code_hash FROM user_mfa_recovery_codes WHERE user_id = 1`).Scan(&code))
	assert.Equal(t, "hash-a", code)

	disabled, err := repository.DeleteMFATOTP(ctx, 1, securityTestEvent("mfa_disabled"))
	require.NoError(t, err)
	assert.True(t, disabled)
	var metadata string
	require.NoError(t, database.QueryRowContext(ctx,
		`SELECT metadata_json FROM authentication_events WHERE event_type = 'mfa_disabled'`).Scan(&metadata))
	assert.JSONEq(t, `{"previous_mfa_status":"active"}`, metadata)

	disabled, err = repository.DeleteMFATOTP(ctx, 1, securityTestEvent("mfa_disabled"))
	require.NoError(t, err)
	assert.False(t, disabled)
	assert.Equal(t, 1, countAuthenticationEvents(t, repository, "mfa_disabled"),
		"disabling an absent enrollment changes nothing and records nothing")
}

// Retention prunes sign-in noise only; who weakened or reset protection is
// kept (#288).
func TestAuthenticationEventRetentionKeepsAccountSecurityEvents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, repository := newMFATestRepository(t)
	for _, eventType := range []string{"login_succeeded", "logout", "mfa_disabled", "trusted_device_revoked"} {
		require.NoError(t, insertAuthenticationEvent(ctx, repository.database, AuthenticationEventParams{
			OccurredAt: "2025-01-01T00:00:00Z", EventType: eventType, Outcome: "success", UserID: 1,
		}))
	}

	deleted, err := repository.DeleteAuthenticationEventsBefore(ctx, "2026-01-01T00:00:00Z")
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)
	assert.Equal(t, 1, countAuthenticationEvents(t, repository, "mfa_disabled"))
	assert.Equal(t, 1, countAuthenticationEvents(t, repository, "trusted_device_revoked"))
}
