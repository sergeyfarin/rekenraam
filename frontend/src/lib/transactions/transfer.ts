import { addScaledAmounts, coefficientSign, negateCoefficient, type ScaledAmount } from '#lib/money/amount.ts';
import type { TransactionResponse } from '#lib/api/transactions.ts';

/**
 * Transfer entry (#481). A transfer moves money between two of the owner's
 * accounts: what left one and what reached the other, as each statement shows
 * it, each posting on its own day, in one currency or two. A fee comes with it
 * but is not part of it: it is its own expense posting (Bank fees by default),
 * charged on top by the account that paid it and posted on that account's day.
 * When the sides are in different currencies the request entry is an
 * `exchange`: the server adds the commodity_trading legs on the day of the
 * side that converted, and the client never converts one amount into the other.
 *
 * Which saved transactions are transfers is the server's rule: a response
 * carries `transfer` only when rebuilding the journal reproduces it exactly.
 * This module maps that shape to the form and the form back to a request.
 */

/** The built-in key of the category a transfer fee is recorded on. */
export const transferFeeCategoryKey = 'expense.financial.bank_fees';

export type TransferSideName = 'sending' | 'receiving';

export type TransferSide = { accountID: number; commodityID: number; postedOn: string; lineKey?: string; memo?: string };

export type TransferFields = {
  from: TransferSide;
  to: TransferSide;
  /** What left From, as its statement shows it (fee included); a magnitude. */
  sent: ScaledAmount;
  /** What reached To, as its statement shows it (after a fee To paid). */
  received: ScaledAmount;
  fee?: { accountID: number; amount: ScaledAmount; paidBy: TransferSideName; lineKey?: string; memo?: string };
  /** Which side converted; only for two currencies. */
  conversionSide?: TransferSideName;
  entryMemo?: string;
  /** The saved entries' kind, sent back so an edit keeps it; absent for a
   *  new transfer. */
  entryKind?: TransferRequestEntry['entry_kind'];
};

const magnitude = (value: string, scale: number): ScaledAmount => ({
  value: coefficientSign(value) < 0 ? negateCoefficient(value) : value,
  scale
});

const negated = (amount: ScaledAmount): ScaledAmount => ({ value: negateCoefficient(amount.value), scale: amount.scale });

/** The form's fields for a saved transfer, or null when it is not one. Each
 *  side's day is '' when it posts on the transaction date. */
export function transferFields(transaction: Pick<TransactionResponse, 'transfer' | 'transaction_date'>): TransferFields | null {
  const shape = transaction.transfer;
  if (!shape) return null;
  // The form's fee is a cost charged on top. The server never offers a
  // refund line as one; a form that read one would turn it into its opposite.
  if (shape.fee && coefficientSign(shape.fee.quantity_value) <= 0) return null;
  const day = (postedOn: string) => (postedOn === transaction.transaction_date ? '' : postedOn);
  const side = (s: typeof shape.sending): TransferSide => ({
    accountID: s.account_id, commodityID: s.commodity_id, postedOn: day(s.posted_on), lineKey: s.line_key, memo: s.memo
  });
  return {
    from: side(shape.sending),
    to: side(shape.receiving),
    sent: magnitude(shape.sending.quantity_value, shape.sending.quantity_scale),
    received: magnitude(shape.receiving.quantity_value, shape.receiving.quantity_scale),
    fee: shape.fee
      ? {
          accountID: shape.fee.account_id,
          amount: magnitude(shape.fee.quantity_value, shape.fee.quantity_scale),
          paidBy: shape.fee.paid_by,
          lineKey: shape.fee.line_key,
          memo: shape.fee.memo
        }
      : undefined,
    conversionSide: shape.conversion_side,
    entryMemo: shape.entry_memo,
    entryKind: shape.entry_kind
  };
}

/** What converted, for two currencies: the statement amounts with the fee
 *  taken back out of the side that paid it. Null when either is not positive. */
export function convertedAmounts(fields: {
  sent: ScaledAmount;
  received: ScaledAmount;
  fee?: { amount: ScaledAmount; paidBy: TransferSideName };
}): { sent: ScaledAmount; received: ScaledAmount } | null {
  const sent = fields.fee?.paidBy === 'sending' ? addScaledAmounts(fields.sent, negated(fields.fee.amount)) : fields.sent;
  const received = fields.fee?.paidBy === 'receiving' ? addScaledAmounts(fields.received, fields.fee.amount) : fields.received;
  if (coefficientSign(sent.value) <= 0 || coefficientSign(received.value) <= 0) return null;
  return { sent, received };
}

/** In one currency the form asks for the amount transferred; the statements
 *  show it with the fee added on the paying side. */
export function sameCurrencyStatements(amount: ScaledAmount, fee?: { amount: ScaledAmount; paidBy: TransferSideName }): { sent: ScaledAmount; received: ScaledAmount } {
  if (!fee) return { sent: amount, received: amount };
  return fee.paidBy === 'sending'
    ? { sent: addScaledAmounts(amount, fee.amount), received: amount }
    : { sent: amount, received: addScaledAmounts(amount, negated(fee.amount)) };
}

/** The amount transferred in one currency, from the statement amounts. */
export function sameCurrencyAmount(fields: Pick<TransferFields, 'sent' | 'fee'>): ScaledAmount {
  return fields.fee?.paidBy === 'sending' ? addScaledAmounts(fields.sent, negated(fields.fee.amount)) : fields.sent;
}

export type TransferPosting = {
  line_key?: string;
  account_id: number;
  commodity_id: number;
  quantity_value: string;
  quantity_scale: number;
  memo: string;
  posted_on?: string;
};

export type TransferRequestEntry = {
  entry_kind?: 'ordinary' | 'transfer_leg' | 'exchange' | 'investment' | 'opening_balance' | 'adjustment';
  conversion_side?: TransferSideName;
  fee_paid_by?: TransferSideName;
  memo?: string;
  postings: TransferPosting[];
};

/**
 * The journal entry a transfer sends. Days are each side's posting day ('' =
 * the transaction date); the fee posts on its payer's day, and only a day that
 * differs from the transaction date is sent. The payer and the converting side
 * are sent as choices the server stores, so they survive a day on which both
 * sides post. Memos, line keys and the saved kind travel back unchanged, so an
 * unchanged save keeps them.
 */
export function transferEntry(fields: TransferFields, transactionDate: string): TransferRequestEntry {
  const exchange = fields.from.commodityID !== fields.to.commodityID;
  const sendDay = (day: string) => (day && day !== transactionDate ? day : undefined);
  const postings: TransferPosting[] = [
    { line_key: fields.from.lineKey, account_id: fields.from.accountID, commodity_id: fields.from.commodityID, quantity_value: negateCoefficient(fields.sent.value), quantity_scale: fields.sent.scale, memo: fields.from.memo ?? '', posted_on: sendDay(fields.from.postedOn) },
    { line_key: fields.to.lineKey, account_id: fields.to.accountID, commodity_id: fields.to.commodityID, quantity_value: fields.received.value, quantity_scale: fields.received.scale, memo: fields.to.memo ?? '', posted_on: sendDay(fields.to.postedOn) }
  ];
  if (fields.fee) {
    const payer = fields.fee.paidBy === 'sending' ? fields.from : fields.to;
    postings.push({
      line_key: fields.fee.lineKey,
      account_id: fields.fee.accountID,
      commodity_id: payer.commodityID,
      quantity_value: fields.fee.amount.value,
      quantity_scale: fields.fee.amount.scale,
      memo: fields.fee.memo ?? '',
      posted_on: sendDay(payer.postedOn)
    });
  }
  const entry: TransferRequestEntry = { memo: fields.entryMemo || undefined, postings };
  if (fields.fee) entry.fee_paid_by = fields.fee.paidBy;
  if (exchange) {
    entry.entry_kind = 'exchange';
    entry.conversion_side = fields.conversionSide ?? 'sending';
  } else if (fields.entryKind && fields.entryKind !== 'exchange') {
    entry.entry_kind = fields.entryKind;
  }
  return entry;
}

export type TransferProblem =
  | 'missing_account'
  | 'same_account'
  | 'missing_amount'
  | 'missing_received'
  | 'arrives_before_leaves'
  | 'invalid_fee'
  | 'fee_exceeds_amount';

/** Why a transfer cannot be saved yet, checked before any write. Amounts are
 *  what the form holds: in one currency `sent` is the amount transferred; in
 *  two, `sent` and `received` are the statement amounts. */
export function transferProblem(input: {
  fromAccountID?: number;
  toAccountID?: number;
  sent: ScaledAmount | null;
  received: ScaledAmount | null;
  exchange: boolean;
  leavesOn: string;
  arrivesOn: string;
  fee: ScaledAmount | null;
  feePaidBy: TransferSideName;
}): TransferProblem | null {
  if (!input.fromAccountID || !input.toAccountID) return 'missing_account';
  if (input.fromAccountID === input.toAccountID) return 'same_account';
  if (!input.sent || coefficientSign(input.sent.value) <= 0) return 'missing_amount';
  if (input.exchange && (!input.received || coefficientSign(input.received.value) <= 0)) return 'missing_received';
  if (input.leavesOn && input.arrivesOn && input.arrivesOn < input.leavesOn) return 'arrives_before_leaves';
  if (input.fee && coefficientSign(input.fee.value) <= 0) return 'invalid_fee';
  if (input.fee) {
    const fee = { amount: input.fee, paidBy: input.feePaidBy };
    const left = input.exchange
      ? convertedAmounts({ sent: input.sent, received: input.received!, fee })
      : sameCurrencyStatements(input.sent, fee);
    if (!left || coefficientSign(left.sent.value) <= 0 || coefficientSign(left.received.value) <= 0) return 'fee_exceeds_amount';
  }
  return null;
}
