package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// RequestKeyRetention is how long a create's request key answers a repeat
// (#660). A browser retries within seconds or minutes of a lost response; a
// week also covers a form left open with its error showing. Older rows are
// pruned when a new key is claimed.
const RequestKeyRetention = 7 * 24 * time.Hour

// ErrRequestKeyRepeated reports that a committed write already holds the
// request key the current write tried to claim. The write's transaction rolls
// back, so nothing is written twice.
var ErrRequestKeyRepeated = errors.New("request key already used by a committed write")

// RequestKeyClaim is one request's claim on its key. Placed in the write's
// context with WithRequestKeyClaim, it is stored by the audit event every
// financial write inserts in its own transaction, so the key commits or rolls
// back with the record it names.
type RequestKeyClaim struct {
	BookID      int64
	Key         string
	Route       string
	RequestHash string

	token    string
	claimed  bool
	repeated bool
}

// NewRequestKeyClaim prepares a claim with a fresh random token.
func NewRequestKeyClaim(bookID int64, key, route, requestHash string) (*RequestKeyClaim, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, fmt.Errorf("generate request key claim token: %w", err)
	}
	return &RequestKeyClaim{BookID: bookID, Key: key, Route: route, RequestHash: requestHash, token: hex.EncodeToString(token[:])}, nil
}

// Claimed reports whether a write under this claim inserted the key. The
// insert may still have rolled back with its write.
func (c *RequestKeyClaim) Claimed() bool { return c.claimed }

// Repeated reports whether a write under this claim found the key already
// held by another committed write and rolled back.
func (c *RequestKeyClaim) Repeated() bool { return c.repeated }

type requestKeyClaimContextKey struct{}

// WithRequestKeyClaim makes the writes under ctx claim the request key.
func WithRequestKeyClaim(ctx context.Context, claim *RequestKeyClaim) context.Context {
	return context.WithValue(ctx, requestKeyClaimContextKey{}, claim)
}

func requestKeyClaimFrom(ctx context.Context) *RequestKeyClaim {
	claim, _ := ctx.Value(requestKeyClaimContextKey{}).(*RequestKeyClaim)
	return claim
}

// claimRequestKeyTx stores the context's request key beside the audit event
// just inserted in tx. A key already stored by another request refuses the
// write with ErrRequestKeyRepeated; the single writer connection makes the
// check and the insert one step. A second audit event of the same request
// finds its own row and leaves it.
func claimRequestKeyTx(ctx context.Context, tx *sql.Tx, auditEventID int64, occurredAt string) error {
	claim := requestKeyClaimFrom(ctx)
	if claim == nil {
		return nil
	}
	if claim.BookID <= 0 {
		return fmt.Errorf("request key claim must name a book")
	}

	var token string
	err := tx.QueryRowContext(ctx, `SELECT claim_token FROM request_keys WHERE book_id = ? AND request_key = ?`, claim.BookID, claim.Key).Scan(&token)
	switch {
	case err == nil:
		if token == claim.token {
			return nil
		}
		claim.repeated = true
		return ErrRequestKeyRepeated
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("read request key: %w", err)
	}

	now, err := time.Parse(time.RFC3339, occurredAt)
	if err != nil {
		return fmt.Errorf("parse request key time: %w", err)
	}
	cutoff := now.Add(-RequestKeyRetention).UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `DELETE FROM request_keys WHERE book_id = ? AND created_at < ?`, claim.BookID, cutoff); err != nil {
		return fmt.Errorf("prune request keys: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO request_keys (book_id, request_key, route, request_hash, claim_token, audit_event_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, claim.BookID, claim.Key, claim.Route, claim.RequestHash, claim.token, auditEventID, occurredAt); err != nil {
		return fmt.Errorf("insert request key: %w", err)
	}
	claim.claimed = true
	return nil
}

// StoredRequestKey is what a request key recorded.
type StoredRequestKey struct {
	Route          string
	RequestHash    string
	ResponseStatus int
	ResponseBody   string
	HasResponse    bool
}

type RequestKeyRepository struct {
	db *sql.DB
}

func NewRequestKeyRepository(database *sql.DB) *RequestKeyRepository {
	return &RequestKeyRepository{db: database}
}

// Lookup reads a stored request key; found is false when none is stored.
func (r *RequestKeyRepository) Lookup(ctx context.Context, bookID int64, key string) (StoredRequestKey, bool, error) {
	var stored StoredRequestKey
	var status sql.NullInt64
	var body sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT route, request_hash, response_status, response_body
		FROM request_keys
		WHERE book_id = ? AND request_key = ?
	`, bookID, key).Scan(&stored.Route, &stored.RequestHash, &status, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return StoredRequestKey{}, false, nil
	}
	if err != nil {
		return StoredRequestKey{}, false, fmt.Errorf("read request key: %w", err)
	}
	if status.Valid && body.Valid {
		stored.ResponseStatus = int(status.Int64)
		stored.ResponseBody = body.String
		stored.HasResponse = true
	}
	return stored, true, nil
}

// RecordResponse stores the response of the write that claimed the key. Only
// the claiming request's own row is updated, and only once; a claim whose
// write rolled back updates nothing.
func (r *RequestKeyRepository) RecordResponse(ctx context.Context, claim *RequestKeyClaim, status int, body string) error {
	if claim == nil || !claim.claimed {
		return nil
	}
	if _, err := r.db.ExecContext(ctx, `
		UPDATE request_keys
		SET response_status = ?, response_body = ?
		WHERE book_id = ? AND request_key = ? AND claim_token = ? AND response_status IS NULL
	`, status, body, claim.BookID, claim.Key, claim.token); err != nil {
		return fmt.Errorf("record request key response: %w", err)
	}
	return nil
}
