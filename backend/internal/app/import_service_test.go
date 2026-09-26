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

func TestApplyImportRules_DescriptionMatchAndTransferSafety(t *testing.T) {
	categoryID := int64(41)
	rules := []db.ImportRuleRecord{{
		ID: 7, Name: "Rail travel", MatchField: "description", ContainsText: "train",
		CategoryID: sql.NullInt64{Int64: categoryID, Valid: true}, TagIDs: []int64{9},
	}}

	matched := applyImportRules(StagedRow{Memo: "Night TRAIN to Berlin"}, rules)
	require.NotNil(t, matched.CategoryID)
	assert.Equal(t, categoryID, *matched.CategoryID)
	assert.Equal(t, []int64{9}, matched.TagIDs)
	assert.Equal(t, "Rail travel", matched.AppliedRuleName)

	transfer := applyImportRules(StagedRow{Memo: "Train transfer", TransferHint: "Savings"}, rules)
	assert.Nil(t, transfer.CategoryID, "a rule must not turn a transfer into spending")
	assert.Equal(t, []int64{9}, transfer.TagIDs)
	assert.Equal(t, "Rail travel", transfer.AppliedRuleName)
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
	staged, err := f.importRepo.ListAllImportStagedRows(ctx, result.Batch.ID)
	require.NoError(t, err)
	require.Len(t, staged, 1)
	require.True(t, staged[0].CommittedIdentityID.Valid)
	require.Len(t, staged[0].CommitEffects, 1)
	assert.Equal(t, staged[0].CommittedTransactionID.Int64, staged[0].CommitEffects[0].TransactionID.Int64)
}

func TestImportIdentityOrdersTwoTransactionsAndRollsBackPartialEffects(t *testing.T) {
	f := newPlainImportTestFixture(t)
	ctx := context.Background()
	batchID, _ := f.stageQIFRow(t, "!Type:Bank\nD2026-08-28\nT-12.34\nPBooks\n^\n")
	staged, err := f.importRepo.ListAllImportStagedRows(ctx, batchID)
	require.NoError(t, err)
	require.Len(t, staged, 1)
	resolution, err := parseResolutionJSON(staged[0].ResolutionJSON)
	require.NoError(t, err)
	spec, err := buildTransactionSpec(staged[0], resolution)
	require.NoError(t, err)
	prepared, err := f.importService.transactionService.prepareCreateTransactionForWrite(ctx, CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "import", Operation: "transaction.create",
		Spec: spec, ChangeReason: "ordered import effects",
	})
	require.NoError(t, err)
	params := db.CommitImportedTransactionParams{
		Identity: db.CreateImportCommitIdentityParams{
			BookID: BookID, DedupeFingerprint: staged[0].DedupeFingerprint,
			SourceKind: "qif", AccountID: f.checkingAccountID, CreatedAt: "2026-08-28T00:00:00Z",
		},
		Row: db.CommitImportStagedRowParams{RowID: staged[0].ID},
	}
	createTwo := func(tx *sql.Tx) ([]db.CreateImportCommitEffectParams, error) {
		var effects []db.CreateImportCommitEffectParams
		for range 2 {
			record, err := f.importService.transactionService.createTransactionRecordInTx(ctx, tx, prepared)
			if err != nil {
				return nil, err
			}
			effects = append(effects, db.CreateImportCommitEffectParams{
				TransactionID: sql.NullInt64{Int64: record.ID, Valid: true},
			})
		}
		return effects, nil
	}
	effects, err := f.importRepo.CommitImportedEffects(ctx, params, createTwo)
	require.NoError(t, err)
	require.Len(t, effects, 2)
	assert.NotEqual(t, effects[0].TransactionID.Int64, effects[1].TransactionID.Int64)
	assert.Equal(t, 2, plainImportTransactionCount(t, f))
	staged, err = f.importRepo.ListAllImportStagedRows(ctx, batchID)
	require.NoError(t, err)
	require.True(t, staged[0].CommittedIdentityID.Valid)
	require.Len(t, staged[0].CommitEffects, 2)
	assert.Equal(t, int64(1), staged[0].CommitEffects[0].EffectSeq)
	assert.Equal(t, int64(2), staged[0].CommitEffects[1].EffectSeq)
	assert.Equal(t, effects[0].TransactionID.Int64, staged[0].CommittedTransactionID.Int64)

	// The book-wide fingerprint blocks a different source kind too; both
	// proposed transactions must roll back with the conflicting identity.
	params.Identity.SourceKind = "csv"
	_, err = f.importRepo.CommitImportedEffects(ctx, params, createTwo)
	require.ErrorIs(t, err, db.ErrCommitIdentityConflict)
	assert.Equal(t, 2, plainImportTransactionCount(t, f))
	var identityCount int
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT count(*) FROM import_commit_identities WHERE book_id = ? AND dedupe_fingerprint = ?`, BookID, params.Identity.DedupeFingerprint).Scan(&identityCount))
	assert.Equal(t, 1, identityCount)

	secondBatchID, _ := f.stageQIFRow(t, "!Type:Bank\nD2026-08-29\nT-17.00\nPTravel\n^\n")
	secondRows, err := f.importRepo.ListAllImportStagedRows(ctx, secondBatchID)
	require.NoError(t, err)
	require.Len(t, secondRows, 1)
	secondResolution, err := parseResolutionJSON(secondRows[0].ResolutionJSON)
	require.NoError(t, err)
	secondSpec, err := buildTransactionSpec(secondRows[0], secondResolution)
	require.NoError(t, err)
	secondPrepared, err := f.importService.transactionService.prepareCreateTransactionForWrite(ctx, CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "import", Operation: "transaction.create",
		Spec: secondSpec, ChangeReason: "partial effect failure",
	})
	require.NoError(t, err)
	params.Identity.DedupeFingerprint = secondRows[0].DedupeFingerprint
	params.Row.RowID = secondRows[0].ID
	_, err = f.importRepo.CommitImportedEffects(ctx, params, func(tx *sql.Tx) ([]db.CreateImportCommitEffectParams, error) {
		record, err := f.importService.transactionService.createTransactionRecordInTx(ctx, tx, secondPrepared)
		if err != nil {
			return nil, err
		}
		return []db.CreateImportCommitEffectParams{
			{TransactionID: sql.NullInt64{Int64: record.ID, Valid: true}},
			{TransactionID: sql.NullInt64{Int64: 999999, Valid: true}},
		}, nil
	})
	require.Error(t, err)
	assert.Equal(t, 2, plainImportTransactionCount(t, f), "first child must roll back when second is invalid")
	_, found, err := f.importRepo.FindCommitIdentity(ctx, BookID, secondRows[0].DedupeFingerprint)
	require.NoError(t, err)
	assert.False(t, found)
	secondRows, err = f.importRepo.ListAllImportStagedRows(ctx, secondBatchID)
	require.NoError(t, err)
	assert.Equal(t, "pending", secondRows[0].CommitStatus)
	assert.Empty(t, secondRows[0].CommitEffects)
}

func TestImportIdentityCanRecordBasisOnlyOperationWithoutJournalTransaction(t *testing.T) {
	f := newPlainImportTestFixture(t)
	ctx := context.Background()
	batchID, _ := f.stageQIFRow(t, "!Type:Bank\nD2026-08-28\nT-12.34\nPBasis source\n^\n")
	rows, err := f.importRepo.ListAllImportStagedRows(ctx, batchID)
	require.NoError(t, err)
	params := db.CommitImportedTransactionParams{
		Identity: db.CreateImportCommitIdentityParams{
			BookID: BookID, DedupeFingerprint: rows[0].DedupeFingerprint,
			SourceKind: "qif", AccountID: f.checkingAccountID, CreatedAt: "2026-08-28T00:00:00Z",
		},
		Row: db.CommitImportStagedRowParams{RowID: rows[0].ID},
	}
	_, err = f.importRepo.CommitImportedEffects(ctx, params, func(tx *sql.Tx) ([]db.CreateImportCommitEffectParams, error) {
		audit, err := tx.ExecContext(ctx, `
			INSERT INTO audit_events (book_id, actor_user_id, occurred_at, origin_type, operation)
			VALUES (?, ?, '2026-08-28T00:00:00Z', 'import', 'investment.basis_adjust')
		`, BookID, f.ownerUserID)
		if err != nil {
			return nil, err
		}
		auditID, err := audit.LastInsertId()
		if err != nil {
			return nil, err
		}
		operation, err := tx.ExecContext(ctx, `
			INSERT INTO investment_operations (book_id, operation_kind, event_date, created_at, created_audit_event_id)
			VALUES (?, 'basis_adjust', '2026-08-28', '2026-08-28T00:00:00Z', ?)
		`, BookID, auditID)
		if err != nil {
			return nil, err
		}
		operationID, err := operation.LastInsertId()
		if err != nil {
			return nil, err
		}
		return []db.CreateImportCommitEffectParams{{OperationID: sql.NullInt64{Int64: operationID, Valid: true}}}, nil
	})
	require.NoError(t, err)
	rows, err = f.importRepo.ListAllImportStagedRows(ctx, batchID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "committed", rows[0].CommitStatus)
	assert.False(t, rows[0].CommittedTransactionID.Valid)
	require.Len(t, rows[0].CommitEffects, 1)
	assert.True(t, rows[0].CommitEffects[0].OperationID.Valid)
	assert.False(t, rows[0].CommitEffects[0].TransactionID.Valid)
	identity, found, err := f.importRepo.FindCommitIdentity(ctx, BookID, rows[0].DedupeFingerprint)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Zero(t, identity.CommittedTransactionID)
}

func TestImportProfileUpdateAndDeletePreserveHistoricalBatch(t *testing.T) {
	f := newPlainImportTestFixture(t)
	ctx := context.Background()
	profile, err := f.importService.CreateImportProfile(ctx, CreateImportProfileInput{
		OwnerUserID: f.ownerUserID,
		Name:        "Old name",
		AdapterKind: "csv",
		ConfigJSON:  `{"delimiter":"comma","date_column":"Date","amount_column":"Amount","date_layout":"YMD","decimal_separator":"."}`,
	})
	require.NoError(t, err)
	result, err := f.importService.StartImport(ctx, StartImportInput{
		OwnerUserID: f.ownerUserID,
		ProfileID:   &profile.ID,
		Input:       RawInput{Filename: "bank.csv", Bytes: []byte("Date,Amount\n2026-08-28,-12.34\n")},
	})
	require.NoError(t, err)

	newName := "Current bank"
	newConfig := `{"delimiter":"comma","date_column":"Booked","amount_column":"Value","date_layout":"YMD","decimal_separator":".","source_filename":"bank.csv","headers":["Booked","Value"]}`
	updated, err := f.importService.UpdateImportProfile(ctx, UpdateImportProfileInput{
		OwnerUserID: f.ownerUserID,
		ProfileID:   profile.ID,
		Name:        &newName,
		ConfigJSON:  &newConfig,
	})
	require.NoError(t, err)
	assert.Equal(t, newName, updated.Name)
	assert.Equal(t, newConfig, updated.ConfigJSON)

	require.NoError(t, f.importService.DeleteImportProfile(ctx, DeleteImportProfileInput{OwnerUserID: f.ownerUserID, ProfileID: profile.ID}))
	_, err = f.importService.UpdateImportProfile(ctx, UpdateImportProfileInput{OwnerUserID: f.ownerUserID, ProfileID: profile.ID, Name: &newName})
	assert.ErrorIs(t, err, ErrImportProfileNotFound)

	batch, err := f.importRepo.ImportBatchByID(ctx, BookID, result.Batch.ID)
	require.NoError(t, err)
	assert.False(t, batch.ProfileID.Valid)
	require.Len(t, result.Rows, 1)
	assert.Equal(t, "previewing", batch.Status)
}

func TestUpdateImportProfileOmittedConfigPreservesMapping(t *testing.T) {
	f := newPlainImportTestFixture(t)
	ctx := context.Background()
	originalConfig := `{"delimiter":"semicolon","date_column":"Datum","amount_column":"Bedrag","date_layout":"DMY","decimal_separator":","}`
	profile, err := f.importService.CreateImportProfile(ctx, CreateImportProfileInput{OwnerUserID: f.ownerUserID, Name: "Bank", AdapterKind: "csv", ConfigJSON: originalConfig})
	require.NoError(t, err)
	newName := "Renamed bank"
	updated, err := f.importService.UpdateImportProfile(ctx, UpdateImportProfileInput{OwnerUserID: f.ownerUserID, ProfileID: profile.ID, Name: &newName})
	require.NoError(t, err)
	assert.Equal(t, originalConfig, updated.ConfigJSON)
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
