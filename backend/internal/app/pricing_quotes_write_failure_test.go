package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/marketdata"
)

// #474: a quote refresh whose item or source-health write fails must finish its
// ingest run as failed instead of leaving it running. The persistence error is
// returned, a stored observation is kept, and the fetch error stays in the
// result so terminal provider failures still fail the work and other
// persistence failures still retry.
func TestQuoteRefreshFinishesRunOnItemAndStateWriteFailures(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name             string
		failingTable     string
		providerStatus   int
		wantItems        int
		wantObservations int
		wantWorkStatus   string
	}{
		{name: "item_write_on_success", failingTable: "market_data_ingest_items", providerStatus: http.StatusOK, wantObservations: 1, wantWorkStatus: "pending"},
		{name: "state_write_on_success", failingTable: "pricing_refresh_state", providerStatus: http.StatusOK, wantItems: 1, wantObservations: 1, wantWorkStatus: "pending"},
		{name: "item_write_on_terminal_provider_failure", failingTable: "market_data_ingest_items", providerStatus: http.StatusUnauthorized, wantWorkStatus: "failed"},
		{name: "state_write_on_terminal_provider_failure", failingTable: "pricing_refresh_state", providerStatus: http.StatusUnauthorized, wantItems: 1, wantWorkStatus: "failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			f := newInvestTestFixture(t)
			instrument, _ := cryptoTestHolding(t, f)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.providerStatus)
				if test.providerStatus == http.StatusOK {
					_, _ = w.Write([]byte(`{"bitcoin":{"eur":65000,"last_updated_at":1781265600}}`))
				}
			}))
			defer server.Close()
			service := NewPricingService(db.NewPricingRepository(f.database), marketdata.NewRegistry(marketdata.NewCoinGeckoProvider(server.Client(), server.URL, "secret")))
			now := time.Date(2026, 6, 12, 12, 10, 0, 0, time.UTC)
			service.SetNowForTest(func() time.Time { return now })
			var sourceID int64
			require.NoError(t, f.database.QueryRow(`SELECT id FROM market_data_sources WHERE code='coingecko'`).Scan(&sourceID))
			_, err := service.SaveSourceAssignment(ctx, PricingSourceAssignmentInput{OwnerUserID: 1, CommodityID: instrument.CommodityID, QuoteCommodityID: f.eurCommodityID, SourceID: sourceID, EffectiveFrom: "2020-01-01"})
			require.NoError(t, err)
			_, err = f.database.Exec(fmt.Sprintf(`CREATE TRIGGER reject_refresh_write BEFORE INSERT ON %s BEGIN SELECT RAISE(ABORT, 'injected %s write failure'); END`, test.failingTable, test.failingTable))
			require.NoError(t, err)
			injected := "injected " + test.failingTable + " write failure"

			targets, err := service.currentQuoteTargets(ctx, 0)
			require.NoError(t, err)
			require.Len(t, targets, 1)
			err = service.refreshQuoteTarget(ctx, targets[0], "manual")
			require.Error(t, err)
			assert.Contains(t, err.Error(), injected)
			var providerErr *marketdata.QuoteError
			assert.Equal(t, test.providerStatus != http.StatusOK, errors.As(err, &providerErr), "the fetch outcome stays classifiable")

			var status string
			var finishedAt, lastError sql.NullString
			require.NoError(t, f.database.QueryRow(`SELECT status, finished_at, last_error FROM market_data_ingest_runs`).Scan(&status, &finishedAt, &lastError))
			assert.Equal(t, "failed", status)
			assert.True(t, finishedAt.Valid)
			require.True(t, lastError.Valid)
			assert.Contains(t, lastError.String, injected)
			assert.Equal(t, test.wantItems, countPricingRefreshRows(t, f.database, "market_data_ingest_items"))
			assert.Zero(t, countPricingRefreshRows(t, f.database, "pricing_refresh_state"))
			assert.Equal(t, test.wantObservations, countPricingRefreshRows(t, f.database, "price_observations"))

			// Through the worker, the unchanged retry policy applies and no
			// attempt leaves an orphaned running run behind.
			_, err = service.QueueQuoteRefresh(ctx, 1)
			require.NoError(t, err)
			service.runDueQuoteRefresh(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), "worker")
			items, err := service.repository.ListBackgroundWorkByStatus(ctx, BookID, quoteRefreshWorkKind, test.wantWorkStatus, 50)
			require.NoError(t, err)
			require.Len(t, items, 1)
			assert.Contains(t, items[0].LastError.String, injected)
			var running int
			require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM market_data_ingest_runs WHERE status = 'running' OR finished_at IS NULL`).Scan(&running))
			assert.Zero(t, running)
			assert.Equal(t, test.wantObservations, countPricingRefreshRows(t, f.database, "price_observations"), "a retry deduplicates the stored quote")
		})
	}
}
