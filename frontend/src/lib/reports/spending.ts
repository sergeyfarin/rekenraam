import type { SpendingGroup, SpendingResponse } from '$lib/api/ledger';

/**
 * Flattens the spending report into table rows.
 *
 * The backend groups per dimension and then per commodity, because a ranking
 * must never compare unlike commodities as one number. A table needs one row
 * per (group, commodity) pair, so the nesting is unwound here — the ranking
 * order the backend chose is preserved, and a group's commodities stay
 * adjacent.
 *
 * No arithmetic happens in this module. Every figure is already exact and
 * final; the frontend's job is layout and formatting only.
 */
export interface SpendingRow {
  /** Stable key for `{#each}` — unique per (group, commodity). */
  key: string;
  group: SpendingGroup;
  commodityId: number;
  quantityValue: string;
  quantityScale: number;
  /** Positive magnitude in the report's direction. */
  normalQuantityValue: string;
  shareBasisPoints: number;
  /** True when this row's commodity is the one the ranking was computed in. */
  isRankCommodity: boolean;
}

function groupKey(group: SpendingGroup): string {
  if (group.category_account_id !== undefined) return `category:${group.category_account_id}`;
  if (group.payee_id !== undefined) return `payee:${group.payee_id}`;
  return 'unassigned';
}

export function spendingRows(report: SpendingResponse): SpendingRow[] {
  return report.groups.flatMap((group) =>
    group.totals.map((total) => ({
      key: `${groupKey(group)}:${total.commodity_id}`,
      group,
      commodityId: total.commodity_id,
      quantityValue: total.quantity_value,
      quantityScale: total.quantity_scale,
      normalQuantityValue: total.normal_quantity_value,
      shareBasisPoints: total.share_basis_points,
      isRankCommodity: total.commodity_id === report.rank_commodity_id
    }))
  );
}

/**
 * Whether the report spans more than one commodity.
 *
 * Drives two things: the "totals are shown separately" notice, and whether a
 * summary chart is offered at all — a bar chart across unlike commodities
 * would draw exactly the comparison the report refuses to make numerically.
 */
export function spendingIsMultiCommodity(report: SpendingResponse): boolean {
  return report.commodity_totals.length > 1;
}

/**
 * Formats a share for display as a percentage with one decimal place.
 *
 * Basis points are integers (10000 = 100%), so this is exact integer-to-string
 * work rather than division: 7525 → "75.3". Rounding to one decimal is a
 * presentation choice; the exact figure a user acts on is always the amount
 * beside it, never the share.
 */
export function formatShare(shareBasisPoints: number, locale: string): string {
  // Round to tenths of a percent, half away from zero, entirely in integers.
  const sign = shareBasisPoints < 0 ? -1 : 1;
  const magnitude = Math.abs(shareBasisPoints);
  const tenths = Math.floor((magnitude + 5) / 10) * sign;

  const whole = Math.trunc(tenths / 10);
  const fraction = Math.abs(tenths % 10);
  const formatted = new Intl.NumberFormat(locale, {
    minimumFractionDigits: 1,
    maximumFractionDigits: 1
  }).format(Number(`${whole}.${fraction}`));
  // A negative share rounding to less than 0.05% loses its sign through the
  // integer path above, so reapply it rather than showing a bare "0.0".
  return sign < 0 && tenths === 0 ? `-${formatted}` : formatted;
}
