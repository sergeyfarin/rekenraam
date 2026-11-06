import { isSubledgerManagedAccountKind } from '#lib/accounts/subledger-accounts.ts';
import type { InvestmentPrefill } from '#lib/investments/investment-prefill.ts';
import { addScaledAmounts, coefficientSign, parseDecimalAmount, type ScaledAmount } from '#lib/money/amount.ts';
import { isOrdinaryEntryAccount } from './entry-account.ts';

/**
 * The New transaction workflow's four stable tabs (#506, design "Transaction
 * entry"). Every entry shows them in this order, whatever the account, so a
 * tab never moves when the account changes; only its label follows the
 * account.
 */
export const entryTabs = ['out', 'in', 'transfer', 'investment'] as const;
export type EntryTab = (typeof entryTabs)[number];

/**
 * What the page a New transaction starts from already knows. It applies once,
 * when the entry opens, and never overwrites a later choice; the account
 * stays editable until save.
 */
export type EntryDefaults = {
  accountID?: number;
  categoryID?: number;
  tab?: EntryTab;
  /** The holding or instrument the Investment tab's forms start on (#186). */
  investment?: InvestmentPrefill;
};

/** Money out of or into the entry's account. */
export type CashDirection = 'out' | 'in';

/** The cash tabs' shape: one account and one category, or split lines. */
export type CashShape = 'simple' | 'split';

export type EntryTabLabel = 'withdrawal' | 'deposit' | 'charge' | 'credit' | 'transfer' | 'investment';

type ContextAccount = {
  account_class: string;
  account_kind?: string | null;
  status: string;
  is_system?: boolean | null;
  allows_postings: boolean;
};

export function isCreditCard(account: { account_kind?: string | null } | undefined): boolean {
  return account?.account_kind === 'credit_card';
}

/**
 * A tab's label for the entry's account. Charge and Credit are card words
 * only; loans and mortgages keep Withdrawal and Deposit, and paying a card
 * from another account is a Transfer.
 */
export function entryTabLabel(tab: EntryTab, account?: { account_kind?: string | null }): EntryTabLabel {
  if (tab === 'out') return isCreditCard(account) ? 'charge' : 'withdrawal';
  if (tab === 'in') return isCreditCard(account) ? 'credit' : 'deposit';
  return tab;
}

/**
 * Investment is muted, never disabled, when the entry starts from an ordinary
 * cash account outside a brokerage: a bank account can pay for an investment,
 * but the holding is still missing. Muted only changes how the tab looks
 * while it is not selected.
 */
export function investmentMuted(account: ContextAccount | undefined): boolean {
  return !!account && isOrdinaryEntryAccount(account) && account.account_kind !== 'brokerage_cash';
}

/** The tab an entry opens on: the caller's choice, else Investment for a holding, else money out. */
export function startingTab(start: { tab?: EntryTab; account?: { account_kind?: string | null } }): EntryTab {
  if (start.tab) return start.tab;
  if (isSubledgerManagedAccountKind(start.account?.account_kind)) return 'investment';
  return 'out';
}

export function directionForTab(tab: EntryTab): CashDirection | undefined {
  return tab === 'out' || tab === 'in' ? tab : undefined;
}

/**
 * The editor shape a tab shows. The cash tabs share one working copy, simple
 * or split, so leaving for Transfer and coming back restores the shape the
 * owner was using; Investment keeps the editor's work while its own panel
 * shows.
 */
export function shapeForTab(tab: EntryTab, cashShape: CashShape): CashShape | 'transfer' | 'investment' {
  if (tab === 'transfer' || tab === 'investment') return tab;
  return cashShape;
}

/**
 * The account posting's ledger coefficient for a positive magnitude. Money
 * out of an account is a credit and money in a debit, for every class: a
 * credit raises what a liability owes, which is a card charge or a loan
 * drawdown, and a debit lowers it, which is a refund or a repayment. This is
 * the same ledger result the signed amount produced before #506 (asset:
 * inflow positive; liability: the typed sign flipped).
 */
export function accountLedgerValue(direction: CashDirection, magnitude: string): string {
  if (/^0+$/.test(magnitude)) return '0';
  return direction === 'out' ? `-${magnitude}` : magnitude;
}

/** A stored account posting as a direction and a positive magnitude. Zero reads as money out. */
export function directionOf(ledgerValue: string): { direction: CashDirection; magnitude: string } {
  return ledgerValue.startsWith('-')
    ? { direction: 'out', magnitude: ledgerValue.slice(1) }
    : { direction: /^0+$/.test(ledgerValue) ? 'out' : 'in', magnitude: ledgerValue };
}

/**
 * Why an account cannot take a cash tab's posting, or undefined when it can.
 * The selection stays as it is, with this explanation, until the owner
 * chooses another account: nothing is substituted (journey 6).
 */
export function cashAccountProblem(account: ContextAccount | undefined): 'holding' | 'not_ordinary' | undefined {
  if (!account || isOrdinaryEntryAccount(account)) return undefined;
  return isSubledgerManagedAccountKind(account.account_kind) ? 'holding' : 'not_ordinary';
}

/**
 * What an approval authorizes: the exact request it reviewed. Any later
 * change to the entry (tab, account, direction, amount) yields another key, so
 * a review shown before the change never authorizes the request after it.
 */
export function approvalKey(request: unknown): string {
  return JSON.stringify(request);
}

export function approvalStillValid(reviewedKey: string | undefined, current: unknown): boolean {
  return reviewedKey !== undefined && reviewedKey === approvalKey(current);
}

/** A split leg as the entry-mode rules read it: its account and parsed amount. */
export type SplitLegAmount = { accountID: string; amount: ScaledAmount | null };

/**
 * Which way a split moves the entry's account: the net of its legs on that
 * account. Undefined when the split has no leg on it, one of them does not
 * parse, or they net to zero; then no tab is contradicted (#506 review).
 */
export function splitDirection(legs: SplitLegAmount[], accountID: number | undefined): CashDirection | undefined {
  if (accountID === undefined) return undefined;
  let net: ScaledAmount = { value: '0', scale: 0 };
  let found = false;
  for (const leg of legs) {
    if (leg.accountID !== String(accountID)) continue;
    if (!leg.amount) return undefined;
    net = addScaledAmounts(net, leg.amount);
    found = true;
  }
  const sign = found ? coefficientSign(net.value) : 0;
  return sign === 0 ? undefined : sign < 0 ? 'out' : 'in';
}

/**
 * The split after the entry's account changed from `from` to `to` on another
 * tab: legs on `from` post to `to`, and legs already on `to` post to `from`,
 * so a split between the two accounts keeps its shape (#506 review). A moved
 * leg takes its new account's currency when that account has one.
 */
export function rebindSplitAccount<L extends { accountID: string; commodityID: string }>(
  legs: L[],
  from: number,
  to: number,
  commodityOf: (accountID: string) => string
): L[] {
  const [a, b] = [String(from), String(to)];
  return legs.map((leg) => {
    if (leg.accountID !== a && leg.accountID !== b) return leg;
    const accountID = leg.accountID === a ? b : a;
    return { ...leg, accountID, commodityID: commodityOf(accountID) || leg.commodityID };
  });
}

/** A split line as the currency rule reads it: its account and stored currency ('' when unset). */
export type SplitLegCurrency = { accountID: string; commodityID: string };

/**
 * Each split line's currency. A line keeps the currency it carries; otherwise
 * it takes its account's own. A line on an account with none of its own (a
 * category, a clearing account) follows the split's account lines when they
 * all share one currency, and has none ('') when they do not: then the owner
 * chooses it on the line (#511).
 *
 * The account lines are the legs `isAccountLine` names (the owner's asset and
 * liability accounts, never a category), each counted by the currency it
 * carries or its account's own — a currency chosen on a receivable's line
 * counts like a checking account's default (#512 review). A line with neither
 * yet counts for nothing; it follows like any other.
 */
export function splitLegCommodities(
  legs: SplitLegCurrency[],
  ownCommodityOf: (accountID: string) => string,
  isAccountLine: (accountID: string) => boolean
): string[] {
  const shared = new Set<string>();
  for (const leg of legs) {
    if (leg.accountID === '' || !isAccountLine(leg.accountID)) continue;
    const currency = leg.commodityID || ownCommodityOf(leg.accountID);
    if (currency) shared.add(currency);
  }
  const follow = shared.size === 1 ? [...shared][0] : '';
  return legs.map((leg) => {
    if (leg.commodityID) return leg.commodityID;
    if (leg.accountID === '') return '';
    return ownCommodityOf(leg.accountID) || follow;
  });
}

/** A split line with its currency resolved and its amount as typed. */
export type SplitLineInput = { accountID: string; commodityID: string; amountStr: string };

/** Why a filled split line cannot be saved; `line` counts from 1. */
export type SplitLineProblem = { line: number; missing: 'account' | 'amount' | 'currency' | 'number' };

/**
 * The first split line the save would otherwise have to drop or guess at. A
 * line is filled once it names an account or an amount, and a filled line
 * saves whole or the save stops; blank lines are ignored (#511).
 */
export function splitLineProblem(legs: SplitLineInput[]): SplitLineProblem | null {
  for (const [i, leg] of legs.entries()) {
    const amount = leg.amountStr.trim();
    if (leg.accountID === '' && amount === '') continue;
    const line = i + 1;
    if (leg.accountID === '') return { line, missing: 'account' };
    if (amount === '') return { line, missing: 'amount' };
    if (parseDecimalAmount(amount) === null) return { line, missing: 'number' };
    if (leg.commodityID === '') return { line, missing: 'currency' };
  }
  return null;
}
