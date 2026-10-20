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
)

// #357: the independent oracle is the time at admission, not the time before
// waiting for the single writer connection. No production hook or wall-clock
// sleeps are used: pool WaitCount proves the request is queued.
func TestPersonalAccessTokenAuditExpiryWhileWaitingForDatabase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, service, input := tokenTestService(t)
	credential, plaintext, err := service.CreatePersonalAccessToken(ctx, input)
	require.NoError(t, err)
	start := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	deadline := start.Add(time.Second)
	_, err = database.Exec(`UPDATE personal_access_tokens SET expires_at=? WHERE id=?`, deadline.Format(time.RFC3339), credential.ID)
	require.NoError(t, err)
	var clock atomic.Int64
	clock.Store(start.Unix())
	service.now = func() time.Time { return time.Unix(clock.Load(), 0).UTC() }
	_, err = service.AuthenticatePersonalAccessToken(ctx, plaintext, TokenScopeLedgerRead, "198.51.100.9", "before-deadline")
	require.NoError(t, err, "the same credential is valid immediately before its deadline")

	connection, err := database.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	before := database.Stats().WaitCount
	result := make(chan error, 1)
	go func() {
		_, err := service.AuthenticatePersonalAccessToken(ctx, plaintext, TokenScopeLedgerRead, "198.51.100.9", "queued-at-deadline")
		result <- err
	}()
	require.Eventually(t, func() bool { return database.Stats().WaitCount > before }, 5*time.Second, time.Millisecond)
	clock.Store(deadline.Unix())
	require.NoError(t, connection.Close())
	select {
	case err := <-result:
		require.ErrorIs(t, err, ErrPersonalAccessTokenInvalid, "a queued request must use a fresh expiry check after acquiring the database")
	case <-time.After(5 * time.Second):
		t.Fatal("queued authentication did not finish")
	}
	var admissions int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM authentication_events WHERE request_id='queued-at-deadline' AND event_type='token_authenticated'`).Scan(&admissions))
	assert.Zero(t, admissions)
}

func TestPersonalAccessTokenAuditExpiryWhileWaitingForSQLiteWriter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, service, input := tokenTestService(t)
	credential, plaintext, err := service.CreatePersonalAccessToken(ctx, input)
	require.NoError(t, err)
	start := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	deadline := start.Add(time.Second)
	_, err = database.Exec(`UPDATE personal_access_tokens SET expires_at=? WHERE id=?`, deadline.Format(time.RFC3339), credential.ID)
	require.NoError(t, err)
	var clock atomic.Int64
	clock.Store(start.Unix())
	service.now = func() time.Time { return time.Unix(clock.Load(), 0).UTC() }
	var sequence int
	var name, path string
	require.NoError(t, database.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path))
	other, err := db.Open(ctx, db.SQLiteFileURL(path))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, other.Close()) })
	writer, err := other.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = writer.Rollback() })
	_, err = writer.Exec(`UPDATE personal_access_tokens SET name=name WHERE id=?`, credential.ID)
	require.NoError(t, err)

	// The native authorizer is a test-only observation seam: it signals the
	// actual admission UPDATE compilation, without changing SQL or its verdict.
	updating := make(chan struct{})
	var once sync.Once
	connection, err := database.Conn(ctx)
	require.NoError(t, err)
	require.NoError(t, connection.Raw(func(raw any) error {
		raw.(*sqlite.SQLiteConn).RegisterAuthorizer(func(operation int, table, column, catalog string) int {
			if operation == sqlite.SQLITE_UPDATE && table == "personal_access_tokens" {
				once.Do(func() { close(updating) })
			}
			return sqlite.SQLITE_OK
		})
		return nil
	}))
	require.NoError(t, connection.Close())
	result := make(chan error, 1)
	go func() {
		_, err := service.AuthenticatePersonalAccessToken(ctx, plaintext, TokenScopeLedgerRead, "198.51.100.9", "sqlite-queued-at-deadline")
		result <- err
	}()
	select {
	case <-updating:
	case <-time.After(5 * time.Second):
		t.Fatal("authentication did not reach the SQLite write admission")
	}
	clock.Store(deadline.Unix())
	require.NoError(t, writer.Commit())
	select {
	case err := <-result:
		require.ErrorIs(t, err, ErrPersonalAccessTokenInvalid, "SQLite busy waiting must not freeze the expiry clock")
	case <-time.After(5 * time.Second):
		t.Fatal("SQLite-queued authentication did not finish")
	}
}

func TestPersonalAccessTokenAuditManagementSessionExpiresWhileQueued(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"create", "revoke"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			database, service, input := tokenTestService(t)
			credential, _, err := service.CreatePersonalAccessToken(ctx, input)
			require.NoError(t, err)
			start := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
			deadline := start.Add(time.Second)
			_, err = database.Exec(`UPDATE auth_sessions SET expires_at=? WHERE id=?`, deadline.Format(time.RFC3339), input.SessionID)
			require.NoError(t, err)
			var clock atomic.Int64
			clock.Store(start.Unix())
			var calls atomic.Int32
			// Creation reads the clock for its reauthentication reservation,
			// token dates, then security event; revocation starts with its event.
			// Hold the real connection at that last pre-transaction clock seam.
			target := int32(3)
			if operation == "revoke" {
				target = 1
			}
			held := make(chan *sql.Conn, 1)
			service.now = func() time.Time {
				sampled := time.Unix(clock.Load(), 0).UTC()
				if calls.Add(1) == target {
					connection, err := database.Conn(ctx)
					require.NoError(t, err)
					held <- connection
				}
				return sampled
			}
			before := database.Stats().WaitCount
			result := make(chan error, 1)
			go func() {
				if operation == "create" {
					_, secret, err := service.CreatePersonalAccessToken(ctx, input)
					assert.True(t, secret == "", "refused creation cannot disclose a token")
					result <- err
				} else {
					result <- service.RevokePersonalAccessToken(ctx, input.MFAPasswordInput, credential.ID)
				}
			}()
			var connection *sql.Conn
			select {
			case connection = <-held:
			case <-time.After(5 * time.Second):
				t.Fatal("management did not reach its final pre-transaction clock")
			}
			t.Cleanup(func() { _ = connection.Close() })
			require.Eventually(t, func() bool { return database.Stats().WaitCount > before }, 5*time.Second, time.Millisecond)
			clock.Store(deadline.Unix())
			require.NoError(t, connection.Close())
			select {
			case err := <-result:
				require.Error(t, err, "an expired management session cannot change credentials after its wait")
			case <-time.After(5 * time.Second):
				t.Fatal("management did not finish")
			}
			var count int
			require.NoError(t, database.QueryRow(`SELECT count(*) FROM personal_access_tokens`).Scan(&count))
			assert.Equal(t, 1, count)
			var revoked sql.NullString
			require.NoError(t, database.QueryRow(`SELECT revoked_at FROM personal_access_tokens WHERE id=?`, credential.ID).Scan(&revoked))
			assert.False(t, revoked.Valid)
		})
	}
}

func TestPersonalAccessTokenAuditCreationRechecksVerifiedCredentials(t *testing.T) {
	t.Parallel()
	for _, change := range []string{"password", "session"} {
		t.Run(change, func(t *testing.T) {
			t.Parallel()
			database, service, input := tokenTestService(t)
			var calls int
			service.now = func() time.Time {
				calls++
				// Password verification has succeeded; the security event is
				// being prepared, immediately before the real creation tx.
				if calls == 3 {
					statement := `UPDATE users SET password_hash='synthetic-changed-hash' WHERE id=1`
					if change == "session" {
						statement = `UPDATE auth_sessions SET revoked_at='2026-10-20T12:00:00Z' WHERE id=1`
					}
					_, err := database.Exec(statement)
					require.NoError(t, err)
				}
				return time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
			}
			_, secret, err := service.CreatePersonalAccessToken(context.Background(), input)
			require.ErrorIs(t, err, ErrInvalidCredentials)
			assert.True(t, secret == "", "stale verified credentials cannot disclose a token")
			var count int
			require.NoError(t, database.QueryRow(`SELECT count(*) FROM personal_access_tokens`).Scan(&count))
			assert.Zero(t, count)
		})
	}
}

func TestPersonalAccessTokenAuditRevocationBetweenLookupAndUse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, service, input := tokenTestService(t)
	credential, plaintext, err := service.CreatePersonalAccessToken(ctx, input)
	require.NoError(t, err)
	var sequence int
	var name, path string
	require.NoError(t, database.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path))
	other, err := db.Open(ctx, db.SQLiteFileURL(path))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, other.Close()) })
	committing, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	connection, err := other.Conn(ctx)
	require.NoError(t, err)
	require.NoError(t, connection.Raw(func(raw any) error {
		raw.(*sqlite.SQLiteConn).RegisterCommitHook(func() int {
			close(committing)
			<-release
			return 0
		})
		return nil
	}))
	require.NoError(t, connection.Close())
	revoker := *service
	revoker.repository = db.NewAuthRepository(other)
	revoked := make(chan error, 1)
	go func() { revoked <- revoker.RevokePersonalAccessToken(ctx, input.MFAPasswordInput, credential.ID) }()
	select {
	case <-committing:
	case <-time.After(5 * time.Second):
		t.Fatal("real revocation did not reach commit")
	}
	// WAL readers still see the unrevoked credential while the independent
	// service owns the writer. Observe use after lookup, before write admission.
	updating := make(chan struct{})
	var once sync.Once
	connection, err = database.Conn(ctx)
	require.NoError(t, err)
	require.NoError(t, connection.Raw(func(raw any) error {
		raw.(*sqlite.SQLiteConn).RegisterAuthorizer(func(operation int, table, column, catalog string) int {
			if operation == sqlite.SQLITE_UPDATE && table == "personal_access_tokens" {
				once.Do(func() { close(updating) })
			}
			return sqlite.SQLITE_OK
		})
		return nil
	}))
	require.NoError(t, connection.Close())
	authenticated := make(chan error, 1)
	go func() {
		_, err := service.AuthenticatePersonalAccessToken(ctx, plaintext, TokenScopeLedgerRead, "198.51.100.9", "revoked-after-lookup")
		authenticated <- err
	}()
	select {
	case <-updating:
	case <-time.After(5 * time.Second):
		t.Fatal("token lookup did not reach write admission")
	}
	releaseOnce.Do(func() { close(release) })
	require.NoError(t, <-revoked)
	select {
	case err := <-authenticated:
		require.ErrorIs(t, err, ErrPersonalAccessTokenInvalid)
	case <-time.After(5 * time.Second):
		t.Fatal("token use did not finish")
	}
	var admissions int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM authentication_events WHERE request_id='revoked-after-lookup' AND event_type='token_authenticated'`).Scan(&admissions))
	assert.Zero(t, admissions)
}
