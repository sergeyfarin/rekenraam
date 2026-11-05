import { isSubledgerManagedAccountKind } from '#lib/accounts/subledger-accounts.ts';
import type { InvestmentPrefill } from '#lib/investments/investment-prefill.ts';
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
