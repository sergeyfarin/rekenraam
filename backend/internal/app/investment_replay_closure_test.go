package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-124 selects an affected-position dependency closure, replayed as one
// dated stream, over a whole-book rebuild (ADR 0013 refinement). These tests
// pin the closure the follow-up implementation must replay, and the atomic
// refusal that stays in force until it lands.

// chainTransfer moves one unit of lotID from source to destination.
func chainTransfer(t *testing.T, f *investmentsTestFixture, source, destination, lotID int64, date string) InternalTransferResult {
	t.Helper()
	input := internalTransferFromLot(f, destination, lotID, exact.New(1), 0)
	input.SourceAccountID, input.EffectiveOn = source, date
	result, err := f.investmentService.InternalTransfer(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, result.DestinationLotIDs, 1)
	return result
}

func replayPosition(f *investmentsTestFixture, accountID int64, from string) db.InvestmentReplayPosition {
	return db.InvestmentReplayPosition{AccountID: accountID, CommodityID: f.stockCommodityID,
		CostCommodityID: f.eurCommodityID, AffectedFrom: from}
}

func TestReplayClosureFollowsTransferChainAndExcludesUnrelatedPositions(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	b := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	c := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	unrelated := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	toB := chainTransfer(t, f, f.holdingAccountID, b, *buy.LotID, "2026-06-01")
	chainTransfer(t, f, b, c, toB.DestinationLotIDs[0], "2026-06-15")
	sale := sellInput(f, "2026-07-01", 1)
	sale.HoldingAccountID = c
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-05-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: unrelated, CashAccountID: f.cashAccountID, QuantityValue: exact.New(2),
		CashAmountValue: 2000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)

	closure, err := f.investmentService.repository.InvestmentReplayClosure(ctx, BookID,
		[]db.InvestmentReplayPosition{replayPosition(f, f.holdingAccountID, "2026-05-01")})
	require.NoError(t, err)
	assert.Equal(t, []db.InvestmentReplayPosition{
		replayPosition(f, f.holdingAccountID, "2026-05-01"),
		replayPosition(f, b, "2026-06-01"),
		replayPosition(f, c, "2026-06-15"),
	}, closure, "source → destination → further transfer, each from its transfer date")

	// A change dated after every transfer cannot alter a carried basis.
	closure, err = f.investmentService.repository.InvestmentReplayClosure(ctx, BookID,
		[]db.InvestmentReplayPosition{replayPosition(f, f.holdingAccountID, "2026-06-02")})
	require.NoError(t, err)
	assert.Equal(t, []db.InvestmentReplayPosition{replayPosition(f, f.holdingAccountID, "2026-06-02")}, closure)

	// Seeds can arrive in any order: a path found later with an earlier date
	// lowers a position already in the closure, and is walked again from it.
	closure, err = f.investmentService.repository.InvestmentReplayClosure(ctx, BookID,
		[]db.InvestmentReplayPosition{replayPosition(f, c, "2026-07-01"), replayPosition(f, f.holdingAccountID, "2026-05-01")})
	require.NoError(t, err)
	assert.Equal(t, replayPosition(f, c, "2026-06-15"), closure[2])

	// A destination change never flows back upstream.
	closure, err = f.investmentService.repository.InvestmentReplayClosure(ctx, BookID,
		[]db.InvestmentReplayPosition{replayPosition(f, c, "2026-06-15")})
	require.NoError(t, err)
	assert.Equal(t, []db.InvestmentReplayPosition{replayPosition(f, c, "2026-06-15")}, closure)
}

func TestReplayClosureSettlesTransferCycleOnEarliestAffectedDates(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	b := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	toB := chainTransfer(t, f, f.holdingAccountID, b, *buy.LotID, "2026-06-01")
	chainTransfer(t, f, b, f.holdingAccountID, toB.DestinationLotIDs[0], "2026-06-15")

	closure, err := f.investmentService.repository.InvestmentReplayClosure(ctx, BookID,
		[]db.InvestmentReplayPosition{replayPosition(f, f.holdingAccountID, "2026-05-01")})
	require.NoError(t, err)
	assert.Equal(t, []db.InvestmentReplayPosition{
		replayPosition(f, f.holdingAccountID, "2026-05-01"),
		replayPosition(f, b, "2026-06-01"),
	}, closure, "the return leg revisits A at a later date and terminates")

	// Seeded inside the cycle, A is affected only from the return transfer;
	// its earlier outbound transfer precedes the change and is not followed.
	closure, err = f.investmentService.repository.InvestmentReplayClosure(ctx, BookID,
		[]db.InvestmentReplayPosition{replayPosition(f, b, "2026-06-10")})
	require.NoError(t, err)
	assert.Equal(t, []db.InvestmentReplayPosition{
		replayPosition(f, b, "2026-06-10"),
		replayPosition(f, f.holdingAccountID, "2026-06-15"),
	}, closure)
}

func TestReplayClosureRejectsIncompleteSeed(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	_, err := f.investmentService.repository.InvestmentReplayClosure(context.Background(), BookID,
		[]db.InvestmentReplayPosition{{AccountID: f.holdingAccountID, CommodityID: f.stockCommodityID,
			CostCommodityID: f.eurCommodityID, AffectedFrom: "2026-02-30"}})
	require.ErrorIs(t, err, db.ErrInvalidDisposalParams)
}

// Until the closure replays as one stream, a correction that changes a
// carried basis anywhere along a chain is refused with the first transfer
// named, and nothing in the source, the destinations or the downstream sale
// changes — including the destination's specific-lot election.
func TestChainedTransferBasisChangeIsRefusedAtomicallyWithLineageIntact(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	b := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	c := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	toB := chainTransfer(t, f, f.holdingAccountID, b, *buy.LotID, "2026-06-01")
	toC := chainTransfer(t, f, b, c, toB.DestinationLotIDs[0], "2026-06-15")
	sale := sellInput(f, "2026-07-01", 1)
	sale.HoldingAccountID, sale.CostBasisMethod = c, "specific_lot"
	sale.LotAllocations = []InvestmentLotAllocationInput{{LotID: toC.DestinationLotIDs[0], QuantityValue: exact.New(1)}}
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)

	closure, err := f.investmentService.repository.InvestmentReplayClosure(ctx, BookID,
		[]db.InvestmentReplayPosition{replayPosition(f, f.holdingAccountID, "2026-05-01")})
	require.NoError(t, err)
	require.Len(t, closure, 3, "the correction's dependency closure reaches the specific-lot sale")

	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: buy.Transaction.ID, Reason: "broker correction",
		Replacement: InvestmentTradeInput{TransactionDate: "2026-05-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(3),
			CashAmountValue: 3300, CashAmountScale: 2},
	})
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	var dependency InvestmentBuyDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, transferOperationID(t, f, toB.Transaction.ID), dependency.OperationID,
		"the first changed carried basis is named")
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "no partial write anywhere in the chain")
}

func transferOperationID(t *testing.T, f *investmentsTestFixture, transactionID int64) int64 {
	t.Helper()
	var id int64
	require.NoError(t, f.database.QueryRow(`SELECT link.operation_id FROM investment_operation_journal_links link
		JOIN transaction_versions version ON version.id = link.transaction_version_id
		WHERE version.transaction_id = ? AND link.role = 'primary'`, transactionID).Scan(&id))
	return id
}
