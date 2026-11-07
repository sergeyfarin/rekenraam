import type { InvestmentEntryAccount } from '#lib/api/investment-entry.ts';
import { entryContext } from '#lib/transactions/entry-context.svelte.ts';
import type { InvestmentPrefill } from './investment-prefill.ts';
import { settlementCandidates, suggestedSettlement } from './settlement-candidates.ts';

/**
 * The settlement picker's selection rule (#507): the owner's choice wins,
 * and a sole valid candidate is suggested only while nothing was chosen.
 * Inside New transaction (#508) an explicit choice is shared: every
 * cash-moving activity opened afterwards starts on it, still validated by its
 * own form, and an automatic suggestion is never shared. Call during the
 * form's initialisation, after its settlement field exists.
 */
export function useSettlementCandidates(
  accounts: () => readonly InvestmentEntryAccount[] | undefined,
  holding: () => number | undefined,
  selected: () => string,
  setSelected: (id: string) => void,
  prefill: InvestmentPrefill | undefined,
  correction = false,
  currency?: () => number | undefined
): () => void {
  const context = correction ? undefined : entryContext();
  const chosen = context?.shared.settlementAccountID;
  if (chosen !== undefined) setSelected(String(chosen));
  let explicit = $state(correction || chosen !== undefined);
  $effect(() => {
    const rows = accounts();
    if (!rows || explicit) return;
    const next = suggestedSettlement(selected(), explicit,
      settlementCandidates(rows, holding(), prefill?.startingAccountID, currency?.()));
    if (next !== selected()) setSelected(next);
  });
  return () => {
    explicit = true;
    // After the select's own binding has stored the choice.
    if (context) queueMicrotask(() => {
      const value = selected();
      context.copies.setShared('settlementAccountID', value ? Number(value) : undefined, true);
    });
  };
}
