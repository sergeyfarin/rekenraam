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
