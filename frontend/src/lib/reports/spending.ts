import type { SpendingReportResponse, SpendingReportGroup } from '$lib/api/reports';

/**
 * One table row: a single group's total in a single commodity. The backend
 * returns a group per dimension value with one total per commodity, so a
 * multi-commodity report yields more rows than groups. Unlike commodities are
 * never combined — see reports-plan.md.
 */
export type SpendingRow = {
  key: string;
  categoryID?: number;
  payeeID?: number;
  code: string;
  name: string;
  commodityID: number;
  quantityValue: string;
  quantityScale: number;
  shareBasisPoints?: number;
  /** True for the group holding postings with no payee record. */
  unattributed: boolean;
};

export function spendingRows(report: SpendingReportResponse): SpendingRow[] {
  return report.groups.flatMap((group, index) =>
    group.totals.map((total) => ({
      key: `${groupKey(group, index)}-${total.commodity_id}`,
      categoryID: group.category_id,
      payeeID: group.payee_id,
      code: group.code ?? '',
      name: group.name ?? '',
      commodityID: total.commodity_id,
      quantityValue: total.quantity_value,
      quantityScale: total.quantity_scale,
      shareBasisPoints: total.share_basis_points,
      unattributed: group.category_id === undefined && group.payee_id === undefined
    }))
  );
}

function groupKey(group: SpendingReportGroup, index: number): string {
  if (group.category_id !== undefined) return `category-${group.category_id}`;
  if (group.payee_id !== undefined) return `payee-${group.payee_id}`;
  return `unattributed-${index}`;
}

/**
 * Commodity IDs present in the report, in the order the backend returned its
 * totals. Used to decide whether a chart summary is honest: a bar chart of
 * magnitudes across unlike commodities would be a fabricated comparison.
 */
export function reportCommodityIDs(report: SpendingReportResponse): number[] {
  return report.commodity_totals.map((total) => total.commodity_id);
}

export function isSingleCommodity(report: SpendingReportResponse): boolean {
  return reportCommodityIDs(report).length === 1;
}

/**
 * Formats an integer basis-point share as a locale-aware percentage. The share
 * is computed exactly on the server; this only presents it, so no money
 * arithmetic happens in the browser.
 */
export function formatShare(shareBasisPoints: number | undefined, locale: string): string {
  if (shareBasisPoints === undefined) return '';
  return new Intl.NumberFormat(locale, {
    style: 'percent',
    minimumFractionDigits: 1,
    maximumFractionDigits: 1
  }).format(shareBasisPoints / 10_000);
}

export type SpendingBar = {
  key: string;
  label: string;
  /** Fraction of the widest bar, 0..1. Presentation geometry only. */
  ratio: number;
  quantityValue: string;
  quantityScale: number;
  shareBasisPoints?: number;
  negative: boolean;
};

/**
 * Builds bar geometry for the chart summary of one commodity's rows.
 *
 * Bars are scaled against the largest magnitude in the set so the longest bar
 * fills the track. Rows that net negative (a refund-dominated category) are
 * marked so the chart can distinguish them by more than colour. Zero-magnitude
 * rows are kept with a zero ratio rather than dropped — the chart must not
 * disagree with the table about which rows exist.
 */
export function spendingBars(rows: SpendingRow[], labelFor: (row: SpendingRow) => string): SpendingBar[] {
  const magnitudes = rows.map((row) => magnitudeOf(row));
  const widest = magnitudes.reduce((max, value) => (value > max ? value : max), 0);

  return rows.map((row, index) => ({
    key: row.key,
    label: labelFor(row),
    ratio: widest === 0 ? 0 : magnitudes[index] / widest,
    quantityValue: row.quantityValue,
    quantityScale: row.quantityScale,
    shareBasisPoints: row.shareBasisPoints,
    negative: row.quantityValue.startsWith('-')
  }));
}

/**
 * Magnitude of a row as a Number, for bar geometry only.
 *
 * Never use this for money: it is a lossy float used to size a rectangle. All
 * displayed amounts come from the exact coefficient strings.
 */
function magnitudeOf(row: SpendingRow): number {
  const value = Number(row.quantityValue);
  if (!Number.isFinite(value)) return 0;
  return Math.abs(value) / 10 ** row.quantityScale;
}
