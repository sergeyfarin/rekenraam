package app

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func allocationTestSave(t *testing.T, f *returnsFixture, expected int64, accounts []int64, targets []db.AllocationTarget) db.AllocationRevision {
	t.Helper()
	revision, err := f.investmentService.SaveAllocation(context.Background(), SaveAllocationInput{ExpectedRevision: expected, AccountIDs: accounts, Targets: targets, OwnerUserID: f.ownerUserID})
	require.NoError(t, err)
	return revision
}
func allocationTestRead(t *testing.T, f *returnsFixture, from, to string) AllocationRead {
	t.Helper()
	result, err := f.investmentService.ReadAllocation(context.Background(), ReturnsInput{From: from, To: to, ReportingCommodityID: f.eurCommodityID})
	require.NoError(t, err)
	return result
}
func allocationTestRow(t *testing.T, p AllocationPoint, id int64, side string) AllocationRow {
	t.Helper()
	for _, r := range p.Rows {
		if r.CommodityID == id && r.Side == side {
			return r
		}
	}
	t.Fatalf("missing allocation row %d/%s", id, side)
	return AllocationRow{}
}

func TestAllocationIncludesDepositsAndKeepsReservedAccountsOutOfPool(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	deposit := seedTestAccountWithClass(t, f.database, "active", true, "asset", "term_deposit")
	college := seedTestAccountWithClass(t, f.database, "active", true, "asset", "savings")
	f.post(t, "2026-02-01", deposit, f.equityID, 300000)
	f.post(t, "2026-02-01", college, f.equityID, 500000)
	f.price(t, "2026-02-01", 60000)
	allocationTestSave(t, f, 0, []int64{deposit, f.brokerCashID, f.holdingAccountID}, []db.AllocationTarget{{CommodityID: f.stockCommodityID, Units: 600000}, {Units: 400000}})
	read := allocationTestRead(t, f, "2026-02-01", "2026-02-01")
	p := read.Points[0]
	require.Equal(t, "910000", p.Total.Value)
	require.Equal(t, "0.659341", *allocationTestRow(t, p, f.stockCommodityID, "long").Weight)
	require.Equal(t, "0.059341", *allocationTestRow(t, p, f.stockCommodityID, "long").Drift)
	require.Equal(t, "310000", allocationTestRow(t, p, 0, "long").Amount.Value)
	for _, a := range read.Accounts {
		if a.ID == college {
			require.False(t, a.Included)
		}
	}
	// Exclusion is reporting membership, never a mutation of the net-worth ledger.
	worth, err := f.transactionService.NetWorth(context.Background(), NetWorthInput{AsOf: "2026-02-01"})
	require.NoError(t, err)
	require.NotEmpty(t, worth.Totals)
	requireAmount(t, worth.Totals, f.eurCommodityID, 1700000, 2, "full native cash net worth retains college savings and all other accounts")
	requireAmount(t, worth.Totals, f.stockCommodityID, 10, 0, "full native net worth retains the security units separately")
}

func TestAllocationMissingPriceGapsRecoverAndDoNotBecomeZero(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	allocationTestSave(t, f, 0, []int64{f.brokerCashID, f.holdingAccountID}, []db.AllocationTarget{{CommodityID: f.stockCommodityID, Units: 900000}, {Units: 100000}})
	f.price(t, "2026-02-10", 20000)
	read := allocationTestRead(t, f, "2026-02-07", "2026-02-10")
	require.Empty(t, read.Points[0].Reason)
	gap := read.Points[1]
	require.Equal(t, "valuation_unavailable", gap.Reason)
	require.Nil(t, gap.Total)
	stock := allocationTestRow(t, gap, f.stockCommodityID, "long")
	require.Nil(t, stock.Amount)
	require.Nil(t, stock.Weight)
	require.Nil(t, stock.Drift)
	require.Len(t, gap.Unknown, 1)
	cash := allocationTestRow(t, gap, 0, "long")
	require.NotNil(t, cash.Amount)
	require.Nil(t, cash.Weight)
	closing := read.Points[len(read.Points)-1]
	require.Empty(t, closing.Reason)
	require.Equal(t, "0.952381", *allocationTestRow(t, closing, f.stockCommodityID, "long").Weight)
	require.Equal(t, "-0.052381", *allocationTestRow(t, closing, 0, "long").Drift)
}

func TestAllocationShortCashUsesSignedTableAndNoConventionalStack(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	f.price(t, "2026-02-01", 11000)
	f.post(t, "2026-02-01", f.feeID, f.brokerCashID, 20000)
	allocationTestSave(t, f, 0, []int64{f.brokerCashID, f.holdingAccountID}, []db.AllocationTarget{{CommodityID: f.stockCommodityID, Units: 900000}, {Units: 100000}})
	p := allocationTestRead(t, f, "2026-02-01", "2026-02-01").Points[0]
	require.Empty(t, p.Reason)
	require.Equal(t, "signed_allocation", p.ChartReason)
	require.Equal(t, "1.100000", *allocationTestRow(t, p, f.stockCommodityID, "long").Weight)
	require.Equal(t, "-0.100000", *allocationTestRow(t, p, 0, "short").Weight)
	require.Equal(t, "-0.100000", *allocationTestRow(t, p, 0, "short").Drift)
	require.Equal(t, "-0.100000", *allocationTestRow(t, p, 0, "long").Drift)
}

func TestAllocationSavedChangesKeepHistoryAndCompareAgainstCurrentTargets(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	f.price(t, "2026-02-01", 10000)
	accounts := []int64{f.brokerCashID, f.holdingAccountID}
	first := allocationTestSave(t, f, 0, accounts, []db.AllocationTarget{{CommodityID: f.stockCommodityID, Units: 600000}, {Units: 400000}})
	original, err := json.Marshal(first)
	require.NoError(t, err)
	second := allocationTestSave(t, f, first.ID, accounts, []db.AllocationTarget{{CommodityID: f.stockCommodityID, Units: 700000}, {Units: 300000}})
	read := allocationTestRead(t, f, "2026-02-01", "2026-02-01")
	require.Equal(t, second.ID, read.Current.ID)
	require.Len(t, read.Revisions, 2)
	frozen, err := json.Marshal(read.Revisions[1])
	require.NoError(t, err)
	require.Equal(t, string(original), string(frozen))
	require.Equal(t, "0.209091", *allocationTestRow(t, read.Points[0], f.stockCommodityID, "long").Drift)
	_, err = f.investmentService.SaveAllocation(context.Background(), SaveAllocationInput{ExpectedRevision: first.ID, AccountIDs: accounts, Targets: first.Targets, OwnerUserID: f.ownerUserID})
	require.ErrorIs(t, err, db.ErrAllocationStale)
	_, err = f.database.Exec(`UPDATE investment_allocation_revisions SET saved_at='changed' WHERE id=?`, first.ID)
	require.ErrorContains(t, err, "immutable")
	snapshot, cleanup, err := f.investmentService.repository.LiveGainsSnapshot(context.Background())
	require.NoError(t, err)
	defer cleanup()
	restored, err := db.AllocationRevisions(context.Background(), snapshot, BookID)
	require.NoError(t, err)
	require.Equal(t, read.Revisions, restored, "native backup image retains settings and immutable history")
}

func TestAllocationValidatesExactTargetsAndExplicitEmptyPool(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	for _, targets := range [][]db.AllocationTarget{
		{{CommodityID: f.stockCommodityID, Units: 999999}},
		{{CommodityID: f.stockCommodityID, Units: 500000}, {CommodityID: f.stockCommodityID, Units: 500000}},
		{{CommodityID: f.stockCommodityID, Units: 1000001}, {Units: -1}},
	} {
		_, err := f.investmentService.SaveAllocation(context.Background(), SaveAllocationInput{AccountIDs: []int64{}, Targets: targets, OwnerUserID: f.ownerUserID})
		require.Error(t, err)
	}
	revision := allocationTestSave(t, f, 0, []int64{}, []db.AllocationTarget{{CommodityID: f.stockCommodityID, Units: 1000000}})
	read := allocationTestRead(t, f, "2026-02-01", "2026-02-01")
	require.Equal(t, revision.ID, read.Current.ID)
	require.Empty(t, read.Current.AccountIDs)
	require.Equal(t, "non_positive_value", read.Points[0].Reason)
	require.Equal(t, "0", read.Points[0].Total.Value)
}

func TestAllocationDriftRoundsOnlyAfterSubtractingTheExactTarget(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	f.price(t, "2026-02-01", 10)
	f.post(t, "2026-02-01", f.brokerCashID, f.equityID, 199989900)
	allocationTestSave(t, f, 0, []int64{f.brokerCashID, f.holdingAccountID}, []db.AllocationTarget{{CommodityID: f.stockCommodityID, Units: 1}, {Units: 999999}})
	row := allocationTestRow(t, allocationTestRead(t, f, "2026-02-01", "2026-02-01").Points[0], f.stockCommodityID, "long")
	require.Equal(t, "0.000001", *row.Weight)
	require.Equal(t, "-0.000001", *row.Drift, "subtracting separately rounded weights would incorrectly produce zero")
}

func TestAllocationSnapshotKeepsScopeTargetsAndActualsTogetherWithoutWrites(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	f.price(t, "2026-02-01", 10000)
	first := allocationTestSave(t, f, 0, []int64{f.brokerCashID, f.holdingAccountID}, []db.AllocationTarget{{CommodityID: f.stockCommodityID, Units: 600000}, {Units: 400000}})
	snapshot, cleanup, err := f.investmentService.repository.LiveGainsSnapshot(context.Background())
	require.NoError(t, err)
	defer cleanup()
	allocationTestSave(t, f, first.ID, []int64{f.brokerCashID}, []db.AllocationTarget{{Units: 1000000}})
	var before, after int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&before))
	read, err := readAllocationOn(context.Background(), snapshot, f.investmentService.now(), ReturnsInput{From: "2026-02-01", To: "2026-02-01", ReportingCommodityID: f.eurCommodityID})
	require.NoError(t, err)
	require.Equal(t, first, read.Current)
	require.Equal(t, "110000", read.Points[0].Total.Value)
	current := allocationTestRead(t, f, "2026-02-01", "2026-02-01")
	require.Equal(t, "10000", current.Points[0].Total.Value)
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&after))
	require.Equal(t, before, after)
}

func TestAllocationConcurrentSavesAdmitOneRevisionAndOneAuditEvent(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	input := SaveAllocationInput{AccountIDs: []int64{f.brokerCashID, f.holdingAccountID}, Targets: []db.AllocationTarget{{CommodityID: f.stockCommodityID, Units: 600000}, {Units: 400000}}, OwnerUserID: f.ownerUserID}
	var before, after int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&before))
	results := make(chan error, 2)
	for range 2 {
		go func() { _, err := f.investmentService.SaveAllocation(context.Background(), input); results <- err }()
	}
	failures := 0
	for range 2 {
		err := <-results
		if err != nil {
			require.True(t, errors.Is(err, db.ErrAllocationStale))
			failures++
		}
	}
	require.Equal(t, 1, failures)
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&after))
	require.Equal(t, before+1, after)
	require.Equal(t, []int64{f.brokerCashID, f.holdingAccountID}, input.AccountIDs, "service must not mutate a caller's fields while canonicalizing")
}

func TestAllocationNativeBackupReopensEveryRevisionAndKeepsImmutableGuards(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	first := allocationTestSave(t, f, 0, []int64{f.holdingAccountID}, []db.AllocationTarget{{CommodityID: f.stockCommodityID, Units: 1000000}})
	allocationTestSave(t, f, first.ID, []int64{f.holdingAccountID, f.brokerCashID}, []db.AllocationTarget{{CommodityID: f.stockCommodityID, Units: 900000}, {Units: 100000}})
	original, err := db.AllocationRevisions(context.Background(), f.database, BookID)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "allocation.sqlite")
	_, err = db.OnlineBackupSQLiteDatabase(context.Background(), f.database, path, db.OnlineBackupOptions{})
	require.NoError(t, err)
	require.NoError(t, db.VerifySQLiteBackup(context.Background(), path))
	restored, err := db.Open(context.Background(), db.SQLiteFileURL(path))
	require.NoError(t, err)
	defer restored.Close()
	revisions, err := db.AllocationRevisions(context.Background(), restored, BookID)
	require.NoError(t, err)
	require.Equal(t, original, revisions)
	_, err = restored.Exec(`DELETE FROM investment_allocation_revisions WHERE id=?`, first.ID)
	require.ErrorContains(t, err, "immutable")
}

func TestAllocationCashWeightChangesWithoutCashMovementAndScopeNeverExpandsParents(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	parent := f.underContainer(t)
	f.price(t, "2026-02-01", 9000)
	f.price(t, "2026-02-02", 19000)
	first := allocationTestSave(t, f, 0, []int64{f.holdingAccountID, f.brokerCashID}, []db.AllocationTarget{{CommodityID: f.stockCommodityID, Units: 800000}, {Units: 200000}})
	read := allocationTestRead(t, f, "2026-02-01", "2026-02-02")
	for i, want := range []string{"-0.100000", "-0.150000"} {
		row := allocationTestRow(t, read.Points[i], 0, "long")
		require.Equal(t, "10000", row.Amount.Value)
		require.Equal(t, want, *row.Drift)
	}
	allocationTestSave(t, f, first.ID, []int64{parent}, first.Targets)
	empty := allocationTestRead(t, f, "2026-02-01", "2026-02-02")
	require.Equal(t, "0", empty.Points[1].Total.Value, "an included parent does not silently include an excluded child")
}

// One registered instrument is one target even when several brokers hold it.
func TestAllocationAggregatesOneInstrumentAcrossHoldingAccounts(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	other := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	_, err := f.investmentService.Buy(context.Background(), InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: other, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(5), CashAmountValue: 50000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	f.price(t, "2026-02-01", 10000)
	allocationTestSave(t, f, 0, []int64{f.holdingAccountID, other, f.brokerCashID}, []db.AllocationTarget{{CommodityID: f.stockCommodityID, Units: 900000}, {Units: 100000}})
	point := allocationTestRead(t, f, "2026-02-01", "2026-02-01").Points[0]
	require.Len(t, point.Rows, 2, "two holding accounts share one instrument row plus cash")
	require.Equal(t, "160000", point.Total.Value)
	row := allocationTestRow(t, point, f.stockCommodityID, "long")
	require.Equal(t, "150000", row.Amount.Value)
	require.Equal(t, "0.937500", *row.Weight)
	require.Equal(t, "0.037500", *row.Drift)
}

func TestAllocationAbsentHoldingsAreKnownZeroDuringAnotherPriceGap(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	empty, err := f.investmentService.CreateInstrument(context.Background(), InvestmentInstrumentInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "investment.instrument.create", CommodityCode: "EMPTY", InstrumentType: "etf", DisplayName: "Not held", QuoteCommodityID: &f.eurCommodityID, TradingCommodityID: &f.eurCommodityID, PriceScale: 2})
	require.NoError(t, err)
	other := seedTestSecurityCommodity(t, f.investmentsTestFixture, "LATER")
	otherAccount := seedTestAccountWithClass(t, f.database, "active", true, "asset", "brokerage_cash")
	f.priceOf(t, other, f.eurCommodityID, "2026-02-10", 10000)
	_, err = f.transactionService.CreateTransaction(context.Background(), CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Spec: TransactionInput{TransactionDate: "2026-02-10", JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
			{AccountID: otherAccount, CommodityID: other, QuantityValue: exact.New(1)},
			{AccountID: f.equityID, CommodityID: other, QuantityValue: exact.New(-1)},
		}}}},
	})
	require.NoError(t, err)
	allocationTestSave(t, f, 0, []int64{f.holdingAccountID, f.brokerCashID, otherAccount}, []db.AllocationTarget{{Units: 700000}, {CommodityID: empty.CommodityID, Units: 300000}})
	read := allocationTestRead(t, f, "2026-02-08", "2026-02-10")
	gap := read.Points[0]
	require.Equal(t, ReturnReasonValuationUnavailable, gap.Reason)
	absent := allocationTestRow(t, gap, other, "long")
	require.NotNil(t, absent.Amount, "the ledger establishes an absent holding independently of another price gap")
	require.Equal(t, "0", absent.Amount.Value)
	require.Nil(t, absent.Weight)
	require.Nil(t, absent.Drift)
	targeted := allocationTestRow(t, gap, empty.CommodityID, "long")
	require.Equal(t, "0", targeted.Amount.Value)
	require.Nil(t, targeted.Weight)
	require.Nil(t, targeted.Drift)
	complete := allocationTestRead(t, f, "2026-02-01", "2026-02-01").Points[0]
	require.Equal(t, "-0.300000", *allocationTestRow(t, complete, empty.CommodityID, "long").Drift)

	require.Nil(t, allocationTestRow(t, gap, f.stockCommodityID, "long").Amount, "a held unpriced instrument remains unknown")
}

func TestAllocationNetsCashAcrossAccountsWithoutHidingTheNegativeSubtotal(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	deposit := seedTestAccountWithClass(t, f.database, "active", true, "asset", "term_deposit")
	f.post(t, "2026-02-01", deposit, f.equityID, 1000000)
	f.post(t, "2026-02-01", f.feeID, f.brokerCashID, 60000)
	f.price(t, "2026-02-01", 100000)
	allocationTestSave(t, f, 0, []int64{deposit, f.holdingAccountID, f.brokerCashID}, []db.AllocationTarget{{CommodityID: f.stockCommodityID, Units: 500000}, {Units: 500000}})
	point := allocationTestRead(t, f, "2026-02-01", "2026-02-01").Points[0]
	require.Empty(t, point.ChartReason)
	require.Len(t, point.Rows, 2)
	require.Equal(t, "1950000", point.Total.Value)
	cash := allocationTestRow(t, point, 0, "long")
	require.Equal(t, "950000", cash.Amount.Value)
	require.Equal(t, "0.487179", *cash.Weight)
	require.Equal(t, "-0.012821", *cash.Drift)
	require.Equal(t, "-50000", point.NegativeCash.Value)
}

func TestAllocationNonPositiveTotalsKeepValuesAndWithholdShares(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		withdrawal int64
		total      string
	}{{"zero", 20000, "0"}, {"negative", 25000, "-5000"}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReturnsFixture(t)
			f.classify(t)
			f.price(t, "2026-02-01", 1000)
			f.post(t, "2026-02-01", f.feeID, f.brokerCashID, tc.withdrawal)
			allocationTestSave(t, f, 0, []int64{f.holdingAccountID, f.brokerCashID}, []db.AllocationTarget{{CommodityID: f.stockCommodityID, Units: 500000}, {Units: 500000}})
			point := allocationTestRead(t, f, "2026-02-01", "2026-02-01").Points[0]
			require.Equal(t, tc.total, point.Total.Value)
			require.Equal(t, ReturnReasonNonPositiveValue, point.Reason)
			for _, row := range point.Rows {
				require.NotNil(t, row.Amount)
				require.Nil(t, row.Weight)
				require.Nil(t, row.Drift)
				require.NotNil(t, row.TargetUnits)
			}
		})
	}
}

func TestAllocationUnregisteredInstrumentKeepsItsPricedActualIdentity(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t) // TEST has no instrument registration/type.
	f.price(t, "2026-02-01", 10000)
	allocationTestSave(t, f, 0, []int64{f.holdingAccountID, f.brokerCashID}, []db.AllocationTarget{{Units: 1000000}})
	read := allocationTestRead(t, f, "2026-02-01", "2026-02-01")
	row := allocationTestRow(t, read.Points[0], f.stockCommodityID, "long")
	require.Equal(t, "100000", row.Amount.Value)
	require.Equal(t, "0.909091", *row.Weight)
	require.Zero(t, *row.TargetUnits)
	require.Equal(t, "0.909091", *row.Drift)
	for _, commodity := range read.Commodities {
		if commodity.ID == f.stockCommodityID {
			require.False(t, commodity.Targetable)
		}
	}
}

func TestAllocationRequiredFXConvertsExactlyAndMissingFXWithholdsDrift(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	usd := f.commodity(t, "USD", "currency", 2)
	f.post(t, "2026-02-10", f.feeID, f.brokerCashID, 9000)           // EUR cash 10.
	f.observe(t, f.stockCommodityID, usd, "raw", "2026-02-10", 1000) // 10 units × USD 10.
	allocationTestSave(t, f, 0, []int64{f.holdingAccountID, f.brokerCashID}, []db.AllocationTarget{{CommodityID: f.stockCommodityID, Units: 800000}, {Units: 200000}})
	gap := allocationTestRead(t, f, "2026-02-10", "2026-02-10").Points[0]
	require.Equal(t, ReturnReasonValuationUnavailable, gap.Reason)
	require.Nil(t, allocationTestRow(t, gap, 0, "long").Drift)
	f.observe(t, usd, f.eurCommodityID, "raw", "2026-02-10", 90)
	point := allocationTestRead(t, f, "2026-02-10", "2026-02-10").Points[0]
	require.Equal(t, "10000", point.Total.Value)
	row := allocationTestRow(t, point, f.stockCommodityID, "long")
	require.Equal(t, "9000", row.Amount.Value)
	require.Equal(t, "0.900000", *row.Weight)
	require.Equal(t, "0.100000", *row.Drift)
}

func TestAllocationClosedAccountAndRenamedArchivedInstrumentKeepTargets(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	f.price(t, "2026-02-01", 10000)
	first := allocationTestSave(t, f, 0, []int64{f.holdingAccountID, f.brokerCashID}, []db.AllocationTarget{{CommodityID: f.stockCommodityID, Units: 900000}, {Units: 100000}})
	_, err := f.database.Exec(`INSERT INTO investment_instrument_versions(instrument_id,version_seq,effective_from,recorded_at,changed_by_user_id,change_reason,status,instrument_type,display_name,quote_commodity_id,trading_commodity_id,quantity_scale,price_scale)
 SELECT instrument_id,2,'2026-02-02','2026-02-02T00:00:00Z',changed_by_user_id,'renamed and retired','archived',instrument_type,'Renamed security',quote_commodity_id,trading_commodity_id,quantity_scale,price_scale FROM investment_instrument_versions WHERE version_seq=1`)
	require.NoError(t, err)
	_, err = f.accountService.CloseAccount(context.Background(), AccountLifecycleInput{OwnerUserID: f.ownerUserID, AccountID: f.holdingAccountID, OriginType: "browser_api", EffectiveFrom: "2026-02-02", ClosedOn: "2026-02-02"})
	require.NoError(t, err)
	read := allocationTestRead(t, f, "2026-02-01", "2026-02-01")
	require.Equal(t, first.Targets, read.Current.Targets)
	for _, account := range read.Accounts {
		if account.ID == f.holdingAccountID {
			require.Equal(t, "closed", account.Status)
			require.True(t, account.Included)
		}
	}
	for _, commodity := range read.Commodities {
		if commodity.ID == f.stockCommodityID {
			require.Equal(t, "Renamed security", commodity.Name)
			require.True(t, commodity.Targetable)
		}
	}
	second := allocationTestSave(t, f, first.ID, first.AccountIDs, first.Targets)
	require.Equal(t, first.Targets, second.Targets)
	require.Equal(t, "100000", allocationTestRow(t, read.Points[0], f.stockCommodityID, "long").Amount.Value)
}

func TestAllocationInvalidReferencesAndClearedTargetsCannotAppendRevisions(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	var system int64
	require.NoError(t, f.database.QueryRow(`SELECT id FROM accounts WHERE system_role IS NOT NULL LIMIT 1`).Scan(&system))
	unregistered := seedTestSecurityCommodity(t, f.investmentsTestFixture, "UNREGISTERED")
	for _, input := range []SaveAllocationInput{
		{AccountIDs: []int64{system}, Targets: []db.AllocationTarget{{Units: 1000000}}},
		{AccountIDs: []int64{f.feeID}, Targets: []db.AllocationTarget{{Units: 1000000}}},
		{AccountIDs: []int64{999999}, Targets: []db.AllocationTarget{{Units: 1000000}}},
		{AccountIDs: []int64{f.brokerCashID}, Targets: []db.AllocationTarget{{CommodityID: unregistered, Units: 1000000}}},
	} {
		input.OwnerUserID = f.ownerUserID
		_, err := f.investmentService.SaveAllocation(context.Background(), input)
		require.ErrorIs(t, err, db.ErrAllocationReference)
	}
	first := allocationTestSave(t, f, 0, []int64{f.brokerCashID}, []db.AllocationTarget{{Units: 1000000}})
	_, err := f.investmentService.SaveAllocation(context.Background(), SaveAllocationInput{OwnerUserID: f.ownerUserID, ExpectedRevision: first.ID, AccountIDs: first.AccountIDs, Targets: []db.AllocationTarget{}})
	require.ErrorIs(t, err, db.ErrAllocationReference)
	read := allocationTestRead(t, f, "2026-02-01", "2026-02-01")
	require.Equal(t, first, read.Current)
	require.Len(t, read.Revisions, 1)
}

func TestAllocationSuggestedPoolIncludesDepositsAndSavedEmptyScopeWins(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	deposit := seedTestAccountWithClass(t, f.database, "active", true, "asset", "term_deposit")
	reservedLoan := seedTestAccountWithClass(t, f.database, "active", true, "asset", "receivable")
	read := allocationTestRead(t, f, "2026-02-01", "2026-02-01")
	require.Zero(t, read.Current.ID)
	for _, id := range []int64{deposit, f.cashAccountID, f.brokerCashID, f.holdingAccountID} {
		require.Contains(t, read.Current.AccountIDs, id)
	}
	for _, id := range []int64{reservedLoan, f.feeID, f.equityID} {
		require.NotContains(t, read.Current.AccountIDs, id)
	}
	first := allocationTestSave(t, f, 0, []int64{}, []db.AllocationTarget{})
	read = allocationTestRead(t, f, "2026-02-01", "2026-02-01")
	require.Equal(t, first, read.Current)
	require.Empty(t, read.Current.AccountIDs)
}

func TestAllocationUnclassifiedShortDoesNotBecomeAValuedZero(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	f.price(t, "2026-02-01", 10000)
	_, err := f.transactionService.CreateTransaction(context.Background(), CreateTransactionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Spec: TransactionInput{TransactionDate: "2026-02-01", JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
		{AccountID: f.brokerCashID, CommodityID: f.stockCommodityID, QuantityValue: exact.New(-1)},
		{AccountID: f.equityID, CommodityID: f.stockCommodityID, QuantityValue: exact.New(1)},
	}}}}})
	require.NoError(t, err)
	allocationTestSave(t, f, 0, []int64{f.holdingAccountID, f.brokerCashID}, []db.AllocationTarget{{Units: 1000000}})
	point := allocationTestRead(t, f, "2026-02-01", "2026-02-01").Points[0]
	require.Equal(t, ReturnReasonUnclassifiedShort, point.Reason)
	require.Nil(t, point.Total)
	require.Nil(t, allocationTestRow(t, point, f.stockCommodityID, "short").Amount)
	for _, row := range point.Rows {
		require.Nil(t, row.Weight)
		require.Nil(t, row.Drift)
	}
}

func TestAllocationOffsettingSecuritiesStillNeedPricesForGrossRows(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	shortAccount := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	_, err := f.investmentService.ShortSale(context.Background(), InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID, HoldingAccountID: shortAccount, CashAccountID: f.brokerCashID, QuantityValue: exact.New(10), CashAmountValue: 100000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID})
	require.NoError(t, err)
	allocationTestSave(t, f, 0, []int64{f.holdingAccountID, shortAccount, f.brokerCashID}, []db.AllocationTarget{{Units: 1000000}})
	point := allocationTestRead(t, f, "2026-02-10", "2026-02-10").Points[0]
	require.Equal(t, ReturnReasonValuationUnavailable, point.Reason)
	require.Len(t, point.Unknown, 2)
	require.Nil(t, allocationTestRow(t, point, f.stockCommodityID, "long").Amount)
	require.Nil(t, allocationTestRow(t, point, f.stockCommodityID, "short").Amount)
}

func TestAllocationNetsCashAcrossCurrenciesUsingExactFX(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	usd := f.commodity(t, "USD", "currency", 2)
	deposit := seedTestAccountWithClass(t, f.database, "active", true, "asset", "term_deposit")
	f.post(t, "2026-02-10", f.feeID, f.brokerCashID, 15000) // EUR -50.
	f.price(t, "2026-02-10", 10000)                         // Securities EUR 1,000.
	_, err := f.transactionService.CreateTransaction(context.Background(), CreateTransactionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Spec: TransactionInput{TransactionDate: "2026-02-10", JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
		{AccountID: deposit, CommodityID: usd, QuantityValue: exact.New(10000), QuantityScale: 2},
		{AccountID: f.equityID, CommodityID: usd, QuantityValue: exact.New(-10000), QuantityScale: 2},
	}}}}})
	require.NoError(t, err)
	allocationTestSave(t, f, 0, []int64{f.holdingAccountID, f.brokerCashID, deposit}, []db.AllocationTarget{{Units: 1000000}})
	gap := allocationTestRead(t, f, "2026-02-10", "2026-02-10").Points[0]
	require.Equal(t, ReturnReasonValuationUnavailable, gap.Reason)
	require.Nil(t, allocationTestRow(t, gap, 0, "long").Amount)
	f.observe(t, usd, f.eurCommodityID, "raw", "2026-02-10", 90)
	point := allocationTestRead(t, f, "2026-02-10", "2026-02-10").Points[0]
	require.Empty(t, point.ChartReason)
	require.Equal(t, "104000", point.Total.Value)
	require.Equal(t, "4000", allocationTestRow(t, point, 0, "long").Amount.Value)
	require.Equal(t, "-5000", point.NegativeCash.Value)
}

func TestAllocationOffsettingNativeCashNeedsNoFXButNegativeSubtotalStaysUnknown(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	f.classify(t)
	usd := f.commodity(t, "USD", "currency", 2)
	positive := seedTestAccountWithClass(t, f.database, "active", true, "asset", "brokerage_cash")
	negative := seedTestAccountWithClass(t, f.database, "active", true, "asset", "brokerage_cash")
	_, err := f.transactionService.CreateTransaction(context.Background(), CreateTransactionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Spec: TransactionInput{TransactionDate: "2026-02-01", JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
		{AccountID: positive, CommodityID: usd, QuantityValue: exact.New(10000), QuantityScale: 2},
		{AccountID: negative, CommodityID: usd, QuantityValue: exact.New(-10000), QuantityScale: 2},
	}}}}})
	require.NoError(t, err)
	f.price(t, "2026-02-01", 10000)
	allocationTestSave(t, f, 0, []int64{f.holdingAccountID, f.brokerCashID, positive, negative}, []db.AllocationTarget{{Units: 1000000}})
	point := allocationTestRead(t, f, "2026-02-01", "2026-02-01").Points[0]
	require.Empty(t, point.Reason)
	require.Empty(t, point.ChartReason)
	require.Equal(t, "110000", point.Total.Value)
	require.Nil(t, point.NegativeCash, "USD borrowing remains disclosed as unavailable; net-zero USD is still a known-zero contribution")
	require.NotNil(t, allocationTestRow(t, point, 0, "long").Drift)
}

func TestAllocationHistoricalReferencesBlockUnusedAccountAndInstrumentDeletion(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	unused := seedTestAccountWithClass(t, f.database, "active", true, "asset", "checking")
	instrument, err := f.investmentService.CreateInstrument(context.Background(), InvestmentInstrumentInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "investment.instrument.create", CommodityCode: "PLANNED", InstrumentType: "etf", DisplayName: "Planned instrument", QuoteCommodityID: &f.eurCommodityID, TradingCommodityID: &f.eurCommodityID, PriceScale: 2})
	require.NoError(t, err)
	first := allocationTestSave(t, f, 0, []int64{unused}, []db.AllocationTarget{{CommodityID: instrument.CommodityID, Units: 1000000}})
	allocationTestSave(t, f, first.ID, []int64{}, []db.AllocationTarget{{Units: 1000000}})
	err = f.accountService.DeleteUnusedAccount(context.Background(), DeleteAccountInput{OwnerUserID: f.ownerUserID, AccountID: unused})
	require.ErrorIs(t, err, ErrAccountDeleteNotAllowed)
	err = f.investmentService.repository.DeleteUnusedInstrument(context.Background(), BookID, instrument.ID)
	require.ErrorContains(t, err, "allocation history")
	account, err := f.accountService.Account(context.Background(), unused)
	require.NoError(t, err)
	require.Equal(t, unused, account.ID)
	retained, err := f.investmentService.Instrument(context.Background(), instrument.ID)
	require.NoError(t, err)
	require.Equal(t, instrument.CommodityID, retained.CommodityID)
}
