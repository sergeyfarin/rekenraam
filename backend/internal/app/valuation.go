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
)

// ReportingCurrencyInput is what a caller asks for.
type ReportingCurrencyInput struct {
	CommodityID     int64
	MaxStalenessDay int
}

// RateUse records that a conversion happened and on what: the rate's own
// observation date, and whether that date was the one asked for.
type RateUse struct {
	CommodityID     int64  `json:"commodity_id"`
	ObservationDate string `json:"observation_date"`
	RequestedDate   string `json:"requested_date"`
	Stale           bool   `json:"stale"`
	Derived         bool   `json:"derived"`
}

// RateGap is a commodity that could not be converted, and why.
type RateGap struct {
	CommodityID int64  `json:"commodity_id"`
	Reason      string `json:"reason"`
	// NearestObservationDate is the closest observation that exists outside the
	// window, when there is one. "No rate" and "no rate recently" are different
	// problems and lead to different fixes.
	NearestObservationDate string `json:"nearest_observation_date,omitempty"`
}

// ValuationCoverage is what a response says about its own conversions.
type ValuationCoverage struct {
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

// RateTable resolves a commodity and a date to a rate, once, for a whole
// report. Built from one query over the window the report covers.
type RateTable struct {
	quoteCommodityID int64
	quoteScale       int
	stalenessDays    int
	// byCommodity holds each commodity's observations in date order.
	byCommodity map[int64][]db.ReportingRateRecord
	// outside remembers the nearest observation that exists but is too old, so
	// a gap can say which problem it is.
	outside map[int64]string

	used map[int64]RateUse
	gaps map[int64]RateGap
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
	if s.pricingRepository == nil {
		return nil, fmt.Errorf("reporting currency requires pricing data")
	}

	staleness := reporting.MaxStalenessDay
	if staleness <= 0 {
		staleness = DefaultRateStalenessDays
	}

	quote, err := s.pricingRepository.ReportingCurrency(ctx, BookID, reporting.CommodityID)
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

	table := &RateTable{
		quoteCommodityID: reporting.CommodityID,
		quoteScale:       quote.StandardScale,
		stalenessDays:    staleness,
		byCommodity:      map[int64][]db.ReportingRateRecord{},
		outside:          map[int64]string{},
		used:             map[int64]RateUse{},
		gaps:             map[int64]RateGap{},
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
	rates, err := s.pricingRepository.ReportingRates(ctx, BookID, reporting.CommodityID, needed, windowStart, toDate)
	if err != nil {
		return nil, err
	}
	for _, rate := range rates {
		table.byCommodity[rate.BaseCommodityID] = append(table.byCommodity[rate.BaseCommodityID], rate)
	}

	// Anything with no observation in the widened window may still have one
	// further back; knowing which tells a reader whether to add a rate or to
	// refresh them.
	for _, commodityID := range needed {
		if len(table.byCommodity[commodityID]) > 0 {
			continue
		}
		nearest, err := s.pricingRepository.ReportingRates(ctx, BookID, reporting.CommodityID, []int64{commodityID}, "0001-01-01", toDate)
		if err != nil {
			return nil, err
		}
		if len(nearest) > 0 {
			table.outside[commodityID] = nearest[len(nearest)-1].ValuationDate
		}
	}

	return table, nil
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
	if commodityID == t.quoteCommodityID {
		// The reporting currency converts to itself at 1, exactly, with no
		// observation involved and nothing to round.
		aligned := exact.ScaledIntFromCoefficient(value, scale)
		aligned.Align(t.quoteScale)
		return aligned.BigInt(), true
	}

	rate, found := t.rateFor(commodityID, onDate)
	if !found {
		return nil, false
	}

	converted, err := exact.MulDivRound(
		value.BigInt(), scale,
		big.NewInt(rate.PriceValue), rate.PriceScale,
		big.NewInt(rate.BaseQuantityValue), rate.BaseQuantityScale,
		t.quoteScale,
	)
	if err != nil {
		t.gaps[commodityID] = RateGap{CommodityID: commodityID, Reason: RateMissingNoObservation}
		return nil, false
	}

	stale := rate.ValuationDate != onDate
	// One record per commodity, keeping the worst case: a reader needs to know
	// that *some* figure leaned on a stale rate, not that most did not.
	if existing, ok := t.used[commodityID]; !ok || (stale && !existing.Stale) {
		t.used[commodityID] = RateUse{
			CommodityID:     commodityID,
			ObservationDate: rate.ValuationDate,
			RequestedDate:   onDate,
			Stale:           stale,
			Derived:         rate.IsDerived,
		}
	}

	return converted, true
}

// rateFor finds the applicable observation: the latest one on or before the
// date, provided it is inside the staleness window.
func (t *RateTable) rateFor(commodityID int64, onDate string) (db.ReportingRateRecord, bool) {
	observations := t.byCommodity[commodityID]
	if len(observations) == 0 {
		t.recordGap(commodityID)
		return db.ReportingRateRecord{}, false
	}

	earliestUsable, err := shiftISODate(onDate, -t.stalenessDays)
	if err != nil {
		t.recordGap(commodityID)
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
	if !found {
		t.recordGap(commodityID)
		return db.ReportingRateRecord{}, false
	}
	return best, true
}

func (t *RateTable) recordGap(commodityID int64) {
	if _, ok := t.gaps[commodityID]; ok {
		return
	}
	gap := RateGap{CommodityID: commodityID, Reason: RateMissingNoObservation}
	if nearest, ok := t.outside[commodityID]; ok {
		gap.NearestObservationDate = nearest
	} else if observations := t.byCommodity[commodityID]; len(observations) > 0 {
		gap.NearestObservationDate = observations[len(observations)-1].ValuationDate
	}
	t.gaps[commodityID] = gap
}

// Coverage reports what the table actually did, for the response to carry.
func (t *RateTable) Coverage(code string) ValuationCoverage {
	coverage := ValuationCoverage{
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
	sort.Slice(coverage.Used, func(i, j int) bool { return coverage.Used[i].CommodityID < coverage.Used[j].CommodityID })
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
