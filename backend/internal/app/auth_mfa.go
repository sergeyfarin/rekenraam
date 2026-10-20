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
	"strconv"
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
	// ErrMFAAuthenticatorUnavailable answers an authenticator code that the
	// server cannot check because the stored secret no longer opens (the
	// secret key was lost, rotated, or never configured on this host). Only
	// the password-verified challenge holder can see it, so naming the cause
	// reveals nothing, and it points them at their recovery codes (#290).
	ErrMFAAuthenticatorUnavailable = errors.New("authenticator codes cannot be checked; use a recovery code")
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
	// authFailureMFAUnavailable records an authenticator code submitted while
	// the secret cannot be opened. It is visible to the operator but never
	// charged to the sign-in budget.
	authFailureMFAUnavailable = "mfa_authenticator_unavailable"
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
	UserID int64
	// SessionID is the session making the change. It selects the
	// reauthentication budget (#262) and is required.
	SessionID int64
	Password  string
	ClientIP  string
	RequestID string
}

type ActivateMFAInput struct {
	UserID int64
	// SessionID selects the same reauthentication budget the password
	// confirmations spend.
	SessionID int64
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
	credentials, err := s.verifyOwnerPassword(ctx, input)
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
	event := s.securityEvent(authEventMFAEnrollmentStarted, input.UserID, input.SessionID, input.ClientIP, input.RequestID)
	if err := s.repository.UpsertMFATOTP(ctx, db.UpsertMFATOTPParams{
		UserID:           input.UserID,
		SecretCiphertext: ciphertext,
		Status:           "pending",
		CreatedAt:        event.OccurredAt,
		Event:            event,
	}); err != nil {
		if errors.Is(err, db.ErrMFAAlreadyActive) {
			return MFAEnrollment{}, ErrMFAAlreadyActive
		}
		return MFAEnrollment{}, fmt.Errorf("persist mfa enrollment: %w", err)
	}
	s.logAuthEvent(ctx, event)

	return MFAEnrollment{
		Secret:     secret,
		OTPAuthURI: totp.URI(mfaIssuer, credentials.Username, secret),
	}, nil
}

// ActivateTOTP turns a pending enrollment on, but only once a code from it has
// verified — proof the authenticator really holds the secret. Returns the
// recovery codes, which are shown once and never again.
//
// Wrong codes spend the session's reauthentication budget (#262): a pending
// enrollment is otherwise an unbounded six-digit verifier.
func (s *AuthService) ActivateTOTP(ctx context.Context, input ActivateMFAInput) ([]string, error) {
	if err := s.requireMFASecretKey(); err != nil {
		return nil, err
	}
	scope, err := reauthThrottleScope(input.SessionID)
	if err != nil {
		return nil, err
	}
	reservation, err := s.reserveReauth(ctx, scope, input.UserID, input.ClientIP, input.RequestID)
	if err != nil {
		return nil, err
	}
	defer reservation.release(ctx)
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
		return nil, s.failReauth(ctx, reservation, ErrMFACodeInvalid, db.AuthenticationEventParams{
			EventType: authEventLoginFailed, Outcome: "failure", UserID: input.UserID,
			ClientIP: input.ClientIP, FailureReason: authFailureMFAInvalid, RequestID: input.RequestID,
		})
	}
	if err := reservation.clear(ctx); err != nil {
		return nil, err
	}

	// The codes are minted before anything is written, and stored in the same
	// transaction that flips the enrollment on. Activating first and issuing
	// codes afterwards would, on any failure in between, leave MFA active with
	// no recovery codes and no way to retry: the retry would be refused as
	// already active.
	codes, hashes, err := newRecoveryCodeSet()
	if err != nil {
		return nil, err
	}
	event := s.securityEvent(authEventMFAActivated, input.UserID, input.SessionID, input.ClientIP, input.RequestID)
	if err := s.repository.ActivateMFATOTPWithRecoveryCodes(
		ctx, input.UserID, record.SecretCiphertext, event.OccurredAt, int64(step), hashes, event,
		func() error { return s.admitTOTPStep(step) },
	); err != nil {
		if errors.Is(err, ErrMFACodeInvalid) {
			return nil, s.failReauth(ctx, reservation, ErrMFACodeInvalid, db.AuthenticationEventParams{
				EventType: authEventLoginFailed, Outcome: "failure", UserID: input.UserID,
				ClientIP: input.ClientIP, FailureReason: authFailureMFAInvalid, RequestID: input.RequestID,
			})
		}
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrMFANotEnrolled
		}
		return nil, fmt.Errorf("activate mfa enrollment: %w", err)
	}
	s.logAuthEvent(ctx, event)

	return codes, nil
}

// DisableMFA removes the second factor. Password-confirmed, because switching
// protection off is exactly the act an attacker on a stolen session wants.
func (s *AuthService) DisableMFA(ctx context.Context, input MFAPasswordInput) error {
	if _, err := s.verifyOwnerPassword(ctx, input); err != nil {
		return err
	}
	// The event commits with the deletion (#288): turning protection off is
	// the change an attacker most wants to go unseen.
	event := s.securityEvent(authEventMFADisabled, input.UserID, input.SessionID, input.ClientIP, input.RequestID)
	disabled, err := s.repository.DeleteMFATOTP(ctx, input.UserID, event)
	if err != nil {
		return fmt.Errorf("disable mfa: %w", err)
	}
	if disabled {
		s.logAuthEvent(ctx, event)
	}

	return nil
}

// RegenerateRecoveryCodes replaces the whole set, invalidating any previous
// code — including ones already written down. Password-confirmed for the same
// reason as disabling.
func (s *AuthService) RegenerateRecoveryCodes(ctx context.Context, input MFAPasswordInput) ([]string, error) {
	if _, err := s.verifyOwnerPassword(ctx, input); err != nil {
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

	codes, hashes, err := newRecoveryCodeSet()
	if err != nil {
		return nil, err
	}
	event := s.securityEvent(authEventMFARecoveryCodesRegenerated, input.UserID, input.SessionID, input.ClientIP, input.RequestID)
	if err := s.repository.ReplaceMFARecoveryCodes(ctx, input.UserID, hashes, event.OccurredAt, event); err != nil {
		return nil, fmt.Errorf("persist mfa recovery codes: %w", err)
	}
	s.logAuthEvent(ctx, event)

	return codes, nil
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
	if !authDeadlineOpen(challenge.ExpiresAt, s.now()) {
		s.recordAuthEvent(ctx, db.AuthenticationEventParams{
			EventType: authEventLoginFailed, Outcome: "failure",
			ClientIP: input.ClientIP, FailureReason: authFailureMFAExpired, RequestID: input.RequestID,
		})
		return LoginResult{}, ErrMFAChallengeInvalid
	}

	credentials, err := s.repository.ReadOwnerCredentialsByID(ctx, challenge.UserID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("read owner credentials: %w", err)
	}

	scopes, trustedDevice, deviceTrusted := s.loginThrottleScopesForAttempt(ctx, LoginInput{
		ClientIP:           input.ClientIP,
		TrustedDeviceToken: input.TrustedDeviceToken,
	}, credentials.Username)
	reservation, err := s.reserveAttempt(ctx, scopes)
	if err != nil {
		if errors.Is(err, ErrRateLimited) {
			s.recordAuthEvent(ctx, db.AuthenticationEventParams{
				EventType: authEventLoginBlocked, Outcome: "failure", Username: credentials.Username,
				UserID: credentials.ID, ClientIP: input.ClientIP,
				FailureReason: authFailureRateLimited, RequestID: input.RequestID,
			})
		}
		return LoginResult{}, err
	}
	defer reservation.release(ctx)

	secondFactor, err := s.verifySecondFactor(ctx, credentials.ID, code, input.ClientIP)
	if errors.Is(err, ErrMFAAuthenticatorUnavailable) {
		// Not a guess: no authenticator code could have verified, so charging
		// it would only burn the budget the owner needs for a recovery code.
		// The deferred release hands the place back.
		s.recordAuthEvent(ctx, db.AuthenticationEventParams{
			EventType: authEventLoginFailed, Outcome: "failure", Username: credentials.Username,
			UserID: credentials.ID, ClientIP: input.ClientIP,
			FailureReason: authFailureMFAUnavailable, RequestID: input.RequestID,
		})
		return LoginResult{}, err
	}
	if err != nil {
		return LoginResult{}, err
	}
	if secondFactor == "" {
		retryAfter, throttleErr := reservation.fail(ctx)
		if throttleErr != nil {
			return LoginResult{}, throttleErr
		}
		s.recordAuthEvent(ctx, db.AuthenticationEventParams{
			EventType: authEventLoginFailed, Outcome: "failure", Username: credentials.Username,
			UserID: credentials.ID, ClientIP: input.ClientIP,
			FailureReason: authFailureMFAInvalid, RequestID: input.RequestID,
		})
		if retryAfter > 0 {
			return LoginResult{}, RateLimitError{RetryAfter: retryAfter}
		}
		return LoginResult{}, ErrMFACodeInvalid
	}

	// Spend the challenge only after the code passed, and treat losing the
	// race as an invalid challenge: two requests must never mint two sessions
	// from one password verification.
	consumed, err := s.repository.ConsumeMFAChallenge(ctx, challenge.ID, s.now)
	if err != nil {
		return LoginResult{}, fmt.Errorf("consume mfa challenge: %w", err)
	}
	if !consumed {
		return LoginResult{}, ErrMFAChallengeInvalid
	}

	return s.completeLogin(ctx, completeLoginInput{
		UserID:           credentials.ID,
		Username:         credentials.Username,
		ClientIP:         input.ClientIP,
		RequestID:        input.RequestID,
		Reservation:      reservation,
		TrustedDevice:    trustedDevice,
		DeviceTrusted:    deviceTrusted,
		SecondFactor:     secondFactor,
		MFAChallengeHash: hashSessionToken(challengeToken),
	})
}

// verifySecondFactor accepts either a TOTP code or an unused recovery code and
// names the one that verified, or returns "" when neither did. Both paths are
// tried for every attempt so the response time does not reveal which kind of
// code was submitted.
//
// When the secret cannot be opened and the code has the TOTP shape, it returns
// ErrMFAAuthenticatorUnavailable instead of "": a six-digit code can never be
// a recovery code, so no verifier could have accepted it, and the caller must
// not treat it as a wrong guess (#290). The recovery lookup still runs first,
// so the timing matches every other attempt.
func (s *AuthService) verifySecondFactor(ctx context.Context, userID int64, code string, clientIP string) (string, error) {
	record, err := s.repository.ReadMFATOTP(ctx, userID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return "", ErrMFANotEnrolled
		}
		return "", fmt.Errorf("read mfa enrollment: %w", err)
	}
	if record.Status != "active" {
		return "", ErrMFANotEnrolled
	}

	// A secret that cannot be opened — the key was lost, rotated, or never
	// configured on this host — must not block the recovery codes. Those are
	// exactly the situation recovery codes exist for, so this falls through
	// rather than failing the attempt.
	authenticatorUnavailable := false
	if secret, err := s.openMFASecret(record.SecretCiphertext); err != nil {
		authenticatorUnavailable = true
		if s.logger != nil {
			s.logger.WarnContext(ctx, "mfa secret could not be opened; only recovery codes can be used", slog.Any("err", err))
		}
	} else if step, ok := totp.Validate(secret, code, s.now().UTC(), mfaClockSkewSteps); ok {
		// A code is valid for its whole step, so the counter must move past it
		// or the same code works again until it expires.
		fresh, err := s.repository.RecordMFATOTPUse(ctx, userID, int64(step), record.SecretCiphertext, func() error { return s.admitTOTPStep(step) })
		if errors.Is(err, ErrMFACodeInvalid) {
			return "", nil
		}
		if err != nil {
			return "", fmt.Errorf("record mfa code use: %w", err)
		}
		if !fresh {
			return "", nil
		}
		return secondFactorTOTP, nil
	}

	err = s.repository.ConsumeMFARecoveryCode(ctx, userID, hashRecoveryCode(code), s.now().UTC().Format(time.RFC3339), clientIP)
	switch {
	case err == nil:
		return secondFactorRecoveryCode, nil
	case errors.Is(err, db.ErrNotFound), errors.Is(err, db.ErrMFARecoveryCodeUsed):
		if authenticatorUnavailable && totp.IsCodeShaped(code) {
			return "", ErrMFAAuthenticatorUnavailable
		}
		return "", nil
	default:
		return "", fmt.Errorf("consume mfa recovery code: %w", err)
	}
}

// admitTOTPStep runs while the repository owns SQLite write admission. The
// matched step must still be in the skew window when it is actually spent.
func (s *AuthService) admitTOTPStep(step uint64) error {
	current := totp.Step(s.now().UTC())
	if step > current+mfaClockSkewSteps || (current > mfaClockSkewSteps && step < current-mfaClockSkewSteps) {
		return ErrMFACodeInvalid
	}
	return nil
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

// newRecoveryCodeSet mints a full set and its hashes without touching the
// database, so callers can decide what transaction persists them.
func newRecoveryCodeSet() (codes []string, hashes []string, err error) {
	codes = make([]string, 0, mfaRecoveryCodeCount)
	hashes = make([]string, 0, mfaRecoveryCodeCount)
	for range mfaRecoveryCodeCount {
		code, err := newRecoveryCode()
		if err != nil {
			return nil, nil, err
		}
		codes = append(codes, code)
		hashes = append(hashes, hashRecoveryCode(code))
	}

	return codes, hashes, nil
}

// verifyOwnerPassword re-authenticates the session holder for the security
// operations that change what protects the account. A wrong password here is
// recorded like any other failed authentication, so an attacker probing from a
// stolen session is visible in the log.
//
// Every confirmation takes a place in the session's reauthentication budget
// before the hash is computed, and a wrong one keeps it (#262, #285).
// Enrollment, disable, regeneration, and activation share that one budget, so
// switching operation buys no extra guesses.
func (s *AuthService) verifyOwnerPassword(ctx context.Context, input MFAPasswordInput) (db.OwnerCredentialsRecord, error) {
	scope, err := reauthThrottleScope(input.SessionID)
	if err != nil {
		return db.OwnerCredentialsRecord{}, err
	}
	reservation, err := s.reserveReauth(ctx, scope, input.UserID, input.ClientIP, input.RequestID)
	if err != nil {
		return db.OwnerCredentialsRecord{}, err
	}
	defer reservation.release(ctx)
	if err := validateLoginPassword(input.Password); err != nil {
		return db.OwnerCredentialsRecord{}, err
	}
	credentials, err := s.repository.ReadOwnerCredentialsByID(ctx, input.UserID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return db.OwnerCredentialsRecord{}, ErrInvalidCredentials
		}
		return db.OwnerCredentialsRecord{}, fmt.Errorf("read owner credentials: %w", err)
	}
	verified, err := verifyPassword(input.Password, credentials.PasswordHash)
	if err != nil {
		return db.OwnerCredentialsRecord{}, fmt.Errorf("verify password: %w", err)
	}
	if !verified {
		return db.OwnerCredentialsRecord{}, s.failReauth(ctx, reservation, ErrInvalidCredentials, db.AuthenticationEventParams{
			EventType: authEventLoginFailed, Outcome: "failure", Username: credentials.Username,
			UserID: credentials.ID, ClientIP: input.ClientIP,
			FailureReason: authFailureInvalidCredentials, RequestID: input.RequestID,
		})
	}
	if err := reservation.clear(ctx); err != nil {
		return db.OwnerCredentialsRecord{}, err
	}

	return credentials, nil
}

// reauthThrottleScope keys the reauthentication budget to the session, not the
// owner or the address. An owner-wide budget would let a stolen session lock
// the real owner out of their own security settings; a session-scoped one
// confines the damage to the stolen session, whose holder cannot mint another
// without the password the budget protects. The owner recovers by signing in
// again, which issues a new session with a fresh budget. The budget is the
// login's own 5-in-15, so a stolen session guesses no faster than the public
// sign-in form already allows.
func reauthThrottleScope(sessionID int64) (loginThrottleScope, error) {
	if sessionID <= 0 {
		return loginThrottleScope{}, errors.New("reauthentication requires an authenticated session")
	}

	return loginThrottleScope{ScopeType: throttleScopeReauthSession, ScopeKey: strconv.FormatInt(sessionID, 10)}, nil
}

// reserveReauth admits one confirmation against the session's budget before
// any verifier runs, and refuses a blocked session even for correct input: a
// correct guess must not be distinguishable from a wrong one while blocked.
func (s *AuthService) reserveReauth(ctx context.Context, scope loginThrottleScope, userID int64, clientIP string, requestID string) (*throttleReservation, error) {
	reservation, err := s.reserveAttempt(ctx, []loginThrottleScope{scope})
	if err == nil {
		return reservation, nil
	}
	if !errors.Is(err, ErrRateLimited) {
		return nil, fmt.Errorf("check reauthentication throttle: %w", err)
	}
	s.recordAuthEvent(ctx, db.AuthenticationEventParams{
		EventType: authEventLoginBlocked, Outcome: "failure", UserID: userID,
		ClientIP: clientIP, FailureReason: authFailureRateLimited, RequestID: requestID,
	})

	return nil, err
}

// failReauth records a failed confirmation and returns the error the caller
// should surface: the rate limit if this failure exhausted the budget,
// otherwise the verifier's own rejection.
func (s *AuthService) failReauth(ctx context.Context, reservation *throttleReservation, rejection error, event db.AuthenticationEventParams) error {
	retryAfter, err := reservation.fail(ctx)
	if err != nil {
		return fmt.Errorf("record reauthentication throttle failure: %w", err)
	}
	s.recordAuthEvent(ctx, event)
	if retryAfter > 0 {
		return RateLimitError{RetryAfter: retryAfter}
	}

	return rejection
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
	for index := range mfaRecoveryCodeLength {
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
