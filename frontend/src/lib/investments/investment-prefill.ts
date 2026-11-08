import { isSubledgerManagedAccountKind } from '#lib/accounts/subledger-accounts.ts';

/**
 * What a screen already knows when it offers investment entry (#186): a
 * holding account's register knows the holding, an instrument's view knows
 * the instrument. Every record form takes this one shape, so the Investments
 * screen, a register and an instrument share the forms rather than keeping
 * per-screen copies. A correction never uses it: it pre-fills from the
 * recorded operation instead.
 */
export type InvestmentPrefill = {
  holdingAccountID?: number;
  startingAccountID?: number;
  transactionDate?: string;
  memo?: string;
  /** New transaction's shared reference (#508). */
  reference?: string;
  /** New transaction's shared currency, where an activity chooses one (#508). */
  currencyID?: number;
  /**
   * The holding and instrument are an earlier activity's choice in this entry,
   * not the page's. A position picker then keeps that position identified,
   * even when it is not available on the current date (#521 review).
   */
  restoredSelection?: boolean;
  commodityID?: number;
};

type PrefillAccount = {
  id: number;
  account_kind?: string | null;
  status: string;
  default_commodity_id?: number | null;
};

type PrefillInstrument = { commodity_id: number };

function isInstrument(instruments: readonly PrefillInstrument[], commodityID: number | null | undefined): commodityID is number {
  return commodityID != null && instruments.some((instrument) => instrument.commodity_id === commodityID);
}

/**
 * A holding account's register pre-selects the holding, and its instrument
 * when the account's default commodity is one (a holding created from an
 * instrument always is).
 */
export function holdingAccountPrefill(
  account: PrefillAccount,
  instruments: readonly PrefillInstrument[]
): InvestmentPrefill {
  return {
    holdingAccountID: account.id,
    commodityID: isInstrument(instruments, account.default_commodity_id) ? account.default_commodity_id : undefined
  };
}

/**
 * An instrument's view pre-selects the instrument, and its holding when
 * exactly one active holding account carries it. With several, the user
 * chooses; guessing would book the trade to the wrong account.
 */
export function instrumentPrefill(commodityID: number, accounts: readonly PrefillAccount[]): InvestmentPrefill {
  const holdings = accounts.filter((account) =>
    account.status === 'active' && isSubledgerManagedAccountKind(account.account_kind) &&
    account.default_commodity_id === commodityID);
  return { commodityID, holdingAccountID: holdings.length === 1 ? holdings[0].id : undefined };
}

export function hasPrefill(prefill: InvestmentPrefill | undefined): prefill is InvestmentPrefill {
  return !!prefill && (prefill.holdingAccountID !== undefined || prefill.commodityID !== undefined);
}

/** The instrument a form's instrument picker starts on, once instruments load. */
export function prefilledInstrument<T extends PrefillInstrument>(
  instruments: readonly T[] | undefined,
  prefill: InvestmentPrefill | undefined
): T | undefined {
  if (prefill?.commodityID === undefined) return undefined;
  return instruments?.find((instrument) => instrument.commodity_id === prefill.commodityID);
}

/**
 * The position a position picker starts on: the only candidate that matches
 * every pre-filled field. A position also has a cost currency (and a side), so
 * a holding with two cost currencies stays a deliberate choice, unless New
 * transaction's shared cost currency names one of them (#508 review). A
 * shared currency the holding does not carry selects nothing: the owner
 * chooses, and nothing is substituted.
 */
export function prefilledPositionKey<T extends { account_id: number; commodity_id: number; cost_commodity_id?: number }>(
  candidates: readonly T[],
  prefill: InvestmentPrefill | undefined,
  keyOf: (candidate: T) => string
): string {
  if (!hasPrefill(prefill)) return '';
  const matching = candidates.filter((candidate) =>
    (prefill.holdingAccountID === undefined || candidate.account_id === prefill.holdingAccountID) &&
    (prefill.commodityID === undefined || candidate.commodity_id === prefill.commodityID) &&
    (prefill.currencyID === undefined || candidate.cost_commodity_id === undefined ||
      candidate.cost_commodity_id === prefill.currencyID));
  const keys = [...new Set(matching.map(keyOf))];
  return keys.length === 1 ? keys[0] : '';
}

/**
 * The position an earlier activity chose, in a picker's key shape: holding,
 * instrument and cost currency (`withCurrency`), or holding and instrument.
 * Undefined unless the selection is restored and fully known.
 */
export function restoredPositionKey(prefill: InvestmentPrefill | undefined, withCurrency: boolean): string | undefined {
  if (!prefill?.restoredSelection || prefill.holdingAccountID === undefined || prefill.commodityID === undefined) return undefined;
  if (!withCurrency) return `${prefill.holdingAccountID}:${prefill.commodityID}`;
  return prefill.currencyID === undefined ? undefined : `${prefill.holdingAccountID}:${prefill.commodityID}:${prefill.currencyID}`;
}
