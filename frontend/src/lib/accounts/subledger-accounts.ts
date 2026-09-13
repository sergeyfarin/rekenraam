import type { components } from '$lib/api/schema';

type AccountKind = components['schemas']['AccountKind'];

/**
 * Account kinds whose holdings the investment subledger owns. A quantity in one
 * of these accounts is the journal half of a position whose other half is a set
 * of lots, so only an investment command — buy, sell, dividend, write-off — may
 * write to it. An ordinary transaction that changes one of these balances would
 * leave shares that no lot accounts for, which makes cost basis and realized
 * gains uncomputable for that position (T-96).
 *
 * The backend refuses these postings outright; this list keeps them out of the
 * pickers so the user never reaches a refusal they cannot act on. It mirrors the
 * `security_holding` base_kind family in the `account_kinds` table — keep the
 * two in step when a holding kind is added.
 *
 * `crypto_wallet` is deliberately absent: the investment commands do not offer
 * crypto accounts, so hiding them here would leave no way to record a crypto
 * balance at all.
 */
const SUBLEDGER_MANAGED_ACCOUNT_KINDS: ReadonlySet<string> = new Set<AccountKind>([
  'security_holding',
  'fund_holding'
]);

export function isSubledgerManagedAccountKind(kind: string | undefined | null): boolean {
  return kind != null && SUBLEDGER_MANAGED_ACCOUNT_KINDS.has(kind);
}
