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

/**
 * A working copy and, per field, the value it last took by itself (opening,
 * restoring, a default or suggestion filling in). A field differing from its
 * baseline was changed by the owner: that is the work closing would lose.
 */
type Copy = { values: WorkingCopyValues; baseline: Record<string, string> };

const fieldKey = (value: unknown) => JSON.stringify(value) ?? 'undefined';

export class EntryWorkingCopies {
  shared: SharedEntrySelections = {};
  /** Each shared selection as the entry opened with it or as it filled itself. */
  sharedBaseline: SharedEntrySelections = {};
  #copies = new Map<InvestmentActivity, Copy>();
  #pairs = new Map<TradePair, PairedTrade>();

  /** The activity's working copy, as it was when the owner left it. */
  restore<T extends WorkingCopyValues>(activity: InvestmentActivity): Partial<T> | undefined {
    const copy = this.#copies.get(activity);
    return copy ? (structuredClone(copy.values) as Partial<T>) : undefined;
  }

  /**
   * Keeps the activity's current fields. `owner` says whether this change
   * came from the owner's own input; any other change (the first keep, a
   * default arriving, a picker filling itself) moves that field's baseline.
   */
  keep(activity: InvestmentActivity, values: WorkingCopyValues, owner: boolean): void {
    const copied = structuredClone(values);
    const existing = this.#copies.get(activity);
    if (!existing) {
      this.#copies.set(activity, {
        values: copied,
        baseline: Object.fromEntries(Object.entries(copied).map(([key, value]) => [key, fieldKey(value)]))
      });
      return;
    }
    if (!owner) {
      for (const [key, value] of Object.entries(copied)) {
        if (fieldKey(value) !== fieldKey(existing.values[key])) existing.baseline[key] = fieldKey(value);
      }
    }
    existing.values = copied;
  }

  /** Records a shared selection; one not made by the owner becomes its baseline. */
  setShared<K extends keyof SharedEntrySelections>(key: K, value: SharedEntrySelections[K], owner: boolean): void {
    this.shared[key] = value;
    if (!owner) this.sharedBaseline[key] = value;
  }

  /**
   * True when closing the entry would lose something the owner entered: a
   * field of an activity, or a shared selection, the owner changed and did
   * not change back.
   */
  hasUnsavedWork(): boolean {
    for (const copy of this.#copies.values()) {
      for (const [key, value] of Object.entries(copy.values)) {
        if (fieldKey(value) !== (copy.baseline[key] ?? fieldKey(undefined))) return true;
      }
    }
    return (Object.keys(this.shared) as (keyof SharedEntrySelections)[]).some((key) =>
      this.shared[key] !== this.sharedBaseline[key]);
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
