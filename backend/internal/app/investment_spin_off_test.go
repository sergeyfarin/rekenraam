package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"math/big"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// Spin-off (#180): the parent lots keep their units and give up an exact
// fraction of their basis to new lots of the distributed instrument, with no
// cash and nothing realized.

func spinOffInput(f *investmentsTestFixture, newCommodityID int64, date string, numerator, denominator int64, fraction string, fractionScale int) SpinOffInput {
	return SpinOffInput{
		OwnerUserID: f.ownerUserID, EffectiveOn: date, HoldingAccountID: f.holdingAccountID,
		CommodityID: f.stockCommodityID, DestinationCommodityID: newCommodityID,
		RatioNumerator: numerator, RatioDenominator: denominator,
		BasisFractionValue: exact.Coefficient(fraction), BasisFractionScale: fractionScale,
		SourceEvidenceJSON: `{"notice":"issuer allocation","ex_date":"2026-05-29"}`, Memo: "spin-off",
	}
}

// lotBasis reads one lot's projected quantity and remaining basis.
func lotBasis(t *testing.T, f *investmentsTestFixture, lotID int64) (string, *exact.ScaledInt, string, string) {
	t.Helper()
	var quantity, knowledge, status string
	var basis *string
	var basisScale *int
	require.NoError(t, f.database.QueryRow(`SELECT remaining_quantity_value, remaining_cost_basis_value,
		remaining_cost_basis_scale, basis_knowledge, status FROM current_investment_lots WHERE id = ?`, lotID).Scan(
		&quantity, &basis, &basisScale, &knowledge, &status))
	if basis == nil {
		return quantity, nil, knowledge, status
	}
	return quantity, exact.ScaledIntFromCoefficient(exact.Coefficient(*basis), *basisScale), knowledge, status
}

func requireSpinOffSelfCheckPasses(t *testing.T, f *investmentsTestFixture) {
	t.Helper()
	run, err := selfCheckOver(t, f.database).RunSelfCheck(context.Background(), "manual")
	require.NoError(t, err)
	for _, check := range []string{CheckInvestmentFoundation, CheckInvestmentReplay, CheckLotReconciliation} {
		assert.Equal(t, SelfCheckPassed, resultFor(t, run, check).Status, "%s: %s", check, resultFor(t, run, check).Summary)
	}
}

func TestSpinOffDividesBasisKeepsUnitsAndRealizesNothing(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	first := buyOn(t, f, "2026-02-01", 60, 60000)
	second := buyOn(t, f, "2026-03-01", 40, 40000)
	var auditsBefore int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsBefore))

	// One new unit per two parent units; 20 % of the basis moves.
	input := spinOffInput(f, newID, "2026-06-01", 1, 2, "20", 2)
	preview, err := f.investmentService.PreviewSpinOff(ctx, input)
	require.NoError(t, err)
	result, err := f.investmentService.SpinOff(ctx, input)
	require.NoError(t, err)
	// The fraction is stored in lowest decimal terms.
	assert.Equal(t, [2]any{exact.Coefficient("2"), 1}, [2]any{result.Plan.BasisFractionValue, result.Plan.BasisFractionScale})

	require.Len(t, preview.Plan.Links, 2)
	require.Len(t, result.Plan.Links, 2)
	for index := range result.Plan.Links {
		committed := result.Plan.Links[index]
		assert.NotZero(t, committed.DestinationLotID)
		committed.DestinationLotID = 0
		assert.Equal(t, preview.Plan.Links[index], committed)
	}

	// Two legs in the new instrument only.
	require.Len(t, result.Transaction.JournalEntries, 1)
	postings := result.Transaction.JournalEntries[0].Postings
	require.Len(t, postings, 2)
	for _, posting := range postings {
		assert.Equal(t, newID, posting.CommodityID)
		if posting.AccountID == f.holdingAccountID {
			assert.Equal(t, "50", posting.QuantityValue.String())
		} else {
			assert.Equal(t, "-50", posting.QuantityValue.String())
		}
	}
	var auditsAfter int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsAfter))
	assert.Equal(t, auditsBefore+1, auditsAfter)

	expected := map[int64]struct {
		parentQuantity, newQuantity string
		parentBasis, newBasis       int64
		original                    string
	}{
		*first.LotID:  {"60", "30", 48000, 12000, "2026-02-01"},
		*second.LotID: {"40", "20", 32000, 8000, "2026-03-01"},
	}
	for _, link := range result.Plan.Links {
		want := expected[link.SourceLotID]
		quantity, basis, knowledge, status := lotBasis(t, f, link.SourceLotID)
		assert.Equal(t, [3]string{want.parentQuantity, db.InvestmentBasisKnown, "open"}, [3]string{quantity, knowledge, status})
		assert.Zero(t, basis.Cmp(exact.ScaledIntFromInt64(want.parentBasis, 2)), "parent basis %s", basis)
		quantity, basis, _, _ = lotBasis(t, f, link.DestinationLotID)
		assert.Equal(t, want.newQuantity, quantity)
		assert.Zero(t, basis.Cmp(exact.ScaledIntFromInt64(want.newBasis, 2)), "new basis %s", basis)
		assert.Equal(t, want.original, link.OriginalAcquiredOn)
	}
	require.Len(t, result.Plan.BasisTotals, 1)
	total := result.Plan.BasisTotals[0]
	assert.Zero(t, exact.ScaledIntFromCoefficient(total.AllocatedBasisValue, total.AllocatedBasisScale).Cmp(exact.ScaledIntFromInt64(20000, 2)))
	assert.Zero(t, exact.ScaledIntFromCoefficient(total.RemainingBasisValue, total.RemainingBasisScale).Cmp(exact.ScaledIntFromInt64(80000, 2)))
	assert.Equal(t, [2]string{"100", "50"}, [2]string{total.SourceQuantityValue.String(), total.DestinationQuantityValue.String()})

	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	assert.Empty(t, gains, "a spin-off realizes nothing")

	// Independent example: the parent sold for 900.00 gains 100.00 against its
	// remaining 800.00, the new units sold for 250.00 gain 50.00 against 200.00,
	// and the cost-currency clearing residual is the negative of their sum.
	sale := sellInput(f, "2026-07-01", 100)
	sale.CashAmountValue = 90000
	_, err = f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	sale = sellInput(f, "2026-07-02", 50)
	sale.CommodityID, sale.CashAmountValue = newID, 25000
	_, err = f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	gains, err = f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	byInstrument := map[int64]*exact.ScaledInt{f.stockCommodityID: exact.NewScaledInt(), newID: exact.NewScaledInt()}
	for _, gain := range gains {
		byInstrument[gain.CommodityID].AddInt64(gain.RealizedGainValue, gain.RealizedGainScale)
	}
	assert.Zero(t, byInstrument[f.stockCommodityID].Cmp(exact.ScaledIntFromInt64(10000, 2)), "parent gain %s", byInstrument[f.stockCommodityID])
	assert.Zero(t, byInstrument[newID].Cmp(exact.ScaledIntFromInt64(5000, 2)), "new gain %s", byInstrument[newID])
	assert.Zero(t, commodityTradingBalance(t, f, f.eurCommodityID).Cmp(exact.ScaledIntFromInt64(-15000, 2)))
	assert.Zero(t, commodityTradingBalance(t, f, f.stockCommodityID).Sign())
	assert.Zero(t, commodityTradingBalance(t, f, newID).Sign())
	requireSpinOffSelfCheckPasses(t, f)
}

// The allocation is truncated at the position's allocation scale and the
// parent keeps the remainder: per lot, reduction and new basis are one amount.
func TestSpinOffTruncatesTheAllocationAndConservesEachLot(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	lots := []int64{*buyOn(t, f, "2026-02-01", 7, 10001).LotID, *buyOn(t, f, "2026-02-02", 3, 333).LotID}
	result, err := f.investmentService.SpinOff(ctx, spinOffInput(f, newID, "2026-06-01", 1, 1, "3333", 4))
	require.NoError(t, err)
	require.Len(t, result.Plan.Links, 2)
	original := map[int64]int64{lots[0]: 10001, lots[1]: 333}
	for _, link := range result.Plan.Links {
		_, parent, _, _ := lotBasis(t, f, link.SourceLotID)
		_, child, _, _ := lotBasis(t, f, link.DestinationLotID)
		sum := exact.ScaledIntFromBig(parent.BigInt(), parent.Scale())
		sum.AddScaled(child)
		assert.Zero(t, sum.Cmp(exact.ScaledIntFromInt64(original[link.SourceLotID], 2)), "lot %d conserves basis", link.SourceLotID)
		// child ≤ basis × fraction < child + one unit at the allocation scale.
		exactShare := exact.ScaledIntFromInt64(original[link.SourceLotID]*3333, 6)
		assert.LessOrEqual(t, child.Cmp(exactShare), 0)
		step := exact.ScaledIntFromBig(child.BigInt(), child.Scale())
		step.Add(big.NewInt(1), child.Scale())
		assert.Positive(t, step.Cmp(exactShare), "truncated, not rounded down further")
	}
	requireSpinOffSelfCheckPasses(t, f)
}

func TestSpinOffCarriesUnknownBasisAndResolvesThroughReplay(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	inbound, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-05-01", "2020-03-01"))
	require.NoError(t, err)
	result, err := f.investmentService.SpinOff(ctx, spinOffInput(f, newID, "2026-06-01", 3, 1, "25", 2))
	require.NoError(t, err)
	require.Len(t, result.Plan.Links, 1)
	link := result.Plan.Links[0]
	assert.Equal(t, db.InvestmentBasisUnknown, link.BasisKnowledge)
	assert.Equal(t, "2020-03-01", link.OriginalAcquiredOn)
	require.Len(t, result.Plan.BasisTotals, 1)
	assert.Equal(t, [2]any{db.InvestmentBasisUnknown, 1}, [2]any{result.Plan.BasisTotals[0].BasisKnowledge, result.Plan.BasisTotals[0].UnknownLots})
	quantity, basis, knowledge, _ := lotBasis(t, f, link.DestinationLotID)
	assert.Equal(t, [2]string{"6", db.InvestmentBasisUnknown}, [2]string{quantity, knowledge})
	assert.Nil(t, basis, "unknown basis is never a zero")
	_, basis, knowledge, _ = lotBasis(t, f, link.SourceLotID)
	assert.Equal(t, db.InvestmentBasisUnknown, knowledge)
	assert.Nil(t, basis)
	// Lot reconciliation is unavailable while basis is unknown; the
	// foundation and replay checks still hold.
	run, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	for _, check := range []string{CheckInvestmentFoundation, CheckInvestmentReplay} {
		assert.Equal(t, SelfCheckPassed, resultFor(t, run, check).Status, "%s: %s", check, resultFor(t, run, check).Summary)
	}

	// A sourced resolution of the parent's 80.00 replays through the spin-off:
	// 20.00 moves to the new lot and the link revision becomes known.
	resolveAcknowledged(t, f, resolveInput(f, inbound.Transaction.ID, 8000))
	_, basis, knowledge, _ = lotBasis(t, f, link.SourceLotID)
	assert.Equal(t, db.InvestmentBasisKnown, knowledge)
	assert.Zero(t, basis.Cmp(exact.ScaledIntFromInt64(6000, 2)), "parent basis %s", basis)
	_, basis, knowledge, _ = lotBasis(t, f, link.DestinationLotID)
	assert.Equal(t, db.InvestmentBasisKnown, knowledge)
	assert.Zero(t, basis.Cmp(exact.ScaledIntFromInt64(2000, 2)), "new basis %s", basis)
	var revisionKnowledge string
	require.NoError(t, f.database.QueryRow(`SELECT basis_knowledge FROM latest_investment_transfer_link_revisions
		WHERE operation_id = (SELECT operation_id FROM investment_transfer_facts WHERE transfer_kind = 'spin_off')`).Scan(&revisionKnowledge))
	assert.Equal(t, db.InvestmentBasisKnown, revisionKnowledge)
	requireSpinOffSelfCheckPasses(t, f)
}

// A corrected acquisition before the spin-off changes what each side gets;
// the link appends a revision and the new lot replays with it.
func TestSpinOffUpstreamBuyReplacementRevisesBothSides(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	bought := buyOn(t, f, "2026-02-01", 10, 10000)
	input := spinOffInput(f, newID, "2026-06-01", 1, 1, "4", 1)
	input.DestinationHoldingAccountID = destinationID
	spun, err := f.investmentService.SpinOff(ctx, input)
	require.NoError(t, err)
	require.Len(t, spun.Plan.Links, 1)
	var account int64
	require.NoError(t, f.database.QueryRow(`SELECT account_id FROM investment_lots WHERE id = ?`,
		spun.Plan.Links[0].DestinationLotID).Scan(&account))
	assert.Equal(t, destinationID, account)

	replaced, err := f.investmentService.ReplaceBuy(ctx, buyReplacementForNetting(f, bought.Transaction.ID, "2026-02-01", 10, 15000))
	require.NoError(t, err)
	require.NotNil(t, replaced)
	var parentLotID int64
	require.NoError(t, f.database.QueryRow(`SELECT id FROM current_investment_lots WHERE commodity_id = ? AND status = 'open'`,
		f.stockCommodityID).Scan(&parentLotID))
	_, parent, _, _ := lotBasis(t, f, parentLotID)
	assert.Zero(t, parent.Cmp(exact.ScaledIntFromInt64(9000, 2)), "parent basis %s", parent)
	quantity, child, _, _ := lotBasis(t, f, spun.Plan.Links[0].DestinationLotID)
	assert.Equal(t, "10", quantity)
	assert.Zero(t, child.Cmp(exact.ScaledIntFromInt64(6000, 2)), "new basis %s", child)
	var revisions int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_link_revisions
		WHERE operation_id = (SELECT operation_id FROM investment_transfer_facts WHERE transfer_kind = 'spin_off')`).Scan(&revisions))
	assert.Equal(t, 1, revisions)

	sale := sellInput(f, "2026-07-01", 10)
	sale.CommodityID, sale.HoldingAccountID, sale.CashAmountValue = newID, destinationID, 7000
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	require.Len(t, sold.Allocations, 1)
	assert.Zero(t, exact.ScaledIntFromInt64(sold.Allocations[0].CostBasisValue, sold.Allocations[0].CostBasisScale).Cmp(
		exact.ScaledIntFromInt64(6000, 2)))
	requireSpinOffSelfCheckPasses(t, f)
}

// History that changes which lots or units the spin-off entitled refuses with
// the spin-off named and writes nothing.
func TestSpinOffRefusesUpstreamChangesItCannotExpress(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		change func(t *testing.T, f *investmentsTestFixture) error
	}{
		{"a backdated purchase leaves an unentitled lot", func(t *testing.T, f *investmentsTestFixture) error {
			_, err := f.investmentService.Buy(context.Background(), tradeOn(f, f.holdingAccountID, "2026-03-01", 5, 5000))
			return err
		}},
		{"a backdated sale changes an entitled lot's units", func(t *testing.T, f *investmentsTestFixture) error {
			_, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-03-01", 4))
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			newID := seedTestSecurityCommodity(t, f, "SPINCO")
			buyOn(t, f, "2026-02-01", 10, 10000)
			_, err := f.investmentService.SpinOff(context.Background(), spinOffInput(f, newID, "2026-06-01", 1, 1, "5", 1))
			require.NoError(t, err)
			var spinOffOperationID int64
			require.NoError(t, f.database.QueryRow(`SELECT operation_id FROM investment_transfer_facts
				WHERE transfer_kind = 'spin_off'`).Scan(&spinOffOperationID))
			before := shareExchangeWriteCounts(t, f.database)
			err = test.change(t, f)
			require.Error(t, err)
			var buy InvestmentBuyDependencyError
			var sale InvestmentSaleDependencyError
			var transfer InvestmentTransferDependencyError
			switch {
			case errors.As(err, &buy):
				assert.Equal(t, spinOffOperationID, buy.OperationID)
			case errors.As(err, &sale):
				assert.Equal(t, spinOffOperationID, sale.OperationID)
			case errors.As(err, &transfer):
				assert.Equal(t, spinOffOperationID, transfer.OperationID)
			default:
				t.Fatalf("error %v does not name the spin-off", err)
			}
			assert.Equal(t, before, shareExchangeWriteCounts(t, f.database))
		})
	}
}

func TestSpinOffRefusalsWriteNothing(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, f *investmentsTestFixture, newID int64) SpinOffInput
		want  func(t *testing.T, err error)
	}{
		{"no holdings on the date", func(t *testing.T, f *investmentsTestFixture, newID int64) SpinOffInput {
			buyOn(t, f, "2026-07-01", 10, 10000)
			return spinOffInput(f, newID, "2026-06-01", 1, 1, "1", 1)
		}, func(t *testing.T, err error) { assert.ErrorIs(t, err, ErrSpinOffNoHoldings) }},
		{"same instrument", func(t *testing.T, f *investmentsTestFixture, _ int64) SpinOffInput {
			buyOn(t, f, "2026-02-01", 10, 10000)
			return spinOffInput(f, f.stockCommodityID, "2026-06-01", 1, 1, "1", 1)
		}, func(t *testing.T, err error) { assert.ErrorAs(t, err, &ValidationError{}) }},
		{"currency instrument", func(t *testing.T, f *investmentsTestFixture, _ int64) SpinOffInput {
			buyOn(t, f, "2026-02-01", 10, 10000)
			return spinOffInput(f, f.eurCommodityID, "2026-06-01", 1, 1, "1", 1)
		}, func(t *testing.T, err error) { assert.ErrorAs(t, err, &ValidationError{}) }},
		{"zero fraction", func(t *testing.T, f *investmentsTestFixture, newID int64) SpinOffInput {
			buyOn(t, f, "2026-02-01", 10, 10000)
			return spinOffInput(f, newID, "2026-06-01", 1, 1, "0", 1)
		}, func(t *testing.T, err error) { assert.ErrorAs(t, err, &ValidationError{}) }},
		{"whole basis fraction", func(t *testing.T, f *investmentsTestFixture, newID int64) SpinOffInput {
			buyOn(t, f, "2026-02-01", 10, 10000)
			return spinOffInput(f, newID, "2026-06-01", 1, 1, "100", 2)
		}, func(t *testing.T, err error) { assert.ErrorAs(t, err, &ValidationError{}) }},
		{"fraction past twelve places", func(t *testing.T, f *investmentsTestFixture, newID int64) SpinOffInput {
			buyOn(t, f, "2026-02-01", 10, 10000)
			return spinOffInput(f, newID, "2026-06-01", 1, 1, "1", 13)
		}, func(t *testing.T, err error) { assert.ErrorAs(t, err, &ValidationError{}) }},
		{"unrepresentable new quantity", func(t *testing.T, f *investmentsTestFixture, newID int64) SpinOffInput {
			buyOn(t, f, "2026-02-01", 1, 10000)
			return spinOffInput(f, newID, "2026-06-01", 1, 3, "1", 1)
		}, func(t *testing.T, err error) { assert.ErrorIs(t, err, ErrSpinOffFraction) }},
		{"dated behind a later sale", func(t *testing.T, f *investmentsTestFixture, newID int64) SpinOffInput {
			buyOn(t, f, "2026-02-01", 10, 10000)
			_, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-07-01", 2))
			require.NoError(t, err)
			return spinOffInput(f, newID, "2026-06-01", 1, 1, "1", 1)
		}, func(t *testing.T, err error) { assert.ErrorIs(t, err, db.ErrOutOfOrderPositionEvent) }},
		{"open short of the parent", func(t *testing.T, f *investmentsTestFixture, newID int64) SpinOffInput {
			shortSaleOn(t, f, "2026-02-01", 5, 5000)
			return spinOffInput(f, newID, "2026-06-01", 1, 1, "1", 1)
		}, func(t *testing.T, err error) { assert.ErrorIs(t, err, ErrInvestmentPositionSideConflict) }},
		{"zero ratio", func(t *testing.T, f *investmentsTestFixture, newID int64) SpinOffInput {
			buyOn(t, f, "2026-02-01", 10, 10000)
			return spinOffInput(f, newID, "2026-06-01", 0, 1, "1", 1)
		}, func(t *testing.T, err error) { assert.ErrorAs(t, err, &ValidationError{}) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			newID := seedTestSecurityCommodity(t, f, "SPINCO")
			input := test.setup(t, f, newID)
			before := shareExchangeWriteCounts(t, f.database)
			_, err := f.investmentService.PreviewSpinOff(context.Background(), input)
			require.Error(t, err)
			test.want(t, err)
			_, err = f.investmentService.SpinOff(context.Background(), input)
			require.Error(t, err)
			test.want(t, err)
			assert.Equal(t, before, shareExchangeWriteCounts(t, f.database))
		})
	}
}

func TestSpinOffPreviewWritesNothing(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	before := shareExchangeWriteCounts(t, f.database)
	preview, err := f.investmentService.PreviewSpinOff(context.Background(), spinOffInput(f, newID, "2026-06-01", 2, 1, "1", 1))
	require.NoError(t, err)
	assert.Equal(t, "20", preview.Plan.DestinationQuantityValue.String())
	assert.Equal(t, before, shareExchangeWriteCounts(t, f.database))
}

func TestSpinOffSelfCheckDetectsADamagedRatio(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	_, err := f.investmentService.SpinOff(ctx, spinOffInput(f, newID, "2026-06-01", 1, 2, "1", 1))
	require.NoError(t, err)
	_, err = f.database.Exec(`DROP TRIGGER investment_transfer_facts_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_transfer_facts SET ratio_numerator = 3 WHERE transfer_kind = 'spin_off'`)
	require.NoError(t, err)
	run, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	result := resultFor(t, run, CheckInvestmentFoundation)
	assert.Equal(t, SelfCheckFailed, result.Status)
	assert.Contains(t, result.Summary, "spin-offs")
}

// A parent reduction that no longer equals the new lot's opening basis is
// damage: basis would be created or lost.
func TestSpinOffSelfCheckDetectsAnUnconservedReduction(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	result, err := f.investmentService.SpinOff(ctx, spinOffInput(f, newID, "2026-06-01", 1, 1, "1", 1))
	require.NoError(t, err)
	_, err = f.database.Exec(`DROP TRIGGER IF EXISTS investment_lot_events_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_lot_events SET cost_basis_value = '-999'
		WHERE lot_id = ? AND event_kind = 'basis_reduction'`, result.Plan.Links[0].SourceLotID)
	require.NoError(t, err)
	run, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	check := resultFor(t, run, CheckInvestmentFoundation)
	assert.Equal(t, SelfCheckFailed, check.Status)
	assert.Contains(t, check.Summary, "spin-off missing its fact or linked basis effects")
}

func TestSpinOffExportCarriesTheBasisFraction(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	_, err := f.investmentService.SpinOff(ctx, spinOffInput(f, newID, "2026-06-01", 1, 2, "141", 3))
	require.NoError(t, err)
	var out bytes.Buffer
	require.NoError(t, NewExportService(db.NewExportRepository(f.database)).WriteBundle(ctx, &out, ExportFilter{}))
	archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	require.NoError(t, err)
	var records [][]string
	for _, entry := range archive.File {
		if entry.Name == "investment-transfer-facts.csv" {
			reader, err := entry.Open()
			require.NoError(t, err)
			records, err = csv.NewReader(reader).ReadAll()
			require.NoError(t, err)
		}
	}
	require.Len(t, records, 2)
	row := map[string]string{}
	for index, column := range records[0] {
		row[column] = records[1][index]
	}
	assert.Equal(t, "spin_off", row["transfer_kind"])
	assert.Equal(t, strconv.FormatInt(newID, 10), row["destination_commodity_id"])
	assert.Equal(t, [4]string{"1", "2", "141", "3"}, [4]string{row["ratio_numerator"], row["ratio_denominator"],
		row["basis_fraction_value"], row["basis_fraction_scale"]})
}
