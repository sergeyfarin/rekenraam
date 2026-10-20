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
	// ErrTrustedDeviceNotFound is returned when revoking a device that does
	// not exist, is already revoked, or belongs to another user.
	ErrTrustedDeviceNotFound = errors.New("trusted device not found")
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

	throttleScopeUsername      = "username"
	throttleScopeClientIP      = "client_ip"
	throttleScopeTrustedDevice = "trusted_device"
	// throttleScopeReauthSession budgets password and activation-code
	// confirmations on the MFA settings, per session (#262).
	throttleScopeReauthSession = "reauth_session"

	// TrustedDeviceLifetime is how long an approved device stays approved
	// without being used. Every successful login on the device slides it
	// forward, so a device in regular use never lapses and an abandoned one
	// does (S-04).
	TrustedDeviceLifetime = 180 * 24 * time.Hour
)

type AuthService struct {
	repository      *db.AuthRepository
	logger          *slog.Logger
	now             func() time.Time
	sessionLifetime time.Duration
	// secretKey seals the TOTP shared secret at rest (S-06). nil = not
	// configured, which refuses MFA enrollment rather than storing the secret
	// in the clear.
	secretKey []byte
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
	// TrustedDeviceToken is the approved-device cookie value, if the browser
	// presented one. It is a throttle-scope selector, never a credential:
	// possessing it lets the attempt use its own throttle budget instead of
	// the shared username/IP budgets, and grants nothing else.
	TrustedDeviceToken string
}

type LoginResult struct {
	User            Owner
	SessionToken    string
	SessionLifetime time.Duration
	// TrustedDeviceToken is set when the caller should store a new
	// approved-device cookie. Empty means the device it presented is still
	// valid; the HTTP layer renews its cookie using the presented token.
	TrustedDeviceToken string
	// TrustedDeviceLifetime is how long the caller should keep the cookie.
	TrustedDeviceLifetime time.Duration
	// MFARequired reports that the password verified but a second factor is
	// still owed (S-06). SessionToken is empty in that case: the caller must
	// carry MFAChallengeToken to CompleteLoginMFA.
	MFARequired bool
	// MFAChallengeToken identifies the half-authenticated attempt. It is not a
	// credential and grants nothing on its own.
	MFAChallengeToken    string
	MFAChallengeLifetime time.Duration
}

// TrustedDevice is an approved device as shown to the owner.
type TrustedDevice struct {
	ID              int64
	CreatedAt       string
	LastUsedAt      string
	ExpiresAt       string
	CreatedClientIP string
	// Current marks the device making this request, so the owner does not
	// revoke the one they are sitting at by accident.
	Current bool
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

	// Account-security changes (#288). Each is written in the same transaction
	// as the change it records, so the change cannot happen unrecorded. A
	// sign-in event stays best-effort (recordAuthEvent): its failure must not
	// become a lockout, while a security change that cannot be recorded is
	// simply retried.
	authEventMFAEnrollmentStarted        = "mfa_enrollment_started"
	authEventMFAActivated                = "mfa_activated"
	authEventMFADisabled                 = "mfa_disabled"
	authEventMFARecoveryCodesRegenerated = "mfa_recovery_codes_regenerated"
	authEventTrustedDeviceRevoked        = "trusted_device_revoked"

	// Second factors recorded on login_succeeded, so a recovery-code sign-in
	// is never mistaken for a TOTP one.
	secondFactorTOTP         = "totp"
	secondFactorRecoveryCode = "recovery_code"

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
	Origin        string
	SecondFactor  string
	// MetadataJSON is a JSON object of non-secret context, "{}" when empty.
	MetadataJSON string
}

type AuthenticationEvents struct {
	Events  []AuthenticationEvent
	HasMore bool
	// FailedLast24h is the headline number: a spike here is the signal an
	// operator is looking for.
	FailedLast24h int
}

func NewAuthService(repository *db.AuthRepository, logger *slog.Logger) *AuthService {
	return NewAuthServiceWithSessionLifetime(repository, logger, SessionLifetime)
}

// SetSecretKey supplies the key that seals MFA secrets at rest. Called during
// wiring; without it, MFA enrollment returns ErrSecretKeyMissing.
func (s *AuthService) SetSecretKey(secretKey []byte) {
	s.secretKey = secretKey
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
	// The query may have waited for the pool. Admission uses the clock after
	// the read, without taking a SQLite writer lock for a session lookup.
	if !authDeadlineOpen(userRecord.ExpiresAt, s.now()) {
		return SessionStatus{Authenticated: false}, nil
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
	scopes, trustedDevice, deviceTrusted := s.loginThrottleScopesForAttempt(ctx, input, username)
	reservation, err := s.reserveAttempt(ctx, scopes)
	if err != nil {
		if errors.Is(err, ErrRateLimited) {
			s.recordAuthEvent(ctx, db.AuthenticationEventParams{
				EventType: authEventLoginBlocked, Outcome: "failure", Username: username,
				ClientIP: input.ClientIP, FailureReason: authFailureRateLimited, RequestID: input.RequestID,
			})
		}
		return LoginResult{}, err
	}
	defer reservation.release(ctx)

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
			retryAfter, err := reservation.fail(ctx)
			if err != nil {
				return LoginResult{}, err
			}
			s.recordAuthEvent(ctx, db.AuthenticationEventParams{
				EventType: authEventLoginFailed, Outcome: "failure", Username: username,
				ClientIP: input.ClientIP, FailureReason: authFailureUnknownUser, RequestID: input.RequestID,
			})
			if retryAfter > 0 {
				return LoginResult{}, RateLimitError{RetryAfter: retryAfter}
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
		retryAfter, err := reservation.fail(ctx)
		if err != nil {
			return LoginResult{}, err
		}
		s.recordAuthEvent(ctx, db.AuthenticationEventParams{
			EventType: authEventLoginFailed, Outcome: "failure", Username: username,
			UserID: credentials.ID, ClientIP: input.ClientIP,
			FailureReason: authFailureInvalidCredentials, RequestID: input.RequestID,
		})
		if retryAfter > 0 {
			return LoginResult{}, RateLimitError{RetryAfter: retryAfter}
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

	// The password is proved; whether that is a login depends on the second
	// factor (S-06). With MFA active this attempt gets a short-lived challenge
	// and no session, no device approval, and no throttle reset — none of
	// those are earned until the second factor verifies. Its throttle place is
	// given back: a correct password is not a failure.
	mfaActive, err := s.mfaActive(ctx, credentials.ID)
	if err != nil {
		return LoginResult{}, err
	}
	if mfaActive {
		challengeToken, challengeLifetime, err := s.mfaChallengeFor(ctx, credentials.ID, input.ClientIP)
		if err != nil {
			return LoginResult{}, err
		}
		return LoginResult{
			User:                 Owner{ID: credentials.ID, Username: credentials.Username},
			MFARequired:          true,
			MFAChallengeToken:    challengeToken,
			MFAChallengeLifetime: challengeLifetime,
		}, nil
	}

	return s.completeLogin(ctx, completeLoginInput{
		UserID:        credentials.ID,
		Username:      credentials.Username,
		ClientIP:      input.ClientIP,
		RequestID:     input.RequestID,
		Reservation:   reservation,
		TrustedDevice: trustedDevice,
		DeviceTrusted: deviceTrusted,
	})
}

// completeLoginInput carries the state a verified attempt needs to become a
// session. It exists so the password-only and password-plus-MFA paths issue
// sessions through exactly one piece of code — a second factor that skipped
// the throttle reset or the device approval would be a subtle mess.
type completeLoginInput struct {
	UserID        int64
	Username      string
	ClientIP      string
	RequestID     string
	Reservation   *throttleReservation
	TrustedDevice db.TrustedDeviceRecord
	DeviceTrusted bool
	// SecondFactor is the factor that completed an MFA sign-in, empty for a
	// password-only one.
	SecondFactor     string
	MFAChallengeHash string
}

func (s *AuthService) completeLogin(ctx context.Context, input completeLoginInput) (LoginResult, error) {
	sessionToken, sessionTokenHash, err := newSessionToken()
	if err != nil {
		return LoginResult{}, fmt.Errorf("create auth session token: %w", err)
	}

	now := s.now().UTC()
	createdAt := now.Format(time.RFC3339)
	clears, refunds := input.Reservation.successSettlement()
	params := db.CompleteLoginParams{
		Session: db.CreateSessionParams{
			UserID:    input.UserID,
			TokenHash: sessionTokenHash,
			CreatedAt: createdAt,
			ExpiresAt: sessionExpiresAt(now, s.sessionLifetime),
		},
		ClearThrottles:   clears,
		RefundThrottles:  refunds,
		MFAChallengeHash: input.MFAChallengeHash,
	}
	// Approve the device only now, after every factor actually verified.
	trustedDeviceToken := ""
	deviceExpiresAt := now.Add(TrustedDeviceLifetime).Format(time.RFC3339)
	if input.DeviceTrusted {
		params.TouchDeviceID = input.TrustedDevice.ID
		params.DeviceUsedAt = createdAt
		params.DeviceExpiresAt = deviceExpiresAt
	} else {
		var deviceTokenHash string
		trustedDeviceToken, deviceTokenHash, err = newSessionToken()
		if err != nil {
			return LoginResult{}, fmt.Errorf("create trusted device token: %w", err)
		}
		params.NewDevice = &db.CreateTrustedDeviceParams{
			UserID:          input.UserID,
			TokenHash:       deviceTokenHash,
			CreatedAt:       createdAt,
			ExpiresAt:       deviceExpiresAt,
			CreatedClientIP: input.ClientIP,
		}
	}
	// One transaction (#287): a rejected attempt leaves no session behind.
	sessionID, err := s.repository.CompleteLogin(ctx, params)
	if err != nil {
		if input.MFAChallengeHash != "" && errors.Is(err, db.ErrNotFound) {
			return LoginResult{}, ErrMFAChallengeInvalid
		}
		return LoginResult{}, fmt.Errorf("persist auth session: %w", err)
	}
	input.Reservation.settled = true

	s.recordAuthEvent(ctx, db.AuthenticationEventParams{
		EventType: authEventLoginSucceeded, Outcome: "success", Username: input.Username,
		UserID: input.UserID, AuthSessionID: sessionID, ClientIP: input.ClientIP,
		RequestID: input.RequestID, SecondFactor: input.SecondFactor,
	})

	return LoginResult{
		User: Owner{
			ID:       input.UserID,
			Username: input.Username,
		},
		SessionToken:          sessionToken,
		SessionLifetime:       s.sessionLifetime,
		TrustedDeviceToken:    trustedDeviceToken,
		TrustedDeviceLifetime: TrustedDeviceLifetime,
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
			Origin:        record.Origin,
			SecondFactor:  record.SecondFactor,
			MetadataJSON:  record.MetadataJSON,
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
	s.logAuthEvent(ctx, params)
}

// securityEvent builds the attributed event an account-security change writes
// inside its own transaction (#288).
func (s *AuthService) securityEvent(eventType string, userID int64, sessionID int64, clientIP string, requestID string) db.AuthenticationEventParams {
	return db.AuthenticationEventParams{
		OccurredAt: s.now().UTC().Format(time.RFC3339), EventType: eventType, Outcome: "success",
		UserID: userID, AuthSessionID: sessionID, ClientIP: clientIP, RequestID: requestID,
	}
}

// logAuthEvent mirrors an event to the structured log.
func (s *AuthService) logAuthEvent(ctx context.Context, params db.AuthenticationEventParams) {
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
	if params.SecondFactor != "" {
		attrs = append(attrs, slog.String("second_factor", params.SecondFactor))
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

func (s *AuthService) StartSessionCleanup(ctx context.Context, logger *slog.Logger) <-chan struct{} {
	done := make(chan struct{})
	if logger == nil {
		logger = s.logger
	}
	if logger == nil {
		logger = slog.Default()
	}

	go func() {
		defer close(done)
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
	return done
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
	s.pruneTrustedDevices(ctx, logger)
	s.pruneMFAChallenges(ctx, logger)
}

// pruneTrustedDevices drops expired and revoked approved-device rows on the
// same daily tick.
func (s *AuthService) pruneTrustedDevices(ctx context.Context, logger *slog.Logger) {
	deleted, err := s.repository.DeleteExpiredOrRevokedTrustedDevices(ctx, s.now().UTC().Format(time.RFC3339))
	if err != nil {
		logger.WarnContext(ctx, "prune trusted devices", slog.Any("err", err))
		return
	}
	if deleted > 0 {
		logger.InfoContext(ctx, "pruned trusted devices", slog.Int64("deleted", deleted))
	}
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

func loginThrottleScopes(username string, clientIP string) []loginThrottleScope {
	scopes := []loginThrottleScope{{ScopeType: throttleScopeUsername, ScopeKey: username}}
	clientIP = strings.TrimSpace(clientIP)
	if clientIP != "" {
		scopes = append(scopes, loginThrottleScope{ScopeType: throttleScopeClientIP, ScopeKey: clientIP})
	}

	return scopes
}

// resolveTrustedDevice looks up the presented approved-device token. A miss —
// no cookie, unknown, expired, revoked, or belonging to a different user — is
// not an error: the attempt simply falls back to the shared throttle scopes.
//
// The device is bound to a user id, so a device approved for one account can
// never lend its throttle budget to guesses against another.
func (s *AuthService) resolveTrustedDevice(ctx context.Context, token string, username string) (db.TrustedDeviceRecord, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return db.TrustedDeviceRecord{}, false
	}
	device, err := s.repository.ReadTrustedDevice(ctx, hashSessionToken(token), s.now().UTC().Format(time.RFC3339))
	if err != nil {
		if !errors.Is(err, db.ErrNotFound) && s.logger != nil {
			s.logger.WarnContext(ctx, "failed to read trusted device", slog.Any("err", err))
		}
		return db.TrustedDeviceRecord{}, false
	}
	credentials, err := s.repository.ReadOwnerCredentials(ctx, username)
	if err != nil || credentials.ID != device.UserID {
		return db.TrustedDeviceRecord{}, false
	}
	if !authDeadlineOpen(device.ExpiresAt, s.now()) {
		return db.TrustedDeviceRecord{}, false
	}

	return device, true
}

func authDeadlineOpen(deadline string, now time.Time) bool {
	expires, err := time.Parse(time.RFC3339, deadline)
	return err == nil && now.Before(expires)
}

// loginThrottleScopesForAttempt is the S-04 fix. A single-owner app publishes
// its owner's username by construction, so a username-scoped throttle is a
// remote lockout switch: any attacker can fail five logins and keep the owner
// out of their own finances for as long as they care to keep trying.
//
// An attempt from a device that has previously completed a successful login is
// therefore throttled on that device's own scope instead of the shared
// username and IP scopes — a budget nobody else can fill, so an attacker can
// no longer lock the owner out. The device scope keeps the same 5-in-15
// budget, so a stolen cookie still buys no extra password guesses; it only
// isolates the blast radius.
//
// Everything else keeps the original behaviour: an unrecognised device is
// throttled by username and IP exactly as before.
func (s *AuthService) loginThrottleScopesForAttempt(ctx context.Context, input LoginInput, username string) ([]loginThrottleScope, db.TrustedDeviceRecord, bool) {
	device, trusted := s.resolveTrustedDevice(ctx, input.TrustedDeviceToken, username)
	if !trusted {
		return loginThrottleScopes(username, input.ClientIP), db.TrustedDeviceRecord{}, false
	}

	return []loginThrottleScope{{
		ScopeType: throttleScopeTrustedDevice,
		ScopeKey:  strconv.FormatInt(device.ID, 10),
	}}, device, true
}

// ListTrustedDevices shows the owner which devices carry a throttle bypass.
func (s *AuthService) ListTrustedDevices(ctx context.Context, userID int64, currentDeviceToken string) ([]TrustedDevice, error) {
	now := s.now().UTC().Format(time.RFC3339)
	records, err := s.repository.ListTrustedDevices(ctx, userID, now)
	if err != nil {
		return nil, fmt.Errorf("list trusted devices: %w", err)
	}
	currentDeviceID := int64(0)
	if token := strings.TrimSpace(currentDeviceToken); token != "" {
		if current, err := s.repository.ReadTrustedDevice(ctx, hashSessionToken(token), now); err == nil {
			currentDeviceID = current.ID
		}
	}

	devices := make([]TrustedDevice, 0, len(records))
	for _, record := range records {
		devices = append(devices, TrustedDevice{
			ID:              record.ID,
			CreatedAt:       record.CreatedAt,
			LastUsedAt:      record.LastUsedAt,
			ExpiresAt:       record.ExpiresAt,
			CreatedClientIP: record.CreatedClientIP,
			Current:         record.ID == currentDeviceID,
		})
	}

	return devices, nil
}

type RevokeTrustedDeviceInput struct {
	UserID    int64
	SessionID int64
	DeviceID  int64
	ClientIP  string
	RequestID string
}

// RevokeTrustedDevice removes a device's throttle bypass. It does not end that
// device's session: the cookie was never a credential. The revocation and its
// attributed event commit together (#288).
func (s *AuthService) RevokeTrustedDevice(ctx context.Context, input RevokeTrustedDeviceInput) error {
	if input.DeviceID <= 0 {
		return ValidationError{Message: "trusted device id is required"}
	}
	event := s.securityEvent(authEventTrustedDeviceRevoked, input.UserID, input.SessionID, input.ClientIP, input.RequestID)
	event.MetadataJSON = fmt.Sprintf(`{"trusted_device_id":%d}`, input.DeviceID)
	if err := s.repository.RevokeTrustedDevice(ctx, input.UserID, input.DeviceID, event.OccurredAt, event); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ErrTrustedDeviceNotFound
		}
		return fmt.Errorf("revoke trusted device: %w", err)
	}
	s.logAuthEvent(ctx, event)

	return nil
}

// ApproveDeviceForUser approves the device that just completed first-run owner
// setup. Without this the brand-new install has no approved device at all, so
// an attacker could start locking the owner's username out before they have
// ever had the chance to earn one (S-04).
func (s *AuthService) ApproveDeviceForUser(ctx context.Context, userID int64, clientIP string) (string, time.Duration, error) {
	if userID <= 0 {
		return "", 0, ValidationError{Message: "user is required"}
	}
	token, err := s.approveDevice(ctx, userID, clientIP)
	if err != nil {
		return "", 0, err
	}

	return token, TrustedDeviceLifetime, nil
}

// approveDevice issues a new approved-device token. Called only after a
// password has actually verified, so approval always follows proof of the
// credential. Sign-in approves inside its own transaction (completeLogin).
func (s *AuthService) approveDevice(ctx context.Context, userID int64, clientIP string) (string, error) {
	now := s.now().UTC()
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return "", fmt.Errorf("create trusted device token: %w", err)
	}
	if err := s.repository.CreateTrustedDevice(ctx, db.CreateTrustedDeviceParams{
		UserID:          userID,
		TokenHash:       tokenHash,
		CreatedAt:       now.Format(time.RFC3339),
		ExpiresAt:       now.Add(TrustedDeviceLifetime).Format(time.RFC3339),
		CreatedClientIP: clientIP,
	}); err != nil {
		return "", fmt.Errorf("create trusted device: %w", err)
	}

	return token, nil
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
