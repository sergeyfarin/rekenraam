package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// T-99. Market value and unrealized gain are reported through int64 columns, so
// a value can be absent for two very different reasons: nobody has priced the
// position, or the read model cannot represent what it computed. Collapsing
// both into "no value" hid a six-figure position that had a perfectly good
// price behind the same placeholder as an unpriced one.

func TestInt64AtUsableScaleKeepsTheComputedScaleWhenItFits(t *testing.T) {
	value, scale, fits := int64AtUsableScale(exact.ScaledIntFromInt64(10000000, 2))
	require.True(t, fits)
	require.Equal(t, int64(10000000), value)
	require.Equal(t, 2, scale, "a value that already fits is reported exactly as computed")
}

func TestInt64AtUsableScaleDropsOnlyRedundantZeros(t *testing.T) {
	// 999.999999 shares × 100 EUR, carried at the sum of both scales.
	padded := exact.ScaledIntFromCoefficient(exact.MustParse("9999999990000000000"), 14)
	value, scale, fits := int64AtUsableScale(padded)
	require.True(t, fits)
	require.Equal(t, int64(999999999), value)
	require.Equal(t, 4, scale)
	require.Equal(t, 0, exact.ScaledIntFromInt64(value, scale).Cmp(padded), "restating must not change the value")
}

func TestInt64AtUsableScaleReportsAGenuinelyTooWideValue(t *testing.T) {
	// Significant digits, not trailing zeros: nothing can be removed.
	wide := exact.ScaledIntFromCoefficient(exact.MustParse("123456789012345678901"), 4)
	_, _, fits := int64AtUsableScale(wide)
	require.False(t, fits, "a value this wide is unrepresentable, not merely padded")
}

func TestPositionsWithGainsReportsAnUnpricedPositionAsSuch(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID := createInvestmentTestAccount(t, database, currencyID)
	instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
	repository := NewInvestmentRepository(database)

	_, err := repository.CreateLot(ctx, CreateInvestmentLotParams{
		BookID:          1,
		AccountID:       accountID,
		CommodityID:     instrument.CommodityID,
		OpenedOn:        "2026-01-01",
		QuantityValue:   exact.New(10),
		QuantityScale:   0,
		CostBasisValue:  100000,
		CostBasisScale:  2,
		CostCommodityID: currencyID,
		MetadataJSON:    `{}`,
		CreatedAt:       "2026-01-01T09:00:00Z",
		CreatedByUserID: ownerID,
		OriginType:      "browser_api",
		Operation:       "investment.lot.acquire",
		ChangeReason:    "unpriced lot",
		EventKind:       "acquisition",
	})
	require.NoError(t, err)

	records, err := repository.PositionsWithGains(ctx, 1)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Nil(t, records[0].MarketValueValue)
	require.Nil(t, records[0].UnrealizedGainValue)
	require.Equal(t, ValuationNoPrice, records[0].ValuationUnavailable)
}
