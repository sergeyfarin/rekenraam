import type { InvestmentEntryAccount } from '#lib/api/investment-entry.ts';
import type { InvestmentPrefill } from './investment-prefill.ts';
import { settlementCandidates, suggestedSettlement } from './settlement-candidates.ts';

export function useSettlementCandidates(
  accounts: () => readonly InvestmentEntryAccount[] | undefined,
  holding: () => number | undefined,
  selected: () => string,
  setSelected: (id: string) => void,
  prefill: InvestmentPrefill | undefined,
  correction = false,
  currency?: () => number | undefined
): () => void {
  let explicit = $state(correction);
  $effect(() => {
    const rows = accounts();
    if (!rows || explicit) return;
    const next = suggestedSettlement(selected(), explicit,
      settlementCandidates(rows, holding(), prefill?.startingAccountID, currency?.()));
    if (next !== selected()) setSelected(next);
  });
  return () => { explicit = true; };
}
