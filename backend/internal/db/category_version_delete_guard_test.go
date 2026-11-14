package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

// #417: SQL removal of an account version is guarded the same way for
// categories and holdings. These tests work below the category service, whose
// own refusals (TestCategoryDeleteRejectsCategoryUsedByPosting) stay unchanged.

const appendOnlyAccountVersions = "account_versions rows are append-only"

func TestUsedCategoryAndHoldingVersionDeletionRefuseAlike(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database := seedReplayTestBook(t)
	before := captureLedgerState(t, database)
	var category, holding int64
	require.NoError(t, database.QueryRowContext(ctx, `SELECT av.account_id FROM account_versions av
		WHERE av.account_class IN ('income','expense')
		  AND json_extract(av.metadata_json,'$.category.is_builtin')=0
		  AND EXISTS(SELECT 1 FROM posting_versions pv WHERE pv.account_id=av.account_id)
		ORDER BY av.id LIMIT 1`).Scan(&category))
	require.NoError(t, database.QueryRowContext(ctx, `SELECT av.account_id FROM account_versions av
		JOIN accounts a ON a.id=av.account_id
		WHERE av.account_class='asset' AND a.system_role IS NULL
		  AND EXISTS(SELECT 1 FROM posting_versions pv WHERE pv.account_id=av.account_id)
		ORDER BY av.id LIMIT 1`).Scan(&holding))

	for name, accountID := range map[string]int64{"custom category": category, "holding": holding} {
		tx, err := database.BeginTx(ctx, nil)
		require.NoError(t, err)
		// The category service's own statement shape: every version at once.
		_, err = tx.ExecContext(ctx, `DELETE FROM account_versions WHERE account_id=?`, accountID)
		require.ErrorContains(t, err, appendOnlyAccountVersions, name)
		var versions int
		require.NoError(t, tx.QueryRowContext(ctx,
			`SELECT count(*) FROM account_versions WHERE account_id=?`, accountID).Scan(&versions))
		require.Positive(t, versions, "%s: the refused statement must remove no version", name)
		require.NoError(t, tx.Rollback())
	}
	require.Equal(t, before, captureLedgerState(t, database))
}

func TestUnusedCategoryVersionsStillDelete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database := seedReplayTestBook(t)
	for name, metadata := range map[string]string{
		"custom":  `{"category":{"type":"expense","is_builtin":false}}`,
		"starter": `{"category":{"type":"expense","is_builtin":false,"is_starter":true,"builtin_key":"guard_starter"}}`,
	} {
		categoryID := createGuardTestCategory(t, database, name, metadata, sql.NullInt64{})
		require.NoError(t, deleteAccountWithVersions(ctx, database, categoryID), name)
		var remaining int
		require.NoError(t, database.QueryRowContext(ctx,
			`SELECT count(*) FROM accounts WHERE id=?`, categoryID).Scan(&remaining))
		require.Zero(t, remaining, name)
	}
}

func TestReferencedUnusedCategoryVersionsStillRefuse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database := seedReplayTestBook(t)
	custom := `{"category":{"type":"expense","is_builtin":false}}`

	// A category that was once a parent keeps that history: the child's
	// version names it even after the child moves elsewhere.
	parent := createGuardTestCategory(t, database, "former parent", custom, sql.NullInt64{})
	child := createGuardTestCategory(t, database, "child", custom, sql.NullInt64{Int64: parent, Valid: true})
	_, err := NewCategoryRepository(database).UpdateCategory(ctx, UpdateCategoryParams{
		BookID: 1, CategoryID: child, ChangedByUserID: 1, AuthSessionID: 1, RequestID: "guard-move",
		OriginType: "browser_api", Operation: "category.update",
		Spec:         AccountSpec{Name: "child", AccountClass: "expense", AccountKind: "expense", AllowsPostings: true, MetadataJSON: custom, OpenedOn: "0001-01-01"},
		ChangeReason: "moved to the top level", RecordedAt: "2026-10-27T00:00:00Z", EffectiveFrom: "0001-01-01",
	})
	require.NoError(t, err)
	err = deleteAccountWithVersions(ctx, database, parent)
	require.ErrorContains(t, err, appendOnlyAccountVersions, "historical child")

	// A payee's default category is a reference the account row itself keeps.
	defaulted := createGuardTestCategory(t, database, "payee default", custom, sql.NullInt64{})
	_, err = database.ExecContext(ctx, `INSERT INTO payee_versions (payee_id, version_seq, effective_from, recorded_at,
		changed_by_user_id, change_reason, status, name, normalized_name, default_account_id,
		default_category_account_id, metadata_json, change_audit_event_id)
		SELECT payee_id, version_seq + 1, effective_from, '2026-10-27T00:00:00Z', changed_by_user_id,
			'default category', status, name, normalized_name, NULL, ?, '{}', change_audit_event_id
		FROM payee_versions WHERE id = 1`, defaulted)
	require.NoError(t, err)
	err = deleteAccountWithVersions(ctx, database, defaulted)
	require.ErrorContains(t, err, "FOREIGN KEY constraint failed", "default payee reference")

	for _, accountID := range []int64{parent, defaulted} {
		var versions int
		require.NoError(t, database.QueryRowContext(ctx,
			`SELECT count(*) FROM account_versions WHERE account_id=?`, accountID).Scan(&versions))
		require.Positive(t, versions, "a refused removal must leave every version in place")
	}
}

func createGuardTestCategory(t *testing.T, database *sql.DB, name string, metadata string, parent sql.NullInt64) int64 {
	t.Helper()
	record, err := NewCategoryRepository(database).CreateCategory(context.Background(), CreateCategoryParams{
		BookID: 1, CreatedByUserID: 1, AuthSessionID: 1, RequestID: "guard-" + name,
		OriginType: "browser_api", Operation: "category.create", ChangeReason: "created category",
		CreatedAt: "2026-10-27T00:00:00Z", EffectiveFrom: "0001-01-01",
		Spec: AccountSpec{Name: name, AccountClass: "expense", AccountKind: "expense", ParentAccountID: parent,
			AllowsPostings: true, MetadataJSON: metadata, OpenedOn: "0001-01-01"},
	})
	require.NoError(t, err)
	return record.ID
}

// deleteAccountWithVersions is the repositories' removal, as raw SQL in one
// transaction, so a refusal at either statement leaves nothing changed.
func deleteAccountWithVersions(ctx context.Context, database *sql.DB, accountID int64) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM account_versions WHERE account_id=?`, accountID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM accounts WHERE id=? AND system_role IS NULL`, accountID); err != nil {
		return err
	}
	return tx.Commit()
}
