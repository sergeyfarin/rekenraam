package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"rekenraam/backend/internal/db"
)

// A create's request key (#660): the browser sends one key with every retry of
// one entry, and the write that commits stores it in the same transaction, so
// a repeat after a lost response gets the first result instead of a second
// record. See docs/conventions.md → Create Request Keys.

type (
	RequestKeyClaim  = db.RequestKeyClaim
	StoredRequestKey = db.StoredRequestKey
)

// RequestKeyRetention is how long a stored key answers a repeat.
const RequestKeyRetention = db.RequestKeyRetention

// ErrRequestKeyRepeated: the write found its key already committed by another
// request and rolled back.
var ErrRequestKeyRepeated = db.ErrRequestKeyRepeated

// MaxRequestKeyLength bounds an Idempotency-Key value.
const MaxRequestKeyLength = 128

type RequestKeyService struct {
	repository *db.RequestKeyRepository
}

func NewRequestKeyService(repository *db.RequestKeyRepository) *RequestKeyService {
	return &RequestKeyService{repository: repository}
}

// ValidateRequestKey accepts 1–128 printable ASCII characters without spaces.
func ValidateRequestKey(key string) error {
	if key == "" || len(key) > MaxRequestKeyLength {
		return ValidationError{Message: fmt.Sprintf("Idempotency-Key must be 1 to %d characters", MaxRequestKeyLength)}
	}
	for i := 0; i < len(key); i++ {
		if key[i] < '!' || key[i] > '~' {
			return ValidationError{Message: "Idempotency-Key must be printable ASCII without spaces"}
		}
	}
	return nil
}

// RequestHash identifies what a request asked for: its route and its JSON body
// with key order and whitespace removed. Numbers are kept as written, never
// read as floats. A body that is not JSON is hashed as sent; the handler
// refuses it before anything is written.
func RequestHash(route string, body []byte) string {
	canonical := body
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err == nil && !decoder.More() {
		if encoded, err := json.Marshal(value); err == nil {
			canonical = encoded
		}
	}
	sum := sha256.New()
	sum.Write([]byte(route))
	sum.Write([]byte{0})
	sum.Write(canonical)
	return hex.EncodeToString(sum.Sum(nil))
}

// NewClaim prepares the claim a write stores with its record.
func (s *RequestKeyService) NewClaim(key, route, requestHash string) (*RequestKeyClaim, error) {
	return db.NewRequestKeyClaim(BookID, key, route, requestHash)
}

// WithClaim makes the writes under ctx store the claim with their record.
func WithRequestKeyClaim(ctx context.Context, claim *RequestKeyClaim) context.Context {
	return db.WithRequestKeyClaim(ctx, claim)
}

// Lookup reads what a key recorded; found is false when nothing holds it.
func (s *RequestKeyService) Lookup(ctx context.Context, key string) (StoredRequestKey, bool, error) {
	if s == nil || s.repository == nil {
		return StoredRequestKey{}, false, errors.New("request key service is not configured")
	}
	return s.repository.Lookup(ctx, BookID, key)
}

// RecordResponse stores the result a repeat of the claim's request replays.
func (s *RequestKeyService) RecordResponse(ctx context.Context, claim *RequestKeyClaim, status int, body string) error {
	if s == nil || s.repository == nil {
		return errors.New("request key service is not configured")
	}
	return s.repository.RecordResponse(ctx, claim, status, body)
}
