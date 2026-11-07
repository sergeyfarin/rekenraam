import { tradePair, type InvestmentActivity, type TradePair } from '#lib/investments/activities.ts';

/**
 * New transaction's browser-only memory for one open entry (#508, design
 * "Shared context and working copies"). Nothing here is a `draft`
 * transaction: it lives exactly as long as the entry and is never sent.
 *
 * - `shared` holds the selections every activity reads by stable ID: the
 *   holding, the instrument in its subject role, the settlement account the
 *   owner chose, and the currency. A value changed in any activity is the one
 *   every activity sees afterwards. (The main date and memo are the editor's
 *   own fields; see entry-context.svelte.ts.)
 * - Each activity keeps its own working copy of the fields only it has:
 *   amounts, charges, secondary dates, lot choices, ratios, declarations.
 * - Buy ↔ Sell and Short sale ↔ Short cover share one quantity and unit
 *   price (`TradePair`). Nothing else carries between activities.
 */
export type SharedEntrySelections = {
  holdingAccountID?: number;
  commodityID?: number;
  /** Only an explicit owner choice; an automatic suggestion is never shared. */
  settlementAccountID?: number;
  currencyID?: number;
};

export type WorkingCopyValues = Record<string, unknown>;

export type PairedTrade = { quantity: string; unitPrice: string };

/** The trade fields a paired activity keeps in its working copy. */
export type TradeCopy = PairedTrade & { cashAmount: string; gross: string };

type Copy = { initial: string; values: WorkingCopyValues; ignore: readonly string[] };

/** The values as loss detection reads them: without fields the form fills by itself. */
function lossKey(values: WorkingCopyValues, ignore: readonly string[]): string {
  return JSON.stringify(Object.fromEntries(Object.entries(values).filter(([key]) => !ignore.includes(key))));
}

export class EntryWorkingCopies {
  shared: SharedEntrySelections = {};
  /** The selections the entry opened with: the page's holding or instrument. */
  opening: SharedEntrySelections = {};
  /**
   * The owner changed a control in an activity form. Pickers also fill
   * themselves (a holding's only position), so a shared selection that
   * differs from the opening counts as work only after the owner acted.
   */
  touched = false;
  #copies = new Map<InvestmentActivity, Copy>();
  #pairs = new Map<TradePair, PairedTrade>();

  /** The activity's working copy, as it was when the owner left it. */
  restore<T extends WorkingCopyValues>(activity: InvestmentActivity): Partial<T> | undefined {
    const copy = this.#copies.get(activity);
    return copy ? (structuredClone(copy.values) as Partial<T>) : undefined;
  }

  /**
   * Keeps the activity's current fields. The first call for an activity is
   * what it opened with, so only a later difference is the owner's work.
   * `ignore` names fields the form fills by itself (a default income
   * account), which are restored but never count as work to lose.
   */
  keep(activity: InvestmentActivity, values: WorkingCopyValues, ignore: readonly string[] = []): void {
    const existing = this.#copies.get(activity);
    const copied = structuredClone(values);
    if (existing) existing.values = copied;
    else this.#copies.set(activity, { initial: lossKey(copied, ignore), values: copied, ignore });
  }

  /**
   * True when closing the entry would lose something the owner entered: an
   * activity's own fields, or a shared selection the owner changed from what
   * the entry opened with (an explicit settlement choice included).
   */
  hasUnsavedWork(): boolean {
    for (const copy of this.#copies.values()) {
      if (lossKey(copy.values, copy.ignore) !== copy.initial) return true;
    }
    if (!this.touched) return false;
    return (Object.keys(this.shared) as (keyof SharedEntrySelections)[]).some((key) =>
      this.shared[key] !== undefined && this.shared[key] !== this.opening[key]);
  }

  pairOf(activity: InvestmentActivity): PairedTrade | undefined {
    const pair = tradePair(activity);
    const value = pair && this.#pairs.get(pair);
    return value ? { ...value } : undefined;
  }

  setPair(activity: InvestmentActivity, value: PairedTrade): void {
    const pair = tradePair(activity);
    if (pair) this.#pairs.set(pair, { ...value });
  }
}

/**
 * A paired trade's fields when it opens (#508): its own working copy,
 * combined with the quantity and unit price last entered in either activity
 * of its pair. A total the owner typed for another quantity or price is never
 * reused: when the pair changed since this activity was left, its typed
 * total and gross are cleared (`recalculate`), and with a unit price the form
 * derives them again with its own arithmetic.
 */
export function restoreTradeCopy(
  copy: Partial<TradeCopy> | undefined,
  pair: PairedTrade | undefined
): TradeCopy & { recalculate: boolean } {
  const quantity = pair?.quantity ?? copy?.quantity ?? '';
  const unitPrice = pair?.unitPrice ?? copy?.unitPrice ?? '';
  const changed = !!copy && ((copy.quantity ?? '') !== quantity || (copy.unitPrice ?? '') !== unitPrice);
  const typed = !!(copy?.cashAmount || copy?.gross);
  return {
    quantity,
    unitPrice,
    cashAmount: changed ? '' : copy?.cashAmount ?? '',
    gross: changed ? '' : copy?.gross ?? '',
    recalculate: changed && typed && unitPrice.trim() === ''
  };
}
