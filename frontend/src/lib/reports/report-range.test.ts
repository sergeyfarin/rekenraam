import { describe, expect, it } from 'vitest';
import { defaultReportRange, parseReportRange, repairReportRange } from './report-range';

const params = (query: string) => new URLSearchParams(query);
const today = '2026-08-23';

describe('parseReportRange', () => {
  it('reads a complete range', () => {
    expect(parseReportRange(params('start_date=2026-01-01&end_date=2026-06-30&bucket=quarter'))).toEqual({
      startDate: '2026-01-01',
      endDate: '2026-06-30',
      bucket: 'quarter'
    });
  });

  it('returns null when a part is missing or malformed', () => {
    expect(parseReportRange(params('start_date=2026-01-01&end_date=2026-06-30'))).toBeNull();
    expect(parseReportRange(params('start_date=January&end_date=2026-06-30&bucket=month'))).toBeNull();
    expect(parseReportRange(params('start_date=2026-01-01&end_date=2026-06-30&bucket=fortnight'))).toBeNull();
  });
});

describe('repairReportRange', () => {
  it('leaves a valid range alone so no redundant navigation happens', () => {
    expect(repairReportRange(params('start_date=2026-01-01&end_date=2026-06-30&bucket=month'), today)).toBeNull();
  });

  it('preserves the view, grouping, and filters of a link that only lacks a range', () => {
    const repaired = repairReportRange(params('view=spending&group_by=payee&account_id=12&mode=income'), today);
    expect(repaired?.get('view')).toBe('spending');
    expect(repaired?.get('group_by')).toBe('payee');
    expect(repaired?.get('mode')).toBe('income');
    expect(repaired?.getAll('account_id')).toEqual(['12']);
    expect(repaired?.get('start_date')).toBe('2026-01-01');
    expect(repaired?.get('end_date')).toBe(today);
    expect(repaired?.get('bucket')).toBe('month');
  });

  it('repairs only the malformed part and keeps the parts that were valid', () => {
    const repaired = repairReportRange(
      params('view=cashflow&start_date=2026-03-01&end_date=2026-03-31&bucket=fortnight'),
      today
    );
    expect(repaired?.get('start_date')).toBe('2026-03-01');
    expect(repaired?.get('end_date')).toBe('2026-03-31');
    expect(repaired?.get('bucket')).toBe('month');
    expect(repaired?.get('view')).toBe('cashflow');
  });

  it('keeps every value of a repeated dimension', () => {
    const repaired = repairReportRange(params('commodity_id=3&commodity_id=7&include_descendants=true'), today);
    expect(repaired?.getAll('commodity_id')).toEqual(['3', '7']);
    expect(repaired?.get('include_descendants')).toBe('true');
  });
});

describe('defaultReportRange', () => {
  it('runs from the start of the year to today, bucketed by month', () => {
    expect(defaultReportRange(today)).toEqual({
      startDate: '2026-01-01',
      endDate: today,
      bucket: 'month'
    });
  });
});
