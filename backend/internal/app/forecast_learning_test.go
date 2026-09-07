package app

import (
	"database/sql"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func learningAccounts() ([]db.ForecastAccountVersionRecord, []db.ForecastCommodityVersionRecord) {
	accounts := []db.ForecastAccountVersionRecord{
		forecastAccountVersion(1, "Checking", "asset", "checking", 1),
		forecastAccountVersion(2, "Card", "liability", "credit_card", 1),
		forecastAccountVersion(3, "Groceries", "expense", "expense", 1),
		forecastAccountVersion(4, "Travel", "expense", "expense", 1),
		forecastAccountVersion(5, "Income", "income", "income", 1),
		forecastAccountVersion(6, "Savings", "asset", "savings", 1),
		forecastAccountVersion(7, "Holding", "asset", "security_holding", 2),
	}
	commodities := []db.ForecastCommodityVersionRecord{
		{CommodityID: 1, BookID: 1, VersionID: 1, VersionSeq: 1, EffectiveFrom: "0001-01-01", Code: "EUR", Kind: "currency", Status: "active", StandardScale: 2},
		{CommodityID: 2, BookID: 1, VersionID: 2, VersionSeq: 1, EffectiveFrom: "0001-01-01", Code: "SEC", Kind: "security", Status: "active", StandardScale: 4},
	}
	return accounts, commodities
}

func learningRows(tx, entry int64, kind, entryKind, date string, legs ...struct {
	account, commodity int64
	value              string
	scale              int
}) []db.ForecastLearningPostingRecord {
	rows := make([]db.ForecastLearningPostingRecord, len(legs))
	for i, leg := range legs {
		rows[i] = db.ForecastLearningPostingRecord{TransactionID: tx, TransactionVersionID: tx, TransactionKind: kind, JournalEntryID: entry, EntryKind: entryKind, EntryDate: date, PostingID: int64(i + 1), LineSeq: i + 1, AccountID: leg.account, CommodityID: leg.commodity, QuantityValue: exact.MustParse(leg.value), QuantityScale: leg.scale}
	}
	return rows
}

func learningLeg(account int64, value string) struct {
	account, commodity int64
	value              string
	scale              int
} {
	return struct {
		account, commodity int64
		value              string
		scale              int
	}{account: account, commodity: 1, value: value, scale: 2}
}

func learningCommodityLeg(account, commodity int64, value string, scale int) struct {
	account, commodity int64
	value              string
	scale              int
} {
	return struct {
		account, commodity int64
		value              string
		scale              int
	}{account: account, commodity: commodity, value: value, scale: scale}
}

func TestForecastLearningClassifiesCompleteEntries(t *testing.T) {
	accounts, commodities := learningAccounts()
	eligible := learningRows(1, 1, "ordinary", "ordinary", "2026-01-05", learningLeg(1, "-15000"), learningLeg(3, "10000"), learningLeg(4, "5000"))
	classified := ClassifyForecastLearningEntries(eligible, accounts, commodities)
	require.Len(t, classified.Purchases, 2)
	assert.Equal(t, []int64{3, 4}, []int64{classified.Purchases[0].Group.CategoryAccountID, classified.Purchases[1].Group.CategoryAccountID})

	tests := []struct {
		name string
		rows []db.ForecastLearningPostingRecord
	}{
		{"two funders", learningRows(2, 2, "ordinary", "ordinary", "2026-01-05", learningLeg(1, "-5000"), learningLeg(6, "-5000"), learningLeg(3, "10000"))},
		{"transfer and card repayment", learningRows(3, 3, "transfer", "transfer_leg", "2026-01-05", learningLeg(1, "-10000"), learningLeg(2, "10000"))},
		{"refund mixed signs", learningRows(4, 4, "ordinary", "ordinary", "2026-01-05", learningLeg(1, "5000"), learningLeg(3, "-5000"))},
		{"hidden refund on repeated expense", learningRows(40, 40, "ordinary", "ordinary", "2026-01-05", learningLeg(1, "-10000"), learningLeg(3, "15000"), learningLeg(3, "-5000"))},
		{"income", learningRows(5, 5, "ordinary", "ordinary", "2026-01-05", learningLeg(1, "10000"), learningLeg(5, "-10000"))},
		{"investment", learningRows(6, 6, "investment", "investment", "2026-01-05", learningLeg(1, "-10000"), learningCommodityLeg(7, 2, "1", 0))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ClassifyForecastLearningEntries(test.rows, accounts, commodities)
			assert.Empty(t, result.Purchases)
			assert.Equal(t, 1, exclusionTotal(result.Excluded))
		})
	}
	linked := eligible
	linked[0].RecurringOccurrenceID = sql.NullInt64{Int64: 9, Valid: true}
	assert.Empty(t, ClassifyForecastLearningEntries(linked, accounts, commodities).Purchases)
}

func exclusionTotal(values map[string]int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func TestForecastLearningPeriodCoverage(t *testing.T) {
	group := ForecastLearningGroup{FundingAccountID: 1, CategoryAccountID: 3, CommodityID: 1}
	purchases := []ForecastLearningPurchase{{Group: group, EntryDate: "2026-01-05", Amount: exact.ScaledIntFromInt64(100, 2)}}
	weekly, err := ForecastLearningPeriods(purchases, group, ForecastLearningWeekly, "2025-09-01", "2026-01-08")
	require.NoError(t, err)
	assert.Equal(t, "2025-09-01", weekly.ActualStart)
	assert.Equal(t, "2026-01-04", weekly.ActualEnd, "the current partial Monday-Sunday week is excluded")
	monthly, err := ForecastLearningPeriods(purchases, group, ForecastLearningMonthly, "2024-09-15", "2026-02-10")
	require.NoError(t, err)
	assert.Equal(t, "2024-10-01", monthly.ActualStart, "the first partial confirmed month is excluded")
	assert.Equal(t, "2026-01-31", monthly.ActualEnd)
}

func TestForecastLearningDoesNotOverlapRecurringGroups(t *testing.T) {
	accounts, _ := learningAccounts()
	templates := []db.ForecastTemplateRecord{
		{ID: 1, StartsOn: "2026-01-01", Enabled: false},
		{ID: 2, StartsOn: "2026-01-01", ArchivedAt: sql.NullString{String: "x", Valid: true}},
	}
	postings := []db.ForecastTemplatePostingRecord{
		{TemplateID: 1, AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-100")}, {TemplateID: 1, AccountID: 3, CommodityID: 1, QuantityValue: exact.MustParse("100")},
		{TemplateID: 2, AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-100")}, {TemplateID: 2, AccountID: 4, CommodityID: 1, QuantityValue: exact.MustParse("100")},
	}
	drafts := []db.ForecastDraftPostingRecord{
		{TemplateID: 2, TransactionID: 8, EntryDate: "2026-01-02", AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-100")},
		{TemplateID: 2, TransactionID: 8, EntryDate: "2026-01-02", AccountID: 4, CommodityID: 1, QuantityValue: exact.MustParse("100")},
	}
	overlaps := ForecastLearningRecurringOverlaps(templates, postings, drafts, accounts, "2026-02-01")
	require.Len(t, overlaps, 2)
	assert.Equal(t, "template", overlaps[0].SourceType, "paused unarchived templates remain overlap")
	assert.Equal(t, "draft", overlaps[1].SourceType, "retained drafts remain overlap after archive")
}

func TestForecastLearningColdStartAndSparseHistory(t *testing.T) {
	group := ForecastLearningGroup{1, 3, 1}
	for _, test := range []struct {
		name, pattern, from, asOf, reason string
	}{
		{"15 weeks", "weekly", "2025-09-22", "2026-01-08", "insufficient_history"},
		{"35 months", "annual_seasonal", "2023-03-01", "2026-02-10", "insufficient_seasonal_history"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ForecastLearningPeriods(nil, group, ForecastLearningPattern(test.pattern), test.from, test.asOf)
			require.NoError(t, err)
			assert.False(t, got.Eligible)
			assert.Equal(t, test.reason, got.Reason)
		})
	}
	weeklyPurchases := make([]ForecastLearningPurchase, 8)
	for i := range weeklyPurchases {
		weeklyPurchases[i] = ForecastLearningPurchase{Group: group, EntryDate: time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i*14).Format(time.DateOnly), Amount: exact.ScaledIntFromInt64(1, 0)}
	}
	got, err := ForecastLearningPeriods(weeklyPurchases, group, ForecastLearningWeekly, "2025-09-01", "2026-01-01")
	require.NoError(t, err)
	assert.True(t, got.Eligible, "16 complete weeks and eight active weeks are eligible")
}

func TestForecastLearningSubtractsKnownPeriodSpend(t *testing.T) {
	group := ForecastLearningGroup{1, 3, 1}
	known, ambiguous, err := ForecastLearningKnownPeriodSpend([]ForecastLearningKnownItem{
		{Group: group, ProjectedDate: "2026-09-01", Amount: exact.ScaledIntFromInt64(2000, 2)},
		{Group: group, ProjectedDate: "2026-09-06", Amount: exact.ScaledIntFromInt64(3000, 2)}, // private beyond a Sep 1-5 public horizon
		{Group: group, ProjectedDate: "2026-09-08", Amount: exact.ScaledIntFromInt64(9999, 2)},
	}, group, ForecastLearningWeekly, "2026-08-31")
	require.NoError(t, err)
	assert.False(t, ambiguous)
	assert.Equal(t, "5000", known.BigInt().String(), "present-period prefix and same-period facts beyond public E are subtracted")
	_, ambiguous, err = ForecastLearningKnownPeriodSpend([]ForecastLearningKnownItem{{Group: group, ProjectedDate: "2026-09-02", Ambiguous: true}}, group, ForecastLearningWeekly, "2026-08-31")
	require.NoError(t, err)
	assert.True(t, ambiguous, "an implicated period is suppressed instead of silently filled")

	for _, test := range []struct {
		name     string
		baseline *big.Rat
		known    *exact.ScaledInt
		scale    int
		want     string
	}{
		{"weekly residual", big.NewRat(140, 1), exact.ScaledIntFromInt64(50, 0), 2, "9000"},
		{"known exceeds baseline", big.NewRat(140, 1), exact.ScaledIntFromInt64(170, 0), 2, "0"},
		{"monthly residual", big.NewRat(120, 1), exact.ScaledIntFromInt64(3000, 2), 2, "9000"},
		{"seasonal residual", big.NewRat(2000, 1), exact.ScaledIntFromInt64(500, 0), 2, "150000"},
	} {
		t.Run(test.name, func(t *testing.T) {
			units, _ := ForecastLearningResidual(test.baseline, test.known, test.scale)
			assert.Equal(t, test.want, units.String())
		})
	}
}

func TestForecastLearningAllocationIsExact(t *testing.T) {
	group := ForecastLearningGroup{1, 3, 1}
	periods := []ForecastLearningPeriod{{Start: "2026-08-31", End: "2026-09-06", Spend: exact.NewScaledInt()}}
	bins := ForecastLearningTimingBins([]ForecastLearningPurchase{
		{Group: group, EntryDate: "2026-08-31", Amount: exact.ScaledIntFromInt64(1, 0)},
		{Group: group, EntryDate: "2026-09-06", Amount: exact.ScaledIntFromInt64(250, 2)},
	}, group, ForecastLearningWeekly, periods)
	assert.Equal(t, "100", bins[0].String(), "mixed scales align exactly in Monday's bin")
	assert.Equal(t, "250", bins[6].String(), "Sunday remains a separate timing bin")

	dates := []string{"2026-09-01", "2026-09-02", "2026-09-03", "2026-09-04", "2026-09-05", "2026-09-06", "2026-09-07"}
	weights := make([]*big.Int, len(dates))
	for i := range weights {
		weights[i] = big.NewInt(1)
	}
	allocation, fallback, err := AllocateForecastLearningUnits(big.NewInt(9000), dates, weights)
	require.NoError(t, err)
	assert.False(t, fallback)
	total := new(big.Int)
	for _, item := range allocation {
		total.Add(total, item.Units)
	}
	assert.Equal(t, "9000", total.String())
	assert.Equal(t, "1286", allocation[0].Units.String())
	assert.Equal(t, "1285", allocation[6].Units.String())

	zero := make([]*big.Int, len(dates))
	for i := range zero {
		zero[i] = new(big.Int)
	}
	_, fallback, err = AllocateForecastLearningUnits(big.NewInt(7), dates, zero)
	require.NoError(t, err)
	assert.True(t, fallback)

	february, err := ForecastLearningAllocationDates(ForecastLearningMonthly, "2028-02-01", "2028-02-27")
	require.NoError(t, err)
	assert.Equal(t, []string{"2028-02-28", "2028-02-29"}, february)
	monthBins := make([]*big.Int, 31)
	for i := range monthBins {
		monthBins[i] = new(big.Int)
	}
	monthBins[30].SetInt64(1)
	dateWeights, err := ForecastLearningDateWeights(ForecastLearningMonthly, february, monthBins)
	require.NoError(t, err)
	assert.Equal(t, "1", dateWeights[1].String(), "day-31 weight clamps to leap-month end")

	negative, _, err := AllocateForecastLearningUnits(big.NewInt(-1), dates, weights)
	assert.Error(t, err)
	assert.Nil(t, negative, "estimated expense allocation cannot invent positive income")
}

func TestForecastLearningBaselinesRemainExact(t *testing.T) {
	periods := make([]ForecastLearningPeriod, 8)
	for i := range periods {
		periods[i] = ForecastLearningPeriod{Start: time.Date(2025, time.Month(i+1), 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly), Spend: exact.ScaledIntFromInt64(int64(i+1), 0)}
	}
	baselines := ForecastLearningBaselines(periods)
	assert.Equal(t, "9/2", baselines[0].Amount.RatString())
	assert.Equal(t, "8", baselines[1].Amount.RatString())
	seasonal := ForecastLearningSeasonalBaselines(append(periods, ForecastLearningPeriod{Start: "2026-01-01", Spend: exact.ScaledIntFromInt64(9, 0)}), time.January)
	assert.Equal(t, "5", seasonal[0].Amount.RatString())
}

func BenchmarkForecastLearningAllocation(b *testing.B) {
	dates := make([]string, 366)
	weights := make([]*big.Int, 366)
	start := time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range dates {
		dates[i] = start.AddDate(0, 0, i).Format(time.DateOnly)
		weights[i] = big.NewInt(int64(i%31 + 1))
	}
	units := new(big.Int).SetUint64(9_007_199_254_740_993)
	b.ResetTimer()
	for range b.N {
		if _, _, err := AllocateForecastLearningUnits(units, dates, weights); err != nil {
			b.Fatal(err)
		}
	}
}
