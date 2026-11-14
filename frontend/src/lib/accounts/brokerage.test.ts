import { describe, expect, it } from 'vitest';
import type { AccountResponse } from '#lib/api/accounts.ts';
import { findBrokerageAncestor, nearestBrokerage } from './brokerage.ts';

const account = (id: number, kind: string, parent?: number) =>
  ({ id, account_kind: kind, parent_account_id: parent ?? null }) as unknown as AccountResponse;

describe('brokerage ancestry', () => {
  const broker = account(1, 'brokerage');
  const cash = account(2, 'brokerage_cash', 1);
  const direct = account(3, 'security_holding', 1);
  const nested = account(4, 'security_holding', 2);
  const orphan = account(5, 'security_holding');
  const loopA = account(6, 'other_asset', 7);
  const loopB = account(7, 'other_asset', 6);
  const looped = account(8, 'security_holding', 6);
  const byID = new Map([broker, cash, direct, nested, orphan, loopA, loopB, looped].map(a => [a.id, a]));

  it('finds a direct or nested brokerage and nothing for an orphan or a cycle', () => {
    expect(nearestBrokerage(direct, byID)?.id).toBe(1);
    expect(nearestBrokerage(nested, byID)?.id).toBe(1);
    expect(nearestBrokerage(orphan, byID)).toBeUndefined();
    expect(nearestBrokerage(looped, byID)).toBeUndefined();
  });

  it('walks by id reads the same way', async () => {
    const read = async (id: number) => byID.get(id)!;
    expect((await findBrokerageAncestor(nested, read))?.id).toBe(1);
    expect(await findBrokerageAncestor(orphan, read)).toBeNull();
    expect(await findBrokerageAncestor(looped, read)).toBeNull();
  });

  it('follows a chain deeper than any fixed limit, as the server owner walk does', async () => {
    const chain = [broker, ...Array.from({ length: 40 }, (_, i) => account(100 + i, 'other_asset', i === 0 ? 1 : 99 + i))];
    const deep = account(200, 'security_holding', 139);
    const deepByID = new Map([...chain, deep].map(a => [a.id, a]));
    expect(nearestBrokerage(deep, deepByID)?.id).toBe(1);
    expect((await findBrokerageAncestor(deep, async id => deepByID.get(id)!))?.id).toBe(1);
  });
});
