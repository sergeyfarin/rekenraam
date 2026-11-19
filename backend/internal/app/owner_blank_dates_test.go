package app

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/marketdata"
)

func TestBlankTransactionAndLedgerDatesFollowOwnerDay(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, instant, zone, today string }{
		{"ahead just after local midnight", "2026-10-18T22:01:00Z", "Europe/Amsterdam", "2026-10-19"},
		{"behind UTC midnight", "2026-10-19T03:00:00Z", "America/Los_Angeles", "2026-10-18"},
		{"no preferences retains UTC default", "2026-10-18T22:01:00Z", "", "2026-10-18"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			f := newInvestmentsTestFixture(t)
			now, err := time.Parse(time.RFC3339, tc.instant)
			require.NoError(t, err)
			if tc.zone != "" {
				setOwnerTimeZone(t, f.database, tc.zone)
			}
			s := f.transactionService
			s.SetSettings(NewSettingsService(db.NewSettingsRepository(f.database)))
			s.now = func() time.Time { return now }
			input := CreateTransactionInput{
				OwnerUserID: f.ownerUserID, OriginType: "browser_api",
				Spec: TransactionInput{JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
					{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: "100", QuantityScale: 2},
					{AccountID: f.incomeAccountID, CommodityID: f.eurCommodityID, QuantityValue: "-100", QuantityScale: 2},
				}}}},
			}
			created, err := s.CreateTransaction(ctx, input)
			require.NoError(t, err)
			assert.Equal(t, tc.today, created.TransactionDate)
			assert.Equal(t, tc.today, created.JournalEntries[0].EntryDate)

			// A dated posting on the owner's today must be included, while a
			// posting on their tomorrow must stay outside every blank read.
			input.Spec.TransactionDate = tc.today
			_, err = s.CreateTransaction(ctx, input)
			require.NoError(t, err)
			day, err := time.Parse(time.DateOnly, tc.today)
			require.NoError(t, err)
			input.Spec.TransactionDate = day.AddDate(0, 0, 1).Format(time.DateOnly)
			_, err = s.CreateTransaction(ctx, input)
			require.NoError(t, err)

			balances, err := s.AccountBalances(ctx, AccountBalancesInput{OwnerUserID: f.ownerUserID})
			require.NoError(t, err)
			assert.Equal(t, tc.today, balances.AsOf)
			for _, account := range balances.Accounts {
				if account.AccountID == f.cashAccountID {
					require.Len(t, account.DirectBalances, 1)
					assert.Equal(t, "200", account.DirectBalances[0].QuantityValue.String())
				}
			}
			explicitBalances, err := s.AccountBalances(ctx, AccountBalancesInput{OwnerUserID: f.ownerUserID, AsOf: "2026-10-01"})
			require.NoError(t, err)
			assert.Equal(t, "2026-10-01", explicitBalances.AsOf)
			explicitWorth, err := s.NetWorth(ctx, NetWorthInput{OwnerUserID: f.ownerUserID, AsOf: "2026-10-01"})
			require.NoError(t, err)
			assert.Equal(t, "2026-10-01", explicitWorth.AsOf)
			assert.Empty(t, explicitWorth.Totals)
			explicitCategories, err := s.CategoryTotals(ctx, CategoryTotalsInput{OwnerUserID: f.ownerUserID, BeforeDate: "2026-10-01"})
			require.NoError(t, err)
			assert.Equal(t, "2026-10-01", explicitCategories.BeforeDate)
			worth, err := s.NetWorth(ctx, NetWorthInput{OwnerUserID: f.ownerUserID})
			require.NoError(t, err)
			assert.Equal(t, tc.today, worth.AsOf)
			require.Len(t, worth.Totals, 1)
			assert.Equal(t, "200", worth.Totals[0].QuantityValue.String())
			categories, err := s.CategoryTotals(ctx, CategoryTotalsInput{OwnerUserID: f.ownerUserID})
			require.NoError(t, err)
			assert.Equal(t, tc.today, categories.BeforeDate)
			for _, category := range categories.Categories {
				if category.CategoryID == f.incomeAccountID {
					require.Len(t, category.DirectTotals, 1)
					assert.Equal(t, "-200", category.DirectTotals[0].QuantityValue.String())
				}
			}

			// Explicit transaction and journal dates keep their precedence.
			input.Spec.TransactionDate = "2026-10-01"
			input.Spec.JournalEntries[0].EntryDate = "2026-10-02"
			explicit, err := s.CreateTransaction(ctx, input)
			require.NoError(t, err)
			assert.Equal(t, "2026-10-01", explicit.TransactionDate)
			assert.Equal(t, "2026-10-02", explicit.JournalEntries[0].EntryDate)
			input.Spec.TransactionDate = ""
			fromEntry, err := s.CreateTransaction(ctx, input)
			require.NoError(t, err)
			assert.Equal(t, "2026-10-02", fromEntry.TransactionDate)
			input.Spec.JournalEntries[0].EntryDate = ""
			updated, err := s.UpdateTransaction(ctx, UpdateTransactionInput{
				OwnerUserID: f.ownerUserID, TransactionID: fromEntry.ID,
				ExpectedVersionID: fromEntry.VersionID, OriginType: "browser_api", Spec: input.Spec,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.today, updated.TransactionDate)
			assert.Equal(t, tc.today, updated.JournalEntries[0].EntryDate)
		})
	}
}

func TestUndatedFXRefreshSelectsAssignmentsOnOwnerDay(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, instant, zone, today string }{
		{"ahead just after local midnight", "2026-10-18T22:01:00Z", "Europe/Amsterdam", "2026-10-19"},
		{"behind UTC midnight", "2026-10-19T03:00:00Z", "America/Los_Angeles", "2026-10-18"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			registry := marketdata.NewRegistry(
				fakeFXProvider{code: "ecb_euro_reference_rates", rates: map[string]fakeFXRate{"USD/EUR": {value: 90, scale: 2}}},
				fakeFXProvider{code: "frankfurter", rates: map[string]fakeFXRate{"USD/EUR": {value: 95, scale: 2}}},
			)
			database, s := newPricingRefreshTestService(t, []string{"USD", "EUR"}, registry)
			now, err := time.Parse(time.RFC3339, tc.instant)
			require.NoError(t, err)
			setOwnerTimeZone(t, database, tc.zone)
			s.SetSettings(NewSettingsService(db.NewSettingsRepository(database)))
			s.SetNowForTest(func() time.Time { return now })
			ecbID := marketDataSourceIDByCode(t, database, "ecb_euro_reference_rates")
			assignedID := marketDataSourceIDByCode(t, database, "frankfurter")
			assignment, err := s.SaveSourceAssignment(ctx, PricingSourceAssignmentInput{
				OwnerUserID: 1, CommodityID: 1, QuoteCommodityID: 2, SourceID: assignedID,
				EffectiveTo: tc.today,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.today, assignment.EffectiveFrom)
			policy := PricingPolicy{DefaultSourceID: &ecbID}
			target := fxRefreshTarget{Base: db.PricingCurrencyRecord{ID: 1, Code: "USD"}, Quote: db.PricingCurrencyRecord{ID: 2, Code: "EUR"}}
			outcome, err := s.fetchAndStoreDirectFX(ctx, 1, target, policy, 0, "", now)
			require.NoError(t, err)
			assert.Equal(t, assignedID, outcome.SourceID)
			assert.Equal(t, int64(95), outcome.Observation.PriceValue)
			// A historical refresh resolves assignments at the requested date.
			outcome, err = s.fetchAndStoreDirectFX(ctx, 1, target, policy, 0, "2026-10-01", now)
			require.NoError(t, err)
			assert.Equal(t, ecbID, outcome.SourceID)
		})
	}
}

// #605 review. A refresh pins the owner day once; if the owner's time zone
// changes mid-run, undated FX work still uses the pinned day rather than
// resolving the same instant in the new zone. Here the run pinned the 18th
// (UTC) and the zone is now Amsterdam, where it is the 19th.
func newPinnedDayFXService(t *testing.T, ecbPublishedAt string) (*sql.DB, *PricingService, time.Time) {
	t.Helper()
	registry := marketdata.NewRegistry(
		fakeFXProvider{code: "ecb_euro_reference_rates", rates: map[string]fakeFXRate{"USD/EUR": {value: 90, scale: 2, publishedAt: ecbPublishedAt}}},
		fakeFXProvider{code: "frankfurter", rates: map[string]fakeFXRate{"USD/EUR": {value: 95, scale: 2, publishedAt: "2026-10-18T00:00:00Z"}}},
	)
	database, s := newPricingRefreshTestService(t, []string{"USD", "EUR"}, registry)
	now := time.Date(2026, 10, 18, 23, 30, 0, 0, time.UTC)
	setOwnerTimeZone(t, database, "Europe/Amsterdam")
	s.SetSettings(NewSettingsService(db.NewSettingsRepository(database)))
	s.SetNowForTest(func() time.Time { return now })
	return database, s, now
}

var pinnedDayFXTarget = fxRefreshTarget{Base: db.PricingCurrencyRecord{ID: 1, Code: "USD"}, Quote: db.PricingCurrencyRecord{ID: 2, Code: "EUR"}}

// Both providers answer with a rate valid on the pinned day, so only the
// assignment lookup decides which one is stored.
func TestUndatedFXSourceSelectionUsesTheDayTheRunPinned(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, s, now := newPinnedDayFXService(t, "2026-10-18T00:00:00Z")
	ecbID := marketDataSourceIDByCode(t, database, "ecb_euro_reference_rates")
	assignedID := marketDataSourceIDByCode(t, database, "frankfurter")
	_, err := s.SaveSourceAssignment(ctx, PricingSourceAssignmentInput{
		OwnerUserID: 1, CommodityID: 1, QuoteCommodityID: 2, SourceID: assignedID,
		EffectiveFrom: "2026-10-01", EffectiveTo: "2026-10-18",
	})
	require.NoError(t, err)

	outcome, err := s.fetchAndStoreDirectFX(db.WithReferenceDate(ctx, "2026-10-18"), 1, pinnedDayFXTarget, PricingPolicy{DefaultSourceID: &ecbID}, 0, "", now)
	require.NoError(t, err)
	assert.Equal(t, assignedID, outcome.SourceID, "the assignment is still valid on the pinned day")
	assert.Equal(t, int64(95), outcome.Observation.PriceValue)
}

// No assignment: ECB is tried first and answers with the 19th, which is in
// the future on the pinned day, so the refresh falls through to Frankfurter.
func TestUndatedFXCutoffUsesTheDayTheRunPinned(t *testing.T) {
	t.Parallel()
	database, s, now := newPinnedDayFXService(t, "2026-10-19T00:00:00Z")
	ecbID := marketDataSourceIDByCode(t, database, "ecb_euro_reference_rates")

	outcome, err := s.fetchAndStoreDirectFX(db.WithReferenceDate(context.Background(), "2026-10-18"), 1, pinnedDayFXTarget, PricingPolicy{}, ecbID, "", now)
	require.NoError(t, err)
	assert.Equal(t, marketDataSourceIDByCode(t, database, "frankfurter"), outcome.SourceID, "an observation after the pinned day is refused")
	assert.Equal(t, "2026-10-18", outcome.Observation.ValuationDate)
}

// #605 review. Demand that starts after the pinned day waits, even when the
// owner's zone has since moved to a day on which it would be current.
func TestFXRefreshDemandEligibilityUsesTheDayTheRunPinned(t *testing.T) {
	t.Parallel()
	f := newFutureFXFixture(t, "Europe/Amsterdam", time.Date(2026, 6, 12, 22, 30, 0, 0, time.UTC))
	f.post(t, "2026-06-13", "posted", "10000", 2)
	run, err := f.pricing.RunRefresh(db.WithReferenceDate(context.Background(), "2026-06-12"), 1, marketDataSourceIDByCode(t, f.database, "frankfurter"), "manual")
	require.NoError(t, err)
	assert.Zero(t, run.ItemsTotal, "demand from the 13th is future on the pinned 12th")
	assert.Zero(t, f.calls.Load())
}
