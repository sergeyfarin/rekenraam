package app

import (
	"context"
	"errors"
	"maps"
	"math/big"
	"slices"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

type OverviewAmount struct {
	Code  string            `json:"commodity_code"`
	Value exact.Coefficient `json:"quantity_value"`
	Scale int               `json:"quantity_scale"`
}

type PlanningOverview struct {
	AsOf      string
	Budget    ReadCard[PlanningBudget]
	Recurring ReadCard[RecurringSummary]
	Forecast  ReadCard[PlanningForecast]
}

type PlanningBudget struct {
	StartDate, EndDate string
	ElapsedBasisPoints int
	Totals             []PlanningBudgetTotal
}

type PlanningBudgetTotal struct {
	CommodityID      int64
	Remaining        OverviewAmount
	SpentBasisPoints string
}

type PlanningForecast struct {
	ThroughDate string
	HorizonDays int
	Complete    bool
	Minimums    []PlanningMinimum
}

type PlanningMinimum struct {
	Date   string         `json:"date"`
	Amount OverviewAmount `json:"amount"`
}

func ReadPlanningOverview(ctx context.Context, settings *SettingsService, budgets *BudgetService, recurring *RecurringService, forecast *ForecastService, ownerID int64) (PlanningOverview, error) {
	now := settings.now()
	ctx, err := settings.referenceContext(ctx, now)
	if err != nil {
		return PlanningOverview{}, err
	}
	asOf, err := pinnedOwnerDay(ctx)
	if err != nil {
		return PlanningOverview{}, err
	}
	return PlanningOverview{
		AsOf:      asOf,
		Budget:    valueReadCard(planningBudget(ctx, budgets, asOf)),
		Recurring: valueReadCard(recurring.reviewSummary(ctx, asOf)),
		Forecast:  valueReadCard(forecast.planningMinimums(ctx, ownerID, now.UTC(), asOf)),
	}, nil
}

func planningBudget(ctx context.Context, budgets *BudgetService, asOf string) (PlanningBudget, error) {
	today, err := time.Parse(time.DateOnly, asOf)
	if err != nil {
		return PlanningBudget{}, err
	}
	month, err := budgets.Month(ctx, monthStart(asOf))
	if err != nil {
		return PlanningBudget{}, err
	}
	days := time.Date(today.Year(), today.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	summary := PlanningBudget{
		StartDate: month.PeriodStart, EndDate: month.PeriodEnd,
		ElapsedBasisPoints: (today.Day()*10000 + days/2) / days,
		Totals:             []PlanningBudgetTotal{},
	}
	commodities := make(map[int64]string, len(month.Commodities))
	for _, c := range month.Commodities {
		commodities[c.ID] = c.Code
	}
	for _, total := range month.Totals {
		a := total.Amount
		if total.CategoryType != "expense" || a.Target.QuantityValue.Sign() <= 0 {
			continue
		}
		share, err := exact.MulDivRound(
			a.Actual.QuantityValue.BigInt(), a.Actual.QuantityScale, big.NewInt(1), 0,
			a.Target.QuantityValue.BigInt(), a.Target.QuantityScale, 4,
		)
		if err != nil {
			return PlanningBudget{}, err
		}
		summary.Totals = append(summary.Totals, PlanningBudgetTotal{
			CommodityID:      a.CommodityID,
			Remaining:        OverviewAmount{Code: commodities[a.CommodityID], Value: a.Remaining.QuantityValue, Scale: a.Remaining.QuantityScale},
			SpentBasisPoints: share.String(),
		})
	}
	return summary, nil
}

// Project the same movements as Forecast, retaining only the per-currency
// aggregate minimums. No per-account series, event details, learned spending,
// conversion, or details token are needed for this read.
func (s *ForecastService) planningMinimums(ctx context.Context, ownerID int64, now time.Time, asOf string) (PlanningForecast, error) {
	input, err := normalizeForecastInput(ForecastInput{HorizonDays: forecastDefaultHorizonDays, IncludeDescendants: true})
	if err != nil {
		return PlanningForecast{}, err
	}
	bounds, scope, snapshot, err := s.loadForecastSnapshot(ctx, input, ownerID, now, asOf)
	if err != nil {
		return PlanningForecast{}, err
	}
	projection, err := prepareForecast(ctx, bounds, scope, snapshot)
	if err != nil {
		return PlanningForecast{}, err
	}
	commodities := commodityRulesAt(snapshot.CommodityVersions, bounds.AsOf)
	pairs := map[int64][]forecastPair{}
	for pair := range projection.scales {
		pairs[pair.CommodityID] = append(pairs[pair.CommodityID], pair)
	}
	result := PlanningForecast{ThroughDate: bounds.Through, HorizonDays: input.HorizonDays, Complete: !projection.incomplete, Minimums: []PlanningMinimum{}}
	for _, id := range slices.Sorted(maps.Keys(pairs)) {
		minimum, err := projection.makeAggregate(id, pairs[id], false)
		if err != nil {
			return PlanningForecast{}, err
		}
		result.Minimums = append(result.Minimums, PlanningMinimum{
			Date: minimum.MinimumDate, Amount: OverviewAmount{Code: commodities[id].Code, Value: minimum.Minimum.Value, Scale: minimum.Minimum.Scale},
		})
	}
	return result, nil
}

type ReportsOverview struct {
	AsOf, MonthStart    string
	ReportingCurrencyID *int64
	Spending            ReadCard[ReportHeadline]
	Cashflow            ReadCard[ReportHeadline]
	NetWorth            ReadCard[ReportHeadline]
	GainsRuns           ReadCard[[]OverviewSavedReport]
	ReturnsRuns         ReadCard[[]OverviewSavedReport]
}

type ReportHeadline struct {
	State   string           `json:"state"`
	Amounts []OverviewAmount `json:"amounts"`
	Stale   bool             `json:"stale"`
}

type OverviewSavedReport struct {
	ID         int64  `json:"id"`
	Kind       string `json:"kind"`
	CapturedAt string `json:"captured_at"`
}

func ReadReportsOverview(ctx context.Context, transactions *TransactionService, investments *InvestmentService) (ReportsOverview, error) {
	ctx, err := transactions.settings.referenceContext(ctx, transactions.now())
	if err != nil {
		return ReportsOverview{}, err
	}
	asOf, err := pinnedOwnerDay(ctx)
	if err != nil {
		return ReportsOverview{}, err
	}
	result := ReportsOverview{AsOf: asOf, MonthStart: monthStart(asOf),
		GainsRuns:   valueReadCard([]OverviewSavedReport{}, nil),
		ReturnsRuns: valueReadCard([]OverviewSavedReport{}, nil),
	}
	if investments != nil {
		result.GainsRuns = valueReadCard(overviewGainsRuns(ctx, investments))
		result.ReturnsRuns = valueReadCard(overviewReturnsRuns(ctx, investments))
	}
	reporting, err := transactions.overviewReportingCurrency(ctx)
	if err != nil {
		result.Spending.Err, result.Cashflow.Err, result.NetWorth.Err = err, err, err
		return result, nil
	}
	if reporting != nil {
		result.ReportingCurrencyID = &reporting.CommodityID
	}
	result.Spending = valueReadCard(transactions.overviewSpending(ctx, asOf, reporting))
	result.Cashflow = valueReadCard(transactions.overviewCashflow(ctx, asOf, reporting))
	result.NetWorth = valueReadCard(transactions.overviewNetWorth(ctx, asOf, reporting))
	return result, nil
}

func (s *TransactionService) overviewReportingCurrency(ctx context.Context) (*ReportingCurrencyInput, error) {
	if s.pricingRepository == nil {
		return nil, nil
	}
	currency, err := s.pricingRepository.DefaultCurrency(ctx, BookID)
	if errors.Is(err, db.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ReportingCurrencyInput{CommodityID: currency.ID}, nil
}

func (s *TransactionService) overviewSpending(ctx context.Context, asOf string, reporting *ReportingCurrencyInput) (ReportHeadline, error) {
	report, err := s.Spending(ctx, monthToDateSpending(asOf, reporting))
	if err != nil {
		return ReportHeadline{}, err
	}
	return s.overviewHeadline(ctx, report.CommodityTotals, report.ConvertedTotal, report.Valuation)
}

func (s *TransactionService) overviewCashflow(ctx context.Context, asOf string, reporting *ReportingCurrencyInput) (ReportHeadline, error) {
	report, err := s.Cashflow(ctx, monthToDateCashflow(asOf, reporting))
	if err != nil {
		return ReportHeadline{}, err
	}
	bucket := report.Buckets[0]
	return s.overviewHeadline(ctx, bucket.OperatingNet, bucket.ConvertedOperatingNet, report.Valuation)
}

func (s *TransactionService) overviewNetWorth(ctx context.Context, asOf string, reporting *ReportingCurrencyInput) (ReportHeadline, error) {
	report, err := s.NetWorthSeries(ctx, NetWorthSeriesInput{StartDate: asOf, EndDate: asOf, Bucket: "month", Reporting: reporting})
	if err != nil {
		return ReportHeadline{}, err
	}
	bucket := report.Buckets[0]
	switch {
	case len(bucket.UnclassifiedShorts) != 0:
		return emptyReportHeadline("incomplete"), nil
	case len(bucket.Totals) == 0:
		return emptyReportHeadline("empty"), nil
	case reporting == nil:
		return emptyReportHeadline("no_currency"), nil
	case bucket.Converted == nil:
		return emptyReportHeadline("incomplete"), nil
	default:
		return s.overviewHeadline(ctx, bucket.Totals, bucket.Converted, report.Valuation)
	}
}

func emptyReportHeadline(state string) ReportHeadline {
	return ReportHeadline{State: state, Amounts: []OverviewAmount{}}
}

func (s *TransactionService) overviewHeadline(ctx context.Context, native []BalanceQuantity, converted *BalanceQuantity, valuation *ValuationCoverage) (ReportHeadline, error) {
	if len(native) == 0 {
		return emptyReportHeadline("empty"), nil
	}
	headline := emptyReportHeadline("ready")
	if converted != nil {
		headline.Amounts = append(headline.Amounts, OverviewAmount{Code: valuation.Code, Value: converted.QuantityValue, Scale: converted.QuantityScale})
		headline.Stale = slices.ContainsFunc(valuation.Used, func(rate RateUse) bool { return rate.Stale })
		return headline, nil
	}
	if valuation != nil {
		headline.State = "incomplete"
	}
	ids := make([]int64, 0, len(native))
	for _, amount := range native {
		ids = append(ids, amount.CommodityID)
	}
	if s.pricingRepository == nil {
		return ReportHeadline{}, errors.New("commodity catalog unavailable")
	}
	commodities, err := s.pricingRepository.ReportingCurrencies(ctx, BookID, ids)
	if err != nil {
		return ReportHeadline{}, err
	}
	for _, amount := range native {
		code := commodities[amount.CommodityID].Code
		if code == "" {
			return ReportHeadline{}, errors.New("report commodity unavailable")
		}
		headline.Amounts = append(headline.Amounts, OverviewAmount{Code: code, Value: amount.QuantityValue, Scale: amount.QuantityScale})
	}
	return headline, nil
}

const overviewSavedReportLimit = 5

func overviewGainsRuns(ctx context.Context, investments *InvestmentService) ([]OverviewSavedReport, error) {
	runs, _, err := investments.ListGainsRuns(ctx, 0)
	if err != nil {
		return nil, err
	}
	result := make([]OverviewSavedReport, 0, min(len(runs), overviewSavedReportLimit))
	for _, run := range runs[:min(len(runs), overviewSavedReportLimit)] {
		result = append(result, OverviewSavedReport{ID: run.ID, Kind: run.ReportType, CapturedAt: run.CapturedAt})
	}
	return result, nil
}

func overviewReturnsRuns(ctx context.Context, investments *InvestmentService) ([]OverviewSavedReport, error) {
	run, found, err := investments.LatestReturnsRun(ctx)
	if err != nil || !found {
		return []OverviewSavedReport{}, err
	}
	return []OverviewSavedReport{{ID: run.ID, Kind: "returns", CapturedAt: run.CapturedAt}}, nil
}
