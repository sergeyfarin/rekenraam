package app

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sqlite "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/totp"
)

var expiryAuditStart = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func expiryAuditClock(service *AuthService) *atomic.Int64 {
	clock := new(atomic.Int64)
	clock.Store(expiryAuditStart.Unix())
	service.now = func() time.Time { return time.Unix(clock.Load(), 0).UTC() }
	return clock
}

// Real pool admission, not a sleep or a production scheduling hook.
func expiryAuditPoolWait(t *testing.T, database *sql.DB, clock *atomic.Int64, deadline time.Time, run func() error) error {
	t.Helper()
	connection, err := database.Conn(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	before := database.Stats().WaitCount
	result := make(chan error, 1)
	go func() { result <- run() }()
	require.Eventually(t, func() bool { return database.Stats().WaitCount > before }, 5*time.Second, time.Millisecond)
	clock.Store(deadline.Unix())
	require.NoError(t, connection.Close())
	select {
	case err := <-result:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("pool-queued consumer did not finish")
		return nil
	}
}

// Compile the chosen statement behind an independently held WAL writer. The
// native authorizer only observes execution; it never alters SQL or verdicts.
func expiryAuditWriterWait(t *testing.T, database *sql.DB, clock *atomic.Int64, deadline time.Time, table string, run func() error) error {
	t.Helper()
	ctx := context.Background()
	var sequence int
	var name, path string
	require.NoError(t, database.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path))
	other, err := db.Open(ctx, db.SQLiteFileURL(path))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, other.Close()) })
	reached, release := make(chan struct{}), make(chan struct{})
	var once, unblock sync.Once
	defer unblock.Do(func() { close(release) })
	connection, err := database.Conn(ctx)
	require.NoError(t, err)
	require.NoError(t, connection.Raw(func(raw any) error {
		raw.(*sqlite.SQLiteConn).RegisterAuthorizer(func(operation int, target, column, catalog string) int {
			if (operation == sqlite.SQLITE_UPDATE || operation == sqlite.SQLITE_INSERT) && target == table {
				once.Do(func() { close(reached); <-release })
			}
			return sqlite.SQLITE_OK
		})
		return nil
	}))
	require.NoError(t, connection.Close())
	result := make(chan error, 1)
	go func() { result <- run() }()
	select {
	case <-reached:
	case <-time.After(10 * time.Second):
		t.Fatal("consumer did not reach chosen write")
	}
	writer, err := other.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = writer.Rollback() }()
	_, err = writer.Exec(`UPDATE users SET username=username WHERE id=1`)
	require.NoError(t, err)
	unblock.Do(func() { close(release) })
	clock.Store(deadline.Unix())
	require.NoError(t, writer.Commit())
	select {
	case err := <-result:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("writer-queued consumer did not finish")
		return nil
	}
}

func TestAuthExpiryAuditSessionPoolAdmission(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, service := newAuthEventTestService(t, expiryAuditStart)
	login, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery"})
	require.NoError(t, err)
	deadline := expiryAuditStart.Add(time.Second)
	_, err = database.Exec(`UPDATE auth_sessions SET expires_at=?`, deadline.Format(time.RFC3339))
	require.NoError(t, err)
	clock := expiryAuditClock(service)
	before, err := service.Session(ctx, login.SessionToken)
	require.NoError(t, err)
	require.True(t, before.Authenticated)
	var status SessionStatus
	err = expiryAuditPoolWait(t, database, clock, deadline, func() error {
		var err error
		status, err = service.Session(ctx, login.SessionToken)
		return err
	})
	require.NoError(t, err)
	assert.False(t, status.Authenticated, "expiry is exclusive even when the read queued before the deadline")
}

func TestAuthExpiryAuditTrustedDevicePoolAdmission(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, service := newAuthEventTestService(t, expiryAuditStart)
	login, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery"})
	require.NoError(t, err)
	deadline := expiryAuditStart.Add(time.Second)
	_, err = database.Exec(`UPDATE login_trusted_devices SET expires_at=?`, deadline.Format(time.RFC3339))
	require.NoError(t, err)
	// Shared username scope is full, but the still-valid device has its own budget.
	_, err = database.Exec(`INSERT INTO login_throttles(scope_type,scope_key,failed_attempts,blocked_until,updated_at) VALUES('username','owner',0,?,?)`, expiryAuditStart.Add(time.Hour).Format(time.RFC3339), expiryAuditStart.Format(time.RFC3339))
	require.NoError(t, err)
	clock := expiryAuditClock(service)
	_, trusted := service.resolveTrustedDevice(ctx, login.TrustedDeviceToken, "owner")
	require.True(t, trusted)
	sessions := countRows(t, database, `SELECT count(*) FROM auth_sessions`)
	err = expiryAuditPoolWait(t, database, clock, deadline, func() error {
		_, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", TrustedDeviceToken: login.TrustedDeviceToken})
		return err
	})
	require.ErrorIs(t, err, ErrRateLimited, "an expired device cannot select the private throttle budget")
	assert.Equal(t, sessions, countRows(t, database, `SELECT count(*) FROM auth_sessions`))
	var expires string
	require.NoError(t, database.QueryRow(`SELECT expires_at FROM login_trusted_devices`).Scan(&expires))
	assert.Equal(t, deadline.Format(time.RFC3339), expires, "refusal cannot renew the bypass")
}

func TestAuthExpiryAuditMFAChallengeAdmission(t *testing.T) {
	t.Parallel()
	for _, boundary := range []string{"pool", "writer"} {
		t.Run(boundary, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			database, service := newMFATestService(t, expiryAuditStart)
			secret := enrollAndActivate(t, service, expiryAuditStart)
			challenge, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery"})
			require.NoError(t, err)
			deadline := expiryAuditStart.Add(time.Second)
			_, err = database.Exec(`UPDATE login_mfa_challenges SET expires_at=?`, deadline.Format(time.RFC3339))
			require.NoError(t, err)
			clock := expiryAuditClock(service)
			code, err := totp.Code(secret, totp.Step(expiryAuditStart))
			require.NoError(t, err)
			run := func() error {
				result, err := service.CompleteLoginMFA(ctx, CompleteMFALoginInput{ChallengeToken: challenge.MFAChallengeToken, Code: code})
				assert.True(t, result.SessionToken == "", "refused challenge discloses no session")
				return err
			}
			if boundary == "pool" {
				err = expiryAuditPoolWait(t, database, clock, deadline, run)
			} else {
				err = expiryAuditWriterWait(t, database, clock, deadline, "login_mfa_challenges", run)
			}
			require.ErrorIs(t, err, ErrMFAChallengeInvalid)
			assert.Zero(t, countRows(t, database, `SELECT count(*) FROM auth_sessions`))
			assert.Zero(t, countRows(t, database, `SELECT count(*) FROM login_trusted_devices`))
			assert.Zero(t, countAuthEvents(t, service, authEventLoginSucceeded, ""))
		})
	}
}

func TestAuthExpiryAuditTOTPWriterAdmission(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"login", "activate"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			database, service := newMFATestService(t, expiryAuditStart)
			var secret, challenge string
			if operation == "login" {
				secret = enrollAndActivate(t, service, expiryAuditStart)
				login, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery"})
				require.NoError(t, err)
				challenge = login.MFAChallengeToken
			} else {
				enrollment, err := service.BeginTOTPEnrollment(ctx, rightPassword(1))
				require.NoError(t, err)
				secret = enrollment.Secret
			}
			// Current code's exact cutoff includes the one-step skew allowance.
			deadline := expiryAuditStart.Add(2 * totp.Period)
			code, err := totp.Code(secret, totp.Step(expiryAuditStart))
			require.NoError(t, err)
			clock := expiryAuditClock(service)
			err = expiryAuditWriterWait(t, database, clock, deadline, "user_mfa_totp", func() error {
				if operation == "activate" {
					_, err := service.ActivateTOTP(ctx, ActivateMFAInput{UserID: 1, SessionID: 1, Code: code})
					return err
				}
				_, err := service.CompleteLoginMFA(ctx, CompleteMFALoginInput{ChallengeToken: challenge, Code: code})
				return err
			})
			require.ErrorIs(t, err, ErrMFACodeInvalid, "a code outside the skew window at writer admission must not authenticate")
			assert.Zero(t, countRows(t, database, `SELECT count(*) FROM auth_sessions`))
			if operation == "activate" {
				assert.Equal(t, 1, countRows(t, database, `SELECT count(*) FROM user_mfa_totp WHERE status='pending'`))
			}
		})
	}
}

func TestAuthExpiryAuditThrottlePoolBoundary(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"login", "settings"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			database, service := newMFATestService(t, expiryAuditStart)
			scopeType, scopeKey := "username", "owner"
			if operation == "settings" {
				scopeType, scopeKey = "reauth_session", "1"
			}
			deadline := expiryAuditStart.Add(time.Second)
			_, err := database.Exec(`INSERT INTO login_throttles(scope_type,scope_key,failed_attempts,blocked_until,updated_at) VALUES(?,?,0,?,?)`, scopeType, scopeKey, deadline.Format(time.RFC3339), expiryAuditStart.Format(time.RFC3339))
			require.NoError(t, err)
			clock := expiryAuditClock(service)
			err = expiryAuditPoolWait(t, database, clock, deadline, func() error {
				if operation == "settings" {
					_, err := service.BeginTOTPEnrollment(ctx, rightPassword(1))
					return err
				}
				_, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery"})
				return err
			})
			require.NoError(t, err, "a block ending exactly at admission no longer blocks the owner")
		})
	}
}

func TestAuthExpiryAuditDisableBeforeSessionCommit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, service := newMFATestService(t, expiryAuditStart)
	secret := enrollAndActivate(t, service, expiryAuditStart)
	challenge, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery"})
	require.NoError(t, err)
	code, err := totp.Code(secret, totp.Step(expiryAuditStart))
	require.NoError(t, err)
	var sequence int
	var name, path string
	require.NoError(t, database.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path))
	other, err := db.Open(ctx, db.SQLiteFileURL(path))
	require.NoError(t, err)
	defer func() { require.NoError(t, other.Close()) }()
	reached, release := make(chan struct{}), make(chan struct{})
	var once, unblock sync.Once
	defer unblock.Do(func() { close(release) })
	connection, err := database.Conn(ctx)
	require.NoError(t, err)
	require.NoError(t, connection.Raw(func(raw any) error {
		raw.(*sqlite.SQLiteConn).RegisterAuthorizer(func(operation int, table, column, catalog string) int {
			if operation == sqlite.SQLITE_INSERT && table == "auth_sessions" {
				once.Do(func() { close(reached); <-release })
			}
			return sqlite.SQLITE_OK
		})
		return nil
	}))
	require.NoError(t, connection.Close())
	completed := make(chan error, 1)
	go func() {
		select {
		case <-reached:
		case <-time.After(5 * time.Second):
			completed <- context.DeadlineExceeded
			return
		}
		replacement := *service
		replacement.repository = db.NewAuthRepository(other)
		completed <- replacement.DisableMFA(ctx, rightPassword(1))
		unblock.Do(func() { close(release) })
	}()
	result, err := service.CompleteLoginMFA(ctx, CompleteMFALoginInput{ChallengeToken: challenge.MFAChallengeToken, Code: code})
	require.NoError(t, <-completed)
	require.ErrorIs(t, err, ErrMFAChallengeInvalid, "disable cuts off the attempt before its session commits")
	assert.True(t, result.SessionToken == "")
	assert.Zero(t, countRows(t, database, `SELECT count(*) FROM auth_sessions`))
	assert.Zero(t, countRows(t, database, `SELECT count(*) FROM login_trusted_devices`))
}

func TestAuthExpiryAuditThrottleWriterAdmission(t *testing.T) {
	t.Parallel()
	database, service := newAuthEventTestService(t, expiryAuditStart)
	clock := expiryAuditClock(service)
	deadline := expiryAuditStart.Add(time.Second)
	err := expiryAuditWriterWait(t, database, clock, deadline, "login_throttles", func() error {
		_, err := service.Login(context.Background(), LoginInput{Username: "owner", Password: "wrong-password"})
		return err
	})
	require.ErrorIs(t, err, ErrInvalidCredentials)
	var updated string
	require.NoError(t, database.QueryRow(`SELECT updated_at FROM login_throttles WHERE scope_type='username'`).Scan(&updated))
	assert.Equal(t, deadline.Format(time.RFC3339), updated, "reservation and settlement cannot be backdated by a writer wait")
}

func TestAuthExpiryAuditMFAPositiveWriterControls(t *testing.T) {
	t.Parallel()
	for _, boundary := range []string{"challenge", "totp"} {
		t.Run(boundary, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			database, service := newMFATestService(t, expiryAuditStart)
			secret := enrollAndActivate(t, service, expiryAuditStart)
			challenge, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery"})
			require.NoError(t, err)
			table := "login_mfa_challenges"
			instant := expiryAuditStart.Add(time.Second)
			if boundary == "totp" {
				table = "user_mfa_totp"
				instant = expiryAuditStart.Add(2*totp.Period - time.Second)
			}
			code, err := totp.Code(secret, totp.Step(expiryAuditStart))
			require.NoError(t, err)
			clock := expiryAuditClock(service)
			err = expiryAuditWriterWait(t, database, clock, instant, table, func() error {
				result, err := service.CompleteLoginMFA(ctx, CompleteMFALoginInput{ChallengeToken: challenge.MFAChallengeToken, Code: code})
				assert.True(t, result.SessionToken != "")
				return err
			})
			require.NoError(t, err, "writer serialization must still admit proof within its deadline")
			assert.Equal(t, 1, countRows(t, database, `SELECT count(*) FROM auth_sessions`))
		})
	}
}

func TestAuthExpiryAuditExpiredChallengeWithEventFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, service := newMFATestService(t, expiryAuditStart)
	secret := enrollAndActivate(t, service, expiryAuditStart)
	challenge, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery"})
	require.NoError(t, err)
	code, err := totp.Code(secret, totp.Step(expiryAuditStart))
	require.NoError(t, err)
	clock := expiryAuditClock(service)
	injectInsertFailure(t, database, "authentication_events")
	err = expiryAuditPoolWait(t, database, clock, expiryAuditStart.Add(mfaChallengeLifetime), func() error {
		_, err := service.CompleteLoginMFA(ctx, CompleteMFALoginInput{ChallengeToken: challenge.MFAChallengeToken, Code: code})
		return err
	})
	require.ErrorIs(t, err, ErrMFAChallengeInvalid)
	assert.Zero(t, countRows(t, database, `SELECT count(*) FROM auth_sessions`))
	assert.Zero(t, countRows(t, database, `SELECT count(*) FROM login_trusted_devices`))
	removeInjectedFailure(t, database)
}

func TestAuthExpiryAuditConcurrentChallengeIsSingleUse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, service := newMFATestService(t, expiryAuditStart)
	enrollAndActivate(t, service, expiryAuditStart)
	codes, err := service.RegenerateRecoveryCodes(ctx, rightPassword(1))
	require.NoError(t, err)
	challenge, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery"})
	require.NoError(t, err)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, code := range codes[:2] {
		go func() {
			<-start
			_, err := service.CompleteLoginMFA(ctx, CompleteMFALoginInput{ChallengeToken: challenge.MFAChallengeToken, Code: code})
			results <- err
		}()
	}
	close(start)
	successes := 0
	for range 2 {
		if err := <-results; err == nil {
			successes++
		} else {
			assert.ErrorIs(t, err, ErrMFAChallengeInvalid)
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, countRows(t, database, `SELECT count(*) FROM auth_sessions`))
	assert.Equal(t, 1, countRows(t, database, `SELECT count(*) FROM login_trusted_devices`))
	assert.Equal(t, 1, countAuthEvents(t, service, authEventLoginSucceeded, ""))
}

func TestAuthExpiryAuditTrustedDeviceAdmittedBeforeExpiry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, service := newAuthEventTestService(t, expiryAuditStart)
	login, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery"})
	require.NoError(t, err)
	deadline := expiryAuditStart.Add(time.Second)
	_, err = database.Exec(`UPDATE login_trusted_devices SET expires_at=?`, deadline.Format(time.RFC3339))
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO login_throttles(scope_type,scope_key,failed_attempts,blocked_until,updated_at) VALUES('username','owner',0,?,?)`, expiryAuditStart.Add(time.Hour).Format(time.RFC3339), expiryAuditStart.Format(time.RFC3339))
	require.NoError(t, err)
	clock := expiryAuditClock(service)
	err = expiryAuditWriterWait(t, database, clock, deadline, "auth_sessions", func() error {
		_, err := service.Login(ctx, LoginInput{Username: "owner", Password: "correct-horse-battery", TrustedDeviceToken: login.TrustedDeviceToken})
		return err
	})
	require.NoError(t, err, "device admission before expiry retains its private budget while sign-in finishes")
	assert.Equal(t, 2, countRows(t, database, `SELECT count(*) FROM auth_sessions`))
	assert.Equal(t, 1, countRows(t, database, `SELECT count(*) FROM login_trusted_devices`), "renew the same device identity")
}
