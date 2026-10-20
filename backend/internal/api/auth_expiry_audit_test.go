package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/db"
)

// Session reads must remain usable under a WAL writer. Real financial and
// settings consumers then refuse the expired cookie without durable changes.
func TestAuthExpiryAuditSessionHTTPReadBoundary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler, database := newSetupTestHandler(t)
	cookie, csrf, _ := setupAccountAPITest(t, handler)
	var sequence int
	var name, path string
	require.NoError(t, database.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path))
	other, err := db.Open(ctx, db.SQLiteFileURL(path))
	require.NoError(t, err)
	defer func() { require.NoError(t, other.Close()) }()
	writer, err := other.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = writer.Rollback() }()
	_, err = writer.Exec(`UPDATE auth_sessions SET expires_at='2020-01-01T00:00:00Z'`)
	require.NoError(t, err)
	connection, err := database.Conn(ctx)
	require.NoError(t, err)
	// query_only enforces read purity through the actual handler's connection.
	_, err = connection.ExecContext(ctx, `PRAGMA query_only=ON`)
	require.NoError(t, err)
	require.NoError(t, connection.Close())
	// The old committed session remains admissible while expiry is uncommitted.
	assert.Equal(t, 200, tokenAPIRequest(handler, "GET", "/api/v1/accounts", "", cookie, "", "").Code)
	connection, err = database.Conn(ctx)
	require.NoError(t, err)
	_, err = connection.ExecContext(ctx, `PRAGMA query_only=OFF`)
	require.NoError(t, err)
	require.NoError(t, connection.Close())
	require.NoError(t, writer.Commit())
	var before int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM accounts`).Scan(&before))
	assert.Equal(t, 401, tokenAPIRequest(handler, "GET", "/api/v1/accounts", "", cookie, "", "").Code)
	assert.Equal(t, 401, tokenAPIRequest(handler, "GET", "/api/v1/auth/mfa", "", cookie, "", "").Code)
	assert.Equal(t, 401, tokenAPIRequest(handler, "POST", "/api/v1/accounts", `{"name":"refused-audit-account","type":"asset","commodity_id":1}`, cookie, csrf, "").Code)
	var after int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM accounts`).Scan(&after))
	assert.Equal(t, before, after, "expired admission must not mutate finances")
}

func TestAuthExpiryAuditSessionHTTPInFlightCutoff(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"logout", "expiry", "recovery"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			handler, database := newSetupTestHandler(t)
			cookie := bootstrapOwner(t, handler)
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			auth := app.NewAuthService(db.NewAuthRepository(database), logger)
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			consumer := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, ok := authenticatedOwner(w, r, logger, auth)
				if !ok {
					return
				}
				close(entered)
				<-release
				w.WriteHeader(200)
			})
			completed := make(chan int, 1)
			go func() { completed <- tokenAPIRequest(consumer, "GET", "/api/v1/accounts", "", cookie, "", "").Code }()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("consumer did not authenticate")
			}
			switch operation {
			case "logout":
				require.NoError(t, auth.Logout(context.Background(), app.LogoutInput{Token: cookie.Value}))
			case "expiry":
				_, err := database.Exec(`UPDATE auth_sessions SET expires_at='2020-01-01T00:00:00Z'`)
				require.NoError(t, err)
			case "recovery":
				require.NoError(t, app.NewRecoveryService(database, "", db.NewRecoveryRepository(database)).ResetOwnerAccess(context.Background(), app.ResetOwnerAccessInput{Password: "synthetic-audit-replacement"}))
			}
			assert.Equal(t, 401, tokenAPIRequest(handler, "GET", "/api/v1/auth/mfa", "", cookie, "", "").Code)
			once.Do(func() { close(release) })
			select {
			case status := <-completed:
				assert.Equal(t, 200, status)
			case <-time.After(5 * time.Second):
				t.Fatal("admitted consumer did not finish")
			}
		})
	}
}
