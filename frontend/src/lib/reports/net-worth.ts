import type { BalanceQuantity, NetWorthSeriesResponse } from '$lib/api/ledger';

export type NetWorthRow = BalanceQuantity & {
  startDate: string;
  endDate: string;
};

export function netWorthRows(report: NetWorthSeriesResponse): NetWorthRow[] {
  return report.buckets.flatMap((bucket) =>
    bucket.totals.map((total) => ({
      ...total,
      startDate: bucket.start_date,
      endDate: bucket.end_date
    }))
  );
}

export function hasMultipleCommodities(rows: NetWorthRow[]): boolean {
  return new Set(rows.map((row) => row.commodity_id)).size > 1;
}
