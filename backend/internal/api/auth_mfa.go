package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"rekenraam/backend/internal/app"
)

// The MFA challenge travels as its own HttpOnly cookie rather than in the JSON
// body. It is short-lived and grants nothing on its own, but keeping it out of
// reach of page scripts costs nothing and means the browser step needs no
// token handling at all.
const (
	mfaChallengeCookieName       = "rekenraam_mfa"
	secureMFAChallengeCookieName = "__Host-rekenraam_mfa"
)

type mfaStatusResponse struct {
	Status                 string `json:"status"`
	ActivatedAt            string `json:"activated_at,omitempty"`
	RecoveryCodesRemaining int    `json:"recovery_codes_remaining"`
	Configured             bool   `json:"configured"`
}

type mfaPasswordRequest struct {
	Password string `json:"password"`
}

type mfaEnrollResponse struct {
	Secret     string `json:"secret"`
	OTPAuthURI string `json:"otpauth_uri"`
}

type mfaCodeRequest struct {
	Code string `json:"code"`
}

type mfaRecoveryCodesResponse struct {
	RecoveryCodes []string `json:"recovery_codes"`
}

func mfaStatus(logger *slog.Logger, authService *app.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		status, err := authService.MFAStatusFor(r.Context(), owner.ID)
		if err != nil {
			writeAuthServiceError(w, r, logger, "read mfa status", err)
			return
		}
		writeJSON(w, http.StatusOK, mfaStatusResponse{
			Status:                 status.Status,
			ActivatedAt:            status.ActivatedAt,
			RecoveryCodesRemaining: status.RecoveryCodesRemaining,
			Configured:             status.Configured,
		})
	}
}

func enrollMFATOTP(logger *slog.Logger, authService *app.AuthService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request mfaPasswordRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		enrollment, err := authService.BeginTOTPEnrollment(r.Context(), app.MFAPasswordInput{
			UserID:    owner.ID,
			Password:  request.Password,
			ClientIP:  loginClientIP(r, options),
			RequestID: RequestIDFromContext(r.Context()),
		})
		if err != nil {
			writeAuthServiceError(w, r, logger, "begin mfa enrollment", err)
			return
		}
		writeJSON(w, http.StatusCreated, mfaEnrollResponse{
			Secret:     enrollment.Secret,
			OTPAuthURI: enrollment.OTPAuthURI,
		})
	}))
}

func activateMFATOTP(logger *slog.Logger, authService *app.AuthService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request mfaCodeRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		codes, err := authService.ActivateTOTP(r.Context(), app.ActivateMFAInput{
			UserID:    owner.ID,
			Code:      request.Code,
			ClientIP:  loginClientIP(r, options),
			RequestID: RequestIDFromContext(r.Context()),
		})
		if err != nil {
			writeAuthServiceError(w, r, logger, "activate mfa", err)
			return
		}
		writeJSON(w, http.StatusOK, mfaRecoveryCodesResponse{RecoveryCodes: codes})
	}))
}

func disableMFA(logger *slog.Logger, authService *app.AuthService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request mfaPasswordRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		if err := authService.DisableMFA(r.Context(), app.MFAPasswordInput{
			UserID:    owner.ID,
			Password:  request.Password,
			ClientIP:  loginClientIP(r, options),
			RequestID: RequestIDFromContext(r.Context()),
		}); err != nil {
			writeAuthServiceError(w, r, logger, "disable mfa", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

func regenerateMFARecoveryCodes(logger *slog.Logger, authService *app.AuthService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request mfaPasswordRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		codes, err := authService.RegenerateRecoveryCodes(r.Context(), app.MFAPasswordInput{
			UserID:    owner.ID,
			Password:  request.Password,
			ClientIP:  loginClientIP(r, options),
			RequestID: RequestIDFromContext(r.Context()),
		})
		if err != nil {
			writeAuthServiceError(w, r, logger, "regenerate mfa recovery codes", err)
			return
		}
		writeJSON(w, http.StatusOK, mfaRecoveryCodesResponse{RecoveryCodes: codes})
	}))
}

// completeLoginMFA is the second half of a login. It is deliberately not
// behind requireAuthenticatedMutation — the caller has no session yet, which
// is the entire point — but it is same-origin checked like the login it
// continues.
func completeLoginMFA(logger *slog.Logger, authService *app.AuthService, options HandlerOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request mfaCodeRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, err := authService.CompleteLoginMFA(r.Context(), app.CompleteMFALoginInput{
			ChallengeToken:     readMFAChallengeToken(r),
			Code:               request.Code,
			ClientIP:           loginClientIP(r, options),
			RequestID:          RequestIDFromContext(r.Context()),
			TrustedDeviceToken: readTrustedDeviceToken(r),
		})
		if err != nil {
			// A dead challenge is the one failure the browser cannot retry, so
			// clear the cookie with the error and send it back to the start.
			if errors.Is(err, app.ErrMFAChallengeInvalid) {
				clearMFAChallengeCookie(w, r, options)
			}
			writeAuthServiceError(w, r, logger, "complete mfa login", err)
			return
		}

		clearMFAChallengeCookie(w, r, options)
		writeSessionCookie(w, r, options, result.SessionToken)
		if result.TrustedDeviceToken != "" {
			writeTrustedDeviceCookie(w, r, options, result.TrustedDeviceToken, result.TrustedDeviceLifetime)
		}
		writeJSON(w, http.StatusOK, loginResponse{
			User: &OwnerResponse{ID: result.User.ID, Username: result.User.Username},
		})
	}
}

func readMFAChallengeToken(r *http.Request) string {
	for _, name := range []string{secureMFAChallengeCookieName, mfaChallengeCookieName} {
		if cookie, err := r.Cookie(name); err == nil {
			return strings.TrimSpace(cookie.Value)
		}
	}

	return ""
}

func writeMFAChallengeCookie(w http.ResponseWriter, r *http.Request, options HandlerOptions, token string, lifetime time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     requestMFAChallengeCookieName(r, options),
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   true,
		MaxAge:   int(lifetime.Seconds()),
	})
}

func clearMFAChallengeCookie(w http.ResponseWriter, r *http.Request, options HandlerOptions) {
	secure := requestUsesHTTPS(r, options)
	clearCookie(w, requestMFAChallengeCookieName(r, options), secure)
	if requestMFAChallengeCookieName(r, options) != mfaChallengeCookieName {
		clearCookie(w, mfaChallengeCookieName, secure)
	}
}

func requestMFAChallengeCookieName(r *http.Request, options HandlerOptions) string {
	if requestUsesHTTPS(r, options) {
		return secureMFAChallengeCookieName
	}

	return mfaChallengeCookieName
}
