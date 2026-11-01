import { addScaledAmounts, coefficientSign, negateCoefficient, type ScaledAmount } from '#lib/money/amount.ts';
import { editablePostings, hasPostingDay } from './posted-on';

/**
 * Transfer entry (#481). A transfer moves money between two of the owner's
 * accounts: the amount that left one and the amount that arrived in the other,
 * each posting on its own day, in one currency or two. A fee comes with it but
 * is not part of it: it is its own expense posting (Bank fees by default),
 * charged on top. When the two sides are in different currencies the request
 * entry is an `exchange`, and the server adds the commodity_trading legs; the
 * client never builds them, and never converts one amount into the other.
 *
 * This module is the one rule for which saved transactions are transfers (so
 * they reopen in Transfer mode) and for the postings a transfer sends.
 */

/** The built-in key of the category a transfer fee is recorded on. */
export const transferFeeCategoryKey = 'expense.financial.bank_fees';

type LoadedPosting = {
  account_id: number;
  account_class?: string | null;
  account_system_role?: string | null;
  commodity_id: number;
  quantity_value: string;
  quantity_scale: number;
  line_key?: string;
};

type LoadedEntry = { entry_date: string; entry_kind?: string; postings?: LoadedPosting[] };

export type TransferSide = { accountID: number; commodityID: number; postedOn: string; lineKey?: string };

export type TransferFields = {
  from: TransferSide;
  to: TransferSide;
  /** What left From, without the fee; a positive magnitude. */
  sent: ScaledAmount;
  /** What arrived in To before a fee To pays; equals sent in one currency. */
  received: ScaledAmount;
  fee?: { accountID: number; amount: ScaledAmount; paidBy: 'from' | 'to'; lineKey?: string };
};

const magnitude = (posting: LoadedPosting): ScaledAmount => ({
  value: coefficientSign(posting.quantity_value) < 0 ? negateCoefficient(posting.quantity_value) : posting.quantity_value,
  scale: posting.quantity_scale
});

const negated = (amount: ScaledAmount): ScaledAmount => ({ value: negateCoefficient(amount.value), scale: amount.scale });

const side = (posting: LoadedPosting, postedOn: string): TransferSide => ({
  accountID: posting.account_id,
  commodityID: posting.commodity_id,
  postedOn,
  lineKey: posting.line_key
});

/**
 * The transfer a saved journal is, or null when it is anything else: two
 * account postings — one out, one in — in one currency with opposite amounts,
 * or in two currencies as an exchange, plus at most one expense posting in
 * one side's currency for the fee. Each side keeps its posting day.
 */
export function transferFields(entries: LoadedEntry[], transactionDate: string): TransferFields | null {
  const loaded = editablePostings<LoadedPosting>(entries, transactionDate);
  if (loaded.some(({ posting }) => posting.account_system_role)) return null;
  const accounts = loaded.filter(({ posting }) => hasPostingDay(posting));
  const others = loaded.filter(({ posting }) => !hasPostingDay(posting));
  if (accounts.length !== 2 || others.length > 1) return null;

  const out = accounts.find(({ posting }) => coefficientSign(posting.quantity_value) < 0);
  const into = accounts.find(({ posting }) => coefficientSign(posting.quantity_value) > 0);
  if (!out || !into) return null;
  const from = side(out.posting, out.postedOn);
  const to = side(into.posting, into.postedOn);
  const exchange = from.commodityID !== to.commodityID;

  const feePosting = others[0]?.posting;
  let fee: TransferFields['fee'];
  if (feePosting) {
    if (feePosting.account_class !== 'expense' || coefficientSign(feePosting.quantity_value) <= 0) return null;
    // In one currency the fee is shown as paid by From: the journal is the
    // same either way.
    const paidBy = feePosting.commodity_id === from.commodityID ? 'from' : feePosting.commodity_id === to.commodityID ? 'to' : null;
    if (!paidBy) return null;
    fee = { accountID: feePosting.account_id, amount: magnitude(feePosting), paidBy, lineKey: feePosting.line_key };
  }

  const fromAmount = magnitude(out.posting);
  const toAmount = magnitude(into.posting);
  const sent = fee?.paidBy === 'from' ? addScaledAmounts(fromAmount, negated(fee.amount)) : fromAmount;
  const received = fee?.paidBy === 'to' ? addScaledAmounts(toAmount, fee.amount) : toAmount;
  if (coefficientSign(sent.value) <= 0 || coefficientSign(received.value) <= 0) return null;
  if (!exchange && addScaledAmounts(sent, negated(received)).value !== '0') return null;
  return { from, to, sent, received, fee };
}

export type TransferPosting = {
  line_key?: string;
  account_id: number;
  commodity_id: number;
  quantity_value: string;
  quantity_scale: number;
  posted_on?: string;
};

/**
 * The journal entry a transfer sends. In one currency `received` is ignored:
 * what left is what arrived. The fee is charged on top of the paying side.
 * Posting days are the caller's to set (`postedOnForRequest`).
 */
export function transferEntry(fields: TransferFields): { entry_kind?: 'exchange'; postings: TransferPosting[] } {
  const exchange = fields.from.commodityID !== fields.to.commodityID;
  const received = exchange ? fields.received : fields.sent;
  const feeFrom = fields.fee && (fields.fee.paidBy === 'from' || !exchange) ? fields.fee.amount : undefined;
  const feeTo = fields.fee && fields.fee.paidBy === 'to' && exchange ? fields.fee.amount : undefined;

  const out = feeFrom ? addScaledAmounts(fields.sent, feeFrom) : fields.sent;
  const into = feeTo ? addScaledAmounts(received, negated(feeTo)) : received;
  const postings: TransferPosting[] = [
    { line_key: fields.from.lineKey, account_id: fields.from.accountID, commodity_id: fields.from.commodityID, quantity_value: negateCoefficient(out.value), quantity_scale: out.scale },
    { line_key: fields.to.lineKey, account_id: fields.to.accountID, commodity_id: fields.to.commodityID, quantity_value: into.value, quantity_scale: into.scale }
  ];
  if (fields.fee) {
    postings.push({
      line_key: fields.fee.lineKey,
      account_id: fields.fee.accountID,
      commodity_id: feeTo ? fields.to.commodityID : fields.from.commodityID,
      quantity_value: fields.fee.amount.value,
      quantity_scale: fields.fee.amount.scale
    });
  }
  return exchange ? { entry_kind: 'exchange', postings } : { postings };
}

export type TransferProblem =
  | 'missing_account'
  | 'same_account'
  | 'missing_amount'
  | 'missing_received'
  | 'arrives_before_leaves'
  | 'invalid_fee'
  | 'fee_exceeds_amount';

/** Why a transfer cannot be saved yet, checked before any write. */
export function transferProblem(input: {
  fromAccountID?: number;
  toAccountID?: number;
  sent: ScaledAmount | null;
  received: ScaledAmount | null;
  exchange: boolean;
  leavesOn: string;
  arrivesOn: string;
  fee: ScaledAmount | null;
  feePaidBy: 'from' | 'to';
}): TransferProblem | null {
  if (!input.fromAccountID || !input.toAccountID) return 'missing_account';
  if (input.fromAccountID === input.toAccountID) return 'same_account';
  if (!input.sent || coefficientSign(input.sent.value) <= 0) return 'missing_amount';
  if (input.exchange && (!input.received || coefficientSign(input.received.value) <= 0)) return 'missing_received';
  if (input.leavesOn && input.arrivesOn && input.arrivesOn < input.leavesOn) return 'arrives_before_leaves';
  if (input.fee && coefficientSign(input.fee.value) <= 0) return 'invalid_fee';
  if (input.fee && input.exchange && input.feePaidBy === 'to' && input.received) {
    if (coefficientSign(addScaledAmounts(input.received, negated(input.fee)).value) <= 0) return 'fee_exceeds_amount';
  }
  return null;
}
