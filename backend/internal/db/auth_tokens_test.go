package db_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/testdb"
	"rekenraam/backend/migrations"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersonalAccessTokenMigrationPreservesHistoryAndRefusesLossyDowngrade(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, _ := testdb.Open(t)
	provider, err := goose.NewProvider(goose.DialectSQLite3, database, migrations.FS)
	require.NoError(t, err)
	_, err = provider.DownTo(ctx, 18)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO authentication_events(id,occurred_at,event_type,outcome,username,client_ip,request_id,origin,second_factor,metadata_json)
		VALUES(17,'2026-10-01T00:00:00Z','login_succeeded','success','synthetic-owner','203.0.113.7','migration-request','browser_api','totp','{"retained":true}')`)
	require.NoError(t, err)
	_, err = provider.Up(ctx)
	require.NoError(t, err)
	var eventType, factor, metadata, requestID, username, clientIP string
	require.NoError(t, database.QueryRow(`SELECT event_type,second_factor,metadata_json,request_id,username,client_ip FROM authentication_events WHERE id=17`).Scan(&eventType, &factor, &metadata, &requestID, &username, &clientIP))
	assert.Equal(t, []string{"login_succeeded", "totp", `{"retained":true}`, "migration-request", "synthetic-owner", "203.0.113.7"}, []string{eventType, factor, metadata, requestID, username, clientIP})
	_, err = provider.DownTo(ctx, 18)
	require.NoError(t, err, "unused credential schema can be downgraded without losing old events")
	_, err = provider.Up(ctx)
	require.NoError(t, err)
	require.NoError(t, db.NewAuthRepository(database).RecordAuthenticationEvent(ctx, db.AuthenticationEventParams{
		OccurredAt: "2026-10-20T00:00:00Z", EventType: "token_denied", Outcome: "failure", Origin: "personal_access_token", FailureReason: "invalid_token",
	}))
	_, err = provider.DownTo(ctx, 18)
	require.ErrorContains(t, err, "CHECK constraint failed", "token event history alone prevents downgrade")
}

func TestPersonalAccessTokenManagementRechecksSessionAndPasswordAndConcurrentLimit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, _ := testdb.Open(t)
	_, err := database.Exec(`INSERT INTO users(id,username,password_hash,is_owner,created_at,updated_at) VALUES(1,'owner','hash',1,'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO auth_sessions(id,user_id,token_hash,created_at,expires_at) VALUES(1,1,'synthetic-session-hash','2026-10-01T00:00:00Z','2026-11-01T00:00:00Z')`)
	require.NoError(t, err)
	repo := db.NewAuthRepository(database)
	p := db.CreatePersonalAccessTokenParams{UserID: 1, SessionID: 1, PasswordHash: "hash", Name: "Synthetic client", TokenHash: fmt.Sprintf("%064d", 100), ScopesJSON: `["ledger:read"]`, CreatedAt: "2026-10-20T00:00:00Z", ExpiresAt: "2026-11-20T00:00:00Z"}
	event := db.AuthenticationEventParams{OccurredAt: p.CreatedAt, EventType: "token_created", Outcome: "success", UserID: 1, AuthSessionID: 1}
	clock := func() time.Time { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) }
	stale := p
	stale.PasswordHash = "old-hash"
	_, err = repo.CreatePersonalAccessToken(ctx, stale, event, clock)
	require.ErrorIs(t, err, db.ErrNotFound, "a password changed after verification cannot mint a token")
	_, err = database.Exec(`UPDATE auth_sessions SET revoked_at='2026-10-19T00:00:00Z' WHERE id=1`)
	require.NoError(t, err)
	_, err = repo.CreatePersonalAccessToken(ctx, p, event, clock)
	require.ErrorIs(t, err, db.ErrNotFound, "a session revoked after verification cannot mint a token")
	_, err = database.Exec(`UPDATE auth_sessions SET revoked_at=NULL WHERE id=1`)
	require.NoError(t, err)
	for i := 0; i < 49; i++ {
		_, err = database.Exec(`INSERT INTO personal_access_tokens(user_id,name,token_hash,scopes_json,created_at,expires_at) VALUES(1,'synthetic client',?,'["ledger:read"]',?,?)`, fmt.Sprintf("%064d", i), p.CreatedAt, p.ExpiresAt)
		require.NoError(t, err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			candidate := p
			candidate.TokenHash = fmt.Sprintf("%064d", 100+i)
			_, err := repo.CreatePersonalAccessToken(ctx, candidate, event, clock)
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	success, limited := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, db.ErrTokenLimit)
			limited++
		}
	}
	assert.Equal(t, 1, success)
	assert.Equal(t, 1, limited)

	row, err := repo.ReadPersonalAccessToken(ctx, p.TokenHash, p.CreatedAt)
	if err == db.ErrNotFound {
		row, err = repo.ReadPersonalAccessToken(ctx, fmt.Sprintf("%064d", 101), p.CreatedAt)
	}
	require.NoError(t, err)
	// Deterministic interleaving: lookup passes, revocation commits, use must fail.
	event.EventType = "token_revoked"
	require.NoError(t, repo.RevokePersonalAccessToken(ctx, 1, 1, row.ID, clock, event))
	event.EventType = "token_authenticated"
	event.Origin = "personal_access_token"
	require.ErrorIs(t, repo.RecordPersonalAccessTokenUse(ctx, row.ID, clock, event), db.ErrNotFound)
	var uses int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM authentication_events WHERE event_type='token_authenticated'`).Scan(&uses))
	assert.Zero(t, uses)
}
