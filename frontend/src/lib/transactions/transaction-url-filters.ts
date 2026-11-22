/**
 * Transaction list filters carried in the URL.
 *
 * Parameter names mirror the API exactly, so a drill-down link built from a
 * report's `drill_down` query needs no translation layer and a shared link
 * reproduces the same list.
 */
import type { ReadonlyURLSearchParams } from '$app/state';
import type { TransactionListOptions } from '#lib/api/transactions.ts';
import { isCalendarDate } from '#lib/dates/calendar-date.ts';

export type TransactionQueryFilters = Pick<
  TransactionListOptions,
  | 'status'
  | 'kind'
  | 'accountID'
  | 'categoryID'
  | 'payeeID'
  | 'tagID'
  | 'q'
  | 'needsReview'
  | 'afterDate'
  | 'beforeDate'
  | 'categoryType'
  | 'dateBasis'
>;

export function payeeTransactionsHref(payeeID: number): string {
  return `/app/ledger/transactions?payee_id=${payeeID}`;
}

export function tagTransactionsHref(tagID: number): string {
  return `/app/ledger/transactions?tag_id=${tagID}`;
}

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
  return raw !== null && isCalendarDate(raw) ? raw : undefined;
}

function oneOf<T extends readonly string[]>(raw: string | null, allowed: T): T[number] | undefined {
  return raw !== null && (allowed as readonly string[]).includes(raw) ? (raw as T[number]) : undefined;
}

/** Reads individually valid filter values while preserving a date range being edited. */
export function parseTransactionFilters(params: ReadonlyURLSearchParams): TransactionQueryFilters {
  return {
    status: oneOf(params.get('status'), STATUSES),
    kind: oneOf(params.get('kind'), KINDS),
    accountID: positiveInt(params.get('account_id')),
    categoryID: positiveInt(params.get('category_id')),
    payeeID: positiveInt(params.get('payee_id')),
    tagID: positiveInt(params.get('tag_id')),
    q: params.get('q') || undefined,
    needsReview: params.get('needs_review') === 'true' || undefined,
    afterDate: isoDate(params.get('after_date')),
    beforeDate: isoDate(params.get('before_date')),
    categoryType: oneOf(params.get('category_type'), CATEGORY_TYPES),
    dateBasis: oneOf(params.get('date_basis'), DATE_BASES)
  };
}

export function hasInvertedTransactionRange({ afterDate, beforeDate }: TransactionQueryFilters): boolean {
  return !!afterDate && !!beforeDate && afterDate > beforeDate;
}

/** Temporarily omit an inverted range from the request without changing the bar or URL. */
export function transactionRequestFilters(filters: TransactionQueryFilters): TransactionQueryFilters {
  return hasInvertedTransactionRange(filters) ? { ...filters, afterDate: undefined, beforeDate: undefined } : filters;
}

/** Writes the filters over a copy of the current parameters, clearing cleared ones. */
export function writeTransactionFilters(
  params: ReadonlyURLSearchParams,
  filters: TransactionQueryFilters
): URLSearchParams {
  const next = new URLSearchParams(params.toString());

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
  set('tag_id', filters.tagID);
  set('q', filters.q);
  set('needs_review', filters.needsReview ? 'true' : undefined);
  set('after_date', filters.afterDate);
  set('before_date', filters.beforeDate);
  set('category_type', filters.categoryType);
  set('date_basis', filters.dateBasis);

  return next;
}
