package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
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
	tradingAccountID := seedCommodityTradingAccount(t, database)

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
}
