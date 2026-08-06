package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"rekenraam/backend/internal/db"

	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSetupRequired      = errors.New("setup required")
	ErrRateLimited        = errors.New("rate limited")
)

// dummyPasswordHash is a precomputed argon2id hash used to ensure the "user not
// found" code path takes the same time as "wrong password", preventing timing
// attacks that could distinguish the two outcomes.
const dummyPasswordHash = `$argon2id$v=19$m=19456,t=2,p=1$yFKjVHLDHsBTRk6lkk88Zg$dJ6u65HlcVuyRD4M7ArLq5QvFzwFgceJMsq/DIucXd0`

// RateLimitError is returned when a login attempt is blocked by the throttle.
// It carries the remaining wait duration so callers can populate Retry-After.
// errors.Is(err, ErrRateLimited) returns true for RateLimitError values.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e RateLimitError) Error() string { return ErrRateLimited.Error() }
func (e RateLimitError) Is(target error) bool {
	return target == ErrRateLimited
}

const (
	loginThrottleMaxFailures = 5
	loginThrottleWindow      = 15 * time.Minute

	throttleScopeUsername = "username"
	throttleScopeClientIP = "client_ip"
)

type AuthService struct {
	repository      *db.AuthRepository
	logger          *slog.Logger
	now             func() time.Time
	sessionLifetime time.Duration
}

type loginThrottleScope struct {
	ScopeType string
	ScopeKey  string
}

type parsedPasswordHash struct {
	Version     int
	MemoryKiB   int
	Iterations  int
	Parallelism int
	Salt        []byte
	Hash        []byte
}

type SessionStatus struct {
	Authenticated bool
	SessionID     int64
	User          *Owner
}

type LoginInput struct {
	Username  string
	Password  string
	ClientIP  string
	RequestID string
}

type LogoutInput struct {
	Token     string
	ClientIP  string
	RequestID string
}

// Authentication event types and failure reasons (S-07). These are stable
// strings: the DB constrains event_type, and an operator's alerting keys off
// failure_reason.
const (
	authEventLoginSucceeded = "login_succeeded"
	authEventLoginFailed    = "login_failed"
	authEventLoginBlocked   = "login_blocked"
	authEventLogout         = "logout"

	authFailureUnknownUser        = "unknown_user"
	authFailureInvalidCredentials = "invalid_credentials"
	authFailureRateLimited        = "rate_limited"

	// authEventRetention bounds how long the authentication log is kept. It is
	// an operational log for spotting brute-force runs and reconstructing a
	// recent incident, not a permanent archive of who signed in when.
	authEventRetention = 90 * 24 * time.Hour
)

// AuthenticationEvent is one recorded authentication attempt or session end.
type AuthenticationEvent struct {
	ID            int64
	OccurredAt    string
	EventType     string
	Outcome       string
	Username      string
	UserID        *int64
	AuthSessionID *int64
	ClientIP      string
	FailureReason string
	RequestID     string
}

type AuthenticationEvents struct {
	Events  []AuthenticationEvent
	HasMore bool
	// FailedLast24h is the headline number: a spike here is the signal an
	// operator is looking for.
	FailedLast24h int
}

type LoginResult struct {
	User         Owner
	SessionToken string
}

func NewAuthService(repository *db.AuthRepository, logger *slog.Logger) *AuthService {
	return NewAuthServiceWithSessionLifetime(repository, logger, SessionLifetime)
}

func NewAuthServiceWithSessionLifetime(repository *db.AuthRepository, logger *slog.Logger, sessionLifetime time.Duration) *AuthService {
	if sessionLifetime <= 0 {
		sessionLifetime = SessionLifetime
	}
	return &AuthService{
		repository:      repository,
		logger:          logger,
		now:             time.Now,
		sessionLifetime: sessionLifetime,
	}
}

func (s *AuthService) Session(ctx context.Context, token string) (SessionStatus, error) {
	if strings.TrimSpace(token) == "" {
		return SessionStatus{Authenticated: false}, nil
	}

	userRecord, err := s.repository.ReadSessionUser(ctx, hashSessionToken(token), s.now().UTC().Format(time.RFC3339))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return SessionStatus{Authenticated: false}, nil
		}

		return SessionStatus{}, fmt.Errorf("read session user: %w", err)
	}

	return SessionStatus{
		Authenticated: true,
		SessionID:     userRecord.SessionID,
		User: &Owner{
			ID:       userRecord.ID,
			Username: userRecord.Username,
		},
	}, nil
}

func (s *AuthService) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	username := strings.TrimSpace(input.Username)
	if username == "" {
		return LoginResult{}, ValidationError{Message: "username is required"}
	}
	if err := validateOwnerUsername(username); err != nil {
		return LoginResult{}, err
	}
	if err := validateLoginPassword(input.Password); err != nil {
		return LoginResult{}, err
	}
	scopes := loginThrottleScopes(username, input.ClientIP)
	if blockedUntil, err := s.isLoginBlocked(ctx, scopes); err != nil {
		return LoginResult{}, fmt.Errorf("check login throttle: %w", err)
	} else if !blockedUntil.IsZero() {
		s.recordAuthEvent(ctx, db.AuthenticationEventParams{
			EventType: authEventLoginBlocked, Outcome: "failure", Username: username,
			ClientIP: input.ClientIP, FailureReason: authFailureRateLimited, RequestID: input.RequestID,
		})
		return LoginResult{}, RateLimitError{RetryAfter: time.Until(blockedUntil)}
	}

	ownerExists, err := s.repository.OwnerExists(ctx)
	if err != nil {
		return LoginResult{}, fmt.Errorf("check owner existence: %w", err)
	}
	if !ownerExists {
		return LoginResult{}, ErrSetupRequired
	}

	credentials, err := s.repository.ReadOwnerCredentials(ctx, username)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			// Constant-time dummy comparison prevents timing oracle distinguishing
			// "no such user" from "wrong password".
			_, _ = verifyPassword(input.Password, dummyPasswordHash)
			blockedUntil, err := s.recordLoginFailure(ctx, scopes)
			if err != nil {
				return LoginResult{}, fmt.Errorf("record login throttle failure: %w", err)
			}
			s.recordAuthEvent(ctx, db.AuthenticationEventParams{
				EventType: authEventLoginFailed, Outcome: "failure", Username: username,
				ClientIP: input.ClientIP, FailureReason: authFailureUnknownUser, RequestID: input.RequestID,
			})
			if !blockedUntil.IsZero() {
				return LoginResult{}, RateLimitError{RetryAfter: time.Until(blockedUntil)}
			}
			return LoginResult{}, ErrInvalidCredentials
		}

		return LoginResult{}, fmt.Errorf("read owner credentials: %w", err)
	}

	verified, err := verifyPassword(input.Password, credentials.PasswordHash)
	if err != nil {
		return LoginResult{}, fmt.Errorf("verify password: %w", err)
	}
	if !verified {
		blockedUntil, err := s.recordLoginFailure(ctx, scopes)
		if err != nil {
			return LoginResult{}, fmt.Errorf("record login throttle failure: %w", err)
		}
		s.recordAuthEvent(ctx, db.AuthenticationEventParams{
			EventType: authEventLoginFailed, Outcome: "failure", Username: username,
			UserID: credentials.ID, ClientIP: input.ClientIP,
			FailureReason: authFailureInvalidCredentials, RequestID: input.RequestID,
		})
		if !blockedUntil.IsZero() {
			return LoginResult{}, RateLimitError{RetryAfter: time.Until(blockedUntil)}
		}
		return LoginResult{}, ErrInvalidCredentials
	}

	if s.passwordNeedsRehash(ctx, credentials.PasswordHash) {
		passwordHash, err := hashPassword(input.Password)
		if err != nil {
			return LoginResult{}, fmt.Errorf("rehash owner password: %w", err)
		}

		if err := s.repository.UpdatePasswordHash(ctx, db.UpdatePasswordHashParams{
			UserID:       credentials.ID,
			PasswordHash: passwordHash,
			UpdatedAt:    s.now().UTC().Format(time.RFC3339),
		}); err != nil {
			return LoginResult{}, fmt.Errorf("update owner password hash: %w", err)
		}
	}

	sessionToken, sessionTokenHash, err := newSessionToken()
	if err != nil {
		return LoginResult{}, fmt.Errorf("create auth session token: %w", err)
	}

	now := s.now().UTC()
	createdAt := now.Format(time.RFC3339)
	sessionID, err := s.repository.CreateSession(ctx, db.CreateSessionParams{
		UserID:    credentials.ID,
		TokenHash: sessionTokenHash,
		CreatedAt: createdAt,
		ExpiresAt: sessionExpiresAt(now, s.sessionLifetime),
	})
	if err != nil {
		return LoginResult{}, fmt.Errorf("persist auth session: %w", err)
	}

	if err := s.clearLoginFailures(ctx, scopes); err != nil {
		return LoginResult{}, fmt.Errorf("clear login throttle: %w", err)
	}

	s.recordAuthEvent(ctx, db.AuthenticationEventParams{
		EventType: authEventLoginSucceeded, Outcome: "success", Username: credentials.Username,
		UserID: credentials.ID, AuthSessionID: sessionID, ClientIP: input.ClientIP,
		RequestID: input.RequestID,
	})

	return LoginResult{
		User: Owner{
			ID:       credentials.ID,
			Username: credentials.Username,
		},
		SessionToken: sessionToken,
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, input LogoutInput) error {
	token := strings.TrimSpace(input.Token)
	if token == "" {
		return nil
	}

	// Resolve the session before revoking it, so the event can name the user
	// and the session it ended. A token that no longer resolves is a no-op
	// logout and records nothing.
	tokenHash := hashSessionToken(token)
	now := s.now().UTC()
	session, sessionErr := s.repository.ReadSessionUser(ctx, tokenHash, now.Format(time.RFC3339))

	if err := s.repository.RevokeSession(ctx, tokenHash, now.Format(time.RFC3339)); err != nil {
		return fmt.Errorf("revoke auth session: %w", err)
	}

	if sessionErr == nil {
		s.recordAuthEvent(ctx, db.AuthenticationEventParams{
			EventType: authEventLogout, Outcome: "success", Username: session.Username,
			UserID: session.ID, AuthSessionID: session.SessionID, ClientIP: input.ClientIP,
			RequestID: input.RequestID,
		})
	}

	return nil
}

// AuthenticationEvents returns the recent authentication log for an operator,
// newest first, plus the 24-hour failure count that makes a brute-force run
// visible at a glance.
func (s *AuthService) AuthenticationEvents(ctx context.Context, limit int) (AuthenticationEvents, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	records, err := s.repository.ListAuthenticationEvents(ctx, limit+1)
	if err != nil {
		return AuthenticationEvents{}, fmt.Errorf("list authentication events: %w", err)
	}
	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	failedLast24h, err := s.repository.FailedLoginsSince(ctx, s.now().UTC().Add(-24*time.Hour).Format(time.RFC3339), "")
	if err != nil {
		return AuthenticationEvents{}, fmt.Errorf("count recent failed logins: %w", err)
	}

	events := make([]AuthenticationEvent, 0, len(records))
	for _, record := range records {
		events = append(events, AuthenticationEvent{
			ID:            record.ID,
			OccurredAt:    record.OccurredAt,
			EventType:     record.EventType,
			Outcome:       record.Outcome,
			Username:      record.Username,
			UserID:        nullableSQLInt64Ptr(record.UserID),
			AuthSessionID: nullableSQLInt64Ptr(record.AuthSessionID),
			ClientIP:      record.ClientIP,
			FailureReason: record.FailureReason,
			RequestID:     record.RequestID,
		})
	}

	return AuthenticationEvents{Events: events, HasMore: hasMore, FailedLast24h: failedLast24h}, nil
}

// recordAuthEvent writes the durable event and mirrors it to the structured
// log, so an operator shipping logs elsewhere sees the same thing without
// querying SQLite.
//
// It never returns an error: failing a successful login because its audit row
// could not be written would turn a logging fault into a lockout. The failure
// is logged loudly instead.
func (s *AuthService) recordAuthEvent(ctx context.Context, params db.AuthenticationEventParams) {
	if params.OccurredAt == "" {
		params.OccurredAt = s.now().UTC().Format(time.RFC3339)
	}
	if err := s.repository.RecordAuthenticationEvent(ctx, params); err != nil && s.logger != nil {
		s.logger.ErrorContext(ctx, "failed to record authentication event",
			slog.String("event_type", params.EventType), slog.Any("err", err))
	}
	if s.logger == nil {
		return
	}
	// No password material, no session token — the session id is enough to
	// correlate, and the username is already in the durable row.
	attrs := []any{
		slog.String("event_type", params.EventType),
		slog.String("outcome", params.Outcome),
		slog.String("client_ip", params.ClientIP),
		slog.String("request_id", params.RequestID),
	}
	if params.FailureReason != "" {
		attrs = append(attrs, slog.String("failure_reason", params.FailureReason))
	}
	if params.Outcome == "failure" {
		s.logger.WarnContext(ctx, "authentication event", attrs...)
		return
	}
	s.logger.InfoContext(ctx, "authentication event", attrs...)
}

func (s *AuthService) CleanupExpiredAndRevokedSessions(ctx context.Context) (int64, error) {
	deleted, err := s.repository.DeleteExpiredOrRevokedSessions(ctx, s.now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("delete expired or revoked auth sessions: %w", err)
	}

	return deleted, nil
}

func (s *AuthService) StartSessionCleanup(ctx context.Context, logger *slog.Logger) {
	if logger == nil {
		logger = s.logger
	}
	if logger == nil {
		logger = slog.Default()
	}

	go func() {
		s.cleanupExpiredAndRevokedSessions(ctx, logger)

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.cleanupExpiredAndRevokedSessions(ctx, logger)
			}
		}
	}()
}

func (s *AuthService) cleanupExpiredAndRevokedSessions(ctx context.Context, logger *slog.Logger) {
	deleted, err := s.CleanupExpiredAndRevokedSessions(ctx)
	if err != nil {
		logger.WarnContext(ctx, "clean up auth sessions", slog.Any("err", err))
		return
	}
	if deleted > 0 {
		logger.InfoContext(ctx, "cleaned up auth sessions", slog.Int64("deleted", deleted))
	}
	s.pruneAuthenticationEvents(ctx, logger)
}

// pruneAuthenticationEvents enforces authEventRetention. It rides the existing
// daily session-cleanup tick rather than adding a second timer.
func (s *AuthService) pruneAuthenticationEvents(ctx context.Context, logger *slog.Logger) {
	cutoff := s.now().UTC().Add(-authEventRetention).Format(time.RFC3339)
	deleted, err := s.repository.DeleteAuthenticationEventsBefore(ctx, cutoff)
	if err != nil {
		logger.WarnContext(ctx, "prune authentication events", slog.Any("err", err))
		return
	}
	if deleted > 0 {
		logger.InfoContext(ctx, "pruned authentication events", slog.Int64("deleted", deleted))
	}
}

func verifyPassword(password string, encodedHash string) (bool, error) {
	parsedHash, err := parsePasswordHash(encodedHash)
	if err != nil {
		return false, err
	}

	derivedHash := argon2.IDKey(
		[]byte(password),
		parsedHash.Salt,
		uint32(parsedHash.Iterations),
		uint32(parsedHash.MemoryKiB),
		uint8(parsedHash.Parallelism),
		uint32(len(parsedHash.Hash)),
	)
	return subtle.ConstantTimeCompare(parsedHash.Hash, derivedHash) == 1, nil
}

func parsePasswordHash(encodedHash string) (parsedPasswordHash, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return parsedPasswordHash{}, fmt.Errorf("invalid password hash format")
	}
	if parts[1] != "argon2id" {
		return parsedPasswordHash{}, fmt.Errorf("unsupported password hash algorithm: %s", parts[1])
	}

	version, err := readArgonParameter(parts[2], "v")
	if err != nil {
		return parsedPasswordHash{}, err
	}

	parameters := strings.Split(parts[3], ",")
	if len(parameters) != 3 {
		return parsedPasswordHash{}, fmt.Errorf("invalid argon2 parameter segment")
	}

	memoryKiB, err := readArgonParameter(parameters[0], "m")
	if err != nil {
		return parsedPasswordHash{}, err
	}
	iterations, err := readArgonParameter(parameters[1], "t")
	if err != nil {
		return parsedPasswordHash{}, err
	}
	parallelism, err := readArgonParameter(parameters[2], "p")
	if err != nil {
		return parsedPasswordHash{}, err
	}
	if err := validateArgonParameters(version, memoryKiB, iterations, parallelism); err != nil {
		return parsedPasswordHash{}, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return parsedPasswordHash{}, fmt.Errorf("decode argon2 salt: %w", err)
	}

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return parsedPasswordHash{}, fmt.Errorf("decode argon2 hash: %w", err)
	}

	return parsedPasswordHash{
		Version:     version,
		MemoryKiB:   memoryKiB,
		Iterations:  iterations,
		Parallelism: parallelism,
		Salt:        salt,
		Hash:        hash,
	}, nil
}

func validateArgonParameters(version, memoryKiB, iterations, parallelism int) error {
	if version < 0 || version > math.MaxUint32 {
		return fmt.Errorf("argon2 version out of range")
	}
	if memoryKiB <= 0 || memoryKiB > math.MaxUint32 {
		return fmt.Errorf("argon2 memory parameter out of range")
	}
	if iterations <= 0 || iterations > math.MaxUint32 {
		return fmt.Errorf("argon2 iterations parameter out of range")
	}
	if parallelism <= 0 || parallelism > math.MaxUint8 {
		return fmt.Errorf("argon2 parallelism parameter out of range")
	}
	return nil
}

func (s *AuthService) passwordNeedsRehash(ctx context.Context, encodedHash string) bool {
	parsedHash, err := parsePasswordHash(encodedHash)
	if err != nil {
		s.logger.WarnContext(ctx, "failed to parse password hash for rehash check", slog.Any("err", err))
		return false
	}

	return parsedHash.Version != argon2.Version ||
		parsedHash.MemoryKiB != argon2MemoryKiB ||
		parsedHash.Iterations != argon2Iterations ||
		parsedHash.Parallelism != argon2Parallelism ||
		len(parsedHash.Salt) != argon2SaltLength ||
		len(parsedHash.Hash) != argon2KeyLength
}

func (s *AuthService) isLoginBlocked(ctx context.Context, scopes []loginThrottleScope) (time.Time, error) {
	for _, scope := range scopes {
		blockedUntil, err := s.isScopeBlocked(ctx, scope)
		if err != nil {
			return time.Time{}, err
		}
		if !blockedUntil.IsZero() {
			return blockedUntil, nil
		}
	}

	return time.Time{}, nil
}

func (s *AuthService) recordLoginFailure(ctx context.Context, scopes []loginThrottleScope) (time.Time, error) {
	var latestBlock time.Time
	for _, scope := range scopes {
		blockedUntil, err := s.recordScopeLoginFailure(ctx, scope)
		if err != nil {
			return time.Time{}, err
		}
		if blockedUntil.After(latestBlock) {
			latestBlock = blockedUntil
		}
	}

	return latestBlock, nil
}

func (s *AuthService) clearLoginFailures(ctx context.Context, scopes []loginThrottleScope) error {
	for _, scope := range scopes {
		// Only clear the username-scoped throttle on success. The IP-scoped throttle
		// is intentionally kept: clearing it on a successful login from the same IP
		// would un-block an attacker sharing a NAT address with the legitimate user.
		if scope.ScopeType != throttleScopeUsername {
			continue
		}
		if err := s.repository.DeleteLoginThrottle(ctx, scope.ScopeType, scope.ScopeKey); err != nil {
			return fmt.Errorf("delete login throttle: %w", err)
		}
	}

	return nil

}

func (s *AuthService) isScopeBlocked(ctx context.Context, scope loginThrottleScope) (time.Time, error) {
	record, err := s.repository.ReadLoginThrottle(ctx, scope.ScopeType, scope.ScopeKey)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return time.Time{}, nil
		}

		return time.Time{}, fmt.Errorf("read login throttle: %w", err)
	}

	if !record.BlockedUntil.Valid {
		return time.Time{}, nil
	}

	blockedUntil, err := time.Parse(time.RFC3339, record.BlockedUntil.String)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse login throttle blocked_until: %w", err)
	}
	if blockedUntil.After(s.now()) {
		return blockedUntil, nil
	}

	// Best-effort cleanup: stale throttle record delete failure must not block login.
	_ = s.repository.DeleteLoginThrottle(ctx, scope.ScopeType, scope.ScopeKey)

	return time.Time{}, nil
}

func (s *AuthService) recordScopeLoginFailure(ctx context.Context, scope loginThrottleScope) (time.Time, error) {
	record, err := s.repository.ReadLoginThrottle(ctx, scope.ScopeType, scope.ScopeKey)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return time.Time{}, fmt.Errorf("read login throttle: %w", err)
	}

	now := s.now().UTC()
	failedAttempts := 1
	var blockedUntil *string
	var blockedUntilTime time.Time

	if err == nil {
		if record.BlockedUntil.Valid {
			existingBlockedUntil, parseErr := time.Parse(time.RFC3339, record.BlockedUntil.String)
			if parseErr != nil {
				return time.Time{}, fmt.Errorf("parse login throttle blocked_until: %w", parseErr)
			}
			if existingBlockedUntil.After(now) {
				blocked := existingBlockedUntil.Format(time.RFC3339)
				if upsertErr := s.repository.UpsertLoginThrottle(ctx, db.UpsertLoginThrottleParams{
					ScopeType:      scope.ScopeType,
					ScopeKey:       scope.ScopeKey,
					FailedAttempts: record.FailedAttempts,
					BlockedUntil:   &blocked,
					UpdatedAt:      now.Format(time.RFC3339),
				}); upsertErr != nil {
					return time.Time{}, fmt.Errorf("refresh active login throttle: %w", upsertErr)
				}
				return existingBlockedUntil, nil
			}
		}

		failedAttempts = record.FailedAttempts + 1
	}

	if failedAttempts >= loginThrottleMaxFailures {
		blockedUntilTime = now.Add(loginThrottleWindow)
		blocked := blockedUntilTime.Format(time.RFC3339)
		blockedUntil = &blocked
		failedAttempts = 0
	}

	if err := s.repository.UpsertLoginThrottle(ctx, db.UpsertLoginThrottleParams{
		ScopeType:      scope.ScopeType,
		ScopeKey:       scope.ScopeKey,
		FailedAttempts: failedAttempts,
		BlockedUntil:   blockedUntil,
		UpdatedAt:      now.Format(time.RFC3339),
	}); err != nil {
		return time.Time{}, fmt.Errorf("upsert login throttle: %w", err)
	}

	return blockedUntilTime, nil
}

func loginThrottleScopes(username string, clientIP string) []loginThrottleScope {
	scopes := []loginThrottleScope{{ScopeType: throttleScopeUsername, ScopeKey: username}}
	clientIP = strings.TrimSpace(clientIP)
	if clientIP != "" {
		scopes = append(scopes, loginThrottleScope{ScopeType: throttleScopeClientIP, ScopeKey: clientIP})
	}

	return scopes
}

func readArgonParameter(segment string, expectedKey string) (int, error) {
	key, value, ok := strings.Cut(segment, "=")
	if !ok || key != expectedKey {
		return 0, fmt.Errorf("invalid argon2 parameter %q", segment)
	}

	parsedValue, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse argon2 parameter %q: %w", expectedKey, err)
	}

	return parsedValue, nil
}

func hashSessionToken(token string) string {
	tokenHash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(tokenHash[:])
}
