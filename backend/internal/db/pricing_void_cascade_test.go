package db

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// voidCascadeFixture is a book with one instrument priced in the setup
// currency, plus the repository under test.
type voidCascadeFixture struct {
	database    *sql.DB
	repository  *PricingRepository
	ownerID     int64
	baseID      int64
	quoteID     int64
	manualSrcID int64
}

func newVoidCascadeFixture(t *testing.T) voidCascadeFixture {
	t.Helper()

	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
	repository := NewPricingRepository(database)
	manualSourceID, err := repository.ManualSourceID(context.Background())
	require.NoError(t, err)

	return voidCascadeFixture{
		database:    database,
		repository:  repository,
		ownerID:     ownerID,
		baseID:      instrument.CommodityID,
		quoteID:     currencyID,
		manualSrcID: manualSourceID,
	}
}

// createObservation stores one observation. A non-empty legs list makes it a
// derived observation naming those observation ids, which is the shape
// fxDerivationJSON writes and the shape the void cascade follows.
func (f voidCascadeFixture) createObservation(t *testing.T, day int, legs ...int64) int64 {
	t.Helper()

	derivation := "{}"
	if len(legs) > 0 {
		encoded := ""
		for index, leg := range legs {
			if index > 0 {
				encoded += ","
			}
			encoded += fmt.Sprintf(`{"observation_id":%d}`, leg)
		}
		derivation = fmt.Sprintf(`{"kind":"triangulated_fx","legs":[%s]}`, encoded)
	}

	record, err := f.repository.CreatePriceObservation(context.Background(), CreatePriceObservationParams{
		BookID:       1,
		ActorUserID:  f.ownerID,
		RequestID:    "request-void-cascade",
		OriginType:   "browser_api",
		Operation:    "pricing.price.create",
		CreatedAt:    "2026-06-12T12:00:00Z",
		ChangeReason: "cascade fixture",
		Spec: PriceObservationSpec{
			BaseCommodityID:   f.baseID,
			QuoteCommodityID:  f.quoteID,
			QuoteType:         "manual",
			AdjustmentBasis:   "raw",
			PriceValue:        1000000,
			PriceScale:        6,
			BaseQuantityValue: 1,
			BaseQuantityScale: 0,
			ValuationDate:     fmt.Sprintf("2026-06-%02d", day),
			SourceID:          sql.NullInt64{Int64: f.manualSrcID, Valid: true},
			IsManual:          len(legs) == 0,
			IsDerived:         len(legs) > 0,
			DerivationJSON:    derivation,
			MetadataJSON:      `{}`,
		},
	})
	require.NoError(t, err)

	return record.ID
}

// createChain returns the root observation plus `length` observations each
// derived from the one before it.
func (f voidCascadeFixture) createChain(t *testing.T, length int) (int64, []int64) {
	t.Helper()

	root := f.createObservation(t, 1)
	chain := make([]int64, 0, length)
	previous := root
	for level := 1; level <= length; level++ {
		// Valuation dates only have to be distinct-ish and valid; the cascade
		// follows derivation metadata, not dates.
		next := f.createObservation(t, (level%28)+1, previous)
		chain = append(chain, next)
		previous = next
	}

	return root, chain
}

func (f voidCascadeFixture) void(t *testing.T, observationID int64) ([]PriceObservationRecord, error) {
	t.Helper()

	return f.repository.VoidPriceObservation(context.Background(), VoidPriceObservationParams{
		BookID:        1,
		ObservationID: observationID,
		ActorUserID:   f.ownerID,
		RequestID:     "request-void",
		OriginType:    "browser_api",
		Operation:     "pricing.price.void",
		VoidedAt:      "2026-06-20T12:00:00Z",
		VoidReason:    "bad quote",
		ChangeReason:  "bad quote",
	})
}

func (f voidCascadeFixture) countActive(t *testing.T) int {
	t.Helper()

	var count int
	require.NoError(t, f.database.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM price_observations WHERE book_id = 1 AND voided_at IS NULL`).Scan(&count))

	return count
}

func (f voidCascadeFixture) countAuditEvents(t *testing.T) int {
	t.Helper()

	var count int
	require.NoError(t, f.database.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM audit_events WHERE operation = 'pricing.price.void'`).Scan(&count))

	return count
}

// A chain exactly at the bound is fully reachable and must be voided whole.
func TestVoidPriceObservationCascadesAChainExactlyAtTheDepthLimit(t *testing.T) {
	fixture := newVoidCascadeFixture(t)
	root, chain := fixture.createChain(t, maxVoidCascadeDepth)

	records, err := fixture.void(t, root)

	require.NoError(t, err)
	assert.Len(t, records, maxVoidCascadeDepth+1)
	assert.Equal(t, 0, fixture.countActive(t))
	assert.Equal(t, root, records[0].ID)
	assert.Equal(t, chain[len(chain)-1], records[len(records)-1].ID)
}

// One level past the bound the traversal cannot prove it reached the end, so
// nothing is voided at all: a partially cascaded void would leave derived
// valuations live after their source was retired.
func TestVoidPriceObservationRefusesAChainBeyondTheDepthLimit(t *testing.T) {
	fixture := newVoidCascadeFixture(t)
	root, _ := fixture.createChain(t, maxVoidCascadeDepth+1)
	activeBefore := fixture.countActive(t)

	records, err := fixture.void(t, root)

	require.ErrorIs(t, err, ErrPriceVoidCascadeTooDeep)
	assert.Nil(t, records)
	assert.Equal(t, activeBefore, fixture.countActive(t), "no observation may be voided when the cascade aborts")
	assert.Equal(t, 0, fixture.countAuditEvents(t), "the audit event must roll back with the void")
}

// A cycle in the derivation metadata terminates on the visited set rather than
// on the depth bound, so it voids cleanly instead of being refused.
func TestVoidPriceObservationCascadesThroughACycleWithoutHittingTheDepthLimit(t *testing.T) {
	fixture := newVoidCascadeFixture(t)
	root := fixture.createObservation(t, 1)
	first := fixture.createObservation(t, 2, root)
	second := fixture.createObservation(t, 3, first)
	// Close the loop: the root is rewritten to derive from the last link.
	_, err := fixture.database.ExecContext(context.Background(), `
		UPDATE price_observations
		SET is_derived = 1, derivation_json = ?
		WHERE id = ?
	`, fmt.Sprintf(`{"kind":"triangulated_fx","legs":[{"observation_id":%d}]}`, second), root)
	require.NoError(t, err)

	records, err := fixture.void(t, root)

	require.NoError(t, err)
	assert.Len(t, records, 3)
	assert.Equal(t, 0, fixture.countActive(t))
}

// The cascade must reach every branch, not just the first: a wide graph inside
// the bound is voided whole.
func TestVoidPriceObservationCascadesAcrossBranches(t *testing.T) {
	fixture := newVoidCascadeFixture(t)
	root := fixture.createObservation(t, 1)
	left := fixture.createObservation(t, 2, root)
	right := fixture.createObservation(t, 3, root)
	joined := fixture.createObservation(t, 4, left, right)

	records, err := fixture.void(t, root)

	require.NoError(t, err)
	assert.Len(t, records, 4)
	assert.Equal(t, 0, fixture.countActive(t))
	assert.Equal(t, joined, records[len(records)-1].ID)
}
