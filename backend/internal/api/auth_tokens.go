package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"rekenraam/backend/internal/app"
)

type personalAccessTokenOwnerKey struct{}

// Each grant is an exact registered GET pattern. New endpoints have no grant
// until deliberately added here and in the public contract. Go's HEAD alias
// uses the same GET pattern. Never authorize by URL prefix.
var personalAccessTokenRouteScopes = map[string]string{
	"GET /api/v1/books/current":                          app.TokenScopeLedgerRead,
	"GET /api/v1/currencies":                             app.TokenScopeLedgerRead,
	"GET /api/v1/currencies/catalog":                     app.TokenScopeLedgerRead,
	"GET /api/v1/institutions":                           app.TokenScopeLedgerRead,
	"GET /api/v1/institutions/{institution_id}":          app.TokenScopeLedgerRead,
	"GET /api/v1/institutions/{institution_id}/versions": app.TokenScopeLedgerRead,
	"GET /api/v1/accounts":                               app.TokenScopeLedgerRead,
	"GET /api/v1/accounts/{account_id}":                  app.TokenScopeLedgerRead,
	"GET /api/v1/accounts/{account_id}/versions":         app.TokenScopeLedgerRead,
	"GET /api/v1/accounts/{account_id}/register":         app.TokenScopeLedgerRead,
	"GET /api/v1/categories":                             app.TokenScopeLedgerRead,
	"GET /api/v1/categories/{category_id}":               app.TokenScopeLedgerRead,
	"GET /api/v1/tags":                                   app.TokenScopeLedgerRead,
	"GET /api/v1/tags/{tag_id}":                          app.TokenScopeLedgerRead,
	"GET /api/v1/payees":                                 app.TokenScopeLedgerRead,
	"GET /api/v1/payees/{payee_id}":                      app.TokenScopeLedgerRead,
	"GET /api/v1/transactions":                           app.TokenScopeLedgerRead,
	"GET /api/v1/transactions/deleted":                   app.TokenScopeLedgerRead,
	"GET /api/v1/transactions/{transaction_id}":          app.TokenScopeLedgerRead,
	"GET /api/v1/ledger/account-balances":                app.TokenScopeReportsRead,
	"GET /api/v1/ledger/category-totals":                 app.TokenScopeReportsRead,
	"GET /api/v1/ledger/net-worth":                       app.TokenScopeReportsRead,
	"GET /api/v1/reports/net-worth":                      app.TokenScopeReportsRead,
	"GET /api/v1/reports/spending":                       app.TokenScopeReportsRead,
	"GET /api/v1/reports/cashflow":                       app.TokenScopeReportsRead,
}

func withPersonalAccessToken(logger *slog.Logger, auth *app.AuthService, options HandlerOptions, mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers, present := r.Header["Authorization"]
		if !present {
			mux.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if auth == nil {
			writeAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "invalid API credential")
			return
		}
		var token string
		if len(headers) == 1 {
			parts := strings.Fields(headers[0])
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				token = parts[1]
			}
		}
		_, pattern := mux.Handler(r)
		scope := personalAccessTokenRouteScopes[pattern]
		owner, err := auth.AuthenticatePersonalAccessToken(r.Context(), token, scope, loginClientIP(r, options), RequestIDFromContext(r.Context()))
		if err != nil {
			switch {
			case errors.Is(err, app.ErrPersonalAccessTokenInvalid):
				w.Header().Set("WWW-Authenticate", `Bearer realm="rekenraam"`)
				writeAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "invalid API credential")
			case errors.Is(err, app.ErrPersonalAccessTokenScope):
				writeAPIError(w, http.StatusForbidden, "FORBIDDEN", "API credential does not grant this operation")
			default:
				writeServiceInternalError(w, r, logger, "authenticate API credential", err)
			}
			return
		}
		ctx := context.WithValue(r.Context(), personalAccessTokenOwnerKey{}, owner)
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}

func listPersonalAccessTokens(logger *slog.Logger, auth *app.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, auth)
		if !ok {
			return
		}
		result, err := auth.ListPersonalAccessTokens(r.Context(), owner.ID, r.URL.Query().Get("cursor"))
		if err != nil {
			writeAuthServiceError(w, r, logger, "list API credentials", err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func createPersonalAccessToken(logger *slog.Logger, auth *app.AuthService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, auth, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request struct {
			Name          string          `json:"name"`
			Password      string          `json:"password"`
			Scopes        []string        `json:"scopes"`
			ExpiresInDays json.RawMessage `json:"expires_in_days"`
		}
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		var expiry *int
		if len(request.ExpiresInDays) > 0 {
			var days int
			if err := json.Unmarshal(request.ExpiresInDays, &days); err != nil || string(request.ExpiresInDays) == "null" {
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "token expiry must be an integer")
				return
			}
			expiry = &days
		}
		metadata, token, err := auth.CreatePersonalAccessToken(r.Context(), app.CreatePersonalAccessTokenInput{
			MFAPasswordInput: app.MFAPasswordInput{UserID: owner.ID, SessionID: authenticatedSessionID(r), Password: request.Password,
				ClientIP: loginClientIP(r, options), RequestID: RequestIDFromContext(r.Context())},
			Name: request.Name, Scopes: request.Scopes, ExpiresInDays: expiry,
		})
		if err != nil {
			writeAuthServiceError(w, r, logger, "create API credential", err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, struct {
			Token      string                  `json:"token"`
			Credential app.PersonalAccessToken `json:"credential"`
		}{token, metadata})
	}))
}

func revokePersonalAccessToken(logger *slog.Logger, auth *app.AuthService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, auth, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		id, ok := readPathInt64(w, r, "token_id", "token id")
		if !ok {
			return
		}
		err := auth.RevokePersonalAccessToken(r.Context(), app.MFAPasswordInput{UserID: owner.ID, SessionID: authenticatedSessionID(r),
			ClientIP: loginClientIP(r, options), RequestID: RequestIDFromContext(r.Context())}, id)
		if errors.Is(err, app.ErrPersonalAccessTokenNotFound) {
			writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "API credential not found")
			return
		}
		if err != nil {
			writeAuthServiceError(w, r, logger, "revoke API credential", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
