package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	sqlite "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/marketdata"
)

func TestFXRefreshEmptyDemandIsReadOnceAcrossTriangulationAttempts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newPostedCurrencyFixture(t)
	f.pricing = NewPricingService(db.NewPricingRepository(f.database), marketdata.NewRegistry())
	f.pricing.SetNowForTest(func() time.Time { return time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC) })
	_, err := f.pricing.SavePolicy(ctx, PricingPolicyInput{OwnerUserID: 1, BaseCommodityID: &f.foreign, TriangulationMaxHops: 1})
	require.NoError(t, err)

	// No statement cache is enabled. Observe compilation of the demand's
	// posting-column read, without replacing any query or its result.
	var reads atomic.Int64
	connection, err := f.database.Conn(ctx)
	require.NoError(t, err)
	require.NoError(t, connection.Raw(func(raw any) error {
		raw.(*sqlite.SQLiteConn).RegisterAuthorizer(func(operation int, table, column, _ string) int {
			if operation == sqlite.SQLITE_READ && table == "posting_versions" && column == "commodity_id" {
				reads.Add(1)
			}
			return sqlite.SQLITE_OK
		})
		return nil
	}))
	require.NoError(t, connection.Close())
	t.Cleanup(func() {
		connection, err := f.database.Conn(ctx)
		require.NoError(t, err)
		require.NoError(t, connection.Raw(func(raw any) error { raw.(*sqlite.SQLiteConn).RegisterAuthorizer(nil); return nil }))
		require.NoError(t, connection.Close())
	})
	demand, err := f.pricing.fxDemand(ctx)
	require.NoError(t, err)
	require.Empty(t, demand)
	oneRead := reads.Load()
	require.Positive(t, oneRead)
	_, err = f.pricing.fxDemand(ctx)
	require.NoError(t, err)
	require.Equal(t, 2*oneRead, reads.Load(), "negative control detects another demand query")
	reads.Store(0)

	run, err := f.pricing.runFXRefresh(ctx, 1, 0, "manual")
	require.NoError(t, err)
	require.Equal(t, 2, run.ItemsFailed, "the configured FX base and book default exercise two unsuccessful derivations")
	assert.Equal(t, oneRead, reads.Load(), "a loaded empty demand must not become an uninitialized demand on each attempt")
}

func TestFXRefreshDemandUsesOwnerDayCapturedAtRunStart(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"latest", "durable"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			database, service := newPricingRefreshTestService(t, []string{"EUR", "USD"}, marketdata.NewRegistry(fakeFXProvider{
				code: "frankfurter", rates: map[string]fakeFXRate{"EUR/USD": {value: 110, scale: 2}, "USD/EUR": {value: 91, scale: 2}},
			}))
			_, err := database.Exec(`INSERT INTO commodity_versions(commodity_id,version_seq,effective_from,recorded_at,changed_by_user_id,change_reason,status,symbol,display_symbol,name,standard_scale,max_quantity_scale)
		VALUES(2,2,'2026-06-13','2026-06-12T12:00:00Z',1,'next-day archive','archived','USD','USD','US Dollar',2,8)`)
			require.NoError(t, err)
			var calls atomic.Int64
			service.SetNowForTest(func() time.Time {
				if calls.Add(1) == 1 {
					return time.Date(2026, 6, 12, 23, 59, 59, 0, time.UTC)
				}
				return time.Date(2026, 6, 13, 0, 0, 1, 0, time.UTC)
			})
			var run PricingRefreshRun
			if mode == "latest" {
				run, err = service.runFXRefresh(ctx, 1, marketDataSourceIDByCode(t, database, "frankfurter"), "manual")
			} else {
				run, _, err = service.runFXCoverage(ctx, 1, fxCoverageWorkPayload{Reason: "manual"})
			}
			require.NoError(t, err)
			assert.Equal(t, "2026-06-12T23:59:59Z", run.StartedAt, "the reference day and run timestamp use the same captured instant")
			assert.Equal(t, 2, run.ItemsSucceeded, "a midnight crossing must retain the active currency set on the run's start day")
			assert.Equal(t, "succeeded", run.Status)
		})
	}
}
