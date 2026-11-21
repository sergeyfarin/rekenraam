package db

import (
	"context"
	"fmt"
	"time"
)

// ObservationBoundary is a live read boundary, not a historical book snapshot.
// Retirements are evaluated now; RecordedAt limits admission of observations.
type ObservationBoundary struct {
	AsOf       string
	RecordedAt string
}

// CurrentObservationBoundary uses the operation's owner day when it has one
// and takes the recording cutoff from now in either case.
func (r *PricingRepository) CurrentObservationBoundary(ctx context.Context, bookID int64, now time.Time) (ObservationBoundary, error) {
	boundary := ObservationBoundary{AsOf: pinnedOwnerToday(ctx), RecordedAt: now.UTC().Format(time.RFC3339Nano)}
	if boundary.AsOf != "" {
		return boundary, nil
	}

	zone, err := r.BookOwnerTimeZone(ctx, bookID)
	if err != nil {
		return ObservationBoundary{}, err
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return ObservationBoundary{}, fmt.Errorf("load valuation owner zone: %w", err)
	}
	boundary.AsOf = now.In(location).Format(time.DateOnly)
	return boundary, nil
}

func (r *PricingRepository) observationBoundary(ctx context.Context, bookID int64, boundaries []ObservationBoundary) (ObservationBoundary, error) {
	if len(boundaries) > 0 {
		return boundaries[0], nil
	}
	return r.CurrentObservationBoundary(ctx, bookID, time.Now())
}

// observationSelectionCTE is the single SQL owner of the live_raw_v1 policy.
// Parameters: book, valuation date ceiling, knowledge timestamp. Date outranks
// source; within a date manual/override > exact > approximate, then time/ID.
// Adjusted series cannot value unadjusted ledger units. Bid/ask and security
// closes used as FX are deliberately incompatible, rather than fallback data.
var observationSelectionCTE = observationSelection(rawSecurityQuotes)

// totalReturnSelectionCTE is the same selection with one change: a security or
// crypto unit is priced only by observations declared total_return. It values
// an R13 benchmark declared total-return, never a ledger balance; currency
// legs stay raw.
var totalReturnSelectionCTE = observationSelection(`po.adjustment_basis = 'total_return'`)

// rawSecurityQuotes admits a non-currency commodity's unadjusted prices.
const rawSecurityQuotes = `(po.adjustment_basis = 'raw' OR (po.quote_type = 'trade_implied' AND po.adjustment_basis = 'not_applicable'))`

func observationSelection(securityQuotes string) string {
	return `WITH
 selection_input(book_id, as_of, recorded_at) AS (VALUES (?, ?, ?)),
 selection_candidates AS (
  SELECT po.*, CASE WHEN
   (c.kind = 'currency' AND po.adjustment_basis IN ('raw','not_applicable')
    AND po.quote_type IN ('manual','valuation_override','official_fixing','spot_mid','trade_implied'))
   OR (c.kind <> 'currency' AND ` + securityQuotes + `
    AND po.quote_type IN ('manual','valuation_override','close','nav','spot_mid','trade_implied'))
   THEN 1 ELSE 0 END AS compatible
  FROM price_observations po
  JOIN commodities c ON c.id = po.base_commodity_id
  JOIN selection_input si ON si.book_id = po.book_id
  WHERE po.voided_at IS NULL
 ),
 selection_ranked AS (
  SELECT po.*, ROW_NUMBER() OVER (
   PARTITION BY base_commodity_id, quote_commodity_id, valuation_date
   ORDER BY CASE WHEN is_manual = 1 OR quote_type = 'valuation_override' THEN 3
    WHEN is_approximate = 0 THEN 2 ELSE 1 END DESC,
    julianday(po.recorded_at) DESC, id DESC
  ) AS day_rank
  FROM selection_candidates po CROSS JOIN selection_input si
  WHERE compatible = 1 AND valuation_date <= si.as_of
   AND julianday(po.recorded_at) <= julianday(si.recorded_at)
 ),
 selection_daily AS (SELECT * FROM selection_ranked WHERE day_rank = 1),
 selection_latest_ranked AS (
  SELECT *, ROW_NUMBER() OVER (
   PARTITION BY base_commodity_id, quote_commodity_id ORDER BY valuation_date DESC
  ) AS latest_rank FROM selection_daily
 ),
 selection_latest AS (SELECT * FROM selection_latest_ranked WHERE latest_rank = 1),
 selection_diagnostics AS (
  SELECT base_commodity_id, quote_commodity_id,
   CASE
    WHEN MAX(CASE WHEN compatible = 1 AND valuation_date <= si.as_of
     AND julianday(po.recorded_at) <= julianday(si.recorded_at) THEN 1 ELSE 0 END) = 1 THEN 'usable'
    WHEN MAX(CASE WHEN compatible = 1 AND julianday(po.recorded_at) <= julianday(si.recorded_at) THEN 1 ELSE 0 END) = 1 THEN 'future_only'
    WHEN MAX(CASE WHEN julianday(po.recorded_at) <= julianday(si.recorded_at) THEN 1 ELSE 0 END) = 1 THEN 'incompatible_quote_basis'
    ELSE 'not_known_at_cutoff' END AS reason
  FROM selection_candidates po CROSS JOIN selection_input si
  GROUP BY base_commodity_id, quote_commodity_id
 ) `
}

func (r *PricingRepository) ObservationDiagnostics(ctx context.Context, bookID int64, pairs [][2]int64, boundary ObservationBoundary) (map[[2]int64]string, error) {
	result := map[[2]int64]string{}
	if len(pairs) == 0 {
		return result, nil
	}
	clause, args := observationPairClause(pairs)
	params := append([]any{bookID, boundary.AsOf, boundary.RecordedAt}, args...)
	rows, err := r.database.QueryContext(ctx, r.selectionCTE()+`SELECT base_commodity_id, quote_commodity_id, reason FROM selection_diagnostics WHERE `+clause, params...)
	if err != nil {
		return nil, fmt.Errorf("read observation diagnostics: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var pair [2]int64
		var reason string
		if err := rows.Scan(&pair[0], &pair[1], &reason); err != nil {
			return nil, err
		}
		result[pair] = reason
	}
	return result, rows.Err()
}

func (r *InvestmentRepository) CurrentObservationBoundary(ctx context.Context, bookID int64, now time.Time) (ObservationBoundary, error) {
	return NewPricingRepository(r.database).CurrentObservationBoundary(ctx, bookID, now)
}
