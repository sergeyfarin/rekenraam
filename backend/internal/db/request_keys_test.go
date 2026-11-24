package db

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #660: a create's request key is stored by its audit event, in the write's
// own transaction.

func requestKeyTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	database := openTestDatabase(t)
	require.NoError(t, Migrate(ctx, database))
	_, err := database.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, is_owner, created_at, updated_at)
		VALUES (1, 'owner', 'hash', 1, '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z')`)
	require.NoError(t, err)
	_, err = database.ExecContext(ctx, bookInsertWithID, 1, "main")
	require.NoError(t, err)
	return database
}

// keyedWrite stands in for a financial write: one audit event in one
// transaction, committed unless the claim refuses it.
func keyedWrite(t *testing.T, database *sql.DB, claim *RequestKeyClaim, occurredAt string, events int, commit bool) error {
	t.Helper()
	ctx := WithRequestKeyClaim(context.Background(), claim)
	tx, err := database.BeginTx(ctx, nil)
	require.NoError(t, err)
	for range events {
		if _, err := insertAuditEvent(ctx, tx, AuditEventParams{BookID: 1, OccurredAt: occurredAt, OriginType: "browser_api", Operation: "transaction.create"}); err != nil {
			require.NoError(t, tx.Rollback())
			return err
		}
	}
	if !commit {
		require.NoError(t, tx.Rollback())
		return nil
	}
	return tx.Commit()
}

func newTestClaim(t *testing.T, key, hash string) *RequestKeyClaim {
	t.Helper()
	claim, err := NewRequestKeyClaim(1, key, "POST /api/v1/transactions", hash)
	require.NoError(t, err)
	return claim
}

var (
	hashA = strings.Repeat("a", 64)
	hashB = strings.Repeat("b", 64)
)

func countIn(t *testing.T, database *sql.DB, query string) int {
	t.Helper()
	var count int
	require.NoError(t, database.QueryRow(query).Scan(&count))
	return count
}

// The race the HTTP pre-check cannot close: a second request passes the
// lookup before the first commits. Its write finds the key inside its own
// transaction and rolls back.
func TestRequestKeyClaimRefusesAKeyAnotherWriteCommitted(t *testing.T) {
	t.Parallel()
	database := requestKeyTestDatabase(t)
	first := newTestClaim(t, "entry-1", hashA)
	require.NoError(t, keyedWrite(t, database, first, "2026-06-01T10:00:00Z", 1, true))
	assert.True(t, first.Claimed())
	events := countIn(t, database, `SELECT count(*) FROM audit_events`)

	second := newTestClaim(t, "entry-1", hashA)
	err := keyedWrite(t, database, second, "2026-06-01T10:00:01Z", 1, true)

	require.ErrorIs(t, err, ErrRequestKeyRepeated)
	assert.True(t, second.Repeated())
	assert.Equal(t, events, countIn(t, database, `SELECT count(*) FROM audit_events`), "the repeated write rolled back")
	assert.Equal(t, 1, countIn(t, database, `SELECT count(*) FROM request_keys`))
}

func TestRequestKeyClaimSurvivesASecondAuditEventAndARolledBackAttempt(t *testing.T) {
	t.Parallel()
	database := requestKeyTestDatabase(t)
	claim := newTestClaim(t, "entry-1", hashA)

	// A rolled-back preview run of the writer stores nothing...
	require.NoError(t, keyedWrite(t, database, claim, "2026-06-01T10:00:00Z", 1, false))
	assert.Zero(t, countIn(t, database, `SELECT count(*) FROM request_keys`))
	// ...and the real write, with two audit events, stores the key once.
	require.NoError(t, keyedWrite(t, database, claim, "2026-06-01T10:00:00Z", 2, true))

	assert.False(t, claim.Repeated())
	assert.Equal(t, 1, countIn(t, database, `SELECT count(*) FROM request_keys`))
}

func TestRequestKeyResponseIsRecordedOnceByTheClaimingRequest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database := requestKeyTestDatabase(t)
	repository := NewRequestKeyRepository(database)
	claim := newTestClaim(t, "entry-1", hashA)
	require.NoError(t, keyedWrite(t, database, claim, "2026-06-01T10:00:00Z", 1, true))

	stored, found, err := repository.Lookup(ctx, 1, "entry-1")
	require.NoError(t, err)
	require.True(t, found)
	assert.False(t, stored.HasResponse, "committed, result not yet stored")

	other := newTestClaim(t, "entry-1", hashB)
	other.claimed = true
	require.NoError(t, repository.RecordResponse(ctx, other, 201, `{"id":99}`))
	require.NoError(t, repository.RecordResponse(ctx, claim, 201, `{"id":1}`))
	require.NoError(t, repository.RecordResponse(ctx, claim, 201, `{"id":2}`))

	stored, found, err = repository.Lookup(ctx, 1, "entry-1")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, StoredRequestKey{Route: "POST /api/v1/transactions", RequestHash: hashA, ResponseStatus: 201, ResponseBody: `{"id":1}`, HasResponse: true}, stored)
}

func TestRequestKeysOlderThanTheRetentionArePrunedWhenAKeyIsClaimed(t *testing.T) {
	t.Parallel()
	database := requestKeyTestDatabase(t)
	require.NoError(t, keyedWrite(t, database, newTestClaim(t, "old", hashA), "2026-06-01T10:00:00Z", 1, true))
	require.NoError(t, keyedWrite(t, database, newTestClaim(t, "recent", hashA), "2026-06-07T10:00:00Z", 1, true))

	// Seven days after the first key, to the second.
	require.NoError(t, keyedWrite(t, database, newTestClaim(t, "new", hashA), "2026-06-08T10:00:01Z", 1, true))

	rows, err := database.Query(`SELECT request_key FROM request_keys ORDER BY id`)
	require.NoError(t, err)
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		require.NoError(t, rows.Scan(&key))
		keys = append(keys, key)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"recent", "new"}, keys)
}

func TestRequestKeyColumnsRefuseNonCanonicalText(t *testing.T) {
	t.Parallel()
	database := requestKeyTestDatabase(t)
	require.NoError(t, keyedWrite(t, database, newTestClaim(t, "seed", hashA), "2026-06-01T10:00:00Z", 1, true))
	insert := `INSERT INTO request_keys (book_id, request_key, route, request_hash, claim_token, audit_event_id, created_at)
		VALUES (1, ?, 'POST /x', ?, ?, (SELECT max(id) FROM audit_events), '2026-06-01T10:00:00Z')`
	token := strings.Repeat("0", 32)
	for name, args := range map[string][]any{
		"key with a space":     {"a b", hashA, token},
		"key with a NUL":       {"ab\x00", hashA, token},
		"key too long":         {strings.Repeat("k", 129), hashA, token},
		"uppercase hash":       {"k1", strings.Repeat("A", 64), token},
		"short hash":           {"k2", "abc", token},
		"hash with a NUL tail": {"k3", strings.Repeat("a", 63) + "\x00", token},
	} {
		_, err := database.Exec(insert, args...)
		assert.Error(t, err, name)
	}
	_, err := database.Exec(`UPDATE request_keys SET created_at = '2026-02-30T10:00:00Z'`)
	assert.Error(t, err, "created_at is a real timestamp")
	_, err = database.Exec(`UPDATE request_keys SET created_at = '2026-06-01 10:00:00'`)
	assert.Error(t, err, "created_at is canonical RFC 3339")
	_, err = database.Exec(`UPDATE request_keys SET response_status = 201, response_body = 'not json'`)
	assert.Error(t, err, "a stored response is JSON")
	_, err = database.Exec(`UPDATE request_keys SET response_status = 201`)
	assert.Error(t, err, "status and body are stored together")
}
