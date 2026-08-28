package app

import (
	"context"
	"database/sql"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
)

// plainImportTestFixture wires a full ImportService against one in-memory
// database for the ordinary (non-investment) QIF commit path — the
// counterpart to investTestFixture in import_trading212_invest_test.go, used
// where the interactive HTTP layer can't cleanly drive a race window.
type plainImportTestFixture struct {
	importService     *ImportService
	importRepo        *db.ImportRepository
	database          *sql.DB
	eurCommodityID    int64
	checkingAccountID int64
	categoryAccountID int64
	ownerUserID       int64
}

func newPlainImportTestFixture(t *testing.T) *plainImportTestFixture {
	t.Helper()
	database := openConnectionTestDatabase(t)
	ctx := context.Background()

	res, err := database.ExecContext(ctx, `
		INSERT INTO commodities (book_id, code, kind, is_builtin, created_at, created_by_user_id)
		VALUES (?, 'EUR', 'currency', 1, '2026-01-01T00:00:00Z', 1)
	`, BookID)
	require.NoError(t, err)
	eurCommodityID, err := res.LastInsertId()
	require.NoError(t, err)
	_, err = database.ExecContext(ctx, `
		INSERT INTO commodity_versions (
			commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id,
			change_reason, status, symbol, display_symbol, name, standard_scale, max_quantity_scale
		) VALUES (?, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'test fixture', 'active', 'EUR', '€', 'Euro', 2, 6)
	`, eurCommodityID)
	require.NoError(t, err)

	checkingAccountID := seedTestAccount(t, database, "active", true)
	categoryAccountID := seedTestAccountWithClass(t, database, "active", true, "expense", "expense")

	accountRepository := db.NewAccountRepository(database)
	transactionService := NewTransactionService(db.NewTransactionRepository(database), db.NewPayeeRepository(database), accountRepository, db.NewCommodityRepository(database))
	importRepo := db.NewImportRepository(database)
	importService := NewImportService(importRepo, transactionService, accountRepository, nil, nil, nil)

	return &plainImportTestFixture{
		importService:     importService,
		importRepo:        importRepo,
		database:          database,
		eurCommodityID:    eurCommodityID,
		checkingAccountID: checkingAccountID,
		categoryAccountID: categoryAccountID,
		ownerUserID:       1,
	}
}

// stageQIFRow starts a plain QIF import with a single bank row and resolves
// it against the fixture's checking/category accounts, returning the batch
// and row IDs ready to commit.
func (f *plainImportTestFixture) stageQIFRow(t *testing.T, qif string) (batchID int64, rowID int64) {
	t.Helper()
	ctx := context.Background()

	result, err := f.importService.StartImport(ctx, StartImportInput{
		OwnerUserID: f.ownerUserID,
		Input:       RawInput{Filename: "test.qif", Bytes: []byte(qif)},
	})
	require.NoError(t, err)
	require.Len(t, result.Rows, 1)

	resolutionJSON := `{"account_id":` + strconv.FormatInt(f.checkingAccountID, 10) +
		`,"commodity_id":` + strconv.FormatInt(f.eurCommodityID, 10) +
		`,"category_id":` + strconv.FormatInt(f.categoryAccountID, 10) + `}`

	err = f.importService.PatchImportBatch(ctx, PatchImportBatchInput{
		OwnerUserID: f.ownerUserID,
		BatchID:     result.Batch.ID,
		RowResolutions: []RowResolutionPatch{{
			RowID:          result.Rows[0].ID,
			DedupeStatus:   "new",
			ResolutionJSON: resolutionJSON,
		}},
	})
	require.NoError(t, err)

	return result.Batch.ID, result.Rows[0].ID
}

func plainImportTransactionCount(t *testing.T, f *plainImportTestFixture) int {
	t.Helper()
	var count int
	require.NoError(t, f.database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM transactions`).Scan(&count))
	return count
}

func TestCSVImportSavedProfileStagesAndCommitsThroughRealLedgerService(t *testing.T) {
	f := newPlainImportTestFixture(t)
	ctx := context.Background()
	profile, err := f.importService.CreateImportProfile(ctx, CreateImportProfileInput{
		OwnerUserID: f.ownerUserID,
		Name:        "EU bank",
		AdapterKind: "csv",
		ConfigJSON:  `{"delimiter":"semicolon","date_column":"Datum","payee_column":"Omschrijving","amount_column":"Bedrag","date_layout":"DMY","decimal_separator":","}`,
	})
	require.NoError(t, err)

	result, err := f.importService.StartImport(ctx, StartImportInput{
		OwnerUserID: f.ownerUserID,
		ProfileID:   &profile.ID,
		Input:       RawInput{Filename: "bank.csv", Bytes: []byte("Datum;Omschrijving;Bedrag\n28/08/2026;Bakker;-12,34\n")},
	})
	require.NoError(t, err)
	require.Len(t, result.Rows, 1)
	assert.Equal(t, "csv", result.Batch.SourceKind)
	assert.Equal(t, profile.ID, *result.Batch.ProfileID)

	resolutionJSON := `{"account_id":` + strconv.FormatInt(f.checkingAccountID, 10) + `,"commodity_id":` + strconv.FormatInt(f.eurCommodityID, 10) + `,"category_id":` + strconv.FormatInt(f.categoryAccountID, 10) + `}`
	require.NoError(t, f.importService.PatchImportBatch(ctx, PatchImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: result.Batch.ID, RowResolutions: []RowResolutionPatch{{RowID: result.Rows[0].ID, DedupeStatus: "new", ResolutionJSON: resolutionJSON}}}))

	committed, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: result.Batch.ID})
	require.NoError(t, err)
	assert.Equal(t, 1, committed.CommittedCount)
	assert.Equal(t, 1, plainImportTransactionCount(t, f))
}

// TestCommitImportBatch_ConcurrentPlainRowCommitsKeepOneWinner mirrors
// TestCommitImportBatch_ConcurrentTrading212CommitsKeepTheCommittedRow for
// the ordinary (non-investment) commit path: two callers racing to commit
// the same batch must not corrupt state or leave the batch commit itself
// failing with a raw identity-conflict error — exactly one ledger
// transaction must exist, and both callers must observe a successful,
// idempotent outcome.
func TestCommitImportBatch_ConcurrentPlainRowCommitsKeepOneWinner(t *testing.T) {
	f := newPlainImportTestFixture(t)
	batchID, _ := f.stageQIFRow(t, qifBankRow("06/01/26", "-42.50", "Grocery Store"))
	initialTransactions := plainImportTransactionCount(t, f)

	var reachedRowCommit sync.WaitGroup
	reachedRowCommit.Add(2)
	releaseCommits := make(chan struct{})
	f.importService.beforeImportRowCommitForTest = func() {
		reachedRowCommit.Done()
		<-releaseCommits
	}

	results := make(chan CommitImportBatchResult, 2)
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			result, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{
				OwnerUserID: f.ownerUserID,
				BatchID:     batchID,
			})
			results <- result
			errs <- err
		}()
	}

	reachedRowCommit.Wait()
	close(releaseCommits)

	for range 2 {
		require.NoError(t, <-errs)
		result := <-results
		assert.Equal(t, 1, result.CommittedCount)
		assert.Equal(t, "committed", result.Status)
	}

	rows, err := f.importRepo.ListAllImportStagedRows(context.Background(), batchID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "committed", rows[0].CommitStatus)
	assert.True(t, rows[0].CommittedTransactionID.Valid)
	assert.False(t, rows[0].CommitError.Valid)
	assert.Equal(t, initialTransactions+1, plainImportTransactionCount(t, f), "idempotent concurrent commits must create exactly one ledger transaction")

	batch, err := f.importRepo.ImportBatchByID(context.Background(), BookID, batchID)
	require.NoError(t, err)
	assert.Equal(t, "committed", batch.Status)
}

func qifBankRow(date, amount, payee string) string {
	return "!Type:Bank\nD" + date + "\nT" + amount + "\nP" + payee + "\n^\n"
}
