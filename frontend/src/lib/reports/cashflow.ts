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

/**
 * The spending-report link that breaks down one bucket's inflow or outflow.
 *
 * Cashflow deliberately takes no category or payee filter: such a filter removes
 * counterpart postings from the basis, and `net_movement` would stop reconciling
 * to the cash balance change. The breakdown belongs to the spending report,
 * whose account filter is a counterpart filter over the same accounts — so the
 * two ask the same question, and this link is how you get from one to the other.
 *
 * The scope is passed as the *resolved* account ids the cashflow report actually
 * measured, so the drill-down inherits the exact set rather than re-deriving a
 * default that may have drifted since.
 */
export function cashflowDrillDownHref(
  row: CashflowRow,
  direction: 'in' | 'out',
  resolvedAccountIDs: number[],
  bucket: string
): string | undefined {
  const amount = direction === 'in' ? row.inflow : row.outflow;
  // Nothing moved in that direction, so there is nothing to break down.
  if (amount === '0') return undefined;

  const params = new URLSearchParams();
  params.set('view', 'spending');
  params.set('group_by', 'category');
  // An inflow is money from income counterparts, an outflow money to expense
  // counterparts — exactly the spending report's direction switch.
  params.set('mode', direction === 'in' ? 'income' : 'spending');
  params.set('start_date', row.startDate);
  params.set('end_date', row.endDate);
  // Spending ranks one range and ignores the bucket, but the reports screen
  // requires a valid one in the URL; carrying cashflow's own keeps a later
  // switch to net worth showing the same granularity.
  params.set('bucket', bucket);

  for (const accountID of resolvedAccountIDs) {
    params.append('account_id', String(accountID));
  }
  // The row's own commodity only. A cashflow row is per commodity, and widening
  // the breakdown to the others would answer a question the row did not ask.
  params.append('commodity_id', String(row.commodityID));

  return `/app/reports?${params.toString()}`;
}
