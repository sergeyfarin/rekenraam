import type { Snippet } from 'svelte';
import { metaHelper, tableFeatures } from '@tanstack/svelte-table';

/**
 * Priority controls responsive column hiding:
 * 1 = always shown
 * 2 = hidden below ~600px
 * 3 = hidden below ~900px
 */
export type ColumnPriority = 1 | 2 | 3;

export type ColumnAlign = 'left' | 'right';

/**
 * Presentation metadata carried on each TanStack column definition.
 * Responsive hiding stays CSS-driven (see PRIORITY_CLASSES in
 * transaction-table.svelte) so prerendered markup matches first paint —
 * columnVisibilityFeature would need `matchMedia`, which the static adapter
 * has no access to at build time.
 */
export type TransactionColumnMeta = {
  width?: string;
  align?: ColumnAlign;
  priority?: ColumnPriority;
};

/**
 * Feature set for every transaction-style table. Core only: sorting,
 * filtering and pagination are all server-side (cursor-paginated `/api/v1`
 * endpoints), so registering those features would only add dead state.
 * Register a feature here — and nowhere else — when a table needs it.
 */
export const transactionTableFeatures = tableFeatures({
  columnMeta: metaHelper<TransactionColumnMeta>()
});

export type TransactionTableFeatures = typeof transactionTableFeatures;

/**
 * Call-site column shape. Kept deliberately narrow: it is mapped onto
 * TanStack display column definitions inside transaction-table.svelte, with
 * `cell` rendered through the adapter's snippet renderer.
 */
export type Column<R> = {
  key: string;
  header: string;
  width?: string;
  align?: ColumnAlign;
  priority?: ColumnPriority;
  cell: Snippet<[R]>;
};
