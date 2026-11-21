package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
	"rekenraam/backend/internal/marketdata"
)

type massiveEventFixture struct {
	*investTestFixture
	pricing            *PricingService
	instrument         InvestmentInstrument
	holding            Account
	ratio              atomic.Int64
	missingListingDate atomic.Value
}

const (
	massiveEventDate       = "2026-06-16"
	massiveUnheldEventDate = "2026-01-15"
)

func newMassiveEventFixture(t *testing.T, key string) *massiveEventFixture {
	t.Helper()
	ctx := context.Background()
	f := &massiveEventFixture{investTestFixture: newInvestTestFixture(t)}
	f.ratio.Store(4)
	f.missingListingDate.Store("")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The single writer connection is free while fetching.
		var count int
		assert.NoError(t, f.database.QueryRowContext(r.Context(), `SELECT count(*) FROM transactions`).Scan(&count))
		switch r.URL.Path {
		case "/stocks/v1/splits":
			fmt.Fprintf(w, `{"status":"OK","results":[
				{"id":"fixture-split","ticker":"TEST","adjustment_type":"forward_split","execution_date":%q,"split_from":1,"split_to":%d,"historical_adjustment_factor":0.25},
				{"id":"fixture-unheld-split","ticker":"TEST","adjustment_type":"reverse_split","execution_date":%q,"split_from":10,"split_to":1},
				{"id":"fixture-stock-dividend","ticker":"TEST","adjustment_type":"stock_dividend","execution_date":%q,"split_from":1,"split_to":1.08}
			]}`, massiveEventDate, f.ratio.Load(), massiveUnheldEventDate, massiveEventDate)
		case "/v3/reference/tickers/TEST":
			if r.URL.Query().Get("date") == f.missingListingDate.Load().(string) {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			fmt.Fprint(w, `{"status":"OK","results":{"ticker":"TEST","primary_exchange":"XNAS","market":"stocks","locale":"us","share_class_figi":"fixture-figi"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	f.pricing = NewPricingService(db.NewPricingRepository(f.database), marketdata.NewRegistry(marketdata.NewMassiveProvider(server.Client(), server.URL, key)))
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	f.pricing.SetNowForTest(func() time.Time { return now })
	f.investmentSvc.SetNowForTest(func() time.Time { return now })
	var err error
	f.instrument, err = f.investmentSvc.CreateInstrument(ctx, InvestmentInstrumentInput{
		OwnerUserID: 1, OriginType: "browser_api", ChangeReason: "fixture", InstrumentType: "stock",
		CommodityCode: "MASSIVE-TEST", DisplayName: "Synthetic test listing", MIC: "XNAS",
		QuoteIdentifiers: map[string]string{"massive_symbol": "TEST"},
	})
	require.NoError(t, err)
	f.holding, err = f.investmentSvc.CreateHoldingAccount(ctx, HoldingAccountInput{OwnerUserID: 1,
		InstrumentID: f.instrument.ID, Name: "Test holding", OpenedOn: "2020-01-01", EffectiveFrom: "2020-01-01"})
	require.NoError(t, err)
	_, err = f.investmentSvc.Buy(ctx, InvestmentTradeInput{OwnerUserID: 1, TransactionDate: "2026-03-02",
		CommodityID: f.instrument.CommodityID, HoldingAccountID: f.holding.ID, CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.MustParse("10"), CashAmountValue: 600000, CashAmountScale: 2})
	require.NoError(t, err)
	return f
}

func (f *massiveEventFixture) fetch(t *testing.T) []InvestmentEventSuggestion {
	t.Helper()
	ctx := context.Background()
	queued, err := f.pricing.QueueSplitEventFetch(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 1, queued)
	f.pricing.runDueSplitEvents(ctx, testLogger(), "worker")
	suggestions, err := f.investmentSvc.ListEventSuggestions(ctx)
	require.NoError(t, err)
	return suggestions
}

func (f *massiveEventFixture) requireCompletedSplitChecks(t *testing.T, want int) {
	t.Helper()
	var count int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM background_work_items WHERE kind = ? AND status = 'completed'`,
		splitEventWorkKind).Scan(&count))
	require.Equal(t, want, count)
}

func TestMassiveSplitSuggestionAcceptsOnceAndRetainsRevisions(t *testing.T) {
	t.Parallel()
	f := newMassiveEventFixture(t, "synthetic-key")
	ctx := context.Background()
	suggestions := f.fetch(t)
	require.Len(t, suggestions, 1)
	proposal := suggestions[0]
	assert.Equal(t, "split", proposal.EventFamily)
	assert.Equal(t, "suggested", proposal.Status)
	assert.Contains(t, proposal.EventEvidenceJSON, `"ratio_numerator":4`)
	assert.Zero(t, countSplitRows(t, f.database).facts, "fetch never posts")
	accepted, err := f.investmentSvc.AcceptSuggestion(ctx, 1, 0, "request", proposal.ID)
	require.NoError(t, err)
	assert.Equal(t, "accepted", accepted.Status)
	assert.Equal(t, "matched", accepted.EventStatus)
	require.NotNil(t, accepted.GeneratedTransactionID)
	lots, err := f.investmentSvc.ListLots(ctx, f.holding.ID, f.instrument.CommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	assert.Equal(t, "40", lots[0].RemainingQuantityValue.String())
	assert.Equal(t, int64(600000), lots[0].RemainingCostBasisValue)
	_, err = f.investmentSvc.AcceptSuggestion(ctx, 1, 0, "request", proposal.ID)
	assert.ErrorIs(t, err, ErrInvestmentSuggestionNotPending)
	f.ratio.Store(5)
	f.fetch(t)
	suggestions = f.fetch(t)
	require.Len(t, suggestions, 1)
	assert.Equal(t, "accepted", suggestions[0].Status)
	require.Len(t, suggestions[0].Revisions, 1)
	assert.Contains(t, suggestions[0].Revisions[0].EvidenceJSON, `"ratio_numerator":5`)
	assert.Equal(t, 1, countSplitRows(t, f.database).facts)
}

func TestMassiveSplitSuggestionLinksExistingSplitWithoutPosting(t *testing.T) {
	t.Parallel()
	f := newMassiveEventFixture(t, "synthetic-key")
	ctx := context.Background()
	suggestions := f.fetch(t)
	require.Len(t, suggestions, 1)
	manual, err := f.investmentSvc.Split(ctx, InvestmentSplitInput{OwnerUserID: 1, EffectiveOn: massiveEventDate,
		HoldingAccountID: f.holding.ID, CommodityID: f.instrument.CommodityID,
		RatioNumerator: 4, RatioDenominator: 1, SourceEvidenceJSON: `{"test":true}`, ChangeReason: "fixture"})
	require.NoError(t, err)
	before := countSplitRows(t, f.database)
	_, err = f.investmentSvc.AcceptSuggestion(ctx, 1, 0, "", suggestions[0].ID)
	require.ErrorContains(t, err, "requires manual review")
	assert.Equal(t, before, countSplitRows(t, f.database))
	suggestions = f.fetch(t)
	require.Len(t, suggestions[0].SplitLinkCandidates, 1)
	assert.Equal(t, manual.Transaction.ID, suggestions[0].SplitLinkCandidates[0].TransactionID)
	linked, err := f.investmentSvc.LinkSuggestion(ctx, 1, 0, "", suggestions[0].ID, manual.Transaction.ID)
	require.NoError(t, err)
	assert.Equal(t, "linked", linked.Status)
	assert.Equal(t, "matched", linked.EventStatus)
	assert.Equal(t, before.transactions, countSplitRows(t, f.database).transactions)
}

func TestMassiveSplitSuggestionRollsBackOnSettlementFailure(t *testing.T) {
	t.Parallel()
	f := newMassiveEventFixture(t, "synthetic-key")
	suggestions := f.fetch(t)
	require.Len(t, suggestions, 1)
	_, err := f.database.Exec(`CREATE TRIGGER refuse_split_settlement BEFORE UPDATE ON investment_event_suggestions
		WHEN NEW.status = 'accepted' BEGIN SELECT RAISE(ABORT, 'split settlement failed'); END`)
	require.NoError(t, err)
	before := countSplitRows(t, f.database)
	_, err = f.investmentSvc.AcceptSuggestion(context.Background(), 1, 0, "", suggestions[0].ID)
	require.ErrorContains(t, err, "split settlement failed")
	assert.Equal(t, before, countSplitRows(t, f.database))
	suggestions = f.fetch(t)
	assert.Equal(t, "suggested", suggestions[0].Status)
}

func TestMassiveSplitSuggestionRequiresManualReviewAfterRevision(t *testing.T) {
	t.Parallel()
	f := newMassiveEventFixture(t, "synthetic-key")
	f.fetch(t)
	f.ratio.Store(5)
	suggestions := f.fetch(t)
	require.Len(t, suggestions, 1)
	require.Len(t, suggestions[0].Revisions, 1)
	_, err := f.investmentSvc.AcceptSuggestion(context.Background(), 1, 0, "", suggestions[0].ID)
	require.ErrorContains(t, err, "requires manual review")
	assert.Zero(t, countSplitRows(t, f.database).facts)
}

func TestMassiveMissingKeyFailsDurableWork(t *testing.T) {
	t.Parallel()
	f := newMassiveEventFixture(t, "")
	assert.Empty(t, f.fetch(t))
	failed, err := f.pricing.FailedBackgroundWork(context.Background())
	require.NoError(t, err)
	require.Len(t, failed, 1)
	assert.Equal(t, splitEventWorkKind, failed[0].Kind)
	assert.Contains(t, failed[0].LastError, "missing operator secret")
}

func TestMassiveSplitFetchKeepsTheMarketDateAcrossMidnight(t *testing.T) {
	t.Parallel()
	f := newMassiveEventFixture(t, "synthetic-key")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/stocks/v1/splits", r.URL.Path)
		assert.Equal(t, "2025-10-16", r.URL.Query().Get("execution_date.gte"))
		assert.Equal(t, "2026-10-16", r.URL.Query().Get("execution_date.lte"))
		fmt.Fprint(w, `{"status":"OK","results":[]}`)
	}))
	t.Cleanup(server.Close)
	f.pricing.providerRegistry = marketdata.NewRegistry(marketdata.NewMassiveProvider(server.Client(), server.URL, "synthetic-key"))
	beforeMidnight := time.Date(2026, 10, 17, 3, 59, 59, 0, time.UTC)
	now := beforeMidnight
	f.pricing.SetNowForTest(func() time.Time {
		current := now
		now = beforeMidnight.Add(2 * time.Second)
		return current
	})
	require.NoError(t, f.pricing.fetchSplitEvents(context.Background(), db.BackgroundWorkItemRecord{
		PayloadVersion: 1,
		PayloadJSON:    fmt.Sprintf(`{"instrument_id":%d}`, f.instrument.ID),
	}))
}

func TestMassiveSplitUnheldUnverifiedListingDoesNotFailFetch(t *testing.T) {
	t.Parallel()
	f := newMassiveEventFixture(t, "synthetic-key")
	f.missingListingDate.Store(massiveUnheldEventDate)

	suggestions := f.fetch(t)
	require.Len(t, suggestions, 1)
	assert.Equal(t, massiveEventDate, suggestions[0].EventDate)
	assert.NotContains(t, suggestions[0].ProposedTransactionJSON, "review_reason")
	f.requireCompletedSplitChecks(t, 1)
}

func TestMassiveSplitHeldUnverifiedListingRequiresManualReview(t *testing.T) {
	t.Parallel()
	f := newMassiveEventFixture(t, "synthetic-key")
	f.missingListingDate.Store(massiveEventDate)

	suggestions := f.fetch(t)
	require.Len(t, suggestions, 1)
	assert.Contains(t, suggestions[0].ProposedTransactionJSON, `"review_reason":"identity"`)
	assert.Empty(t, suggestions[0].SplitLinkCandidates)
	_, err := f.investmentSvc.AcceptSuggestion(context.Background(), 1, 0, "", suggestions[0].ID)
	require.ErrorContains(t, err, "requires manual review")
	assert.Zero(t, countSplitRows(t, f.database).facts)
	f.requireCompletedSplitChecks(t, 1)
}

func TestMassiveSplitRecordsProviderCorrectionDespiteUnverifiedListing(t *testing.T) {
	t.Parallel()
	f := newMassiveEventFixture(t, "synthetic-key")
	ctx := context.Background()
	require.Len(t, f.fetch(t), 1)
	var raw string
	require.NoError(t, f.database.QueryRow(`SELECT raw_json FROM investment_provider_events`).Scan(&raw))
	assert.Contains(t, raw, `"historical_adjustment_factor":0.25`, "retain the provider row, including unparsed fields")

	f.missingListingDate.Store(massiveEventDate)
	suggestions := f.fetch(t)
	require.Len(t, suggestions, 1)
	assert.Empty(t, suggestions[0].Revisions)
	assert.NotContains(t, suggestions[0].ProposedTransactionJSON, "review_reason")

	f.ratio.Store(5)
	f.fetch(t)
	suggestions = f.fetch(t)
	require.Len(t, suggestions, 1)
	require.Len(t, suggestions[0].Revisions, 1)
	assert.Contains(t, suggestions[0].EventEvidenceJSON, `"ratio_numerator":4`)
	assert.Contains(t, suggestions[0].Revisions[0].EvidenceJSON, `"ratio_numerator":5`)
	f.requireCompletedSplitChecks(t, 4)

	_, err := f.investmentSvc.AcceptSuggestion(ctx, 1, 0, "request", suggestions[0].ID)
	require.ErrorContains(t, err, "requires manual review")
	assert.Zero(t, countSplitRows(t, f.database).facts)
	lots, err := f.investmentSvc.ListLots(ctx, f.holding.ID, f.instrument.CommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	assert.Equal(t, "10", lots[0].RemainingQuantityValue.String())
}
