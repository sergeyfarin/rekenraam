package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"rekenraam/backend/internal/db"
)

const (
	TokenScopeLedgerRead  = "ledger:read"
	TokenScopeReportsRead = "reports:read"
)

var (
	ErrPersonalAccessTokenInvalid  = errors.New("invalid personal access token")
	ErrPersonalAccessTokenScope    = errors.New("personal access token scope denied")
	ErrPersonalAccessTokenNotFound = errors.New("personal access token not found")
)

type PersonalAccessToken struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Scopes     []string `json:"scopes"`
	CreatedAt  string   `json:"created_at"`
	ExpiresAt  string   `json:"expires_at"`
	RevokedAt  *string  `json:"revoked_at"`
	LastUsedAt *string  `json:"last_used_at"`
}

type PersonalAccessTokens struct {
	Tokens     []PersonalAccessToken `json:"tokens"`
	NextCursor *string               `json:"next_cursor"`
}

type CreatePersonalAccessTokenInput struct {
	MFAPasswordInput
	Name          string
	Scopes        []string
	ExpiresInDays *int
}

func tokenScopes(raw string) ([]string, error) {
	var scopes []string
	if err := json.Unmarshal([]byte(raw), &scopes); err != nil {
		return nil, fmt.Errorf("read personal access token scopes: %w", err)
	}
	return scopes, nil
}

func (s *AuthService) CreatePersonalAccessToken(ctx context.Context, input CreatePersonalAccessTokenInput) (PersonalAccessToken, string, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > 80 || strings.ContainsFunc(name, unicode.IsControl) {
		return PersonalAccessToken{}, "", ValidationError{Message: "token name is invalid"}
	}
	if len(input.Scopes) < 1 || len(input.Scopes) > 2 {
		return PersonalAccessToken{}, "", ValidationError{Message: "token scopes are invalid"}
	}
	seen := map[string]bool{}
	for _, scope := range input.Scopes {
		if (scope != TokenScopeLedgerRead && scope != TokenScopeReportsRead) || seen[scope] {
			return PersonalAccessToken{}, "", ValidationError{Message: "token scopes are invalid"}
		}
		seen[scope] = true
	}
	days := 30
	if input.ExpiresInDays != nil {
		days = *input.ExpiresInDays
	}
	if days < 1 || days > 365 {
		return PersonalAccessToken{}, "", ValidationError{Message: "token expiry must be between 1 and 365 days"}
	}
	credentials, err := s.verifyOwnerPassword(ctx, input.MFAPasswordInput)
	if err != nil {
		return PersonalAccessToken{}, "", err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return PersonalAccessToken{}, "", fmt.Errorf("generate personal access token: %w", err)
	}
	plaintext := "rr_pat_" + base64.RawURLEncoding.EncodeToString(secret)
	now := s.now().UTC()
	created := now.Format(time.RFC3339)
	expires := now.Add(time.Duration(days) * 24 * time.Hour).Format(time.RFC3339)
	scopes, err := json.Marshal(input.Scopes)
	if err != nil {
		return PersonalAccessToken{}, "", err
	}
	event := s.securityEvent("token_created", input.UserID, input.SessionID, input.ClientIP, input.RequestID)
	id, err := s.repository.CreatePersonalAccessToken(ctx, db.CreatePersonalAccessTokenParams{
		UserID: input.UserID, SessionID: input.SessionID, PasswordHash: credentials.PasswordHash,
		Name: name, ScopesJSON: string(scopes), TokenHash: hashSessionToken("pat:" + plaintext), CreatedAt: created, ExpiresAt: expires,
	}, event, s.now)
	if errors.Is(err, db.ErrNotFound) {
		return PersonalAccessToken{}, "", ErrInvalidCredentials
	}
	if errors.Is(err, db.ErrTokenLimit) {
		return PersonalAccessToken{}, "", ValidationError{Message: "revoke an active token before creating another"}
	}
	if err != nil {
		return PersonalAccessToken{}, "", err
	}
	return PersonalAccessToken{ID: id, Name: name, Scopes: slices.Clone(input.Scopes), CreatedAt: created, ExpiresAt: expires}, plaintext, nil
}

func (s *AuthService) ListPersonalAccessTokens(ctx context.Context, userID int64, cursor string) (PersonalAccessTokens, error) {
	var before int64
	if cursor != "" {
		v, err := strconv.ParseInt(cursor, 10, 64)
		if err != nil || v <= 0 {
			return PersonalAccessTokens{}, ValidationError{Message: "token cursor is invalid"}
		}
		before = v
	}
	rows, err := s.repository.ListPersonalAccessTokens(ctx, userID, before)
	if err != nil {
		return PersonalAccessTokens{}, err
	}
	result := PersonalAccessTokens{Tokens: make([]PersonalAccessToken, 0)}
	if len(rows) > 50 {
		cursor := strconv.FormatInt(rows[49].ID, 10)
		result.NextCursor = &cursor
		rows = rows[:50]
	}
	for _, row := range rows {
		scopes, err := tokenScopes(row.ScopesJSON)
		if err != nil {
			return PersonalAccessTokens{}, err
		}
		v := PersonalAccessToken{ID: row.ID, Name: row.Name, Scopes: scopes, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt}
		if row.RevokedAt.Valid {
			v.RevokedAt = &row.RevokedAt.String
		}
		if row.LastUsedAt.Valid {
			v.LastUsedAt = &row.LastUsedAt.String
		}
		result.Tokens = append(result.Tokens, v)
	}
	return result, nil
}

func (s *AuthService) RevokePersonalAccessToken(ctx context.Context, input MFAPasswordInput, id int64) error {
	event := s.securityEvent("token_revoked", input.UserID, input.SessionID, input.ClientIP, input.RequestID)
	event.MetadataJSON = fmt.Sprintf(`{"token_id":%d}`, id)
	err := s.repository.RevokePersonalAccessToken(ctx, input.UserID, input.SessionID, id, s.now, event)
	if errors.Is(err, db.ErrNotFound) {
		return ErrPersonalAccessTokenNotFound
	}
	return err
}

// AuthenticatePersonalAccessToken never falls back to the session credential.
// requiredScope is empty for ungranted routes, which always refuse access.
func (s *AuthService) AuthenticatePersonalAccessToken(ctx context.Context, token, requiredScope, clientIP, requestID string) (Owner, error) {
	now := s.now().UTC().Format(time.RFC3339)
	invalid := func() (Owner, error) {
		err := s.repository.RecordAuthenticationEvent(ctx, db.AuthenticationEventParams{
			OccurredAt: s.now().UTC().Format(time.RFC3339), EventType: "token_denied", Outcome: "failure", Origin: "personal_access_token",
			ClientIP: clientIP, RequestID: requestID, FailureReason: "invalid_token",
		})
		if err != nil {
			return Owner{}, err
		}
		return Owner{}, ErrPersonalAccessTokenInvalid
	}
	if !strings.HasPrefix(token, "rr_pat_") || len(token) != 50 {
		return invalid()
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(token, "rr_pat_"))
	if err != nil || len(raw) != 32 {
		return invalid()
	}
	row, err := s.repository.ReadPersonalAccessToken(ctx, hashSessionToken("pat:"+token), now)
	if errors.Is(err, db.ErrNotFound) {
		return invalid()
	}
	if err != nil {
		return Owner{}, err
	}
	scopes, err := tokenScopes(row.ScopesJSON)
	if err != nil {
		return Owner{}, err
	}
	granted := (requiredScope == TokenScopeLedgerRead || requiredScope == TokenScopeReportsRead) && slices.Contains(scopes, requiredScope)
	event := db.AuthenticationEventParams{
		OccurredAt: now, UserID: row.UserID, EventType: "token_authenticated", Outcome: "success",
		Origin: "personal_access_token", ClientIP: clientIP, RequestID: requestID,
	}
	metadata, err := json.Marshal(struct {
		TokenID       int64  `json:"token_id"`
		RequiredScope string `json:"required_scope,omitempty"`
	}{row.ID, requiredScope})
	if err != nil {
		return Owner{}, err
	}
	event.MetadataJSON = string(metadata)
	if !granted {
		event.EventType, event.Outcome, event.FailureReason = "token_denied", "failure", "insufficient_scope"
	}
	if err := s.repository.RecordPersonalAccessTokenUse(ctx, row.ID, s.now, event); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return invalid()
		}
		return Owner{}, err
	}
	if !granted {
		return Owner{}, ErrPersonalAccessTokenScope
	}
	return Owner{ID: row.UserID, Username: row.Username}, nil
}
