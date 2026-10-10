package app

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// Share exchange (#177): the whole holding of one instrument becomes another
// at an exact ratio, each lot carrying its basis and original date, with no
// cash and nothing realized.

func shareExchangeInput(f *investmentsTestFixture, newCommodityID int64, date string, numerator, denominator int64) ShareExchangeInput {
	return ShareExchangeInput{
		OwnerUserID: f.ownerUserID, EffectiveOn: date, HoldingAccountID: f.holdingAccountID,
		CommodityID: f.stockCommodityID, DestinationCommodityID: newCommodityID,
		RatioNumerator: numerator, RatioDenominator: denominator,
		SourceEvidenceJSON: `{"notice":"merger terms"}`, Memo: "merger",
	}
}

// shareExchangeWriteCounts are the durable rows a refused exchange must not add.
func shareExchangeWriteCounts(t *testing.T, database *sql.DB) [5]int {
	t.Helper()
	var counts [5]int
	for index, table := range []string{"transactions", "investment_operations", "investment_lots",
		"investment_lot_events", "audit_events"} {
		require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&counts[index]))
	}
	return counts
}

// commodityTradingBalance folds the trading account's postings in one
// commodity exactly.
func commodityTradingBalance(t *testing.T, f *investmentsTestFixture, commodityID int64) *exact.ScaledInt {
	t.Helper()
	tradingID, err := f.investmentService.repository.CommodityTradingAccountID(context.Background(), BookID)
	require.NoError(t, err)
	rows, err := f.database.Query(`SELECT pv.quantity_value, pv.quantity_scale FROM posting_versions pv
		JOIN current_transaction_versions v ON v.id = pv.transaction_version_id AND v.status = 'posted'
		WHERE pv.account_id = ? AND pv.commodity_id = ?`, tradingID, commodityID)
	require.NoError(t, err)
	defer rows.Close()
	total := exact.NewScaledInt()
	for rows.Next() {
		var value exact.Coefficient
		var scale int
		require.NoError(t, rows.Scan(&value, &scale))
		total.AddCoefficient(value, scale)
	}
	require.NoError(t, rows.Err())
	return total
}

func TestShareExchangeCarriesBasisAndDatesAndRealizesNothing(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	first := buyOn(t, f, "2026-02-01", 60, 60000)
	second := buyOn(t, f, "2026-03-01", 40, 40000)
	var auditsBefore int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsBefore))

	preview, err := f.investmentService.PreviewShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 3, 2))
	require.NoError(t, err)
	result, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 3, 2))
	require.NoError(t, err)

	// The preview showed exactly what the commit wrote, minus durable IDs.
	require.Len(t, preview.Plan.Links, 2)
	require.Len(t, result.Plan.Links, 2)
	for index := range result.Plan.Links {
		committed := result.Plan.Links[index]
		assert.NotZero(t, committed.DestinationLotID)
		committed.DestinationLotID = 0
		assert.Equal(t, preview.Plan.Links[index], committed)
	}

	// Four legs, each balanced in its own instrument; no cash, no cost currency.
	require.Len(t, result.Transaction.JournalEntries, 1)
	postings := result.Transaction.JournalEntries[0].Postings
	require.Len(t, postings, 4)
	legs := map[int64]*exact.ScaledInt{}
	for _, posting := range postings {
		assert.NotEqual(t, f.eurCommodityID, posting.CommodityID)
		if legs[posting.CommodityID] == nil {
			legs[posting.CommodityID] = exact.NewScaledInt()
		}
		legs[posting.CommodityID].AddCoefficient(posting.QuantityValue, posting.QuantityScale)
		if posting.AccountID == f.holdingAccountID {
			expected := map[int64]string{f.stockCommodityID: "-100", newID: "150"}[posting.CommodityID]
			assert.Equal(t, expected, posting.QuantityValue.String())
		}
	}
	assert.Zero(t, legs[f.stockCommodityID].Sign())
	assert.Zero(t, legs[newID].Sign())
	var auditsAfter int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsAfter))
	assert.Equal(t, auditsBefore+1, auditsAfter)

	// Each lot moved whole: quantity times 3/2, the same basis, its own date.
	expected := map[int64]struct {
		quantity, basis, original string
	}{*first.LotID: {"90", "60000", "2026-02-01"}, *second.LotID: {"60", "40000", "2026-03-01"}}
	for _, link := range result.Plan.Links {
		want := expected[link.SourceLotID]
		var quantity, status string
		var basis exact.Coefficient
		var basisScale int
		require.NoError(t, f.database.QueryRow(`SELECT remaining_quantity_value, remaining_cost_basis_value,
			remaining_cost_basis_scale FROM current_investment_lots WHERE id = ? AND commodity_id = ? AND opened_on = '2026-06-01'`,
			link.DestinationLotID, newID).Scan(&quantity, &basis, &basisScale))
		assert.Equal(t, want.quantity, quantity)
		assert.Zero(t, exact.ScaledIntFromCoefficient(basis, basisScale).Cmp(
			exact.ScaledIntFromCoefficient(exact.Coefficient(want.basis), 2)), "basis %s", basis)
		assert.Equal(t, want.original, link.OriginalAcquiredOn)
		require.NoError(t, f.database.QueryRow(`SELECT status FROM current_investment_lots WHERE id = ?`,
			link.SourceLotID).Scan(&status))
		assert.Equal(t, "closed", status)
	}
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	assert.Empty(t, gains, "an exchange realizes nothing")
	var methodLocks int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_position_basis_state
		WHERE commodity_id = ?`, f.stockCommodityID).Scan(&methodLocks))
	assert.Zero(t, methodLocks, "an emptied position releases its lock")

	// Independent example: 1,000.00 of basis sold for 1,200.00 gains 200.00,
	// and the cost-currency clearing residual is the negative of that gain.
	sale := sellInput(f, "2026-07-01", 150)
	sale.CommodityID, sale.CashAmountValue = newID, 120000
	_, err = f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	gains, err = f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	total := exact.NewScaledInt()
	for _, gain := range gains {
		assert.Equal(t, newID, gain.CommodityID)
		total.AddInt64(gain.RealizedGainValue, gain.RealizedGainScale)
	}
	assert.Zero(t, total.Cmp(exact.ScaledIntFromInt64(20000, 2)), "gain %s", total.Normalized())
	assert.Zero(t, commodityTradingBalance(t, f, f.eurCommodityID).Cmp(exact.ScaledIntFromInt64(-20000, 2)))
	assert.Zero(t, commodityTradingBalance(t, f, f.stockCommodityID).Sign())
	assert.Zero(t, commodityTradingBalance(t, f, newID).Sign())

	run, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	for _, check := range []string{CheckInvestmentFoundation, CheckInvestmentReplay, CheckLotReconciliation} {
		assert.Equal(t, SelfCheckPassed, resultFor(t, run, check).Status, check)
	}
}

func TestShareExchangeOrdersDestinationLotsByOriginalDate(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	buyOn(t, f, "2026-03-01", 10, 30000)
	_, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 1, 1))
	require.NoError(t, err)
	// FIFO over the new instrument takes the February units first.
	sale := sellInput(f, "2026-07-01", 10)
	sale.CommodityID, sale.CostBasisMethod = newID, "fifo"
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	require.Len(t, sold.Allocations, 1)
	assert.Zero(t, exact.ScaledIntFromInt64(sold.Allocations[0].CostBasisValue, sold.Allocations[0].CostBasisScale).Cmp(
		exact.ScaledIntFromInt64(10000, 2)))
}

func TestShareExchangeCarriesUnknownBasisAsUnknown(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	_, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-05-01", "2020-03-01"))
	require.NoError(t, err)
	result, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 2, 1))
	require.NoError(t, err)
	require.Len(t, result.Plan.Links, 1)
	link := result.Plan.Links[0]
	assert.Equal(t, db.InvestmentBasisUnknown, link.BasisKnowledge)
	assert.Equal(t, "2020-03-01", link.OriginalAcquiredOn)
	var knowledge string
	var basis sql.NullString
	var quantity string
	require.NoError(t, f.database.QueryRow(`SELECT basis_knowledge, remaining_cost_basis_value, remaining_quantity_value
		FROM current_investment_lots WHERE id = ?`, link.DestinationLotID).Scan(&knowledge, &basis, &quantity))
	assert.Equal(t, db.InvestmentBasisUnknown, knowledge)
	assert.False(t, basis.Valid, "unknown basis is never a zero")
	assert.Equal(t, "4", quantity)
	run, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	assert.Equal(t, SelfCheckPassed, resultFor(t, run, CheckInvestmentFoundation).Status)
}

func TestShareExchangeRefusalsWriteNothing(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, f *investmentsTestFixture, newID int64) ShareExchangeInput
		want  func(t *testing.T, err error)
	}{
		{"no holdings on the date", func(t *testing.T, f *investmentsTestFixture, newID int64) ShareExchangeInput {
			buyOn(t, f, "2026-07-01", 10, 10000)
			return shareExchangeInput(f, newID, "2026-06-01", 1, 1)
		}, func(t *testing.T, err error) { assert.ErrorIs(t, err, ErrShareExchangeNoHoldings) }},
		{"same instrument", func(t *testing.T, f *investmentsTestFixture, _ int64) ShareExchangeInput {
			buyOn(t, f, "2026-02-01", 10, 10000)
			return shareExchangeInput(f, f.stockCommodityID, "2026-06-01", 1, 1)
		}, func(t *testing.T, err error) { assert.ErrorAs(t, err, &ValidationError{}) }},
		{"currency destination", func(t *testing.T, f *investmentsTestFixture, _ int64) ShareExchangeInput {
			buyOn(t, f, "2026-02-01", 10, 10000)
			return shareExchangeInput(f, f.eurCommodityID, "2026-06-01", 1, 1)
		}, func(t *testing.T, err error) { assert.ErrorAs(t, err, &ValidationError{}) }},
		{"unrepresentable fraction", func(t *testing.T, f *investmentsTestFixture, newID int64) ShareExchangeInput {
			buyOn(t, f, "2026-02-01", 1, 10000)
			return shareExchangeInput(f, newID, "2026-06-01", 1, 3)
		}, func(t *testing.T, err error) { assert.ErrorIs(t, err, ErrShareExchangeFraction) }},
		{"dated behind a later sale it would leave without units", func(t *testing.T, f *investmentsTestFixture, newID int64) ShareExchangeInput {
			buyOn(t, f, "2026-02-01", 10, 10000)
			_, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-07-01", 2))
			require.NoError(t, err)
			return shareExchangeInput(f, newID, "2026-06-01", 1, 1)
		}, func(t *testing.T, err error) { assert.ErrorIs(t, err, ErrInvestmentTransferDependency) }},
		{"open short of the old instrument", func(t *testing.T, f *investmentsTestFixture, newID int64) ShareExchangeInput {
			shortSaleOn(t, f, "2026-02-01", 5, 5000)
			return shareExchangeInput(f, newID, "2026-06-01", 1, 1)
		}, func(t *testing.T, err error) { assert.ErrorIs(t, err, ErrInvestmentPositionSideConflict) }},
		{"zero ratio", func(t *testing.T, f *investmentsTestFixture, newID int64) ShareExchangeInput {
			buyOn(t, f, "2026-02-01", 10, 10000)
			return shareExchangeInput(f, newID, "2026-06-01", 0, 1)
		}, func(t *testing.T, err error) { assert.ErrorAs(t, err, &ValidationError{}) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			newID := seedTestSecurityCommodity(t, f, "NEWCO")
			input := test.setup(t, f, newID)
			before := shareExchangeWriteCounts(t, f.database)
			_, err := f.investmentService.PreviewShareExchange(context.Background(), input)
			require.Error(t, err)
			test.want(t, err)
			_, err = f.investmentService.ShareExchange(context.Background(), input)
			require.Error(t, err)
			test.want(t, err)
			assert.Equal(t, before, shareExchangeWriteCounts(t, f.database))
		})
	}
}

func TestShareExchangePreviewWritesNothing(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	before := shareExchangeWriteCounts(t, f.database)
	preview, err := f.investmentService.PreviewShareExchange(context.Background(), shareExchangeInput(f, newID, "2026-06-01", 2, 1))
	require.NoError(t, err)
	assert.Equal(t, "20", preview.Plan.DestinationQuantityValue.String())
	assert.Equal(t, before, shareExchangeWriteCounts(t, f.database))
}

// A corrected acquisition before the exchange changes the carried basis; the
// link appends a revision and the new instrument's lot replays with it.
func TestShareExchangeUpstreamBuyReplacementRevisesTheNewLot(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	bought := buyOn(t, f, "2026-02-01", 10, 10000)
	exchanged, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 1, 2))
	require.NoError(t, err)
	require.Len(t, exchanged.Plan.Links, 1)
	_, err = f.investmentService.ReplaceBuy(ctx, buyReplacementForNetting(f, bought.Transaction.ID, "2026-02-01", 10, 15000))
	require.NoError(t, err)

	var basis exact.Coefficient
	var basisScale int
	var quantity string
	require.NoError(t, f.database.QueryRow(`SELECT remaining_cost_basis_value, remaining_cost_basis_scale, remaining_quantity_value
		FROM current_investment_lots WHERE id = ?`, exchanged.Plan.Links[0].DestinationLotID).Scan(&basis, &basisScale, &quantity))
	assert.Zero(t, exact.ScaledIntFromCoefficient(basis, basisScale).Cmp(exact.ScaledIntFromInt64(15000, 2)), "basis %s", basis)
	assert.Equal(t, "5", quantity)
	var revisions int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_link_revisions
		WHERE operation_id = (SELECT operation_id FROM investment_transfer_facts WHERE transfer_kind = 'exchange')`).Scan(&revisions))
	assert.Equal(t, 1, revisions)

	sale := sellInput(f, "2026-07-01", 5)
	sale.CommodityID, sale.CashAmountValue = newID, 20000
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	require.Len(t, sold.Allocations, 1)
	assert.Zero(t, exact.ScaledIntFromInt64(sold.Allocations[0].CostBasisValue, sold.Allocations[0].CostBasisScale).Cmp(
		exact.ScaledIntFromInt64(15000, 2)))
	run, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	for _, check := range []string{CheckInvestmentFoundation, CheckInvestmentReplay, CheckLotReconciliation} {
		assert.Equal(t, SelfCheckPassed, resultFor(t, run, check).Status, "%s: %s", check, resultFor(t, run, check).Summary)
	}
}

// A purchase dated before the exchange would leave old units held after the
// whole holding was exchanged; it refuses with the exchange named.
func TestShareExchangeRefusesABackdatedPurchaseItWouldNotCover(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	exchanged, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 1, 1))
	require.NoError(t, err)
	var exchangeOperationID int64
	require.NoError(t, f.database.QueryRow(`SELECT operation_id FROM investment_transfer_facts
		WHERE transfer_kind = 'exchange'`).Scan(&exchangeOperationID))
	before := shareExchangeWriteCounts(t, f.database)
	_, err = f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, "2026-03-01", 5, 5000))
	require.Error(t, err)
	var dependency InvestmentBuyDependencyError
	var sale InvestmentSaleDependencyError
	switch {
	case errors.As(err, &dependency):
		assert.Equal(t, exchangeOperationID, dependency.OperationID)
	case errors.As(err, &sale):
		assert.Equal(t, exchangeOperationID, sale.OperationID)
	default:
		t.Fatalf("backdated purchase error %v does not name the exchange", err)
	}
	assert.Equal(t, before, shareExchangeWriteCounts(t, f.database))
	assert.Len(t, exchanged.Plan.Links, 1)

	// A purchase after the exchange date stays a separate holding.
	_, err = f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, "2026-07-01", 5, 5000))
	require.NoError(t, err)
}

func TestShareExchangeSelfCheckDetectsADamagedRatio(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	_, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 3, 2))
	require.NoError(t, err)
	_, err = f.database.Exec(`DROP TRIGGER investment_transfer_facts_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_transfer_facts SET ratio_numerator = 2 WHERE transfer_kind = 'exchange'`)
	require.NoError(t, err)
	run, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	result := resultFor(t, run, CheckInvestmentFoundation)
	assert.Equal(t, SelfCheckFailed, result.Status)
	assert.Contains(t, result.Summary, "share exchanges")
}

func TestShareExchangeSelfCheckDetectsAMissingDestinationEffect(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	result, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 1, 1))
	require.NoError(t, err)
	_, err = f.database.Exec(`DROP TRIGGER investment_operation_lot_effects_no_delete`)
	require.NoError(t, err)
	_, err = f.database.Exec(`DELETE FROM investment_operation_lot_effects WHERE lot_event_id IN (
		SELECT id FROM investment_lot_events WHERE lot_id = ? AND event_kind = 'transfer_in')`,
		result.Plan.Links[0].DestinationLotID)
	require.NoError(t, err)
	run, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	check := resultFor(t, run, CheckInvestmentFoundation)
	assert.Equal(t, SelfCheckFailed, check.Status)
	assert.Contains(t, check.Summary, "share exchange")
}

func TestShareExchangeExportCarriesInstrumentsAndRatio(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	_, err := f.investmentService.ShareExchange(context.Background(), shareExchangeInput(f, newID, "2026-06-01", 3, 2))
	require.NoError(t, err)
	var kind string
	var destination, numerator, denominator int64
	require.NoError(t, f.database.QueryRow(`SELECT transfer_kind, destination_commodity_id, ratio_numerator, ratio_denominator
		FROM investment_transfer_facts`).Scan(&kind, &destination, &numerator, &denominator))
	assert.Equal(t, "exchange", kind)
	assert.Equal(t, newID, destination)
	assert.Equal(t, [2]int64{3, 2}, [2]int64{numerator, denominator})
}

// The new units may go to the new instrument's own holding; the closure then
// follows the exchange across accounts as well as instruments.
func TestShareExchangeIntoAnotherHoldingFollowsUpstreamCorrections(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	bought := buyOn(t, f, "2026-02-01", 10, 10000)
	input := shareExchangeInput(f, newID, "2026-06-01", 2, 1)
	input.DestinationHoldingAccountID = destinationID
	exchanged, err := f.investmentService.ShareExchange(ctx, input)
	require.NoError(t, err)
	require.Len(t, exchanged.Plan.Links, 1)
	var account int64
	require.NoError(t, f.database.QueryRow(`SELECT account_id FROM investment_lots WHERE id = ?`,
		exchanged.Plan.Links[0].DestinationLotID).Scan(&account))
	assert.Equal(t, destinationID, account)
	postings := exchanged.Transaction.JournalEntries[0].Postings
	require.Len(t, postings, 4)
	for _, posting := range postings {
		if posting.CommodityID == newID && posting.QuantityValue.Sign() > 0 {
			assert.Equal(t, destinationID, posting.AccountID)
		}
	}

	_, err = f.investmentService.ReplaceBuy(ctx, buyReplacementForNetting(f, bought.Transaction.ID, "2026-02-01", 10, 12000))
	require.NoError(t, err)
	var basis exact.Coefficient
	var basisScale int
	require.NoError(t, f.database.QueryRow(`SELECT remaining_cost_basis_value, remaining_cost_basis_scale
		FROM current_investment_lots WHERE id = ?`, exchanged.Plan.Links[0].DestinationLotID).Scan(&basis, &basisScale))
	assert.Zero(t, exact.ScaledIntFromCoefficient(basis, basisScale).Cmp(exact.ScaledIntFromInt64(12000, 2)), "basis %s", basis)
	run, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	for _, check := range []string{CheckInvestmentFoundation, CheckInvestmentReplay, CheckLotReconciliation} {
		assert.Equal(t, SelfCheckPassed, resultFor(t, run, check).Status, "%s: %s", check, resultFor(t, run, check).Summary)
	}
}
