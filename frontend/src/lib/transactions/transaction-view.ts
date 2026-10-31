import type { ReadonlyURLSearchParams } from '$app/state';

/**
 * The three views of the Transactions page (#446): the ordinary list (posted
 * and voided), drafts produced by automation, and soft-deleted transactions.
 * A view is part of the page URL so copied links and Back work.
 */
export type TransactionView = 'all' | 'drafts' | 'deleted';

const VIEWS: readonly TransactionView[] = ['all', 'drafts', 'deleted'];

export function parseTransactionView(params: ReadonlyURLSearchParams): TransactionView {
  const view = params.get('view');
  return view !== null && (VIEWS as readonly string[]).includes(view) ? (view as TransactionView) : 'all';
}

/**
 * A view link keeps the filters that still apply. All and Drafts share the
 * ordinary list's filters except status, which the view itself owns; Deleted
 * reads a different endpoint and keeps none of them.
 */
export function transactionViewHref(view: TransactionView, params: ReadonlyURLSearchParams): string {
  if (view === 'deleted') {
    return '/app/ledger/transactions?view=deleted';
  }

  const next = new URLSearchParams(params.toString());
  next.delete('view');
  next.delete('status');
  if (view === 'drafts') {
    next.set('view', 'drafts');
  }
  const search = next.toString();
  return `/app/ledger/transactions${search ? `?${search}` : ''}`;
}
