import { untrack } from 'svelte';
import { createQuery } from '@tanstack/svelte-query';
import { investmentInstrumentsQueryOptions, type InvestmentInstrumentResponse } from '#lib/api/investments.ts';
import { prefilledInstrument, prefilledPositionKey, type InvestmentPrefill } from './investment-prefill.ts';

// The form-side halves of InvestmentPrefill (#186). Call them during a form's
// initialisation; the pure rules they apply live in investment-prefill.ts.

/**
 * Selects the pre-filled instrument in an instrument search once the
 * instrument list loads, through the form's own `select` so any per-instrument
 * defaults follow. Applied once: later choices are the user's.
 */
export function usePrefilledInstrument(
  prefill: InvestmentPrefill | undefined,
  select: (instrument: InvestmentInstrumentResponse) => void
): void {
  if (prefill?.commodityID === undefined) return;
  const instrumentsQuery = createQuery(() => investmentInstrumentsQueryOptions());
  let applied = false;
  $effect(() => {
    const instrument = prefilledInstrument(instrumentsQuery.data?.instruments, prefill);
    if (applied || !instrument) return;
    applied = true;
    untrack(() => select(instrument));
  });
}

/**
 * Selects the pre-filled position in a position picker whenever its
 * candidates change (a dated picker reloads them with the date) and nothing
 * is chosen yet. A position an earlier activity in the entry chose is kept
 * as it is (`restoredKey`), available or not: the form explains an
 * unavailable one rather than leaving the picker blank (#521 review).
 */
export function usePrefilledPosition<T extends { account_id: number; commodity_id: number; cost_commodity_id?: number }>(
  prefill: InvestmentPrefill | undefined,
  candidates: () => readonly T[],
  keyOf: (candidate: T) => string,
  current: () => string,
  apply: (key: string) => void,
  restoredKey?: string
): void {
  if (!prefill) return;
  if (restoredKey) {
    // In an effect: forms call this before declaring the picker's state.
    let applied = false;
    $effect(() => {
      if (applied) return;
      applied = true;
      untrack(() => { if (current() === '') apply(restoredKey); });
    });
    return;
  }
  $effect(() => {
    const key = prefilledPositionKey(candidates(), prefill, keyOf);
    if (key && untrack(current) === '') untrack(() => apply(key));
  });
}
