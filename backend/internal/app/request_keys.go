package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// RequestHash identifies what a request asked for: its route and its exact
// body bytes. Equal hashes mean equal bytes, so the endpoint's decoder reads
// the same request; a body that differs only in key order or whitespace is
// another request, refused as IDEMPOTENCY_KEY_REUSED rather than replayed.
// A browser retry sends the same bytes.
func RequestHash(route string, body []byte) string {
	sum := sha256.New()
	sum.Write([]byte(route))
	sum.Write([]byte{0})
	sum.Write(body)
	return hex.EncodeToString(sum.Sum(nil))
}

// ValidateRequestKeyBody accepts a keyed request's body only when it is
// exactly one JSON value and no object in it names a member twice, compared
// the way the endpoint decoder matches field names (case-insensitively).
// Encoding/json keeps the last of two such members, so their order would
// change what is saved while the request still looked like an earlier one.
func ValidateRequestKeyBody(body []byte) error {
	if !json.Valid(body) {
		return ValidationError{Message: "a request with an Idempotency-Key must carry exactly one JSON value"}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	type frame struct {
		object    bool
		expectKey bool
		keys      []string
	}
	var stack []*frame
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return ValidationError{Message: "a request with an Idempotency-Key must carry exactly one JSON value"}
		}
		var top *frame
		if len(stack) > 0 {
			top = stack[len(stack)-1]
		}
		if key, isString := token.(string); isString && top != nil && top.object && top.expectKey {
			for _, seen := range top.keys {
				if strings.EqualFold(seen, key) {
					return ValidationError{Message: fmt.Sprintf("a request with an Idempotency-Key must name each field once; %q appears twice", key)}
				}
			}
			top.keys = append(top.keys, key)
			top.expectKey = false
			continue
		}
		switch token {
		case json.Delim('{'):
			stack = append(stack, &frame{object: true, expectKey: true})
			continue
		case json.Delim('['):
			stack = append(stack, &frame{})
			continue
		case json.Delim('}'), json.Delim(']'):
			stack = stack[:len(stack)-1]
		}
		// A value ended: an enclosing object expects its next key.
		if len(stack) > 0 && stack[len(stack)-1].object {
			stack[len(stack)-1].expectKey = true
		}
	}
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
