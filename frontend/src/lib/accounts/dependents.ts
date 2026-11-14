import type { AccountResponse } from '#lib/api/accounts.ts';

/**
 * Child accounts that stand in the way of each lifecycle action on their
 * parent. Each action has its own rule:
 *
 * - archive: the server refuses while any child is not archived (#294);
 * - delete: any child version references the parent, archived or not, and
 *   the server refuses any referenced account;
 * - close: the server allows it, but the index asks for active children to be
 *   closed first, so a closed parent never carries active accounts.
 *
 * Counts must include accounts the index folds into a brokerage row and
 * holdings listed apart, or a parent offers an action the server refuses
 * (#567 review).
 */
export type ChildDependents = { active: number; unarchived: number; all: number };

export const noDependents: ChildDependents = { active: 0, unarchived: 0, all: 0 };

export function childDependents(accounts: Iterable<AccountResponse>): Map<number, ChildDependents> {
  const counts = new Map<number, ChildDependents>();
  const seen = new Set<number>();
  for (const account of accounts) {
    if (!account.parent_account_id || seen.has(account.id)) continue;
    seen.add(account.id);
    const current = counts.get(account.parent_account_id) ?? { ...noDependents };
    current.all++;
    if (account.status !== 'archived') current.unarchived++;
    if (account.status === 'active') current.active++;
    counts.set(account.parent_account_id, current);
  }
  return counts;
}
