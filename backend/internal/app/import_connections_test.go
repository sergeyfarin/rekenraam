package app

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
)

// --- Helpers ---

func testKey() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}

func openConnectionTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(context.Background(), "file:"+filepath.Join(t.TempDir(), "test.sqlite"))
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })
	require.NoError(t, db.Migrate(context.Background(), database))

	// Seed the minimal row needed to satisfy import_connections.book_id FK.
	_, err = database.ExecContext(context.Background(), `
		INSERT INTO users (id, username, password_hash, is_owner, created_at, updated_at)
		VALUES (1, 'owner', 'hash', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
		INSERT INTO books (id, owner_user_id, code, name, updated_by_user_id, created_at, updated_at)
		VALUES (1, 1, 'personal', 'Personal', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
	`)
	require.NoError(t, err)
	return database
}

func openTestConnectionRepo(t *testing.T) *db.ImportConnectionRepository {
	t.Helper()
	return db.NewImportConnectionRepository(openConnectionTestDatabase(t))
}

func newTestConnectionService(t *testing.T, key []byte, prober ConnectionProber) *ImportConnectionService {
	t.Helper()
	repo := openTestConnectionRepo(t)
	return NewImportConnectionService(repo, nil, key, prober)
}

// newTestConnectionServiceWithAccounts wires a real AccountService, used by
// the cash_account_id validation tests. Returns the underlying *sql.DB too
// so tests can seed account fixtures directly (account_kinds is seeded by
// the initial migration, so a minimal account_versions row is enough —
// going through the full setup-wizard bootstrap isn't needed just to
// validate an FK reference).
func newTestConnectionServiceWithAccounts(t *testing.T) (*ImportConnectionService, *sql.DB) {
	t.Helper()
	database := openConnectionTestDatabase(t)
	accountService := NewAccountService(db.NewAccountRepository(database), db.NewInstitutionRepository(database), nil)
	svc := NewImportConnectionService(db.NewImportConnectionRepository(database), accountService, testKey(), NoOpProber{})
	return svc, database
}

// seedTestAccount inserts a minimal, real account_versions row directly
// (bypassing AccountService/SetupService bootstrap, which normally seeds
// account_kinds/system accounts at first run — not needed here since
// account_kinds is already seeded by the initial schema migration).
func seedTestAccount(t *testing.T, database *sql.DB, status string, allowsPostings bool) int64 {
	t.Helper()
	return seedTestAccountWithClass(t, database, status, allowsPostings, "asset", "checking")
}

func seedTestAccountWithClass(t *testing.T, database *sql.DB, status string, allowsPostings bool, accountClass string, accountKind string) int64 {
	t.Helper()
	ctx := context.Background()
	result, err := database.ExecContext(ctx, `
		INSERT INTO accounts (book_id, created_at, created_by_user_id)
		VALUES (?, '2026-01-01T00:00:00Z', 1)
	`, BookID)
	require.NoError(t, err)
	accountID, err := result.LastInsertId()
	require.NoError(t, err)

	closedOn := sql.NullString{}
	if status != "active" {
		closedOn = sql.NullString{String: "2026-01-02", Valid: true}
	}
	_, err = database.ExecContext(ctx, `
		INSERT INTO account_versions (
			account_id, version_seq, effective_from, recorded_at, changed_by_user_id,
			change_reason, status, opened_on, closed_on, code, name, account_class,
			account_kind, allows_postings
		) VALUES (?, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'test fixture', ?, '2026-01-01', ?, 'CASH', 'Test Cash', ?, ?, ?)
	`, accountID, status, closedOn, accountClass, accountKind, boolToIntForTest(allowsPostings))
	require.NoError(t, err)
	return accountID
}

func boolToIntForTest(v bool) int {
	if v {
		return 1
	}
	return 0
}

// errProber always rejects.
type errProber struct{ err error }

func (p errProber) Probe(_ context.Context, _ string, _ string, _ string) error { return p.err }

// recordingProber records the configJSON it was last called with, so a test
// can assert which config a rotation was actually validated against.
type recordingProber struct{ lastConfigJSON *string }

func (p *recordingProber) Probe(_ context.Context, _ string, _ string, configJSON string) error {
	*p.lastConfigJSON = configJSON
	return nil
}

// --- Tests ---

func TestCreateImportConnection_MissingKeyReturnsConfigRequired(t *testing.T) {
	svc := newTestConnectionService(t, nil, NoOpProber{})
	_, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1,
		SourceKind:  "trading212",
		DisplayName: "My T212",
		APIKey:      "secret-key",
	})
	require.ErrorIs(t, err, ErrSecretKeyMissing)
}

func TestCreateImportConnection_RoundTrip(t *testing.T) {
	svc := newTestConnectionService(t, testKey(), NoOpProber{})
	conn, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1,
		SourceKind:  "trading212",
		DisplayName: "ISA Account",
		APIKey:      "api-key-abcd",
	})
	require.NoError(t, err)
	assert.Greater(t, conn.ID, int64(0))
	assert.Equal(t, "trading212", conn.SourceKind)
	assert.Equal(t, "ISA Account", conn.DisplayName)
	// Key must be masked — plaintext must not appear.
	assert.NotContains(t, conn.KeyHint, "api-key-abcd")
	assert.Contains(t, conn.KeyHint, "abcd") // last-4 visible
	assert.Contains(t, conn.KeyHint, "••••") // prefix masked
}

func TestCreateImportConnection_SecretsNeverInPlaintext(t *testing.T) {
	repo := openTestConnectionRepo(t)
	svc := NewImportConnectionService(repo, nil, testKey(), NoOpProber{})

	_, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1,
		SourceKind:  "trading212",
		DisplayName: "Leak Test",
		APIKey:      "super-secret-12345",
	})
	require.NoError(t, err)

	// Directly query the DB to confirm no plaintext in any column.
	conns, err := repo.ListImportConnections(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, conns, 1)
	assert.NotContains(t, conns[0].SecretCiphertext, "super-secret-12345")
	assert.NotContains(t, conns[0].DisplayName, "super-secret-12345")
	assert.NotContains(t, conns[0].ConfigJSON, "super-secret-12345")
}

func TestCreateImportConnection_ProbeFailureStoresNothing(t *testing.T) {
	repo := openTestConnectionRepo(t)
	svc := NewImportConnectionService(repo, nil, testKey(), errProber{err: ErrProviderUnauthorized})

	_, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1,
		SourceKind:  "trading212",
		DisplayName: "Bad Key",
		APIKey:      "invalid-key",
	})
	require.ErrorIs(t, err, ErrProviderUnauthorized)

	// Nothing must have been persisted.
	conns, err := repo.ListImportConnections(context.Background(), 1)
	require.NoError(t, err)
	assert.Empty(t, conns)
}

func TestCreateImportConnection_DuplicateNameRejected(t *testing.T) {
	svc := newTestConnectionService(t, testKey(), NoOpProber{})

	_, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "Shared Name", APIKey: "key1",
	})
	require.NoError(t, err)

	_, err = svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "Shared Name", APIKey: "key2",
	})
	require.ErrorIs(t, err, ErrImportConnectionDuplicate)
}

func TestListImportConnections_ReturnsAllForBook(t *testing.T) {
	svc := newTestConnectionService(t, testKey(), NoOpProber{})

	for _, name := range []string{"A", "B", "C"} {
		_, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
			OwnerUserID: 1, SourceKind: "trading212", DisplayName: name, APIKey: "key-" + name,
		})
		require.NoError(t, err)
	}

	conns, err := svc.ListImportConnections(context.Background())
	require.NoError(t, err)
	require.Len(t, conns, 3)
	names := []string{conns[0].DisplayName, conns[1].DisplayName, conns[2].DisplayName}
	assert.Equal(t, []string{"A", "B", "C"}, names)
}

func TestListImportConnections_KeyHintNeverExposesFullKey(t *testing.T) {
	svc := newTestConnectionService(t, testKey(), NoOpProber{})

	_, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "Hint Check", APIKey: "full-api-key-xyz",
	})
	require.NoError(t, err)

	conns, err := svc.ListImportConnections(context.Background())
	require.NoError(t, err)
	require.Len(t, conns, 1)
	assert.NotContains(t, conns[0].KeyHint, "full-api-key-xyz")
	assert.Contains(t, conns[0].KeyHint, "-xyz") // last 4 chars of the key
}

func TestUpdateImportConnection_RenameOnly(t *testing.T) {
	svc := newTestConnectionService(t, testKey(), NoOpProber{})

	conn, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "Original", APIKey: "api-key-xxxx",
	})
	require.NoError(t, err)

	updated, err := svc.UpdateImportConnection(context.Background(), UpdateImportConnectionInput{
		OwnerUserID: 1,
		ID:          conn.ID,
		DisplayName: "Renamed",
		ConfigJSON:  "{}",
	})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", updated.DisplayName)
	// Key hint should be unchanged.
	assert.Equal(t, conn.KeyHint, updated.KeyHint)
}

func TestUpdateImportConnection_OmittedAutoRefreshPreservesExisting(t *testing.T) {
	svc := newTestConnectionService(t, testKey(), NoOpProber{})

	conn, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "Original", APIKey: "api-key-xxxx",
	})
	require.NoError(t, err)

	enabled, err := svc.UpdateImportConnection(context.Background(), UpdateImportConnectionInput{
		OwnerUserID:        1,
		ID:                 conn.ID,
		DisplayName:        conn.DisplayName,
		ConfigJSON:         "{}",
		AutoRefreshEnabled: new(true),
	})
	require.NoError(t, err)
	require.True(t, enabled.AutoRefreshEnabled)

	// Rename without touching auto_refresh_enabled (the field is omitted, as
	// a client that only knows about renaming would do). It must NOT be
	// silently disabled.
	renamed, err := svc.UpdateImportConnection(context.Background(), UpdateImportConnectionInput{
		OwnerUserID: 1,
		ID:          conn.ID,
		DisplayName: "Renamed",
		ConfigJSON:  "{}",
	})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", renamed.DisplayName)
	assert.True(t, renamed.AutoRefreshEnabled, "omitting auto_refresh_enabled on update must preserve the existing setting")

	// Rotating the key without touching auto_refresh_enabled must also
	// preserve it.
	rotated, err := svc.UpdateImportConnection(context.Background(), UpdateImportConnectionInput{
		OwnerUserID: 1,
		ID:          conn.ID,
		DisplayName: "Renamed",
		NewAPIKey:   "new-key-2222",
		ConfigJSON:  "{}",
	})
	require.NoError(t, err)
	assert.True(t, rotated.AutoRefreshEnabled, "key rotation must preserve the existing auto_refresh_enabled setting")

	// An explicit false must still disable it.
	disabled, err := svc.UpdateImportConnection(context.Background(), UpdateImportConnectionInput{
		OwnerUserID:        1,
		ID:                 conn.ID,
		DisplayName:        "Renamed",
		ConfigJSON:         "{}",
		AutoRefreshEnabled: new(false),
	})
	require.NoError(t, err)
	assert.False(t, disabled.AutoRefreshEnabled)
}

func TestUpdateImportConnection_RotateKey(t *testing.T) {
	svc := newTestConnectionService(t, testKey(), NoOpProber{})

	conn, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "Rotate Me", APIKey: "old-key-1111",
	})
	require.NoError(t, err)

	updated, err := svc.UpdateImportConnection(context.Background(), UpdateImportConnectionInput{
		OwnerUserID: 1,
		ID:          conn.ID,
		DisplayName: "Rotate Me",
		NewAPIKey:   "new-key-9999",
		ConfigJSON:  "{}",
	})
	require.NoError(t, err)
	assert.Contains(t, updated.KeyHint, "9999")    // new last-4
	assert.NotContains(t, updated.KeyHint, "1111") // old last-4 gone
}

// TestUpdateImportConnection_RotateKeyAndConfigProbesTheNewConfig is a
// regression test: rotating api_key and provider config together must probe
// the config actually being persisted, not the connection's stale config.
func TestUpdateImportConnection_RotateKeyAndConfigProbesTheNewConfig(t *testing.T) {
	repo := openTestConnectionRepo(t)
	var lastConfigJSON string
	prober := &recordingProber{lastConfigJSON: &lastConfigJSON}
	svc := NewImportConnectionService(repo, nil, testKey(), prober)

	conn, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "Rotate Both",
		APIKey: "old-key-1111", ConfigJSON: `{"label":"old"}`,
	})
	require.NoError(t, err)

	newConfigJSON := `{"label":"new"}`
	_, err = svc.UpdateImportConnection(context.Background(), UpdateImportConnectionInput{
		OwnerUserID: 1,
		ID:          conn.ID,
		DisplayName: "Rotate Both",
		NewAPIKey:   "new-key-9999",
		ConfigJSON:  newConfigJSON,
	})
	require.NoError(t, err)
	assert.Equal(t, newConfigJSON, lastConfigJSON, "probe must validate the new key against the config being saved, not the connection's stale config")
}

func TestCreateImportConnection_Trading212BaseURLConfigRejected(t *testing.T) {
	svc := newTestConnectionService(t, testKey(), NoOpProber{})

	_, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1,
		SourceKind:  "trading212",
		DisplayName: "Unsafe Config",
		APIKey:      "good-key-1234",
		ConfigJSON:  `{"base_url":"https://attacker.example"}`,
	})

	require.Error(t, err)
	assert.Equal(t, ValidationError{Message: "trading212 config must not include base_url"}, err)
}

func TestUpdateImportConnection_KeyRotationProbeFailure(t *testing.T) {
	svc := newTestConnectionService(t, testKey(), NoOpProber{})

	conn, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "Probe Fail", APIKey: "good-key-1234",
	})
	require.NoError(t, err)

	badProber := errProber{err: ErrProviderUnauthorized}
	badSvc := NewImportConnectionService(openTestConnectionRepo(t), nil, testKey(), badProber)

	// Create using the good svc first, so the connection exists.
	// For this test case, use the original repo with a bad prober.
	// Simpler: use goodSvc for creation, then try a rotation with badProber directly.
	// Instead, test that a new service with a bad prober rejects rotation.
	// (The connection won't exist in badSvc's DB, so this tests the prober rejection code path.)
	_ = badSvc
	_ = conn

	// Create with good prober, then rotate with bad prober on the same DB.
	repo := openTestConnectionRepo(t)
	goodSvc := NewImportConnectionService(repo, nil, testKey(), NoOpProber{})
	badSvc2 := NewImportConnectionService(repo, nil, testKey(), errProber{err: ErrProviderUnauthorized})

	conn2, err := goodSvc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "Rotate Probe", APIKey: "good-key",
	})
	require.NoError(t, err)

	_, err = badSvc2.UpdateImportConnection(context.Background(), UpdateImportConnectionInput{
		OwnerUserID: 1, ID: conn2.ID, DisplayName: "Rotate Probe", NewAPIKey: "bad-key",
	})
	require.ErrorIs(t, err, ErrProviderUnauthorized)

	// Verify the original key is still there (unchanged).
	updated, err := goodSvc.GetImportConnection(context.Background(), conn2.ID)
	require.NoError(t, err)
	assert.Equal(t, conn2.KeyHint, updated.KeyHint)
}

func TestDeleteImportConnection_RemovesIt(t *testing.T) {
	svc := newTestConnectionService(t, testKey(), NoOpProber{})

	conn, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "Delete Me", APIKey: "key",
	})
	require.NoError(t, err)

	require.NoError(t, svc.DeleteImportConnection(context.Background(), conn.ID))

	_, err = svc.GetImportConnection(context.Background(), conn.ID)
	require.ErrorIs(t, err, ErrImportConnectionNotFound)
}

func TestDeleteImportConnection_NotFoundError(t *testing.T) {
	svc := newTestConnectionService(t, testKey(), NoOpProber{})
	err := svc.DeleteImportConnection(context.Background(), 9999)
	require.ErrorIs(t, err, ErrImportConnectionNotFound)
}

func TestCreateImportConnection_ValidationErrors(t *testing.T) {
	svc := newTestConnectionService(t, testKey(), NoOpProber{})
	ctx := context.Background()

	cases := []struct {
		name  string
		input CreateImportConnectionInput
	}{
		{"empty display_name", CreateImportConnectionInput{OwnerUserID: 1, SourceKind: "trading212", APIKey: "k"}},
		{"empty source_kind", CreateImportConnectionInput{OwnerUserID: 1, DisplayName: "x", APIKey: "k"}},
		{"empty api_key", CreateImportConnectionInput{OwnerUserID: 1, SourceKind: "trading212", DisplayName: "x"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateImportConnection(ctx, tc.input)
			var ve ValidationError
			require.True(t, errors.As(err, &ve), "expected ValidationError, got %T: %v", err, err)
		})
	}
}

func TestCreateImportConnection_DisplayNameIsTrimmed(t *testing.T) {
	svc := newTestConnectionService(t, testKey(), NoOpProber{})
	conn, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1,
		SourceKind:  "trading212",
		DisplayName: "  My ISA  ",
		APIKey:      "api-key-abcd",
	})
	require.NoError(t, err)
	assert.Equal(t, "My ISA", conn.DisplayName)
}

func TestCreateImportConnection_InvalidConfigJSONRejected(t *testing.T) {
	svc := newTestConnectionService(t, testKey(), NoOpProber{})
	_, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1,
		SourceKind:  "trading212",
		DisplayName: "Bad Config",
		APIKey:      "api-key-abcd",
		ConfigJSON:  "{not valid json",
	})
	var ve ValidationError
	require.True(t, errors.As(err, &ve), "expected ValidationError, got %T: %v", err, err)
}

func TestUpdateImportConnection_DisplayNameIsTrimmedAndInvalidConfigRejected(t *testing.T) {
	svc := newTestConnectionService(t, testKey(), NoOpProber{})
	conn, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "Original", APIKey: "api-key-abcd",
	})
	require.NoError(t, err)

	updated, err := svc.UpdateImportConnection(context.Background(), UpdateImportConnectionInput{
		OwnerUserID: 1, ID: conn.ID, DisplayName: "  Renamed  ",
	})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", updated.DisplayName)

	_, err = svc.UpdateImportConnection(context.Background(), UpdateImportConnectionInput{
		OwnerUserID: 1, ID: conn.ID, DisplayName: "Renamed", ConfigJSON: "{not valid json",
	})
	var ve ValidationError
	require.True(t, errors.As(err, &ve), "expected ValidationError, got %T: %v", err, err)
}

func TestOpenSecret_ReturnsPlaintext(t *testing.T) {
	svc := newTestConnectionService(t, testKey(), NoOpProber{})

	conn, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "Secret Fetch", APIKey: "the-real-key-xyz",
	})
	require.NoError(t, err)

	secret, err := svc.OpenSecret(context.Background(), conn.ID)
	require.NoError(t, err)
	assert.Equal(t, "the-real-key-xyz", secret)
}

// --- cash_account_id (B-T212-INVST / Slice 4b) ---

func TestCreateImportConnection_CashAccountIDSetAndValidated(t *testing.T) {
	svc, database := newTestConnectionServiceWithAccounts(t)
	accountID := seedTestAccount(t, database, "active", true)

	conn, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "ISA", APIKey: "api-key-xxxx",
		CashAccountID: &accountID,
	})
	require.NoError(t, err)
	require.NotNil(t, conn.CashAccountID)
	assert.Equal(t, accountID, *conn.CashAccountID)
}

func TestCreateImportConnection_CashAccountIDNotFoundRejected(t *testing.T) {
	svc, _ := newTestConnectionServiceWithAccounts(t)
	missing := int64(999999)

	_, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "ISA", APIKey: "api-key-xxxx",
		CashAccountID: &missing,
	})
	var ve ValidationError
	require.True(t, errors.As(err, &ve), "expected ValidationError, got %T: %v", err, err)
}

func TestCreateImportConnection_CashAccountIDNonPostableRejected(t *testing.T) {
	svc, database := newTestConnectionServiceWithAccounts(t)
	accountID := seedTestAccount(t, database, "active", false)

	_, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "ISA", APIKey: "api-key-xxxx",
		CashAccountID: &accountID,
	})
	var ve ValidationError
	require.True(t, errors.As(err, &ve), "expected ValidationError, got %T: %v", err, err)
}

func TestCreateImportConnection_CashAccountIDArchivedRejected(t *testing.T) {
	svc, database := newTestConnectionServiceWithAccounts(t)
	accountID := seedTestAccount(t, database, "archived", true)

	_, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "ISA", APIKey: "api-key-xxxx",
		CashAccountID: &accountID,
	})
	var ve ValidationError
	require.True(t, errors.As(err, &ve), "expected ValidationError, got %T: %v", err, err)
}

func TestCreateImportConnection_CashAccountIDClosedRejected(t *testing.T) {
	svc, database := newTestConnectionServiceWithAccounts(t)
	accountID := seedTestAccount(t, database, "closed", true)

	_, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "ISA", APIKey: "api-key-xxxx",
		CashAccountID: &accountID,
	})
	var ve ValidationError
	require.True(t, errors.As(err, &ve), "expected ValidationError, got %T: %v", err, err)
	assert.Equal(t, "cash_account_id must reference an active account", ve.Message)
}

func TestCreateImportConnection_CashAccountIDNonAssetRejected(t *testing.T) {
	svc, database := newTestConnectionServiceWithAccounts(t)
	accountID := seedTestAccountWithClass(t, database, "active", true, "expense", "expense")

	_, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "ISA", APIKey: "api-key-xxxx",
		CashAccountID: &accountID,
	})
	var ve ValidationError
	require.True(t, errors.As(err, &ve), "expected ValidationError, got %T: %v", err, err)
	assert.Equal(t, "cash_account_id must reference an asset account", ve.Message)
}

func TestCreateImportConnection_CashAccountIDSystemAccountRejected(t *testing.T) {
	svc, database := newTestConnectionServiceWithAccounts(t)
	accountID := seedCommodityTradingAccount(t, database)

	_, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "ISA", APIKey: "api-key-xxxx",
		CashAccountID: &accountID,
	})
	var ve ValidationError
	require.True(t, errors.As(err, &ve), "expected ValidationError, got %T: %v", err, err)
	assert.Equal(t, "cash_account_id must reference a user account", ve.Message)
}

// TestUpdateImportConnection_OmittedCashAccountIDPreservesExisting guards the
// same PATCH-omission class of bug as auto_refresh_enabled (T-21-adjacent,
// see docs/backlog.md's "PATCH omission overwrites" review checklist entry):
// a rename/rotate request that never mentions cash_account_id must not clear it.
func TestUpdateImportConnection_OmittedCashAccountIDPreservesExisting(t *testing.T) {
	svc, database := newTestConnectionServiceWithAccounts(t)
	accountID := seedTestAccount(t, database, "active", true)

	conn, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "ISA", APIKey: "api-key-xxxx",
		CashAccountID: &accountID,
	})
	require.NoError(t, err)

	renamed, err := svc.UpdateImportConnection(context.Background(), UpdateImportConnectionInput{
		OwnerUserID: 1, ID: conn.ID, DisplayName: "ISA Renamed", ConfigJSON: "{}",
	})
	require.NoError(t, err)
	require.NotNil(t, renamed.CashAccountID)
	assert.Equal(t, accountID, *renamed.CashAccountID)
}

func TestUpdateImportConnection_CashAccountIDCanBeSetLater(t *testing.T) {
	svc, database := newTestConnectionServiceWithAccounts(t)

	conn, err := svc.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1, SourceKind: "trading212", DisplayName: "ISA", APIKey: "api-key-xxxx",
	})
	require.NoError(t, err)
	assert.Nil(t, conn.CashAccountID)

	accountID := seedTestAccount(t, database, "active", true)
	updated, err := svc.UpdateImportConnection(context.Background(), UpdateImportConnectionInput{
		OwnerUserID: 1, ID: conn.ID, DisplayName: conn.DisplayName, ConfigJSON: "{}",
		CashAccountID: &accountID,
	})
	require.NoError(t, err)
	require.NotNil(t, updated.CashAccountID)
	assert.Equal(t, accountID, *updated.CashAccountID)
}
