import { getContext, setContext, untrack } from 'svelte';
import type { InvestmentActivity } from '#lib/investments/activities.ts';
import { EntryWorkingCopies, restoreTradeCopy, type SharedEntrySelections, type TradeCopy, type WorkingCopyValues } from './entry-working-copies.ts';

/**
 * One open New transaction's shared context (#508). The editor provides it in
 * create mode; the main date, memo and reference are the editor's own fields, read and
 * written through `access`, so a date changed in Sell is the date every tab
 * and activity sees. Investment forms opened elsewhere (corrections) find
 * none and behave exactly as before.
 *
 * A plain context key rather than `createContext`: its getter throws where no
 * entry exists, and the same forms run outside one.
 */
const key = Symbol('new-transaction-entry');

/** The editor's main date and memo fields. */
export type EntryAccess = {
  date(): string; setDate(value: string): void;
  memo(): string; setMemo(value: string): void;
  reference(): string; setReference(value: string): void;
};

export class EntryContext {
  /** The Activity selector's choice, kept while another tab shows. */
  activity = $state<InvestmentActivity | ''>('');
  /** An investment save is in flight: account, tab and activity are locked. */
  saving = $state(false);
  readonly copies = new EntryWorkingCopies();
  /**
   * True while the owner's own input event is being handled, including the
   * effects it causes (bindings, derived totals). A change outside it (a
   * query arriving, a picker filling itself) is not the owner's work.
   */
  ownerInput = false;
  #inputTimer: ReturnType<typeof setTimeout> | undefined;

  /** An activity form received the owner's input, change, click or key event. */
  markOwnerInput(): void {
    this.ownerInput = true;
    clearTimeout(this.#inputTimer);
    // After the event's handlers and Svelte's effect flush (microtasks).
    this.#inputTimer = setTimeout(() => { this.ownerInput = false; }, 0);
  }
  #access: EntryAccess;

  constructor(access: EntryAccess) {
    this.#access = access;
  }

  get date() { return this.#access.date(); }
  set date(value: string) { this.#access.setDate(value); }
  get memo() { return this.#access.memo(); }
  set memo(value: string) { this.#access.setMemo(value); }
  get reference() { return this.#access.reference(); }
  set reference(value: string) { this.#access.setReference(value); }
  get shared(): SharedEntrySelections { return this.copies.shared; }
}

export function provideEntryContext(context: EntryContext): void {
  setContext(key, context);
}

export function entryContext(): EntryContext | undefined {
  return getContext<EntryContext | undefined>(key);
}

/** Calls `write` when `read` changes after the form opened, never for its opening value. */
function follow<V>(read: () => V, write: (value: V) => void): void {
  let first = true;
  let last: V;
  $effect(() => {
    const value = read();
    if (first) {
      first = false;
      last = value;
      return;
    }
    if (Object.is(value, last)) return;
    last = value;
    untrack(() => write(value));
  });
}

export type ActivityBinding<T extends WorkingCopyValues> = {
  /** Corrections pre-fill from the recorded operation and keep no working copy. */
  correction?: boolean;
  /** The fields only this activity has; restored when it opens again. */
  values: () => T;
  /** The activity's main date; secondary dates belong in `values`. */
  date?: () => string;
  memo?: () => string;
  /** The source or broker reference, shared like the memo. */
  reference?: () => string;
  holding?: () => number | undefined;
  /** The instrument in its subject role (never a new or received instrument). */
  instrument?: () => number | undefined;
  currency?: () => number | undefined;
  pending: () => boolean;
};

/**
 * Connects an investment form to the open entry (#508). Call it during the
 * form's initialisation, after its fields exist. It returns the working copy
 * the activity was left with, if any; the form restores those fields itself,
 * and takes the shared selections from its prefill. From then on every change
 * flows back: owner fields to the working copy, and shared selections, the
 * main date and memo to the context.
 */
export function useEntryActivity<T extends WorkingCopyValues>(
  activity: InvestmentActivity,
  binding: ActivityBinding<T>
): Partial<T> | undefined {
  const context = untrack(() => binding.correction) ? undefined : entryContext();
  if (!context) return undefined;
  const restored = context.copies.restore<T>(activity);
  $effect(() => {
    const values = $state.snapshot(binding.values()) as WorkingCopyValues;
    untrack(() => context.copies.keep(activity, values, context.ownerInput));
  });
  // An emptied date keeps the last real one: the main date is never blank.
  if (binding.date) follow(binding.date, (value) => { if (value) context.date = value; });
  if (binding.memo) follow(binding.memo, (value) => { context.memo = value; });
  if (binding.reference) follow(binding.reference, (value) => { context.reference = value; });
  // A holding, instrument or currency is shared once chosen. An emptied
  // picker (a dated list reloading, a write-off date change) keeps the last
  // choice for the other activities, which validate it themselves.
  const share = (key: 'holdingAccountID' | 'commodityID' | 'currencyID') => (value: number | undefined) => {
    if (value !== undefined) context.copies.setShared(key, value, context.ownerInput);
  };
  if (binding.holding) follow(binding.holding, share('holdingAccountID'));
  if (binding.instrument) follow(binding.instrument, share('commodityID'));
  if (binding.currency) follow(binding.currency, share('currencyID'));
  $effect(() => {
    context.saving = binding.pending();
    return () => { context.saving = false; };
  });
  return restored;
}

/**
 * A paired trade's opening fields and the pair's upkeep (#508): Buy ↔ Sell
 * and Short sale ↔ Short cover keep one quantity and unit price; each keeps
 * its own total, gross and charges, and a total typed for another quantity is
 * never reused (restoreTradeCopy).
 */
export function useTradePair(
  activity: InvestmentActivity,
  correction: boolean,
  copy: Partial<TradeCopy> | undefined,
  current: () => { quantity: string; unitPrice: string }
): (TradeCopy & { recalculate: boolean }) | undefined {
  const context = correction ? undefined : entryContext();
  if (!context) return undefined;
  const opening = restoreTradeCopy(copy, context.copies.pairOf(activity));
  $effect(() => {
    const { quantity, unitPrice } = current();
    untrack(() => context.copies.setPair(activity, { quantity, unitPrice }));
  });
  return opening;
}
