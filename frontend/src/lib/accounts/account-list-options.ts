import type { AccountKind } from './account-labels';

export type StatusFilter = 'active' | 'closed' | 'all' | 'archived';
export type AccountTypeFilter = AccountKind | 'all';
