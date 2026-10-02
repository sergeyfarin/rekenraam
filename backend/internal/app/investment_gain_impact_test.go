package app

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func backdatedBuy(f *investmentsTestFixture, date string, quantity, cash int64) InvestmentTradeInput {
	return InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: date,
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(quantity), CashAmountValue: cash, CashAmountScale: 2}
}

// acknowledgedBuy previews a buy and commits it with the returned gain-impact
// acknowledgement, as the buy form does after the user confirms.
func acknowledgedBuy(t *testing.T, f *investmentsTestFixture, input InvestmentTradeInput) InvestmentTradeResult {
	t.Helper()
	input.GainImpactAcknowledgement = previewBuyGainImpact(t, f, input).Acknowledgement
	result, err := f.investmentService.Buy(context.Background(), input)
	require.NoError(t, err)
	return result
}

func requireScaled(t *testing.T, want int64, wantScale int, got *exact.ScaledInt, label string) {
	t.Helper()
	require.NotNil(t, got, label)
	require.Zerof(t, got.Cmp(exact.ScaledIntFromInt64(want, wantScale)), "%s: want %s, got %s",
		label, exact.ScaledIntFromInt64(want, wantScale), got)
}

func previewBuyGainImpact(t *testing.T, f *investmentsTestFixture, input InvestmentTradeInput) db.InvestmentGainImpact {
	t.Helper()
	impact, err := f.investmentService.TradeReconciliationImpact(context.Background(), InvestmentImpactBuy, input)
	require.NoError(t, err)
	require.NotNil(t, impact.GainImpact, "a manual buy preview always reports its gain impact")
	return *impact.GainImpact
}

// The issue's named FIFO case: 10 shares bought for 200.00, 5 sold for 150.00
// (basis 100.00, gain 50.00); an earlier 10-share buy for 20.00 makes FIFO
// consume the cheaper lot (basis 10.00, gain 140.00).
func TestBuyGainImpactDisclosesFIFORevisionAndRequiresExactAcknowledgement(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-02-01", 10, 20000)
	sale := sellInput(f, "2026-03-01", 5)
	sale.CashAmountValue = 15000
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	assertMoneyValue(t, 5000, 2, gains[0].RealizedGainValue, gains[0].RealizedGainScale, "gain before replay")

	input := backdatedBuy(f, "2026-01-01", 10, 2000)
	before := buyReplacementPreviewSnapshot(t, f.database)
	impact := previewBuyGainImpact(t, f, input)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "preview leaves every durable row unchanged")
	require.Equal(t, impact, previewBuyGainImpact(t, f, input), "the same state previews the same change set and token")
	require.Len(t, impact.Changes, 1)
	change := impact.Changes[0]
	require.Equal(t, db.GainImpactRevised, change.Kind)
	require.Equal(t, sold.Transaction.ID, change.TransactionID)
	var decisionID, operationID int64
	require.NoError(t, f.database.QueryRow(`SELECT id, operation_id FROM investment_disposal_decisions WHERE transaction_id = ?`,
		sold.Transaction.ID).Scan(&decisionID, &operationID))
	require.Equal(t, decisionID, change.DecisionID)
	require.Equal(t, db.InvestmentGainIdentity{RootOperationID: operationID, DecisionSeq: 1}, change.Identity)
	require.Equal(t, "2026-03-01", change.Before.DisposalDate)
	require.Equal(t, f.eurCommodityID, change.Before.CostCommodityID)
	require.Equal(t, "fifo", change.Before.CostBasisMethod)
	requireScaled(t, 10000, 2, change.Before.DisposedBasis, "basis before")
	requireScaled(t, 15000, 2, change.Before.Proceeds, "proceeds before")
	requireScaled(t, 5000, 2, change.Before.Gain, "gain before")
	require.NotNil(t, change.After)
	require.Equal(t, db.InvestmentBasisKnown, change.After.BasisKnowledge)
	requireScaled(t, 1000, 2, change.After.DisposedBasis, "basis after")
	requireScaled(t, 15000, 2, change.After.Proceeds, "proceeds after")
	requireScaled(t, 14000, 2, change.After.Gain, "gain after")
	require.Len(t, impact.Acknowledgement, 64)

	_, err = f.investmentService.Buy(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	input.GainImpactAcknowledgement = "0" + impact.Acknowledgement[1:]
	if input.GainImpactAcknowledgement == impact.Acknowledgement {
		input.GainImpactAcknowledgement = "1" + impact.Acknowledgement[1:]
	}
	_, err = f.investmentService.Buy(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementStale)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))

	input.GainImpactAcknowledgement = impact.Acknowledgement
	_, err = f.investmentService.Buy(ctx, input)
	require.NoError(t, err)
	gains, err = f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 1)
	assertMoneyValue(t, 14000, 2, gains[0].RealizedGainValue, gains[0].RealizedGainScale, "gain after acknowledged replay")
}

func TestBuyGainImpactIsEmptyWhenBasisIsUnchanged(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-02-01", 10, 20000)
	sale := sellInput(f, "2026-03-01", 5)
	sale.CostBasisMethod = "lifo"
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)

	// A buy after every disposal does not replay at all.
	later := backdatedBuy(f, "2026-04-01", 1, 100)
	require.Empty(t, previewBuyGainImpact(t, f, later).Changes)
	_, err = f.investmentService.Buy(ctx, later)
	require.NoError(t, err)

	// LIFO still selects the February lot after an earlier January buy:
	// replay runs, but the committed disposal's result is identical.
	earlier := backdatedBuy(f, "2026-01-01", 10, 2000)
	impact := previewBuyGainImpact(t, f, earlier)
	require.Empty(t, impact.Changes)
	require.Empty(t, impact.Acknowledgement)
	_, err = f.investmentService.Buy(ctx, earlier)
	require.NoError(t, err, "an empty change set needs no acknowledgement")
}

func TestBuyGainImpactIgnoresReselectedAllocationsWithEqualGain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	february := buyOn(t, f, "2026-02-01", 10, 20000)
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-03-01", 5))
	require.NoError(t, err)

	// Same unit cost: FIFO now consumes the January lot, with identical basis.
	earlier := backdatedBuy(f, "2026-01-01", 10, 20000)
	require.Empty(t, previewBuyGainImpact(t, f, earlier).Changes)
	bought, err := f.investmentService.Buy(ctx, earlier)
	require.NoError(t, err)
	var lotID int64
	require.NoError(t, f.database.QueryRow(`SELECT a.lot_id FROM investment_disposal_revision_allocations a
		JOIN investment_disposal_revisions r ON r.id = a.revision_id
		JOIN investment_disposal_decisions d ON d.id = r.decision_id
		WHERE d.transaction_id = ? ORDER BY r.revision_seq DESC LIMIT 1`, sold.Transaction.ID).Scan(&lotID))
	require.Equal(t, *bought.LotID, lotID, "the allocation moved to the earlier lot")
	require.NotEqual(t, *february.LotID, lotID)
}

func TestBuyGainImpactKeepsNegativeProceedsSigned(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-02-01", 2, 4000)
	_, err := f.investmentService.Sell(ctx, negativeProceedsSale(f, "2026-03-01"))
	require.NoError(t, err)

	impact := previewBuyGainImpact(t, f, backdatedBuy(f, "2026-01-01", 1, 500))
	require.Len(t, impact.Changes, 1)
	change := impact.Changes[0]
	requireScaled(t, -50, 2, change.Before.Proceeds, "negative proceeds before")
	requireScaled(t, 2000, 2, change.Before.DisposedBasis, "basis before")
	requireScaled(t, -2050, 2, change.Before.Gain, "loss before")
	requireScaled(t, -50, 2, change.After.Proceeds, "negative proceeds after")
	requireScaled(t, 500, 2, change.After.DisposedBasis, "basis after")
	requireScaled(t, -550, 2, change.After.Gain, "loss after")
}

func TestBuyGainImpactDisclosesEveryRevisedDisposalAgainstItsLatestRevision(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-03-01", 10, 10000) // 10.00 per share
	first := sellInput(f, "2026-04-01", 2)
	first.CashAmountValue = 3000
	_, err := f.investmentService.Sell(ctx, first)
	require.NoError(t, err)
	second := sellInput(f, "2026-05-01", 2)
	second.CashAmountValue = 3000
	_, err = f.investmentService.Sell(ctx, second)
	require.NoError(t, err)

	// Two shares at 5.00 each: the April sale now consumes them.
	february := backdatedBuy(f, "2026-02-01", 2, 1000)
	impact := previewBuyGainImpact(t, f, february)
	require.Len(t, impact.Changes, 1, "the May sale still consumes the March lot")
	requireScaled(t, 2000, 2, impact.Changes[0].Before.DisposedBasis, "April basis before")
	requireScaled(t, 1000, 2, impact.Changes[0].After.DisposedBasis, "April basis after")
	february.GainImpactAcknowledgement = impact.Acknowledgement
	_, err = f.investmentService.Buy(ctx, february)
	require.NoError(t, err)

	// Four shares at 1.00 each: both sales move, and the April sale is
	// compared against its first revision rather than the original snapshot.
	january := backdatedBuy(f, "2026-01-01", 4, 400)
	impact = previewBuyGainImpact(t, f, january)
	require.Len(t, impact.Changes, 2)
	require.Less(t, impact.Changes[0].Identity.RootOperationID, impact.Changes[1].Identity.RootOperationID)
	require.Equal(t, "2026-04-01", impact.Changes[0].Before.DisposalDate)
	requireScaled(t, 1000, 2, impact.Changes[0].Before.DisposedBasis, "April basis from the latest revision")
	requireScaled(t, 200, 2, impact.Changes[0].After.DisposedBasis, "April basis after second replay")
	requireScaled(t, 2800, 2, impact.Changes[0].After.Gain, "April gain after second replay")
	require.Equal(t, "2026-05-01", impact.Changes[1].Before.DisposalDate)
	requireScaled(t, 2000, 2, impact.Changes[1].Before.DisposedBasis, "May basis before")
	requireScaled(t, 200, 2, impact.Changes[1].After.DisposedBasis, "May basis after")
	january.GainImpactAcknowledgement = impact.Acknowledgement
	_, err = f.investmentService.Buy(ctx, january)
	require.NoError(t, err)
}

func TestBuyGainImpactRejectsAcknowledgementOfAnEarlierChangeSet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-02-01", 10, 20000)
	sale := sellInput(f, "2026-03-01", 5)
	sale.CashAmountValue = 15000
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)

	pending := backdatedBuy(f, "2026-01-01", 10, 2000)
	stale := previewBuyGainImpact(t, f, pending)
	require.Len(t, stale.Changes, 1)
	requireScaled(t, 10000, 2, stale.Changes[0].Before.DisposedBasis, "previewed basis before")
	pending.GainImpactAcknowledgement = stale.Acknowledgement

	// Another acknowledged backdated buy changes the committed basis first.
	other := backdatedBuy(f, "2026-01-15", 10, 1000)
	other.GainImpactAcknowledgement = previewBuyGainImpact(t, f, other).Acknowledgement
	_, err = f.investmentService.Buy(ctx, other)
	require.NoError(t, err)

	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.Buy(ctx, pending)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementStale)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))

	// The pending buy still changes the sale, now from 5.00 to 10.00, and
	// only an acknowledgement of that current change set commits it.
	fresh := previewBuyGainImpact(t, f, pending)
	require.Len(t, fresh.Changes, 1)
	requireScaled(t, 500, 2, fresh.Changes[0].Before.DisposedBasis, "current basis before")
	requireScaled(t, 1000, 2, fresh.Changes[0].After.DisposedBasis, "current basis after")
	require.NotEqual(t, stale.Acknowledgement, fresh.Acknowledgement)
	pending.GainImpactAcknowledgement = fresh.Acknowledgement
	_, err = f.investmentService.Buy(ctx, pending)
	require.NoError(t, err)
}

func TestBuyGainImpactAcknowledgementRollsBackOnLateFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-02-01", 10, 20000)
	sale := sellInput(f, "2026-03-01", 5)
	sale.CashAmountValue = 15000
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	checkpoint := reconcileCash(t, f, "2026-04-01", -50)

	input := backdatedBuy(f, "2026-01-01", 10, 2000)
	impact, err := f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactBuy, input)
	require.NoError(t, err)
	require.Len(t, impact.AffectedCheckpoints, 1)
	require.Len(t, impact.GainImpact.Changes, 1)
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement

	// The gain acknowledgement is valid, but the reconciliation guard still
	// refuses: one acknowledgement never stands in for the other.
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.Buy(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))

	// A failure after the acknowledgement is validated, journals written,
	// replay persisted and checkpoints invalidated still rolls back all of it.
	input.ReconciliationOverride = true
	transactionParams, lotParams, err := f.investmentService.prepareBuyWrite(ctx, input)
	require.NoError(t, err)
	transactionParams.GainImpact = &db.GainImpactPolicy{Acknowledgement: input.GainImpactAcknowledgement}
	lateFailure := errors.New("late source evidence failure")
	var sawRevision bool
	_, _, err = f.investmentService.repository.CreateTransactionAndLotWithPostWrite(ctx, transactionParams, lotParams,
		func(tx *sql.Tx, _ int64) error {
			var revisions int
			require.NoError(t, tx.QueryRow(`SELECT count(*) FROM investment_disposal_revisions`).Scan(&revisions))
			sawRevision = revisions > 0
			return lateFailure
		})
	require.ErrorIs(t, err, lateFailure)
	require.True(t, sawRevision, "the failure happened after the acknowledged replay was written")
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))

	result, err := f.investmentService.Buy(ctx, input)
	require.NoError(t, err)
	require.Equal(t, []int64{checkpoint}, result.Transaction.InvalidatedCheckpointIDs)
}

func TestBuyGainImpactLeavesRowsUnchangedOnImpossibleDependency(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	source := buyOn(t, f, "2026-02-01", 3, 1000)
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-03-01", 2))
	require.NoError(t, err)
	destination := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	_, err = f.investmentService.InternalTransfer(ctx, internalTransferFromLot(f, destination, *source.LotID, exact.New(1), 0))
	require.NoError(t, err)

	// Replay would change the transfer's carried basis, which is refused. An
	// acknowledgement never turns that refusal into a partial commit.
	input := backdatedBuy(f, "2026-01-01", 2, 100)
	input.GainImpactAcknowledgement = "0000000000000000000000000000000000000000000000000000000000000000"
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.Buy(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

// Removals and replacements happen in correction commands that are not yet
// opted in (T-126), so these cases drive the shared snapshot comparison across
// those commands to prove superseded and reversed disposals are disclosed.
func TestGainImpactSnapshotDisclosesRemovedAndReplacedDisposals(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	repository := f.investmentService.repository
	buyOn(t, f, "2026-01-01", 10, 10000)
	first, err := f.investmentService.Sell(ctx, sellInput(f, "2026-02-01", 2))
	require.NoError(t, err)
	second, err := f.investmentService.Sell(ctx, sellInput(f, "2026-03-01", 2))
	require.NoError(t, err)

	before, err := repository.InvestmentGainSnapshot(ctx, BookID)
	require.NoError(t, err)
	replacement := sellInput(f, "2026-03-01", 2)
	replacement.CashAmountValue = 12000
	replacement.CostBasisMethod = "fifo"
	_, err = f.investmentService.ReplaceSale(ctx, ReplaceInvestmentSaleInput{OwnerUserID: f.ownerUserID,
		TransactionID: second.Transaction.ID, Reason: "broker corrected the fill", Replacement: replacement})
	require.NoError(t, err)
	after, err := repository.InvestmentGainSnapshot(ctx, BookID)
	require.NoError(t, err)
	impact := db.CompareInvestmentGainSnapshots(before, after)
	require.Len(t, impact.Changes, 1)
	replaced := impact.Changes[0]
	require.Equal(t, db.GainImpactReplaced, replaced.Kind)
	require.Equal(t, second.Transaction.ID, replaced.TransactionID)
	requireScaled(t, 10000, 2, replaced.Before.Proceeds, "proceeds before replacement")
	requireScaled(t, 12000, 2, replaced.After.Proceeds, "proceeds after replacement")
	requireScaled(t, 10000, 2, replaced.After.Gain, "gain after replacement")

	before = after
	_, err = f.investmentService.ReverseSale(ctx, ReverseInvestmentSaleInput{OwnerUserID: f.ownerUserID,
		TransactionID: first.Transaction.ID, Reason: "entered twice"})
	require.NoError(t, err)
	after, err = repository.InvestmentGainSnapshot(ctx, BookID)
	require.NoError(t, err)
	impact = db.CompareInvestmentGainSnapshots(before, after)
	require.Len(t, impact.Changes, 1, "the replayed later sale keeps its single-lot basis")
	removed := impact.Changes[0]
	require.Equal(t, db.GainImpactRemoved, removed.Kind)
	require.Equal(t, first.Transaction.ID, removed.TransactionID)
	require.Nil(t, removed.After, "a removed disposal has no effective result")
	requireScaled(t, 8000, 2, removed.Before.Gain, "removed gain")
	require.NotEqual(t, db.CompareInvestmentGainSnapshots(after, after).Acknowledgement, impact.Acknowledgement)
}

func TestBuyGainImpactLeavesRowsUnchangedWhenReplayEvidenceIsMissing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	february := buyOn(t, f, "2026-02-01", 10, 20000)
	sale := sellInput(f, "2026-03-01", 5)
	sale.CashAmountValue = 15000
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	input := backdatedBuy(f, "2026-01-01", 10, 2000)
	input.GainImpactAcknowledgement = previewBuyGainImpact(t, f, input).Acknowledgement

	// Simulate damaged history, which the schema normally forbids: the
	// February lot loses its immutable opening fact. Replay cannot prove the sale's inputs, so even a valid
	// acknowledgement must not let any part of the buy survive.
	_, err = f.database.Exec(`DROP TRIGGER investment_lot_facts_no_delete`)
	require.NoError(t, err)
	_, err = f.database.Exec(`DELETE FROM investment_lot_facts WHERE lot_id = ?`, *february.LotID)
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactBuy, input)
	require.Error(t, err)
	_, err = f.investmentService.Buy(ctx, input)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrGainImpactAcknowledgementStale)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}
