package app

import (
	"fmt"
	"math/big"
	"sort"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

const forecastMaxRateStalenessDays = 7

func addForecastConversion(result *ForecastResult, input forecastNormalizedInput, snapshot db.ForecastSnapshot) error {
	quoteID := *input.ReportingCurrencyID
	quoteRule, ok := commodityRulesAt(snapshot.CommodityVersions, result.AsOfDate)[quoteID]
	if !ok || quoteRule.Kind != "currency" {
		return ValidationError{Message: "reporting currency must identify an existing currency"}
	}
	valuation := &ForecastValuation{Method: "constant_as_of", RateSelection: "observed_on_or_before", AsOfDate: result.AsOfDate, ReportingCurrencyID: quoteID, ReportingCurrencyCode: quoteRule.Code, ReportingCurrencyScale: quoteRule.StandardScale, MaxStalenessDays: forecastMaxRateStalenessDays, Rounding: "per_currency_component_half_away_from_zero", Complete: true, UsedRates: []ForecastRateUse{}, Gaps: []ForecastRateGap{}}
	result.Valuation = valuation

	needed := forecastNeededCurrencies(result.Series)
	rates := map[int64]db.ForecastRateRecord{}
	for _, rate := range snapshot.Rates {
		current, exists := rates[rate.BaseCommodityID]
		if rate.ValuationDate > result.AsOfDate || exists && !laterForecastRate(rate, current) {
			continue
		}
		rates[rate.BaseCommodityID] = rate
	}
	for _, commodityID := range needed {
		if commodityID == quoteID {
			continue
		}
		rate, found := rates[commodityID]
		if !found {
			valuation.Complete = false
			valuation.Gaps = append(valuation.Gaps, ForecastRateGap{CommodityID: commodityID, Reason: "no_observation_in_window"})
			continue
		}
		age, err := forecastDateAge(result.AsOfDate, rate.ValuationDate)
		if err != nil {
			return err
		}
		if age > forecastMaxRateStalenessDays {
			valuation.Complete = false
			valuation.Gaps = append(valuation.Gaps, ForecastRateGap{CommodityID: commodityID, Reason: "no_observation_in_window", NearestObservationDate: rate.ValuationDate})
			continue
		}
		valuation.UsedRates = append(valuation.UsedRates, ForecastRateUse{ObservationID: rate.ObservationID, BaseCommodityID: rate.BaseCommodityID, QuoteCommodityID: rate.QuoteCommodityID, ValuationDate: rate.ValuationDate, RecordedAt: rate.RecordedAt, PriceValue: exact.New(rate.PriceValue), PriceScale: rate.PriceScale, BaseQuantityValue: exact.New(rate.BaseQuantityValue), BaseQuantityScale: rate.BaseQuantityScale, IsDerived: rate.IsDerived, Stale: age > 0})
	}
	sort.Slice(valuation.Gaps, func(i, j int) bool { return valuation.Gaps[i].CommodityID < valuation.Gaps[j].CommodityID })
	sort.Slice(valuation.UsedRates, func(i, j int) bool {
		return valuation.UsedRates[i].BaseCommodityID < valuation.UsedRates[j].BaseCommodityID
	})
	if !valuation.Complete {
		result.Converted = nil
		return nil
	}

	aggregates := map[int64]ForecastSeries{}
	for _, series := range result.Aggregates {
		aggregates[series.CommodityID] = series
	}
	converted := ForecastSeries{CommodityID: quoteID, Points: make([]ForecastPoint, 0, result.HorizonDays)}
	opening := exact.NewScaledInt()
	for _, commodityID := range needed {
		series := aggregates[commodityID]
		value, err := convertForecastQuantity(series.Opening, commodityID, quoteID, quoteRule.StandardScale, rates)
		if err != nil {
			return err
		}
		opening.AddCoefficient(value.Value, value.Scale)
	}
	if err := checkForecastValue(opening, quoteID); err != nil {
		return err
	}
	converted.Opening = forecastQuantityAtScale(opening, quoteRule.StandardScale)
	converted.Minimum = converted.Opening
	converted.MinimumDate = result.AsOfDate
	recorded := exact.ScaledIntFromCoefficient(converted.Opening.Value, converted.Opening.Scale)
	projected := exact.ScaledIntFromCoefficient(converted.Opening.Value, converted.Opening.Scale)
	for pointIndex := 0; pointIndex < result.HorizonDays; pointIndex++ {
		components := ForecastComponents{Posted: zeroForecastQuantity(quoteRule.StandardScale), Draft: zeroForecastQuantity(quoteRule.StandardScale), Template: zeroForecastQuantity(quoteRule.StandardScale)}
		for _, commodityID := range needed {
			series := aggregates[commodityID]
			if pointIndex >= len(series.Points) {
				continue
			}
			var err error
			components.Posted, err = addConvertedForecastQuantity(components.Posted, series.Points[pointIndex].Components.Posted, commodityID, quoteID, quoteRule.StandardScale, rates)
			if err != nil {
				return err
			}
			components.Draft, err = addConvertedForecastQuantity(components.Draft, series.Points[pointIndex].Components.Draft, commodityID, quoteID, quoteRule.StandardScale, rates)
			if err != nil {
				return err
			}
			components.Template, err = addConvertedForecastQuantity(components.Template, series.Points[pointIndex].Components.Template, commodityID, quoteID, quoteRule.StandardScale, rates)
			if err != nil {
				return err
			}
		}
		recorded.AddCoefficient(components.Posted.Value, components.Posted.Scale)
		projected.AddCoefficient(components.Posted.Value, components.Posted.Scale)
		projected.AddCoefficient(components.Draft.Value, components.Draft.Scale)
		projected.AddCoefficient(components.Template.Value, components.Template.Scale)
		if err := checkForecastValue(recorded, quoteID); err != nil {
			return err
		}
		if err := checkForecastValue(projected, quoteID); err != nil {
			return err
		}
		date := result.FirstDate
		if len(result.Aggregates) > 0 && pointIndex < len(result.Aggregates[0].Points) {
			date = result.Aggregates[0].Points[pointIndex].Date
		} else {
			parsed, _ := time.Parse(time.DateOnly, result.FirstDate)
			date = parsed.AddDate(0, 0, pointIndex).Format(time.DateOnly)
		}
		counts, carried := convertedForecastEventCounts(result.Events, date)
		point := ForecastPoint{Date: date, Components: components, RecordedBalance: forecastQuantityAtScale(recorded, quoteRule.StandardScale), ProjectedBalance: forecastQuantityAtScale(projected, quoteRule.StandardScale), SourceCounts: counts, CarriedForward: carried}
		converted.Points = append(converted.Points, point)
		if exact.ScaledIntFromCoefficient(point.ProjectedBalance.Value, point.ProjectedBalance.Scale).Cmp(exact.ScaledIntFromCoefficient(converted.Minimum.Value, converted.Minimum.Scale)) < 0 {
			converted.Minimum, converted.MinimumDate = point.ProjectedBalance, date
		}
	}
	result.Converted = &converted
	return nil
}

func laterForecastRate(candidate, current db.ForecastRateRecord) bool {
	if candidate.ValuationDate != current.ValuationDate {
		return candidate.ValuationDate > current.ValuationDate
	}
	if candidate.RecordedAt != current.RecordedAt {
		return candidate.RecordedAt > current.RecordedAt
	}
	return candidate.ObservationID > current.ObservationID
}

func forecastNeededCurrencies(series []ForecastSeries) []int64 {
	set := map[int64]bool{}
	for _, accountSeries := range series {
		if accountSeries.Opening.Value != "0" {
			set[accountSeries.CommodityID] = true
		}
		for _, point := range accountSeries.Points {
			if point.Components.Posted.Value != "0" || point.Components.Draft.Value != "0" || point.Components.Template.Value != "0" {
				set[accountSeries.CommodityID] = true
			}
		}
	}
	result := make([]int64, 0, len(set))
	for id := range set {
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func forecastDateAge(asOf, observed string) (int, error) {
	a, err := time.Parse(time.DateOnly, asOf)
	if err != nil {
		return 0, fmt.Errorf("parse forecast as-of date: %w", err)
	}
	o, err := time.Parse(time.DateOnly, observed)
	if err != nil {
		return 0, fmt.Errorf("parse forecast rate date: %w", err)
	}
	return int(a.Sub(o).Hours() / 24), nil
}

func convertForecastQuantity(value ForecastQuantity, commodityID, quoteID int64, quoteScale int, rates map[int64]db.ForecastRateRecord) (ForecastQuantity, error) {
	multiplier, multiplierScale := big.NewInt(1), 0
	divisor, divisorScale := big.NewInt(1), 0
	if commodityID != quoteID {
		rate := rates[commodityID]
		multiplier, multiplierScale = big.NewInt(rate.PriceValue), rate.PriceScale
		divisor, divisorScale = big.NewInt(rate.BaseQuantityValue), rate.BaseQuantityScale
	}
	converted, err := exact.MulDivRound(value.Value.BigInt(), value.Scale, multiplier, multiplierScale, divisor, divisorScale, quoteScale)
	if err != nil {
		return ForecastQuantity{}, fmt.Errorf("convert forecast quantity: %w", err)
	}
	coefficient, err := exact.Parse(converted.String())
	if err != nil {
		return ForecastQuantity{}, LedgerOverflowError{CommodityID: quoteID}
	}
	return ForecastQuantity{Value: coefficient, Scale: quoteScale}, nil
}

func addConvertedForecastQuantity(total, source ForecastQuantity, commodityID, quoteID int64, quoteScale int, rates map[int64]db.ForecastRateRecord) (ForecastQuantity, error) {
	converted, err := convertForecastQuantity(source, commodityID, quoteID, quoteScale, rates)
	if err != nil {
		return ForecastQuantity{}, err
	}
	value := exact.ScaledIntFromCoefficient(total.Value, total.Scale)
	value.AddCoefficient(converted.Value, converted.Scale)
	if err := checkForecastValue(value, quoteID); err != nil {
		return ForecastQuantity{}, err
	}
	return forecastQuantityAtScale(value, quoteScale), nil
}

func forecastQuantityAtScale(value *exact.ScaledInt, scale int) ForecastQuantity {
	aligned := value.TruncatedTo(scale)
	return ForecastQuantity{Value: exact.Coefficient(aligned.BigInt().String()), Scale: scale}
}

func zeroForecastQuantity(scale int) ForecastQuantity {
	return ForecastQuantity{Value: exact.Coefficient("0"), Scale: scale}
}

func convertedForecastEventCounts(events []ForecastEvent, date string) (ForecastSourceCounts, int) {
	counts := ForecastSourceCounts{}
	carried := 0
	for _, event := range events {
		if event.ProjectedDate != date {
			continue
		}
		switch event.Source {
		case "posted":
			counts.Posted++
		case "draft":
			counts.Draft++
		case "template":
			counts.Template++
		}
		if event.CarriedForward {
			carried++
		}
	}
	return counts, carried
}
