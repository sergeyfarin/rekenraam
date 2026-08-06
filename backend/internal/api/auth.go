package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"rekenraam/backend/internal/app"
)

const csrfTokenHeader = "X-CSRF-Token"

type authenticatedOwnerKey struct{}
type authenticatedSessionIDKey struct{}

type authSessionResponse struct {
	Authenticated bool           `json:"authenticated"`
	User          *OwnerResponse `json:"user,omitempty"`
	CSRFToken     string         `json:"csrf_token,omitempty"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	User OwnerResponse `json:"user"`
}

func sessionStatus(logger *slog.Logger, authService *app.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := readSessionToken(r)
		status, err := authService.Session(r.Context(), token)
		if err != nil {
			writeAuthServiceError(w, r, logger, "read auth session", err)
			return
		}

		response := authSessionResponse{Authenticated: status.Authenticated}
		if status.User != nil {
			response.User = &OwnerResponse{
				ID:       status.User.ID,
				Username: status.User.Username,
			}
			response.CSRFToken = deriveCSRFToken(token)
		}

		writeJSON(w, http.StatusOK, response)
	}
}

func login(logger *slog.Logger, authService *app.AuthService, options HandlerOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request loginRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		result, err := authService.Login(r.Context(), app.LoginInput{
			Username:  request.Username,
			Password:  request.Password,
			ClientIP:  loginClientIP(r, options),
			RequestID: RequestIDFromContext(r.Context()),
		})
		if err != nil {
			writeAuthServiceError(w, r, logger, "login owner", err)
			return
		}

		writeSessionCookie(w, r, options, result.SessionToken)
		writeJSON(w, http.StatusOK, loginResponse{
			User: OwnerResponse{ID: result.User.ID, Username: result.User.Username},
		})
	}
}

func logout(logger *slog.Logger, authService *app.AuthService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := authService.Logout(r.Context(), app.LogoutInput{
			Token:     readSessionToken(r),
			ClientIP:  loginClientIP(r, options),
			RequestID: RequestIDFromContext(r.Context()),
		}); err != nil {
			writeAuthServiceError(w, r, logger, "logout owner", err)
			return
		}

		clearSessionCookie(w, r, options)
		w.WriteHeader(http.StatusNoContent)
	}))
}

type authenticationEventResponse struct {
	ID            int64  `json:"id"`
	OccurredAt    string `json:"occurred_at"`
	EventType     string `json:"event_type"`
	Outcome       string `json:"outcome"`
	Username      string `json:"username,omitempty"`
	UserID        *int64 `json:"user_id,omitempty"`
	AuthSessionID *int64 `json:"auth_session_id,omitempty"`
	ClientIP      string `json:"client_ip,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
	RequestID     string `json:"request_id,omitempty"`
}

type authenticationEventsResponse struct {
	Events  []authenticationEventResponse `json:"events"`
	HasMore bool                          `json:"has_more"`
	// FailedLast24h makes a brute-force run visible without scanning the list.
	FailedLast24h int `json:"failed_last_24h"`
}

// authenticationEvents exposes the authentication log to the owner (S-07).
// Owner-only: the log names client IPs and attempted usernames, so it is not
// something an unauthenticated caller may read.
func authenticationEvents(logger *slog.Logger, authService *app.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		limit := 50
		if raw := r.URL.Query().Get("limit"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 {
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "limit is invalid")
				return
			}
			limit = parsed
		}
		events, err := authService.AuthenticationEvents(r.Context(), limit)
		if err != nil {
			writeAuthServiceError(w, r, logger, "list authentication events", err)
			return
		}
		responses := make([]authenticationEventResponse, 0, len(events.Events))
		for _, event := range events.Events {
			responses = append(responses, authenticationEventResponse{
				ID: event.ID, OccurredAt: event.OccurredAt, EventType: event.EventType,
				Outcome: event.Outcome, Username: event.Username, UserID: event.UserID,
				AuthSessionID: event.AuthSessionID, ClientIP: event.ClientIP,
				FailureReason: event.FailureReason, RequestID: event.RequestID,
			})
		}
		writeJSON(w, http.StatusOK, authenticationEventsResponse{
			Events: responses, HasMore: events.HasMore, FailedLast24h: events.FailedLast24h,
		})
	}
}

func writeAuthServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, action string, err error) {
	var validationError app.ValidationError
	var rateLimitError app.RateLimitError
	switch {
	case errors.As(err, &validationError):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationError.Error())
	case errors.As(err, &rateLimitError):
		retryAfterSecs := int(math.Ceil(rateLimitError.RetryAfter.Seconds()))
		if retryAfterSecs < 1 {
			retryAfterSecs = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterSecs))
		writeAPIError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many login attempts, try again later")
	case errors.Is(err, app.ErrSetupRequired):
		writeAPIError(w, http.StatusConflict, "SETUP_REQUIRED", "owner setup is required before login")
	case errors.Is(err, app.ErrInvalidCredentials):
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "invalid username or password")
	default:
		writeServiceInternalError(w, r, logger, action, err)
	}
}

func readSessionToken(r *http.Request) string {
	for _, name := range []string{secureSessionCookieName, sessionCookieName} {
		cookie, err := r.Cookie(name)
		if err == nil {
			return strings.TrimSpace(cookie.Value)
		}
	}

	return ""
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request, options HandlerOptions) {
	secure := requestUsesHTTPS(r, options)
	clearCookie(w, requestSessionCookieName(r, options), secure)
	if requestSessionCookieName(r, options) != sessionCookieName {
		clearCookie(w, sessionCookieName, secure)
	}
}

func clearCookie(w http.ResponseWriter, name string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   true,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
	_ = secure
}

func requestUsesHTTPS(r *http.Request, options HandlerOptions) bool {
	if r.TLS != nil {
		return true
	}

	if !shouldTrustProxyHeaders(r, options) {
		return false
	}

	forwardedProto, _, _ := strings.Cut(r.Header.Get("X-Forwarded-Proto"), ",")
	return strings.EqualFold(strings.TrimSpace(forwardedProto), "https")
}

func requireSameOrigin(options HandlerOptions, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !originMatchesRequest(r, options) {
			writeAPIError(w, http.StatusForbidden, "CSRF_INVALID", "origin validation failed")
			return
		}

		next.ServeHTTP(w, r)
	}
}

func requireAuthenticatedMutation(logger *slog.Logger, authService *app.AuthService, options HandlerOptions, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if authService == nil {
			writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			return
		}

		token := readSessionToken(r)
		if token == "" {
			writeAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication is required")
			return
		}

		if !originMatchesRequest(r, options) || !csrfTokenMatches(token, r.Header.Get(csrfTokenHeader)) {
			writeAPIError(w, http.StatusForbidden, "CSRF_INVALID", "csrf validation failed")
			return
		}

		status, err := authService.Session(r.Context(), token)
		if err != nil {
			writeServiceInternalError(w, r, logger, "read auth session", err)
			return
		}
		if !status.Authenticated {
			writeAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication is required")
			return
		}

		ctx := r.Context()
		if status.User != nil {
			ctx = context.WithValue(ctx, authenticatedOwnerKey{}, *status.User)
		}
		if status.SessionID > 0 {
			ctx = context.WithValue(ctx, authenticatedSessionIDKey{}, status.SessionID)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func originMatchesRequest(r *http.Request, options HandlerOptions) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}

	allowedOrigins := []string{requestOrigin(r, options)}
	if shouldTrustProxyHeaders(r, options) {
		if forwardedHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwardedHost != "" {
			allowedOrigins = append(allowedOrigins, requestOriginForHost(r, forwardedHost, options))
		}
	}

	for _, allowedOrigin := range allowedOrigins {
		if subtle.ConstantTimeCompare([]byte(origin), []byte(allowedOrigin)) == 1 {
			return true
		}
	}

	return false
}

func requestOrigin(r *http.Request, options HandlerOptions) string {
	return requestOriginForHost(r, r.Host, options)
}

func requestOriginForHost(r *http.Request, host string, options HandlerOptions) string {
	scheme := "http"
	if shouldTrustProxyHeaders(r, options) {
		if forwardedProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwardedProto != "" {
			scheme = forwardedProto
		} else if r.TLS != nil {
			scheme = "https"
		}
	} else if r.TLS != nil {
		scheme = "https"
	}

	return (&url.URL{Scheme: scheme, Host: host}).String()
}

func csrfTokenMatches(token string, providedToken string) bool {
	providedToken = strings.TrimSpace(providedToken)
	if providedToken == "" {
		return false
	}

	expectedToken := deriveCSRFToken(token)
	return subtle.ConstantTimeCompare([]byte(expectedToken), []byte(providedToken)) == 1
}

func deriveCSRFToken(token string) string {
	tokenHash := sha256.Sum256([]byte("csrf:" + token))
	return base64.RawURLEncoding.EncodeToString(tokenHash[:])
}

func loginClientIP(r *http.Request, options HandlerOptions) string {
	if shouldTrustProxyHeaders(r, options) {
		if clientIP := trustedForwardedClientIP(r.Header.Get("X-Forwarded-For"), options.TrustedProxyCIDRs); clientIP != "" {
			return clientIP
		}
	}

	if host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil {
		if clientIP := canonicalizeIP(host); clientIP != "" {
			return clientIP
		}
	}

	return canonicalizeIP(r.RemoteAddr)
}

// trustedForwardedClientIP walks from the app-facing end of X-Forwarded-For
// towards the client. The direct peer was already allowlisted by
// shouldTrustProxyHeaders. Trusted proxy hops are skipped; the first remaining
// address is the client. Proxies must therefore overwrite or append XFF rather
// than passing a client-supplied header through unchanged.
func trustedForwardedClientIP(forwardedFor string, trustedProxyCIDRs []netip.Prefix) string {
	parts := strings.Split(forwardedFor, ",")
	for index := len(parts) - 1; index >= 0; index-- {
		clientIP := canonicalizeIP(parts[index])
		if clientIP == "" {
			return ""
		}

		address, err := netip.ParseAddr(clientIP)
		if err != nil {
			return ""
		}
		if isTrustedProxy(address.Unmap(), trustedProxyCIDRs) {
			continue
		}

		return clientIP
	}

	return ""
}

func shouldTrustProxyHeaders(r *http.Request, options HandlerOptions) bool {
	if !options.TrustProxyHeaders || len(options.TrustedProxyCIDRs) == 0 {
		return false
	}

	peerIP := requestPeerIP(r)
	if !peerIP.IsValid() {
		return false
	}

	return isTrustedProxy(peerIP, options.TrustedProxyCIDRs)
}

func isTrustedProxy(address netip.Addr, trustedProxyCIDRs []netip.Prefix) bool {
	for _, prefix := range trustedProxyCIDRs {
		if prefix.Contains(address) {
			return true
		}
	}

	return false
}

func requestPeerIP(r *http.Request) netip.Addr {
	if host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil {
		if addr, err := netip.ParseAddr(host); err == nil {
			return addr.Unmap()
		}
	}

	if addr, err := netip.ParseAddr(strings.TrimSpace(r.RemoteAddr)); err == nil {
		return addr.Unmap()
	}

	return netip.Addr{}
}

func canonicalizeIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	parsedIP := net.ParseIP(value)
	if parsedIP == nil {
		return ""
	}

	return parsedIP.String()
}
