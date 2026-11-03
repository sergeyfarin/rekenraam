import type { components } from '#lib/api/schema.js';

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
 * The choice preview offers for a row: a decision the server still accepts,
 * otherwise the only candidate when there is one. With several candidates
 * the owner picks; nothing is preselected.
 */
export function initialMatchChoice(review: ImportRowMatchReview): ImportMatchChoice | undefined {
  if (review.choice && !review.needs_decision) return review.choice;
  if (review.candidates.length === 1) return linkChoice(review.candidates[0]);
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
