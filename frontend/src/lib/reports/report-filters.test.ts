import { describe, it, expect } from 'vitest';
import {
  defaultFilters,
  parseReportFilters,
  reportFiltersToHref,
  reportFiltersToSearch,
  todayISO,
  type ReportFilters
} from './report-filters';

const filters = (overrides: Partial<ReportFilters> = {}): ReportFilters => ({
  view: 'net-worth',
  startDate: '2026-01-01',
  endDate: '2026-06-30',
  bucket: 'month',
  groupBy: 'category',
  direction: 'expense',
  ...overrides
});

describe('todayISO', () => {
  it('returns the local calendar date, not the UTC one', () => {
    // 2026-06-30T23:30 in a UTC+2 zone is still the 30th locally while
    // toISOString() would already say the 30th at 21:30Z — pick a moment where
    // the two genuinely disagree by constructing from local parts.
    const localMidday = new Date(2026, 5, 30, 12, 0, 0);
    expect(todayISO(localMidday)).toBe('2026-06-30');
  });

  it('zero-pads single-digit months and days', () => {
    expect(todayISO(new Date(2026, 0, 5, 12, 0, 0))).toBe('2026-01-05');
  });
});

describe('defaultFilters', () => {
  it('opens on year-to-date by month', () => {
    const now = new Date(2026, 7, 8, 12, 0, 0);

    expect(defaultFilters(now)).toEqual({
      view: 'net-worth',
      startDate: '2026-01-01',
      endDate: '2026-08-08',
      bucket: 'month',
      groupBy: 'category',
      direction: 'expense'
    });
  });
});

describe('parseReportFilters', () => {
  const now = new Date(2026, 7, 8, 12, 0, 0);

  it('reads a fully specified URL', () => {
    const search = new URLSearchParams(
      'view=spending&start_date=2026-02-01&end_date=2026-02-28&bucket=week&group_by=payee&direction=income'
    );

    expect(parseReportFilters(search, now)).toEqual({
      view: 'spending',
      startDate: '2026-02-01',
      endDate: '2026-02-28',
      bucket: 'week',
      groupBy: 'payee',
      direction: 'income'
    });
  });

  it('falls back to defaults for unrecognised non-date parameters', () => {
    // A bad bucket is a broken link, not a reason to refuse to show the report.
    const search = new URLSearchParams(
      'start_date=2026-02-01&end_date=2026-02-28&view=treemap&bucket=fortnight&group_by=tag&direction=transfer'
    );

    expect(parseReportFilters(search, now)).toEqual({
      view: 'net-worth',
      startDate: '2026-02-01',
      endDate: '2026-02-28',
      bucket: 'month',
      groupBy: 'category',
      direction: 'expense'
    });
  });

  it.each([
    ['both dates missing', ''],
    ['only a start date', 'start_date=2026-02-01'],
    ['a malformed date', 'start_date=2026-2-1&end_date=2026-02-28'],
    ['a non-date value', 'start_date=last-tuesday&end_date=2026-02-28']
  ])('returns null for %s so the screen can redirect to the default preset', (_name, query) => {
    expect(parseReportFilters(new URLSearchParams(query), now)).toBeNull();
  });

  it('returns null for an inverted range rather than passing it to the backend', () => {
    // Failing at the URL boundary shows a filter form the user can fix instead
    // of an error panel.
    const search = new URLSearchParams('start_date=2026-06-30&end_date=2026-01-01');

    expect(parseReportFilters(search, now)).toBeNull();
  });

  it('accepts a single-day range', () => {
    const search = new URLSearchParams('start_date=2026-02-01&end_date=2026-02-01');

    expect(parseReportFilters(search, now)?.startDate).toBe('2026-02-01');
  });
});

describe('reportFiltersToSearch', () => {
  it('writes every filter so switching views preserves the others', () => {
    // The user picked payee/income while on spending; moving to cashflow and
    // back must not silently reset those to the defaults.
    const search = new URLSearchParams(
      reportFiltersToSearch(filters({ view: 'cashflow', groupBy: 'payee', direction: 'income' }))
    );

    expect(search.get('view')).toBe('cashflow');
    expect(search.get('group_by')).toBe('payee');
    expect(search.get('direction')).toBe('income');
    expect(search.get('bucket')).toBe('month');
  });

  it('round-trips through parseReportFilters unchanged', () => {
    const original = filters({ view: 'spending', bucket: 'quarter', groupBy: 'payee', direction: 'income' });

    expect(parseReportFilters(new URLSearchParams(reportFiltersToSearch(original)))).toEqual(original);
  });

  it('builds a href on the reports route', () => {
    expect(reportFiltersToHref(filters())).toBe(
      '/app/reports?view=net-worth&start_date=2026-01-01&end_date=2026-06-30&bucket=month&group_by=category&direction=expense'
    );
  });
});
