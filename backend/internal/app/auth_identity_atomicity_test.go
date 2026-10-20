package app

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/totp"
)

// Identity concurrency and failure-atomicity audit (#267). Interleavings use
// the service's existing clock seam; failures are injected with a temporary
// SQLite trigger on the test database, so no production hook is involved.

func injectInsertFailure(t *testing.T, database *sql.DB, table string) {
	t.Helper()
	_, err := database.ExecContext(context.Background(), `
		CREATE TEMP TRIGGER inject_failure BEFORE INSERT ON `+table+`
		BEGIN SELECT RAISE(ABORT, 'injected failure'); END
	`)
	require.NoError(t, err)
}

func injectDeleteFailure(t *testing.T, database *sql.DB, table string) {
	t.Helper()
	_, err := database.ExecContext(context.Background(), `
		CREATE TEMP TRIGGER inject_failure BEFORE DELETE ON `+table+`
		BEGIN SELECT RAISE(ABORT, 'injected failure'); END
	`)
	require.NoError(t, err)
}

func removeInjectedFailure(t *testing.T, database *sql.DB) {
	t.Helper()
	_, err := database.ExecContext(context.Background(), `DROP TRIGGER temp.inject_failure`)
	require.NoError(t, err)
}

func countRows(t *testing.T, database *sql.DB, query string, args ...any) int {
	t.Helper()
	var count int
	require.NoError(t, database.QueryRowContext(context.Background(), query, args...).Scan(&count))
	return count
}

// A failure that interleaves with another one must still be counted: the
// throttle row is a read-modify-write, and a lost update gives the attacker
// a free guess per overlap.
func TestOverlappingPasswordFailuresAreAllCounted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	database, service := newAuthEventTestService(t, now)
	// No client address, so the username counter is the only scope: a lost
	// update there cannot be masked by the address counter.
	wrong := LoginInput{Username: "owner", Password: "wrong-password"}

	// Queue both real consumers before admitting either. Decision clocks now
	// run under the writer lock, where a recursive service call would deadlock.
	connection, err := database.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	before := database.Stats().WaitCount
	results := make(chan error, 2)
	for range 2 {
		go func() { _, err := service.Login(ctx, wrong); results <- err }()
	}
	require.Eventually(t, func() bool { return database.Stats().WaitCount >= before+2 }, 5*time.Second, time.Millisecond)
	require.NoError(t, connection.Close())
	for range 2 {
		require.ErrorIs(t, <-results, ErrInvalidCredentials)
	}
	for range loginThrottleMaxFailures - 2 {
		_, err := service.Login(ctx, wrong)
		require.Error(t, err)
	}

	_, err = service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery"})
	assert.ErrorIs(t, err, ErrRateLimited, "five failures, two of them overlapping, must exhaust the budget")
}

// A burst of simultaneous guesses must not verify more passwords than the
// window allows. Admission has to be decided atomically before the hash is
// computed, or every request that read the counter early gets a guess.
func TestConcurrentPasswordGuessesNeverExceedTheBudget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	_, service := newAuthEventTestService(t, now)

	const attempts = 16
	var wg sync.WaitGroup
	results := make(chan error, attempts)
	start := make(chan struct{})
	for range attempts {
		wg.Go(func() {
			<-start
			_, err := service.Login(ctx, LoginInput{Username: "owner", Password: "wrong-password", ClientIP: "203.0.113.10"})
			results <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)

	for err := range results {
		if !errors.Is(err, ErrRateLimited) && !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("unexpected login result: %v", err)
		}
	}
	// A rate-limited response can still follow a verified guess, so count the
	// verifications the authentication log recorded, not the response codes.
	verified := countAuthEvents(t, service, authEventLoginFailed, authFailureInvalidCredentials)
	assert.LessOrEqual(t, verified, loginThrottleMaxFailures, "guesses verified within one window")
}

// An attempt that is admitted but not yet settled holds its place in the
// budget, and a correct password gives that place back.
func TestReservedAttemptsHoldTheBudgetUntilSettled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	_, service := newAuthEventTestService(t, now)
	scopes := loginThrottleScopes("owner", "203.0.113.11")

	reservations := make([]*throttleReservation, 0, loginThrottleMaxFailures)
	for range loginThrottleMaxFailures {
		reservation, err := service.reserveAttempt(ctx, scopes)
		require.NoError(t, err)
		reservations = append(reservations, reservation)
	}
	_, err := service.reserveAttempt(ctx, scopes)
	require.ErrorIs(t, err, ErrRateLimited, "a sixth in-flight attempt must wait")

	reservations[0].release(ctx)
	reservation, err := service.reserveAttempt(ctx, scopes)
	require.NoError(t, err, "a released place is available again")
	reservations[0] = reservation

	// Unsettled reservations from a crashed process count as failures, then
	// age out with the window instead of locking the scope forever.
	service.now = func() time.Time { return now.Add(loginThrottleWindow + time.Second) }
	_, err = service.reserveAttempt(ctx, scopes)
	assert.NoError(t, err)
}

// A rejected login must not leave a live session behind: if device approval
// fails after the session row was written, the attempt is an error and its
// session must not exist.
func TestFailedDeviceApprovalLeavesNoOrphanSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	database, service := newAuthEventTestService(t, now)
	injectInsertFailure(t, database, "login_trusted_devices")

	_, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "203.0.113.12"})
	require.Error(t, err)

	assert.Zero(t, countRows(t, database, `SELECT COUNT(1) FROM auth_sessions WHERE revoked_at IS NULL`), "orphan session")
	assert.Zero(t, countAuthEvents(t, service, authEventLoginSucceeded, ""), "no success record")
}

// A failure after the second factor and challenge were spent rejects the
// attempt without leaving either one spendable or a session behind.
func TestFailedSessionInsertAfterMFASpendsTheChallenge(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	database, service := newMFATestService(t, now)
	secret := enrollAndActivate(t, service, now)

	login, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "203.0.113.13"})
	require.NoError(t, err)
	require.True(t, login.MFARequired)
	code, err := totp.Code(secret, totp.Step(now))
	require.NoError(t, err)

	injectInsertFailure(t, database, "auth_sessions")
	_, err = service.CompleteLoginMFA(ctx, CompleteMFALoginInput{ChallengeToken: login.MFAChallengeToken, Code: code, ClientIP: "203.0.113.13"})
	require.Error(t, err)
	removeInjectedFailure(t, database)

	assert.Zero(t, countRows(t, database, `SELECT COUNT(1) FROM auth_sessions`))
	assert.Zero(t, countAuthEvents(t, service, authEventLoginSucceeded, ""))
	// Neither the challenge nor the code can be replayed into a session.
	_, err = service.CompleteLoginMFA(ctx, CompleteMFALoginInput{ChallengeToken: login.MFAChallengeToken, Code: code, ClientIP: "203.0.113.13"})
	assert.ErrorIs(t, err, ErrMFAChallengeInvalid)
	fresh, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "203.0.113.13"})
	require.NoError(t, err)
	_, err = service.CompleteLoginMFA(ctx, CompleteMFALoginInput{ChallengeToken: fresh.MFAChallengeToken, Code: code, ClientIP: "203.0.113.13"})
	assert.ErrorIs(t, err, ErrMFACodeInvalid, "the spent step stays spent")
}

// A failed authentication-event write does not fail a verified login; it is
// logged instead (recordAuthEvent). This pins that documented choice.
func TestFailedEventWriteDoesNotTurnIntoALockout(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	database, service := newAuthEventTestService(t, now)
	injectInsertFailure(t, database, "authentication_events")

	result, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "203.0.113.14"})
	require.NoError(t, err)
	assert.NotEmpty(t, result.SessionToken)
}

// Disable, regeneration and owner recovery are each one transaction: a
// failure part-way leaves the previous protection fully in place.
func TestMFAManagementFailuresLeaveThePreviousStateIntact(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	database, service := newMFATestService(t, now)
	enrollAndActivate(t, service, now)
	codesBefore := countRows(t, database, `SELECT COUNT(1) FROM user_mfa_recovery_codes WHERE used_at IS NULL`)
	// A row-level trigger needs a row: an outstanding challenge to delete.
	_, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "203.0.113.15"})
	require.NoError(t, err)

	injectDeleteFailure(t, database, "login_mfa_challenges")
	require.Error(t, service.DisableMFA(ctx, rightPassword(1)))
	removeInjectedFailure(t, database)
	status, err := service.MFAStatusFor(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "active", status.Status)
	assert.Equal(t, codesBefore, status.RecoveryCodesRemaining)

	_, err = database.ExecContext(ctx, `
		CREATE TEMP TRIGGER inject_failure BEFORE INSERT ON user_mfa_recovery_codes
		WHEN (SELECT COUNT(1) FROM user_mfa_recovery_codes) >= 3
		BEGIN SELECT RAISE(ABORT, 'injected failure'); END
	`)
	require.NoError(t, err)
	_, err = service.RegenerateRecoveryCodes(ctx, rightPassword(1))
	require.Error(t, err)
	removeInjectedFailure(t, database)
	status, err = service.MFAStatusFor(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, codesBefore, status.RecoveryCodesRemaining, "the old set survives a failed regeneration")

	challengesBefore := countRows(t, database, `SELECT COUNT(1) FROM login_mfa_challenges`)
	_, err = database.ExecContext(ctx, `INSERT INTO auth_sessions (user_id, token_hash, created_at, expires_at) VALUES (1, 'live', '2026-10-17T12:00:00Z', '2026-11-17T12:00:00Z')`)
	require.NoError(t, err)
	injectDeleteFailure(t, database, "user_mfa_totp")
	recovery := NewRecoveryService(database, "", db.NewRecoveryRepository(database))
	require.Error(t, recovery.ResetOwnerAccess(ctx, ResetOwnerAccessInput{Password: "a-brand-new-password"}))
	removeInjectedFailure(t, database)
	assert.Equal(t, 1, countRows(t, database, `SELECT COUNT(1) FROM auth_sessions WHERE revoked_at IS NULL`))
	assert.Equal(t, challengesBefore, countRows(t, database, `SELECT COUNT(1) FROM login_mfa_challenges`))
	login, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "203.0.113.15"})
	require.NoError(t, err, "the old password still opens the account")
	assert.True(t, login.MFARequired)
}

// Revocation cutoff: disabling MFA ends every outstanding challenge, and a
// regeneration ends every old recovery code, even for an attempt that had
// already proved its password.
func TestMFAChangesCutOffInFlightChallengesAndCodes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	_, service := newMFATestService(t, now)
	secret := enrollAndActivate(t, service, now)
	code, err := totp.Code(secret, totp.Step(now))
	require.NoError(t, err)

	pending, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "203.0.113.16"})
	require.NoError(t, err)
	oldCodes, err := service.RegenerateRecoveryCodes(ctx, rightPassword(1))
	require.NoError(t, err)
	_, err = service.RegenerateRecoveryCodes(ctx, rightPassword(1))
	require.NoError(t, err)
	_, err = service.CompleteLoginMFA(ctx, CompleteMFALoginInput{ChallengeToken: pending.MFAChallengeToken, Code: oldCodes[0], ClientIP: "203.0.113.16"})
	assert.ErrorIs(t, err, ErrMFACodeInvalid, "a replaced recovery code is dead for an in-flight challenge")

	require.NoError(t, service.DisableMFA(ctx, rightPassword(1)))
	_, err = service.CompleteLoginMFA(ctx, CompleteMFALoginInput{ChallengeToken: pending.MFAChallengeToken, Code: code, ClientIP: "203.0.113.16"})
	assert.ErrorIs(t, err, ErrMFAChallengeInvalid, "disable ends every outstanding challenge")
}

// The MFA-settings reauthentication budget (#262) is admitted the same way:
// parallel wrong confirmations from one session verify at most five guesses.
func TestConcurrentReauthenticationGuessesNeverExceedTheBudget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	_, service := newMFATestService(t, now)
	enrollAndActivate(t, service, now)

	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 16 {
		wg.Go(func() {
			<-start
			err := service.DisableMFA(ctx, wrongPassword(7))
			assert.True(t, errors.Is(err, ErrRateLimited) || errors.Is(err, ErrInvalidCredentials), "unexpected result: %v", err)
		})
	}
	close(start)
	wg.Wait()

	assert.LessOrEqual(t, countAuthEvents(t, service, authEventLoginFailed, authFailureInvalidCredentials), loginThrottleMaxFailures)
	status, err := service.MFAStatusFor(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "active", status.Status)
}
