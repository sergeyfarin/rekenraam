import type { BalanceQuantity } from '$lib/api/ledger';
import type { CashflowReportResponse } from '$lib/api/reports';

/**
 * One table row: a single calendar bucket in a single commodity.
 *
 * The response carries seven independent per-commodity lists per bucket. A
 * commodity may appear in some and not others — a month with only a card
 * payment has a transfer and a net movement but no inflow — so the rows are
 * built from the union of commodities present, with an absent measure read as
 * zero rather than as a missing row.
 */
export type CashflowRow = {
  key: string;
  startDate: string;
  endDate: string;
  commodityID: number;
  quantityScale: number;
  inflow: string;
  outflow: string;
  operatingNet: string;
  transferIn: string;
  transferOut: string;
  transferNet: string;
  netMovement: string;
};

type Measure = keyof Pick<
  CashflowRow,
  'inflow' | 'outflow' | 'operatingNet' | 'transferIn' | 'transferOut' | 'transferNet' | 'netMovement'
>;

const MEASURES: Array<[Measure, keyof CashflowReportResponse['buckets'][number]]> = [
  ['inflow', 'inflow'],
  ['outflow', 'outflow'],
  ['operatingNet', 'operating_net'],
  ['transferIn', 'transfer_in'],
  ['transferOut', 'transfer_out'],
  ['transferNet', 'transfer_net'],
  ['netMovement', 'net_movement']
];

function byCommodity(quantities: BalanceQuantity[] | undefined): Map<number, BalanceQuantity> {
  const map = new Map<number, BalanceQuantity>();
  for (const quantity of quantities ?? []) {
    map.set(quantity.commodity_id, quantity);
  }
  return map;
}

export function cashflowRows(report: CashflowReportResponse): CashflowRow[] {
  return report.buckets.flatMap((bucket) => {
    const measures = new Map<Measure, Map<number, BalanceQuantity>>();
    for (const [measure, field] of MEASURES) {
      measures.set(measure, byCommodity(bucket[field] as BalanceQuantity[]));
    }

    const commodityIDs = new Set<number>();
    for (const quantities of measures.values()) {
      for (const commodityID of quantities.keys()) {
        commodityIDs.add(commodityID);
      }
    }

    return [...commodityIDs]
      .sort((a, b) => a - b)
      .map((commodityID) => {
        const row: CashflowRow = {
          key: `${bucket.start_date}-${bucket.end_date}-${commodityID}`,
          startDate: bucket.start_date,
          endDate: bucket.end_date,
          commodityID,
          // Every measure of one commodity is produced at the same scale by the
          // server, so the first one present names the scale for the row.
          quantityScale: 0,
          inflow: '0',
          outflow: '0',
          operatingNet: '0',
          transferIn: '0',
          transferOut: '0',
          transferNet: '0',
          netMovement: '0'
        };

        let scale: number | undefined;
        for (const [measure] of MEASURES) {
          const quantity = measures.get(measure)?.get(commodityID);
          if (!quantity) continue;
          row[measure] = quantity.quantity_value;
          scale ??= quantity.quantity_scale;
        }
        row.quantityScale = scale ?? 0;
        return row;
      });
  });
}

export function hasMultipleCommodities(rows: CashflowRow[]): boolean {
  return new Set(rows.map((row) => row.commodityID)).size > 1;
}

/**
 * True when nothing moved anywhere in the range.
 *
 * A bucket whose movements all net to zero is still real activity, so the empty
 * state keys on there being no rows at all rather than on the totals reading
 * zero — telling a user "no activity" when their transfers cancelled out would
 * be wrong.
 */
export function isEmptyCashflow(rows: CashflowRow[]): boolean {
  return rows.length === 0;
}

/**
 * Which sides of the financing movement are worth naming under the net figure.
 *
 * A row with money moving one way only should not read "in 0.00 · out 750.00":
 * the zero half is noise, and in a dense table noise is what stops people
 * reading the column at all.
 */
export function transferDetail(row: CashflowRow): 'none' | 'in' | 'out' | 'both' {
  const incoming = row.transferIn !== '0';
  const outgoing = row.transferOut !== '0';
  if (incoming && outgoing) return 'both';
  if (incoming) return 'in';
  if (outgoing) return 'out';
  return 'none';
}
