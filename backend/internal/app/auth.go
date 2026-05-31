package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
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

const (
	loginThrottleMaxFailures = 5
	loginThrottleWindow      = 15 * time.Minute

	throttleScopeUsername = "username"
	throttleScopeClientIP = "client_ip"
)

type AuthService struct {
	repository *db.AuthRepository
	now        func() time.Time
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
	User          *Owner
}

type LoginInput struct {
	Username string
	Password string
	ClientIP string
}

type LoginResult struct {
	User         Owner
	SessionToken string
}

func NewAuthService(repository *db.AuthRepository) *AuthService {
	return &AuthService{
		repository: repository,
		now:        time.Now,
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
	if err := validateLoginPassword(input.Password); err != nil {
		return LoginResult{}, err
	}
	scopes := loginThrottleScopes(username, input.ClientIP)
	if blocked, err := s.isLoginBlocked(ctx, scopes); err != nil {
		return LoginResult{}, fmt.Errorf("check login throttle: %w", err)
	} else if blocked {
		return LoginResult{}, ErrRateLimited
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
			blocked, err := s.recordLoginFailure(ctx, scopes)
			if err != nil {
				return LoginResult{}, fmt.Errorf("record login throttle failure: %w", err)
			}
			if blocked {
				return LoginResult{}, ErrRateLimited
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
		blocked, err := s.recordLoginFailure(ctx, scopes)
		if err != nil {
			return LoginResult{}, fmt.Errorf("record login throttle failure: %w", err)
		}
		if blocked {
			return LoginResult{}, ErrRateLimited
		}
		return LoginResult{}, ErrInvalidCredentials
	}

	if passwordNeedsRehash(credentials.PasswordHash) {
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
	if err := s.repository.CreateSession(ctx, db.CreateSessionParams{
		UserID:    credentials.ID,
		TokenHash: sessionTokenHash,
		CreatedAt: createdAt,
		ExpiresAt: sessionExpiresAt(now),
	}); err != nil {
		return LoginResult{}, fmt.Errorf("persist auth session: %w", err)
	}

	if err := s.clearLoginFailures(ctx, scopes); err != nil {
		return LoginResult{}, fmt.Errorf("clear login throttle: %w", err)
	}

	return LoginResult{
		User: Owner{
			ID:       credentials.ID,
			Username: credentials.Username,
		},
		SessionToken: sessionToken,
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}

	if err := s.repository.RevokeSession(ctx, hashSessionToken(token), s.now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("revoke auth session: %w", err)
	}

	return nil
}

func verifyPassword(password string, encodedHash string) (bool, error) {
	parsedHash, err := parsePasswordHash(encodedHash)
	if err != nil {
		return false, err
	}

	derivedHash := argon2.IDKey([]byte(password), parsedHash.Salt, uint32(parsedHash.Iterations), uint32(parsedHash.MemoryKiB), uint8(parsedHash.Parallelism), uint32(len(parsedHash.Hash)))
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

func passwordNeedsRehash(encodedHash string) bool {
	parsedHash, err := parsePasswordHash(encodedHash)
	if err != nil {
		return false
	}

	return parsedHash.Version != argon2.Version ||
		parsedHash.MemoryKiB != argon2MemoryKiB ||
		parsedHash.Iterations != argon2Iterations ||
		parsedHash.Parallelism != argon2Parallelism ||
		len(parsedHash.Salt) != argon2SaltLength ||
		len(parsedHash.Hash) != argon2KeyLength
}

func (s *AuthService) isLoginBlocked(ctx context.Context, scopes []loginThrottleScope) (bool, error) {
	for _, scope := range scopes {
		blocked, err := s.isScopeBlocked(ctx, scope)
		if err != nil {
			return false, err
		}
		if blocked {
			return true, nil
		}
	}

	return false, nil
}

func (s *AuthService) recordLoginFailure(ctx context.Context, scopes []loginThrottleScope) (bool, error) {
	blocked := false
	for _, scope := range scopes {
		scopeBlocked, err := s.recordScopeLoginFailure(ctx, scope)
		if err != nil {
			return false, err
		}
		if scopeBlocked {
			blocked = true
		}
	}

	return blocked, nil
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

func (s *AuthService) isScopeBlocked(ctx context.Context, scope loginThrottleScope) (bool, error) {
	record, err := s.repository.ReadLoginThrottle(ctx, scope.ScopeType, scope.ScopeKey)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return false, nil
		}

		return false, fmt.Errorf("read login throttle: %w", err)
	}

	if !record.BlockedUntil.Valid {
		return false, nil
	}

	blockedUntil, err := time.Parse(time.RFC3339, record.BlockedUntil.String)
	if err != nil {
		return false, fmt.Errorf("parse login throttle blocked_until: %w", err)
	}
	if blockedUntil.After(s.now()) {
		return true, nil
	}

	// Best-effort cleanup: stale throttle record delete failure must not block login.
	_ = s.repository.DeleteLoginThrottle(ctx, scope.ScopeType, scope.ScopeKey)

	return false, nil
}

func (s *AuthService) recordScopeLoginFailure(ctx context.Context, scope loginThrottleScope) (bool, error) {
	record, err := s.repository.ReadLoginThrottle(ctx, scope.ScopeType, scope.ScopeKey)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return false, fmt.Errorf("read login throttle: %w", err)
	}

	now := s.now().UTC()
	failedAttempts := 1
	var blockedUntil *string

	if err == nil {
		if record.BlockedUntil.Valid {
			existingBlockedUntil, parseErr := time.Parse(time.RFC3339, record.BlockedUntil.String)
			if parseErr != nil {
				return false, fmt.Errorf("parse login throttle blocked_until: %w", parseErr)
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
					return false, fmt.Errorf("refresh active login throttle: %w", upsertErr)
				}
				return true, nil
			}
		}

		failedAttempts = record.FailedAttempts + 1
	}

	if failedAttempts >= loginThrottleMaxFailures {
		blocked := now.Add(loginThrottleWindow).Format(time.RFC3339)
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
		return false, fmt.Errorf("upsert login throttle: %w", err)
	}

	return blockedUntil != nil, nil
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
