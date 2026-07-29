package app

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// investmentsTestFixture wires a full InvestmentService against one
// in-memory database for service-level tests — trade entrypoints,
// configuration CRUD, gains read models, and suggestion acceptance. Modeled
// on TestPreviewSellRealizedGainAlignsMismatchedCostBasisScales's inline
// setup and investTestFixture in import_trading212_invest_test.go.
type investmentsTestFixture struct {
	database           *sql.DB
	investmentService  *InvestmentService
	accountService     *AccountService
	transactionService *TransactionService
	eurCommodityID     int64
	stockCommodityID   int64
	cashAccountID      int64
	holdingAccountID   int64
	incomeAccountID    int64
	ownerUserID        int64
}

func newInvestmentsTestFixture(t *testing.T) *investmentsTestFixture {
	t.Helper()
	return newInvestmentsTestFixtureWithOptions(t, true)
}

// newInvestmentsTestFixtureWithOptions lets a test opt out of wiring a
// PricingService (withPricing=false) — Buy/Sell/ReinvestedDividend silently
// skip creating a trade-implied price observation when it's nil, which is
// the only way to exercise a genuinely unpriced open position (any real
// pricingService auto-creates an implied price the moment a trade posts).
func newInvestmentsTestFixtureWithOptions(t *testing.T, withPricing bool) *investmentsTestFixture {
	t.Helper()
	ctx := context.Background()
	database := openConnectionTestDatabase(t)

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

	res, err = database.ExecContext(ctx, `
		INSERT INTO commodities (book_id, code, kind, is_builtin, created_at, created_by_user_id)
		VALUES (?, 'TEST', 'security', 0, '2026-01-01T00:00:00Z', 1)
	`, BookID)
	require.NoError(t, err)
	stockCommodityID, err := res.LastInsertId()
	require.NoError(t, err)
	_, err = database.ExecContext(ctx, `
		INSERT INTO commodity_versions (
			commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id,
			change_reason, status, symbol, display_symbol, name, standard_scale, max_quantity_scale
		) VALUES (?, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'test fixture', 'active', 'TEST', 'TEST', 'Test Security', 0, 6)
	`, stockCommodityID)
	require.NoError(t, err)

	cashAccountID := seedTestAccount(t, database, "active", true)
	holdingAccountID := seedTestAccount(t, database, "active", true)
	incomeAccountID := seedTestAccountWithClass(t, database, "active", true, "income", "income")
	seedCommodityTradingAccount(t, database)

	accountRepository := db.NewAccountRepository(database)
	accountService := NewAccountService(accountRepository, db.NewInstitutionRepository(database), nil)
	transactionService := NewTransactionService(db.NewTransactionRepository(database), db.NewPayeeRepository(database), accountRepository, db.NewCommodityRepository(database))
	var pricingService *PricingService
	if withPricing {
		pricingService = NewPricingService(db.NewPricingRepository(database))
	}
	investmentService := NewInvestmentService(db.NewInvestmentRepository(database), accountService, transactionService, pricingService)

	return &investmentsTestFixture{
		database:           database,
		investmentService:  investmentService,
		accountService:     accountService,
		transactionService: transactionService,
		eurCommodityID:     eurCommodityID,
		stockCommodityID:   stockCommodityID,
		cashAccountID:      cashAccountID,
		holdingAccountID:   holdingAccountID,
		incomeAccountID:    incomeAccountID,
		ownerUserID:        1,
	}
}

// seedSuggestion inserts a provider event + a 'suggested' event suggestion
// directly (there is no service method to create one — no producer of
// investment_provider_events exists yet; see docs/backlog.md).
func (f *investmentsTestFixture) seedSuggestion(t *testing.T, proposedTransactionJSON string) int64 {
	t.Helper()
	ctx := context.Background()

	var manualSourceID int64
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT id FROM market_data_sources WHERE code = 'manual'`).Scan(&manualSourceID))

	res, err := f.database.ExecContext(ctx, `
		INSERT INTO investment_provider_events (book_id, source_id, event_family, event_date, status, created_at)
		VALUES (?, ?, 'dividend', '2026-06-01', 'new', '2026-06-01T00:00:00Z')
	`, BookID, manualSourceID)
	require.NoError(t, err)
	providerEventID, err := res.LastInsertId()
	require.NoError(t, err)

	res, err = f.database.ExecContext(ctx, `
		INSERT INTO investment_event_suggestions (
			book_id, provider_event_id, confidence_bps, status, proposed_transaction_json, created_at, updated_at
		) VALUES (?, ?, 9000, 'suggested', ?, '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z')
	`, BookID, providerEventID, proposedTransactionJSON)
	require.NoError(t, err)
	suggestionID, err := res.LastInsertId()
	require.NoError(t, err)
	return suggestionID
}

func (f *investmentsTestFixture) transactionCount(t *testing.T) int {
	t.Helper()
	var count int
	require.NoError(t, f.database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM transactions`).Scan(&count))
	return count
}

func validDividendProposalJSON(cashAccountID, cashCommodityID, incomeAccountID int64) string {
	return `{
		"kind":"dividend_income",
		"transaction_date":"2026-06-01",
		"cash_account_id":` + strconv.FormatInt(cashAccountID, 10) + `,
		"cash_commodity_id":` + strconv.FormatInt(cashCommodityID, 10) + `,
		"amount_value":1250,
		"amount_scale":2,
		"income_account_id":` + strconv.FormatInt(incomeAccountID, 10) + `
	}`
}

// --- AcceptSuggestion: real posting (Part 2 of the investments plan) ---

func TestAcceptSuggestion_PostsProposedDividendAndMarksAccepted(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	suggestionID := f.seedSuggestion(t, validDividendProposalJSON(f.cashAccountID, f.eurCommodityID, f.incomeAccountID))
	initialTransactions := f.transactionCount(t)

	accepted, err := f.investmentService.AcceptSuggestion(context.Background(), f.ownerUserID, 0, "req-accept", suggestionID)
	require.NoError(t, err)

	assert.Equal(t, "accepted", accepted.Status)
	require.NotNil(t, accepted.GeneratedTransactionID)
	assert.Empty(t, accepted.FailureReason)
	assert.Equal(t, initialTransactions+1, f.transactionCount(t))

	transaction, err := f.transactionService.Transaction(context.Background(), *accepted.GeneratedTransactionID)
	require.NoError(t, err)
	assert.Equal(t, "posted", transaction.Status)
	require.Len(t, transaction.JournalEntries, 1)
	require.Len(t, transaction.JournalEntries[0].Postings, 2)
}

func TestAcceptSuggestion_MalformedProposalMarksFailedWithoutPosting(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	suggestionID := f.seedSuggestion(t, `{not-valid-json`)
	initialTransactions := f.transactionCount(t)

	failed, err := f.investmentService.AcceptSuggestion(context.Background(), f.ownerUserID, 0, "req-accept", suggestionID)
	require.NoError(t, err, "a bad suggestion payload must not surface as an API error")

	assert.Equal(t, "failed", failed.Status)
	assert.NotEmpty(t, failed.FailureReason)
	assert.Nil(t, failed.GeneratedTransactionID)
	assert.Equal(t, initialTransactions, f.transactionCount(t))
}

func TestAcceptSuggestion_UnsupportedKindMarksFailedWithoutPosting(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	suggestionID := f.seedSuggestion(t, `{"kind":"split","transaction_date":"2026-06-01"}`)

	failed, err := f.investmentService.AcceptSuggestion(context.Background(), f.ownerUserID, 0, "req-accept", suggestionID)
	require.NoError(t, err)
	assert.Equal(t, "failed", failed.Status)
	assert.Contains(t, failed.FailureReason, "unsupported proposed transaction kind")
}

func TestAcceptSuggestion_InvalidCashAccountMarksFailedWithoutPosting(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	const unknownAccountID = 999999
	suggestionID := f.seedSuggestion(t, validDividendProposalJSON(unknownAccountID, f.eurCommodityID, f.incomeAccountID))
	initialTransactions := f.transactionCount(t)

	failed, err := f.investmentService.AcceptSuggestion(context.Background(), f.ownerUserID, 0, "req-accept", suggestionID)
	require.NoError(t, err)
	assert.Equal(t, "failed", failed.Status)
	assert.NotEmpty(t, failed.FailureReason)
	assert.Equal(t, initialTransactions, f.transactionCount(t))
}

func TestAcceptSuggestion_AlreadyAcceptedReturnsNotPending(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	suggestionID := f.seedSuggestion(t, validDividendProposalJSON(f.cashAccountID, f.eurCommodityID, f.incomeAccountID))

	_, err := f.investmentService.AcceptSuggestion(context.Background(), f.ownerUserID, 0, "req-accept-1", suggestionID)
	require.NoError(t, err)
	initialTransactions := f.transactionCount(t)

	_, err = f.investmentService.AcceptSuggestion(context.Background(), f.ownerUserID, 0, "req-accept-2", suggestionID)
	require.ErrorIs(t, err, ErrInvestmentSuggestionNotPending)
	assert.Equal(t, initialTransactions, f.transactionCount(t), "a rejected double-accept must not post a second transaction")
}

func TestAcceptSuggestion_NotFoundReturnsNotFound(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	_, err := f.investmentService.AcceptSuggestion(context.Background(), f.ownerUserID, 0, "req-accept", 999999)
	require.ErrorIs(t, err, ErrInvestmentSuggestionNotFound)
}

func TestIgnoreSuggestion_AlreadyIgnoredReturnsNotPending(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	suggestionID := f.seedSuggestion(t, validDividendProposalJSON(f.cashAccountID, f.eurCommodityID, f.incomeAccountID))

	ignored, err := f.investmentService.IgnoreSuggestion(context.Background(), f.ownerUserID, 0, "req-ignore-1", suggestionID)
	require.NoError(t, err)
	assert.Equal(t, "ignored", ignored.Status)

	_, err = f.investmentService.IgnoreSuggestion(context.Background(), f.ownerUserID, 0, "req-ignore-2", suggestionID)
	require.ErrorIs(t, err, ErrInvestmentSuggestionNotPending)
}

func TestIgnoreSuggestion_AcceptedSuggestionCannotBeIgnored(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	suggestionID := f.seedSuggestion(t, validDividendProposalJSON(f.cashAccountID, f.eurCommodityID, f.incomeAccountID))

	_, err := f.investmentService.AcceptSuggestion(context.Background(), f.ownerUserID, 0, "req-accept", suggestionID)
	require.NoError(t, err)

	_, err = f.investmentService.IgnoreSuggestion(context.Background(), f.ownerUserID, 0, "req-ignore", suggestionID)
	require.ErrorIs(t, err, ErrInvestmentSuggestionNotPending)
}

// --- 3a: trade entrypoints ---

func TestSell_PostsFourLegTransactionAndDisposesLot(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	bought, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(10), QuantityScale: 0,
		CashAmountValue: 100000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	require.NotNil(t, bought.LotID)

	sold, err := f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(10), QuantityScale: 0,
		CashAmountValue: 120000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)

	assert.Equal(t, "posted", sold.Transaction.Status)
	require.Len(t, sold.Transaction.JournalEntries, 1)
	assert.Len(t, sold.Transaction.JournalEntries[0].Postings, 4)
	require.Len(t, sold.Allocations, 1)
	assert.Equal(t, *bought.LotID, sold.Allocations[0].LotID)
	assert.Equal(t, "10", sold.Allocations[0].QuantityValue.String())
	assert.Equal(t, int64(100000), sold.Allocations[0].CostBasisValue)

	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	assert.Equal(t, "closed", lots[0].Status)
	assert.Equal(t, "0", lots[0].RemainingQuantityValue.String())
}

// TestPreviewSellAndSellProduceIdenticalAllocationsAcrossCostBasisMethods is
// the single most important test in this file: a preview/commit divergence
// is a user-facing trust bug (the sell UI shows the preview, then commits —
// they must always agree). Table-driven over all four cost-basis methods.
func TestPreviewSellAndSellProduceIdenticalAllocationsAcrossCostBasisMethods(t *testing.T) {
	methods := []string{"fifo", "lifo", "average_cost", "specific_lot"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()

			buys := []struct {
				date string
				cash int64
			}{
				{"2026-01-01", 100000},
				{"2026-02-01", 120000},
				{"2026-03-01", 150000},
			}
			var lotIDs []int64
			for _, b := range buys {
				result, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
					OwnerUserID: f.ownerUserID, TransactionDate: b.date,
					CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
					QuantityValue: exact.New(10), QuantityScale: 0,
					CashAmountValue: b.cash, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
				})
				require.NoError(t, err)
				require.NotNil(t, result.LotID)
				lotIDs = append(lotIDs, *result.LotID)
			}

			sellInput := InvestmentTradeInput{
				OwnerUserID: f.ownerUserID, TransactionDate: "2026-04-01",
				CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
				QuantityValue: exact.New(15), QuantityScale: 0,
				CashAmountValue: 200000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
				CostBasisMethod: method,
			}
			if method == "specific_lot" {
				sellInput.LotAllocations = []InvestmentLotAllocationInput{
					{LotID: lotIDs[0], QuantityValue: exact.New(10), QuantityScale: 0},
					{LotID: lotIDs[1], QuantityValue: exact.New(5), QuantityScale: 0},
				}
			}

			preview, err := f.investmentService.PreviewSell(ctx, sellInput)
			require.NoError(t, err)
			assert.Equal(t, method, preview.CostBasisMethod)
			require.NotEmpty(t, preview.Allocations)

			sold, err := f.investmentService.Sell(ctx, sellInput)
			require.NoError(t, err)

			require.Len(t, sold.Allocations, len(preview.Allocations))
			var previewTotalBasis, soldTotalBasis int64
			for i := range preview.Allocations {
				assert.Equal(t, preview.Allocations[i].LotID, sold.Allocations[i].LotID)
				assert.Equal(t, preview.Allocations[i].QuantityValue.String(), sold.Allocations[i].QuantityValue.String())
				assert.Equal(t, preview.Allocations[i].CostBasisValue, sold.Allocations[i].CostBasisValue)
				previewTotalBasis += preview.Allocations[i].CostBasisValue
				soldTotalBasis += sold.Allocations[i].CostBasisValue
			}
			assert.Equal(t, previewTotalBasis, soldTotalBasis, "preview and commit must agree on total disposed basis (drives realized gain)")
		})
	}
}

func TestSell_MethodActuallyChangesDisposedBasis(t *testing.T) {
	// FIFO and LIFO over the same 2-lot seed must produce different disposed
	// basis totals — proves the method resolver isn't silently always FIFO
	// (the exact bug class I-02 in docs/plans/investments-plan.md guarded against).
	newSeededFixture := func(t *testing.T) *investmentsTestFixture {
		f := newInvestmentsTestFixture(t)
		ctx := context.Background()
		_, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
			OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
			CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			QuantityValue: exact.New(10), QuantityScale: 0,
			CashAmountValue: 100000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
		})
		require.NoError(t, err)
		_, err = f.investmentService.Buy(ctx, InvestmentTradeInput{
			OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
			CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			QuantityValue: exact.New(10), QuantityScale: 0,
			CashAmountValue: 150000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
		})
		require.NoError(t, err)
		return f
	}

	fifoBasis := func() int64 {
		f := newSeededFixture(t)
		preview, err := f.investmentService.PreviewSell(context.Background(), InvestmentTradeInput{
			OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01",
			CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			QuantityValue: exact.New(10), QuantityScale: 0,
			CashAmountValue: 130000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
			CostBasisMethod: "fifo",
		})
		require.NoError(t, err)
		var total int64
		for _, a := range preview.Allocations {
			total += a.CostBasisValue
		}
		return total
	}()

	lifoBasis := func() int64 {
		f := newSeededFixture(t)
		preview, err := f.investmentService.PreviewSell(context.Background(), InvestmentTradeInput{
			OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01",
			CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			QuantityValue: exact.New(10), QuantityScale: 0,
			CashAmountValue: 130000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
			CostBasisMethod: "lifo",
		})
		require.NoError(t, err)
		var total int64
		for _, a := range preview.Allocations {
			total += a.CostBasisValue
		}
		return total
	}()

	assert.Equal(t, int64(100000), fifoBasis, "fifo disposes the older (cheaper) lot first")
	assert.Equal(t, int64(150000), lifoBasis, "lifo disposes the newer (pricier) lot first")
	assert.NotEqual(t, fifoBasis, lifoBasis)
}

func TestSell_InsufficientLotsReturnsSentinelError(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	_, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(5), QuantityScale: 0,
		CashAmountValue: 50000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)

	_, err = f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(10), QuantityScale: 0,
		CashAmountValue: 100000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.ErrorIs(t, err, ErrInvestmentLotsInsufficient)
}

func TestReinvestedDividend_OpensNewLotFromIncomeWithoutCashLeg(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	result, err := f.investmentService.ReinvestedDividend(ctx, ReinvestedDividendInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		IncomeAccountID: &f.incomeAccountID,
		QuantityValue:   exact.New(2), QuantityScale: 0,
		AmountValue: 5000, AmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	assert.Equal(t, "posted", result.Transaction.Status)
	require.Len(t, result.Transaction.JournalEntries[0].Postings, 4)
	require.NotNil(t, result.LotID)

	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	assert.Equal(t, "2", lots[0].QuantityValue.String())
	assert.Equal(t, int64(5000), lots[0].CostBasisValue)
}

func TestReinvestedDividend_UsesConfiguredDividendDefaultWhenIncomeAccountOmitted(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	_, err := f.investmentService.SaveDividendDefault(ctx, DividendDefaultInput{
		OwnerUserID: f.ownerUserID, CommodityID: &f.stockCommodityID,
		IncomeAccountID: f.incomeAccountID, Status: "active", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)

	result, err := f.investmentService.ReinvestedDividend(ctx, ReinvestedDividendInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		QuantityValue: exact.New(1), QuantityScale: 0,
		AmountValue: 2500, AmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	assert.NotNil(t, result.LotID)
}

func TestValidateReinvestedDividendInput_RejectsMissingFields(t *testing.T) {
	valid := ReinvestedDividendInput{
		OwnerUserID: 1, TransactionDate: "2026-03-01",
		CommodityID: 5, HoldingAccountID: 6, CashCommodityID: 7,
		QuantityValue: exact.New(1), QuantityScale: 0, AmountValue: 100, AmountScale: 2,
	}
	tests := []struct {
		name   string
		mutate func(*ReinvestedDividendInput)
	}{
		{"missing owner", func(i *ReinvestedDividendInput) { i.OwnerUserID = 0 }},
		{"missing commodity", func(i *ReinvestedDividendInput) { i.CommodityID = 0 }},
		{"missing holding account", func(i *ReinvestedDividendInput) { i.HoldingAccountID = 0 }},
		{"missing cash commodity", func(i *ReinvestedDividendInput) { i.CashCommodityID = 0 }},
		{"zero quantity", func(i *ReinvestedDividendInput) { i.QuantityValue = exact.New(0) }},
		{"negative quantity", func(i *ReinvestedDividendInput) { i.QuantityValue = exact.New(-1) }},
		{"zero amount", func(i *ReinvestedDividendInput) { i.AmountValue = 0 }},
		{"missing date", func(i *ReinvestedDividendInput) { i.TransactionDate = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			_, err := validateReinvestedDividendInput(input)
			require.Error(t, err)
		})
	}
}

func TestDividend_DirectWithExplicitIncomeAccount(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	transaction, err := f.investmentService.Dividend(ctx, DividendInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01",
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		IncomeAccountID: &f.incomeAccountID,
		AmountValue:     10000, AmountScale: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, "posted", transaction.Status)
	require.Len(t, transaction.JournalEntries[0].Postings, 2)
}

func TestDividend_WithholdingLegOnlyPresentWhenConfigured(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	withholdingAccountID := seedTestAccountWithClass(t, f.database, "active", true, "liability", "other_liability")

	transaction, err := f.investmentService.Dividend(ctx, DividendInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01",
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		IncomeAccountID: &f.incomeAccountID,
		AmountValue:     10000, AmountScale: 2,
		WithholdingValue:     ptrInt64(1500),
		WithholdingAccountID: &withholdingAccountID,
	})
	require.NoError(t, err)
	assert.Len(t, transaction.JournalEntries[0].Postings, 4, "withholding adds a second posting pair")
}

func TestDividend_ResolvesDefaultsFromCommodityWhenAccountsOmitted(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	_, err := f.investmentService.SaveDividendDefault(ctx, DividendDefaultInput{
		OwnerUserID: f.ownerUserID, CommodityID: &f.stockCommodityID,
		IncomeAccountID: f.incomeAccountID, Status: "active", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)

	transaction, err := f.investmentService.Dividend(ctx, DividendInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01",
		CommodityID:   &f.stockCommodityID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		AmountValue: 10000, AmountScale: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, "posted", transaction.Status)
	assert.Len(t, transaction.JournalEntries[0].Postings, 2, "no withholding leg when the default has none configured")
}

func TestValidateTradeInput_RejectsEdgeCases(t *testing.T) {
	valid := InvestmentTradeInput{
		OwnerUserID: 1, TransactionDate: "2026-03-01",
		CommodityID: 5, HoldingAccountID: 6, CashAccountID: 7, CashCommodityID: 8,
		QuantityValue: exact.New(1), QuantityScale: 0,
		CashAmountValue: 100, CashAmountScale: 2,
	}
	tests := []struct {
		name   string
		mutate func(*InvestmentTradeInput)
	}{
		{"zero quantity", func(i *InvestmentTradeInput) { i.QuantityValue = exact.New(0) }},
		{"negative quantity", func(i *InvestmentTradeInput) { i.QuantityValue = exact.New(-5) }},
		{"zero cash amount", func(i *InvestmentTradeInput) { i.CashAmountValue = 0 }},
		{"negative cash amount", func(i *InvestmentTradeInput) { i.CashAmountValue = -100 }},
		{"missing holding account", func(i *InvestmentTradeInput) { i.HoldingAccountID = 0 }},
		{"missing cash account", func(i *InvestmentTradeInput) { i.CashAccountID = 0 }},
		{"invalid quantity scale", func(i *InvestmentTradeInput) { i.QuantityScale = -1 }},
		{"invalid cost basis method", func(i *InvestmentTradeInput) { i.CostBasisMethod = "bogus" }},
		{"lot allocations without specific_lot", func(i *InvestmentTradeInput) {
			i.LotAllocations = []InvestmentLotAllocationInput{{LotID: 1, QuantityValue: exact.New(1)}}
		}},
		{"specific_lot without allocations", func(i *InvestmentTradeInput) { i.CostBasisMethod = "specific_lot" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			require.Error(t, validateTradeInput(input))
		})
	}
}

func TestValidateDividendInput_RejectsEdgeCases(t *testing.T) {
	valid := DividendInput{
		OwnerUserID: 1, TransactionDate: "2026-03-01",
		CashAccountID: 7, CashCommodityID: 8, AmountValue: 100, AmountScale: 2,
	}
	tests := []struct {
		name   string
		mutate func(*DividendInput)
	}{
		{"missing owner", func(i *DividendInput) { i.OwnerUserID = 0 }},
		{"missing cash account", func(i *DividendInput) { i.CashAccountID = 0 }},
		{"missing cash commodity", func(i *DividendInput) { i.CashCommodityID = 0 }},
		{"zero amount", func(i *DividendInput) { i.AmountValue = 0 }},
		{"negative amount", func(i *DividendInput) { i.AmountValue = -1 }},
		{"missing date", func(i *DividendInput) { i.TransactionDate = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			_, err := validateDividendInput(input)
			require.Error(t, err)
		})
	}
}

// --- 3b: configuration CRUD ---

func TestListCostBasisProfiles_SeedsAndReturnsDefaultFIFO(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	profiles, err := f.investmentService.ListCostBasisProfiles(context.Background(), f.ownerUserID, 0, "req-list")
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, "fifo", profiles[0].Method)
	assert.True(t, profiles[0].IsDefault)
}

func TestSaveCostBasisProfile_CreateUpdateAndDefaultUniqueness(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	// Ensure the system default exists first (mirrors ListCostBasisProfiles's
	// EnsureDefaultCostBasisProfile call the UI relies on).
	_, err := f.investmentService.ListCostBasisProfiles(ctx, f.ownerUserID, 0, "req-ensure")
	require.NoError(t, err)

	created, err := f.investmentService.SaveCostBasisProfile(ctx, CostBasisProfileInput{
		OwnerUserID: f.ownerUserID, Name: "ISA Average Cost", Method: "average_cost",
		IsDefault: true, Status: "active",
	})
	require.NoError(t, err)
	assert.Equal(t, "average_cost", created.Method)
	assert.True(t, created.IsDefault)

	profiles, err := f.investmentService.ListCostBasisProfiles(ctx, f.ownerUserID, 0, "req-list")
	require.NoError(t, err)
	var defaults int
	for _, p := range profiles {
		if p.IsDefault {
			defaults++
		}
	}
	assert.Equal(t, 1, defaults, "saving a new default must clear the previous one, never leave two")

	updated, err := f.investmentService.SaveCostBasisProfile(ctx, CostBasisProfileInput{
		OwnerUserID: f.ownerUserID, ProfileID: created.ID, Name: "ISA Average Cost v2", Method: "average_cost",
		IsDefault: true, Status: "active",
	})
	require.NoError(t, err)
	assert.Equal(t, created.ID, updated.ID)
	assert.Equal(t, "ISA Average Cost v2", updated.Name)
}

func TestCleanCostBasisProfileSpec_RejectsInvalidInputs(t *testing.T) {
	valid := CostBasisProfileInput{Name: "Default", Method: "fifo", Status: "active"}
	tests := []struct {
		name   string
		mutate func(*CostBasisProfileInput)
	}{
		{"missing name", func(i *CostBasisProfileInput) { i.Name = " " }},
		{"unknown method", func(i *CostBasisProfileInput) { i.Method = "bogus" }},
		{"invalid status", func(i *CostBasisProfileInput) { i.Status = "deleted" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			_, err := cleanCostBasisProfileSpec(input)
			require.Error(t, err)
		})
	}
}

func TestListAndSaveDividendDefaults(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	empty, err := f.investmentService.ListDividendDefaults(ctx, 0)
	require.NoError(t, err)
	assert.Empty(t, empty)

	created, err := f.investmentService.SaveDividendDefault(ctx, DividendDefaultInput{
		OwnerUserID: f.ownerUserID, CommodityID: &f.stockCommodityID,
		IncomeAccountID: f.incomeAccountID, Status: "active", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)
	assert.Equal(t, f.incomeAccountID, created.IncomeAccountID)

	list, err := f.investmentService.ListDividendDefaults(ctx, f.stockCommodityID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, created.ID, list[0].ID)

	updated, err := f.investmentService.SaveDividendDefault(ctx, DividendDefaultInput{
		OwnerUserID: f.ownerUserID, DefaultID: created.ID, CommodityID: &f.stockCommodityID,
		IncomeAccountID: f.incomeAccountID, Status: "archived", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)
	assert.Equal(t, "archived", updated.Status)
}

func TestCleanDividendDefaultSpec_RejectsInvalidInputs(t *testing.T) {
	valid := DividendDefaultInput{IncomeAccountID: 1, Status: "active", EffectiveFrom: "2026-01-01"}
	tests := []struct {
		name   string
		mutate func(*DividendDefaultInput)
	}{
		{"missing income account", func(i *DividendDefaultInput) { i.IncomeAccountID = 0 }},
		{"invalid status", func(i *DividendDefaultInput) { i.Status = "deleted" }},
		{"effective_to before effective_from", func(i *DividendDefaultInput) { i.EffectiveTo = "2025-01-01" }},
		{"tax country wrong length", func(i *DividendDefaultInput) { i.TaxCountryCode = "N" }},
		{"withholding rate out of range", func(i *DividendDefaultInput) { i.WithholdingRateBPS = ptrInt64(10001) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			_, err := cleanDividendDefaultSpec(input, mustParseTestTime(t, "2026-01-01"))
			require.Error(t, err)
		})
	}
}

func TestSearch_MatchesInstrumentAndReturnsEmptyForBlankQuery(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	instrument, err := f.investmentService.CreateInstrument(ctx, InvestmentInstrumentInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", CommodityCode: "VWRL", InstrumentType: "etf",
		DisplayName: "Vanguard FTSE All-World", Symbol: "VWRL",
		QuantityScale: 6, PriceScale: 4, EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)

	empty, err := f.investmentService.Search(ctx, "  ")
	require.NoError(t, err)
	assert.Empty(t, empty)

	results, err := f.investmentService.Search(ctx, "VWRL")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, instrument.ID, results[0].ID)
}

func TestUpdateInstrument_CreatesNewVersionWithChangedFields(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	instrument, err := f.investmentService.CreateInstrument(ctx, InvestmentInstrumentInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", CommodityCode: "VWRL", InstrumentType: "etf",
		DisplayName: "Vanguard FTSE All-World", Symbol: "VWRL",
		QuantityScale: 6, PriceScale: 4, EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)

	updated, err := f.investmentService.UpdateInstrument(ctx, InvestmentInstrumentInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", InstrumentID: instrument.ID,
		CommodityCode: "VWRL", InstrumentType: "etf",
		DisplayName: "Vanguard FTSE All-World UCITS ETF", Symbol: "VWRL",
		QuantityScale: 6, PriceScale: 4, EffectiveFrom: "2026-06-01",
	})
	require.NoError(t, err)
	assert.Equal(t, instrument.ID, updated.ID)
	assert.Equal(t, "Vanguard FTSE All-World UCITS ETF", updated.DisplayName)
}

func TestListProviderEvents_NewestFirst(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	var manualSourceID int64
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT id FROM market_data_sources WHERE code = 'manual'`).Scan(&manualSourceID))
	_, err := f.database.ExecContext(ctx, `
		INSERT INTO investment_provider_events (book_id, source_id, event_family, event_date, status, created_at)
		VALUES (?, ?, 'dividend', '2026-01-01', 'new', '2026-01-01T00:00:00Z'),
		       (?, ?, 'dividend', '2026-06-01', 'new', '2026-06-01T00:00:00Z')
	`, BookID, manualSourceID, BookID, manualSourceID)
	require.NoError(t, err)

	events, err := f.investmentService.ListProviderEvents(ctx)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "2026-06-01", events[0].EventDate, "newest event date first")
	assert.Equal(t, "2026-01-01", events[1].EventDate)
}

func mustParseTestTime(t *testing.T, date string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", date)
	require.NoError(t, err)
	return parsed
}

// --- 3c: gains read models ---

func TestListRealizedGains_AppLayerMapsSignAndFilters(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	_, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(10), QuantityScale: 0,
		CashAmountValue: 100000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(10), QuantityScale: 0,
		CashAmountValue: 120000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)

	entries, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, int64(20000), entries[0].RealizedGainValue, "sold for 1200.00, cost 1000.00 -> 200.00 gain")

	inRange, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{From: "2026-02-01", To: "2026-02-01"})
	require.NoError(t, err)
	assert.Len(t, inRange, 1, "date filter is inclusive on both ends")

	outOfRange, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{From: "2026-03-01"})
	require.NoError(t, err)
	assert.Empty(t, outOfRange)
}

// TestListUnrealizedGains_NilWhenNoPriceExists uses a fixture with no
// PricingService wired, so Buy does not auto-create a trade-implied price —
// the only way to exercise a genuinely unpriced open position (any real
// pricingService creates an implied price the moment a trade posts).
func TestListUnrealizedGains_NilWhenNoPriceExists(t *testing.T) {
	f := newInvestmentsTestFixtureWithOptions(t, false)
	ctx := context.Background()

	_, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(10), QuantityScale: 0,
		CashAmountValue: 100000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)

	unpriced, err := f.investmentService.ListUnrealizedGains(ctx)
	require.NoError(t, err)
	require.Len(t, unpriced, 1)
	assert.Nil(t, unpriced[0].UnrealizedGainValue, "no price observation yet -> nil, never zero")
	assert.Nil(t, unpriced[0].MarketValueValue)
}

// TestListUnrealizedGains_NonNilWhenPriceExists proves the app-layer wrapper
// passes a real price observation through with non-nil gain/market-value
// fields — the arithmetic itself is already covered by the DB-layer
// PositionsWithGains tests. The default fixture's live PricingService means
// Buy already creates a trade-implied price, so this needs no manual seed.
func TestListUnrealizedGains_NonNilWhenPriceExists(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	_, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(10), QuantityScale: 0,
		CashAmountValue: 100000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)

	priced, err := f.investmentService.ListUnrealizedGains(ctx)
	require.NoError(t, err)
	require.Len(t, priced, 1)
	require.NotNil(t, priced[0].UnrealizedGainValue, "the buy's trade-implied price makes this position priced")
	require.NotNil(t, priced[0].MarketValueValue)
}

// --- 3b (continued): automation rules and suggestions at the app layer ---
// (the DB-layer replace-all mechanics are proven by
// TestReplaceAutomationRulesArchivesOmittedRules in db/investments_test.go —
// these tests cover the app-layer wiring: validation, listing, error mapping.)

func TestSaveAndListAutomationRules_AppLayer(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	saved, err := f.investmentService.SaveAutomationRules(ctx, SaveAutomationRulesInput{
		OwnerUserID: f.ownerUserID,
		Rules: []InvestmentAutomationRuleInput{
			{EventFamily: "dividend", Mode: "suggest", ConfidenceThresholdBPS: 8000, EffectiveFrom: "2026-01-01"},
		},
	})
	require.NoError(t, err)
	require.Len(t, saved, 1)
	assert.Equal(t, "suggest", saved[0].Mode)

	list, err := f.investmentService.ListAutomationRules(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, saved[0].ID, list[0].ID)

	// An empty-list save archives the rule (true replace semantics, Part 1).
	empty, err := f.investmentService.SaveAutomationRules(ctx, SaveAutomationRulesInput{OwnerUserID: f.ownerUserID})
	require.NoError(t, err)
	assert.Empty(t, empty)

	afterClear, err := f.investmentService.ListAutomationRules(ctx)
	require.NoError(t, err)
	require.Len(t, afterClear, 1)
	assert.Equal(t, "archived", afterClear[0].Status)
}

func TestCleanAutomationRuleSpec_RejectsInvalidInputs(t *testing.T) {
	valid := InvestmentAutomationRuleInput{EventFamily: "dividend", Mode: "suggest", ConfidenceThresholdBPS: 8000, EffectiveFrom: "2026-01-01"}
	tests := []struct {
		name   string
		mutate func(*InvestmentAutomationRuleInput)
	}{
		{"invalid event family", func(i *InvestmentAutomationRuleInput) { i.EventFamily = "bogus" }},
		{"invalid mode", func(i *InvestmentAutomationRuleInput) { i.Mode = "bogus" }},
		{"confidence threshold too high", func(i *InvestmentAutomationRuleInput) { i.ConfidenceThresholdBPS = 10001 }},
		{"confidence threshold negative", func(i *InvestmentAutomationRuleInput) { i.ConfidenceThresholdBPS = -1 }},
		{"invalid status", func(i *InvestmentAutomationRuleInput) { i.Status = "deleted" }},
		{"effective_to before effective_from", func(i *InvestmentAutomationRuleInput) { i.EffectiveTo = "2025-01-01" }},
		{"auto_post without required accounts", func(i *InvestmentAutomationRuleInput) { i.Mode = "auto_post" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			_, err := cleanAutomationRuleSpec(input, mustParseTestTime(t, "2026-01-01"))
			require.Error(t, err)
		})
	}
}

func TestListEventSuggestions_ReturnsSeededSuggestions(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	f.seedSuggestion(t, validDividendProposalJSON(f.cashAccountID, f.eurCommodityID, f.incomeAccountID))
	f.seedSuggestion(t, validDividendProposalJSON(f.cashAccountID, f.eurCommodityID, f.incomeAccountID))

	suggestions, err := f.investmentService.ListEventSuggestions(context.Background())
	require.NoError(t, err)
	assert.Len(t, suggestions, 2)
}

// TestSell_AverageCostMismatchedQuantityScaleFailsLoudlyAtServiceLayer is the
// service-path counterpart to db.TestInvestmentLotsAverageCostMismatchedScaleReturnsError
// (Workstream 5 item 6): two ordinary Buy calls with different QuantityScale
// values for the same holding account/commodity is a real, reachable way to
// end up with open lots at different scales (validateTradeInput has no
// cross-lot scale consistency check) — average_cost must reject this loudly
// through Sell/PreviewSell, not just at the raw disposeLotTx layer.
func TestSell_AverageCostMismatchedQuantityScaleFailsLoudlyAtServiceLayer(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	_, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(10), QuantityScale: 0,
		CashAmountValue: 100000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(1000), QuantityScale: 2, // same 10 units, expressed at a finer scale
		CashAmountValue: 120000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)

	_, err = f.investmentService.PreviewSell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(5), QuantityScale: 0,
		CashAmountValue: 60000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
		CostBasisMethod: "average_cost",
	})
	require.Error(t, err, "average_cost must fail loudly, not silently mis-compute, when open lots have mismatched quantity scales")

	_, err = f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(5), QuantityScale: 0,
		CashAmountValue: 60000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
		CostBasisMethod: "average_cost",
	})
	require.Error(t, err, "commit must reject the same way preview did")
}
