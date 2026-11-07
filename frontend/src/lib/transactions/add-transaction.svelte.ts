import type { TransactionResponse } from '#lib/api/transactions.ts';
import type { EntryDefaults } from './entry-mode.ts';

export type InvestmentSaveKind = 'activity' | 'write-off';

export type { EntryDefaults } from './entry-mode.ts';

// New transaction from anywhere (#459, #477, #506). A page that shows *New
// transaction* in its header (Transactions, an account register, a category,
// an instrument) provides its own opener, so the N shortcut, search's
// commands and Home's Transfer open the same entry in its side panel,
// prefilled for that page. Elsewhere, Home's tile included, they open the
// shared entry dialog the shell mounts once.
let pageAdd = $state<((defaults?: EntryDefaults) => void) | undefined>();

/** The shared entry dialog: open with its prefill, and the last transaction it saved. */
export const transactionEntry = $state<{ open: boolean; defaults?: EntryDefaults; saved?: TransactionResponse; savedInvestment?: InvestmentSaveKind }>({ open: false });

/**
 * Provide the current page's opener; the returned function withdraws it. The
 * opener receives the caller's tab, if any, to apply over the page's own
 * prefill.
 */
export function providePageAdd(open: (defaults?: EntryDefaults) => void): () => void {
  pageAdd = open;
  return () => {
    if (pageAdd === open) pageAdd = undefined;
  };
}

export function addTransaction(defaults?: EntryDefaults): void {
  if (pageAdd) {
    pageAdd(defaults);
    return;
  }
  // Never replace an open working copy and its unsaved input.
  if (transactionEntry.open) return;
  transactionEntry.open = true;
  transactionEntry.defaults = defaults;
  transactionEntry.saved = undefined;
  transactionEntry.savedInvestment = undefined;
}

/** New transaction on its Transfer tab (#481), the account as From when given. */
export function addTransfer(accountID?: number): void {
  addTransaction({ accountID, tab: 'transfer' });
}

/** Cancel discards the working copy; saving closes it with the saved transaction. */
export function closeTransactionEntry(saved?: TransactionResponse, investment?: InvestmentSaveKind): void {
  transactionEntry.open = false;
  transactionEntry.defaults = undefined;
  transactionEntry.saved = saved;
  transactionEntry.savedInvestment = investment;
}
