/**
 * Posting dates (#187). A transaction has one display date; each account
 * posting may post on another day, which the account's running balance and
 * reconciliation use. The editor sends that day as `posted_on` on the account
 * posting and the server builds the dated journal entries, bridging them
 * through transfer_clearing. This module is the editor's side of that
 * contract: which postings have a posting day, how a loaded journal maps back
 * to editor fields, and what to send.
 */

type PostingAccount = {
  account_class?: string | null;
  account_system_role?: string | null;
  system_role?: string | null;
};

/** Whether a posting has its own posting day: a user's asset or liability,
 *  which a statement lists. Mirrors the server's rule. */
export function hasPostingDay(account: PostingAccount | undefined): boolean {
  if (!account || account.account_system_role || account.system_role) return false;
  return account.account_class === 'asset' || account.account_class === 'liability';
}

type LoadedEntry<P> = { entry_date: string; postings?: P[] };

/**
 * The postings the editor shows for a saved journal, each with its posted-on
 * field: '' when it posts on the transaction date (the field follows the
 * date), otherwise its entry date. transfer_clearing legs of a journal with
 * several entries are the server's bridge and are left out; the server
 * rebuilds them on save. An exchange's commodity_trading legs stay: a
 * journal Transfer entry cannot restate (#481) is edited as it is.
 */
export function editablePostings<P extends PostingAccount>(
  entries: LoadedEntry<P>[],
  transactionDate: string
): { posting: P; postedOn: string }[] {
  const bridged = entries.length > 1;
  return entries.flatMap((entry) =>
    (entry.postings ?? [])
      .filter((posting) => !(bridged && posting.account_system_role === 'transfer_clearing'))
      .map((posting) => ({
        posting,
        postedOn: hasPostingDay(posting) && entry.entry_date !== transactionDate ? entry.entry_date : ''
      }))
  );
}

/** The `posted_on` to send for a posting: only a day that differs from the
 *  transaction date, and only on an account posting. */
export function postedOnForRequest(
  postedOn: string,
  transactionDate: string,
  account: PostingAccount | undefined
): string | undefined {
  if (!hasPostingDay(account)) return undefined;
  const day = postedOn.trim();
  return day && day !== transactionDate ? day : undefined;
}

/** The posting days of a transaction that differ from its transaction date,
 *  in date order: what a list shows beside a row dated by the transaction. */
export function otherPostingDays(transaction: {
  transaction_date: string;
  journal_entries?: { entry_date: string }[];
}): string[] {
  const days = new Set(
    (transaction.journal_entries ?? [])
      .map((entry) => entry.entry_date)
      .filter((day) => day !== transaction.transaction_date)
  );
  return [...days].sort();
}
