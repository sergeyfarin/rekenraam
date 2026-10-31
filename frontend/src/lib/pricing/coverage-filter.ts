import type { PricingWorkspace } from '#lib/api/price-management.ts';

type Coverage = PricingWorkspace['coverage'][number];

/**
 * Prices filtered from Home's status line (#476). `unusable` is every pair
 * without a price the reports can value with, which is what Home calls
 * missing; `stale` is a price older than the pricing policy, what Home and
 * Review call out of date. The workspace's own statuses decide both.
 */
export type CoverageFilter = 'unusable' | 'stale';

export function coverageFilter(value: string | null): CoverageFilter | null {
  return value === 'unusable' || value === 'stale' ? value : null;
}

export function filterCoverage(coverage: readonly Coverage[], filter: CoverageFilter | null): Coverage[] {
  if (filter === null) return [...coverage];
  return coverage.filter(item => (filter === 'stale' ? item.status === 'stale' : item.status !== 'usable'));
}
