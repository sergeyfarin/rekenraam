import type { components } from '#lib/api/schema.js';

type AccountRegisterResponse = components['schemas']['AccountRegisterResponse'];
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

/** Link to reconcile the account; voiding a statement happens there (#595). */
export function reconcileHref(accountID: number): string {
  return `/app/ledger/reconcile?account=${accountID}`;
}
