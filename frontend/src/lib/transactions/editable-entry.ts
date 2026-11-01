import { coefficientSign } from '#lib/money/amount.ts';
import type { components } from '#lib/api/schema.js';
import type { TransactionRequest } from '#lib/api/transactions.ts';

/**
 * The split editor's journal (#481). The server restates a saved transaction
 * as the one journal entry that rebuilds it exactly (`editable_entry`); the
 * editor shows its postings and sends them back with what it does not show
 * carried unchanged: a category's own day, posting tags and metadata, the
 * entry's kind, memo and metadata, and a transfer's recorded choices. A
 * transaction without one is edited only in what it describes
 * (`keep_journal`), never restated by the editor.
 */

export type EditableEntry = components['schemas']['EditableEntry'];
type JournalEntryRequest = NonNullable<TransactionRequest['journal_entries']>[number];
type PostingRequest = NonNullable<JournalEntryRequest['postings']>[number];
export type TransferSideName = 'sending' | 'receiving';

/** What the split editor carries for the entry and never shows. */
export type SplitEntryCarry = {
  entryKind?: JournalEntryRequest['entry_kind'];
  memo?: string;
  metadata?: Record<string, unknown>;
  conversionSide?: TransferSideName;
  feePaidBy?: TransferSideName;
};

/** One posting as the split editor sends it: what the form holds, plus the
 *  carried fields. `postedOn` is an account posting's day ('' = the
 *  transaction date); `followsDay` is the day a category was restated on. */
export type SplitPosting = {
  accountID: number;
  commodityID: number;
  value: string;
  scale: number;
  memo: string;
  lineKey?: string;
  /** Whether the account has a posting day of its own (an asset or liability). */
  isAccount: boolean;
  postedOn: string;
  followsDay?: string;
  tagIDs?: number[];
  metadata?: Record<string, unknown>;
};

export function splitEntryCarry(entry: EditableEntry): SplitEntryCarry {
  return {
    entryKind: entry.entry_kind,
    memo: entry.memo,
    metadata: entry.metadata,
    conversionSide: entry.conversion_side,
    feePaidBy: entry.fee_paid_by
  };
}

const hasKeys = (value: Record<string, unknown> | undefined) => !!value && Object.keys(value).length > 0;

/** Whether the simple form, which carries none of this, shows the entry
 *  without losing anything: one account and one category, no memo, tag,
 *  metadata or category day, an ordinary entry. */
export function simpleFormKeeps(entry: EditableEntry, isAccount: (accountID: number) => boolean): boolean {
  const accounts = entry.postings.filter((posting) => isAccount(posting.account_id));
  if (entry.postings.length !== 2 || accounts.length !== 1) return false;
  if (entry.entry_kind !== 'ordinary' || entry.memo || hasKeys(entry.metadata) || entry.conversion_side || entry.fee_paid_by) return false;
  return entry.postings.every(
    (posting) =>
      !posting.memo &&
      !posting.tag_ids.length &&
      !hasKeys(posting.metadata) &&
      (isAccount(posting.account_id) || !posting.posted_on)
  );
}

/**
 * The journal entry the split editor sends. Carried fields go back as they
 * came while they still fit what the owner changed: a category keeps its own
 * day while an account posting in its currency still posts that day; a fee
 * payer stays while the split is still one amount out, one in and one
 * positive fee in the payer's currency (the server then dates the fee); a
 * conversion side stays while the split is still an exchange.
 */
export function splitEntryRequest(postings: SplitPosting[], carry: SplitEntryCarry, transactionDate: string, hasSystemAccount: boolean): JournalEntryRequest {
  const accountDay = (posting: SplitPosting) => posting.postedOn || transactionDate;
  const accounts = postings.filter((posting) => posting.isAccount);
  const others = postings.filter((posting) => !posting.isAccount);

  let feePaidBy: TransferSideName | undefined;
  if (carry.feePaidBy && accounts.length === 2 && others.length === 1) {
    const [first, second] = accounts;
    const signs = [coefficientSign(first.value), coefficientSign(second.value)];
    if (signs[0] * signs[1] < 0) {
      const sending = signs[0] < 0 ? first : second;
      const payer = carry.feePaidBy === 'sending' ? sending : sending === first ? second : first;
      const fee = others[0];
      if (coefficientSign(fee.value) > 0 && fee.commodityID === payer.commodityID) feePaidBy = carry.feePaidBy;
    }
  }

  const commodities = new Set(postings.map((posting) => posting.commodityID));
  const exchange = commodities.size > 1 && !hasSystemAccount;
  const entryKind = exchange ? 'exchange' : carry.entryKind !== 'exchange' ? carry.entryKind : undefined;

  const requestPostings: PostingRequest[] = postings.map((posting) => {
    let postedOn: string | undefined;
    if (posting.isAccount) {
      postedOn = posting.postedOn && posting.postedOn !== transactionDate ? posting.postedOn : undefined;
    } else if (!feePaidBy && posting.followsDay) {
      const stillPosts = accounts.some((account) => account.commodityID === posting.commodityID && accountDay(account) === posting.followsDay);
      postedOn = stillPosts ? posting.followsDay : undefined;
    }
    return {
      line_key: posting.lineKey,
      account_id: posting.accountID,
      posted_on: postedOn,
      commodity_id: posting.commodityID,
      quantity_value: posting.value,
      quantity_scale: posting.scale,
      memo: posting.memo,
      metadata: posting.metadata,
      tag_ids: posting.tagIDs
    };
  });

  const entry: JournalEntryRequest = {
    entry_date: transactionDate,
    entry_kind: entryKind,
    memo: carry.memo || undefined,
    metadata: hasKeys(carry.metadata) ? carry.metadata : undefined,
    postings: requestPostings
  };
  if (exchange && carry.conversionSide) entry.conversion_side = carry.conversionSide;
  if (feePaidBy) entry.fee_paid_by = feePaidBy;
  return entry;
}
