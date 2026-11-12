package app

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
)

// Opt-in cost probe for #561 and the #562 review: run with SPLIT_QUERY_COST=1
// and optionally SPLIT_QUERY_OPS (buys, default 500), SPLIT_QUERY_SPLITS
// (splits, default 3; alternating 2:1 and 1:2 so quantities stay bounded) and
// SPLIT_QUERY_REPLACEMENTS (times the last split is replaced, default 0).
func TestSplitQueryCost(t *testing.T) {
	t.Parallel()
	if os.Getenv("SPLIT_QUERY_COST") != "1" {
		t.Skip("set SPLIT_QUERY_COST=1 to run the #561 split query cost probe")
	}
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	setting := func(name string, fallback int) int {
		if n, err := strconv.Atoi(os.Getenv(name)); err == nil {
			return n
		}
		return fallback
	}
	operations := setting("SPLIT_QUERY_OPS", 500)
	splitCount := setting("SPLIT_QUERY_SPLITS", 3)
	replacements := setting("SPLIT_QUERY_REPLACEMENTS", 0)
	start := time.Now()
	day := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	for i := 0; i < operations; i++ {
		buyOn(t, f, day.AddDate(0, 0, i).Format("2006-01-02"), 1, 1000)
	}
	splitDays := []string{}
	var lastSplitID int64
	for i := 0; i < splitCount; i++ {
		date := day.AddDate(0, 0, operations+10*(i+1)).Format("2006-01-02")
		numerator, denominator := int64(2), int64(1)
		if i%2 == 1 {
			numerator, denominator = 1, 2
		}
		result, err := acknowledgedSplit(ctx, f.investmentService, splitInput(f, date, numerator, denominator))
		require.NoError(t, err)
		lastSplitID = result.OperationID
		splitDays = append(splitDays, date)
	}
	for i := 0; i < replacements; i++ {
		numerator := int64(3)
		if i%2 == 1 {
			numerator = 2
		}
		if (splitCount-1)%2 == 1 {
			numerator = 1
		}
		denominator := int64(1)
		if (splitCount-1)%2 == 1 {
			denominator = 2
			if i%2 == 0 {
				denominator = 3
			}
		}
		replaced, err := f.investmentService.ReplaceSplit(ctx, ReplaceInvestmentSplitInput{OwnerUserID: f.ownerUserID,
			OperationID: lastSplitID, Reason: "probe replacement", EffectiveOn: splitDays[len(splitDays)-1],
			RatioNumerator: numerator, RatioDenominator: denominator})
		require.NoError(t, err)
		lastSplitID = replaced.OperationID
	}
	var bookOperations, splitFacts int
	require.NoError(t, f.database.QueryRow(`SELECT (SELECT count(*) FROM investment_operations WHERE book_id = ?),
		(SELECT count(*) FROM investment_split_facts WHERE book_id = ?)`, BookID, BookID).Scan(&bookOperations, &splitFacts))
	t.Logf("fixture: buys=%d splits=%d replacements=%d book_operations=%d split_facts=%d built in %s",
		operations, len(splitDays), replacements, bookOperations, splitFacts, time.Since(start))
	var version string
	var qpsg int
	require.NoError(t, f.database.QueryRow("SELECT sqlite_version(), sqlite_compileoption_used('ENABLE_QPSG')").Scan(&version, &qpsg))
	repository := db.NewPricingRepository(f.database)
	commodities := []int64{f.stockCommodityID, f.eurCommodityID}
	splits, err := repository.InstrumentSplits(ctx, BookID, commodities, "2000-01-01", "2100-01-01")
	require.NoError(t, err)
	require.Len(t, splits, splitCount)
	bases, err := repository.TradeQuotePendingSplits(ctx, BookID, commodities, "2000-01-01", "2100-01-01")
	require.NoError(t, err)
	measure := func(name string, iterations int, fn func() error) {
		for i := 0; i < 5; i++ {
			require.NoError(t, fn())
		}
		start := time.Now()
		for i := 0; i < iterations; i++ {
			require.NoError(t, fn())
		}
		t.Logf("engine=%s qpsg=%d operations=%d splits=%d replacements=%d %s: per_op=%.2fus", version, qpsg, operations, splitCount, replacements, name, float64(time.Since(start).Microseconds())/float64(iterations))
	}
	measure("InstrumentSplits (2 ids)", 200, func() error {
		_, err := repository.InstrumentSplits(ctx, BookID, commodities, "2000-01-01", "2100-01-01")
		return err
	})
	measure("TradeQuotePendingSplits (2 ids)", 200, func() error {
		_, err := repository.TradeQuotePendingSplits(ctx, BookID, commodities, "2000-01-01", "2100-01-01")
		return err
	})
	t.Logf("result rows: splits=%d trade_quote_bases=%d", len(splits), len(bases))
}
