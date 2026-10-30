import { goto } from '$app/navigation';

// Add transaction from anywhere (#459). A page that shows *Add transaction* in
// its header (Transactions, an account register, a category) provides its own
// opener, so the N shortcut and search's Add command open the same editor,
// prefilled for that page. Elsewhere they open the Transactions editor.
let pageAdd = $state<(() => void) | undefined>();

/** Provide the current page's opener; the returned function withdraws it. */
export function providePageAdd(open: () => void): () => void {
  pageAdd = open;
  return () => {
    if (pageAdd === open) pageAdd = undefined;
  };
}

export async function addTransaction(): Promise<void> {
  if (pageAdd) {
    pageAdd();
    return;
  }
  // Transactions consumes the trigger, so refresh or Back does not reopen a
  // dismissed working copy.
  await goto('/app/ledger/transactions?create=transaction');
}
