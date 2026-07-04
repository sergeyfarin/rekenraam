package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/secretbox"
)

// sentinel errors exposed to the API layer
var (
	ErrImportConnectionNotFound  = errors.New("import connection not found")
	ErrSecretKeyMissing          = errors.New("REKENRAAM_SECRET_KEY is not configured; set it to use online import connections")
	ErrProviderUnauthorized      = errors.New("provider rejected the API key")
	ErrImportConnectionDuplicate = errors.New("a connection with this name already exists for this source")
)

// ConnectionProber validates a provider API key by making a lightweight
// authenticated call. The real Trading 212 implementation is
// Trading212Prober (import_trading212.go), backed by
// internal/onlinesource/trading212.
type ConnectionProber interface {
	// Probe returns nil if the key is valid, ErrProviderUnauthorized if the
	// provider rejects it, or another error for transient failures.
	// configJSON is the connection's non-secret config (e.g. base_url
	// override for sandbox/test endpoints); sourceKind selects which
	// provider's prober logic applies.
	Probe(ctx context.Context, sourceKind string, apiKey string, configJSON string) error
}

// NoOpProber skips validation. Used in tests and for source kinds with no
// real prober wired yet.
type NoOpProber struct{}

func (NoOpProber) Probe(_ context.Context, _ string, _ string, _ string) error { return nil }

// ImportConnectionService manages online source connections.
type ImportConnectionService struct {
	repository     *db.ImportConnectionRepository
	accountService *AccountService
	secretKey      []byte // nil = not configured
	prober         ConnectionProber
	now            func() time.Time
}

func NewImportConnectionService(
	repository *db.ImportConnectionRepository,
	accountService *AccountService,
	secretKey []byte,
	prober ConnectionProber,
) *ImportConnectionService {
	if prober == nil {
		prober = NoOpProber{}
	}
	return &ImportConnectionService{
		repository:     repository,
		accountService: accountService,
		secretKey:      secretKey,
		prober:         prober,
		now:            time.Now,
	}
}

// --- Types ---

// ImportConnection is the safe, public view of a connection — no secret.
type ImportConnection struct {
	ID          int64
	BookID      int64
	SourceKind  string
	DisplayName string
	KeyHint     string // "••••<last4>" — the only key info we surface
	ConfigJSON  string
	// TransactionsCursor/OrdersCursor/DividendsCursor are the incremental
	// cursors for each independently-paginated Trading 212 endpoint
	// (B-T212-INVST, Slice 4b) — internal fetch-worker state, not surfaced
	// on the API response.
	TransactionsCursor string
	OrdersCursor       string
	DividendsCursor    string
	LastFetchStatus    string
	LastFetchedAt      *string
	AutoRefreshEnabled bool
	// CashAccountID is the settlement account for BUY/SELL/DIVIDEND cash
	// legs (B-T212-INVST, docs/import-connection-accounts-plan.md). Nil
	// until the user sets one; rows resolve to needs_attention until then.
	CashAccountID *int64
	CreatedAt     string
	UpdatedAt     string
}

type CreateImportConnectionInput struct {
	OwnerUserID int64
	SourceKind  string
	DisplayName string
	APIKey      string
	ConfigJSON  string
	// CashAccountID is optional at create time (nil = set it later via PATCH).
	CashAccountID *int64
}

type UpdateImportConnectionInput struct {
	OwnerUserID int64
	ID          int64
	DisplayName string
	// NewAPIKey is optional. Empty = keep existing key.
	NewAPIKey  string
	ConfigJSON string
	// AutoRefreshEnabled is optional. Nil = keep the existing setting.
	AutoRefreshEnabled *bool
	// CashAccountID is optional. Nil = keep the existing setting; a non-nil
	// pointer sets/replaces it. There is currently no way to explicitly
	// clear it back to unset via PATCH (not needed by the accounts plan —
	// revisit if a real need shows up).
	CashAccountID *int64
}

// --- Service methods ---

func (s *ImportConnectionService) CreateImportConnection(ctx context.Context, input CreateImportConnectionInput) (ImportConnection, error) {
	if err := s.requireSecretKey(); err != nil {
		return ImportConnection{}, err
	}
	if err := validateConnectionInput(input.DisplayName, input.SourceKind, input.APIKey); err != nil {
		return ImportConnection{}, err
	}
	if err := validateConfigJSON(input.ConfigJSON); err != nil {
		return ImportConnection{}, err
	}
	if err := s.validateCashAccountID(ctx, input.CashAccountID); err != nil {
		return ImportConnection{}, err
	}
	displayName := strings.TrimSpace(input.DisplayName)

	// Probe before storing anything — if the key is bad we store nothing.
	if err := s.prober.Probe(ctx, input.SourceKind, input.APIKey, input.ConfigJSON); err != nil {
		if errors.Is(err, ErrProviderUnauthorized) {
			return ImportConnection{}, ErrProviderUnauthorized
		}
		return ImportConnection{}, fmt.Errorf("provider probe: %w", err)
	}

	box, err := secretbox.New(s.secretKey)
	if err != nil {
		return ImportConnection{}, fmt.Errorf("create cipher: %w", err)
	}
	ciphertext, err := box.Seal([]byte(input.APIKey))
	if err != nil {
		return ImportConnection{}, fmt.Errorf("seal api key: %w", err)
	}

	configJSON := input.ConfigJSON
	if configJSON == "" {
		configJSON = "{}"
	}

	now := s.now().UTC().Format(time.RFC3339)
	rec, err := s.repository.CreateImportConnection(ctx, db.CreateImportConnectionParams{
		BookID:           BookID,
		SourceKind:       input.SourceKind,
		DisplayName:      displayName,
		SecretCiphertext: ciphertext,
		ConfigJSON:       configJSON,
		CashAccountID:    nullableInt64(input.CashAccountID),
		CreatedAt:        now,
	})
	if err != nil {
		if isDuplicateError(err) {
			return ImportConnection{}, ErrImportConnectionDuplicate
		}
		return ImportConnection{}, fmt.Errorf("create import connection: %w", err)
	}

	return toImportConnection(rec, input.APIKey), nil
}

func (s *ImportConnectionService) ListImportConnections(ctx context.Context) ([]ImportConnection, error) {
	if err := s.requireSecretKey(); err != nil {
		return nil, err
	}
	recs, err := s.repository.ListImportConnections(ctx, BookID)
	if err != nil {
		return nil, fmt.Errorf("list import connections: %w", err)
	}
	out := make([]ImportConnection, 0, len(recs))
	for _, rec := range recs {
		plaintext, err := s.openSecret(rec.SecretCiphertext)
		if err != nil {
			// Corrupted record — surface as internal but continue listing others.
			return nil, fmt.Errorf("open secret for connection %d: %w", rec.ID, err)
		}
		out = append(out, toImportConnection(rec, string(plaintext)))
	}
	return out, nil
}

func (s *ImportConnectionService) GetImportConnection(ctx context.Context, id int64) (ImportConnection, error) {
	if err := s.requireSecretKey(); err != nil {
		return ImportConnection{}, err
	}
	rec, err := s.repository.ImportConnectionByID(ctx, id, BookID)
	if err != nil {
		if errors.Is(err, db.ErrImportConnectionNotFound) {
			return ImportConnection{}, ErrImportConnectionNotFound
		}
		return ImportConnection{}, fmt.Errorf("get import connection: %w", err)
	}
	plaintext, err := s.openSecret(rec.SecretCiphertext)
	if err != nil {
		return ImportConnection{}, fmt.Errorf("open secret for connection %d: %w", rec.ID, err)
	}
	return toImportConnection(rec, string(plaintext)), nil
}

func (s *ImportConnectionService) UpdateImportConnection(ctx context.Context, input UpdateImportConnectionInput) (ImportConnection, error) {
	if err := s.requireSecretKey(); err != nil {
		return ImportConnection{}, err
	}
	if strings.TrimSpace(input.DisplayName) == "" {
		return ImportConnection{}, ValidationError{Message: "display_name is required"}
	}
	if err := validateConfigJSON(input.ConfigJSON); err != nil {
		return ImportConnection{}, err
	}
	if err := s.validateCashAccountID(ctx, input.CashAccountID); err != nil {
		return ImportConnection{}, err
	}
	displayName := strings.TrimSpace(input.DisplayName)

	existing, err := s.repository.ImportConnectionByID(ctx, input.ID, BookID)
	if err != nil {
		if errors.Is(err, db.ErrImportConnectionNotFound) {
			return ImportConnection{}, ErrImportConnectionNotFound
		}
		return ImportConnection{}, fmt.Errorf("get import connection for update: %w", err)
	}

	newCiphertext := ""
	keyForHint := ""
	if input.NewAPIKey != "" {
		// Probe the new key before rotating.
		if err := s.prober.Probe(ctx, existing.SourceKind, input.NewAPIKey, existing.ConfigJSON); err != nil {
			if errors.Is(err, ErrProviderUnauthorized) {
				return ImportConnection{}, ErrProviderUnauthorized
			}
			return ImportConnection{}, fmt.Errorf("provider probe: %w", err)
		}
		box, err := secretbox.New(s.secretKey)
		if err != nil {
			return ImportConnection{}, fmt.Errorf("create cipher: %w", err)
		}
		newCiphertext, err = box.Seal([]byte(input.NewAPIKey))
		if err != nil {
			return ImportConnection{}, fmt.Errorf("seal new api key: %w", err)
		}
		keyForHint = input.NewAPIKey
	} else {
		// No rotation — derive the hint from the existing secret.
		plaintext, err := s.openSecret(existing.SecretCiphertext)
		if err != nil {
			return ImportConnection{}, fmt.Errorf("open existing secret: %w", err)
		}
		keyForHint = string(plaintext)
	}

	configJSON := input.ConfigJSON
	if configJSON == "" {
		configJSON = "{}"
	}

	autoRefreshEnabled := existing.AutoRefreshEnabled
	if input.AutoRefreshEnabled != nil {
		autoRefreshEnabled = *input.AutoRefreshEnabled
	}
	cashAccountID := existing.CashAccountID
	if input.CashAccountID != nil {
		cashAccountID = nullableInt64(input.CashAccountID)
	}

	now := s.now().UTC().Format(time.RFC3339)
	rec, err := s.repository.UpdateImportConnection(ctx, db.UpdateImportConnectionParams{
		ID:                 input.ID,
		BookID:             BookID,
		DisplayName:        displayName,
		SecretCiphertext:   newCiphertext,
		ConfigJSON:         configJSON,
		AutoRefreshEnabled: autoRefreshEnabled,
		CashAccountID:      cashAccountID,
		UpdatedAt:          now,
	})
	if err != nil {
		if errors.Is(err, db.ErrImportConnectionNotFound) {
			return ImportConnection{}, ErrImportConnectionNotFound
		}
		if isDuplicateError(err) {
			return ImportConnection{}, ErrImportConnectionDuplicate
		}
		return ImportConnection{}, fmt.Errorf("update import connection: %w", err)
	}

	return toImportConnection(rec, keyForHint), nil
}

func (s *ImportConnectionService) DeleteImportConnection(ctx context.Context, id int64) error {
	if err := s.requireSecretKey(); err != nil {
		return err
	}
	if err := s.repository.DeleteImportConnection(ctx, id, BookID); err != nil {
		if errors.Is(err, db.ErrImportConnectionNotFound) {
			return ErrImportConnectionNotFound
		}
		return fmt.Errorf("delete import connection: %w", err)
	}
	return nil
}

// OpenSecret decrypts the stored API key for use during a fetch.
// Only called by the fetch worker, not any HTTP handler.
func (s *ImportConnectionService) OpenSecret(ctx context.Context, id int64) (string, error) {
	if err := s.requireSecretKey(); err != nil {
		return "", err
	}
	rec, err := s.repository.ImportConnectionByID(ctx, id, BookID)
	if err != nil {
		if errors.Is(err, db.ErrImportConnectionNotFound) {
			return "", ErrImportConnectionNotFound
		}
		return "", fmt.Errorf("get import connection for open secret: %w", err)
	}
	plaintext, err := s.openSecret(rec.SecretCiphertext)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// UpdateFetchCursor saves the incremental fetch cursors (one per endpoint)
// and status after a fetch attempt. Called by the Trading 212 fetch worker,
// never by an HTTP handler.
func (s *ImportConnectionService) UpdateFetchCursor(ctx context.Context, id int64, transactionsCursor string, ordersCursor string, dividendsCursor string, status string, fetchedAt string) error {
	if err := s.requireSecretKey(); err != nil {
		return err
	}
	now := s.now().UTC().Format(time.RFC3339)
	if err := s.repository.UpdateImportConnectionCursor(ctx, db.UpdateImportConnectionCursorParams{
		ID:                 id,
		BookID:             BookID,
		TransactionsCursor: transactionsCursor,
		OrdersCursor:       ordersCursor,
		DividendsCursor:    dividendsCursor,
		LastFetchStatus:    status,
		LastFetchedAt:      fetchedAt,
		UpdatedAt:          now,
	}); err != nil {
		if errors.Is(err, db.ErrImportConnectionNotFound) {
			return ErrImportConnectionNotFound
		}
		return fmt.Errorf("update fetch cursor: %w", err)
	}
	return nil
}

// ListDueAutoRefreshConnectionIDs returns the ids of auto-refresh-enabled
// connections for sourceKind whose last fetch attempt was before cutoff (or
// that have never been fetched). Used by ImportService's scheduler
// (import_scheduler.go); returns bare ids rather than full ImportConnection
// values since the caller only needs them to call RefreshImportConnection.
func (s *ImportConnectionService) ListDueAutoRefreshConnectionIDs(ctx context.Context, sourceKind string, cutoff time.Time) ([]int64, error) {
	if err := s.requireSecretKey(); err != nil {
		return nil, err
	}
	recs, err := s.repository.ListDueAutoRefreshConnections(ctx, BookID, sourceKind, cutoff.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("list due auto-refresh connections: %w", err)
	}
	ids := make([]int64, len(recs))
	for i, rec := range recs {
		ids[i] = rec.ID
	}
	return ids, nil
}

// MarkFetchFailed flips a connection's last_fetch_status to "failed" after a
// terminal fetch error, preserving its existing cursor.
func (s *ImportConnectionService) MarkFetchFailed(ctx context.Context, id int64, fetchedAt string) error {
	if err := s.requireSecretKey(); err != nil {
		return err
	}
	existing, err := s.repository.ImportConnectionByID(ctx, id, BookID)
	if err != nil {
		if errors.Is(err, db.ErrImportConnectionNotFound) {
			return ErrImportConnectionNotFound
		}
		return fmt.Errorf("load import connection: %w", err)
	}
	return s.UpdateFetchCursor(ctx, id, existing.TransactionsCursor, existing.OrdersCursor, existing.DividendsCursor, "failed", fetchedAt)
}

// HoldingAccountForCommodity looks up the holding account already linked to
// (connectionID, commodityID) for B-T212-INVST commit-time routing. The bool
// is false if this commodity has never been seen for this connection before.
func (s *ImportConnectionService) HoldingAccountForCommodity(ctx context.Context, connectionID int64, commodityID int64) (int64, bool, error) {
	holdingAccountID, err := s.repository.HoldingAccountForCommodity(ctx, connectionID, commodityID)
	if errors.Is(err, db.ErrNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("find holding account for commodity: %w", err)
	}
	return holdingAccountID, true, nil
}

// LinkHolding records (connectionID, commodityID) -> holdingAccountID so
// future fetches reuse the same holding account. Called only from the
// commit path (never at fetch time — see docs/import-connection-accounts-plan.md).
func (s *ImportConnectionService) LinkHolding(ctx context.Context, connectionID int64, commodityID int64, holdingAccountID int64, createdAt string) error {
	return s.repository.LinkHolding(ctx, connectionID, commodityID, holdingAccountID, createdAt)
}

// --- Helpers ---

func (s *ImportConnectionService) requireSecretKey() error {
	if len(s.secretKey) == 0 {
		return ErrSecretKeyMissing
	}
	return nil
}

func (s *ImportConnectionService) openSecret(ciphertext string) ([]byte, error) {
	box, err := secretbox.New(s.secretKey)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	plaintext, err := box.Open(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("open ciphertext: %w", err)
	}
	return plaintext, nil
}

// validateCashAccountID checks that a proposed cash_account_id (if any)
// references a real, postable, non-archived account — per
// docs/import-connection-accounts-plan.md ("any postable asset kind — do not
// restrict to security_holding, since the cash leg is ordinary cash").
func (s *ImportConnectionService) validateCashAccountID(ctx context.Context, cashAccountID *int64) error {
	if cashAccountID == nil {
		return nil
	}
	if s.accountService == nil {
		return fmt.Errorf("import connections require an account service to validate cash_account_id")
	}
	account, err := s.accountService.Account(ctx, *cashAccountID)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			return ValidationError{Message: "cash_account_id does not reference an existing account"}
		}
		return fmt.Errorf("validate cash account: %w", err)
	}
	if !account.AllowsPostings {
		return ValidationError{Message: "cash_account_id must reference a postable account"}
	}
	if account.Status == "archived" {
		return ValidationError{Message: "cash_account_id must reference a non-archived account"}
	}
	return nil
}

func validateConnectionInput(displayName, sourceKind, apiKey string) error {
	if strings.TrimSpace(displayName) == "" {
		return ValidationError{Message: "display_name is required"}
	}
	if strings.TrimSpace(sourceKind) == "" {
		return ValidationError{Message: "source is required"}
	}
	if strings.TrimSpace(apiKey) == "" {
		return ValidationError{Message: "api_key is required"}
	}
	return nil
}

// validateConfigJSON rejects malformed JSON before it reaches the
// json_valid(config_json) CHECK constraint, so callers get a 400
// VALIDATION_FAILED instead of an internal SQLite constraint error.
func validateConfigJSON(configJSON string) error {
	if configJSON == "" {
		return nil
	}
	if !json.Valid([]byte(configJSON)) {
		return ValidationError{Message: "config must be valid JSON"}
	}
	return nil
}

// keyHint returns "••••<last4>" — safe to surface in API responses.
// We use unicode bullet (•) to signal masking visually.
func keyHint(apiKey string) string {
	runes := []rune(apiKey)
	if len(runes) <= 4 {
		return "••••"
	}
	last4 := string(runes[len(runes)-4:])
	if !utf8.ValidString(last4) {
		return "••••"
	}
	return "••••" + last4
}

func toImportConnection(rec db.ImportConnectionRecord, apiKey string) ImportConnection {
	var lastFetchedAt *string
	if rec.LastFetchedAt.Valid {
		lastFetchedAt = &rec.LastFetchedAt.String
	}
	return ImportConnection{
		ID:                 rec.ID,
		BookID:             rec.BookID,
		SourceKind:         rec.SourceKind,
		DisplayName:        rec.DisplayName,
		KeyHint:            keyHint(apiKey),
		ConfigJSON:         rec.ConfigJSON,
		TransactionsCursor: rec.TransactionsCursor,
		OrdersCursor:       rec.OrdersCursor,
		DividendsCursor:    rec.DividendsCursor,
		LastFetchStatus:    rec.LastFetchStatus,
		LastFetchedAt:      lastFetchedAt,
		AutoRefreshEnabled: rec.AutoRefreshEnabled,
		CashAccountID:      nullableSQLInt64Ptr(rec.CashAccountID),
		CreatedAt:          rec.CreatedAt,
		UpdatedAt:          rec.UpdatedAt,
	}
}

// isDuplicateError detects a SQLite UNIQUE constraint violation.
func isDuplicateError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
