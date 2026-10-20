package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	sqlite "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/totp"
)

// The original failing-before #360/#366 component probe, retained by name.
func TestAuthExpiryAuditReservationRolloverProbe(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, service := newAuthEventTestService(t, expiryAuditStart)
	clock := expiryAuditClock(service)
	scopes := []loginThrottleScope{{ScopeType: throttleScopeUsername, ScopeKey: "owner"}}
	old := make([]*throttleReservation, 0, loginThrottleMaxFailures)
	for range loginThrottleMaxFailures {
		r, err := service.reserveAttempt(ctx, scopes)
		require.NoError(t, err)
		old = append(old, r)
	}
	clock.Store(expiryAuditStart.Add(loginThrottleWindow).Unix())
	for range loginThrottleMaxFailures {
		_, err := service.reserveAttempt(ctx, scopes)
		require.NoError(t, err)
	}
	old[0].release(ctx)
	_, err := service.reserveAttempt(ctx, scopes)
	require.ErrorIs(t, err, ErrRateLimited, "old reservation release must not refund a new window's place")
}

// Pause an actual consumer after admission, before it reads credentials. Each
// gets a normal single-connection native SQLite pool on the SAME WAL database,
// allowing real replacement consumers to reserve while the old one is stalled.
// The authorizer observes preparation only; it never changes SQL or verdicts.
func throttleAuditPause(t *testing.T, database *sql.DB, service *AuthService, table, column string, run func(*AuthService) error, skipReads ...int) func() error {
	t.Helper()
	var sequence int
	var name, path string
	require.NoError(t, database.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path))
	other, err := db.Open(context.Background(), db.SQLiteFileURL(path))
	require.NoError(t, err)
	clone := *service
	clone.repository = db.NewAuthRepository(other)
	reached, release := make(chan struct{}), make(chan struct{})
	var once, unblock sync.Once
	remaining := 0
	if len(skipReads) > 0 {
		remaining = skipReads[0]
	}
	t.Cleanup(func() { unblock.Do(func() { close(release) }); require.NoError(t, other.Close()) })
	connection, err := other.Conn(context.Background())
	require.NoError(t, err)
	require.NoError(t, connection.Raw(func(raw any) error {
		raw.(*sqlite.SQLiteConn).RegisterAuthorizer(func(operation int, target, field, catalog string) int {
			if operation == sqlite.SQLITE_READ && target == table && field == column {
				if remaining > 0 {
					remaining--
					return sqlite.SQLITE_OK
				}
				once.Do(func() { close(reached); <-release })
			}
			return sqlite.SQLITE_OK
		})
		return nil
	}))
	require.NoError(t, connection.Close())
	result := make(chan error, 1)
	go func() { result <- run(&clone) }()
	select {
	case <-reached:
	case err := <-result:
		t.Fatalf("consumer finished before credential barrier: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("consumer did not reach credential barrier")
	}
	return func() error {
		unblock.Do(func() { close(release) })
		select {
		case err := <-result:
			return err
		case <-time.After(10 * time.Second):
			t.Fatal("consumer did not finish after credential barrier")
			return nil
		}
	}
}

func throttleAuditSnapshot(t *testing.T, database *sql.DB) []string {
	t.Helper()
	rows, err := database.Query(`SELECT scope_type || ':' || scope_key || ':' || failed_attempts || ':' || COALESCE(blocked_until,'') || ':' || updated_at || ':' || generation FROM login_throttles ORDER BY scope_type,scope_key`)
	require.NoError(t, err)
	defer rows.Close()
	result := []string{}
	for rows.Next() {
		var row string
		require.NoError(t, rows.Scan(&row))
		result = append(result, row)
	}
	require.NoError(t, rows.Err())
	return result
}

func TestThrottleRolloverRealConsumersPreserveReplacementBudget(t *testing.T) {
	t.Parallel()
	for _, consumer := range []string{"login", "device", "settings", "mfa_password_release", "internal_error_release", "activation", "mfa_code"} {
		for _, verdict := range []string{"failure", "success"} {
			if (consumer == "mfa_password_release" || consumer == "internal_error_release") && verdict == "failure" {
				continue
			}
			t.Run(consumer+"/"+verdict, func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				database, service := newMFATestService(t, expiryAuditStart)
				var challengeToken, factorCode string
				if consumer == "mfa_password_release" || consumer == "mfa_code" {
					secret := enrollAndActivate(t, service, expiryAuditStart)
					if consumer == "mfa_code" {
						login, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery"})
						require.NoError(t, err)
						challengeToken = login.MFAChallengeToken
						factorCode, err = totp.Code(secret, totp.Step(expiryAuditStart.Add(loginThrottleWindow)))
						require.NoError(t, err)
					}
				}
				var deviceToken string
				if consumer == "device" {
					login, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery"})
					require.NoError(t, err)
					deviceToken = login.TrustedDeviceToken
				}
				var activationCode string
				if consumer == "activation" {
					enrollment, err := service.BeginTOTPEnrollment(ctx, MFAPasswordInput{UserID: 1, SessionID: 1, Password: "correct-horse-battery"})
					require.NoError(t, err)
					activationCode, err = totp.Code(enrollment.Secret, totp.Step(expiryAuditStart.Add(loginThrottleWindow)))
					require.NoError(t, err)
				}
				clock := expiryAuditClock(service)
				scopes := loginThrottleScopes("owner", "203.0.113.40")
				if consumer == "device" {
					scopes = []loginThrottleScope{{ScopeType: throttleScopeTrustedDevice, ScopeKey: "1"}}
				}
				if consumer == "settings" || consumer == "activation" {
					scopes = []loginThrottleScope{{ScopeType: throttleScopeReauthSession, ScopeKey: "1"}}
				}
				guess := func(s *AuthService) error {
					if consumer == "settings" || consumer == "activation" {
						_, err := s.BeginTOTPEnrollment(ctx, MFAPasswordInput{UserID: 1, SessionID: 1, Password: "wrong-password"})
						return err
					}
					_, err := s.Login(ctx, LoginInput{Username: "owner", Password: "wrong-password", ClientIP: "203.0.113.40", TrustedDeviceToken: deviceToken})
					return err
				}
				oldRun := func(s *AuthService) error {
					password := "wrong-password"
					if verdict == "success" {
						password = "correct-horse-battery"
					}
					if consumer == "mfa_code" {
						code := "not-a-code"
						if verdict == "success" {
							code = factorCode
						}
						_, err := s.CompleteLoginMFA(ctx, CompleteMFALoginInput{ChallengeToken: challengeToken, Code: code, ClientIP: "203.0.113.40"})
						return err
					}
					if consumer == "activation" {
						code := "not-a-code"
						if verdict == "success" {
							code = activationCode
						}
						_, err := s.ActivateTOTP(ctx, ActivateMFAInput{UserID: 1, SessionID: 1, Code: code})
						return err
					}
					if consumer == "settings" {
						_, err := s.BeginTOTPEnrollment(ctx, MFAPasswordInput{UserID: 1, SessionID: 1, Password: password})
						return err
					}
					login, err := s.Login(ctx, LoginInput{Username: "owner", Password: password, ClientIP: "203.0.113.40", TrustedDeviceToken: deviceToken})
					if consumer == "mfa_password_release" && err == nil {
						assert.True(t, login.MFARequired)
						assert.Empty(t, login.SessionToken)
					}
					return err
				}
				table, column := "users", "password_hash"
				if consumer == "activation" || consumer == "mfa_code" {
					table, column = "user_mfa_totp", "secret_ciphertext"
				}
				skip := 0
				if consumer == "device" {
					skip = 1
				} // device identity lookup precedes admission
				resumeOld := throttleAuditPause(t, database, service, table, column, oldRun, skip)
				for range loginThrottleMaxFailures - 1 {
					_, err := service.reserveAttempt(ctx, scopes)
					require.NoError(t, err)
				}
				clock.Store(expiryAuditStart.Add(loginThrottleWindow).Unix())
				resumes := make([]func() error, 0, loginThrottleMaxFailures)
				for range loginThrottleMaxFailures {
					resumes = append(resumes, throttleAuditPause(t, database, service, "users", "password_hash", guess, skip))
				}
				before := throttleAuditSnapshot(t, database)
				verifiedBefore := countAuthEvents(t, service, authEventLoginFailed, authFailureInvalidCredentials)
				var savedHash string
				if consumer == "internal_error_release" {
					require.NoError(t, database.QueryRow(`SELECT password_hash FROM users WHERE id=1`).Scan(&savedHash))
					_, err := database.Exec(`UPDATE users SET password_hash='invalid hash' WHERE id=1`)
					require.NoError(t, err)
				}
				err := resumeOld()
				switch {
				case consumer == "internal_error_release":
					require.ErrorContains(t, err, "verify password")
					_, err = database.Exec(`UPDATE users SET password_hash=? WHERE id=1`, savedHash)
					require.NoError(t, err)
				case consumer == "mfa_code" && verdict == "success":
					require.ErrorIs(t, err, ErrMFAChallengeInvalid, "factor proof cannot extend challenge expiry")
					assert.Zero(t, countRows(t, database, `SELECT COUNT(*) FROM auth_sessions`))
				case verdict == "success":
					require.NoError(t, err)
				case consumer == "activation" || consumer == "mfa_code":
					require.ErrorIs(t, err, ErrMFACodeInvalid)
				default:
					require.ErrorIs(t, err, ErrInvalidCredentials)
				}
				assert.Equal(t, before, throttleAuditSnapshot(t, database), "a late verdict must leave every replacement row unchanged")
				assert.ErrorIs(t, guess(service), ErrRateLimited, "sixth replacement guess must be refused before verification")
				for _, resume := range resumes {
					err := resume()
					require.True(t, err != nil && (errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrRateLimited)), "replacement verdict: %v", err)
				}
				verified := countAuthEvents(t, service, authEventLoginFailed, authFailureInvalidCredentials) - verifiedBefore
				if verdict == "failure" && consumer != "activation" && consumer != "mfa_code" {
					verified--
				} // separately admitted old guess
				assert.Equal(t, loginThrottleMaxFailures, verified, "exactly five replacement password guesses verified, regardless of returned statuses")
			})
		}
	}
}

func TestThrottleRolloverMissingRowsAndSuccessReset(t *testing.T) {
	t.Parallel()
	for _, scopeType := range []string{throttleScopeUsername, throttleScopeClientIP, throttleScopeTrustedDevice, throttleScopeReauthSession} {
		for _, settlement := range []string{"failure", "success", "release"} {
			t.Run(scopeType+"/"+settlement, func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				database, service := newAuthEventTestService(t, expiryAuditStart)
				scopes := []loginThrottleScope{{ScopeType: scopeType, ScopeKey: "isolated"}}
				old, err := service.reserveAttempt(ctx, scopes)
				require.NoError(t, err)
				reset, err := service.reserveAttempt(ctx, scopes)
				require.NoError(t, err)
				// IP never resets on success; delete models a missing row independently.
				if scopeType == throttleScopeClientIP {
					_, err = database.Exec(`DELETE FROM login_throttles`)
					require.NoError(t, err)
				} else {
					require.NoError(t, reset.clear(ctx))
				}
				require.Empty(t, throttleAuditSnapshot(t, database))
				settle := func(r *throttleReservation) {
					switch settlement {
					case "failure":
						wait, err := r.fail(ctx)
						require.NoError(t, err)
						require.Zero(t, wait)
					case "success":
						require.NoError(t, r.clear(ctx))
					case "release":
						r.release(ctx)
						r.release(ctx)
					}
				}
				// A late failure must not resurrect a success-cleared generation.
				settle(old)
				require.Empty(t, throttleAuditSnapshot(t, database))
				// Repeat with a replacement at the EXACT same clock time: identity must
				// not be based on timestamps or a counter reset by row deletion.
				old, err = service.reserveAttempt(ctx, scopes)
				require.NoError(t, err)
				_, err = database.Exec(`DELETE FROM login_throttles`)
				require.NoError(t, err)
				for range loginThrottleMaxFailures {
					_, err = service.reserveAttempt(ctx, scopes)
					require.NoError(t, err)
				}
				before := throttleAuditSnapshot(t, database)
				settle(old)
				require.Equal(t, before, throttleAuditSnapshot(t, database))
				_, err = service.reserveAttempt(ctx, scopes)
				require.ErrorIs(t, err, ErrRateLimited)
			})
		}
	}
}

func TestThrottleRolloverExpiredBlockAndLateFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, service := newAuthEventTestService(t, expiryAuditStart)
	clock := expiryAuditClock(service)
	scopes := loginThrottleScopes("owner", "203.0.113.41")
	old, err := service.reserveAttempt(ctx, scopes)
	require.NoError(t, err)
	var last *throttleReservation
	for range loginThrottleMaxFailures - 1 {
		last, err = service.reserveAttempt(ctx, scopes)
		require.NoError(t, err)
	}
	wait, err := last.fail(ctx)
	require.NoError(t, err)
	require.Equal(t, loginThrottleWindow, wait)
	clock.Store(expiryAuditStart.Add(loginThrottleWindow).Unix())
	before := throttleAuditSnapshot(t, database)
	wait, err = old.fail(ctx)
	require.NoError(t, err)
	require.Zero(t, wait)
	require.Equal(t, before, throttleAuditSnapshot(t, database), "late failure cannot resurrect an expired block")
	for range loginThrottleMaxFailures {
		_, err = service.reserveAttempt(ctx, scopes)
		require.NoError(t, err)
	}
	old.release(ctx) // already settled; never refunds again
	_, err = service.reserveAttempt(ctx, scopes)
	require.ErrorIs(t, err, ErrRateLimited)
}

func TestThrottleRolloverPartialAdmissionRefundIsFenced(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, service := newAuthEventTestService(t, expiryAuditStart)
	scopes := loginThrottleScopes("owner", "203.0.113.42")
	// Fill only IP; username admission succeeds, IP refuses, username must be
	// refunded while the IP row stays intact.
	for range loginThrottleMaxFailures {
		_, err := service.reserveAttempt(ctx, scopes[1:])
		require.NoError(t, err)
	}
	_, err := service.reserveAttempt(ctx, scopes)
	require.ErrorIs(t, err, ErrRateLimited)
	var attempts int
	require.NoError(t, database.QueryRow(`SELECT failed_attempts FROM login_throttles WHERE scope_type='username'`).Scan(&attempts))
	assert.Zero(t, attempts)
	require.NoError(t, database.QueryRow(`SELECT failed_attempts FROM login_throttles WHERE scope_type='client_ip'`).Scan(&attempts))
	assert.Equal(t, loginThrottleMaxFailures, attempts)
}

func TestThrottleResetRealConsumersDoNotResurrectMissingRows(t *testing.T) {
	t.Parallel()
	for _, consumer := range []string{"login", "settings"} {
		for _, success := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/success=%t", consumer, success), func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				database, service := newMFATestService(t, expiryAuditStart)
				resume := throttleAuditPause(t, database, service, "users", "password_hash", func(s *AuthService) error {
					password := "wrong-password"
					if success {
						password = "correct-horse-battery"
					}
					if consumer == "settings" {
						_, err := s.BeginTOTPEnrollment(ctx, MFAPasswordInput{UserID: 1, SessionID: 1, Password: password})
						return err
					}
					_, err := s.Login(ctx, LoginInput{Username: "owner", Password: password, ClientIP: "203.0.113.43"})
					return err
				})
				// Model a concurrent successful reset (or operator cleanup), after this
				// consumer was admitted but before its verdict.
				_, err := database.Exec(`DELETE FROM login_throttles`)
				require.NoError(t, err)
				err = resume()
				if success {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, ErrInvalidCredentials)
				}
				assert.Empty(t, throttleAuditSnapshot(t, database))
				if !success {
					assert.Equal(t, 1, countAuthEvents(t, service, authEventLoginFailed, authFailureInvalidCredentials), "the wrong guess remains observable")
				}
			})
		}
	}
}
