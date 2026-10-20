package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/totp"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signedInSession signs the owner in with the password alone and returns the
// real session id, so security events can be checked against it.
func signedInSession(t *testing.T, service *AuthService, clientIP string) (int64, LoginResult) {
	t.Helper()
	ctx := context.Background()

	result, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: clientIP})
	require.NoError(t, err)
	require.NotEmpty(t, result.SessionToken)
	status, err := service.Session(ctx, result.SessionToken)
	require.NoError(t, err)
	require.True(t, status.Authenticated)

	return status.SessionID, result
}

func eventsOfType(t *testing.T, service *AuthService, eventType string) []AuthenticationEvent {
	t.Helper()

	all, err := service.AuthenticationEvents(context.Background(), 200)
	require.NoError(t, err)
	var matching []AuthenticationEvent
	for _, event := range all.Events {
		if event.EventType == eventType {
			matching = append(matching, event)
		}
	}

	return matching
}

func requireOneAttributedEvent(t *testing.T, service *AuthService, eventType string, sessionID int64, clientIP string, requestID string) AuthenticationEvent {
	t.Helper()

	events := eventsOfType(t, service, eventType)
	require.Lenf(t, events, 1, "%s must be recorded exactly once", eventType)
	event := events[0]
	assert.Equal(t, "success", event.Outcome)
	assert.Equal(t, "owner", event.Username)
	require.NotNil(t, event.UserID)
	assert.Equal(t, int64(1), *event.UserID)
	require.NotNilf(t, event.AuthSessionID, "%s must name the session that made it", eventType)
	assert.Equal(t, sessionID, *event.AuthSessionID)
	assert.Equal(t, clientIP, event.ClientIP)
	assert.Equal(t, requestID, event.RequestID)
	assert.Equal(t, "browser_api", event.Origin)
	assert.Empty(t, event.SecondFactor)

	return event
}

// #288: every account-security change made from the app is one attributed
// event naming the user, the session, the client address and the request —
// and none of them carries the secret or codes the change issued.
func TestAccountSecurityChangesAreRecordedAsAttributedEvents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	_, service := newMFATestService(t, now)
	const clientIP = "198.51.100.4"
	sessionID, login := signedInSession(t, service, clientIP)
	confirm := func(requestID string) MFAPasswordInput {
		return MFAPasswordInput{UserID: 1, SessionID: sessionID, Password: "correct-horse-battery", ClientIP: clientIP, RequestID: requestID}
	}

	enrollment, err := service.BeginTOTPEnrollment(ctx, confirm("req-enroll"))
	require.NoError(t, err)
	requireOneAttributedEvent(t, service, authEventMFAEnrollmentStarted, sessionID, clientIP, "req-enroll")

	code, err := totp.Code(enrollment.Secret, totp.Step(now)-1)
	require.NoError(t, err)
	firstCodes, err := service.ActivateTOTP(ctx, ActivateMFAInput{UserID: 1, SessionID: sessionID, Code: code, ClientIP: clientIP, RequestID: "req-activate"})
	require.NoError(t, err)
	requireOneAttributedEvent(t, service, authEventMFAActivated, sessionID, clientIP, "req-activate")

	secondCodes, err := service.RegenerateRecoveryCodes(ctx, confirm("req-regenerate"))
	require.NoError(t, err)
	requireOneAttributedEvent(t, service, authEventMFARecoveryCodesRegenerated, sessionID, clientIP, "req-regenerate")

	wrong := confirm("req-disable-wrong")
	wrong.Password = "wrong-password"
	require.ErrorIs(t, service.DisableMFA(ctx, wrong), ErrInvalidCredentials)
	assert.Empty(t, eventsOfType(t, service, authEventMFADisabled), "a refused disable is a failed confirmation, not a disable")
	require.NoError(t, service.DisableMFA(ctx, confirm("req-disable")))
	disabled := requireOneAttributedEvent(t, service, authEventMFADisabled, sessionID, clientIP, "req-disable")
	assert.JSONEq(t, `{"previous_mfa_status":"active"}`, disabled.MetadataJSON)

	devices, err := service.ListTrustedDevices(ctx, 1, login.TrustedDeviceToken)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	require.NoError(t, service.RevokeTrustedDevice(ctx, RevokeTrustedDeviceInput{
		UserID: 1, SessionID: sessionID, DeviceID: devices[0].ID, ClientIP: clientIP, RequestID: "req-revoke",
	}))
	revoked := requireOneAttributedEvent(t, service, authEventTrustedDeviceRevoked, sessionID, clientIP, "req-revoke")
	assert.JSONEq(t, `{"trusted_device_id":`+jsonInt(devices[0].ID)+`}`, revoked.MetadataJSON)
	require.ErrorIs(t, service.RevokeTrustedDevice(ctx, RevokeTrustedDeviceInput{UserID: 1, SessionID: sessionID, DeviceID: devices[0].ID}), ErrTrustedDeviceNotFound)
	assert.Len(t, eventsOfType(t, service, authEventTrustedDeviceRevoked), 1, "a refused revocation records nothing")

	all, err := service.AuthenticationEvents(ctx, 200)
	require.NoError(t, err)
	serialized, err := json.Marshal(all)
	require.NoError(t, err)
	for _, secret := range append(append([]string{enrollment.Secret, code, login.SessionToken, login.TrustedDeviceToken}, firstCodes...), secondCodes...) {
		assert.NotContains(t, string(serialized), secret, "an authentication event must never carry a secret, code or token")
	}
}

func jsonInt(value int64) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

// #288: a sign-in completed by a recovery code is told apart from a TOTP one,
// and a password-only sign-in names no second factor.
func TestRecoveryCodeSignInIsDistinguishableFromTOTPSignIn(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	_, service := newMFATestService(t, now)
	sessionID, _ := signedInSession(t, service, "198.51.100.4")

	enrollment, err := service.BeginTOTPEnrollment(ctx, MFAPasswordInput{UserID: 1, SessionID: sessionID, Password: "correct-horse-battery"})
	require.NoError(t, err)
	activationCode, err := totp.Code(enrollment.Secret, totp.Step(now)-1)
	require.NoError(t, err)
	recoveryCodes, err := service.ActivateTOTP(ctx, ActivateMFAInput{UserID: 1, SessionID: sessionID, Code: activationCode})
	require.NoError(t, err)

	signInWith := func(code string, requestID string) {
		t.Helper()
		challenge, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", ClientIP: "203.0.113.9"})
		require.NoError(t, err)
		_, err = service.CompleteLoginMFA(ctx, CompleteMFALoginInput{
			ChallengeToken: challenge.MFAChallengeToken, Code: code, ClientIP: "203.0.113.9", RequestID: requestID,
		})
		require.NoError(t, err)
	}
	totpCode, err := totp.Code(enrollment.Secret, totp.Step(now))
	require.NoError(t, err)
	signInWith(totpCode, "req-totp")
	signInWith(recoveryCodes[0], "req-recovery")

	factors := map[string]string{}
	for _, event := range eventsOfType(t, service, authEventLoginSucceeded) {
		require.NotNil(t, event.AuthSessionID)
		factors[event.RequestID] = event.SecondFactor
	}
	assert.Equal(t, map[string]string{
		"":             "",
		"req-totp":     secondFactorTOTP,
		"req-recovery": secondFactorRecoveryCode,
	}, factors)
}

// #288: recover-owner runs from the CLI, so its record is the event it writes
// in the reset's own transaction, not the command's stdout.
func TestOwnerRecoveryIsRecordedAsOneCLIEvent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	database, service := newMFATestService(t, now)
	signedInSession(t, service, "198.51.100.4")
	enrollAndActivate(t, service, now)

	recovery := NewRecoveryService(database, "file:test.sqlite", db.NewRecoveryRepository(database))
	recovery.now = func() time.Time { return now }
	require.NoError(t, recovery.ResetOwnerAccess(ctx, ResetOwnerAccessInput{
		Password: "a-brand-new-password", BackupPath: "/backups/rekenraam.recovery.sqlite",
	}))

	events := eventsOfType(t, service, "owner_access_recovered")
	require.Len(t, events, 1)
	event := events[0]
	assert.Equal(t, "cli_recovery", event.Origin)
	assert.Equal(t, "success", event.Outcome)
	assert.Equal(t, "owner", event.Username)
	require.NotNil(t, event.UserID)
	assert.Equal(t, int64(1), *event.UserID)
	assert.Nil(t, event.AuthSessionID, "the CLI has no session")
	assert.Empty(t, event.ClientIP, "the CLI has no client address")
	assert.Equal(t, now.Format(time.RFC3339), event.OccurredAt)
	assert.JSONEq(t, `{"sessions_revoked":1,"previous_mfa_status":"active","backup_written":true,"tokens_revoked":0}`, event.MetadataJSON)
	assert.NotContains(t, event.MetadataJSON, "/backups", "the record says a backup exists, not where")
}

// #289: the daily cleanup deletes revoked and expired sessions, but the events
// made from a session must keep naming it afterwards, and the next login must
// never be handed the deleted session's id, or a kept event would point at
// the wrong session.
func TestSecurityEventsKeepTheirSessionAfterSessionCleanup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	database, service := newMFATestService(t, now)
	const clientIP = "198.51.100.4"

	sessionID, login := signedInSession(t, service, clientIP)
	_, err := service.BeginTOTPEnrollment(ctx, MFAPasswordInput{
		UserID: 1, SessionID: sessionID, Password: "correct-horse-battery", ClientIP: clientIP, RequestID: "req-enroll",
	})
	require.NoError(t, err)
	_, err = database.ExecContext(ctx, `
		INSERT INTO audit_events (book_id, actor_user_id, auth_session_id, occurred_at, origin_type, operation)
		VALUES (NULL, 1, ?, '2026-08-07T09:00:00Z', 'browser_api', 'settings.update')
	`, sessionID)
	require.NoError(t, err)

	// The signed-in session is the newest row, so a reusing rowid allocator
	// would hand its id straight to the next login once it is deleted.
	require.NoError(t, service.Logout(ctx, LogoutInput{Token: login.SessionToken, ClientIP: clientIP}))
	deleted, err := service.CleanupExpiredAndRevokedSessions(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	nextSessionID, _ := signedInSession(t, service, clientIP)
	assert.NotEqual(t, sessionID, nextSessionID, "a cleaned-up session id must never be reused")

	requireOneAttributedEvent(t, service, authEventMFAEnrollmentStarted, sessionID, clientIP, "req-enroll")
	logouts := eventsOfType(t, service, authEventLogout)
	require.Len(t, logouts, 1)
	require.NotNil(t, logouts[0].AuthSessionID)
	assert.Equal(t, sessionID, *logouts[0].AuthSessionID)
	var logins []int64
	for _, event := range eventsOfType(t, service, authEventLoginSucceeded) {
		require.NotNil(t, event.AuthSessionID)
		logins = append(logins, *event.AuthSessionID)
	}
	assert.ElementsMatch(t, []int64{sessionID, nextSessionID}, logins,
		"the enrollment still joins to the sign-in that created its session")

	var auditSessionID sql.NullInt64
	require.NoError(t, database.QueryRowContext(ctx,
		`SELECT auth_session_id FROM audit_events WHERE operation = 'settings.update'`).Scan(&auditSessionID))
	require.True(t, auditSessionID.Valid, "the audit trail must keep naming the session")
	assert.Equal(t, sessionID, auditSessionID.Int64)
}
