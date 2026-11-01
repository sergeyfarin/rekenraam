import { describe, expect, it } from 'vitest';
import {
  choiceKey,
  initialMatchChoice,
  keepAsNew,
  linkChoice,
  matchDecisionsPending,
  undecidedMatchRows,
  type ImportMatchCandidate,
  type ImportRowMatchReview
} from './match-review.ts';

function candidate(transactionID: number, postedOn = '2026-06-10'): ImportMatchCandidate {
  return {
    transaction_id: transactionID, line_key: `line-${transactionID}`, transaction_date: '2026-06-08',
    transaction_kind: 'transfer', posted_on: postedOn, day_offset: 0, account_id: 2, commodity_id: 1,
    quantity_value: '10000', quantity_scale: 2, payee_name: '', description: 'To savings'
  };
}

function review(rowID: number, candidates: ImportMatchCandidate[], choice: ImportRowMatchReview['choice'] = null, needsDecision = true): ImportRowMatchReview {
  return { row_id: rowID, row_index: rowID - 1, candidates, choice, needs_decision: needsDecision, window_days: 5 };
}

describe('import match review (#485)', () => {
  it('offers the only candidate and never preselects among several', () => {
    expect(initialMatchChoice(review(1, [candidate(7)]))).toEqual(linkChoice(candidate(7)));
    expect(initialMatchChoice(review(1, [candidate(7), candidate(8)]))).toBeUndefined();
  });

  it('keeps a standing decision, but not one the server no longer accepts', () => {
    expect(initialMatchChoice(review(1, [candidate(7), candidate(8)], linkChoice(candidate(8)), false))).toEqual(linkChoice(candidate(8)));
    expect(initialMatchChoice(review(1, [candidate(7)], keepAsNew, false))).toEqual(keepAsNew);
    // The posting moved to another day: the old link is offered again only as the current candidate.
    const moved = review(1, [candidate(7, '2026-06-11')], linkChoice(candidate(7)), true);
    expect(initialMatchChoice(moved)).toEqual(linkChoice(candidate(7, '2026-06-11')));
  });

  it('counts a row undecided until its choice is one preview offers', () => {
    const reviews = [review(1, [candidate(7), candidate(8)]), review(2, [candidate(9)])];
    const choices = new Map([[2, linkChoice(candidate(9))]]);
    expect(undecidedMatchRows(reviews, choices)).toEqual([1]);
    choices.set(1, keepAsNew);
    expect(undecidedMatchRows(reviews, choices)).toEqual([]);
    choices.set(2, linkChoice(candidate(9, '2026-06-12')));
    expect(undecidedMatchRows(reviews, choices)).toEqual([2]);
  });

  it('holds commit while the server still needs a decision', () => {
    expect(matchDecisionsPending([review(1, [candidate(7)])])).toBe(true);
    expect(matchDecisionsPending([review(1, [candidate(7)], linkChoice(candidate(7)), false)])).toBe(false);
    expect(matchDecisionsPending([])).toBe(false);
  });

  it('keys choices by line and reviewed day', () => {
    expect(choiceKey(linkChoice(candidate(7)))).toBe('link:7:line-7:2026-06-10');
    expect(choiceKey(keepAsNew)).toBe('new');
    expect(choiceKey(undefined)).toBe('');
  });
});
