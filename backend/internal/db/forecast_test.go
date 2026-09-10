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

func newForecastTestRepository(t testing.TB) (*sql.DB, *ForecastRepository) {
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
func insertForecastTransaction(t testing.TB, database *sql.DB, transactionID, versionID int64, status string, entryDate string, postings []PostingSpec) {
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

func TestForecastLearningUsesPostedHistoryOnly(t *testing.T) {
	database, repository := newForecastTestRepository(t)
	legs := []PostingSpec{{AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-100"), QuantityScale: 2}, {AccountID: 2, CommodityID: 1, QuantityValue: exact.MustParse("100"), QuantityScale: 2}}
	insertForecastTransaction(t, database, 80, 80, "posted", "2026-08-20", legs)
	_, err := database.Exec(`
		INSERT INTO transaction_versions (id, book_id, transaction_id, version_seq, supersedes_version_id, status, transaction_kind, transaction_date, recorded_at, changed_by_user_id, change_reason)
		VALUES (800, 1, 80, 2, 80, 'posted', 'ordinary', '2026-08-20', '2026-08-31T00:00:00Z', 1, 'current learning fixture');
		INSERT INTO journal_entries (id, book_id, transaction_version_id, entry_seq, entry_date, entry_kind)
		VALUES (8000, 1, 800, 1, '2026-08-20', 'ordinary');
		INSERT INTO posting_versions (id, book_id, transaction_version_id, journal_entry_id, posting_line_id, line_seq, account_id, quantity_value, quantity_scale, commodity_id, reconciliation_status)
		VALUES
			(80001, 1, 800, 8000, 801, 1, 1, '-200', 2, 1, 'uncleared'),
			(80002, 1, 800, 8000, 802, 2, 2, '200', 2, 1, 'uncleared')`)
	require.NoError(t, err)
	insertForecastTransaction(t, database, 81, 81, "voided", "2026-08-21", legs)
	insertForecastTransaction(t, database, 82, 82, "draft", "2026-08-22", legs)
	insertForecastTransaction(t, database, 83, 83, "posted", "2026-08-23", legs)
	_, err = database.Exec(`UPDATE transactions SET deleted_at = '2026-08-31T00:00:00Z' WHERE id = 83`)
	require.NoError(t, err)
	insertForecastTransaction(t, database, 84, 84, "posted", "2026-09-01", legs)

	template := createRentTemplate(t, NewRecurringRepository(database))
	_, err = database.Exec(`INSERT INTO recurring_occurrences (book_id, template_id, occurrence_date, status, transaction_id, materialized_at, created_at, updated_at) VALUES (1, ?, '2026-08-20', 'generated', 80, '2026-08-20T00:00:00Z', '2026-08-20T00:00:00Z', '2026-08-20T00:00:00Z')`, template.ID)
	require.NoError(t, err)
	var auditsBefore int
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsBefore))

	snapshot, err := repository.LoadLearningSnapshot(context.Background(), ForecastLearningSnapshotRequest{BookID: 1, AccountIDs: []int64{1, 2}, HistoryStart: "2026-01-01", HistoryEnd: "2026-08-31"})
	require.NoError(t, err)
	require.Len(t, snapshot.Postings, 2, "only both siblings of the one current posted, non-deleted, in-range entry are read")
	assert.Equal(t, []int64{1, 2}, []int64{snapshot.Postings[0].AccountID, snapshot.Postings[1].AccountID})
	assert.Equal(t, int64(800), snapshot.Postings[0].TransactionVersionID, "superseded posted versions are excluded")
	assert.Equal(t, "-200", snapshot.Postings[0].QuantityValue.String())
	assert.True(t, snapshot.Postings[0].RecurringOccurrenceID.Valid, "real occurrence identity is retained for exact overlap exclusion")
	var auditsAfter int
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsAfter))
	assert.Equal(t, auditsBefore, auditsAfter, "learning snapshot reads have no audit or domain writes")
}

func TestForecastLearningSnapshotRejectsPostingPrefixes(t *testing.T) {
	database, repository := newForecastTestRepository(t)
	insertForecastTransaction(t, database, 90, 90, "posted", "2026-08-20", []PostingSpec{{AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-100")}, {AccountID: 2, CommodityID: 1, QuantityValue: exact.MustParse("100")}})
	_, err := repository.LoadLearningSnapshot(context.Background(), ForecastLearningSnapshotRequest{BookID: 1, AccountIDs: []int64{1, 2}, HistoryStart: "2026-01-01", HistoryEnd: "2026-08-31", PostingLimit: 1})
	require.ErrorIs(t, err, ErrForecastInputTooLarge)
}

func TestForecastLearningRateCandidatesIncludeModelOnlyCurrency(t *testing.T) {
	ids := forecastCandidateCommodityIDs([]int64{1}, nil, nil, nil, []ForecastLearningPostingRecord{
		{AccountID: 1, CommodityID: 2},
		{AccountID: 9, CommodityID: 3},
	})
	assert.Equal(t, []int64{2}, ids, "learning history can require FX even when the core curve has no movement in that currency")
}

func BenchmarkForecastLearningRead(b *testing.B) {
	database, repository := newForecastTestRepository(b)
	insertForecastTransaction(b, database, 100, 100, "posted", "2026-08-20", []PostingSpec{{AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-100")}, {AccountID: 2, CommodityID: 1, QuantityValue: exact.MustParse("100")}})
	request := ForecastLearningSnapshotRequest{BookID: 1, AccountIDs: []int64{1, 2}, HistoryStart: "2021-09-01", HistoryEnd: "2026-08-31"}
	b.ResetTimer()
	for range b.N {
		_, err := repository.LoadLearningSnapshot(context.Background(), request)
		if err != nil {
			b.Fatal(err)
		}
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

func TestForecastConstantFXStalenessAndTies(t *testing.T) {
	database, repository := newForecastTestRepository(t)
	_, err := database.Exec(`
		INSERT INTO commodities (id, book_id, code, kind, is_builtin, created_at, created_by_user_id)
		VALUES (2, 1, 'USD', 'currency', 1, '2026-01-01T00:00:00Z', 1);
		INSERT INTO commodity_versions (commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason, status, symbol, display_symbol, name, standard_scale, max_quantity_scale)
		VALUES (2, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'seed', 'active', 'USD', '$', 'US Dollar', 2, 2);
		INSERT INTO price_series (id, book_id, base_commodity_id, quote_commodity_id, quote_type, adjustment_basis, status, created_at, created_by_user_id)
		VALUES (1, 1, 2, 1, 'manual', 'raw', 'active', '2026-08-01T00:00:00Z', 1);
		INSERT INTO price_observations (id, book_id, series_id, base_commodity_id, quote_commodity_id, quote_type, adjustment_basis, price_value, price_scale, base_quantity_value, base_quantity_scale, valuation_date, is_manual, is_derived, derivation_json, metadata_json, recorded_at, created_by_user_id)
		VALUES
			(1, 1, 1, 2, 1, 'manual', 'raw', 80, 2, 1, 0, '2026-08-24', 1, 0, '{}', '{}', '2026-08-24T10:00:00Z', 1),
			(2, 1, 1, 2, 1, 'manual', 'raw', 90, 2, 1, 0, '2026-08-31', 1, 0, '{}', '{}', '2026-08-31T10:00:00Z', 1),
			(3, 1, 1, 2, 1, 'manual', 'raw', 91, 2, 1, 0, '2026-08-31', 1, 1, '{}', '{}', '2026-08-31T11:00:00Z', 1),
			(4, 1, 1, 2, 1, 'manual', 'raw', 92, 2, 1, 0, '2026-08-31', 1, 0, '{}', '{}', '2026-08-31T11:00:00Z', 1),
			(5, 1, 1, 2, 1, 'manual', 'raw', 200, 2, 1, 0, '2026-09-01', 1, 0, '{}', '{}', '2026-09-01T10:00:00Z', 1);
		UPDATE price_observations SET voided_at = '2026-08-31T12:00:00Z', void_reason = 'withdrawn' WHERE id = 4;
	`)
	require.NoError(t, err)
	insertForecastTransaction(t, database, 70, 70, "posted", "2026-08-31", []PostingSpec{{AccountID: 1, CommodityID: 2, QuantityValue: exact.MustParse("100"), QuantityScale: 2}, {AccountID: 2, CommodityID: 2, QuantityValue: exact.MustParse("-100"), QuantityScale: 2}})
	quoteID := int64(1)
	request := forecastSnapshotRequest(1)
	snapshot, err := repository.LoadResolvedSnapshot(context.Background(), request, func(ForecastSnapshot) (ForecastSnapshotResolution, error) {
		return ForecastSnapshotResolution{AccountIDs: []int64{1}, ThroughDate: "2026-12-31", AsOfDate: "2026-08-31", ReportingCurrencyID: &quoteID}, nil
	})
	require.NoError(t, err)
	require.Len(t, snapshot.Rates, 1)
	assert.Equal(t, int64(3), snapshot.Rates[0].ObservationID, "latest recorded active tie wins; voided and future rows cannot win")
	assert.True(t, snapshot.Rates[0].IsDerived)
}
