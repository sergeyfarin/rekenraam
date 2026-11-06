import type { AccountResponse } from '#lib/api/accounts.ts';
import { isSubledgerManagedAccountKind } from '#lib/accounts/subledger-accounts.ts';

/**
 * Accounts an investment cash leg may settle through, mirroring the backend's
 * `settlementRole` (`app/investments_roles.go`): an asset or liability that
 * accepts postings, is not a system account and is not a holding the
 * investment subledger manages. A margin loan or credit line is a valid
 * settlement account, so liabilities are offered alongside cash.
 *
 * `retainedID` keeps a correction's recorded account selectable even after it
 * was archived, so its picker never silently falls back to another account.
 * The caller must fetch archived accounts for that to work.
 */
export function isSettlementAccount(account: AccountResponse & { investment_settlement?: boolean }, retainedID?: number): boolean {
  if (retainedID !== undefined && account.id === retainedID) return true;
  if (account.investment_settlement !== undefined) return account.investment_settlement;
  return ['asset', 'liability'].includes(account.account_class) && account.allows_postings &&
    !account.system_role && !isSubledgerManagedAccountKind(account.account_kind) && account.status === 'active';
}

export function isInvestmentHoldingAccount(account: AccountResponse & { investment_holding?: boolean }): boolean {
  return account.investment_holding ?? (account.status === 'active' && account.allows_postings &&
    !account.system_role && isSubledgerManagedAccountKind(account.account_kind));
}
