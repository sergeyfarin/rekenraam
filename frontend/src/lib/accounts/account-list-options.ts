import type { AccountClass } from './account-labels';

export type GroupMode = 'class' | 'institution' | 'country' | 'kind' | 'status' | 'none';
export type SortMode = 'name' | 'class' | 'institution' | 'country' | 'kind' | 'updated';
export type StatusFilter = 'active' | 'closed' | 'all';
export type ClassFilter = AccountClass | 'all';
