import type { BalanceQuantity } from '$lib/api/ledger';
import type { NetWorthSeriesResponse } from '$lib/api/reports';

export type NetWorthRow = BalanceQuantity & {
  startDate: string;
  endDate: string;
  /**
   * True for the reporting-currency restatement of a bucket. It is not a
   * holding — nobody owns it — so it is excluded from anything that reasons
   * about which commodities the book is in.
   */
  converted: boolean;
};

export function netWorthRows(report: NetWorthSeriesResponse): NetWorthRow[] {
  return report.buckets.flatMap((bucket) => {
    const held = bucket.totals.map((total) => ({
      ...total,
      startDate: bucket.start_date,
      endDate: bucket.end_date,
      converted: false
    }));
    // A bucket with no holdings has nothing to restate. The backend converts an
    // empty bucket to an exact zero, which is true but is not a net worth —
    // emitting it would replace the "no data yet" empty state with a table of
    // zeros.
    if (!bucket.converted || held.length === 0) {
      return held;
    }
    // After the holdings, not before: the restatement is a summary of the rows
    // above it, and reading it first would suggest the holdings break it down.
    return [
      ...held,
      { ...bucket.converted, startDate: bucket.start_date, endDate: bucket.end_date, converted: true }
    ];
  });
}

export function heldRows(rows: NetWorthRow[]): NetWorthRow[] {
  return rows.filter((row) => !row.converted);
}

export function hasMultipleCommodities(rows: NetWorthRow[]): boolean {
  return new Set(heldRows(rows).map((row) => row.commodity_id)).size > 1;
}

/**
 * The converted restatements, but only when every bucket has one.
 *
 * A series with a hole in it would draw a fall to zero that never happened, so
 * a partial conversion charts nothing at all. The table still shows whichever
 * buckets converted, and the valuation notice names what was missing.
 */
export function convertedSeries(rows: NetWorthRow[]): NetWorthRow[] {
  const buckets = new Set(rows.map((row) => `${row.startDate}/${row.endDate}`));
  const converted = rows.filter((row) => row.converted);
  return converted.length > 0 && converted.length === buckets.size ? converted : [];
}
