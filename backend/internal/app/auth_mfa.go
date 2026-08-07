package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/secretbox"
	"rekenraam/backend/internal/totp"
)

var (
	// ErrMFAChallengeInvalid covers an unknown, expired, or already-spent
	// challenge. The three are one error on purpose: the client's only correct
	// response to any of them is to start the login again.
	ErrMFAChallengeInvalid = errors.New("multi-factor challenge is invalid or expired")
	// ErrMFACodeInvalid is a wrong, reused, or malformed second factor.
	ErrMFACodeInvalid = errors.New("multi-factor code is invalid")
	// ErrMFANotEnrolled is returned when activating or regenerating codes
	// without an enrollment to act on.
	ErrMFANotEnrolled = errors.New("multi-factor authentication is not enrolled")
	// ErrMFAAlreadyActive rejects a second enrollment over a live one: turning
	// MFA off is an explicit, password-confirmed act, never a side effect of
	// starting a new enrollment.
	ErrMFAAlreadyActive = errors.New("multi-factor authentication is already active")
)

const (
	// mfaChallengeLifetime bounds the half-authenticated window between a
	// verified password and a verified code. Long enough to open an
	// authenticator app, short enough that an abandoned challenge is worthless.
	mfaChallengeLifetime = 5 * time.Minute

	// mfaClockSkewSteps accepts one 30-second step either side of now, the
	// conventional tolerance for unsynchronised phone clocks.
	mfaClockSkewSteps = 1

	// mfaRecoveryCodeCount and mfaRecoveryCodeLength give ten codes of ~51
	// bits each — unguessable against the login throttle by orders of
	// magnitude, and still short enough to write down.
	mfaRecoveryCodeCount  = 10
	mfaRecoveryCodeLength = 10

	// mfaRecoveryAlphabet omits I, L, O, U, and 0/1 so a handwritten code is
	// unambiguous.
	mfaRecoveryAlphabet = "ABCDEFGHJKMNPQRSTVWXYZ23456789"

	// mfaIssuer is the label an authenticator app shows.
	mfaIssuer = "Rekenraam"

	authFailureMFAInvalid = "mfa_invalid"
	authFailureMFAExpired = "mfa_challenge_expired"
)

// MFAStatus is what the owner's security screen shows.
type MFAStatus struct {
	// Status is "disabled", "pending" (secret issued, never confirmed), or
	// "active" (a login needs it).
	Status                 string
	ActivatedAt            string
	RecoveryCodesRemaining int
	// Configured reports whether the server can store a secret at all
	// (REKENRAAM_SECRET_KEY). Without it, enrollment is refused rather than
	// storing the secret in the clear.
	Configured bool
}

// MFAEnrollment is shown once, at enrollment time.
type MFAEnrollment struct {
	Secret     string
	OTPAuthURI string
}

type MFAPasswordInput struct {
	UserID    int64
	Password  string
	ClientIP  string
	RequestID string
}

type ActivateMFAInput struct {
	UserID    int64
	Code      string
	ClientIP  string
	RequestID string
}

type CompleteMFALoginInput struct {
	ChallengeToken     string
	Code               string
	ClientIP           string
	RequestID          string
	TrustedDeviceToken string
}

// MFAStatusFor reports the owner's enrollment state.
func (s *AuthService) MFAStatusFor(ctx context.Context, userID int64) (MFAStatus, error) {
	status := MFAStatus{Status: "disabled", Configured: len(s.secretKey) > 0}
	record, err := s.repository.ReadMFATOTP(ctx, userID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return status, nil
		}
		return MFAStatus{}, fmt.Errorf("read mfa enrollment: %w", err)
	}

	status.Status = record.Status
	if record.ActivatedAt.Valid {
		status.ActivatedAt = record.ActivatedAt.String
	}
	remaining, err := s.repository.CountUnusedMFARecoveryCodes(ctx, userID)
	if err != nil {
		return MFAStatus{}, fmt.Errorf("count recovery codes: %w", err)
	}
	status.RecoveryCodesRemaining = remaining

	return status, nil
}

// BeginTOTPEnrollment issues a new secret. The password is re-checked even
// though the caller already holds a session: adding a second factor from a
// stolen session would otherwise let an attacker lock the real owner out of
// their own account.
func (s *AuthService) BeginTOTPEnrollment(ctx context.Context, input MFAPasswordInput) (MFAEnrollment, error) {
	if err := s.requireMFASecretKey(); err != nil {
		return MFAEnrollment{}, err
	}
	credentials, err := s.verifyOwnerPassword(ctx, input.UserID, input.Password, input.ClientIP, input.RequestID)
	if err != nil {
		return MFAEnrollment{}, err
	}

	if existing, err := s.repository.ReadMFATOTP(ctx, input.UserID); err == nil && existing.Status == "active" {
		return MFAEnrollment{}, ErrMFAAlreadyActive
	} else if err != nil && !errors.Is(err, db.ErrNotFound) {
		return MFAEnrollment{}, fmt.Errorf("read mfa enrollment: %w", err)
	}

	secret, err := totp.GenerateSecret()
	if err != nil {
		return MFAEnrollment{}, fmt.Errorf("generate mfa secret: %w", err)
	}
	ciphertext, err := s.sealMFASecret(secret)
	if err != nil {
		return MFAEnrollment{}, err
	}
	if err := s.repository.UpsertMFATOTP(ctx, db.UpsertMFATOTPParams{
		UserID:           input.UserID,
		SecretCiphertext: ciphertext,
		Status:           "pending",
		CreatedAt:        s.now().UTC().Format(time.RFC3339),
	}); err != nil {
		return MFAEnrollment{}, fmt.Errorf("persist mfa enrollment: %w", err)
	}

	return MFAEnrollment{
		Secret:     secret,
		OTPAuthURI: totp.URI(mfaIssuer, credentials.Username, secret),
	}, nil
}

// ActivateTOTP turns a pending enrollment on, but only once a code from it has
// verified — proof the authenticator really holds the secret. Returns the
// recovery codes, which are shown once and never again.
func (s *AuthService) ActivateTOTP(ctx context.Context, input ActivateMFAInput) ([]string, error) {
	if err := s.requireMFASecretKey(); err != nil {
		return nil, err
	}
	record, err := s.repository.ReadMFATOTP(ctx, input.UserID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrMFANotEnrolled
		}
		return nil, fmt.Errorf("read mfa enrollment: %w", err)
	}
	if record.Status == "active" {
		return nil, ErrMFAAlreadyActive
	}

	secret, err := s.openMFASecret(record.SecretCiphertext)
	if err != nil {
		return nil, err
	}
	step, ok := totp.Validate(secret, input.Code, s.now().UTC(), mfaClockSkewSteps)
	if !ok {
		return nil, ErrMFACodeInvalid
	}

	if err := s.repository.ActivateMFATOTP(ctx, input.UserID, s.now().UTC().Format(time.RFC3339), int64(step)); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrMFANotEnrolled
		}
		return nil, fmt.Errorf("activate mfa enrollment: %w", err)
	}

	return s.issueRecoveryCodes(ctx, input.UserID)
}

// DisableMFA removes the second factor. Password-confirmed, because switching
// protection off is exactly the act an attacker on a stolen session wants.
func (s *AuthService) DisableMFA(ctx context.Context, input MFAPasswordInput) error {
	if _, err := s.verifyOwnerPassword(ctx, input.UserID, input.Password, input.ClientIP, input.RequestID); err != nil {
		return err
	}
	if err := s.repository.DeleteMFATOTP(ctx, input.UserID); err != nil {
		return fmt.Errorf("disable mfa: %w", err)
	}

	return nil
}

// RegenerateRecoveryCodes replaces the whole set, invalidating any previous
// code — including ones already written down. Password-confirmed for the same
// reason as disabling.
func (s *AuthService) RegenerateRecoveryCodes(ctx context.Context, input MFAPasswordInput) ([]string, error) {
	if _, err := s.verifyOwnerPassword(ctx, input.UserID, input.Password, input.ClientIP, input.RequestID); err != nil {
		return nil, err
	}
	record, err := s.repository.ReadMFATOTP(ctx, input.UserID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrMFANotEnrolled
		}
		return nil, fmt.Errorf("read mfa enrollment: %w", err)
	}
	if record.Status != "active" {
		return nil, ErrMFANotEnrolled
	}

	return s.issueRecoveryCodes(ctx, input.UserID)
}

// CompleteLoginMFA finishes a login whose password already verified. It is the
// only place an MFA challenge turns into a session.
//
// The failed-attempt budget is the same one the password step spends, so
// guessing a six-digit code is throttled exactly like guessing a password —
// without which 5-in-15 on the password and unlimited guesses on the code
// would make the second factor decorative.
// A missing secret key deliberately does not block this path: TOTP will fail
// without it, but the recovery codes are hashed rather than sealed, and an
// owner whose key went missing must still be able to get in.
func (s *AuthService) CompleteLoginMFA(ctx context.Context, input CompleteMFALoginInput) (LoginResult, error) {
	challengeToken := strings.TrimSpace(input.ChallengeToken)
	if challengeToken == "" {
		return LoginResult{}, ErrMFAChallengeInvalid
	}
	code := strings.TrimSpace(input.Code)
	if code == "" {
		return LoginResult{}, ValidationError{Message: "authentication code is required"}
	}
	if len(code) > 64 {
		return LoginResult{}, ValidationError{Message: "authentication code is invalid"}
	}

	now := s.now().UTC()
	challenge, err := s.repository.ReadMFAChallenge(ctx, hashSessionToken(challengeToken), now.Format(time.RFC3339))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			s.recordAuthEvent(ctx, db.AuthenticationEventParams{
				EventType: authEventLoginFailed, Outcome: "failure",
				ClientIP: input.ClientIP, FailureReason: authFailureMFAExpired, RequestID: input.RequestID,
			})
			return LoginResult{}, ErrMFAChallengeInvalid
		}
		return LoginResult{}, fmt.Errorf("read mfa challenge: %w", err)
	}

	credentials, err := s.repository.ReadOwnerCredentialsByID(ctx, challenge.UserID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("read owner credentials: %w", err)
	}

	scopes, trustedDevice, deviceTrusted := s.loginThrottleScopesForAttempt(ctx, LoginInput{
		ClientIP:           input.ClientIP,
		TrustedDeviceToken: input.TrustedDeviceToken,
	}, credentials.Username)
	if blockedUntil, err := s.isLoginBlocked(ctx, scopes); err != nil {
		return LoginResult{}, fmt.Errorf("check login throttle: %w", err)
	} else if !blockedUntil.IsZero() {
		s.recordAuthEvent(ctx, db.AuthenticationEventParams{
			EventType: authEventLoginBlocked, Outcome: "failure", Username: credentials.Username,
			UserID: credentials.ID, ClientIP: input.ClientIP,
			FailureReason: authFailureRateLimited, RequestID: input.RequestID,
		})
		return LoginResult{}, RateLimitError{RetryAfter: time.Until(blockedUntil)}
	}

	accepted, err := s.verifySecondFactor(ctx, credentials.ID, code, now, input.ClientIP)
	if err != nil {
		return LoginResult{}, err
	}
	if !accepted {
		blockedUntil, throttleErr := s.recordLoginFailure(ctx, scopes)
		if throttleErr != nil {
			return LoginResult{}, fmt.Errorf("record login throttle failure: %w", throttleErr)
		}
		s.recordAuthEvent(ctx, db.AuthenticationEventParams{
			EventType: authEventLoginFailed, Outcome: "failure", Username: credentials.Username,
			UserID: credentials.ID, ClientIP: input.ClientIP,
			FailureReason: authFailureMFAInvalid, RequestID: input.RequestID,
		})
		if !blockedUntil.IsZero() {
			return LoginResult{}, RateLimitError{RetryAfter: time.Until(blockedUntil)}
		}
		return LoginResult{}, ErrMFACodeInvalid
	}

	// Spend the challenge only after the code passed, and treat losing the
	// race as an invalid challenge: two requests must never mint two sessions
	// from one password verification.
	consumed, err := s.repository.ConsumeMFAChallenge(ctx, challenge.ID, now.Format(time.RFC3339))
	if err != nil {
		return LoginResult{}, fmt.Errorf("consume mfa challenge: %w", err)
	}
	if !consumed {
		return LoginResult{}, ErrMFAChallengeInvalid
	}

	return s.completeLogin(ctx, completeLoginInput{
		UserID:        credentials.ID,
		Username:      credentials.Username,
		ClientIP:      input.ClientIP,
		RequestID:     input.RequestID,
		Scopes:        scopes,
		TrustedDevice: trustedDevice,
		DeviceTrusted: deviceTrusted,
	})
}

// verifySecondFactor accepts either a TOTP code or an unused recovery code.
// Both paths are tried for every attempt so the response time does not reveal
// which kind of code was submitted.
func (s *AuthService) verifySecondFactor(ctx context.Context, userID int64, code string, now time.Time, clientIP string) (bool, error) {
	record, err := s.repository.ReadMFATOTP(ctx, userID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return false, ErrMFANotEnrolled
		}
		return false, fmt.Errorf("read mfa enrollment: %w", err)
	}
	if record.Status != "active" {
		return false, ErrMFANotEnrolled
	}

	// A secret that cannot be opened — the key was lost, rotated, or never
	// configured on this host — must not block the recovery codes. Those are
	// exactly the situation recovery codes exist for, so this falls through
	// rather than failing the attempt.
	if secret, err := s.openMFASecret(record.SecretCiphertext); err != nil {
		if s.logger != nil {
			s.logger.WarnContext(ctx, "mfa secret could not be opened; only recovery codes can be used", slog.Any("err", err))
		}
	} else if step, ok := totp.Validate(secret, code, now, mfaClockSkewSteps); ok {
		// A code is valid for its whole step, so the counter must move past it
		// or the same code works again until it expires.
		fresh, err := s.repository.RecordMFATOTPUse(ctx, userID, int64(step))
		if err != nil {
			return false, fmt.Errorf("record mfa code use: %w", err)
		}
		return fresh, nil
	}

	err = s.repository.ConsumeMFARecoveryCode(ctx, userID, hashRecoveryCode(code), now.Format(time.RFC3339), clientIP)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, db.ErrNotFound), errors.Is(err, db.ErrMFARecoveryCodeUsed):
		return false, nil
	default:
		return false, fmt.Errorf("consume mfa recovery code: %w", err)
	}
}

// mfaChallengeFor issues the half-authenticated token handed back when a
// password verifies but a second factor is still owed.
func (s *AuthService) mfaChallengeFor(ctx context.Context, userID int64, clientIP string) (string, time.Duration, error) {
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return "", 0, fmt.Errorf("create mfa challenge token: %w", err)
	}
	now := s.now().UTC()
	if err := s.repository.CreateMFAChallenge(ctx, db.CreateMFAChallengeParams{
		UserID:    userID,
		TokenHash: tokenHash,
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(mfaChallengeLifetime).Format(time.RFC3339),
		ClientIP:  clientIP,
	}); err != nil {
		return "", 0, fmt.Errorf("persist mfa challenge: %w", err)
	}

	return token, mfaChallengeLifetime, nil
}

// mfaActive reports whether a login for this user must present a second
// factor. A pending enrollment is not active, so an abandoned enrollment can
// never lock the owner out.
func (s *AuthService) mfaActive(ctx context.Context, userID int64) (bool, error) {
	record, err := s.repository.ReadMFATOTP(ctx, userID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("read mfa enrollment: %w", err)
	}

	return record.Status == "active", nil
}

func (s *AuthService) issueRecoveryCodes(ctx context.Context, userID int64) ([]string, error) {
	codes := make([]string, 0, mfaRecoveryCodeCount)
	hashes := make([]string, 0, mfaRecoveryCodeCount)
	for index := 0; index < mfaRecoveryCodeCount; index++ {
		code, err := newRecoveryCode()
		if err != nil {
			return nil, err
		}
		codes = append(codes, code)
		hashes = append(hashes, hashRecoveryCode(code))
	}
	if err := s.repository.ReplaceMFARecoveryCodes(ctx, userID, hashes, s.now().UTC().Format(time.RFC3339)); err != nil {
		return nil, fmt.Errorf("persist mfa recovery codes: %w", err)
	}

	return codes, nil
}

// verifyOwnerPassword re-authenticates the session holder for the security
// operations that change what protects the account. A wrong password here is
// recorded like any other failed authentication, so an attacker probing from a
// stolen session is visible in the log.
func (s *AuthService) verifyOwnerPassword(ctx context.Context, userID int64, password string, clientIP string, requestID string) (db.OwnerCredentialsRecord, error) {
	if err := validateLoginPassword(password); err != nil {
		return db.OwnerCredentialsRecord{}, err
	}
	credentials, err := s.repository.ReadOwnerCredentialsByID(ctx, userID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return db.OwnerCredentialsRecord{}, ErrInvalidCredentials
		}
		return db.OwnerCredentialsRecord{}, fmt.Errorf("read owner credentials: %w", err)
	}
	verified, err := verifyPassword(password, credentials.PasswordHash)
	if err != nil {
		return db.OwnerCredentialsRecord{}, fmt.Errorf("verify password: %w", err)
	}
	if !verified {
		s.recordAuthEvent(ctx, db.AuthenticationEventParams{
			EventType: authEventLoginFailed, Outcome: "failure", Username: credentials.Username,
			UserID: credentials.ID, ClientIP: clientIP,
			FailureReason: authFailureInvalidCredentials, RequestID: requestID,
		})
		return db.OwnerCredentialsRecord{}, ErrInvalidCredentials
	}

	return credentials, nil
}

func (s *AuthService) requireMFASecretKey() error {
	if len(s.secretKey) == 0 {
		return ErrSecretKeyMissing
	}

	return nil
}

func (s *AuthService) sealMFASecret(secret string) (string, error) {
	box, err := secretbox.New(s.secretKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	ciphertext, err := box.Seal([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("seal mfa secret: %w", err)
	}

	return ciphertext, nil
}

func (s *AuthService) openMFASecret(ciphertext string) (string, error) {
	box, err := secretbox.New(s.secretKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	plaintext, err := box.Open(ciphertext)
	if err != nil {
		return "", fmt.Errorf("open mfa secret: %w", err)
	}

	return string(plaintext), nil
}

// pruneMFAChallenges rides the daily session-cleanup tick.
func (s *AuthService) pruneMFAChallenges(ctx context.Context, logger *slog.Logger) {
	deleted, err := s.repository.DeleteExpiredOrConsumedMFAChallenges(ctx, s.now().UTC().Format(time.RFC3339))
	if err != nil {
		logger.WarnContext(ctx, "prune mfa challenges", slog.Any("err", err))
		return
	}
	if deleted > 0 {
		logger.InfoContext(ctx, "pruned mfa challenges", slog.Int64("deleted", deleted))
	}
}

func newRecoveryCode() (string, error) {
	alphabetSize := big.NewInt(int64(len(mfaRecoveryAlphabet)))
	characters := make([]byte, 0, mfaRecoveryCodeLength+1)
	for index := 0; index < mfaRecoveryCodeLength; index++ {
		// Rejection-free selection via crypto/rand.Int keeps the distribution
		// uniform; modulo over a raw byte would not.
		pick, err := rand.Int(rand.Reader, alphabetSize)
		if err != nil {
			return "", fmt.Errorf("generate recovery code: %w", err)
		}
		if index == mfaRecoveryCodeLength/2 {
			characters = append(characters, '-')
		}
		characters = append(characters, mfaRecoveryAlphabet[pick.Int64()])
	}

	return string(characters), nil
}

// hashRecoveryCode normalizes before hashing so the dashes and case a user
// types back are irrelevant.
func hashRecoveryCode(code string) string {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.ReplaceAll(normalized, " ", "")
	sum := sha256.Sum256([]byte("mfa-recovery:" + normalized))

	return hex.EncodeToString(sum[:])
}
