package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// investTestFixture wires a full ImportService (with a real InvestmentService)
// against one in-memory database, for testing the B-T212-INVST commit-path
// branch end to end — the only place in the test suite that needs the whole
// stack (accounts, currencies, transactions, investments, import) wired
// together rather than a single service in isolation.
type investTestFixture struct {
	importService     *ImportService
	connService       *ImportConnectionService
	investmentSvc     *InvestmentService
	importRepo        *db.ImportRepository
	database          *sql.DB
	eurCommodityID    int64
	cashAccountID     int64
	categoryAccountID int64
	tradingAccountID  int64
	ownerUserID       int64
}

func newInvestTestFixture(t *testing.T) *investTestFixture {
	t.Helper()
	return newInvestTestFixtureWithOptions(t, true)
}

// newInvestTestFixtureWithOptions lets a test opt out of seeding the
// commodity_trading system account (seedTradingAccount=false) — used to
// simulate a real configuration gap Buy/Sell requires, since
// account_versions rows are append-only and can't be deleted afterwards.
func newInvestTestFixtureWithOptions(t *testing.T, seedTradingAccount bool) *investTestFixture {
	t.Helper()
	database := openConnectionTestDatabase(t)
	ctx := context.Background()

	// EUR currency commodity, active version.
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

	cashAccountID := seedTestAccount(t, database, "active", true)
	categoryAccountID := seedTestAccount(t, database, "active", true)
	var tradingAccountID int64
	if seedTradingAccount {
		tradingAccountID = seedCommodityTradingAccount(t, database)
	}

	accountRepository := db.NewAccountRepository(database)
	accountService := NewAccountService(accountRepository, db.NewInstitutionRepository(database), nil)
	transactionService := NewTransactionService(db.NewTransactionRepository(database), db.NewPayeeRepository(database), accountRepository, db.NewCommodityRepository(database))
	investmentService := NewInvestmentService(db.NewInvestmentRepository(database), accountService, transactionService, nil)
	connService := NewImportConnectionService(db.NewImportConnectionRepository(database), accountService, testKey(), NoOpProber{})
	importRepo := db.NewImportRepository(database)
	importService := NewImportService(importRepo, transactionService, accountRepository, connService, db.NewBackgroundWorkRepository(database), investmentService)

	return &investTestFixture{
		importService:     importService,
		connService:       connService,
		investmentSvc:     investmentService,
		importRepo:        importRepo,
		database:          database,
		eurCommodityID:    eurCommodityID,
		cashAccountID:     cashAccountID,
		categoryAccountID: categoryAccountID,
		tradingAccountID:  tradingAccountID,
		ownerUserID:       1,
	}
}

// seedCommodityTradingAccount inserts the commodity_trading system account
// Buy/Sell require (InvestmentRepository.CommodityTradingAccountID).
// system_role must be set at INSERT time — a trigger enforces that account
// system-identity fields are immutable after creation, so seedTestAccount's
// plain account + a follow-up UPDATE doesn't work here.
func seedCommodityTradingAccount(t *testing.T, database *sql.DB) int64 {
	t.Helper()
	ctx := context.Background()
	result, err := database.ExecContext(ctx, `
		INSERT INTO accounts (book_id, system_role, created_at, created_by_user_id)
		VALUES (?, 'commodity_trading', '2026-01-01T00:00:00Z', 1)
	`, BookID)
	require.NoError(t, err)
	accountID, err := result.LastInsertId()
	require.NoError(t, err)
	_, err = database.ExecContext(ctx, `
		INSERT INTO account_versions (
			account_id, version_seq, effective_from, recorded_at, changed_by_user_id,
			change_reason, status, opened_on, code, name, account_class,
			account_kind, allows_postings
		) VALUES (?, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'test fixture', 'active', '2026-01-01', 'TRADING', 'Commodity Trading', 'asset', 'other_asset', 1)
	`, accountID)
	require.NoError(t, err)
	return accountID
}

func (f *investTestFixture) createConnection(t *testing.T, cashAccountID *int64) ImportConnection {
	t.Helper()
	conn, err := f.connService.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: f.ownerUserID, SourceKind: "trading212", DisplayName: "ISA", APIKey: "api-key-xxxx",
		CashAccountID: cashAccountID,
	})
	require.NoError(t, err)
	return conn
}

// stageOrderFillRow creates a batch and stages one order-fill row directly
// (bypassing the HTTP fetch — these tests only care about
// CommitImportBatch's routing, not the fetcher).
func (f *investTestFixture) stageOrderFillRow(t *testing.T, connectionID int64, o trading212OrderFill) (batchID int64, rowID int64) {
	t.Helper()
	ctx := context.Background()
	batch, err := f.importRepo.CreateImportBatch(ctx, db.CreateImportBatchParams{
		BookID: BookID, SourceKind: "trading212",
		ConnectionID:   sql.NullInt64{Int64: connectionID, Valid: true},
		SourceMetaJSON: `{"fetch_status":"ready"}`, CreatedAt: time.Now().UTC().Format(time.RFC3339), ActorUserID: f.ownerUserID,
	})
	require.NoError(t, err)

	adapter := &Trading212Adapter{}
	payload := trading212FetchPayload{ConnectionID: connectionID, OrderFills: []trading212OrderFill{o}}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)
	parseResult, err := adapter.Parse(ctx, RawInput{Bytes: payloadJSON}, nil)
	require.NoError(t, err)

	rows, _, err := f.importService.stageParseResult(ctx, batch.ID, parseResult)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	return batch.ID, rows[0].ID
}

func (f *investTestFixture) stageDividendRow(t *testing.T, connectionID int64, d trading212Dividend) (batchID int64, rowID int64) {
	t.Helper()
	ctx := context.Background()
	batch, err := f.importRepo.CreateImportBatch(ctx, db.CreateImportBatchParams{
		BookID: BookID, SourceKind: "trading212",
		ConnectionID:   sql.NullInt64{Int64: connectionID, Valid: true},
		SourceMetaJSON: `{"fetch_status":"ready"}`, CreatedAt: time.Now().UTC().Format(time.RFC3339), ActorUserID: f.ownerUserID,
	})
	require.NoError(t, err)

	adapter := &Trading212Adapter{}
	payload := trading212FetchPayload{ConnectionID: connectionID, Dividends: []trading212Dividend{d}}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)
	parseResult, err := adapter.Parse(ctx, RawInput{Bytes: payloadJSON}, nil)
	require.NoError(t, err)

	rows, _, err := f.importService.stageParseResult(ctx, batch.ID, parseResult)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	return batch.ID, rows[0].ID
}

// resolveRowGeneric sets a plain account/category resolution on a staged
// row, the way the normal preview UI's per-row picker would — used to prove
// the generic fallback path still works for a row investment routing didn't
// (or couldn't) claim.
func (f *investTestFixture) resolveRowGeneric(t *testing.T, batchID int64, rowID int64, accountID int64) {
	t.Helper()
	resolutionJSON, err := json.Marshal(ImportRowResolution{
		AccountID:   accountID,
		CommodityID: f.eurCommodityID,
		CategoryID:  &f.categoryAccountID,
	})
	require.NoError(t, err)
	require.NoError(t, f.importRepo.UpdateImportStagedRowResolution(context.Background(), db.UpdateImportStagedRowResolutionParams{
		RowID:          rowID,
		BatchID:        batchID,
		DedupeStatus:   "new",
		ResolutionJSON: string(resolutionJSON),
	}))
}

func (f *investTestFixture) createCashReconciliationCheckpoint(t *testing.T, statementDate string) int64 {
	t.Helper()
	ctx := context.Background()
	transaction, err := f.importService.transactionService.CreateTransaction(ctx, CreateTransactionInput{
		OwnerUserID:  f.ownerUserID,
		OriginType:   "browser_api",
		Operation:    "transaction.create",
		ChangeReason: "test fixture",
		Spec: TransactionInput{
			Status:          "posted",
			TransactionKind: "ordinary",
			TransactionDate: statementDate,
			JournalEntries: []JournalEntryInput{{
				EntryDate: statementDate,
				EntryKind: "ordinary",
				Postings: []PostingInput{
					{AccountID: f.cashAccountID, QuantityValue: exact.New(10000), QuantityScale: 2, CommodityID: f.eurCommodityID},
					{AccountID: f.categoryAccountID, QuantityValue: exact.New(-10000), QuantityScale: 2, CommodityID: f.eurCommodityID},
				},
			}},
		},
	})
	require.NoError(t, err)

	var cashPostingID int64
	for _, posting := range transaction.JournalEntries[0].Postings {
		if posting.AccountID == f.cashAccountID {
			cashPostingID = posting.ID
			break
		}
	}
	require.NotZero(t, cashPostingID)

	session, err := f.importService.transactionService.StartReconciliation(ctx, StartReconciliationInput{
		OwnerUserID:           f.ownerUserID,
		OriginType:            "browser_api",
		AccountID:             f.cashAccountID,
		CommodityID:           f.eurCommodityID,
		SourceKind:            "statement",
		StatementDate:         statementDate,
		StatementBalanceValue: exact.New(10000),
		StatementBalanceScale: 2,
		ChangeReason:          "test fixture",
	})
	require.NoError(t, err)
	session, err = f.importService.transactionService.UpdateReconciliationSelection(ctx, ReconciliationSelectionInput{
		OwnerUserID:       f.ownerUserID,
		OriginType:        "browser_api",
		SessionID:         session.ID,
		PostingVersionIDs: []int64{cashPostingID},
		ChangeReason:      "selected test posting",
	})
	require.NoError(t, err)
	_, err = f.importService.transactionService.FinishReconciliation(ctx, FinishReconciliationInput{
		OwnerUserID:  f.ownerUserID,
		OriginType:   "browser_api",
		SessionID:    session.ID,
		ChangeReason: "matched test statement",
	})
	require.NoError(t, err)

	checkpoints, err := f.importService.transactionService.ReconciliationCheckpoints(ctx, ListReconciliationCheckpointsInput{
		AccountID:   f.cashAccountID,
		CommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	require.Len(t, checkpoints, 1)
	require.Equal(t, "active", checkpoints[0].Status)
	return checkpoints[0].ID
}

func TestCommitImportBatch_BuyOrderFillCreatesInstrumentHoldingAndLot(t *testing.T) {
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)

	batchID, _ := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
		FillID: "fill-1", OrderID: "order-1", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "BUY", Quantity: "2", Price: "150.25", Currency: "USD",
		FilledAt: "2026-06-01T10:00:00Z", NetValue: "-300.75", NetValueCurrency: "EUR",
	})

	result, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	assert.Equal(t, 1, result.CommittedCount, "the buy should route through the investment path and commit")
	assert.Equal(t, "committed", result.Status)

	positions, err := f.investmentSvc.Positions(context.Background())
	require.NoError(t, err)
	require.Len(t, positions, 1, "a lot should now exist for AAPL")
	assert.Equal(t, "2", positions[0].QuantityValue.String())

	instruments, err := f.investmentSvc.ListInstruments(context.Background())
	require.NoError(t, err)
	require.Len(t, instruments, 1)
	assert.Equal(t, "AAPL_US_EQ", instruments[0].Symbol)

	rows, err := f.importRepo.ListAllImportStagedRows(context.Background(), batchID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.True(t, rows[0].CommittedTransactionID.Valid)
	transaction, err := f.importService.transactionService.Transaction(context.Background(), rows[0].CommittedTransactionID.Int64)
	require.NoError(t, err)
	assert.Equal(t, "fill-1", transaction.ExternalRefHint)
	assert.JSONEq(t, `{"source_kind":"trading212","connection_id":1,"provider_kind":"order_fill","provider_id":"fill-1"}`, transaction.MetadataJSON)

	holdingAccountID, found, err := f.connService.HoldingAccountForCommodity(context.Background(), conn.ID, instruments[0].CommodityID)
	require.NoError(t, err)
	require.True(t, found)
	lots, err := f.investmentSvc.ListLots(context.Background(), holdingAccountID, instruments[0].CommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	assert.JSONEq(t, transaction.MetadataJSON, lots[0].MetadataJSON)
}

func TestCommitImportBatch_InvestmentBuyCrossingReconciledPeriodRequiresOverride(t *testing.T) {
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	checkpointID := f.createCashReconciliationCheckpoint(t, "2026-06-10")

	batchID, _ := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
		FillID: "fill-reconciled-1", OrderID: "order-reconciled-1", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "BUY", Quantity: "2", Price: "150.25", Currency: "USD",
		FilledAt: "2026-06-01T10:00:00Z", NetValue: "-300.75", NetValueCurrency: "EUR",
	})

	result, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	assert.Equal(t, 0, result.CommittedCount)
	assert.Equal(t, 1, result.SkippedCount, "investment imports should use the same reconciled-period guard as generic imports")

	rows, err := f.importRepo.ListAllImportStagedRows(context.Background(), batchID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "skipped", rows[0].CommitStatus)
	require.True(t, rows[0].CommitError.Valid)
	assert.Equal(t, "reconciliation override required", rows[0].CommitError.String)

	checkpoints, err := f.importService.transactionService.ReconciliationCheckpoints(context.Background(), ListReconciliationCheckpointsInput{
		AccountID:   f.cashAccountID,
		CommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	require.Len(t, checkpoints, 1)
	assert.Equal(t, checkpointID, checkpoints[0].ID)
	assert.Equal(t, "active", checkpoints[0].Status)

	overrideBatchID, _ := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
		FillID: "fill-reconciled-2", OrderID: "order-reconciled-2", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "BUY", Quantity: "1", Price: "151.00", Currency: "USD",
		FilledAt: "2026-06-02T10:00:00Z", NetValue: "-151.00", NetValueCurrency: "EUR",
	})

	overrideResult, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{
		OwnerUserID:            f.ownerUserID,
		BatchID:                overrideBatchID,
		ReconciliationOverride: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, overrideResult.CommittedCount)
	assert.Equal(t, 0, overrideResult.SkippedCount)

	checkpoints, err = f.importService.transactionService.ReconciliationCheckpoints(context.Background(), ListReconciliationCheckpointsInput{
		AccountID:   f.cashAccountID,
		CommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	require.Len(t, checkpoints, 1)
	assert.Equal(t, checkpointID, checkpoints[0].ID)
	assert.Equal(t, "invalidated", checkpoints[0].Status)
}

func TestCommitImportBatch_BuyOrderFillReusesHoldingAccountAcrossFetches(t *testing.T) {
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)

	fill := func(fillID string, filledAt string) trading212OrderFill {
		return trading212OrderFill{
			FillID: fillID, OrderID: "order-" + fillID, Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
			Side: "BUY", Quantity: "1", Price: "150.00", Currency: "USD",
			FilledAt: filledAt, NetValue: "-150.00", NetValueCurrency: "EUR",
		}
	}

	batch1, _ := f.stageOrderFillRow(t, conn.ID, fill("fill-1", "2026-06-01T10:00:00Z"))
	_, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batch1})
	require.NoError(t, err)

	batch2, _ := f.stageOrderFillRow(t, conn.ID, fill("fill-2", "2026-06-02T10:00:00Z"))
	result2, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batch2})
	require.NoError(t, err)
	assert.Equal(t, 1, result2.CommittedCount)

	// Both fills must have accumulated into the SAME holding account/lot
	// pool, not two separate ones — otherwise cost-basis tracking breaks.
	positions, err := f.investmentSvc.Positions(context.Background())
	require.NoError(t, err)
	require.Len(t, positions, 1, "both fills must land in one position, not two separate holding accounts")
	assert.Equal(t, "2", positions[0].QuantityValue.String())

	instruments, err := f.investmentSvc.ListInstruments(context.Background())
	require.NoError(t, err)
	require.Len(t, instruments, 1, "the instrument must be reused, not recreated")
}

// TestCommitImportBatch_GenericCashRowCommitsToRealLedger is a regression
// test for a real, severe bug found while building the Slice 4b fixtures
// above: buildTransactionSpec (import_service.go) set JournalEntryInput's
// EntryKind to "main", which is not a member of the valid entryKinds set
// (transactions_validate.go) — every single CommitImportBatch call for
// *any* source (QIF file upload, Trading 212 cash movements) failed
// "entry kind is invalid" the moment the row actually reached
// TransactionService.CreateTransaction's real validation. This had never
// been caught because no existing test drove a staged row all the way
// through CommitImportBatch against a real account/ledger — every prior
// test either checked buildTransactionSpec's output in isolation or never
// called CommitImportBatch at all. Fixed: EntryKind is now "ordinary". This
// test does NOT use Trading 212 at all — it stages a plain generic cash
// row (the same shape any import source produces) specifically to prove
// the generic commit path itself, independent of B-T212-INVST.
func TestCommitImportBatch_GenericCashRowCommitsToRealLedger(t *testing.T) {
	f := newInvestTestFixture(t)
	ctx := context.Background()

	batch, err := f.importRepo.CreateImportBatch(ctx, db.CreateImportBatchParams{
		BookID: BookID, SourceKind: "qif", SourceMetaJSON: "{}", CreatedAt: "2026-06-01T00:00:00Z", ActorUserID: f.ownerUserID,
	})
	require.NoError(t, err)

	parseResult := ParseResult{Rows: []StagedRow{{
		DedupeFingerprint: "qif|generic-row-1",
		Date:              "2026-06-01",
		Amount:            "42.50",
		CommodityHint:     "EUR",
		PayeeHint:         "Test Payee",
		Memo:              "test memo",
		ExternalRef:       "ext-1",
	}}}
	rows, _, err := f.importService.stageParseResult(ctx, batch.ID, parseResult)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	f.resolveRowGeneric(t, batch.ID, rows[0].ID, f.cashAccountID)

	result, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batch.ID})
	require.NoError(t, err)
	assert.Equal(t, 1, result.CommittedCount, "a plain generic cash row must commit to the real ledger, not just validate in isolation")
	assert.Equal(t, "committed", result.Status)
}

func TestCommitImportedTransaction_RollsBackLedgerWhenIdentityWriteFails(t *testing.T) {
	f := newInvestTestFixture(t)
	ctx := context.Background()

	batch, err := f.importRepo.CreateImportBatch(ctx, db.CreateImportBatchParams{
		BookID: BookID, SourceKind: "qif", SourceMetaJSON: "{}", CreatedAt: "2026-06-01T00:00:00Z", ActorUserID: f.ownerUserID,
	})
	require.NoError(t, err)

	parseResult := ParseResult{Rows: []StagedRow{{
		DedupeFingerprint: "qif|atomic-row-1",
		Date:              "2026-06-01",
		Amount:            "42.50",
		CommodityHint:     "EUR",
		PayeeHint:         "Atomic Payee",
		Memo:              "atomic rollback probe",
		ExternalRef:       "atomic-ext-1",
	}}}
	rows, _, err := f.importService.stageParseResult(ctx, batch.ID, parseResult)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	f.resolveRowGeneric(t, batch.ID, rows[0].ID, f.cashAccountID)

	rows, err = f.importRepo.ListAllImportStagedRows(ctx, batch.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	resolution, err := parseResolutionJSON(rows[0].ResolutionJSON)
	require.NoError(t, err)
	spec, err := buildTransactionSpec(rows[0], resolution)
	require.NoError(t, err)
	createParams, err := f.importService.transactionService.prepareCreateTransactionForWrite(ctx, CreateTransactionInput{
		OwnerUserID:  f.ownerUserID,
		OriginType:   "import",
		Operation:    "transaction.create",
		Spec:         spec,
		ChangeReason: "imported from qif",
	})
	require.NoError(t, err)

	var beforeTransactions int
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions`).Scan(&beforeTransactions))

	_, err = f.importRepo.CommitImportedTransaction(ctx, db.CommitImportedTransactionParams{
		Identity: db.CreateImportCommitIdentityParams{
			BookID:            BookID,
			DedupeFingerprint: rows[0].DedupeFingerprint,
			SourceKind:        "qif",
			AccountID:         0, // Invalid FK: force failure after createTransactionTx has inserted the ledger rows.
			CreatedAt:         "2026-06-01T00:00:00Z",
		},
		Row: db.CommitImportStagedRowParams{RowID: rows[0].ID},
	}, func(tx *sql.Tx) (int64, error) {
		record, err := f.importService.transactionService.createTransactionRecordInTx(ctx, tx, createParams)
		if err != nil {
			return 0, err
		}
		return record.ID, nil
	})
	require.Error(t, err)

	var afterTransactions int
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions`).Scan(&afterTransactions))
	assert.Equal(t, beforeTransactions, afterTransactions, "ledger transaction must roll back when import identity recording fails")

	var identityCount int
	require.NoError(t, f.database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM import_commit_identities WHERE dedupe_fingerprint = ?
	`, rows[0].DedupeFingerprint).Scan(&identityCount))
	assert.Equal(t, 0, identityCount)

	updatedRows, err := f.importRepo.ListAllImportStagedRows(ctx, batch.ID)
	require.NoError(t, err)
	require.Len(t, updatedRows, 1)
	assert.Equal(t, "pending", updatedRows[0].CommitStatus)
	assert.False(t, updatedRows[0].CommittedTransactionID.Valid)
}

func TestCommitImportBatch_OrderFillWithoutCashAccountFallsBackToGenericCommit(t *testing.T) {
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, nil) // no cash_account_id configured

	batchID, rowID := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
		FillID: "fill-1", OrderID: "order-1", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "BUY", Quantity: "2", Price: "150.25", Currency: "USD",
		FilledAt: "2026-06-01T10:00:00Z", NetValue: "-300.75", NetValueCurrency: "EUR",
	})
	f.resolveRowGeneric(t, batchID, rowID, f.cashAccountID)

	result, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	assert.Equal(t, 1, result.CommittedCount, "without a cash_account_id, the row must still commit via the generic cash path")

	instruments, err := f.investmentSvc.ListInstruments(context.Background())
	require.NoError(t, err)
	assert.Empty(t, instruments, "no instrument should be created when investment routing never engaged")
}

// TestCommitImportBatch_SameDaySellBeforeBuyStillCommitsBothAsInvestmentTrades
// proves that full fill timestamps, not only calendar dates, order a
// newest-first Trading 212 history correctly. With a date-only stable sort,
// the 15:00 SELL below remains before the 09:00 BUY, hits insufficient lots,
// and falls back to a plain cash transaction instead of disposing the lot.
func TestCommitImportBatch_SameDaySellBeforeBuyStillCommitsBothAsInvestmentTrades(t *testing.T) {
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	ctx := context.Background()

	batch, err := f.importRepo.CreateImportBatch(ctx, db.CreateImportBatchParams{
		BookID: BookID, SourceKind: "trading212",
		ConnectionID:   sql.NullInt64{Int64: conn.ID, Valid: true},
		SourceMetaJSON: `{"fetch_status":"ready"}`, CreatedAt: time.Now().UTC().Format(time.RFC3339), ActorUserID: f.ownerUserID,
	})
	require.NoError(t, err)

	adapter := &Trading212Adapter{}
	stage := func(payload trading212FetchPayload) {
		payloadJSON, err := json.Marshal(payload)
		require.NoError(t, err)
		parseResult, err := adapter.Parse(ctx, RawInput{Bytes: payloadJSON}, nil)
		require.NoError(t, err)
		_, _, err = f.importService.stageParseResult(ctx, batch.ID, parseResult)
		require.NoError(t, err)
	}

	// Row index 0: the SELL (newer trade, provider's first page).
	stage(trading212FetchPayload{ConnectionID: conn.ID, OrderFills: []trading212OrderFill{{
		FillID: "fill-sell", OrderID: "order-sell", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "SELL", Quantity: "1", Price: "160.00", Currency: "USD",
		FilledAt: "2026-06-01T15:00:00Z", NetValue: "160.00", NetValueCurrency: "EUR",
	}}})
	// Row index 1: the intraday BUY that opened the lot (later provider page).
	stage(trading212FetchPayload{ConnectionID: conn.ID, OrderFills: []trading212OrderFill{{
		FillID: "fill-buy", OrderID: "order-buy", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "BUY", Quantity: "1", Price: "150.00", Currency: "USD",
		FilledAt: "2026-06-01T09:00:00Z", NetValue: "-150.00", NetValueCurrency: "EUR",
	}}})

	result, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batch.ID})
	require.NoError(t, err)
	assert.Equal(t, 2, result.CommittedCount, "both rows should commit")

	// A fully-closed position (0 remaining) isn't returned by Positions (it
	// only lists open lots), so its absence here plus the closed lot check
	// below together prove the sell actually disposed the buy's lot instead
	// of falling back to a generic cash transaction that leaves the lot open.
	positions, err := f.investmentSvc.Positions(context.Background())
	require.NoError(t, err)
	assert.Empty(t, positions, "buy then sell of 1 share nets to 0 held, so no open position remains")

	instruments, err := f.investmentSvc.ListInstruments(context.Background())
	require.NoError(t, err)
	require.Len(t, instruments, 1, "the instrument must have been resolved for both the buy and the sell")

	holdingAccountID, found, err := f.connService.HoldingAccountForCommodity(context.Background(), conn.ID, instruments[0].CommodityID)
	require.NoError(t, err)
	require.True(t, found, "the buy must have linked a holding account for this commodity")

	lots, err := f.investmentSvc.ListLots(context.Background(), holdingAccountID, instruments[0].CommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1, "the buy must have created exactly one lot")
	assert.Equal(t, "closed", lots[0].Status, "the sell must have disposed the buy's lot — proves it committed as a real investment trade, not a generic cash fallback")

	var disposalMetadata string
	require.NoError(t, f.database.QueryRowContext(ctx, `
		SELECT metadata_json
		FROM investment_lot_events
		WHERE lot_id = ? AND event_kind = 'disposal'
	`, lots[0].ID).Scan(&disposalMetadata))
	assert.JSONEq(t, `{"source_kind":"trading212","connection_id":1,"provider_kind":"order_fill","provider_id":"fill-sell"}`, disposalMetadata)
}

// TestCommitImportBatch_BuyOrderFillUnexpectedInvestmentErrorFailsRowInsteadOfFallingBack
// is a regression test: commitTrading212OrderFill used to treat every error
// from InvestmentService.Buy/Sell as "fall back to the generic cash path",
// including real configuration/ledger bugs like a missing commodity_trading
// system account. That silently hid the bug behind what looked like a
// normal plain cash transaction. Now only the expected data-gap errors
// (insufficient lots, no dividend default) fall back; anything else must
// fail the row with the real error message so the underlying problem is
// visible instead of masked.
func TestCommitImportBatch_BuyOrderFillUnexpectedInvestmentErrorFailsRowInsteadOfFallingBack(t *testing.T) {
	// No commodity_trading system account seeded — Buy() requires one, so
	// this simulates a real configuration bug rather than a normal
	// "can't resolve this row" gap.
	f := newInvestTestFixtureWithOptions(t, false)
	conn := f.createConnection(t, &f.cashAccountID)

	batchID, rowID := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
		FillID: "fill-1", OrderID: "order-1", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "BUY", Quantity: "2", Price: "150.25", Currency: "USD",
		FilledAt: "2026-06-01T10:00:00Z", NetValue: "-300.75", NetValueCurrency: "EUR",
	})
	// Resolve the row generically too, so a silent-fallback regression would
	// still show up as a (wrong) committed cash transaction, not a row that
	// fails for an unrelated reason (missing resolution).
	f.resolveRowGeneric(t, batchID, rowID, f.cashAccountID)

	result, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	assert.Equal(t, 1, result.FailedCount, "the row must fail, not silently commit as a generic cash transaction")
	assert.Equal(t, 0, result.CommittedCount)

	rows, err := f.importRepo.ListAllImportStagedRows(context.Background(), batchID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "failed", rows[0].CommitStatus)
	require.True(t, rows[0].CommitError.Valid)
	assert.Contains(t, rows[0].CommitError.String, "commodity trading system account is required")

	instruments, err := f.investmentSvc.ListInstruments(context.Background())
	require.NoError(t, err)
	assert.Empty(t, instruments, "a failed trade must not leave a newly-created instrument")
	var holdingCount, accountCount int
	require.NoError(t, f.database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM import_connection_holdings`).Scan(&holdingCount))
	require.NoError(t, f.database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM accounts`).Scan(&accountCount))
	assert.Zero(t, holdingCount, "a failed trade must not leave a connection holding map")
	assert.Equal(t, 2, accountCount, "a failed trade must not leave a holding account")
}

func TestCommitImportBatch_InsufficientLotFallbackCleansCreatedInvestmentSetup(t *testing.T) {
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	batchID, rowID := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
		FillID: "orphan-sell", OrderID: "orphan-sell-order", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "SELL", Quantity: "1", Price: "160.00", Currency: "USD",
		FilledAt: "2026-06-02T10:00:00Z", NetValue: "160.00", NetValueCurrency: "EUR",
	})
	f.resolveRowGeneric(t, batchID, rowID, f.cashAccountID)

	result, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	assert.Equal(t, 1, result.CommittedCount, "insufficient lots should retain the generic cash fallback")

	instruments, err := f.investmentSvc.ListInstruments(context.Background())
	require.NoError(t, err)
	assert.Empty(t, instruments, "cash fallback must clean the newly-created instrument")
	var holdingCount, accountCount int
	require.NoError(t, f.database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM import_connection_holdings`).Scan(&holdingCount))
	require.NoError(t, f.database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM accounts`).Scan(&accountCount))
	assert.Zero(t, holdingCount)
	assert.Equal(t, 3, accountCount, "cash fallback must clean the newly-created holding account")
}

func TestCommitImportBatch_MissingDividendDefaultCleansCreatedInstrument(t *testing.T) {
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	batchID, rowID := f.stageDividendRow(t, conn.ID, trading212Dividend{
		Reference: "orphan-dividend", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Quantity: "2", Amount: "4.32", Currency: "EUR", PaidOn: "2026-06-15T00:00:00Z", Type: "DIVIDEND",
	})
	f.resolveRowGeneric(t, batchID, rowID, f.cashAccountID)

	result, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	assert.Equal(t, 1, result.CommittedCount, "missing dividend defaults should retain the generic cash fallback")

	instruments, err := f.investmentSvc.ListInstruments(context.Background())
	require.NoError(t, err)
	assert.Empty(t, instruments, "cash fallback must clean the newly-created dividend instrument")
}

func TestCommitImportBatch_DividendPostsAsInvestmentIncome(t *testing.T) {
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)

	// Set up a dividend default for AAPL first: create the instrument via a
	// buy, then save a dividend default income account, mirroring what a
	// real user would already have from owning the position.
	buyBatch, _ := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
		FillID: "fill-1", OrderID: "order-1", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "BUY", Quantity: "2", Price: "150.25", Currency: "USD",
		FilledAt: "2026-06-01T10:00:00Z", NetValue: "-300.75", NetValueCurrency: "EUR",
	})
	_, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: buyBatch})
	require.NoError(t, err)

	instruments, err := f.investmentSvc.ListInstruments(context.Background())
	require.NoError(t, err)
	require.Len(t, instruments, 1)

	dividendCommodityID := instruments[0].CommodityID
	_, err = f.investmentSvc.SaveDividendDefault(context.Background(), DividendDefaultInput{
		OwnerUserID:     f.ownerUserID,
		CommodityID:     &dividendCommodityID,
		IncomeAccountID: f.categoryAccountID,
		EffectiveFrom:   "2026-01-01", // must be on/before the dividend's paid_on date below
		ChangeReason:    "test fixture",
	})
	require.NoError(t, err)

	divBatch, _ := f.stageDividendRow(t, conn.ID, trading212Dividend{
		Reference: "div-1", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Quantity: "2", Amount: "4.32", Currency: "EUR", PaidOn: "2026-06-15T00:00:00Z", Type: "DIVIDEND",
	})
	result, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: divBatch})
	require.NoError(t, err)
	assert.Equal(t, 1, result.CommittedCount, "the dividend should route through the investment path and commit")

	// A dividend must NOT create a new lot (unlike a buy).
	positions, err := f.investmentSvc.Positions(context.Background())
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.Equal(t, "2", positions[0].QuantityValue.String(), "the dividend must not change the held quantity")

	rows, err := f.importRepo.ListAllImportStagedRows(context.Background(), divBatch)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.True(t, rows[0].CommittedTransactionID.Valid)
	transaction, err := f.importService.transactionService.Transaction(context.Background(), rows[0].CommittedTransactionID.Int64)
	require.NoError(t, err)
	assert.Equal(t, "div-1", transaction.ExternalRefHint)
	assert.JSONEq(t, `{"source_kind":"trading212","connection_id":1,"provider_kind":"dividend","provider_id":"div-1"}`, transaction.MetadataJSON)
}

// failInvestmentImportIdentityWrites injects the exact failure point T-26
// used to leave outside the investment transaction. The trigger runs only
// after the ledger (and, for buys/sells, lot) writes have been attempted.
func failInvestmentImportIdentityWrites(t *testing.T, f *investTestFixture) {
	t.Helper()
	_, err := f.database.ExecContext(context.Background(), `
		CREATE TRIGGER fail_investment_import_identity
		BEFORE INSERT ON import_commit_identities
		BEGIN
			SELECT RAISE(ABORT, 'injected investment import identity failure');
		END
	`)
	require.NoError(t, err)
}

func transactionCount(t *testing.T, f *investTestFixture) int {
	t.Helper()
	var count int
	require.NoError(t, f.database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM transactions`).Scan(&count))
	return count
}

func assertInvestmentIdentityFailure(t *testing.T, f *investTestFixture, batchID int64) {
	t.Helper()
	result, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	assert.Equal(t, 0, result.CommittedCount)
	assert.Equal(t, 1, result.FailedCount)

	rows, err := f.importRepo.ListAllImportStagedRows(context.Background(), batchID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "failed", rows[0].CommitStatus)
	require.True(t, rows[0].CommitError.Valid)
	assert.Contains(t, rows[0].CommitError.String, "injected investment import identity failure")
}

func TestCommitImportBatch_InvestmentBuyRollsBackWhenIdentityRecordingFails(t *testing.T) {
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	batchID, _ := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
		FillID: "atomic-buy", OrderID: "atomic-buy-order", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "BUY", Quantity: "2", Price: "150.00", Currency: "USD",
		FilledAt: "2026-06-01T09:00:00Z", NetValue: "-300.00", NetValueCurrency: "EUR",
	})

	failInvestmentImportIdentityWrites(t, f)
	assertInvestmentIdentityFailure(t, f, batchID)
	assert.Equal(t, 0, transactionCount(t, f), "identity failure must roll back the buy ledger transaction")

	positions, err := f.investmentSvc.Positions(context.Background())
	require.NoError(t, err)
	assert.Empty(t, positions, "identity failure must roll back the acquired lot")
}

func TestCommitImportBatch_InvestmentSellRollsBackWhenIdentityRecordingFails(t *testing.T) {
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	buyBatchID, _ := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
		FillID: "atomic-sell-buy", OrderID: "atomic-sell-buy-order", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "BUY", Quantity: "2", Price: "150.00", Currency: "USD",
		FilledAt: "2026-06-01T09:00:00Z", NetValue: "-300.00", NetValueCurrency: "EUR",
	})
	_, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: buyBatchID})
	require.NoError(t, err)

	sellBatchID, _ := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
		FillID: "atomic-sell", OrderID: "atomic-sell-order", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "SELL", Quantity: "1", Price: "160.00", Currency: "USD",
		FilledAt: "2026-06-02T09:00:00Z", NetValue: "160.00", NetValueCurrency: "EUR",
	})

	failInvestmentImportIdentityWrites(t, f)
	assertInvestmentIdentityFailure(t, f, sellBatchID)
	assert.Equal(t, 1, transactionCount(t, f), "identity failure must roll back the sell ledger transaction")

	positions, err := f.investmentSvc.Positions(context.Background())
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.Equal(t, "2", positions[0].QuantityValue.String(), "identity failure must roll back the lot disposal")
}

func TestCommitImportBatch_InvestmentDividendRollsBackWhenIdentityRecordingFails(t *testing.T) {
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	buyBatchID, _ := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
		FillID: "atomic-dividend-buy", OrderID: "atomic-dividend-buy-order", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "BUY", Quantity: "2", Price: "150.00", Currency: "USD",
		FilledAt: "2026-06-01T09:00:00Z", NetValue: "-300.00", NetValueCurrency: "EUR",
	})
	_, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: buyBatchID})
	require.NoError(t, err)

	instruments, err := f.investmentSvc.ListInstruments(context.Background())
	require.NoError(t, err)
	require.Len(t, instruments, 1)
	commodityID := instruments[0].CommodityID
	_, err = f.investmentSvc.SaveDividendDefault(context.Background(), DividendDefaultInput{
		OwnerUserID: f.ownerUserID, CommodityID: &commodityID, IncomeAccountID: f.categoryAccountID,
		EffectiveFrom: "2026-01-01", ChangeReason: "test fixture",
	})
	require.NoError(t, err)

	dividendBatchID, _ := f.stageDividendRow(t, conn.ID, trading212Dividend{
		Reference: "atomic-dividend", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Quantity: "2", Amount: "4.32", Currency: "EUR", PaidOn: "2026-06-15T00:00:00Z", Type: "DIVIDEND",
	})

	failInvestmentImportIdentityWrites(t, f)
	assertInvestmentIdentityFailure(t, f, dividendBatchID)
	assert.Equal(t, 1, transactionCount(t, f), "identity failure must roll back the dividend ledger transaction")
}

// TestCommitImportBatch_ConcurrentTrading212CommitsKeepTheCommittedRow verifies
// that two callers which both read the same pending Trading 212 row cannot
// turn the winner's committed marker into failed. The loser must recognize the
// existing import identity as its successful idempotent outcome.
func TestCommitImportBatch_ConcurrentTrading212CommitsKeepTheCommittedRow(t *testing.T) {
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	setupBatchID, _ := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
		FillID: "concurrent-setup", OrderID: "concurrent-setup-order", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "BUY", Quantity: "1", Price: "150.00", Currency: "USD",
		FilledAt: "2026-05-30T09:00:00Z", NetValue: "-150.00", NetValueCurrency: "EUR",
	})
	_, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: setupBatchID})
	require.NoError(t, err)
	initialTransactions := transactionCount(t, f)

	batchID, _ := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
		FillID: "concurrent-buy", OrderID: "concurrent-buy-order", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "BUY", Quantity: "2", Price: "150.00", Currency: "USD",
		FilledAt: "2026-06-01T09:00:00Z", NetValue: "-300.00", NetValueCurrency: "EUR",
	})

	var reachedIdentityCheck sync.WaitGroup
	reachedIdentityCheck.Add(2)
	releaseCommits := make(chan struct{})
	f.importService.beforeImportRowCommitForTest = func() {
		reachedIdentityCheck.Done()
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

	reachedIdentityCheck.Wait()
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
	assert.Equal(t, initialTransactions+1, transactionCount(t, f), "idempotent concurrent commits must create one additional ledger transaction")

	batch, err := f.importRepo.ImportBatchByID(context.Background(), BookID, batchID)
	require.NoError(t, err)
	assert.Equal(t, "committed", batch.Status)
}

// TestCommitImportBatch_ConcurrentFirstTimeHoldingMapReusesWinner proves that
// simultaneous first imports for one connection and instrument create one
// holding map/account, reuse that winner for both trades, and clean up the
// losing unused holding account.
func TestCommitImportBatch_ConcurrentFirstTimeHoldingMapReusesWinner(t *testing.T) {
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	_, commodityID, _, err := f.investmentSvc.ResolveOrCreateInstrumentForImport(
		context.Background(), f.ownerUserID, 0, "", "US0378331005", "AAPL_US_EQ", "2026-06-01",
	)
	require.NoError(t, err)

	var initialAccountCount int
	require.NoError(t, f.database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM accounts`).Scan(&initialAccountCount))

	batchOneID, _ := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
		FillID: "holding-race-one", OrderID: "holding-race-one-order", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "BUY", Quantity: "2", Price: "150.00", Currency: "USD",
		FilledAt: "2026-06-01T09:00:00Z", NetValue: "-300.00", NetValueCurrency: "EUR",
	})
	batchTwoID, _ := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
		FillID: "holding-race-two", OrderID: "holding-race-two-order", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "BUY", Quantity: "2", Price: "151.00", Currency: "USD",
		FilledAt: "2026-06-01T10:00:00Z", NetValue: "-302.00", NetValueCurrency: "EUR",
	})

	var reachedMissingHoldingMap sync.WaitGroup
	reachedMissingHoldingMap.Add(2)
	releaseHoldingCreates := make(chan struct{})
	f.importService.beforeTrading212HoldingCreateForTest = func() {
		reachedMissingHoldingMap.Done()
		<-releaseHoldingCreates
	}

	type commitResult struct {
		result CommitImportBatchResult
		err    error
	}
	results := make(chan commitResult, 2)
	for _, batchID := range []int64{batchOneID, batchTwoID} {
		go func() {
			result, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{
				OwnerUserID: f.ownerUserID,
				BatchID:     batchID,
			})
			results <- commitResult{result: result, err: err}
		}()
	}

	reachedMissingHoldingMap.Wait()
	close(releaseHoldingCreates)
	for range 2 {
		commit := <-results
		require.NoError(t, commit.err)
		assert.Equal(t, 1, commit.result.CommittedCount)
		assert.Equal(t, "committed", commit.result.Status)
	}

	holdingAccountID, found, err := f.connService.HoldingAccountForCommodity(context.Background(), conn.ID, commodityID)
	require.NoError(t, err)
	assert.True(t, found)
	require.Positive(t, holdingAccountID)

	var holdingCount, accountCount int
	require.NoError(t, f.database.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM import_connection_holdings WHERE connection_id = ? AND commodity_id = ?
	`, conn.ID, commodityID).Scan(&holdingCount))
	require.NoError(t, f.database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM accounts`).Scan(&accountCount))
	assert.Equal(t, 1, holdingCount)
	assert.Equal(t, initialAccountCount+1, accountCount, "the losing unused holding account must be cleaned up")

	positions, err := f.investmentSvc.Positions(context.Background())
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.Equal(t, holdingAccountID, positions[0].AccountID)
	assert.Equal(t, "4", positions[0].QuantityValue.String())
}

func TestCommitImportStagedRow_DoesNotOverwriteCommitted(t *testing.T) {
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	batchID, _ := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
		FillID: "terminal-status-guard", OrderID: "terminal-status-guard-order", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "BUY", Quantity: "1", Price: "150.00", Currency: "USD",
		FilledAt: "2026-06-01T09:00:00Z", NetValue: "-150.00", NetValueCurrency: "EUR",
	})
	_, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)

	rows, err := f.importRepo.ListAllImportStagedRows(context.Background(), batchID)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	err = f.importRepo.CommitImportStagedRow(context.Background(), db.CommitImportStagedRowParams{
		RowID:        rows[0].ID,
		CommitStatus: "failed",
		CommitError:  sql.NullString{String: "stale concurrent failure", Valid: true},
	})
	require.ErrorIs(t, err, db.ErrImportStagedRowAlreadyCommitted)

	rows, err = f.importRepo.ListAllImportStagedRows(context.Background(), batchID)
	require.NoError(t, err)
	assert.Equal(t, "committed", rows[0].CommitStatus)
	assert.False(t, rows[0].CommitError.Valid)

	err = f.importRepo.CommitImportStagedRow(context.Background(), db.CommitImportStagedRowParams{
		RowID:        rows[0].ID + 1_000_000,
		CommitStatus: "failed",
	})
	require.ErrorIs(t, err, db.ErrNotFound)
}

// TestCommitImportBatch_StaleIdentitySnapshotReturnsCommitted covers a caller
// that has already listed a pending row when another caller atomically posts
// its ledger transaction, import identity, and staged-row marker. The stale
// caller must converge on that committed outcome instead of failing while it
// tries to mark the stale snapshot as skipped.
func TestCommitImportBatch_StaleIdentitySnapshotReturnsCommitted(t *testing.T) {
	f := newInvestTestFixture(t)
	ctx := context.Background()
	batch, err := f.importRepo.CreateImportBatch(ctx, db.CreateImportBatchParams{
		BookID: BookID, SourceKind: "qif", SourceMetaJSON: "{}", CreatedAt: "2026-06-01T00:00:00Z", ActorUserID: f.ownerUserID,
	})
	require.NoError(t, err)
	rows, _, err := f.importService.stageParseResult(ctx, batch.ID, ParseResult{Rows: []StagedRow{{
		DedupeFingerprint: "qif|stale-identity-snapshot", Date: "2026-06-01", Amount: "42.50", CommodityHint: "EUR",
		PayeeHint: "Test Payee", Memo: "stale snapshot", ExternalRef: "stale-snapshot-1",
	}}})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	f.resolveRowGeneric(t, batch.ID, rows[0].ID, f.cashAccountID)

	readerListedPending := make(chan struct{})
	releaseReader := make(chan struct{})
	var identityChecks atomic.Int32
	f.importService.beforeImportCommitIdentityCheckForTest = func() {
		if identityChecks.Add(1) == 1 {
			close(readerListedPending)
			<-releaseReader
		}
	}

	type commitResult struct {
		result CommitImportBatchResult
		err    error
	}
	reader := make(chan commitResult, 1)
	go func() {
		result, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batch.ID})
		reader <- commitResult{result: result, err: err}
	}()
	<-readerListedPending

	winner, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batch.ID})
	require.NoError(t, err)
	assert.Equal(t, 1, winner.CommittedCount)
	assert.Equal(t, "committed", winner.Status)
	close(releaseReader)

	stale := <-reader
	require.NoError(t, stale.err)
	assert.Equal(t, 1, stale.result.CommittedCount)
	assert.Equal(t, "committed", stale.result.Status)
}
