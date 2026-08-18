/**
 * Transaction list filters carried in the URL.
 *
 * Parameter names mirror the API exactly, so a drill-down link built from a
 * report's `drill_down` query needs no translation layer and a shared link
 * reproduces the same list.
 */

import type { TransactionListOptions } from '$lib/api/transactions';

export type TransactionQueryFilters = Pick<
  TransactionListOptions,
  | 'status'
  | 'kind'
  | 'accountID'
  | 'categoryID'
  | 'payeeID'
  | 'q'
  | 'needsReview'
  | 'afterDate'
  | 'beforeDate'
  | 'categoryType'
  | 'dateBasis'
>;

const DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/;

const STATUSES = ['draft', 'posted', 'voided'] as const;
const KINDS = ['ordinary', 'transfer', 'investment', 'opening_balance', 'adjustment'] as const;
const CATEGORY_TYPES = ['income', 'expense'] as const;
const DATE_BASES = ['transaction', 'entry'] as const;

function positiveInt(raw: string | null): number | undefined {
  if (raw === null) return undefined;
  const value = Number(raw);
  return Number.isInteger(value) && value > 0 ? value : undefined;
}

function isoDate(raw: string | null): string | undefined {
  return raw !== null && DATE_PATTERN.test(raw) ? raw : undefined;
}

function oneOf<T extends readonly string[]>(raw: string | null, allowed: T): T[number] | undefined {
  return raw !== null && (allowed as readonly string[]).includes(raw) ? (raw as T[number]) : undefined;
}

/**
 * Reads the filters from a URL.
 *
 * A value the API would reject is dropped rather than passed through: a
 * hand-edited link should show an unfiltered list, not a validation error where
 * a list belongs.
 */
export function parseTransactionFilters(params: URLSearchParams): TransactionQueryFilters {
  const filters: TransactionQueryFilters = {
    status: oneOf(params.get('status'), STATUSES),
    kind: oneOf(params.get('kind'), KINDS),
    accountID: positiveInt(params.get('account_id')),
    categoryID: positiveInt(params.get('category_id')),
    payeeID: positiveInt(params.get('payee_id')),
    q: params.get('q') || undefined,
    needsReview: params.get('needs_review') === 'true' || undefined,
    afterDate: isoDate(params.get('after_date')),
    beforeDate: isoDate(params.get('before_date')),
    categoryType: oneOf(params.get('category_type'), CATEGORY_TYPES),
    dateBasis: oneOf(params.get('date_basis'), DATE_BASES)
  };

  // An inverted range is a validation error at the API; drop both ends so the
  // screen shows a list rather than an error banner it cannot explain.
  if (filters.afterDate && filters.beforeDate && filters.afterDate > filters.beforeDate) {
    filters.afterDate = undefined;
    filters.beforeDate = undefined;
  }

  return filters;
}

/** Writes the filters over a copy of the current parameters, clearing cleared ones. */
export function writeTransactionFilters(
  params: URLSearchParams,
  filters: TransactionQueryFilters
): URLSearchParams {
  const next = new URLSearchParams(params);

  const set = (name: string, value: string | number | undefined) => {
    if (value === undefined || value === '') {
      next.delete(name);
    } else {
      next.set(name, String(value));
    }
  };

  set('status', filters.status);
  set('kind', filters.kind);
  set('account_id', filters.accountID);
  set('category_id', filters.categoryID);
  set('payee_id', filters.payeeID);
  set('q', filters.q);
  set('needs_review', filters.needsReview ? 'true' : undefined);
  set('after_date', filters.afterDate);
  set('before_date', filters.beforeDate);
  set('category_type', filters.categoryType);
  set('date_basis', filters.dateBasis);

  return next;
}
