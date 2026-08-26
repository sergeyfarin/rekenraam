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
  /**
   * The deepest scale any of this row's measures carries, for anything that
   * needs one number for the row. Never use it to render a measure: read that
   * measure's own entry in `scales`.
   */
  quantityScale: number;
  /**
   * Each measure's own scale.
   *
   * The server writes every measure of one commodity at one scale, but a
   * coefficient must be paired with the scale it was produced at rather than
   * with a scale borrowed from a sibling measure: the two differ by a power of
   * ten per digit, so borrowing renders 100 as 1.00.
   */
  scales: Record<Measure, number>;
  inflow: string;
  outflow: string;
  operatingNet: string;
  transferIn: string;
  transferOut: string;
  transferNet: string;
  netMovement: string;
  /**
   * True for the reporting-currency restatement of a bucket. Its three net
   * measures are derived from its own converted parts, so the identities hold
   * within the row exactly as they do in a commodity row.
   */
  converted: boolean;
};

export type Measure =
  | 'inflow'
  | 'outflow'
  | 'operatingNet'
  | 'transferIn'
  | 'transferOut'
  | 'transferNet'
  | 'netMovement';

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

    const rows = [...commodityIDs]
      .sort((a, b) => a - b)
      .map((commodityID) => {
        const row: CashflowRow = {
          key: `${bucket.start_date}-${bucket.end_date}-${commodityID}`,
          startDate: bucket.start_date,
          endDate: bucket.end_date,
          commodityID,
          quantityScale: 0,
          scales: {
            inflow: 0,
            outflow: 0,
            operatingNet: 0,
            transferIn: 0,
            transferOut: 0,
            transferNet: 0,
            netMovement: 0
          },
          inflow: '0',
          outflow: '0',
          operatingNet: '0',
          transferIn: '0',
          transferOut: '0',
          transferNet: '0',
          netMovement: '0',
          converted: false
        };

        for (const [measure] of MEASURES) {
          const quantity = measures.get(measure)?.get(commodityID);
          if (!quantity) continue;
          row[measure] = quantity.quantity_value;
          row.scales[measure] = quantity.quantity_scale;
          row.quantityScale = Math.max(row.quantityScale, quantity.quantity_scale);
        }
        // An absent measure reads as zero, and zero is the same number at every
        // scale, so it takes the row's scale rather than a misleading 0.
        for (const [measure] of MEASURES) {
          if (!measures.get(measure)?.get(commodityID)) {
            row.scales[measure] = row.quantityScale;
          }
        }
        return row;
      });

    // A bucket with no commodity rows has nothing to restate. The backend
    // converts an empty bucket to an exact zero, which is true but is not
    // activity — emitting it would replace the "nothing moved" empty state with
    // a table of zeros.
    const restatement = rows.length === 0 ? null : convertedRow(bucket);
    // After the commodity rows: the restatement summarizes them, and reading it
    // first would suggest they break it down.
    return restatement ? [...rows, restatement] : rows;
  });
}

/**
 * A bucket's reporting-currency restatement, or null when the bucket did not
 * convert.
 *
 * Every measure has to be present for a row to be built. A partial row would
 * put a real inflow beside a zero outflow that only means "unknown", and the
 * net movement beside them would be neither.
 */
function convertedRow(bucket: CashflowReportResponse['buckets'][number]): CashflowRow | null {
  const parts: Array<[Measure, BalanceQuantity | undefined]> = [
    ['inflow', bucket.converted_inflow],
    ['outflow', bucket.converted_outflow],
    ['transferIn', bucket.converted_transfer_in],
    ['transferOut', bucket.converted_transfer_out],
    ['operatingNet', bucket.converted_operating_net],
    ['transferNet', bucket.converted_transfer_net],
    ['netMovement', bucket.converted_net_movement]
  ];
  if (parts.some(([, quantity]) => quantity === undefined)) return null;

  // Every converted figure is rounded once into the reporting currency's own
  // scale, so one scale covers the whole row.
  const scale = bucket.converted_net_movement!.quantity_scale;
  const value = (measure: Measure) =>
    parts.find(([name]) => name === measure)![1]!.quantity_value;

  return {
    key: `${bucket.start_date}-${bucket.end_date}-converted`,
    startDate: bucket.start_date,
    endDate: bucket.end_date,
    commodityID: bucket.converted_net_movement!.commodity_id,
    quantityScale: scale,
    scales: {
      inflow: scale,
      outflow: scale,
      operatingNet: scale,
      transferIn: scale,
      transferOut: scale,
      transferNet: scale,
      netMovement: scale
    },
    inflow: value('inflow'),
    outflow: value('outflow'),
    operatingNet: value('operatingNet'),
    transferIn: value('transferIn'),
    transferOut: value('transferOut'),
    transferNet: value('transferNet'),
    netMovement: value('netMovement'),
    converted: true
  };
}

export function heldRows(rows: CashflowRow[]): CashflowRow[] {
  return rows.filter((row) => !row.converted);
}

export function hasMultipleCommodities(rows: CashflowRow[]): boolean {
  return new Set(heldRows(rows).map((row) => row.commodityID)).size > 1;
}

/**
 * The converted restatements, but only when every bucket has one: a series
 * with a hole would chart a month of no movement that in fact did not convert.
 */
export function convertedSeries(rows: CashflowRow[]): CashflowRow[] {
  const buckets = new Set(rows.map((row) => `${row.startDate}/${row.endDate}`));
  const converted = rows.filter((row) => row.converted);
  return converted.length > 0 && converted.length === buckets.size ? converted : [];
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
  // A restatement has no postings of its own. Its commodity is the reporting
  // currency, so the link would narrow the breakdown to whatever happens to be
  // held in that currency — a different set from the one the row sums. The
  // commodity rows it summarizes each keep their own link.
  if (row.converted) return undefined;

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
