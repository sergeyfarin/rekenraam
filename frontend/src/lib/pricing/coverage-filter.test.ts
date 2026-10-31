import { describe, expect, it } from 'vitest';
import { coverageFilter, filterCoverage } from './coverage-filter.ts';

const pair = (base: number, status: 'usable' | 'stale' | 'missing'): Parameters<typeof filterCoverage>[0][number] => ({ base_commodity_id: base, quote_commodity_id: 1, status });
const coverage = [pair(2, 'usable'), pair(3, 'stale'), pair(4, 'missing')];

describe('coverage filter', () => {
  it('reads only the filters Home links to', () => {
    expect(coverageFilter('unusable')).toBe('unusable');
    expect(coverageFilter('stale')).toBe('stale');
    expect(coverageFilter('missing')).toBeNull();
    expect(coverageFilter(null)).toBeNull();
  });

  it('lists a stale pair under unusable: Home calls a price beyond the window missing', () => {
    expect(filterCoverage(coverage, 'unusable').map(item => item.base_commodity_id)).toEqual([3, 4]);
  });

  it('lists only pairs older than the policy under stale', () => {
    expect(filterCoverage(coverage, 'stale').map(item => item.base_commodity_id)).toEqual([3]);
    expect(filterCoverage(coverage, null)).toHaveLength(3);
  });
});
