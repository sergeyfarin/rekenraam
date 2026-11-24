package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net/http"

	"rekenraam/backend/internal/app"
)

// Create request keys (#660). A browser form sends one Idempotency-Key with
// every retry of one entry. The write that commits stores the key in its own
// transaction; a repeat is answered with the stored result instead of writing
// again. See docs/conventions.md → Create Request Keys.
const (
	idempotencyKeyHeader      = "Idempotency-Key"
	idempotentReplayedHeader  = "Idempotent-Replayed"
	requestKeyReusedCode      = "IDEMPOTENCY_KEY_REUSED"
	requestKeyUnavailableCode = "IDEMPOTENCY_KEY_RESULT_UNAVAILABLE"
)

var errRequestKeysNotConfigured = errors.New("request key service is not configured")

type requestKeyRouteContextKey struct{}

// withRequestKey marks a create route as accepting a request key. The key is
// handled after authentication (requireAuthenticatedMutation), so a stored
// result is only ever replayed to the signed-in owner.
func withRequestKey(service *app.RequestKeyService, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next(w, r.WithContext(context.WithValue(r.Context(), requestKeyRouteContextKey{}, service)))
	}
}

// serveWithRequestKey runs an authenticated mutation, applying its request key
// when the request carries one.
func serveWithRequestKey(logger *slog.Logger, w http.ResponseWriter, r *http.Request, next http.Handler) {
	values := r.Header.Values(idempotencyKeyHeader)
	if len(values) == 0 {
		next.ServeHTTP(w, r)
		return
	}
	service, accepted := r.Context().Value(requestKeyRouteContextKey{}).(*app.RequestKeyService)
	if !accepted {
		// A client that believes a retry is safe must not be silently wrong.
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "Idempotency-Key is not accepted on this route")
		return
	}
	if len(values) > 1 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "send one Idempotency-Key")
		return
	}
	key := values[0]
	if err := app.ValidateRequestKey(key); err != nil {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	if service == nil {
		writeServiceInternalError(w, r, logger, "request key", errRequestKeysNotConfigured)
		return
	}

	// The body is read once, under the shared decoder's limit, and handed on
	// unchanged to the handler's own decoder.
	var body []byte
	if r.Body != nil {
		read, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBodyBytes+1))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "DECODE_ERROR", "request body could not be read")
			return
		}
		if len(read) > maxJSONBodyBytes {
			writeDecodeError(w, errRequestTooLarge)
			return
		}
		body = read
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	if err := app.ValidateRequestKeyBody(body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	requestHash := app.RequestHash(r.Pattern, body)

	if answered := answerFromStoredKey(logger, w, r, service, key, requestHash); answered {
		return
	}

	claim, err := service.NewClaim(key, r.Pattern, requestHash)
	if err != nil {
		writeServiceInternalError(w, r, logger, "request key", err)
		return
	}
	buffered := newBufferedResponse()
	next.ServeHTTP(buffered, r.WithContext(app.WithRequestKeyClaim(r.Context(), claim)))

	if claim.Repeated() {
		// Another request committed the key while this one was being
		// prepared; this write rolled back. Answer as a repeat.
		if !answerFromStoredKey(logger, w, r, service, key, requestHash) {
			writeAPIError(w, http.StatusConflict, requestKeyUnavailableCode, "a request with this Idempotency-Key was already recorded, but its result is not available")
		}
		return
	}
	if buffered.status >= 200 && buffered.status < 300 {
		if !claim.Claimed() {
			// Every financial create writes an audit event in its own
			// transaction; a success that stored no key would let a repeat
			// write again.
			logger.ErrorContext(r.Context(), "create succeeded without storing its request key", slog.String("route", r.Pattern))
		} else if err := service.RecordResponse(r.Context(), claim, buffered.status, buffered.body.String()); err != nil {
			// The write is committed and its key stored, so a repeat is
			// still refused; it reports the result as unavailable.
			logger.ErrorContext(r.Context(), "record request key response", slog.String("route", r.Pattern), slog.Any("err", err))
		}
	}
	buffered.flush(w)
}

// answerFromStoredKey answers a request whose key is already stored and
// reports whether it did.
func answerFromStoredKey(logger *slog.Logger, w http.ResponseWriter, r *http.Request, service *app.RequestKeyService, key, requestHash string) bool {
	stored, found, err := service.Lookup(r.Context(), key)
	if err != nil {
		writeServiceInternalError(w, r, logger, "read request key", err)
		return true
	}
	if !found {
		return false
	}
	switch {
	case stored.RequestHash != requestHash:
		writeAPIError(w, http.StatusConflict, requestKeyReusedCode, "this Idempotency-Key was already used for a different request")
	case !stored.HasResponse:
		writeAPIError(w, http.StatusConflict, requestKeyUnavailableCode, "a request with this Idempotency-Key was already recorded, but its result is not available")
	default:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set(idempotentReplayedHeader, "true")
		w.WriteHeader(stored.ResponseStatus)
		if _, err := io.WriteString(w, stored.ResponseBody); err != nil {
			logger.ErrorContext(r.Context(), "write replayed response", slog.Any("err", err))
		}
	}
	return true
}

// bufferedResponse holds a handler's response until its result is stored.
type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newBufferedResponse() *bufferedResponse {
	return &bufferedResponse{header: http.Header{}}
}

func (b *bufferedResponse) Header() http.Header { return b.header }

func (b *bufferedResponse) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}

func (b *bufferedResponse) Write(data []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.body.Write(data)
}

func (b *bufferedResponse) flush(w http.ResponseWriter) {
	maps.Copy(w.Header(), b.header)
	status := b.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write(b.body.Bytes())
}
