import type { components } from '#lib/api/schema.js';
import { formatQuantity } from '#lib/money/format.ts';

type AccountRegisterResponse = components['schemas']['AccountRegisterResponse'];
type BalanceQuantity = components['schemas']['BalanceQuantity'];
export type AccountRegisterStatementResponse = components['schemas']['AccountRegisterStatementResponse'];

/**
 * A statement row placed among the register's posting rows (#595). The
 * server places it: `beforeIndex` is the index in the flattened entries list
 * before which the row sits, equal to the number of entries when it follows
 * them all.
 */
export interface RegisterStatementMarker {
  key: string;
  beforeIndex: number;
  statement: AccountRegisterStatementResponse;
}

/**
 * Flattens the register's statement rows across its loaded pages. Each page's
 * `before_entry_index` counts that page's entries only, so it is offset by the
 * entries of the pages before it. A statement that ends a page sits before the
 * next page's first entry, which the offset already expresses.
 */
export function registerStatementMarkers(pages: readonly Pick<AccountRegisterResponse, 'entries' | 'statements'>[]): RegisterStatementMarker[] {
  const markers: RegisterStatementMarker[] = [];
  let offset = 0;
  for (const page of pages) {
    const entryCount = page.entries?.length ?? 0;
    for (const statement of page.statements ?? []) {
      const index = Math.min(Math.max(statement.before_entry_index, 0), entryCount);
      markers.push({ key: `statement-${statement.checkpoint_id}`, beforeIndex: offset + index, statement });
    }
    offset += entryCount;
  }
  return markers;
}

/** Groups markers by the row index they precede, keeping their order. */
export function markersByRowIndex<M extends { beforeIndex: number }>(markers: readonly M[], rowCount: number): Map<number, M[]> {
  const grouped = new Map<number, M[]>();
  for (const marker of markers) {
    const index = Math.min(Math.max(marker.beforeIndex, 0), rowCount);
    const list = grouped.get(index);
    if (list) list.push(marker);
    else grouped.set(index, [marker]);
  }
  return grouped;
}

/**
 * A register balance (a running balance or a statement row's balance) as the
 * account reads it. The server class-normalizes `normal_quantity_value` like
 * every other balance read, so a card owing 500 reads 500 and only a balance
 * against the account's normal side is negative (#598). Both rows go through
 * here so they never show different conventions side by side.
 */
export function registerBalanceView(balance: BalanceQuantity, locale: string): { text: string; negative: boolean } {
  return {
    text: formatQuantity(balance.normal_quantity_value, balance.quantity_scale, locale),
    negative: balance.normal_quantity_value.startsWith('-')
  };
}
