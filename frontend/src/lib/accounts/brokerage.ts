import type { AccountResponse } from '#lib/api/accounts.ts';
import { isSubledgerManagedAccountKind } from './subledger-accounts.ts';

// A holding may sit directly under its brokerage or under one of the
// brokerage's children (a cash account, a group). Ledger Overview owns it by
// its nearest brokerage ancestor, so links back to the broker walk the same
// ancestry rather than trusting the direct parent (#567 review). Like the
// server's owner walk, it has no depth limit; only a loop stops it.

export function isHoldingAccount(account: Pick<AccountResponse, 'account_kind'> | undefined | null): boolean {
  return isSubledgerManagedAccountKind(account?.account_kind);
}

/** The nearest brokerage above `account`, read from an in-memory catalog. */
export function nearestBrokerage(
  account: AccountResponse,
  byID: ReadonlyMap<number, AccountResponse>
): AccountResponse | undefined {
  const seen = new Set<number>([account.id]);
  let parentID = account.parent_account_id;
  while (parentID && !seen.has(parentID)) {
    seen.add(parentID);
    const parent = byID.get(parentID);
    if (!parent) return undefined;
    if (parent.account_kind === 'brokerage') return parent;
    parentID = parent.parent_account_id;
  }
  return undefined;
}

/**
 * The same walk for a page that holds one account rather than the catalog:
 * reads each ancestor by id (usually one read, the direct parent).
 */
export async function findBrokerageAncestor(
  account: AccountResponse,
  read: (id: number) => Promise<AccountResponse>
): Promise<AccountResponse | null> {
  const seen = new Set<number>([account.id]);
  let parentID = account.parent_account_id;
  while (parentID && !seen.has(parentID)) {
    seen.add(parentID);
    const parent = await read(parentID);
    if (parent.account_kind === 'brokerage') return parent;
    parentID = parent.parent_account_id;
  }
  return null;
}
