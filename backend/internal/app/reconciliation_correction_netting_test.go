package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// T-120 #135. A correction appends an inverse and a replacement journal under
// one command. The writer used to guard each journal on its own, so an inverse
// and replacement that cancel at a checkpoint's boundary still invalidated it.
// The command's combined delta is now summed per account and commodity up to
// each checkpoint's own (date, sequence) boundary: the first boundary whose
// sum is non-zero is invalidated with every later checkpoint, and a checkpoint
// whose reconciled balance the command leaves unchanged stays active.

func reconcileAccount(t *testing.T, f *investmentsTestFixture, accountID, commodityID int64, statementDate string, balance int64) int64 {
	t.Helper()
	ctx := context.Background()
	session, err := f.transactionService.StartReconciliation(ctx, StartReconciliationInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, AccountID: accountID,
		CommodityID: commodityID, StatementDate: statementDate, StatementBalanceValue: exact.New(balance),
	})
	require.NoError(t, err)
	postingIDs := make([]int64, 0, len(session.Candidates))
	for _, candidate := range session.Candidates {
		postingIDs = append(postingIDs, candidate.PostingID)
	}
	_, err = f.transactionService.UpdateReconciliationSelection(ctx, ReconciliationSelectionInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, SessionID: session.ID, PostingVersionIDs: postingIDs,
	})
	require.NoError(t, err)
	_, err = f.transactionService.FinishReconciliation(ctx, FinishReconciliationInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, SessionID: session.ID, ChangeReason: "statement verified",
	})
	require.NoError(t, err)
	var checkpointID int64
	require.NoError(t, f.database.QueryRowContext(ctx,
		`SELECT id FROM reconciliation_checkpoints WHERE session_id = ?`, session.ID).Scan(&checkpointID))
	return checkpointID
}

func activeCheckpointIDsFor(t *testing.T, f *investmentsTestFixture, accountID, commodityID int64) []int64 {
	t.Helper()
	checkpoints, err := f.transactionService.repository.ListReconciliationCheckpoints(context.Background(), BookID, accountID, commodityID)
	require.NoError(t, err)
	active := []int64{}
	for _, checkpoint := range checkpoints {
		if checkpoint.Status == "active" {
			active = append(active, checkpoint.ID)
		}
	}
	return active
}

func buyForNetting(t *testing.T, f *investmentsTestFixture, date string, quantity int64, cash int64) InvestmentTradeResult {
	t.Helper()
	bought, err := f.investmentService.Buy(context.Background(), InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: date,
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(quantity), CashAmountValue: cash, CashAmountScale: 2,
	})
	require.NoError(t, err)
	return bought
}

func buyReplacementForNetting(f *investmentsTestFixture, transactionID int64, date string, quantity int64, cash int64) ReplaceInvestmentBuyInput {
	return ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: transactionID, Reason: "broker corrected the fill",
		Replacement: InvestmentTradeInput{
			TransactionDate: date, CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(quantity),
			CashAmountValue: cash, CashAmountScale: 2,
		},
	}
}

func checkpointIntegrityResult(t *testing.T, f *investmentsTestFixture) SelfCheckResult {
	t.Helper()
	run, err := selfCheckOver(t, f.database).RunSelfCheck(context.Background(), "manual")
	require.NoError(t, err)
	for _, result := range run.Results {
		if result.CheckID == CheckCheckpointIntegrity {
			return result
		}
	}
	t.Fatal("self-check has no checkpoint integrity result")
	return SelfCheckResult{}
}

// The issue's first named case: a same-date, quantity-only buy correction. The
// cash inverse and replacement cancel, so the cash statement still holds; the
// holding quantity changes, so the holding checkpoint is still guarded.
func TestQuantityOnlyBuyCorrectionPreservesCashCheckpointAndGuardsHolding(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original := buyForNetting(t, f, "2026-01-01", 10, 100000)
	cash := reconcileAccount(t, f, f.cashAccountID, f.eurCommodityID, "2026-01-31", -1000)
	holding := reconcileAccount(t, f, f.holdingAccountID, f.stockCommodityID, "2026-01-31", 10)
	input := buyReplacementForNetting(f, original.Transaction.ID, "2026-01-01", 12, 100000)

	impact, err := f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
	require.NoError(t, err)
	requireCheckpointImpactIDs(t, impact, holding)

	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired, "the reconciled holding quantity changes")
	require.Equal(t, []int64{cash}, activeCheckpointIDsFor(t, f, f.cashAccountID, f.eurCommodityID))
	require.Equal(t, []int64{holding}, activeCheckpointIDsFor(t, f, f.holdingAccountID, f.stockCommodityID))

	input.ReconciliationOverride = true
	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, input)
	require.NoError(t, err)
	require.Equal(t, []int64{cash}, activeCheckpointIDsFor(t, f, f.cashAccountID, f.eurCommodityID),
		"the inverse and replacement cancel at the cash boundary")
	require.Empty(t, activeCheckpointIDsFor(t, f, f.holdingAccountID, f.stockCommodityID))
	require.Equal(t, SelfCheckPassed, checkpointIntegrityResult(t, f).Status)

	// The offsetting pair stays uncleared inside the reconciled period and is
	// offered to the next session, where it changes nothing: reconciling the
	// next statement at the unchanged balance still finishes.
	reconcileAccount(t, f, f.cashAccountID, f.eurCommodityID, "2026-02-28", -1000)
}

// A correction that keeps the cash amount but moves the settlement date across
// a boundary changes that statement: the inverse settles inside it and the
// replacement after it. A later checkpoint both settle inside still goes too,
// with the cascade.
func TestCorrectionMovingSettlementAcrossBoundaryInvalidates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), GrossAmountValue: tradeMoney(-100000), GrossAmountScale: 2,
		NetSettlementValue: tradeMoney(-100000), NetSettlementScale: 2, SettlementDate: "2026-01-03",
	})
	require.NoError(t, err)
	early := reconcileAccount(t, f, f.cashAccountID, f.eurCommodityID, "2026-01-04", -1000)
	postEUR(t, f, "2026-01-10", 10)
	late := reconcileAccount(t, f, f.cashAccountID, f.eurCommodityID, "2026-01-31", -990)
	input := ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID, Reason: "settled later",
		ReconciliationOverride: true,
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(10),
			GrossAmountValue: tradeMoney(-100000), GrossAmountScale: 2,
			NetSettlementValue: tradeMoney(-100000), NetSettlementScale: 2, SettlementDate: "2026-01-05",
		},
	}

	impact, err := f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
	require.NoError(t, err)
	requireCheckpointImpactIDs(t, impact, early, late)
	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, input)
	require.NoError(t, err)
	require.Empty(t, activeCheckpointIDsFor(t, f, f.cashAccountID, f.eurCommodityID))
}

// Multiple checkpoints: a correction whose whole delta lies after the first
// boundary leaves the first checkpoint active and invalidates the later one.
func TestCorrectionAfterEarlierCheckpointPreservesIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	postEUR(t, f, "2026-01-10", 50)
	january := reconcileAccount(t, f, f.cashAccountID, f.eurCommodityID, "2026-01-31", 50)
	original := buyForNetting(t, f, "2026-02-10", 10, 100000)
	february := reconcileAccount(t, f, f.cashAccountID, f.eurCommodityID, "2026-02-28", -950)
	input := buyReplacementForNetting(f, original.Transaction.ID, "2026-02-10", 10, 110000)
	input.ReconciliationOverride = true

	impact, err := f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
	require.NoError(t, err)
	requireCheckpointImpactIDs(t, impact, february)
	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, input)
	require.NoError(t, err)
	require.Equal(t, []int64{january}, activeCheckpointIDsFor(t, f, f.cashAccountID, f.eurCommodityID))
}

// Equal amounts on different sides of a same-day boundary do not cancel. The
// inverse keeps the original 31 January date but lands after January's
// boundary on that day; the replacement on 30 January lands inside it. Netting
// by date alone would see both inside and preserve a statement that changed.
func TestEqualCorrectionAmountsAcrossSameDayBoundaryDoNotCancel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original := buyForNetting(t, f, "2026-01-31", 10, 100000)
	january := reconcileAccount(t, f, f.cashAccountID, f.eurCommodityID, "2026-01-31", -1000)
	input := buyReplacementForNetting(f, original.Transaction.ID, "2026-01-30", 10, 100000)

	impact, err := f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
	require.NoError(t, err)
	requireCheckpointImpactIDs(t, impact, january)
	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	require.Equal(t, []int64{january}, activeCheckpointIDsFor(t, f, f.cashAccountID, f.eurCommodityID))
}

// A chained correction — replacing the replacement — nets the same way.
func TestChainedCashNeutralCorrectionsPreserveCashCheckpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original := buyForNetting(t, f, "2026-01-01", 10, 100000)
	cash := reconcileAccount(t, f, f.cashAccountID, f.eurCommodityID, "2026-01-31", -1000)

	first, err := acknowledgedReplaceBuy(ctx, f.investmentService,
		buyReplacementForNetting(f, original.Transaction.ID, "2026-01-01", 12, 100000))
	require.NoError(t, err)
	second, err := acknowledgedReplaceBuy(ctx, f.investmentService,
		buyReplacementForNetting(f, first.Replacement.Transaction.ID, "2026-01-01", 11, 100000))
	require.NoError(t, err)
	require.NotZero(t, second.Replacement.Transaction.ID)
	require.Equal(t, []int64{cash}, activeCheckpointIDsFor(t, f, f.cashAccountID, f.eurCommodityID))
	require.Equal(t, SelfCheckPassed, checkpointIntegrityResult(t, f).Status)
}

// Differing fee dates: a fee paid separately from the cash account posts on its
// own date. Moving that date within one statement period cancels at every
// boundary, so both checkpoints stay active without an override; moving it
// across a boundary changes that statement and invalidates it and later ones.
func TestCorrectionMovingSeparatelyPaidFeeNetsPerBoundary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	feeAccount := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	trade := func(feePaidOn string) InvestmentTradeInput {
		return InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
			CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
			CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
			QuantityValue: exact.New(10), GrossAmountValue: tradeMoney(-100000), GrossAmountScale: 2,
			NetSettlementValue: tradeMoney(-100000), NetSettlementScale: 2,
			Charges: []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -500, AmountScale: 2,
				CommodityID: f.eurCommodityID, ChargeAccountID: &feeAccount, CashAccountID: &f.cashAccountID, PaidOn: feePaidOn,
				Treatment: "separately_expensed"}}}
	}
	original, err := f.investmentService.Buy(ctx, trade("2026-01-10"))
	require.NoError(t, err)
	early := reconcileAccount(t, f, f.cashAccountID, f.eurCommodityID, "2026-01-05", -1000)
	late := reconcileAccount(t, f, f.cashAccountID, f.eurCommodityID, "2026-01-31", -1005)

	within := ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
		Reason: "fee paid later in January", Replacement: trade("2026-01-20")}
	impact, err := f.investmentService.ReplaceBuyReconciliationImpact(ctx, within)
	require.NoError(t, err)
	require.Empty(t, impact.AffectedCheckpoints)
	first, err := acknowledgedReplaceBuy(ctx, f.investmentService, within)
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{early, late}, activeCheckpointIDsFor(t, f, f.cashAccountID, f.eurCommodityID))

	across := ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: first.Replacement.Transaction.ID,
		Reason: "fee was paid with the trade", ReconciliationOverride: true, Replacement: trade("2026-01-03")}
	impact, err = f.investmentService.ReplaceBuyReconciliationImpact(ctx, across)
	require.NoError(t, err)
	requireCheckpointImpactIDs(t, impact, early, late)
	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, across)
	require.NoError(t, err)
	require.Empty(t, activeCheckpointIDsFor(t, f, f.cashAccountID, f.eurCommodityID))
}
