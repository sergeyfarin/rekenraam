package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/db"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #357: hold a real HTTP consumer after committed authentication. Revocation
// and owner recovery stop the next admission; neither cancels admitted work.
func TestPersonalAccessTokenAuditInFlightCutoff(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"revoke", "recover"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			handler, database := newSetupTestHandler(t)
			cookie := bootstrapOwner(t, handler)
			csrf := authSessionCSRFToken(t, handler, cookie)
			credential, token := createTokenAPI(t, handler, cookie, csrf, []string{app.TokenScopeLedgerRead})
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			auth := app.NewAuthService(db.NewAuthRepository(database), logger)
			entered, release := make(chan struct{}), make(chan struct{})
			defer close(release)
			var dispatched atomic.Int32
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v1/accounts", func(w http.ResponseWriter, r *http.Request) {
				_, ok := authenticatedOwner(w, r, logger, auth)
				if !ok {
					return
				}
				dispatched.Add(1)
				close(entered)
				<-release
				w.WriteHeader(http.StatusOK)
			})
			boundary := withPersonalAccessToken(logger, auth, HandlerOptions{}, mux)
			result := make(chan int, 1)
			go func() {
				result <- tokenAPIRequest(boundary, "GET", "/api/v1/accounts", "", nil, "", "Bearer "+token).Code
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("consumer did not pass authentication")
			}
			if operation == "revoke" {
				require.NoError(t, auth.RevokePersonalAccessToken(context.Background(), app.MFAPasswordInput{UserID: 1, SessionID: 1}, credential.ID))
			} else {
				recovery := app.NewRecoveryService(database, "", db.NewRecoveryRepository(database))
				require.NoError(t, recovery.ResetOwnerAccess(context.Background(), app.ResetOwnerAccessInput{Password: "synthetic-audit-replacement-password"}))
			}
			assert.Equal(t, 401, tokenAPIRequest(boundary, "GET", "/api/v1/accounts", "", cookie, csrf, "Bearer "+token).Code)
			assert.Equal(t, int32(1), dispatched.Load())
			release <- struct{}{}
			select {
			case status := <-result:
				assert.Equal(t, 200, status)
			case <-time.After(5 * time.Second):
				t.Fatal("admitted consumer did not finish")
			}
		})
	}
}

func TestPersonalAccessTokenAuditDeniedEventFailuresNeverDispatch(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	cookie := bootstrapOwner(t, handler)
	csrf := authSessionCSRFToken(t, handler, cookie)
	credential, token := createTokenAPI(t, handler, cookie, csrf, []string{app.TokenScopeLedgerRead})
	_, err := database.Exec(`CREATE TEMP TRIGGER fail_audit_denied_event BEFORE INSERT ON authentication_events WHEN NEW.event_type='token_denied' BEGIN SELECT RAISE(ABORT,'synthetic audit event failure'); END`)
	require.NoError(t, err)
	for _, tc := range []struct{ token, path string }{
		{"invalid", "/api/v1/accounts"}, {token, "/api/v1/reports/cashflow"}, {token, "/api/v1/auth/tokens"},
	} {
		got := tokenAPIRequest(handler, "GET", tc.path, "", cookie, csrf, "Bearer "+tc.token)
		assert.Equal(t, 500, got.Code, "audit failure refuses even with a valid owner cookie")
	}
	var used any
	require.NoError(t, database.QueryRow(`SELECT last_used_at FROM personal_access_tokens WHERE id=?`, credential.ID).Scan(&used))
	assert.Nil(t, used)
	_, err = database.Exec(`DROP TRIGGER fail_audit_denied_event`)
	require.NoError(t, err)
	assert.Equal(t, 403, tokenAPIRequest(handler, "GET", "/api/v1/reports/cashflow", "", cookie, csrf, "Bearer "+token).Code)
}

func TestPersonalAccessTokenAuditNativeRestorePreservesExpiry(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	cookie, csrf, _ := setupAccountAPITest(t, handler)
	credential, token := createTokenAPI(t, handler, cookie, csrf, []string{app.TokenScopeLedgerRead})
	assert.Equal(t, 200, tokenAPIRequest(handler, "GET", "/api/v1/accounts", "", nil, "", "Bearer "+token).Code)
	// Synthetic dates model a backup taken after expiry without a wall-clock
	// sleep or a production clock/authorization bypass.
	_, err := database.Exec(`UPDATE personal_access_tokens SET created_at='2020-01-01T00:00:00Z',expires_at='2020-01-02T00:00:00Z' WHERE id=?`, credential.ID)
	require.NoError(t, err)
	backup := filepath.Join(t.TempDir(), "expired.sqlite")
	require.NoError(t, db.BackupSQLiteDatabase(context.Background(), database, backup))
	restored, restoredDatabase, _ := restoreOntoFreshInstance(t, backup)
	assert.Equal(t, 401, tokenAPIRequest(restored.handler, "GET", "/api/v1/accounts", "", cookie, csrf, "Bearer "+token).Code)
	var expiry string
	var revoked any
	require.NoError(t, restoredDatabase.QueryRow(`SELECT expires_at,revoked_at FROM personal_access_tokens WHERE id=?`, credential.ID).Scan(&expiry, &revoked))
	assert.Equal(t, "2020-01-02T00:00:00Z", expiry)
	assert.Nil(t, revoked, "expiry, rather than a synthetic revocation, caused the refusal")
}
