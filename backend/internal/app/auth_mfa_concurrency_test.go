package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/totp"
)

func TestMFAEnrollmentCannotOverwriteAnActivation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	_, service := newMFATestService(t, now)
	input := MFAPasswordInput{UserID: 1, SessionID: 1, Password: "correct-horse-battery", ClientIP: "198.51.100.4"}
	initial, err := service.BeginTOTPEnrollment(ctx, input)
	require.NoError(t, err)
	code, err := totp.Code(initial.Secret, totp.Step(now)-1)
	require.NoError(t, err)
	// The clock is the existing service seam immediately before enrollment writes.
	// A second request activates after the first request read pending state.
	calls := 0
	service.now = func() time.Time {
		calls++
		if calls == 1 {
			return now
		} // throttle clock owns the transaction
		service.now = func() time.Time { return now }
		_, err := service.ActivateTOTP(ctx, ActivateMFAInput{UserID: 1, SessionID: 1, Code: code})
		require.NoError(t, err)
		return now
	}
	_, err = service.BeginTOTPEnrollment(ctx, input)
	assert.ErrorIs(t, err, ErrMFAAlreadyActive)
	status, err := service.MFAStatusFor(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "active", status.Status)
	login, err := service.Login(ctx, LoginInput{Username: "owner", Password: input.Password, ClientIP: input.ClientIP})
	require.NoError(t, err)
	assert.True(t, login.MFARequired)
	assert.Empty(t, login.SessionToken)
}

func TestMFAActivationMustProveCurrentPendingSecret(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	_, service := newMFATestService(t, now)
	input := MFAPasswordInput{UserID: 1, SessionID: 1, Password: "correct-horse-battery", ClientIP: "198.51.100.4"}
	initial, err := service.BeginTOTPEnrollment(ctx, input)
	require.NoError(t, err)
	code, err := totp.Code(initial.Secret, totp.Step(now)-1)
	require.NoError(t, err)
	// Another request replaces the pending secret after this activation read it.
	calls := 0
	service.now = func() time.Time {
		calls++
		if calls == 1 {
			return now
		} // throttle clock owns the transaction
		service.now = func() time.Time { return now }
		replacement, err := service.BeginTOTPEnrollment(ctx, input)
		require.NoError(t, err)
		require.NotEqual(t, initial.Secret, replacement.Secret)
		return now
	}
	_, err = service.ActivateTOTP(ctx, ActivateMFAInput{UserID: 1, SessionID: 1, Code: code})
	assert.Error(t, err, "a code for a replaced secret must not activate its successor")
	status, err := service.MFAStatusFor(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "pending", status.Status)
	assert.Zero(t, status.RecoveryCodesRemaining)
}
