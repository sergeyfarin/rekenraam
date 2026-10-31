package app

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// Reporting-currency valuation.
//
// A report groups by commodity because unlike commodities cannot be added.
// That stays true: conversion here is **additive**, never replacing. Every
// response keeps its exact per-commodity figures, and a converted total sits
// beside them, labelled with the method that produced it.
//
// The three decisions the R2 acceptance review left open, settled here:
//
//  1. **Which rate date.** A stock is converted at the date it is measured on
//     (a net-worth bucket end); a flow is converted at the date it happened
//     (each posting's entry date), then summed. Converting a period of flows at
//     one range-end rate is cheaper and reproducible from the displayed
//     subtotals, but it misattributes everything that happened before a large
//     mid-period move.
//  2. **Missing coverage.** The nearest earlier observation inside a named
//     window is used and reported as stale; with nothing in the window the
//     converted figure is omitted and the commodity is named, with a reason.
//     Never silently.
//  3. **Prerequisite.** T-37 shipped: a voided observation cascades to every
//     rate triangulated from it, and ReportingRates excludes voided rows. A
//     poisoned rate can be withdrawn, which is what makes rates safe to put
//     behind headline figures.

// ValuationMethodObservedOnOrBefore is the only method implemented. It is named
// in every response rather than assumed, so a second method can arrive without
// changing what an existing figure means.
const ValuationMethodObservedOnOrBefore = "observed_on_or_before"

// Observation selection policies a coverage can report. live_total_return_v1
// is live_raw_v1 with securities and crypto priced only by total_return
// observations; it values an R13 benchmark declared total-return, never a
// ledger balance.
const (
	SelectionPolicyLiveRaw         = "live_raw_v1"
	SelectionPolicyLiveTotalReturn = "live_total_return_v1"
)

// DefaultRateStalenessDays bounds how far back the nearest-earlier search
// looks. A week covers weekends and public holidays — the ordinary reasons a
// date has no observation — without letting a month-old rate masquerade as
// today's.
const DefaultRateStalenessDays = 7

// Reasons a commodity could not be converted. Returned rather than described,
// so a screen can translate them.
const (
	RateMissingNoObservation = "no_observation_in_window"
	RateMissingNotACurrency  = "reporting_currency_is_not_a_currency"
	// RateMissingConflictingSplits: the observation predates a day on which
	// holdings recorded different split factors for the instrument, so the
	// units it prices on the requested date are unknown (#407).
	RateMissingConflictingSplits = "conflicting_splits"
)

// ReportingCurrencyInput is what a caller asks for.
type ReportingCurrencyInput struct {
	CommodityID     int64
	MaxStalenessDay int
}

// RateUse records that a conversion happened and on what: the rate's own
// observation date, and whether that date was the one asked for.
type RateUse struct {
	ObservationID   int64  `json:"observation_id"`
	Approximate     bool   `json:"approximate"`
	CommodityID     int64  `json:"commodity_id"`
	ObservationDate string `json:"observation_date"`
	RequestedDate   string `json:"requested_date"`
	Stale           bool   `json:"stale"`
	Derived         bool   `json:"derived"`
	// Via* name the second leg of a chained conversion (#281): the commodity was
	// priced in ViaCommodityID by the observation above, and that currency was
	// converted by ViaObservationID. Zero for a direct rate.
	ViaCommodityID     int64  `json:"via_commodity_id,omitempty"`
	ViaObservationID   int64  `json:"via_observation_id,omitempty"`
	ViaObservationDate string `json:"via_observation_date,omitempty"`
	// SplitRatio* are the exact factor (lowest terms) of the instrument's
	// recorded splits between the observation date and the requested date:
	// the quote priced a unit of its own day, and each such unit is this many
	// units on the requested date (#407). A trade-implied quote ordered before
	// a split on its own day also counts that split. Empty when no split
	// intervened.
	SplitRatioNumerator   string `json:"split_ratio_numerator,omitempty"`
	SplitRatioDenominator string `json:"split_ratio_denominator,omitempty"`
}

// RateGap is a commodity that could not be converted, and why.
type RateGap struct {
	CommodityID int64  `json:"commodity_id"`
	Reason      string `json:"reason"`
	// NearestObservationDate is the closest observation that exists outside the
	// window, when there is one. "No rate" and "no rate recently" are different
	// problems and lead to different fixes.
	NearestObservationDate string `json:"nearest_observation_date,omitempty"`
	// ViaCommodityID is set when the commodity has a usable price in that
	// currency but the currency itself has no usable reporting rate: the
	// missing leg is the currency's, and Reason/NearestObservationDate are
	// about it.
	ViaCommodityID int64 `json:"via_commodity_id,omitempty"`
}

// ValuationCoverage is what a response says about its own conversions.
type ValuationCoverage struct {
	SelectionPolicy  string
	ValuationAsOf    string
	KnowledgeCutoff  string
	CommodityID      int64
	Code             string
	Scale            int
	Method           string
	MaxStalenessDays int
	Used             []RateUse
	Gaps             []RateGap
	// Complete is false when any figure in the response omits a commodity,
	// which is the one thing a reader must not have to infer from a total that
	// looks plausible.
	Complete bool
}

type rateUseKey struct {
	commodityID, observationID, viaObservationID int64
	requestedDate                                string
}

// RateTable resolves a commodity and a date to a rate, once, for a whole
// report. Built from one query over the window the report covers.
type RateTable struct {
	boundary         db.ObservationBoundary
	diagnostics      map[[2]int64]string
	quoteCommodityID int64
	quoteScale       int
	stalenessDays    int
	// byCommodity holds each commodity's observations in date order.
	byCommodity map[int64][]db.ReportingRateRecord
	// chainQuotes holds, per non-currency commodity, its observations in each
	// other currency, in date order: the first legs of chained conversions.
	chainQuotes map[int64]map[int64][]db.ReportingRateRecord

	used map[rateUseKey]RateUse
	gaps map[int64]RateGap
	// selectionPolicy names the observation selection behind byCommodity and
	// chainQuotes; empty is live_raw_v1.
	selectionPolicy string
	// splitDays and splitConflicts hold, per instrument, its recorded split
	// factors (one per day, in date order) and the days holdings disagree on,
	// from the staleness window before the first date through the last. Empty
	// under total-return selection.
	splitDays      map[int64][]SplitDay
	splitConflicts map[int64][]string
	// pendingSplits holds, per trade-implied observation dated on a split
	// day, its instrument-wide basis: the factor still pending after the
	// trade (that quote priced pre-split units), or disputed when holdings
	// recorded the day's split on both sides of the trade.
	pendingSplits map[int64]db.TradeQuoteBasis
}

// NewRateTable loads every rate that could be needed to convert the given
// commodities into the reporting currency across [fromDate, toDate].
//
// The window is widened backwards by the staleness allowance, because the rate
// that applies to the first day of a report may have been observed before it.
func (s *TransactionService) NewRateTable(
	ctx context.Context,
	reporting ReportingCurrencyInput,
	commodityIDs []int64,
	fromDate string,
	toDate string,
) (*RateTable, error) {
	ctx, referenceErr := s.settings.referenceContext(ctx, s.now())
	if referenceErr != nil {
		return nil, referenceErr
	}

	if s.pricingRepository == nil {
		return nil, fmt.Errorf("reporting currency requires pricing data")
	}
	boundary, err := s.pricingRepository.CurrentObservationBoundary(ctx, BookID, s.now())
	if err != nil {
		return nil, err
	}
	return newRateTableAtBoundary(ctx, s.pricingRepository, reporting, commodityIDs, fromDate, toDate, boundary)
}

// newRateTableAtBoundary lets projections share their observation knowledge instant.
func newRateTableAtBoundary(ctx context.Context, pricing *db.PricingRepository, reporting ReportingCurrencyInput, commodityIDs []int64, fromDate, toDate string, boundary db.ObservationBoundary) (*RateTable, error) {
	if pricing == nil {
		return nil, fmt.Errorf("reporting currency requires pricing data")
	}

	staleness := reporting.MaxStalenessDay
	if staleness <= 0 {
		staleness = DefaultRateStalenessDays
	}

	quote, err := pricing.ReportingCurrency(ctx, BookID, reporting.CommodityID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ValidationError{Message: "reporting currency is invalid"}
		}
		return nil, err
	}
	if quote.Kind != "currency" {
		// A report denominated in a security would be a different feature and a
		// far stranger one; refusing is clearer than converting into it.
		return nil, ValidationError{Message: "reporting currency must be a currency"}
	}

	boundary.AsOf = min(boundary.AsOf, toDate)
	table := &RateTable{
		boundary:         boundary,
		quoteCommodityID: reporting.CommodityID,
		quoteScale:       quote.StandardScale,
		stalenessDays:    staleness,
		byCommodity:      map[int64][]db.ReportingRateRecord{},
		chainQuotes:      map[int64]map[int64][]db.ReportingRateRecord{},

		used: map[rateUseKey]RateUse{},
		gaps: map[int64]RateGap{},
	}

	needed := make([]int64, 0, len(commodityIDs))
	for _, commodityID := range dedupeIDs(commodityIDs) {
		if commodityID != reporting.CommodityID {
			needed = append(needed, commodityID)
		}
	}
	if len(needed) == 0 {
		return table, nil
	}

	windowStart, err := shiftISODate(fromDate, -staleness)
	if err != nil {
		return nil, err
	}
	// A security is usually priced in its quote currency, not the reporting
	// currency, so its rate is chained: security → quote currency → reporting
	// currency, each leg under the same on-or-before and staleness rules. Only
	// one currency hop is attempted; currency-to-currency triangulation stays
	// with the FX refresh worker's derived observations.
	chainQuotes, err := pricing.ReportingChainQuotes(ctx, BookID, reporting.CommodityID, needed, windowStart, toDate, boundary)
	if err != nil {
		return nil, err
	}
	rateBases := append([]int64(nil), needed...)
	for _, quote := range chainQuotes {
		byQuote := table.chainQuotes[quote.BaseCommodityID]
		if byQuote == nil {
			byQuote = map[int64][]db.ReportingRateRecord{}
			table.chainQuotes[quote.BaseCommodityID] = byQuote
		}
		byQuote[quote.QuoteCommodityID] = append(byQuote[quote.QuoteCommodityID], quote)
		rateBases = append(rateBases, quote.QuoteCommodityID)
	}
	rateBases = dedupeIDs(rateBases)

	rates, err := pricing.ReportingRates(ctx, BookID, reporting.CommodityID, rateBases, windowStart, toDate, boundary)
	if err != nil {
		return nil, err
	}
	for _, rate := range rates {
		table.byCommodity[rate.BaseCommodityID] = append(table.byCommodity[rate.BaseCommodityID], rate)
	}

	pairs := make([][2]int64, 0, len(rateBases))
	for _, id := range rateBases {
		pairs = append(pairs, [2]int64{id, reporting.CommodityID})
	}
	table.diagnostics, err = pricing.ObservationDiagnostics(ctx, BookID, pairs, boundary)
	if err != nil {
		return nil, err
	}

	// A raw quote prices a unit of its own day; a total-return series is a
	// continuous index level and is never split-adjusted.
	if !pricing.TotalReturnSecurities() {
		splits, err := pricing.InstrumentSplits(ctx, BookID, needed, windowStart, toDate)
		if err != nil {
			return nil, err
		}
		table.splitDays, table.splitConflicts = db.InstrumentSplitDays(splits)
		if table.pendingSplits, err = pricing.TradeQuotePendingSplits(ctx, BookID, needed, previousDay(windowStart), toDate); err != nil {
			return nil, err
		}
	}

	return table, nil
}

// SplitDay is one day's split factor of an instrument (db.SplitDay): the
// holdings read applies the same factors (#408).
type SplitDay = db.SplitDay

// splitFactor is db.SplitFactor over this table's loaded splits.
func (t *RateTable) splitFactor(commodityID int64, observed, onDate string) (*big.Rat, string) {
	return db.SplitFactor(t.splitDays[commodityID], t.splitConflicts[commodityID], observed, onDate)
}

// QuoteCommodityID is the currency this table converts into.
func (t *RateTable) QuoteCommodityID() int64 { return t.quoteCommodityID }

// Scale is the reporting currency's own scale — what converted figures are
// rounded to, once, at the end of each conversion.
func (t *RateTable) Scale() int { return t.quoteScale }

// Convert values one amount of one commodity as at a date.
//
// Returns ok=false when no rate applies, having recorded why; the caller omits
// the figure rather than substituting a zero, which would read as "nothing"
// instead of "unknown".
func (t *RateTable) Convert(value exact.Coefficient, scale int, commodityID int64, onDate string) (*big.Int, bool) {
	rate, ok := t.resolveUnits(commodityID, onDate)
	if !ok {
		return nil, false
	}
	if rate.identity {
		// The reporting currency converts to itself at 1, exactly, with no
		// observation involved and nothing to round.
		aligned := exact.ScaledIntFromCoefficient(value, scale)
		aligned.Align(t.quoteScale)
		return aligned.BigInt(), true
	}
	converted, err := exact.MulDivRound(value.BigInt(), scale, rate.price, rate.priceScale, rate.base, rate.baseScale, t.quoteScale)
	if err != nil {
		t.gaps[commodityID] = RateGap{CommodityID: commodityID, Reason: RateMissingNoObservation}
		return nil, false
	}
	t.recordUse(commodityID, onDate, rate)
	return converted, true
}

// ConvertExact values an amount like Convert, with the same rate selection,
// split rescaling, coverage and gaps, but returns the exact rational value
// instead of rounding it to the reporting currency's scale. A consumer whose
// arithmetic needs the true sign or ratio (R13's time-weighted chain) rounds
// only what it shows.
func (t *RateTable) ConvertExact(value exact.Coefficient, scale int, commodityID int64, onDate string) (*big.Rat, bool) {
	rate, ok := t.resolveUnits(commodityID, onDate)
	if !ok {
		return nil, false
	}
	return t.convertExact(value, scale, commodityID, onDate, rate), true
}

// ConvertExactObserved values units of the observation's own day, not of
// onDate: it is ConvertExact without split rescaling, plus the valuation date
// of the observation that priced the commodity itself (a chain's first leg).
// A caller that tracks its own units through splits (the R13 benchmark) uses
// it with SplitDays. The date is onDate for the reporting currency.
func (t *RateTable) ConvertExactObserved(value exact.Coefficient, scale int, commodityID int64, onDate string) (*big.Rat, string, bool) {
	rate, ok := t.resolve(commodityID, onDate)
	if !ok {
		return nil, "", false
	}
	if rate.identity {
		return t.convertExact(value, scale, commodityID, onDate, rate), onDate, true
	}
	return t.convertExact(value, scale, commodityID, onDate, rate), rate.use.ObservationDate, true
}

// SplitDays are the instrument's recorded split factors this table loaded and
// the days holdings disagree on, both in date order.
func (t *RateTable) SplitDays(commodityID int64) ([]SplitDay, []string) {
	return t.splitDays[commodityID], t.splitConflicts[commodityID]
}

func (t *RateTable) convertExact(value exact.Coefficient, scale int, commodityID int64, onDate string, rate resolvedRate) *big.Rat {
	amount := new(big.Rat).SetFrac(value.BigInt(), pow10(scale))
	if rate.identity {
		return amount
	}
	amount.Mul(amount, new(big.Rat).SetFrac(rate.price, pow10(rate.priceScale)))
	amount.Quo(amount, new(big.Rat).SetFrac(rate.base, pow10(rate.baseScale)))
	t.recordUse(commodityID, onDate, rate)
	return amount
}

func pow10(scale int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
}

// resolvedRate is the conversion a commodity takes on a date: price over base
// quantity, direct or chained through a quote currency (#281), or identity
// for the reporting currency itself.
type resolvedRate struct {
	identity              bool
	price, base           *big.Int
	priceScale, baseScale int
	use                   RateUse
	// pending is the split factor a same-day trade quote was brought to the
	// end of its own day by; nil when none.
	pending *big.Rat
}

// endOfDay brings a trade-implied quote ordered before a split on its own day
// to that day's end-of-day units: each unit it priced became the pending
// factor's units, so the price per unit is divided by it, exactly. Every
// other quote already prices an end-of-day unit. A quote whose basis the
// holdings dispute prices unknown units: a conflicting_splits gap.
func (t *RateTable) endOfDay(commodityID int64, rate resolvedRate, observationID int64) (resolvedRate, bool) {
	basis := t.pendingSplits[observationID]
	if basis.Disputed {
		if _, recorded := t.gaps[commodityID]; !recorded {
			t.gaps[commodityID] = RateGap{CommodityID: commodityID, Reason: RateMissingConflictingSplits, NearestObservationDate: rate.use.ObservationDate}
		}
		return resolvedRate{}, false
	}
	factor := basis.Pending
	if factor == nil {
		return rate, true
	}
	rate.price = new(big.Int).Mul(rate.price, factor.Denom())
	rate.base = new(big.Int).Mul(rate.base, factor.Num())
	rate.pending = factor
	rate.use.SplitRatioNumerator, rate.use.SplitRatioDenominator = factor.Num().String(), factor.Denom().String()
	return rate, true
}

// resolve selects a commodity's rate on a date and records a gap on a miss.
// A direct rate, when usable, always wins; a chain never stands in for a
// missing leg.
func (t *RateTable) resolve(commodityID int64, onDate string) (resolvedRate, bool) {
	if commodityID == t.quoteCommodityID {
		return resolvedRate{identity: true}, true
	}
	if rate, found := t.observationOnOrBefore(t.byCommodity[commodityID], onDate); found {
		return t.direct(commodityID, onDate, rate)
	}
	return t.resolveChained(commodityID, onDate)
}

func (t *RateTable) direct(commodityID int64, onDate string, rate db.ReportingRateRecord) (resolvedRate, bool) {
	return t.endOfDay(commodityID, resolvedRate{
		price: big.NewInt(rate.PriceValue), priceScale: rate.PriceScale,
		base: big.NewInt(rate.BaseQuantityValue), baseScale: rate.BaseQuantityScale,
		use: RateUse{ObservationID: rate.ObservationID, Approximate: rate.IsApproximate,
			CommodityID: commodityID, ObservationDate: rate.ValuationDate, RequestedDate: onDate,
			Stale: rate.ValuationDate != onDate, Derived: rate.IsDerived},
	}, rate.ObservationID)
}

// DirectGap says whether the commodity's own observations in this table's
// quote currency value units of onDate, under exactly the window, end-of-day
// and split rules Convert applies, without chaining through another currency.
// It records nothing in the table's coverage. Prices judges a pair with it,
// so a quote reports cannot use is never listed as usable (#476).
func (t *RateTable) DirectGap(commodityID int64, onDate string) (RateGap, bool) {
	probe := t.withFreshCoverage()
	if rate, found := probe.observationOnOrBefore(probe.byCommodity[commodityID], onDate); found {
		resolved, ok := probe.direct(commodityID, onDate, rate)
		probe.inUnits(commodityID, onDate, resolved, ok)
	} else {
		probe.recordGap(commodityID, onDate)
	}
	gap, failed := probe.gaps[commodityID]
	return gap, failed
}

// resolveUnits is resolve for units of onDate: a security or crypto quote
// observed before onDate priced a unit of its own day, so its base quantity is
// multiplied by the instrument's split factor in between (#407), exactly; the
// use discloses the factor. A conflicting split day in between is a gap.
func (t *RateTable) resolveUnits(commodityID int64, onDate string) (resolvedRate, bool) {
	rate, ok := t.resolve(commodityID, onDate)
	return t.inUnits(commodityID, onDate, rate, ok)
}

func (t *RateTable) inUnits(commodityID int64, onDate string, rate resolvedRate, ok bool) (resolvedRate, bool) {
	if !ok || rate.identity || rate.use.ObservationDate == onDate {
		return rate, ok
	}
	factor, conflict := t.splitFactor(commodityID, rate.use.ObservationDate, onDate)
	if conflict != "" {
		if _, recorded := t.gaps[commodityID]; !recorded {
			t.gaps[commodityID] = RateGap{CommodityID: commodityID, Reason: RateMissingConflictingSplits, NearestObservationDate: rate.use.ObservationDate}
		}
		return resolvedRate{}, false
	}
	if factor.Cmp(big.NewRat(1, 1)) == 0 {
		return rate, true
	}
	rate.price = new(big.Int).Mul(rate.price, factor.Denom())
	rate.base = new(big.Int).Mul(rate.base, factor.Num())
	// The disclosed factor is the whole one from the quote's units to onDate's.
	total := new(big.Rat).Set(factor)
	if rate.pending != nil {
		total.Mul(total, rate.pending)
	}
	rate.use.SplitRatioNumerator, rate.use.SplitRatioDenominator = total.Num().String(), total.Denom().String()
	return rate, true
}

// recordUse preserves each selected observation/date, so a later estimated
// or stale use cannot disappear behind the first exact quote of that
// currency.
func (t *RateTable) recordUse(commodityID int64, onDate string, rate resolvedRate) {
	t.used[rateUseKey{commodityID, rate.use.ObservationID, rate.use.ViaObservationID, onDate}] = rate.use
}

// resolveChained values a commodity with no direct reporting rate through a
// currency it is priced in: value × price in that currency × that currency's
// reporting rate, rounded once by the caller (#281).
//
// With several usable quote currencies, the freshest first leg wins, then the
// freshest second leg, then the lowest currency id, so the choice is
// deterministic. When a price exists but no quote currency converts, the gap
// names that currency, because its rate is what the owner has to add.
func (t *RateTable) resolveChained(commodityID int64, onDate string) (resolvedRate, bool) {
	byQuote := t.chainQuotes[commodityID]
	quoteIDs := make([]int64, 0, len(byQuote))
	for quoteID := range byQuote {
		quoteIDs = append(quoteIDs, quoteID)
	}
	sort.Slice(quoteIDs, func(i, j int) bool { return quoteIDs[i] < quoteIDs[j] })

	var first, second db.ReportingRateRecord
	var found bool
	var missingLeg int64
	var missingLegDate string
	for _, quoteID := range quoteIDs {
		price, ok := t.observationOnOrBefore(byQuote[quoteID], onDate)
		if !ok {
			continue
		}
		rate, ok := t.observationOnOrBefore(t.byCommodity[quoteID], onDate)
		if !ok {
			if missingLeg == 0 || price.ValuationDate > missingLegDate {
				missingLeg, missingLegDate = quoteID, price.ValuationDate
			}
			continue
		}
		if !found || price.ValuationDate > first.ValuationDate ||
			(price.ValuationDate == first.ValuationDate && rate.ValuationDate > second.ValuationDate) {
			first, second, found = price, rate, true
		}
	}
	if !found {
		if missingLeg != 0 {
			t.recordChainGap(commodityID, missingLeg, onDate)
		} else {
			t.recordGap(commodityID, onDate)
		}
		return resolvedRate{}, false
	}
	return t.endOfDay(commodityID, resolvedRate{
		price:      new(big.Int).Mul(big.NewInt(first.PriceValue), big.NewInt(second.PriceValue)),
		priceScale: first.PriceScale + second.PriceScale,
		base:       new(big.Int).Mul(big.NewInt(first.BaseQuantityValue), big.NewInt(second.BaseQuantityValue)),
		baseScale:  first.BaseQuantityScale + second.BaseQuantityScale,
		use: RateUse{ObservationID: first.ObservationID, Approximate: first.IsApproximate || second.IsApproximate,
			CommodityID: commodityID, ObservationDate: first.ValuationDate, RequestedDate: onDate,
			Stale:   first.ValuationDate != onDate || second.ValuationDate != onDate,
			Derived: true, ViaCommodityID: second.BaseCommodityID, ViaObservationID: second.ObservationID, ViaObservationDate: second.ValuationDate},
	}, first.ObservationID)
}

// rateFor finds a commodity's direct reporting rate, recording a gap on a miss.
func (t *RateTable) rateFor(commodityID int64, onDate string) (db.ReportingRateRecord, bool) {
	rate, found := t.observationOnOrBefore(t.byCommodity[commodityID], onDate)
	if !found {
		t.recordGap(commodityID, onDate)
	}
	return rate, found
}

// observationOnOrBefore finds the applicable observation: the latest one on or
// before the date, provided it is inside the staleness window. It records
// nothing; the caller decides which gap a miss is.
func (t *RateTable) observationOnOrBefore(observations []db.ReportingRateRecord, onDate string) (db.ReportingRateRecord, bool) {
	if len(observations) == 0 {
		return db.ReportingRateRecord{}, false
	}

	earliestUsable, err := shiftISODate(onDate, -t.stalenessDays)
	if err != nil {
		return db.ReportingRateRecord{}, false
	}

	var best db.ReportingRateRecord
	var found bool
	for _, observation := range observations {
		if observation.ValuationDate > onDate {
			break
		}
		if observation.ValuationDate < earliestUsable {
			continue
		}
		best, found = observation, true
	}
	return best, found
}

func (t *RateTable) recordGap(commodityID int64, onDate string) {
	if _, ok := t.gaps[commodityID]; ok {
		return
	}
	t.gaps[commodityID] = t.pairGap(commodityID, commodityID, onDate)
}

// recordChainGap records a commodity whose price leg exists but whose quote
// currency has no usable reporting rate; the reason is the currency's.
func (t *RateTable) recordChainGap(commodityID, viaCommodityID int64, onDate string) {
	if _, ok := t.gaps[commodityID]; ok {
		return
	}
	gap := t.pairGap(commodityID, viaCommodityID, onDate)
	gap.ViaCommodityID = viaCommodityID
	t.gaps[commodityID] = gap
}

// pairGap explains why rateCommodityID has no usable reporting rate on onDate,
// reported against commodityID.
func (t *RateTable) pairGap(commodityID, rateCommodityID int64, onDate string) RateGap {
	gap := RateGap{CommodityID: commodityID, Reason: RateMissingNoObservation}
	reason := t.diagnostics[[2]int64{rateCommodityID, t.quoteCommodityID}]
	if reason != "" && reason != "usable" {
		gap.Reason = reason
	}
	// Never identify an observation after this specific bucket/posting as a
	// nearest prior rate, even when a later bucket in the same report has one.
	for _, observation := range t.byCommodity[rateCommodityID] {
		if observation.ValuationDate > onDate {
			break
		}
		gap.NearestObservationDate = observation.ValuationDate
	}
	if gap.NearestObservationDate == "" && reason == "usable" {
		gap.Reason = "future_only"
	}
	return gap
}

// withFreshCoverage is a copy of t that shares its loaded observations, which
// are only read after loading, but records its own uses and gaps. Each R13
// instrument sleeve values through one, so a price only a sleeve needed never
// appears in another figure's coverage.
func (t *RateTable) withFreshCoverage() *RateTable {
	copied := *t
	copied.used = map[rateUseKey]RateUse{}
	copied.gaps = map[int64]RateGap{}
	return &copied
}

// Coverage reports what the table actually did, for the response to carry.
func (t *RateTable) Coverage(code string) ValuationCoverage {
	policy := t.selectionPolicy
	if policy == "" {
		policy = SelectionPolicyLiveRaw
	}
	coverage := ValuationCoverage{
		SelectionPolicy: policy, ValuationAsOf: t.boundary.AsOf, KnowledgeCutoff: t.boundary.RecordedAt,
		CommodityID:      t.quoteCommodityID,
		Code:             code,
		Scale:            t.quoteScale,
		Method:           ValuationMethodObservedOnOrBefore,
		MaxStalenessDays: t.stalenessDays,
		Complete:         len(t.gaps) == 0,
	}
	for _, use := range t.used {
		coverage.Used = append(coverage.Used, use)
	}
	for _, gap := range t.gaps {
		coverage.Gaps = append(coverage.Gaps, gap)
	}
	sort.Slice(coverage.Used, func(i, j int) bool {
		a, b := coverage.Used[i], coverage.Used[j]
		if a.CommodityID != b.CommodityID {
			return a.CommodityID < b.CommodityID
		}
		return a.RequestedDate < b.RequestedDate
	})
	sort.Slice(coverage.Gaps, func(i, j int) bool { return coverage.Gaps[i].CommodityID < coverage.Gaps[j].CommodityID })
	return coverage
}

// ConvertedTotal accumulates converted amounts. It is a plain sum: every
// addend has already been rounded to the reporting currency's scale, so there
// is nothing left to align.
type ConvertedTotal struct {
	total   *big.Int
	scale   int
	partial bool
}

func NewConvertedTotal(scale int) *ConvertedTotal {
	return &ConvertedTotal{total: new(big.Int), scale: scale}
}

// Add includes one converted amount. Adding an unconvertible amount marks the
// total partial rather than skipping it silently.
func (c *ConvertedTotal) Add(value *big.Int, ok bool) {
	if !ok {
		c.partial = true
		return
	}
	c.total.Add(c.total, value)
}

// Value returns the total and whether it accounts for everything it was asked
// to. A partial total is not returned as a number: a figure that quietly means
// "most of it" is worse than no figure.
func (c *ConvertedTotal) Value() (exact.Coefficient, int, bool) {
	if c.partial {
		return "", c.scale, false
	}
	coefficient, err := exact.FromBig(c.total)
	if err != nil {
		return "", c.scale, false
	}
	return coefficient, c.scale, true
}

func shiftISODate(date string, days int) (string, error) {
	parsed, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return "", fmt.Errorf("parse date %q: %w", date, err)
	}
	return parsed.AddDate(0, 0, days).Format(time.DateOnly), nil
}
