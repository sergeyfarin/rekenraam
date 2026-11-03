package app

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/marketdata"
)

// #469: a manual/scheduled FX refresh must not report success when its item or
// source-health row could not be written. The first persistence failure stops
// the sweep, fails and finishes the persisted run, is returned to the caller,
// and leaves already stored observations intact.
func TestFXRefreshPropagatesItemAndStateWriteFailures(t *testing.T) {
	t.Parallel()
	rates := map[string]fakeFXRate{
		"EUR/USD": {value: 110, scale: 2},
		"USD/EUR": {value: 91, scale: 2},
	}
	for _, test := range []struct {
		name             string
		failingTable     string
		rates            map[string]fakeFXRate
		triggerCondition string
		wantItems        int
		wantStates       int
		wantObservations int
	}{
		{name: "item_write_on_success", failingTable: "market_data_ingest_items", rates: rates, wantObservations: 1},
		{name: "state_write_on_success", failingTable: "pricing_refresh_state", rates: rates, wantItems: 1, wantObservations: 1},
		{name: "item_write_on_provider_failure", failingTable: "market_data_ingest_items"},
		{name: "state_write_on_provider_failure", failingTable: "pricing_refresh_state", wantItems: 1},
		{name: "item_write_after_partial_provider_failure", failingTable: "market_data_ingest_items",
			rates:            map[string]fakeFXRate{"EUR/USD": {value: 110, scale: 2}},
			triggerCondition: "WHEN (SELECT count(*) FROM market_data_ingest_items) >= 1",
			wantItems:        1, wantStates: 1, wantObservations: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			provider := fakeFXProvider{code: "frankfurter", rates: test.rates}
			database, service := newPricingRefreshTestService(t, []string{"EUR", "USD"}, marketdata.NewRegistry(provider))
			_, err := database.Exec(fmt.Sprintf(`CREATE TRIGGER reject_refresh_write BEFORE INSERT ON %s %s BEGIN SELECT RAISE(ABORT, 'injected %s write failure'); END`, test.failingTable, test.triggerCondition, test.failingTable))
			require.NoError(t, err)

			run, err := service.RunRefresh(ctx, 1, 0, "manual")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "injected "+test.failingTable+" write failure")
			require.NotZero(t, run.ID)
			assert.Equal(t, "failed", run.Status)
			assert.NotEmpty(t, run.FinishedAt)
			assert.Contains(t, run.LastError, "injected "+test.failingTable+" write failure")

			var status string
			var lastError sql.NullString
			require.NoError(t, database.QueryRow(`SELECT status, last_error FROM market_data_ingest_runs WHERE id = ?`, run.ID).Scan(&status, &lastError))
			assert.Equal(t, "failed", status)
			require.True(t, lastError.Valid)
			assert.Contains(t, lastError.String, "injected "+test.failingTable+" write failure")

			assert.Equal(t, test.wantItems, countPricingRefreshRows(t, database, "market_data_ingest_items"))
			assert.Equal(t, test.wantStates, countPricingRefreshRows(t, database, "pricing_refresh_state"))
			assert.Equal(t, test.wantObservations, countPricingRefreshRows(t, database, "price_observations"))
		})
	}
}

func countPricingRefreshRows(t *testing.T, database *sql.DB, table string) int {
	t.Helper()
	var count int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM `+table).Scan(&count))
	return count
}
