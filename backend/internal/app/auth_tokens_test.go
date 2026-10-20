package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rekenraam/backend/internal/db"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tokenTestService(t *testing.T) (*sql.DB, *AuthService, CreatePersonalAccessTokenInput) {
	t.Helper()
	database, service := newAuthEventTestService(t, time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	sessionID, _ := signedInSession(t, service, "198.51.100.7")
	return database, service, CreatePersonalAccessTokenInput{
		MFAPasswordInput: MFAPasswordInput{UserID: 1, SessionID: sessionID, Password: "correct-horse-battery", ClientIP: "198.51.100.7", RequestID: "token-create"},
		Name:             "Synthetic test client", Scopes: []string{TokenScopeLedgerRead},
	}
}

func TestPersonalAccessTokenLifecycleScopesExpiryAndHashOnlyBackup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, service, input := tokenTestService(t)
	credential, token, err := service.CreatePersonalAccessToken(ctx, input)
	require.NoError(t, err)
	assert.Len(t, token, 50)
	assert.Equal(t, "2026-11-19T12:00:00Z", credential.ExpiresAt, "default absolute lifetime is 30 days")
	owner, err := service.AuthenticatePersonalAccessToken(ctx, token, TokenScopeLedgerRead, "203.0.113.4", "token-use")
	require.NoError(t, err)
	assert.Equal(t, int64(1), owner.ID)
	_, err = service.AuthenticatePersonalAccessToken(ctx, token, TokenScopeReportsRead, "203.0.113.4", "token-refused")
	require.ErrorIs(t, err, ErrPersonalAccessTokenScope)
	_, err = service.AuthenticatePersonalAccessToken(ctx, token, "", "203.0.113.4", "token-ungrafted-route")
	require.ErrorIs(t, err, ErrPersonalAccessTokenScope)

	var hash string
	require.NoError(t, database.QueryRow(`SELECT token_hash FROM personal_access_tokens WHERE id = ?`, credential.ID).Scan(&hash))
	assert.Equal(t, hashSessionToken("pat:"+token), hash)
	assert.NotEqual(t, hashSessionToken(token), hash, "a token is not a session credential")
	created := requireOneAttributedEvent(t, service, "token_created", input.SessionID, input.ClientIP, input.RequestID)
	assert.JSONEq(t, fmt.Sprintf(`{"token_id":%d}`, credential.ID), created.MetadataJSON)
	used := eventsOfType(t, service, "token_authenticated")
	require.Len(t, used, 1)
	assert.Equal(t, "personal_access_token", used[0].Origin)
	assert.Equal(t, "203.0.113.4", used[0].ClientIP)
	assert.Equal(t, "token-use", used[0].RequestID)
	assert.Nil(t, used[0].AuthSessionID)

	list, err := service.ListPersonalAccessTokens(ctx, 1, "")
	require.NoError(t, err)
	require.Len(t, list.Tokens, 1)
	assert.NotNil(t, list.Tokens[0].LastUsedAt)
	metadata, err := json.Marshal(list)
	require.NoError(t, err)
	assert.NotContains(t, string(metadata), token)
	assert.NotContains(t, string(metadata), hash)
	backup := filepath.Join(t.TempDir(), "token-backup.sqlite")
	require.NoError(t, db.BackupSQLiteDatabase(ctx, database, backup))
	require.NoError(t, db.VerifySQLiteBackup(ctx, backup))
	bytes, err := os.ReadFile(backup)
	require.NoError(t, err)
	assert.NotContains(t, string(bytes), token, "native backup contains no plaintext credential")
	assert.Contains(t, string(bytes), hash, "native backup retains hash-only credentials")

	require.NoError(t, service.RevokePersonalAccessToken(ctx, input.MFAPasswordInput, credential.ID))
	_, err = service.AuthenticatePersonalAccessToken(ctx, token, TokenScopeLedgerRead, "203.0.113.4", "after-revoke")
	require.ErrorIs(t, err, ErrPersonalAccessTokenInvalid)
	assert.ErrorIs(t, service.RevokePersonalAccessToken(ctx, input.MFAPasswordInput, credential.ID), ErrPersonalAccessTokenNotFound)
	_, err = service.AuthenticatePersonalAccessToken(ctx, "rr_pat_"+strings.Repeat("A", 43), TokenScopeLedgerRead, "203.0.113.4", "unknown")
	require.ErrorIs(t, err, ErrPersonalAccessTokenInvalid)

	input.Scopes = []string{TokenScopeReportsRead}
	_, reportToken, err := service.CreatePersonalAccessToken(ctx, input)
	require.NoError(t, err)
	_, err = service.AuthenticatePersonalAccessToken(ctx, reportToken, TokenScopeReportsRead, "", "")
	require.NoError(t, err, "reports scope works without ledger grant")
	service.now = func() time.Time { return time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC) }
	_, err = service.AuthenticatePersonalAccessToken(ctx, reportToken, TokenScopeReportsRead, "", "")
	require.ErrorIs(t, err, ErrPersonalAccessTokenInvalid, "expiry is exclusive at the exact boundary")
}

func TestPersonalAccessTokenCreationValidationAndSharedReauthenticationBudget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, service, input := tokenTestService(t)
	for _, tc := range []struct {
		name   string
		change func(*CreatePersonalAccessTokenInput)
	}{
		{"empty name", func(p *CreatePersonalAccessTokenInput) { p.Name = " " }},
		{"control name", func(p *CreatePersonalAccessTokenInput) { p.Name = "a\nb" }},
		{"UTF8 byte limit", func(p *CreatePersonalAccessTokenInput) { p.Name = strings.Repeat("€", 27) }},
		{"empty scopes", func(p *CreatePersonalAccessTokenInput) { p.Scopes = nil }},
		{"wildcard", func(p *CreatePersonalAccessTokenInput) { p.Scopes = []string{"*"} }},
		{"write scope", func(p *CreatePersonalAccessTokenInput) { p.Scopes = []string{"ledger:write"} }},
		{"duplicate scope", func(p *CreatePersonalAccessTokenInput) {
			p.Scopes = []string{TokenScopeLedgerRead, TokenScopeLedgerRead}
		}},
		{"zero lifetime", func(p *CreatePersonalAccessTokenInput) { v := 0; p.ExpiresInDays = &v }},
		{"excess lifetime", func(p *CreatePersonalAccessTokenInput) { v := 366; p.ExpiresInDays = &v }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := input
			tc.change(&p)
			_, token, err := service.CreatePersonalAccessToken(ctx, p)
			var validation ValidationError
			require.ErrorAs(t, err, &validation)
			assert.Empty(t, token)
		})
	}
	for range 4 {
		bad := input
		bad.Password = "wrong-password"
		_, token, err := service.CreatePersonalAccessToken(ctx, bad)
		require.ErrorIs(t, err, ErrInvalidCredentials)
		assert.Empty(t, token)
	}
	// The fifth failure on MFA settings exhausts exactly the same budget.
	assert.ErrorIs(t, service.DisableMFA(ctx, MFAPasswordInput{UserID: 1, SessionID: input.SessionID, Password: "wrong-password"}), ErrRateLimited)
	_, token, err := service.CreatePersonalAccessToken(ctx, input)
	require.ErrorIs(t, err, ErrRateLimited)
	assert.Empty(t, token)
	var count int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM personal_access_tokens`).Scan(&count))
	assert.Zero(t, count)
}

func TestPersonalAccessTokenSecurityEventsFailClosedAndRecoveryRevokes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, service, input := tokenTestService(t)
	_, err := database.Exec(`CREATE TRIGGER fail_token_event BEFORE INSERT ON authentication_events WHEN NEW.event_type = 'token_created' BEGIN SELECT RAISE(ABORT, 'synthetic event failure'); END`)
	require.NoError(t, err)
	_, token, err := service.CreatePersonalAccessToken(ctx, input)
	require.Error(t, err)
	assert.Empty(t, token)
	var count int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM personal_access_tokens`).Scan(&count))
	assert.Zero(t, count)
	_, err = database.Exec(`DROP TRIGGER fail_token_event`)
	require.NoError(t, err)
	credential, token, err := service.CreatePersonalAccessToken(ctx, input)
	require.NoError(t, err)
	_, err = database.Exec(`CREATE TRIGGER fail_token_event BEFORE INSERT ON authentication_events WHEN NEW.event_type IN ('token_revoked', 'token_authenticated', 'owner_access_recovered') BEGIN SELECT RAISE(ABORT, 'synthetic event failure'); END`)
	require.NoError(t, err)
	require.Error(t, service.RevokePersonalAccessToken(ctx, input.MFAPasswordInput, credential.ID))
	_, err = service.AuthenticatePersonalAccessToken(ctx, token, TokenScopeLedgerRead, "", "")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrPersonalAccessTokenInvalid)
	var revoked, used sql.NullString
	require.NoError(t, database.QueryRow(`SELECT revoked_at,last_used_at FROM personal_access_tokens WHERE id=?`, credential.ID).Scan(&revoked, &used))
	assert.False(t, revoked.Valid)
	assert.False(t, used.Valid)
	recovery := NewRecoveryService(database, "", db.NewRecoveryRepository(database))
	recovery.now = service.now
	require.Error(t, recovery.ResetOwnerAccess(ctx, ResetOwnerAccessInput{Password: "synthetic-replacement-password"}))
	require.NoError(t, database.QueryRow(`SELECT revoked_at FROM personal_access_tokens WHERE id=?`, credential.ID).Scan(&revoked))
	assert.False(t, revoked.Valid, "failed recovery rolls token revocation back")
	_, err = database.Exec(`DROP TRIGGER fail_token_event`)
	require.NoError(t, err)
	_, err = service.AuthenticatePersonalAccessToken(ctx, token, TokenScopeLedgerRead, "", "")
	require.NoError(t, err, "failed revocation leaves the token usable")
	require.NoError(t, recovery.ResetOwnerAccess(ctx, ResetOwnerAccessInput{Password: "synthetic-replacement-password"}))
	_, err = service.AuthenticatePersonalAccessToken(ctx, token, TokenScopeLedgerRead, "", "")
	require.ErrorIs(t, err, ErrPersonalAccessTokenInvalid)
	events := eventsOfType(t, service, "owner_access_recovered")
	require.Len(t, events, 1)
	assert.Contains(t, events[0].MetadataJSON, `"tokens_revoked":1`)
}

func TestPersonalAccessTokenListConsumesHistoryAndActiveLimit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, service, input := tokenTestService(t)
	for i := 0; i < 51; i++ {
		_, err := database.Exec(`INSERT INTO personal_access_tokens(user_id,name,token_hash,scopes_json,created_at,expires_at,revoked_at) VALUES(1,'history',?,'["ledger:read"]','2026-10-01T00:00:00Z','2026-11-01T00:00:00Z','2026-10-02T00:00:00Z')`, fmt.Sprintf("%064d", i))
		require.NoError(t, err)
	}
	first, err := service.ListPersonalAccessTokens(ctx, 1, "")
	require.NoError(t, err)
	require.Len(t, first.Tokens, 50)
	require.NotNil(t, first.NextCursor)
	last, err := service.ListPersonalAccessTokens(ctx, 1, *first.NextCursor)
	require.NoError(t, err)
	require.Len(t, last.Tokens, 1)
	assert.Nil(t, last.NextCursor)
	assert.Less(t, last.Tokens[0].ID, first.Tokens[49].ID)
	_, _, err = service.CreatePersonalAccessToken(ctx, input)
	require.NoError(t, err, "revoked history does not spend the active limit")
	_, err = database.Exec(`UPDATE personal_access_tokens SET revoked_at=NULL WHERE id>1`)
	require.NoError(t, err)
	_, token, err := service.CreatePersonalAccessToken(ctx, input)
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	assert.Empty(t, token)
}
