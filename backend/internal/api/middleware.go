package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
)

const requestIDHeader = "X-Request-ID"

type requestIDKey struct{}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAPIError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, errorResponse{
		Error: errorBody{
			Code:    code,
			Message: message,
		},
	})
}

func RequestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey{}).(string)
	return requestID
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.NewString()
		w.Header().Set(requestIDHeader, requestID)

		ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func withSecurityHeaders(options HandlerOptions, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("X-Frame-Options", "DENY")
		// NOTE: script-src includes 'unsafe-inline' because SvelteKit adapter-static injects
		// an inline bootstrapping script into index.html at build time. Removing 'unsafe-inline'
		// requires computing a SHA-256 hash of that script at build time and embedding it here;
		// that work is deferred. See docs/conventions.md for the planned approach.
		header.Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; frame-ancestors 'none'; object-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'self'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; form-action 'self'")
		header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		if requestUsesHTTPS(r, options) {
			header.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}

		if strings.HasPrefix(r.URL.Path, "/api/v1/auth/") || strings.HasPrefix(r.URL.Path, "/api/v1/setup/") || strings.HasPrefix(r.URL.Path, "/api/v1/books/") || strings.HasPrefix(r.URL.Path, "/api/v1/currencies") || strings.HasPrefix(r.URL.Path, "/api/v1/institutions") {
			header.Set("Cache-Control", "no-store")
			header.Set("Pragma", "no-cache")
		}

		next.ServeHTTP(w, r)
	})
}

func withRequestLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		recorder := &responseRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)

		logger.LogAttrs(r.Context(), logLevelForStatus(recorder.status), "request completed",
			slog.String("request_id", RequestIDFromContext(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", recorder.status),
			slog.Int("bytes", recorder.bytes),
			slog.Duration("duration", time.Since(startedAt)),
		)
	})
}

func withRecovery(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}

			logger.ErrorContext(r.Context(), "panic recovered",
				slog.String("request_id", RequestIDFromContext(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("panic_type", fmt.Sprintf("%T", recovered)),
				slog.String("stack", string(debug.Stack())),
			)

			if writer, ok := w.(interface{ Written() bool }); ok && writer.Written() {
				return
			}

			writeInternalServerError(w, r)
		}()

		next.ServeHTTP(w, r)
	})
}

func writeInternalServerError(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func logLevelForStatus(status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

type responseRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (r *responseRecorder) Header() http.Header {
	return r.ResponseWriter.Header()
}

func (r *responseRecorder) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status = status
		r.wroteHeader = true
	}

	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}

	bytesWritten, err := r.ResponseWriter.Write(data)
	r.bytes += bytesWritten
	return bytesWritten, err
}

func (r *responseRecorder) ReadFrom(reader io.Reader) (int64, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}

	readerFrom, ok := r.ResponseWriter.(io.ReaderFrom)
	if !ok {
		bytesWritten, err := io.Copy(r.ResponseWriter, reader)
		r.bytes += int(bytesWritten)
		return bytesWritten, err
	}

	bytesWritten, err := readerFrom.ReadFrom(reader)
	r.bytes += int(bytesWritten)
	return bytesWritten, err
}

func (r *responseRecorder) Flush() {
	flusher, ok := r.ResponseWriter.(http.Flusher)
	if ok {
		flusher.Flush()
	}
}

func (r *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}

	return hijacker.Hijack()
}

func (r *responseRecorder) Push(target string, opts *http.PushOptions) error {
	pusher, ok := r.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}

	return pusher.Push(target, opts)
}

func (r *responseRecorder) Written() bool {
	return r.wroteHeader
}
