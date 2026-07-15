package db

import (
	"context"
	"database/sql"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// This file hardens the lot-disposal invariants that db/investments_test.go
// exercises example-by-example, as reusable assertion helpers run across a
// scenario table. New disposal methods (I-03 analytical reporting, I-04 gain
// postings, R13 return analytics) should be able to reuse these directly.
//
// One finding shaped every helper below and is worth recording up front:
// "Σ disposed_basis + Σ remaining_basis == Σ acquired_basis" — the naive
// basis-conservation invariant — holds exactly for fifo/lifo/specific_lot,
// but NOT for average_cost when open lots have different unit costs.
// disposeAverageCostLotTx (investments.go) deliberately computes two
// different numbers per touched lot: the *reported* basis (pool rate,
// what LotDisposalRecord.CostBasisValue returns and what PreviewSell/
// ListRealizedGains show) and the *DB-deducted* basis (each lot's own rate,
// what's actually subtracted from remaining_cost_basis_value). The code
// comment explains why: using the pool rate for the DB write could drive a
// below-average-cost lot's remaining basis negative on a partial sale, which
// the "remaining stays non-negative" invariant forbids. This was verified
// empirically before writing these tests (two lots, qty=100 each, basis
// 10000 and 30000, average-cost sale of 150 units: reported total is 30000,
// but summing reported-basis-disposed + remaining-basis-after gives 45000,
// not the original pool's 40000 — a real, intentional "redistribution
// effect", not a bug). assertAverageCostPoolInvariants below tests the
// invariants that actually hold for average_cost instead of the ones that
// don't.

// --- Fixture ---

type lotSpec struct {
	openedOn   string
	qty        int64
	qtyScale   int
	basis      int64
	basisScale int
}

func seedLots(t *testing.T, database *sql.DB, repo *InvestmentRepository, accountID int64, commodityID int64, costCommodityID int64, ownerID int64, specs []lotSpec) []InvestmentLotRecord {
	t.Helper()
	ctx := context.Background()
	lots := make([]InvestmentLotRecord, 0, len(specs))
	for i, spec := range specs {
		lot, err := repo.CreateLot(ctx, CreateInvestmentLotParams{
			BookID: 1, AccountID: accountID, CommodityID: commodityID,
			OpenedOn: spec.openedOn, QuantityValue: exact.New(spec.qty), QuantityScale: spec.qtyScale,
			CostBasisValue: spec.basis, CostBasisScale: spec.basisScale, CostCommodityID: costCommodityID,
			MetadataJSON: "{}", CreatedAt: spec.openedOn + "T09:00:00Z", CreatedByUserID: ownerID,
			OriginType: "browser_api", Operation: "investment.lot.acquire",
			ChangeReason: "invariant test lot", EventKind: "acquisition",
		})
		require.NoError(t, err, "seed lot %d", i)
		lots = append(lots, lot)
	}
	return lots
}

// --- Reusable invariant assertions ---

func sumAcquiredBasis(lots []InvestmentLotRecord) int64 {
	var total int64
	for _, l := range lots {
		total += l.CostBasisValue
	}
	return total
}

func sumRemainingBasis(lots []InvestmentLotRecord) int64 {
	var total int64
	for _, l := range lots {
		total += l.RemainingCostBasisValue
	}
	return total
}

func sumRemainingQuantity(t *testing.T, lots []InvestmentLotRecord) *big.Int {
	t.Helper()
	total := new(big.Int)
	for _, l := range lots {
		total.Add(total, l.RemainingQuantityValue.BigInt())
	}
	return total
}

func sumDisposedBasis(disposals []LotDisposalRecord) int64 {
	var total int64
	for _, d := range disposals {
		total += d.CostBasisValue
	}
	return total
}

// assertNoNegativeRemainders checks the invariant that must hold for every
// lot regardless of cost-basis method: remaining quantity and basis never go
// negative, and "closed" is exactly the state where both hit zero together.
func assertNoNegativeRemainders(t *testing.T, lots []InvestmentLotRecord) {
	t.Helper()
	for _, l := range lots {
		assert.True(t, l.RemainingQuantityValue.Sign() >= 0, "lot %d remaining quantity must not be negative, got %s", l.ID, l.RemainingQuantityValue.String())
		assert.GreaterOrEqual(t, l.RemainingCostBasisValue, int64(0), "lot %d remaining basis must not be negative", l.ID)
		if l.Status == "closed" {
			assert.Equal(t, "0", l.RemainingQuantityValue.String(), "lot %d closed but remaining quantity is not zero", l.ID)
			assert.Equal(t, int64(0), l.RemainingCostBasisValue, "lot %d closed but remaining basis is not zero", l.ID)
		}
		if l.RemainingQuantityValue.Sign() == 0 {
			assert.Equal(t, "closed", l.Status, "lot %d has zero remaining quantity but is not closed", l.ID)
		}
	}
}

// assertLotLevelMethodConservesBasisExactly is the true conservation
// invariant for fifo, lifo, and specific_lot: every disposed unit's reported
// basis is prorated within its own lot (proratedCostBasis), so the same
// number is both returned to the caller and subtracted from that lot's
// remaining basis — no cross-lot redistribution, so summing always balances
// exactly against what was originally acquired. Does NOT apply to
// average_cost — see assertAverageCostPoolInvariants.
func assertLotLevelMethodConservesBasisExactly(t *testing.T, acquiredBasisBefore int64, disposals []LotDisposalRecord, lotsAfter []InvestmentLotRecord) {
	t.Helper()
	got := sumDisposedBasis(disposals) + sumRemainingBasis(lotsAfter)
	assert.Equal(t, acquiredBasisBefore, got, "disposed + remaining must equal originally acquired basis exactly")
}

// assertAverageCostPoolInvariants tests what actually holds for average_cost
// disposals: the pool-rate residual assignment is exact (no minor unit lost
// or invented across the disposed lots in one sale), the sold quantity is
// removed from the pool exactly, and every lot's remaining basis/quantity
// stays non-negative — the guarantee the own-rate DB deduction exists to
// provide. It does not assert reported-basis-plus-remaining equals acquired
// basis; that does not hold when lots have mixed unit costs (see the file
// comment above).
func assertAverageCostPoolInvariants(t *testing.T, poolBasisBefore int64, poolQtyBefore *big.Int, soldQty *big.Int, disposals []LotDisposalRecord, lotsAfter []InvestmentLotRecord) {
	t.Helper()

	expectedDisposedTotal := new(big.Int).Mul(big.NewInt(poolBasisBefore), soldQty)
	expectedDisposedTotal.Quo(expectedDisposedTotal, poolQtyBefore)
	require.True(t, expectedDisposedTotal.IsInt64(), "expected disposed total overflowed int64")
	assert.Equal(t, expectedDisposedTotal.Int64(), sumDisposedBasis(disposals), "sum of reported disposed basis must exactly equal the pool-rate total — the residual-assignment invariant")

	expectedRemainingQty := new(big.Int).Sub(poolQtyBefore, soldQty)
	assert.Equal(t, expectedRemainingQty.String(), sumRemainingQuantity(t, lotsAfter).String(), "pool quantity must reduce by exactly the sold amount")

	assertNoNegativeRemainders(t, lotsAfter)
}

func snapshotLots(t *testing.T, repo *InvestmentRepository, accountID int64, commodityID int64) []InvestmentLotRecord {
	t.Helper()
	lots, err := repo.ListLots(context.Background(), 1, accountID, commodityID)
	require.NoError(t, err)
	return lots
}

// assertLotsIdentical does a full field-by-field snapshot comparison, not
// spot checks on a couple of fields — used for the oversell-atomicity
// invariant (a rejected sale must leave every lot byte-identical) and for
// proving untouched lots in a partial sale are not silently touched.
func assertLotsIdentical(t *testing.T, before []InvestmentLotRecord, after []InvestmentLotRecord) {
	t.Helper()
	require.Len(t, after, len(before), "lot count changed")
	for i := range before {
		assert.Equal(t, before[i], after[i], "lot %d changed when it should not have", before[i].ID)
	}
}

// --- Scenario matrix: single lot exact close, partial across three lots ---

func TestInvariant_ConservationAcrossMethodsAndScenarios(t *testing.T) {
	type scenario struct {
		name    string
		lots    []lotSpec
		sellQty int64
	}
	scenarios := []scenario{
		{
			name:    "single lot exact close",
			lots:    []lotSpec{{openedOn: "2026-01-01", qty: 100, basis: 10000, basisScale: 2}},
			sellQty: 100,
		},
		{
			name: "partial across three lots",
			lots: []lotSpec{
				{openedOn: "2026-01-01", qty: 100, basis: 10000, basisScale: 2},
				{openedOn: "2026-02-01", qty: 200, basis: 25000, basisScale: 2},
				{openedOn: "2026-03-01", qty: 150, basis: 21000, basisScale: 2},
			},
			sellQty: 320, // closes lot 1 and 2 exactly, partial into lot 3
		},
		{
			name: "one-minor-unit quantities stress truncation",
			lots: []lotSpec{
				{openedOn: "2026-01-01", qty: 1, basis: 7, basisScale: 2},
				{openedOn: "2026-02-01", qty: 1, basis: 11, basisScale: 2},
				{openedOn: "2026-03-01", qty: 1, basis: 13, basisScale: 2},
			},
			sellQty: 3,
		},
		{
			name: "large-value lots near int64 range",
			lots: []lotSpec{
				{openedOn: "2026-01-01", qty: 1_000_000, basis: 900_000_000_000_000, basisScale: 2},
				{openedOn: "2026-02-01", qty: 2_000_000, basis: 1_800_000_000_000_000, basisScale: 2},
			},
			sellQty: 2_500_000,
		},
	}
	methods := []string{"fifo", "lifo", "average_cost", "specific_lot"}

	for _, sc := range scenarios {
		for _, method := range methods {
			t.Run(sc.name+"/"+method, func(t *testing.T) {
				ctx := context.Background()
				database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
				accountID := createInvestmentTestAccount(t, database, currencyID)
				instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
				repo := NewInvestmentRepository(database)

				lotsBefore := seedLots(t, database, repo, accountID, instrument.CommodityID, currencyID, ownerID, sc.lots)
				acquiredBasisBefore := sumAcquiredBasis(lotsBefore)
				poolQtyBefore := new(big.Int)
				for _, l := range sc.lots {
					poolQtyBefore.Add(poolQtyBefore, big.NewInt(l.qty))
				}

				params := DisposeLotsParams{
					BookID: 1, AccountID: accountID, CommodityID: instrument.CommodityID,
					EventDate: "2026-06-01", QuantityValue: exact.New(sc.sellQty), QuantityScale: 0,
					CostBasisMethod: method, CreatedAt: "2026-06-01T09:00:00Z", ActorUserID: ownerID,
					OriginType: "browser_api", Operation: "investment.lot.dispose", ChangeReason: "invariant test sell",
				}
				if method == "specific_lot" {
					params.Allocations = allocateSequentially(lotsBefore, sc.sellQty)
				}

				disposals, err := repo.DisposeLots(ctx, params)
				require.NoError(t, err)

				lotsAfter := snapshotLots(t, repo, accountID, instrument.CommodityID)
				assertNoNegativeRemainders(t, lotsAfter)

				if method == "average_cost" {
					assertAverageCostPoolInvariants(t, acquiredBasisBefore, poolQtyBefore, big.NewInt(sc.sellQty), disposals, lotsAfter)
				} else {
					assertLotLevelMethodConservesBasisExactly(t, acquiredBasisBefore, disposals, lotsAfter)
				}
			})
		}
	}
}

// allocateSequentially builds specific_lot allocations that consume lots in
// the order given (oldest-first, matching FIFO) until sellQty is satisfied —
// a stand-in for whatever lot picker a real caller would use.
func allocateSequentially(lots []InvestmentLotRecord, sellQty int64) []LotAllocation {
	remaining := sellQty
	var allocations []LotAllocation
	for _, l := range lots {
		if remaining <= 0 {
			break
		}
		lotQty := l.RemainingQuantityValue.BigInt().Int64()
		take := lotQty
		if take > remaining {
			take = remaining
		}
		allocations = append(allocations, LotAllocation{LotID: l.ID, QuantityValue: exact.New(take), QuantityScale: l.RemainingQuantityScale})
		remaining -= take
	}
	return allocations
}

// --- Interleaved buy/sell/buy/sell ---

func TestInvariant_InterleavedAcquisitionsAndDisposalsConserveBasis(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID := createInvestmentTestAccount(t, database, currencyID)
	instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)

	var totalAcquired int64
	acquire := func(openedOn string, qty int64, basis int64) {
		_, err := repo.CreateLot(ctx, CreateInvestmentLotParams{
			BookID: 1, AccountID: accountID, CommodityID: instrument.CommodityID,
			OpenedOn: openedOn, QuantityValue: exact.New(qty), QuantityScale: 0,
			CostBasisValue: basis, CostBasisScale: 2, CostCommodityID: currencyID,
			MetadataJSON: "{}", CreatedAt: openedOn + "T09:00:00Z", CreatedByUserID: ownerID,
			OriginType: "browser_api", Operation: "investment.lot.acquire",
			ChangeReason: "interleaved test", EventKind: "acquisition",
		})
		require.NoError(t, err)
		totalAcquired += basis
	}
	var totalDisposed int64
	dispose := func(eventDate string, qty int64) {
		disposals, err := repo.DisposeLots(ctx, DisposeLotsParams{
			BookID: 1, AccountID: accountID, CommodityID: instrument.CommodityID,
			EventDate: eventDate, QuantityValue: exact.New(qty), QuantityScale: 0,
			CostBasisMethod: "fifo", CreatedAt: eventDate + "T09:00:00Z", ActorUserID: ownerID,
			OriginType: "browser_api", Operation: "investment.lot.dispose", ChangeReason: "interleaved test",
		})
		require.NoError(t, err)
		totalDisposed += sumDisposedBasis(disposals)
	}

	acquire("2026-01-01", 100, 10000)
	dispose("2026-02-01", 40)
	acquire("2026-03-01", 50, 6000)
	dispose("2026-04-01", 90) // spans the rest of lot 1 and part of lot 2

	lotsAfter := snapshotLots(t, repo, accountID, instrument.CommodityID)
	assertNoNegativeRemainders(t, lotsAfter)
	assert.Equal(t, totalAcquired, totalDisposed+sumRemainingBasis(lotsAfter), "cumulative basis must balance across the whole interleaved sequence, fifo has no reporting/deduction split")
}

// --- FIFO orders by opened_on, not insertion order ---

func TestInvariant_FIFOOrdersByOpenedOnNotInsertionOrder(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID := createInvestmentTestAccount(t, database, currencyID)
	instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)

	// Insert the later-dated lot first, so it gets the lower ID — proves
	// disposeFIFOOrLIFOTx's "ORDER BY opened_on, id" genuinely sorts by date,
	// not by insertion/id order.
	laterLot, err := repo.CreateLot(ctx, CreateInvestmentLotParams{
		BookID: 1, AccountID: accountID, CommodityID: instrument.CommodityID,
		OpenedOn: "2026-03-01", QuantityValue: exact.New(100), QuantityScale: 0,
		CostBasisValue: 15000, CostBasisScale: 2, CostCommodityID: currencyID,
		MetadataJSON: "{}", CreatedAt: "2026-03-01T09:00:00Z", CreatedByUserID: ownerID,
		OriginType: "browser_api", Operation: "investment.lot.acquire",
		ChangeReason: "later-dated lot inserted first", EventKind: "acquisition",
	})
	require.NoError(t, err)
	earlierLot, err := repo.CreateLot(ctx, CreateInvestmentLotParams{
		BookID: 1, AccountID: accountID, CommodityID: instrument.CommodityID,
		OpenedOn: "2026-01-01", QuantityValue: exact.New(100), QuantityScale: 0,
		CostBasisValue: 10000, CostBasisScale: 2, CostCommodityID: currencyID,
		MetadataJSON: "{}", CreatedAt: "2026-01-01T09:00:00Z", CreatedByUserID: ownerID,
		OriginType: "browser_api", Operation: "investment.lot.acquire",
		ChangeReason: "earlier-dated lot inserted second", EventKind: "acquisition",
	})
	require.NoError(t, err)
	require.Greater(t, earlierLot.ID, laterLot.ID, "test setup: the earlier-dated lot must have the higher id for this to be a meaningful check")

	disposals, err := repo.DisposeLots(ctx, DisposeLotsParams{
		BookID: 1, AccountID: accountID, CommodityID: instrument.CommodityID,
		EventDate: "2026-06-01", QuantityValue: exact.New(50), QuantityScale: 0,
		CostBasisMethod: "fifo", CreatedAt: "2026-06-01T09:00:00Z", ActorUserID: ownerID,
		OriginType: "browser_api", Operation: "investment.lot.dispose", ChangeReason: "fifo order test",
	})
	require.NoError(t, err)
	require.Len(t, disposals, 1)
	assert.Equal(t, earlierLot.ID, disposals[0].LotID, "fifo must dispose the earlier opened_on lot first regardless of insertion order/id")
}

// --- Method actually matters ---

func TestInvariant_MethodActuallyChangesDisposedBasis(t *testing.T) {
	// The original I-02 bug class: a method resolver that silently always
	// disposes FIFO regardless of the requested method. fifo, lifo, and
	// average_cost over the identical 3-lot seed must produce three
	// different disposed-basis totals.
	newSeed := func(t *testing.T) (*InvestmentRepository, int64, int64) {
		t.Helper()
		database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
		accountID := createInvestmentTestAccount(t, database, currencyID)
		instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
		repo := NewInvestmentRepository(database)
		seedLots(t, database, repo, accountID, instrument.CommodityID, currencyID, ownerID, []lotSpec{
			{openedOn: "2026-01-01", qty: 10, basis: 10000, basisScale: 2},
			{openedOn: "2026-02-01", qty: 10, basis: 15000, basisScale: 2},
			{openedOn: "2026-03-01", qty: 10, basis: 21000, basisScale: 2},
		})
		return repo, accountID, instrument.CommodityID
	}

	disposedTotal := func(t *testing.T, method string) int64 {
		repo, accountID, commodityID := newSeed(t)
		disposals, err := repo.DisposeLots(context.Background(), DisposeLotsParams{
			BookID: 1, AccountID: accountID, CommodityID: commodityID,
			EventDate: "2026-06-01", QuantityValue: exact.New(15), QuantityScale: 0,
			CostBasisMethod: method, CreatedAt: "2026-06-01T09:00:00Z", ActorUserID: 1,
			OriginType: "browser_api", Operation: "investment.lot.dispose", ChangeReason: "method comparison",
		})
		require.NoError(t, err)
		return sumDisposedBasis(disposals)
	}

	fifoTotal := disposedTotal(t, "fifo")
	lifoTotal := disposedTotal(t, "lifo")
	averageCostTotal := disposedTotal(t, "average_cost")

	assert.NotEqual(t, fifoTotal, lifoTotal, "fifo disposes the oldest (cheapest) lots first, lifo the newest (priciest)")
	assert.NotEqual(t, fifoTotal, averageCostTotal)
	assert.NotEqual(t, lifoTotal, averageCostTotal)
}

// --- Oversell fails atomically ---

func TestInvariant_OversellFailsAtomicallyAcrossMethods(t *testing.T) {
	methods := []string{"fifo", "lifo", "average_cost"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			ctx := context.Background()
			database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
			accountID := createInvestmentTestAccount(t, database, currencyID)
			instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
			repo := NewInvestmentRepository(database)
			seedLots(t, database, repo, accountID, instrument.CommodityID, currencyID, ownerID, []lotSpec{
				{openedOn: "2026-01-01", qty: 100, basis: 10000, basisScale: 2},
				{openedOn: "2026-02-01", qty: 50, basis: 6000, basisScale: 2},
			})
			before := snapshotLots(t, repo, accountID, instrument.CommodityID)

			_, err := repo.DisposeLots(ctx, DisposeLotsParams{
				BookID: 1, AccountID: accountID, CommodityID: instrument.CommodityID,
				EventDate: "2026-06-01", QuantityValue: exact.New(1000), QuantityScale: 0, // far more than the 150 available
				CostBasisMethod: method, CreatedAt: "2026-06-01T09:00:00Z", ActorUserID: ownerID,
				OriginType: "browser_api", Operation: "investment.lot.dispose", ChangeReason: "oversell test",
			})
			require.ErrorIs(t, err, ErrInsufficientLots)

			after := snapshotLots(t, repo, accountID, instrument.CommodityID)
			assertLotsIdentical(t, before, after)
		})
	}
}

// --- Residual determinism ---

func TestInvariant_AverageCostResidualIsDeterministic(t *testing.T) {
	// Repeating the identical disposal on an identical seed (two independent
	// databases, not a shared one) must produce byte-identical lot events —
	// no hidden non-determinism (map iteration, floating point, etc.) in the
	// residual-assignment math.
	run := func(t *testing.T) []LotDisposalRecord {
		database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
		accountID := createInvestmentTestAccount(t, database, currencyID)
		instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
		repo := NewInvestmentRepository(database)
		seedLots(t, database, repo, accountID, instrument.CommodityID, currencyID, ownerID, []lotSpec{
			{openedOn: "2026-01-01", qty: 7, basis: 1001, basisScale: 2},
			{openedOn: "2026-02-01", qty: 11, basis: 1703, basisScale: 2},
			{openedOn: "2026-03-01", qty: 13, basis: 2201, basisScale: 2},
		})
		disposals, err := repo.DisposeLots(context.Background(), DisposeLotsParams{
			BookID: 1, AccountID: accountID, CommodityID: instrument.CommodityID,
			EventDate: "2026-06-01", QuantityValue: exact.New(17), QuantityScale: 0,
			CostBasisMethod: "average_cost", CreatedAt: "2026-06-01T09:00:00Z", ActorUserID: ownerID,
			OriginType: "browser_api", Operation: "investment.lot.dispose", ChangeReason: "determinism test",
		})
		require.NoError(t, err)
		return disposals
	}

	first := run(t)
	second := run(t)
	require.Equal(t, len(first), len(second))
	for i := range first {
		assert.Equal(t, first[i].QuantityValue.String(), second[i].QuantityValue.String(), "disposal %d quantity diverged across identical runs", i)
		assert.Equal(t, first[i].CostBasisValue, second[i].CostBasisValue, "disposal %d reported basis diverged across identical runs", i)
	}
}
