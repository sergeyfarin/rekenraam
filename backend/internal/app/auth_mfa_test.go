package app

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/totp"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mfaTestSecretKey is a fixed 32-byte AES key. Production loads this from
// REKENRAAM_SECRET_KEY; the tests only need it to be stable.
var mfaTestSecretKey = []byte("0123456789abcdef0123456789abcdef")

// newMFATestService is the S-07 fixture plus a configured secret key, since
// MFA refuses to store a secret without one.
func newMFATestService(t *testing.T, now time.Time) (*sql.DB, *AuthService) {
	t.Helper()
	database, service := newAuthEventTestService(t, now)
	service.SetSecretKey(mfaTestSecretKey)
	return database, service
}

// enrollAndActivate walks the real enrollment path and returns the secret, so
// a test can mint codes the way an authenticator app would.
func enrollAndActivate(t *testing.T, service *AuthService, now time.Time) string {
	t.Helper()
	ctx := context.Background()

	enrollment, err := service.BeginTOTPEnrollment(ctx, MFAPasswordInput{
		UserID: 1, Password: "correct-horse-battery", ClientIP: "198.51.100.4",
	})
	require.NoError(t, err)
	require.NotEmpty(t, enrollment.Secret)

	// Activate with the *previous* step's code, which the skew window accepts.
	// The replay guard is real: activating with the current step's code would
	// burn it, and every login below would then be rejected as a replay.
	code, err := totp.Code(enrollment.Secret, totp.Step(now)-1)
	require.NoError(t, err)
	codes, err := service.ActivateTOTP(ctx, ActivateMFAInput{UserID: 1, Code: code, ClientIP: "198.51.100.4"})
	require.NoError(t, err)
	require.Len(t, codes, mfaRecoveryCodeCount)

	return enrollment.Secret
}

func TestLoginWithMFAActiveIssuesAChallengeInsteadOfASession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	database, service := newMFATestService(t, now)
	secret := enrollAndActivate(t, service, now)

	result, err := service.Login(ctx, LoginInput{
		Username: "owner", Password: "correct-horse-battery", ClientIP: "198.51.100.4",
	})
	require.NoError(t, err)
	assert.True(t, result.MFARequired)
	assert.Empty(t, result.SessionToken, "a password alone must not produce a session once MFA is active")
	assert.Empty(t, result.TrustedDeviceToken, "the device must not be approved before the second factor")
	require.NotEmpty(t, result.MFAChallengeToken)

	var sessions int
	require.NoError(t, database.QueryRow(`SELECT COUNT(1) FROM auth_sessions`).Scan(&sessions))
	assert.Zero(t, sessions, "no session row may exist for a half-authenticated attempt")

	code, err := totp.Code(secret, totp.Step(now))
	require.NoError(t, err)
	completed, err := service.CompleteLoginMFA(ctx, CompleteMFALoginInput{
		ChallengeToken: result.MFAChallengeToken, Code: code, ClientIP: "198.51.100.4",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, completed.SessionToken)
	assert.NotEmpty(t, completed.TrustedDeviceToken, "the device is approved once every factor verified")
	assert.Equal(t, "owner", completed.User.Username)
}

// A TOTP code is valid for its whole 30-second step, so an observed code must
// not work twice.
func TestCompleteLoginMFARejectsAReplayedCode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	_, service := newMFATestService(t, now)
	secret := enrollAndActivate(t, service, now)
	code, err := totp.Code(secret, totp.Step(now))
	require.NoError(t, err)

	first, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "198.51.100.4"})
	require.NoError(t, err)
	_, err = service.CompleteLoginMFA(ctx, CompleteMFALoginInput{
		ChallengeToken: first.MFAChallengeToken, Code: code, ClientIP: "198.51.100.4",
	})
	require.NoError(t, err)

	second, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "198.51.100.4"})
	require.NoError(t, err)
	_, err = service.CompleteLoginMFA(ctx, CompleteMFALoginInput{
		ChallengeToken: second.MFAChallengeToken, Code: code, ClientIP: "198.51.100.4",
	})
	assert.ErrorIs(t, err, ErrMFACodeInvalid, "the same code must not be usable twice within its step")
}

func TestCompleteLoginMFARejectsAReusedChallenge(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	_, service := newMFATestService(t, now)
	secret := enrollAndActivate(t, service, now)

	challenge, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "198.51.100.4"})
	require.NoError(t, err)
	code, err := totp.Code(secret, totp.Step(now))
	require.NoError(t, err)
	_, err = service.CompleteLoginMFA(ctx, CompleteMFALoginInput{
		ChallengeToken: challenge.MFAChallengeToken, Code: code, ClientIP: "198.51.100.4",
	})
	require.NoError(t, err)

	// Same challenge, a fresh code from the next step: the challenge itself is
	// spent, so this must not mint a second session.
	later := now.Add(totp.Period)
	service.now = func() time.Time { return later }
	nextCode, err := totp.Code(secret, totp.Step(later))
	require.NoError(t, err)
	_, err = service.CompleteLoginMFA(ctx, CompleteMFALoginInput{
		ChallengeToken: challenge.MFAChallengeToken, Code: nextCode, ClientIP: "198.51.100.4",
	})
	assert.ErrorIs(t, err, ErrMFAChallengeInvalid)
}

func TestCompleteLoginMFARejectsAnExpiredChallenge(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	_, service := newMFATestService(t, now)
	secret := enrollAndActivate(t, service, now)

	challenge, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "198.51.100.4"})
	require.NoError(t, err)

	later := now.Add(mfaChallengeLifetime + time.Second)
	service.now = func() time.Time { return later }
	code, err := totp.Code(secret, totp.Step(later))
	require.NoError(t, err)
	_, err = service.CompleteLoginMFA(ctx, CompleteMFALoginInput{
		ChallengeToken: challenge.MFAChallengeToken, Code: code, ClientIP: "198.51.100.4",
	})
	assert.ErrorIs(t, err, ErrMFAChallengeInvalid)
}

// Without this the second factor would be decorative: five password guesses
// per 15 minutes, but unlimited guesses at a six-digit code.
func TestCompleteLoginMFAThrottlesWrongCodes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	_, service := newMFATestService(t, now)
	enrollAndActivate(t, service, now)

	var lastErr error
	for range loginThrottleMaxFailures {
		challenge, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "198.51.100.4"})
		require.NoError(t, err)
		_, lastErr = service.CompleteLoginMFA(ctx, CompleteMFALoginInput{
			ChallengeToken: challenge.MFAChallengeToken, Code: "000000", ClientIP: "198.51.100.4",
		})
		require.Error(t, lastErr)
	}
	assert.ErrorIs(t, lastErr, ErrRateLimited, "code guessing must spend the same budget as password guessing")

	// The block is the shared one, so the password step is now refused too —
	// wrong codes cannot be used to keep guessing under a fresh challenge.
	_, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "198.51.100.4"})
	assert.ErrorIs(t, err, ErrRateLimited)
}

func TestRecoveryCodeCompletesLoginOnceAndOnlyOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	_, service := newMFATestService(t, now)

	enrollment, err := service.BeginTOTPEnrollment(ctx, MFAPasswordInput{UserID: 1, Password: "correct-horse-battery"})
	require.NoError(t, err)
	activationCode, err := totp.Code(enrollment.Secret, totp.Step(now)-1)
	require.NoError(t, err)
	recoveryCodes, err := service.ActivateTOTP(ctx, ActivateMFAInput{UserID: 1, Code: activationCode})
	require.NoError(t, err)

	challenge, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "198.51.100.4"})
	require.NoError(t, err)
	completed, err := service.CompleteLoginMFA(ctx, CompleteMFALoginInput{
		ChallengeToken: challenge.MFAChallengeToken, Code: recoveryCodes[0], ClientIP: "198.51.100.4",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, completed.SessionToken)

	status, err := service.MFAStatusFor(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, mfaRecoveryCodeCount-1, status.RecoveryCodesRemaining, "a spent code must be gone from the count")

	second, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "198.51.100.4"})
	require.NoError(t, err)
	_, err = service.CompleteLoginMFA(ctx, CompleteMFALoginInput{
		ChallengeToken: second.MFAChallengeToken, Code: recoveryCodes[0], ClientIP: "198.51.100.4",
	})
	assert.ErrorIs(t, err, ErrMFACodeInvalid, "a recovery code is single use")
}

// Lowercase, spaced, dash-free — a code read off paper must still work.
func TestRecoveryCodeAcceptsTheWayPeopleTypeItBack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	_, service := newMFATestService(t, now)

	enrollment, err := service.BeginTOTPEnrollment(ctx, MFAPasswordInput{UserID: 1, Password: "correct-horse-battery"})
	require.NoError(t, err)
	activationCode, err := totp.Code(enrollment.Secret, totp.Step(now)-1)
	require.NoError(t, err)
	recoveryCodes, err := service.ActivateTOTP(ctx, ActivateMFAInput{UserID: 1, Code: activationCode})
	require.NoError(t, err)

	messy := " " + strings.ToLower(strings.ReplaceAll(recoveryCodes[0], "-", " ")) + " "
	challenge, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "198.51.100.4"})
	require.NoError(t, err)
	completed, err := service.CompleteLoginMFA(ctx, CompleteMFALoginInput{
		ChallengeToken: challenge.MFAChallengeToken, Code: messy, ClientIP: "198.51.100.4",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, completed.SessionToken)
}

// A secret that was issued but never confirmed must never gate a login —
// otherwise an abandoned enrollment locks the owner out of their own app.
func TestPendingEnrollmentDoesNotGateLogin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	_, service := newMFATestService(t, now)

	_, err := service.BeginTOTPEnrollment(ctx, MFAPasswordInput{UserID: 1, Password: "correct-horse-battery"})
	require.NoError(t, err)

	result, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "198.51.100.4"})
	require.NoError(t, err)
	assert.False(t, result.MFARequired)
	assert.NotEmpty(t, result.SessionToken)

	status, err := service.MFAStatusFor(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "pending", status.Status)
	assert.Zero(t, status.RecoveryCodesRemaining, "recovery codes are issued at activation, not enrollment")
}

func TestActivateTOTPRejectsAWrongCode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	_, service := newMFATestService(t, now)

	_, err := service.BeginTOTPEnrollment(ctx, MFAPasswordInput{UserID: 1, Password: "correct-horse-battery"})
	require.NoError(t, err)
	_, err = service.ActivateTOTP(ctx, ActivateMFAInput{UserID: 1, Code: "000000"})
	assert.ErrorIs(t, err, ErrMFACodeInvalid)

	status, err := service.MFAStatusFor(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "pending", status.Status, "a failed activation must not turn MFA on")
}

// Enrollment and disabling both change what protects the account, so a stolen
// session alone must not be enough.
func TestMFAChangesRequireThePasswordAgain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	_, service := newMFATestService(t, now)

	_, err := service.BeginTOTPEnrollment(ctx, MFAPasswordInput{UserID: 1, Password: "wrong-password"})
	assert.ErrorIs(t, err, ErrInvalidCredentials)

	enrollAndActivate(t, service, now)

	err = service.DisableMFA(ctx, MFAPasswordInput{UserID: 1, Password: "wrong-password"})
	assert.ErrorIs(t, err, ErrInvalidCredentials)
	status, err := service.MFAStatusFor(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "active", status.Status)

	_, err = service.RegenerateRecoveryCodes(ctx, MFAPasswordInput{UserID: 1, Password: "wrong-password"})
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

// Re-enrolling over a live enrollment would be a silent way to swap the second
// factor; turning it off is an explicit act.
func TestEnrollmentIsRefusedWhileMFAIsActive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	_, service := newMFATestService(t, now)
	enrollAndActivate(t, service, now)

	_, err := service.BeginTOTPEnrollment(ctx, MFAPasswordInput{UserID: 1, Password: "correct-horse-battery"})
	assert.ErrorIs(t, err, ErrMFAAlreadyActive)
}

func TestDisableMFARemovesTheEnrollmentAndItsRecoveryCodes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	database, service := newMFATestService(t, now)
	enrollAndActivate(t, service, now)

	require.NoError(t, service.DisableMFA(ctx, MFAPasswordInput{UserID: 1, Password: "correct-horse-battery"}))

	status, err := service.MFAStatusFor(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "disabled", status.Status)

	var codes int
	require.NoError(t, database.QueryRow(`SELECT COUNT(1) FROM user_mfa_recovery_codes`).Scan(&codes))
	assert.Zero(t, codes, "orphan recovery codes would outlive the enrollment they belong to")

	result, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "198.51.100.4"})
	require.NoError(t, err)
	assert.False(t, result.MFARequired)
	assert.NotEmpty(t, result.SessionToken)
}

func TestRegenerateRecoveryCodesInvalidatesTheOldSet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	_, service := newMFATestService(t, now)

	enrollment, err := service.BeginTOTPEnrollment(ctx, MFAPasswordInput{UserID: 1, Password: "correct-horse-battery"})
	require.NoError(t, err)
	activationCode, err := totp.Code(enrollment.Secret, totp.Step(now)-1)
	require.NoError(t, err)
	original, err := service.ActivateTOTP(ctx, ActivateMFAInput{UserID: 1, Code: activationCode})
	require.NoError(t, err)

	replacement, err := service.RegenerateRecoveryCodes(ctx, MFAPasswordInput{UserID: 1, Password: "correct-horse-battery"})
	require.NoError(t, err)
	require.Len(t, replacement, mfaRecoveryCodeCount)
	assert.NotEqual(t, original[0], replacement[0])

	challenge, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "198.51.100.4"})
	require.NoError(t, err)
	_, err = service.CompleteLoginMFA(ctx, CompleteMFALoginInput{
		ChallengeToken: challenge.MFAChallengeToken, Code: original[0], ClientIP: "198.51.100.4",
	})
	assert.ErrorIs(t, err, ErrMFACodeInvalid, "a code from the replaced set must be dead")
}

// The secret is credential-equivalent, so without somewhere safe to put it the
// answer is to refuse, never to store it in the clear.
func TestEnrollmentRefusesWithoutAConfiguredSecretKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	_, service := newAuthEventTestService(t, now)

	_, err := service.BeginTOTPEnrollment(ctx, MFAPasswordInput{UserID: 1, Password: "correct-horse-battery"})
	assert.ErrorIs(t, err, ErrSecretKeyMissing)

	status, err := service.MFAStatusFor(ctx, 1)
	require.NoError(t, err)
	assert.False(t, status.Configured, "the UI must be able to explain why enrollment is unavailable")
	assert.Equal(t, "disabled", status.Status)
}

func TestSecretIsNotStoredInTheClear(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	database, service := newMFATestService(t, now)
	secret := enrollAndActivate(t, service, now)

	var stored string
	require.NoError(t, database.QueryRow(`SELECT secret_ciphertext FROM user_mfa_totp WHERE user_id = 1`).Scan(&stored))
	assert.NotContains(t, stored, secret)
	assert.NotEmpty(t, stored)
}

func TestFailedMFACodeIsVisibleInTheAuthenticationLog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	database, service := newMFATestService(t, now)
	enrollAndActivate(t, service, now)

	challenge, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "198.51.100.4"})
	require.NoError(t, err)
	_, err = service.CompleteLoginMFA(ctx, CompleteMFALoginInput{
		ChallengeToken: challenge.MFAChallengeToken, Code: "000000", ClientIP: "198.51.100.4", RequestID: "req-mfa",
	})
	require.ErrorIs(t, err, ErrMFACodeInvalid)

	var reason string
	require.NoError(t, database.QueryRow(`
		SELECT failure_reason FROM authentication_events
		WHERE request_id = 'req-mfa' ORDER BY id DESC LIMIT 1
	`).Scan(&reason))
	assert.Equal(t, authFailureMFAInvalid, reason,
		"an operator must be able to tell a wrong code from a wrong password")
}

// Losing REKENRAAM_SECRET_KEY makes the TOTP secret unreadable. That is
// precisely what recovery codes are for, so it must not lock the owner out:
// the codes are hashed, not sealed, and stay usable.
func TestRecoveryCodesStillWorkWhenTheSecretCannotBeOpened(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	_, service := newMFATestService(t, now)

	enrollment, err := service.BeginTOTPEnrollment(ctx, MFAPasswordInput{UserID: 1, Password: "correct-horse-battery"})
	require.NoError(t, err)
	activationCode, err := totp.Code(enrollment.Secret, totp.Step(now)-1)
	require.NoError(t, err)
	recoveryCodes, err := service.ActivateTOTP(ctx, ActivateMFAInput{UserID: 1, Code: activationCode})
	require.NoError(t, err)

	// The operator rotated or lost the key.
	service.SetSecretKey([]byte("ffffffffffffffffffffffffffffffff"))

	challenge, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "198.51.100.4"})
	require.NoError(t, err)
	assert.True(t, challenge.MFARequired, "a lost key must not silently drop the second factor either")

	completed, err := service.CompleteLoginMFA(ctx, CompleteMFALoginInput{
		ChallengeToken: challenge.MFAChallengeToken, Code: recoveryCodes[0], ClientIP: "198.51.100.4",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, completed.SessionToken)
}

// The recover-owner command is the last resort for an owner who has lost both
// the authenticator and the recovery codes. It already requires filesystem
// access to the database, so an enrollment must not outlive it — otherwise the
// new password opens nothing.
func TestOwnerRecoveryClearsTheMFAEnrollment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	database, service := newMFATestService(t, now)
	enrollAndActivate(t, service, now)

	recovery := NewRecoveryService(database, "file:test.sqlite", db.NewRecoveryRepository(database))
	require.NoError(t, recovery.ResetOwnerAccess(ctx, "a-brand-new-password"))

	status, err := service.MFAStatusFor(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "disabled", status.Status)

	result, err := service.Login(ctx, LoginInput{Username: "owner", Password: "a-brand-new-password", ClientIP: "198.51.100.4"})
	require.NoError(t, err)
	assert.False(t, result.MFARequired)
	assert.NotEmpty(t, result.SessionToken)
}
