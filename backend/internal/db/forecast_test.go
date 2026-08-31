package db

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/exact"
)

func newForecastTestRepository(t *testing.T) (*sql.DB, *ForecastRepository) {
	t.Helper()
	writer := newRecurringTestDatabase(t)
	var databaseFile string
	require.NoError(t, writer.QueryRow(`SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&databaseFile))
	reader, err := OpenReadOnly(context.Background(), "file:"+databaseFile)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reader.Close()) })
	return writer, NewForecastRepository(reader)
}

func forecastSnapshotRequest(accountIDs ...int64) ForecastSnapshotRequest {
	return ForecastSnapshotRequest{BookID: 1, OwnerUserID: 1, AccountIDs: accountIDs, ThroughDate: "2026-12-31"}
}

// insertForecastTransaction writes a deliberately small but structurally real
// current transaction version. The forecast repository must observe it through
// the same current-version/journal/posting relationships as production code.
func insertForecastTransaction(t *testing.T, database *sql.DB, transactionID, versionID int64, status string, entryDate string, postings []PostingSpec) {
	t.Helper()
	ctx := context.Background()
	_, err := database.ExecContext(ctx, `INSERT INTO transactions (id, book_id, created_at, created_by_user_id) VALUES (?, 1, '2026-08-31T00:00:00Z', 1)`, transactionID)
	require.NoError(t, err)
	_, err = database.ExecContext(ctx, `INSERT INTO transaction_versions (id, book_id, transaction_id, version_seq, status, transaction_kind, transaction_date, recorded_at, changed_by_user_id, change_reason) VALUES (?, 1, ?, 1, ?, 'ordinary', ?, '2026-08-31T00:00:00Z', 1, 'forecast fixture')`, versionID, transactionID, status, entryDate)
	require.NoError(t, err)
	entryID := versionID * 10
	_, err = database.ExecContext(ctx, `INSERT INTO journal_entries (id, book_id, transaction_version_id, entry_seq, entry_date, entry_kind) VALUES (?, 1, ?, 1, ?, 'ordinary')`, entryID, versionID, entryDate)
	require.NoError(t, err)
	for index, posting := range postings {
		lineID := transactionID*10 + int64(index+1)
		postingID := versionID*100 + int64(index+1)
		_, err = database.ExecContext(ctx, `INSERT INTO posting_lines (id, book_id, transaction_id, line_key, created_at, created_by_user_id) VALUES (?, 1, ?, ?, '2026-08-31T00:00:00Z', 1)`, lineID, transactionID, "line-"+string(rune('a'+index)))
		require.NoError(t, err)
		_, err = database.ExecContext(ctx, `INSERT INTO posting_versions (id, book_id, transaction_version_id, journal_entry_id, posting_line_id, line_seq, account_id, quantity_value, quantity_scale, commodity_id, reconciliation_status) VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?, ?, 'uncleared')`, postingID, versionID, entryID, lineID, index+1, posting.AccountID, posting.QuantityValue, posting.QuantityScale, posting.CommodityID)
		require.NoError(t, err)
	}
}

func TestForecastSnapshotReadsCurrentPostedVersionsOnly(t *testing.T) {
	database, repository := newForecastTestRepository(t)
	insertForecastTransaction(t, database, 10, 10, "posted", "2026-08-20", []PostingSpec{{AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("100"), QuantityScale: 2}, {AccountID: 2, CommodityID: 1, QuantityValue: exact.MustParse("-100"), QuantityScale: 2}})
	insertForecastTransaction(t, database, 11, 11, "voided", "2026-08-21", []PostingSpec{{AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("200"), QuantityScale: 2}, {AccountID: 2, CommodityID: 1, QuantityValue: exact.MustParse("-200"), QuantityScale: 2}})
	insertForecastTransaction(t, database, 12, 12, "posted", "2026-08-22", []PostingSpec{{AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("300"), QuantityScale: 2}, {AccountID: 2, CommodityID: 1, QuantityValue: exact.MustParse("-300"), QuantityScale: 2}})
	_, err := database.Exec(`UPDATE transactions SET deleted_at = '2026-08-31T00:00:00Z' WHERE id = 12`)
	require.NoError(t, err)

	snapshot, err := repository.LoadSnapshot(context.Background(), forecastSnapshotRequest(1))
	require.NoError(t, err)
	require.Len(t, snapshot.PostedPostings, 1)
	assert.Equal(t, int64(10), snapshot.PostedPostings[0].TransactionID)
	assert.Equal(t, "100", snapshot.PostedPostings[0].QuantityValue.String())
}

func TestForecastSnapshotBulkQueriesUseExistingIndexes(t *testing.T) {
	database, _ := newForecastTestRepository(t)
	insertForecastTransaction(t, database, 15, 15, "posted", "2026-08-20", []PostingSpec{{AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("100"), QuantityScale: 2}, {AccountID: 2, CommodityID: 1, QuantityValue: exact.MustParse("-100"), QuantityScale: 2}})
	rows, err := database.Query(`EXPLAIN QUERY PLAN
		SELECT pv.id FROM current_transaction_versions tv
		JOIN transactions t ON t.id = tv.transaction_id
		JOIN journal_entries je ON je.transaction_version_id = tv.id
		JOIN posting_versions pv ON pv.journal_entry_id = je.id
		WHERE tv.book_id = 1 AND tv.status = 'posted' AND t.deleted_at IS NULL
			AND je.entry_date <= '2026-12-31' AND pv.account_id IN (1)`)
	require.NoError(t, err)
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		details = append(details, detail)
	}
	require.NoError(t, rows.Err())
	assert.True(t, strings.Contains(strings.Join(details, "\n"), "posting_versions_account_idx"), strings.Join(details, "\n"))
}

func TestForecastSnapshotKeepsDraftsAfterTemplateArchiveAndAccountEdit(t *testing.T) {
	database, repository := newForecastTestRepository(t)
	recurring := NewRecurringRepository(database)
	template := createRentTemplate(t, recurring)
	insertForecastTransaction(t, database, 20, 20, "draft", "2026-09-01", []PostingSpec{{AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-120000"), QuantityScale: 2}, {AccountID: 2, CommodityID: 1, QuantityValue: exact.MustParse("120000"), QuantityScale: 2}})
	_, err := database.Exec(`INSERT INTO recurring_occurrences (book_id, template_id, occurrence_date, status, transaction_id, materialized_at, created_at, updated_at) VALUES (1, ?, '2026-09-01', 'generated', 20, '2026-08-31T00:00:00Z', '2026-08-31T00:00:00Z', '2026-08-31T00:00:00Z')`, template.ID)
	require.NoError(t, err)
	// The current template no longer touches checking and is archived. Its
	// saved draft still does, so the draft-link branch must retain it.
	_, err = database.Exec(`UPDATE recurring_template_postings SET account_id = 2 WHERE template_id = ?`, template.ID)
	require.NoError(t, err)
	_, err = database.Exec(`UPDATE recurring_templates SET archived_at = '2026-08-31T00:00:00Z' WHERE id = ?`, template.ID)
	require.NoError(t, err)

	snapshot, err := repository.LoadSnapshot(context.Background(), forecastSnapshotRequest(1))
	require.NoError(t, err)
	require.Len(t, snapshot.Templates, 1)
	assert.True(t, snapshot.Templates[0].ArchivedAt.Valid)
	require.Len(t, snapshot.DraftPostings, 2)
	assert.Equal(t, template.ID, snapshot.DraftPostings[0].TemplateID)
}

func TestForecastSnapshotLoadsFullDraftCounterparts(t *testing.T) {
	database, repository := newForecastTestRepository(t)
	recurring := NewRecurringRepository(database)
	template := createRentTemplate(t, recurring)
	insertForecastTransaction(t, database, 30, 30, "draft", "2027-01-15", []PostingSpec{{AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-100"), QuantityScale: 2}, {AccountID: 2, CommodityID: 1, QuantityValue: exact.MustParse("100"), QuantityScale: 2}})
	_, err := database.Exec(`INSERT INTO recurring_occurrences (book_id, template_id, occurrence_date, status, transaction_id, materialized_at, created_at, updated_at) VALUES (1, ?, '2026-09-01', 'generated', 30, '2026-08-31T00:00:00Z', '2026-08-31T00:00:00Z', '2026-08-31T00:00:00Z')`, template.ID)
	require.NoError(t, err)

	snapshot, err := repository.LoadSnapshot(context.Background(), forecastSnapshotRequest(1))
	require.NoError(t, err)
	require.Len(t, snapshot.DraftPostings, 2)
	assert.Equal(t, []int64{1, 2}, []int64{snapshot.DraftPostings[0].AccountID, snapshot.DraftPostings[1].AccountID})
	assert.Equal(t, "2027-01-15", snapshot.DraftPostings[0].EntryDate, "full draft validation retains an out-of-horizon entry")
}

func TestForecastSnapshotSeesOneSideOfConcurrentPosting(t *testing.T) {
	database, repository := newForecastTestRepository(t)
	insertForecastTransaction(t, database, 40, 40, "posted", "2026-08-20", []PostingSpec{{AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("100"), QuantityScale: 2}, {AccountID: 2, CommodityID: 1, QuantityValue: exact.MustParse("-100"), QuantityScale: 2}})
	var before, after []ForecastPostingRecord
	err := repository.WithSnapshot(context.Background(), func(reader *ForecastSnapshotReader) error {
		var err error
		before, err = reader.postedPostings(context.Background(), 1, []int64{1}, "2026-12-31", 10)
		if err != nil {
			return err
		}
		insertForecastTransaction(t, database, 41, 41, "posted", "2026-08-21", []PostingSpec{{AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("200"), QuantityScale: 2}, {AccountID: 2, CommodityID: 1, QuantityValue: exact.MustParse("-200"), QuantityScale: 2}})
		after, err = reader.postedPostings(context.Background(), 1, []int64{1}, "2026-12-31", 10)
		return err
	})
	require.NoError(t, err)
	assert.Len(t, before, 1)
	assert.Len(t, after, 1, "the established read snapshot must remain coherent while the writer commits")
	newSnapshot, err := repository.LoadSnapshot(context.Background(), forecastSnapshotRequest(1))
	require.NoError(t, err)
	assert.Len(t, newSnapshot.PostedPostings, 2)
}

func TestForecastSnapshotRejectsTruncatedInputs(t *testing.T) {
	database, repository := newForecastTestRepository(t)
	insertForecastTransaction(t, database, 50, 50, "posted", "2026-08-20", []PostingSpec{{AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("100"), QuantityScale: 2}, {AccountID: 2, CommodityID: 1, QuantityValue: exact.MustParse("-100"), QuantityScale: 2}})
	request := forecastSnapshotRequest(1)
	request.Limits = DefaultForecastSnapshotLimits()
	request.Limits.PostedPostingRows = 0
	_, err := repository.LoadSnapshot(context.Background(), request)
	require.ErrorIs(t, err, ErrForecastInputTooLarge)
}

func TestForecastSnapshotEmptyScopeNeverMeansAllAccounts(t *testing.T) {
	database, repository := newForecastTestRepository(t)
	insertForecastTransaction(t, database, 60, 60, "posted", "2026-08-20", []PostingSpec{{AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("100"), QuantityScale: 2}, {AccountID: 2, CommodityID: 1, QuantityValue: exact.MustParse("-100"), QuantityScale: 2}})
	request := forecastSnapshotRequest()
	request.Limits = DefaultForecastSnapshotLimits()
	request.Limits.PostedPostingRows = 0
	snapshot, err := repository.LoadSnapshot(context.Background(), request)
	require.NoError(t, err)
	assert.Empty(t, snapshot.PostedPostings)
	assert.Empty(t, snapshot.Templates)
	assert.Empty(t, snapshot.DraftPostings)
}
