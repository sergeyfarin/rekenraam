import type { AccountKind } from './account-labels';

export type StatusFilter = 'active' | 'closed' | 'all' | 'archived';

const statusFilters: readonly StatusFilter[] = ['active', 'closed', 'all', 'archived'];

/** The `status` query parameter; anything else is the default, active. */
export function parseStatusFilter(value: string | null): StatusFilter {
  return statusFilters.find((filter) => filter === value) ?? 'active';
}
export type AccountTypeFilter = AccountKind | 'all';
