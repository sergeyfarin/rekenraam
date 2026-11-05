import type { components } from '#lib/api/schema.js';
import { m } from '#lib/paraglide/messages.js';

export type ImportMatchChoice = components['schemas']['ImportMatchChoice'];
export type ImportMatchCandidate = components['schemas']['ImportMatchCandidate'];
export type ImportRowMatchReview = components['schemas']['ImportRowMatchReview'];

// #485: a CSV or QIF row that matches a posting already in the ledger is a
// suggestion. The server's preview lists the candidates; the owner links the
// row to one or keeps it as new, and the choice travels in the row's
// resolution. Nothing here decides whether a posting matches.

export const keepAsNew: ImportMatchChoice = { decision: 'new' };

export function linkChoice(candidate: ImportMatchCandidate): ImportMatchChoice {
  return {
    decision: 'link',
    transaction_id: candidate.transaction_id,
    line_key: candidate.line_key,
    posted_on: candidate.posted_on
  };
}

/** A stable key for a choice, for radio values and comparisons. */
export function choiceKey(choice: ImportMatchChoice | null | undefined): string {
  if (!choice) return '';
  if (choice.decision === 'new') return 'new';
  return `link:${choice.transaction_id}:${choice.line_key}:${choice.posted_on}`;
}

/**
 * The choice preview shows for a row: a decision the server still accepts.
 * Nothing is preselected, not even a single candidate: an exact amount in
 * the window does not establish that a row is that transaction (repeated
 * equal payments are common), so the owner links each row explicitly
 * (#485 review).
 */
export function initialMatchChoice(review: ImportRowMatchReview): ImportMatchChoice | undefined {
  if (review.choice && !review.needs_decision) return review.choice;
  return undefined;
}

/** Whether a choice is one this preview still offers for the row. */
export function choiceOffered(review: ImportRowMatchReview, choice: ImportMatchChoice | undefined): boolean {
  if (!choice) return false;
  if (choice.decision === 'new') return true;
  return review.candidates.some((candidate) => choiceKey(linkChoice(candidate)) === choiceKey(choice));
}

/** Rows the owner still has to decide before commit can settle them. */
export function undecidedMatchRows(reviews: readonly ImportRowMatchReview[], choices: ReadonlyMap<number, ImportMatchChoice>): number[] {
  return reviews.filter((review) => !choiceOffered(review, choices.get(review.row_id))).map((review) => review.row_id);
}

/**
 * Whether commit would hold a row. Preview runs after the page saved its
 * choices, so a row the server still needs a decision on was sent none, or
 * one that no longer stands.
 */
export function matchDecisionsPending(reviews: readonly ImportRowMatchReview[]): boolean {
  return reviews.some((review) => review.needs_decision);
}

/**
 * Why a candidate matched, given its exact amount already formatted: the
 * same account and currency, that amount, and its posting day against the
 * statement date.
 */
export function matchReasonText(candidate: ImportMatchCandidate, amount: string, locale: string): string {
  const days = Math.abs(candidate.day_offset);
  const n = new Intl.NumberFormat(locale).format(days);
  if (candidate.day_offset > 0) return m.import_match_reason_after({ amount, count: days, n });
  if (candidate.day_offset < 0) return m.import_match_reason_before({ amount, count: days, n });
  return m.import_match_reason_same_day({ amount });
}

/** A candidate's radio label, given its transaction date already formatted. */
export function matchCandidateLabel(candidate: ImportMatchCandidate, date: string): string {
  return m.import_match_link_option({
    description: candidate.description || candidate.payee_name || m.import_match_untitled(),
    date
  });
}
