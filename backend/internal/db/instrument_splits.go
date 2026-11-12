package db

import (
	"context"
	"fmt"
	"math/big"
	"time"
)

// InstrumentSplit is one effective split of an instrument: on EffectiveOn each
// unit became RatioNumerator / RatioDenominator units. AccountID is the
// holding that recorded it; OrderOperationID is its same-day replay slot (its
// correction root), so a same-day trade or delivery with an earlier slot was
// split and one with a later slot was not.
type InstrumentSplit struct {
	CommodityID      int64
	AccountID        int64
	EffectiveOn      string
	RatioNumerator   int64
	RatioDenominator int64
	OrderOperationID int64
}

// correctionRootsOf maps each investment operation selected by seed to its
// correction root, the same-day slot replay orders by
// (investmentReplayOrderOperationIDsQuery). It walks up each seed's
// correction_of_operation_id chain rather than down from every root, so the
// cost follows the operations a read needs, not the book's whole history
// (#561). An operation whose chain leaves the book has no root, as before.
// prelude, when not empty, is one or more leading CTEs ending in a comma that
// seed may read. Parameters: the prelude's, the seed's, book, book.
func correctionRootsOf(prelude, seed string) string {
	// CROSS JOIN keeps SQLite from driving the walk from every book
	// operation and probing the seeds; each seed is one primary-key lookup.
	return `WITH RECURSIVE ` + prelude + `correction_seeds(operation_id) AS (SELECT DISTINCT * FROM (` + seed + `)),
correction_ancestry(operation_id, ancestor_id, parent_id) AS (
	SELECT o.id, o.id, o.correction_of_operation_id FROM correction_seeds s
	CROSS JOIN investment_operations o ON o.id = s.operation_id
	WHERE o.book_id = ?
	UNION ALL
	SELECT a.operation_id, parent.id, parent.correction_of_operation_id FROM correction_ancestry a
	JOIN investment_operations parent ON parent.id = a.parent_id
	WHERE parent.book_id = ?
), correction_roots(operation_id, root_id) AS (
	SELECT operation_id, ancestor_id FROM correction_ancestry WHERE parent_id IS NULL
) `
}

// TotalReturnSecurities reports whether this repository prices securities by
// total-return observations, which are continuous index levels and are never
// split-adjusted.
func (r *PricingRepository) TotalReturnSecurities() bool {
	return r.selection == totalReturnSelectionCTE
}

// InstrumentSplits returns the effective splits of the given instruments
// recorded in any holding, after `after` and through `through`, ordered by
// instrument and date. Reversed and replaced splits are excluded. The same
// split recorded in several holdings appears once per holding; a holding's
// successive splits on one day each appear. A raw quote prices a unit of its
// own day, so valuation reads these to bring a stale quote to a later day's
// units (#407).
func (r *PricingRepository) InstrumentSplits(ctx context.Context, bookID int64, commodityIDs []int64, after, through string) ([]InstrumentSplit, error) {
	return instrumentSplits(ctx, r.database, bookID, commodityIDs, after, through)
}

func instrumentSplits(ctx context.Context, queryer rowsQueryer, bookID int64, commodityIDs []int64, after, through string) ([]InstrumentSplit, error) {
	if len(commodityIDs) == 0 {
		return nil, nil
	}
	clause, args := inClause("f.commodity_id", commodityIDs)
	// Select the effective splits in range once: they are both the result and
	// the only operations whose roots are needed. Seeding superseded facts
	// too would walk a replacement chain once per version (#562 review), and
	// repeating the IN list would halve how many instruments one read takes.
	effectiveSplits := `effective_splits AS MATERIALIZED (
		SELECT f.operation_id, f.commodity_id, f.account_id, f.effective_on, f.ratio_numerator, f.ratio_denominator
		FROM investment_split_facts f
		JOIN effective_investment_operations o ON o.id = f.operation_id
		WHERE f.book_id = ? AND ` + clause + ` AND f.effective_on > ? AND f.effective_on <= ?
	), `
	queryArgs := append(append(append([]any{bookID}, args...), after, through), bookID, bookID)
	rows, err := queryer.QueryContext(ctx, correctionRootsOf(effectiveSplits, `SELECT operation_id FROM effective_splits`)+`
		SELECT s.commodity_id, s.account_id, s.effective_on, s.ratio_numerator, s.ratio_denominator, root.root_id
		FROM effective_splits s
		JOIN correction_roots root ON root.operation_id = s.operation_id
		ORDER BY s.commodity_id, s.effective_on, root.root_id`, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("read instrument splits: %w", err)
	}
	defer rows.Close()
	var splits []InstrumentSplit
	for rows.Next() {
		var split InstrumentSplit
		if err := rows.Scan(&split.CommodityID, &split.AccountID, &split.EffectiveOn, &split.RatioNumerator, &split.RatioDenominator, &split.OrderOperationID); err != nil {
			return nil, fmt.Errorf("scan instrument split: %w", err)
		}
		splits = append(splits, split)
	}
	return splits, rows.Err()
}

// SplitDay is one day's split factor of an instrument: every unit held at the
// end of the previous day became Factor units (a holding's successive splits
// that day compounded). Exact, so it is never rounded.
type SplitDay struct {
	EffectiveOn string
	Factor      *big.Rat
}

// InstrumentSplitDays combines recorded splits (ordered by instrument, then
// date) into one factor per instrument and day. A holding's successive splits
// on one day compound (2:1 twice is 4:1, as its ledger units are); holdings
// that recorded the same event must agree on the day's factor. A day they
// disagree on has no factor and is a conflict, rather than a guess.
//
// The factor is the instrument's, not a holding's: a quote prices the
// instrument, so a holding that never recorded a split is misvalued by a
// fresh post-split quote just as by a rescaled stale one.
func InstrumentSplitDays(recorded []InstrumentSplit) (map[int64][]SplitDay, map[int64][]string) {
	days := map[int64][]SplitDay{}
	conflicts := map[int64][]string{}
	for start := 0; start < len(recorded); {
		first := recorded[start]
		end := start
		for end < len(recorded) && recorded[end].CommodityID == first.CommodityID && recorded[end].EffectiveOn == first.EffectiveOn {
			end++
		}
		byHolding := map[int64]*big.Rat{}
		for _, split := range recorded[start:end] {
			if byHolding[split.AccountID] == nil {
				byHolding[split.AccountID] = big.NewRat(1, 1)
			}
			byHolding[split.AccountID].Mul(byHolding[split.AccountID], big.NewRat(split.RatioNumerator, split.RatioDenominator))
		}
		var factor *big.Rat
		agree := true
		for _, holding := range byHolding {
			agree = agree && (factor == nil || factor.Cmp(holding) == 0)
			factor = holding
		}
		if agree {
			days[first.CommodityID] = append(days[first.CommodityID], SplitDay{EffectiveOn: first.EffectiveOn, Factor: factor})
		} else {
			conflicts[first.CommodityID] = append(conflicts[first.CommodityID], first.EffectiveOn)
		}
		start = end
	}
	return days, conflicts
}

// SplitFactor is the product of an instrument's split factors effective in
// (observed, onDate]: how many units on onDate one unit of the observation's
// day became. A conflicting day in that span returns that day instead.
func SplitFactor(days []SplitDay, conflicts []string, observed, onDate string) (*big.Rat, string) {
	for _, conflict := range conflicts {
		if conflict > observed && conflict <= onDate {
			return nil, conflict
		}
	}
	factor := big.NewRat(1, 1)
	for _, split := range days {
		if split.EffectiveOn > observed && split.EffectiveOn <= onDate {
			factor.Mul(factor, split.Factor)
		}
	}
	return factor, ""
}

// previousISODate is the calendar day before date, or date when it is not one.
func previousISODate(date string) string {
	parsed, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return date
	}
	return parsed.AddDate(0, 0, -1).Format(time.DateOnly)
}

// PendingSplitFactor is how many end-of-day units one unit of an event earlier
// in a split day became: the product of the splits the same holding recorded
// for the instrument that day in a later replay slot. It is for quantities,
// which belong to a holding; a quote's basis is instrument-wide
// (TradeQuotePendingSplits). A same-day trade or
// delivery ordered before the split carried pre-split units (the split split
// them), one ordered after it carried post-split units. Nil when none is
// pending. Splits must be ordered as InstrumentSplits returns them.
func PendingSplitFactor(splits []InstrumentSplit, commodityID, accountID int64, date string, slot int64) *big.Rat {
	var factor *big.Rat
	for _, split := range splits {
		if split.CommodityID != commodityID || split.AccountID != accountID || split.EffectiveOn != date || split.OrderOperationID <= slot {
			continue
		}
		if factor == nil {
			factor = big.NewRat(1, 1)
		}
		factor.Mul(factor, big.NewRat(split.RatioNumerator, split.RatioDenominator))
	}
	return factor
}

// TradeQuoteBasis is how a trade-implied quote dated on a split day relates
// to that day's end-of-day units. A quote prices the instrument, not one
// holding, so its basis is instrument-wide: Pending is the factor of the
// day's splits recorded after the trade's slot, which every holding that
// recorded the day's split agrees on. Disputed is set when they do not (the
// trade falls between one holding's split and another's): the units the
// quote priced are then unknown.
type TradeQuoteBasis struct {
	Pending  *big.Rat
	Disputed bool
}

// TradeQuotePendingSplits returns, per trade-implied observation of the given
// instruments dated in (after, through] on a day with a recorded split, its
// TradeQuoteBasis: such a quote is cash per unit of its trade, and a trade
// ordered before that day's split priced pre-split units. Observations with
// nothing pending and nothing disputed are omitted. Quantities follow the
// operation's own holding instead (PendingSplitFactor): units are a
// holding's, a price is the instrument's.
func (r *PricingRepository) TradeQuotePendingSplits(ctx context.Context, bookID int64, commodityIDs []int64, after, through string) (map[int64]TradeQuoteBasis, error) {
	return tradeQuotePendingSplits(ctx, r.database, bookID, commodityIDs, after, through)
}

func tradeQuotePendingSplits(ctx context.Context, queryer rowsQueryer, bookID int64, commodityIDs []int64, after, through string) (map[int64]TradeQuoteBasis, error) {
	bases := map[int64]TradeQuoteBasis{}
	if len(commodityIDs) == 0 {
		return bases, nil
	}
	clause, args := inClause("po.base_commodity_id", commodityIDs)
	// Match each trade-implied observation to the effective splits recorded
	// on its day once; roots are then needed only for those trades and splits.
	// CROSS JOIN reads the instrument's observations once and seeks each
	// day's splits by investment_split_facts_lookup_idx; driving from the
	// splits instead rescans the observations once per split (#562 review).
	matches := `split_day_quotes AS MATERIALIZED (
		SELECT po.id AS observation_id, link.operation_id AS trade_operation_id, f.operation_id AS split_operation_id,
			f.account_id, f.ratio_numerator, f.ratio_denominator
		FROM price_observations po
		CROSS JOIN investment_split_facts f ON f.book_id = po.book_id AND f.commodity_id = po.base_commodity_id
			AND f.effective_on = po.valuation_date
		JOIN investment_operation_journal_links link ON link.transaction_version_id = po.source_transaction_version_id
		JOIN effective_investment_operations split ON split.id = f.operation_id
		WHERE po.book_id = ? AND po.quote_type = 'trade_implied' AND ` + clause + `
			AND po.valuation_date > ? AND po.valuation_date <= ?
	), `
	seeds := `SELECT trade_operation_id FROM split_day_quotes UNION SELECT split_operation_id FROM split_day_quotes`
	queryArgs := append(append(append([]any{bookID}, args...), after, through), bookID, bookID)
	rows, err := queryer.QueryContext(ctx, correctionRootsOf(matches, seeds)+`
		SELECT q.observation_id, q.account_id, q.ratio_numerator, q.ratio_denominator, slot.root_id > trade.root_id
		FROM split_day_quotes q
		JOIN correction_roots trade ON trade.operation_id = q.trade_operation_id
		JOIN correction_roots slot ON slot.operation_id = q.split_operation_id
		ORDER BY q.observation_id, q.account_id, slot.root_id`, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("read trade quote pending splits: %w", err)
	}
	defer rows.Close()
	// Per observation, per holding: the factor that holding recorded after the
	// trade (1 when all of its splits that day came before it).
	byHolding := map[int64]map[int64]*big.Rat{}
	var order []int64
	for rows.Next() {
		var observationID, accountID, numerator, denominator int64
		var afterTrade bool
		if err := rows.Scan(&observationID, &accountID, &numerator, &denominator, &afterTrade); err != nil {
			return nil, fmt.Errorf("scan trade quote pending split: %w", err)
		}
		if byHolding[observationID] == nil {
			byHolding[observationID] = map[int64]*big.Rat{}
			order = append(order, observationID)
		}
		if byHolding[observationID][accountID] == nil {
			byHolding[observationID][accountID] = big.NewRat(1, 1)
		}
		if afterTrade {
			byHolding[observationID][accountID].Mul(byHolding[observationID][accountID], big.NewRat(numerator, denominator))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trade quote pending splits: %w", err)
	}
	one := big.NewRat(1, 1)
	for _, observationID := range order {
		var pending *big.Rat
		disputed := false
		for _, factor := range byHolding[observationID] {
			disputed = disputed || (pending != nil && pending.Cmp(factor) != 0)
			pending = factor
		}
		switch {
		case disputed:
			bases[observationID] = TradeQuoteBasis{Disputed: true}
		case pending.Cmp(one) != 0:
			bases[observationID] = TradeQuoteBasis{Pending: pending}
		}
	}
	return bases, nil
}

// splitAdjustPositionPrices brings each position's latest raw quote to the
// units of its valuation date (#408), the rule RateTable applies to every
// other consumer (#407): a quote observed before a recorded split priced
// pre-split units, so valuePositions divides it by the instrument's split
// factor in between, recorded on the position. A trade-implied quote ordered
// before a split on its own day priced pre-split units too, so the split
// still pending after its trade joins the factor. A conflicting split day in
// between leaves the position unvalued as conflicting_splits. Holdings read
// live_raw_v1 only; total-return levels never reach this path.
func splitAdjustPositionPrices(ctx context.Context, queryer rowsQueryer, bookID int64, positions []InvestmentPositionRecord) error {
	var commodityIDs []int64
	seen := map[int64]bool{}
	after, through := "", ""
	for _, position := range positions {
		observed := position.LatestPriceDate
		if !observed.Valid || observed.String > position.ValuationAsOf {
			continue
		}
		if !seen[position.CommodityID] {
			seen[position.CommodityID] = true
			commodityIDs = append(commodityIDs, position.CommodityID)
		}
		if after == "" || observed.String < after {
			after = observed.String
		}
		through = max(through, position.ValuationAsOf)
	}
	if len(commodityIDs) == 0 {
		return nil
	}
	recorded, err := instrumentSplits(ctx, queryer, bookID, commodityIDs, after, through)
	if err != nil {
		return err
	}
	// A quote's own day can hold a split still pending after its trade.
	pending, err := tradeQuotePendingSplits(ctx, queryer, bookID, commodityIDs, previousISODate(after), through)
	if err != nil {
		return err
	}
	days, conflicts := InstrumentSplitDays(recorded)
	for i := range positions {
		position := &positions[i]
		observed := position.LatestPriceDate
		if !observed.Valid || observed.String > position.ValuationAsOf {
			continue
		}
		factor, conflict := SplitFactor(days[position.CommodityID], conflicts[position.CommodityID], observed.String, position.ValuationAsOf)
		if basis := pending[position.latestPriceObservationID.Int64]; conflict == "" {
			switch {
			case basis.Disputed:
				conflict = position.LatestPriceDate.String
			case basis.Pending != nil:
				factor.Mul(factor, basis.Pending)
			}
		}
		switch {
		case conflict != "":
			position.PriceStatus = ValuationConflictingSplits
		case factor.Cmp(big.NewRat(1, 1)) != 0:
			position.splitFactor = factor
			position.SplitRatioNumerator, position.SplitRatioDenominator = factor.Num().String(), factor.Denom().String()
		}
	}
	return nil
}
