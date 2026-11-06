import type { InvestmentEntryAccount } from '#lib/api/investment-entry.ts';

/** Only the explicit direct-child hierarchy is evidence for a suggestion. */
export function settlementCandidates(
  accounts: readonly InvestmentEntryAccount[], holdingID?: number, startingID?: number,
  currencyID?: number
): InvestmentEntryAccount[] {
  const holding = accounts.find((account) => account.id === holdingID);
  const start = accounts.find((account) => account.id === startingID);
  const parent = accounts.find((account) => account.id === holding?.parent_account_id);
  const brokerageID = parent?.account_kind === 'brokerage' ? parent.id
    : start?.account_kind === 'brokerage' ? start.id : undefined;
  return accounts.filter((account) => account.investment_settlement &&
    (currencyID === undefined || account.default_commodity_id === currencyID) &&
    (account.id === startingID || (brokerageID !== undefined &&
      account.parent_account_id === brokerageID && account.account_kind === 'brokerage_cash')));
}

/** Explicit decisions, including choosing the empty prompt, are sticky. */
export function suggestedSettlement(
  current: string, explicit: boolean, candidates: readonly InvestmentEntryAccount[]
): string {
  if (explicit) return current;
  return candidates.length === 1 && candidates[0].account_class === 'asset' ? String(candidates[0].id) : '';
}
