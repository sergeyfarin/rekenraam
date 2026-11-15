import { describe, expect, it } from 'vitest';
import type { AccountResponse } from '#lib/api/accounts.ts';
import { childDependents } from './dependents.ts';

const account = (id: number, status: string, parent?: number) =>
  ({ id, status, parent_account_id: parent ?? null }) as unknown as AccountResponse;

describe('childDependents', () => {
  it('counts each child once per action rule', () => {
    const counts = childDependents([
      account(1, 'active'),
      account(2, 'active', 1),
      account(3, 'closed', 1),
      account(4, 'archived', 1),
      account(4, 'archived', 1),
      account(5, 'archived', 9)
    ]);
    expect(counts.get(1)).toEqual({ active: 1, unarchived: 2, all: 3 });
    // An archived child never blocks archiving its parent, but still blocks delete.
    expect(counts.get(9)).toEqual({ active: 0, unarchived: 0, all: 1 });
    expect(counts.get(2)).toBeUndefined();
  });
});
