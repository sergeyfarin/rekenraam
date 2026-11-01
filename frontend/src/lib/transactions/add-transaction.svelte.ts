import type { TransactionResponse } from '#lib/api/transactions.ts';

// Add transaction from anywhere (#459, #477). A page that shows *Add
// transaction* in its header (Transactions, an account register, a category)
// provides its own opener, so the N shortcut and search's Add command open the
// same editor, prefilled for that page. Elsewhere, Home's tile included, they
// open the shared entry dialog the shell mounts once.
let pageAdd = $state<(() => void) | undefined>();

export type EntryDefaults = { accountID?: number; categoryID?: number; transfer?: boolean };

/** The shared entry dialog: open with its prefill, and the last transaction it saved. */
export const transactionEntry = $state<{ open: boolean; defaults?: EntryDefaults; saved?: TransactionResponse }>({ open: false });

/** Provide the current page's opener; the returned function withdraws it. */
export function providePageAdd(open: () => void): () => void {
  pageAdd = open;
  return () => {
    if (pageAdd === open) pageAdd = undefined;
  };
}

export function addTransaction(defaults?: EntryDefaults): void {
  if (pageAdd) {
    pageAdd();
    return;
  }
  // Never replace an open working copy and its unsaved input.
  if (transactionEntry.open) return;
  transactionEntry.open = true;
  transactionEntry.defaults = defaults;
  transactionEntry.saved = undefined;
}

/**
 * Transfer between accounts (#481): the shared dialog in Transfer mode, the
 * account as From when given. It opens the shared dialog even on a page with
 * its own editor, which has no transfer opener of its own.
 */
export function addTransfer(accountID?: number): void {
  if (transactionEntry.open) return;
  transactionEntry.open = true;
  transactionEntry.defaults = { accountID, transfer: true };
  transactionEntry.saved = undefined;
}

/** Cancel discards the working copy; saving closes it with the saved transaction. */
export function closeTransactionEntry(saved?: TransactionResponse): void {
  transactionEntry.open = false;
  transactionEntry.defaults = undefined;
  transactionEntry.saved = saved;
}
