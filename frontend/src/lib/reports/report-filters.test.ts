import { describe, expect, it } from 'vitest';
import {
  activeFilterCount,
  clearDimensions,
  emptyReportFilters,
  parseIDParam,
  parseReportFilters,
  toggleID,
  withSelectedIDs,
  writeReportFilters,
  type ReportFilterState
} from './report-filters';

const params = (query: string) => new URLSearchParams(query);

describe('parseIDParam', () => {
  it('reads a repeated parameter as an OR-set', () => {
    expect(parseIDParam(params('account_id=12&account_id=14'), 'account_id')).toEqual([12, 14]);
  });

  it('drops unparseable, zero, and negative entries instead of failing the report', () => {
    expect(parseIDParam(params('account_id=12&account_id=x&account_id=0&account_id=-3'), 'account_id')).toEqual([12]);
  });

  it('de-duplicates and sorts so equivalent links share one query key', () => {
    expect(parseIDParam(params('account_id=14&account_id=12&account_id=14'), 'account_id')).toEqual([12, 14]);
  });

  it('returns an empty set when the parameter is absent', () => {
    expect(parseIDParam(params('view=spending'), 'account_id')).toEqual([]);
  });
});

describe('parseReportFilters', () => {
  it('reads every dimension', () => {
    const filters = parseReportFilters(
      params('account_id=3&include_descendants=true&commodity_id=1&category_id=7&category_id=8&payee_id=5')
    );
    expect(filters).toEqual({
      accountIDs: [3],
      includeDescendants: true,
      commodityIDs: [1],
      categoryIDs: [7, 8],
      payeeIDs: [5]
    });
  });

  it('treats any value other than true as descendants off', () => {
    expect(parseReportFilters(params('include_descendants=1')).includeDescendants).toBe(false);
    expect(parseReportFilters(params('include_descendants=false')).includeDescendants).toBe(false);
    expect(parseReportFilters(params('')).includeDescendants).toBe(false);
  });
});

describe('writeReportFilters', () => {
  it('preserves parameters the filters do not own', () => {
    const next = writeReportFilters(params('view=spending&start_date=2026-01-01&group_by=payee'), {
      ...emptyReportFilters,
      accountIDs: [4]
    });
    expect(next.get('view')).toBe('spending');
    expect(next.get('start_date')).toBe('2026-01-01');
    expect(next.get('group_by')).toBe('payee');
  });

  it('writes one entry per ID', () => {
    const next = writeReportFilters(params(''), { ...emptyReportFilters, categoryIDs: [7, 8] });
    expect(next.getAll('category_id')).toEqual(['7', '8']);
  });

  it('deletes a dimension that is cleared rather than writing it empty', () => {
    const next = writeReportFilters(params('account_id=4&commodity_id=1'), emptyReportFilters);
    expect(next.getAll('account_id')).toEqual([]);
    expect(next.toString()).toBe('');
  });

  it('drops include_descendants without an account selection, since it expands nothing', () => {
    const next = writeReportFilters(params(''), { ...emptyReportFilters, includeDescendants: true });
    expect(next.has('include_descendants')).toBe(false);
  });

  it('keeps include_descendants alongside an account selection', () => {
    const next = writeReportFilters(params(''), {
      ...emptyReportFilters,
      accountIDs: [4],
      includeDescendants: true
    });
    expect(next.get('include_descendants')).toBe('true');
  });

  it('round-trips through parse unchanged', () => {
    const filters: ReportFilterState = {
      accountIDs: [3, 9],
      includeDescendants: true,
      commodityIDs: [1],
      categoryIDs: [7],
      payeeIDs: [2, 5]
    };
    expect(parseReportFilters(writeReportFilters(params('view=spending'), filters))).toEqual(filters);
  });
});

describe('toggleID', () => {
  it('adds in sorted order and removes on a second toggle', () => {
    expect(toggleID([3, 9], 5)).toEqual([3, 5, 9]);
    expect(toggleID([3, 5, 9], 5)).toEqual([3, 9]);
  });
});

describe('activeFilterCount', () => {
  it('counts dimensions, not IDs, and ignores dimensions the view cannot express', () => {
    const filters: ReportFilterState = {
      accountIDs: [1, 2, 3],
      includeDescendants: true,
      commodityIDs: [],
      categoryIDs: [7],
      payeeIDs: []
    };
    expect(activeFilterCount(filters, ['account', 'commodity', 'category', 'payee'])).toBe(2);
    expect(activeFilterCount(filters, ['account', 'commodity'])).toBe(1);
  });

  it('does not count include_descendants, which widens rather than narrows', () => {
    expect(activeFilterCount({ ...emptyReportFilters, includeDescendants: true }, ['account'])).toBe(0);
  });
});

describe('clearDimensions', () => {
  it('clears only the named dimensions', () => {
    const filters: ReportFilterState = {
      accountIDs: [1],
      includeDescendants: true,
      commodityIDs: [2],
      categoryIDs: [3],
      payeeIDs: [4]
    };
    expect(clearDimensions(filters, ['category', 'payee'])).toEqual({
      accountIDs: [1],
      includeDescendants: true,
      commodityIDs: [2],
      categoryIDs: [],
      payeeIDs: []
    });
  });

  it('clears include_descendants with the account dimension', () => {
    const cleared = clearDimensions({ ...emptyReportFilters, accountIDs: [1], includeDescendants: true }, ['account']);
    expect(cleared.accountIDs).toEqual([]);
    expect(cleared.includeDescendants).toBe(false);
  });
});

describe('withSelectedIDs', () => {
  it('replaces one dimension without touching the others', () => {
    const filters = withSelectedIDs({ ...emptyReportFilters, payeeIDs: [9] }, 'account', [1, 2]);
    expect(filters.accountIDs).toEqual([1, 2]);
    expect(filters.payeeIDs).toEqual([9]);
  });
});
