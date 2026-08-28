package app

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/testdb"
)

type selfCheckHarness struct {
	service  *SelfCheckService
	writer   *sql.DB
	readOnly *sql.DB
}

func newSelfCheckHarness(t *testing.T) *selfCheckHarness {
	t.Helper()

	ctx := context.Background()
	writer, databaseURL := testdb.Open(t)
	seedRestoreLedger(t, writer)

	readOnly, err := db.OpenReadOnly(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, readOnly.Close()) })

	service := NewSelfCheckService(db.NewSelfCheckRepository(writer, readOnly))
	service.SetNowForTest(func() time.Time { return time.Date(2026, 8, 24, 4, 0, 0, 0, time.UTC) })

	return &selfCheckHarness{service: service, writer: writer, readOnly: readOnly}
}

func (h *selfCheckHarness) run(t *testing.T) SelfCheckRun {
	t.Helper()

	run, err := h.service.RunSelfCheck(context.Background(), "manual")
	require.NoError(t, err)
	return run
}

func resultFor(t *testing.T, run SelfCheckRun, checkID string) SelfCheckResult {
	t.Helper()

	for _, result := range run.Results {
		if result.CheckID == checkID {
			return result
		}
	}
	t.Fatalf("no result for check %q", checkID)
	return SelfCheckResult{}
}

// A healthy book passes everything, and says why each check exists.
func TestSelfCheckPassesOnHealthyBook(t *testing.T) {
	t.Parallel()

	harness := newSelfCheckHarness(t)
	run := harness.run(t)

	assert.Equal(t, SelfCheckPassed, run.Status)
	assert.Zero(t, run.FailedCheckCount)

	expected := []string{
		CheckEntryBalance, CheckTransactionBalance, CheckBookBalance, CheckVersionIntegrity,
		CheckLotReconciliation, CheckCheckpointIntegrity, CheckAccountVersionCoverage,
		CheckSQLiteIntegrity, CheckAttachments,
	}
	require.Len(t, run.Results, len(expected))
	for _, checkID := range expected {
		result := resultFor(t, run, checkID)
		assert.NotEqual(t, SelfCheckFailed, result.Status, checkID)
		assert.NotEmpty(t, result.Summary, "%s must say what it found", checkID)
		assert.NotEmpty(t, result.Explanation, "%s must explain what it checks", checkID)
		assert.NotEmpty(t, result.NextStep, "%s must say what to do about it", checkID)
	}

	// The reserved slot reports not_applicable rather than passing: passing a
	// check nobody ran is how a coverage claim gets made by accident.
	assert.Equal(t, SelfCheckNotApplicable, resultFor(t, run, CheckAttachments).Status)
}

// One deliberate corruption per check, written with raw SQL because every one
// of these is something the service layer refuses to do.
func TestSelfCheckCatchesEachCorruption(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		corrupt func(t *testing.T, database *sql.DB)
		checkID string
	}{
		"an entry that does not balance": {
			checkID: CheckEntryBalance,
			corrupt: func(t *testing.T, database *sql.DB) {
				// Inserted, not updated: posting_versions rows are append-only
				// by schema trigger, so the reachable corruption is a backfill
				// that writes a one-sided entry.
				execRaw(t, database, `
					INSERT INTO transactions (id, book_id, created_at, created_by_user_id)
					VALUES (2, 1, '2026-06-05T00:00:00Z', 1);
					INSERT INTO transaction_versions (
						id, book_id, transaction_id, version_seq, status, transaction_kind, transaction_date,
						recorded_at, changed_by_user_id, change_reason
					) VALUES (2, 1, 2, 1, 'posted', 'ordinary', '2026-06-05', '2026-06-05T00:00:00Z', 1, 'backfill');
					INSERT INTO journal_entries (id, book_id, transaction_version_id, entry_seq, entry_date, entry_kind)
					VALUES (2, 1, 2, 1, '2026-06-05', 'ordinary');
					INSERT INTO posting_lines (id, book_id, transaction_id, line_key, created_at, created_by_user_id)
					VALUES (3, 1, 2, 'a', '2026-06-05T00:00:00Z', 1);
					INSERT INTO posting_versions (
						book_id, transaction_version_id, journal_entry_id, posting_line_id, line_seq,
						account_id, quantity_value, quantity_scale, commodity_id, reconciliation_status
					) VALUES (1, 2, 2, 3, 1, 1, '5000', 2, 1, 'uncleared');
				`)
			},
		},
		"a transaction whose entries do not net": {
			checkID: CheckTransactionBalance,
			corrupt: func(t *testing.T, database *sql.DB) {
				execRaw(t, database, `
					INSERT INTO journal_entries (id, book_id, transaction_version_id, entry_seq, entry_date, entry_kind)
					VALUES (2, 1, 1, 2, '2026-06-02', 'ordinary');
					INSERT INTO posting_lines (id, book_id, transaction_id, line_key, created_at, created_by_user_id)
					VALUES (3, 1, 1, 'c', '2026-06-02T00:00:00Z', 1);
					INSERT INTO posting_versions (
						book_id, transaction_version_id, journal_entry_id, posting_line_id, line_seq,
						account_id, quantity_value, quantity_scale, commodity_id, reconciliation_status
					) VALUES (1, 1, 2, 3, 1, 1, '5000', 2, 1, 'uncleared');
				`)
			},
		},
		"a posting whose entry belongs to another version": {
			checkID: CheckVersionIntegrity,
			corrupt: func(t *testing.T, database *sql.DB) {
				// posting_versions_same_book_and_lineage blocks this outright,
				// so producing it means dropping the trigger first — which is
				// exactly the situation the check is for: rows that did not
				// come through this app. The check is the second line of
				// defence behind the schema, not the first.
				execRaw(t, database, `DROP TRIGGER posting_versions_same_book_and_lineage`)
				execRaw(t, database, `
					INSERT INTO transaction_versions (
						id, book_id, transaction_id, version_seq, status, transaction_kind, transaction_date,
						recorded_at, changed_by_user_id, change_reason
					) VALUES (2, 1, 1, 2, 'posted', 'ordinary', '2026-06-01', '2026-06-01T00:00:00Z', 1, 'backfill');
					INSERT INTO posting_lines (id, book_id, transaction_id, line_key, created_at, created_by_user_id)
					VALUES (3, 1, 1, 'c', '2026-06-01T00:00:00Z', 1);
					INSERT INTO posting_versions (
						book_id, transaction_version_id, journal_entry_id, posting_line_id, line_seq,
						account_id, quantity_value, quantity_scale, commodity_id, reconciliation_status
					) VALUES (1, 2, 1, 3, 3, 1, '0', 2, 1, 'uncleared');
				`)
			},
		},
		"a lot that holds more than its account does": {
			checkID: CheckLotReconciliation,
			corrupt: func(t *testing.T, database *sql.DB) {
				// A lot needs a security commodity and a holding account in the
				// same book — the schema enforces that much — so the reachable
				// corruption is a lot whose remaining quantity the ledger does
				// not back.
				execRaw(t, database, `
					INSERT INTO commodities (id, book_id, code, kind, is_builtin, created_at, created_by_user_id)
					VALUES (2, 1, 'IWDA', 'security', 0, '2026-01-01T00:00:00Z', 1);
					INSERT INTO commodity_versions (
						commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason,
						status, symbol, display_symbol, name, standard_scale, max_quantity_scale
					) VALUES (2, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'seed', 'active', 'IWDA', 'IWDA', 'World ETF', 4, 8);
					INSERT INTO accounts (id, book_id, created_at, created_by_user_id)
					VALUES (3, 1, '2026-01-01T00:00:00Z', 1);
					INSERT INTO account_versions (
						account_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason,
						status, opened_on, name, account_class, account_kind, allows_postings
					) VALUES (3, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'seed', 'active', '2026-01-01', 'Holdings', 'asset', 'security_holding', 1);
					INSERT INTO investment_lots (
						id, book_id, account_id, commodity_id, opened_on, status,
						quantity_value, quantity_scale, remaining_quantity_value, remaining_quantity_scale,
						cost_basis_value, cost_basis_scale, remaining_cost_basis_value, remaining_cost_basis_scale,
						cost_commodity_id, created_at, created_by_user_id, updated_at, updated_by_user_id
					) VALUES (
						1, 1, 3, 2, '2026-06-01', 'open',
						'100000', 4, '100000', 4, 100000, 2, 100000, 2, 1,
						'2026-06-01T00:00:00Z', 1, '2026-06-01T00:00:00Z', 1
					);
				`)
			},
		},
		"a checkpoint that no longer sums to its statement": {
			checkID: CheckCheckpointIntegrity,
			corrupt: func(t *testing.T, database *sql.DB) {
				execRaw(t, database, `
					INSERT INTO reconciliation_checkpoints (
						id, book_id, account_id, commodity_id, status, statement_date,
						statement_balance_value, statement_balance_scale, created_at, created_by_user_id
					) VALUES (1, 1, 1, 1, 'active', '2026-06-30', '999999', 2, '2026-06-30T00:00:00Z', 1);
					INSERT INTO reconciliation_checkpoint_postings (
						checkpoint_id, posting_version_id, book_id, account_id, commodity_id,
						entry_date, quantity_value, quantity_scale
					) SELECT 1, id, 1, 1, 1, '2026-06-01', quantity_value, quantity_scale
					  FROM posting_versions WHERE account_id = 1;
				`)
			},
		},
		"a posting predating every version of its account": {
			checkID: CheckAccountVersionCoverage,
			corrupt: func(t *testing.T, database *sql.DB) {
				// Two things stand in the way of this row: the service layer
				// (T-63) and posting_versions_account_version_valid in the
				// schema. Both are working as intended, which is why the check
				// reports zero on every healthy book — and why a non-zero count
				// means the rows arrived from somewhere else entirely.
				execRaw(t, database, `DROP TRIGGER posting_versions_account_version_valid`)
				execRaw(t, database, `
					INSERT INTO accounts (id, book_id, created_at, created_by_user_id)
					VALUES (4, 1, '2026-01-01T00:00:00Z', 1);
					INSERT INTO account_versions (
						account_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason,
						status, opened_on, name, account_class, account_kind, allows_postings
					) VALUES (4, 1, '2026-09-01', '2026-09-01T00:00:00Z', 1, 'backfill', 'active', '2026-09-01', 'Late Account', 'asset', 'checking', 1);
					INSERT INTO posting_lines (id, book_id, transaction_id, line_key, created_at, created_by_user_id)
					VALUES (4, 1, 1, 'd', '2026-06-01T00:00:00Z', 1);
					INSERT INTO posting_versions (
						book_id, transaction_version_id, journal_entry_id, posting_line_id, line_seq,
						account_id, quantity_value, quantity_scale, commodity_id, reconciliation_status
					) VALUES (1, 1, 1, 4, 4, 4, '0', 2, 1, 'uncleared');
				`)
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			harness := newSelfCheckHarness(t)
			require.Equal(t, SelfCheckPassed, harness.run(t).Status, "the fixture must start healthy")

			testCase.corrupt(t, harness.writer)

			run := harness.run(t)
			assert.Equal(t, SelfCheckFailed, run.Status)

			result := resultFor(t, run, testCase.checkID)
			assert.Equalf(t, SelfCheckFailed, result.Status, "%s should have caught this", testCase.checkID)
			assert.Positive(t, result.FindingCount)
			assert.NotEmpty(t, result.Summary)
			assert.NotEmpty(t, result.NextStep, "a failure a reader cannot act on is only half reported")
		})
	}
}

// The check reports; it never repairs. A run against a broken book must leave
// the book exactly as broken as it found it.
func TestSelfCheckWritesNothingToLedgerTables(t *testing.T) {
	t.Parallel()

	harness := newSelfCheckHarness(t)
	ctx := context.Background()

	// Append-only triggers mean a backfill inserts rather than edits, so this
	// is what a broken book actually looks like: a one-sided entry.
	execRaw(t, harness.writer, `
		INSERT INTO posting_lines (id, book_id, transaction_id, line_key, created_at, created_by_user_id)
		VALUES (3, 1, 1, 'c', '2026-06-01T00:00:00Z', 1);
		INSERT INTO posting_versions (
			book_id, transaction_version_id, journal_entry_id, posting_line_id, line_seq,
			account_id, quantity_value, quantity_scale, commodity_id, reconciliation_status
		) VALUES (1, 1, 1, 3, 3, 1, '5000', 2, 1, 'uncleared');
	`)

	before := ledgerSnapshot(t, harness.writer)
	run := harness.run(t)
	require.Equal(t, SelfCheckFailed, run.Status)
	after := ledgerSnapshot(t, harness.writer)

	assert.Equal(t, before, after, "the self-check must not touch a single ledger row, least of all to make itself pass")

	// What it does write: its own record, and only that.
	var runs, results int64
	require.NoError(t, harness.writer.QueryRowContext(ctx, "SELECT COUNT(*) FROM self_check_runs").Scan(&runs))
	require.NoError(t, harness.writer.QueryRowContext(ctx, "SELECT COUNT(*) FROM self_check_results").Scan(&results))
	assert.Equal(t, int64(1), runs)
	assert.Positive(t, results)
}

// The counter behind slice 1's export fallback: it exists to stop the fallback
// covering for corrupt data in silence.
func TestSelfCheckReportsPostingsWithNoEffectiveAccountVersion(t *testing.T) {
	t.Parallel()

	harness := newSelfCheckHarness(t)

	passing := resultFor(t, harness.run(t), CheckAccountVersionCoverage)
	assert.Equal(t, SelfCheckPassed, passing.Status)
	assert.Zero(t, passing.FindingCount, "a healthy book reports zero, which is what makes a non-zero count meaningful")

	execRawCoverage(t, harness.writer)

	failing := resultFor(t, harness.run(t), CheckAccountVersionCoverage)
	assert.Equal(t, SelfCheckFailed, failing.Status)
	assert.Equal(t, int64(1), failing.FindingCount)
	assert.NotEmpty(t, failing.Sample, "a reader needs to know which posting")
	assert.Contains(t, failing.NextStep, "outside it",
		"the next step must say that these rows did not come from the app")
}

// A run is stored so a screen can show the last answer without making anyone
// wait for a new one.
func TestLatestSelfCheckReturnsTheStoredRun(t *testing.T) {
	t.Parallel()

	harness := newSelfCheckHarness(t)
	ctx := context.Background()

	_, hasRun, err := harness.service.LatestSelfCheck(ctx)
	require.NoError(t, err)
	assert.False(t, hasRun, "never checked is an answer, not an error")

	executed := harness.run(t)
	latest, hasRun, err := harness.service.LatestSelfCheck(ctx)
	require.NoError(t, err)
	require.True(t, hasRun)

	assert.Equal(t, executed.ID, latest.ID)
	assert.Equal(t, SelfCheckPassed, latest.Status)
	assert.Len(t, latest.Results, len(executed.Results))
	for _, result := range latest.Results {
		assert.NotEmptyf(t, result.Explanation, "%s lost its explanation on the way back out", result.CheckID)
	}
}

// ledgerSnapshot is every row the check reads, so a comparison catches any
// write it might have made.
func ledgerSnapshot(t *testing.T, database *sql.DB) map[string]string {
	t.Helper()

	ctx := context.Background()
	snapshot := map[string]string{}
	for _, table := range []string{
		"transactions", "transaction_versions", "journal_entries", "posting_versions",
		"posting_lines", "accounts", "account_versions", "investment_lots",
		"reconciliation_checkpoints", "reconciliation_checkpoint_postings",
	} {
		var fingerprint sql.NullString
		err := database.QueryRowContext(ctx,
			`SELECT group_concat(quoted, '|') FROM (SELECT quote(t.*) AS quoted FROM `+table+` t ORDER BY rowid)`).Scan(&fingerprint)
		if err != nil {
			// A table without a rowid or a quotable form is compared by count
			// instead; the point is that nothing changed, not how it is spelled.
			var count int64
			require.NoError(t, database.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count))
			snapshot[table] = "count:" + string(rune(count))
			continue
		}
		snapshot[table] = fingerprint.String
	}
	return snapshot
}

// execRawCoverage inserts the one shape that produces an account-version gap:
// an account whose first version begins after a posting it is given. The write
// path refuses this and account_versions is append-only, so only something
// outside the app can produce it — which is the whole point of the check.
func execRawCoverage(t *testing.T, database *sql.DB) {
	t.Helper()

	execRaw(t, database, `DROP TRIGGER posting_versions_account_version_valid`)
	execRaw(t, database, `
		INSERT INTO accounts (id, book_id, created_at, created_by_user_id)
		VALUES (4, 1, '2026-01-01T00:00:00Z', 1);
		INSERT INTO account_versions (
			account_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason,
			status, opened_on, name, account_class, account_kind, allows_postings
		) VALUES (4, 1, '2026-09-01', '2026-09-01T00:00:00Z', 1, 'backfill', 'active', '2026-09-01', 'Late Account', 'asset', 'checking', 1);
		INSERT INTO posting_lines (id, book_id, transaction_id, line_key, created_at, created_by_user_id)
		VALUES (4, 1, 1, 'd', '2026-06-01T00:00:00Z', 1);
		INSERT INTO posting_versions (
			book_id, transaction_version_id, journal_entry_id, posting_line_id, line_seq,
			account_id, quantity_value, quantity_scale, commodity_id, reconciliation_status
		) VALUES (1, 1, 1, 4, 4, 4, '0', 2, 1, 'uncleared');
	`)
}

func execRaw(t *testing.T, database *sql.DB, statements string) {
	t.Helper()

	_, err := database.ExecContext(context.Background(), statements)
	require.NoError(t, err)
}
