import type { CashflowResponse } from '$lib/api/ledger';

/**
 * Flattens the cashflow report into table rows.
 *
 * One row per (bucket, commodity). Buckets with no activity are kept as rows
 * with no commodity, because a gap in a cashflow table reads as missing data
 * rather than as a quiet month — the backend returns those empty buckets for
 * the same reason.
 *
 * No arithmetic happens here. Every figure is exact and already reconciles
 * (`net_movement = inflow - outflow + transfer_in - transfer_out`); the
 * frontend only lays it out.
 */
export interface CashflowRow {
  key: string;
  startDate: string;
  endDate: string;
  /** Null for a bucket with no movement at all. */
  commodityId: number | null;
  quantityScale: number;
  inflow: string;
  outflow: string;
  operatingNet: string;
  transferIn: string;
  transferOut: string;
  netMovement: string;
}

export function cashflowRows(report: CashflowResponse): CashflowRow[] {
  return report.buckets.flatMap((bucket): CashflowRow[] => {
    if (bucket.totals.length === 0) {
      return [
        {
          key: `${bucket.start_date}:${bucket.end_date}:empty`,
          startDate: bucket.start_date,
          endDate: bucket.end_date,
          commodityId: null,
          quantityScale: 0,
          inflow: '0',
          outflow: '0',
          operatingNet: '0',
          transferIn: '0',
          transferOut: '0',
          netMovement: '0'
        }
      ];
    }
    return bucket.totals.map((total) => ({
      key: `${bucket.start_date}:${bucket.end_date}:${total.commodity_id}`,
      startDate: bucket.start_date,
      endDate: bucket.end_date,
      commodityId: total.commodity_id,
      quantityScale: total.quantity_scale,
      inflow: total.inflow,
      outflow: total.outflow,
      operatingNet: total.operating_net,
      transferIn: total.transfer_in,
      transferOut: total.transfer_out,
      netMovement: total.net_movement
    }));
  });
}

/** True once the report covers more than one commodity. */
export function cashflowIsMultiCommodity(report: CashflowResponse): boolean {
  const commodities = new Set<number>();
  for (const bucket of report.buckets) {
    for (const total of bucket.totals) {
      commodities.add(total.commodity_id);
    }
  }
  return commodities.size > 1;
}

/** True when the report found no movement in any bucket. */
export function cashflowIsEmpty(report: CashflowResponse): boolean {
  return report.buckets.every((bucket) => bucket.totals.length === 0);
}

/**
 * Whether a signed figure should read as negative.
 *
 * Sign is taken from the coefficient string rather than by converting to a
 * `Number`, which would be lossless only up to `Number.MAX_SAFE_INTEGER`.
 */
export function isNegativeAmount(value: string): boolean {
  return value.startsWith('-');
}
