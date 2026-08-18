/**
 * Report filter state, read from and written to the URL.
 *
 * The URL is the source of truth for every report control (`reports-plan.md`
 * "Frontend shape"), so a link carries the whole query. Repeated parameters are
 * an OR-set within one dimension and combine with AND across dimensions, which
 * matches the API contract exactly — no translation layer in between.
 */

export type ReportFilterState = {
  accountIDs: number[];
  includeDescendants: boolean;
  commodityIDs: number[];
  categoryIDs: number[];
  payeeIDs: number[];
};

/** One selectable value in a filter control, already resolved to display text. */
export type FilterOption = { id: number; label: string };

/** The dimensions a report can filter on. Spending adds category and payee. */
export type ReportFilterDimension = 'account' | 'commodity' | 'category' | 'payee';

const ID_PARAMS: Record<Exclude<ReportFilterDimension, never>, string> = {
  account: 'account_id',
  commodity: 'commodity_id',
  category: 'category_id',
  payee: 'payee_id'
};

export const emptyReportFilters: ReportFilterState = {
  accountIDs: [],
  includeDescendants: false,
  commodityIDs: [],
  categoryIDs: [],
  payeeIDs: []
};

/**
 * Reads one repeated ID parameter.
 *
 * A hand-edited or stale link is not an error state worth a screen: unparseable
 * and duplicate entries are dropped so the report still renders, and the result
 * is sorted so two links selecting the same set produce the same query key.
 */
export function parseIDParam(params: URLSearchParams, name: string): number[] {
  const ids = new Set<number>();
  for (const raw of params.getAll(name)) {
    const value = Number(raw);
    if (Number.isInteger(value) && value > 0) {
      ids.add(value);
    }
  }
  return [...ids].sort((a, b) => a - b);
}

export function parseReportFilters(params: URLSearchParams): ReportFilterState {
  return {
    accountIDs: parseIDParam(params, ID_PARAMS.account),
    includeDescendants: params.get('include_descendants') === 'true',
    commodityIDs: parseIDParam(params, ID_PARAMS.commodity),
    categoryIDs: parseIDParam(params, ID_PARAMS.category),
    payeeIDs: parseIDParam(params, ID_PARAMS.payee)
  };
}

/**
 * Writes the filter state over a copy of the current parameters, preserving
 * everything the filters do not own (view, dates, bucket, group_by, mode).
 *
 * An empty dimension is deleted rather than written empty, so the default
 * report has a clean, short URL. `include_descendants` is only meaningful
 * alongside an account selection, and is dropped without one.
 */
export function writeReportFilters(params: URLSearchParams, filters: ReportFilterState): URLSearchParams {
  const next = new URLSearchParams(params);

  const setIDs = (name: string, ids: number[]) => {
    next.delete(name);
    for (const id of ids) {
      next.append(name, String(id));
    }
  };

  setIDs(ID_PARAMS.account, filters.accountIDs);
  setIDs(ID_PARAMS.commodity, filters.commodityIDs);
  setIDs(ID_PARAMS.category, filters.categoryIDs);
  setIDs(ID_PARAMS.payee, filters.payeeIDs);

  next.delete('include_descendants');
  if (filters.accountIDs.length > 0 && filters.includeDescendants) {
    next.set('include_descendants', 'true');
  }

  return next;
}

/** Toggles one ID within a dimension, keeping the sorted, de-duplicated shape. */
export function toggleID(ids: number[], id: number): number[] {
  return ids.includes(id) ? ids.filter((value) => value !== id) : [...ids, id].sort((a, b) => a - b);
}

/**
 * How many dimensions are narrowing the report.
 *
 * Counted per dimension rather than per ID: the user needs to know *what* is
 * constrained, and a badge reading "12" for one account tree would overstate it.
 * `include_descendants` widens an existing filter, so it never counts.
 */
export function activeFilterCount(filters: ReportFilterState, dimensions: ReportFilterDimension[]): number {
  return dimensions.filter((dimension) => selectedIDs(filters, dimension).length > 0).length;
}

export function selectedIDs(filters: ReportFilterState, dimension: ReportFilterDimension): number[] {
  switch (dimension) {
    case 'account':
      return filters.accountIDs;
    case 'commodity':
      return filters.commodityIDs;
    case 'category':
      return filters.categoryIDs;
    case 'payee':
      return filters.payeeIDs;
  }
}

export function withSelectedIDs(
  filters: ReportFilterState,
  dimension: ReportFilterDimension,
  ids: number[]
): ReportFilterState {
  switch (dimension) {
    case 'account':
      return { ...filters, accountIDs: ids };
    case 'commodity':
      return { ...filters, commodityIDs: ids };
    case 'category':
      return { ...filters, categoryIDs: ids };
    case 'payee':
      return { ...filters, payeeIDs: ids };
  }
}

/**
 * Clears the dimensions a view owns, leaving the others untouched.
 *
 * Net worth cannot express a category or payee filter, so switching to it must
 * not silently discard a spending selection the user can switch back to.
 */
export function clearDimensions(
  filters: ReportFilterState,
  dimensions: ReportFilterDimension[]
): ReportFilterState {
  return dimensions.reduce((next, dimension) => withSelectedIDs(next, dimension, []), {
    ...filters,
    includeDescendants: dimensions.includes('account') ? false : filters.includeDescendants
  });
}
