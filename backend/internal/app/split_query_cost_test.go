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

// Opt-in cost probe for #561: run with SPLIT_QUERY_COST=1 [SPLIT_QUERY_OPS=n].
func TestSplitQueryCost(t *testing.T) {
	t.Parallel()
	if os.Getenv("SPLIT_QUERY_COST") != "1" {
		t.Skip("set SPLIT_QUERY_COST=1 to run the #561 split query cost probe")
	}
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	operations := 500
	if n, err := strconv.Atoi(os.Getenv("SPLIT_QUERY_OPS")); err == nil {
		operations = n
	}
	start := time.Now()
	day := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	for i := 0; i < operations; i++ {
		buyOn(t, f, day.AddDate(0, 0, i).Format("2006-01-02"), 1, 1000)
	}
	splitDays := []string{}
	for i := 0; i < 3; i++ {
		date := day.AddDate(0, 0, operations+10*(i+1)).Format("2006-01-02")
		_, err := acknowledgedSplit(ctx, f.investmentService, splitInput(f, date, 2, 1))
		require.NoError(t, err)
		splitDays = append(splitDays, date)
	}
	t.Logf("fixture: operations=%d splits=%d built in %s", operations, len(splitDays), time.Since(start))
	var version string
	var qpsg int
	require.NoError(t, f.database.QueryRow("SELECT sqlite_version(), sqlite_compileoption_used('ENABLE_QPSG')").Scan(&version, &qpsg))
	repository := db.NewPricingRepository(f.database)
	commodities := []int64{f.stockCommodityID, f.eurCommodityID}
	splits, err := repository.InstrumentSplits(ctx, BookID, commodities, "2000-01-01", "2100-01-01")
	require.NoError(t, err)
	require.Len(t, splits, 3)
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
		t.Logf("engine=%s qpsg=%d operations=%d %s: per_op=%.2fus", version, qpsg, operations, name, float64(time.Since(start).Microseconds())/float64(iterations))
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
