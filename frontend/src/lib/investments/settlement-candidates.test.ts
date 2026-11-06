import { describe, expect, it } from 'vitest';
import type { InvestmentEntryAccount } from '#lib/api/investment-entry.ts';
import { settlementCandidates, suggestedSettlement } from './settlement-candidates.ts';

function account(id: number, kind: string, parent?: number, eligible = false, liability = false): InvestmentEntryAccount {
  return { id, account_kind: kind, parent_account_id: parent, investment_settlement: eligible,
    investment_holding: kind === 'security_holding', account_class: liability ? 'liability' : 'asset',
    default_commodity_id: 10 } as InvestmentEntryAccount;
}
const brokerage = account(1, 'brokerage');
const holding = account(2, 'security_holding', 1);
const cash = account(3, 'brokerage_cash', 1, true);
const bank = account(4, 'checking', undefined, true);
const other = account(5, 'brokerage_cash', 99, true);

describe('explicit settlement candidates', () => {
  it('requires choice with no candidates and never infers an institution or import mapping', () => {
    expect(settlementCandidates([holding, bank, other], 2)).toEqual([]);
    expect(suggestedSettlement('', false, [])).toBe('');
  });
  it('suggests a sole direct brokerage cash child and names its id', () => {
    const rows = settlementCandidates([brokerage, holding, cash, other], 2);
    expect(rows).toEqual([cash]); expect(suggestedSettlement('', false, rows)).toBe('3');
  });
  it('starts from a brokerage container before a holding has been chosen', () => {
    expect(settlementCandidates([brokerage, cash, other], undefined, 1)).toEqual([cash]);
  });
  it('keeps a starting bank as the sole suggestion outside a brokerage', () => {
    expect(suggestedSettlement('', false, settlementCandidates([bank, other], undefined, 4))).toBe('4');
  });
  it('deduplicates the starting cash account when it is also a child', () => {
    expect(settlementCandidates([brokerage, holding, cash], 2, 3)).toEqual([cash]);
  });
  it('requires choice between a valid bank and a distinct brokerage child', () => {
    const rows = settlementCandidates([brokerage, holding, cash, bank], 2, 4);
    expect(rows).toEqual([cash, bank]); expect(suggestedSettlement('4', false, rows)).toBe('');
  });
  it('requires choice between multiple children', () => {
    expect(suggestedSettlement('', false, [cash, account(6, 'brokerage_cash', 1, true)])).toBe('');
  });
  it('never suggests a sole liability', () => {
    expect(suggestedSettlement('', false, [account(6, 'margin', undefined, true, true)])).toBe('');
  });
  it('filters backend role/date/currency refusals before counting candidates', () => {
    const invalid = { ...cash, investment_settlement: false };
    expect(settlementCandidates([brokerage, holding, invalid], 2)).toEqual([]);
    expect(settlementCandidates([brokerage, holding, cash], 2, undefined, 99)).toEqual([]);
  });
  it('preserves an explicit choice across refresh, a new candidate, and invalidation', () => {
    for (const rows of [[], [cash], [cash, bank]]) expect(suggestedSettlement('4', true, rows)).toBe('4');
    expect(suggestedSettlement('', true, [cash])).toBe('');
  });
  it('does not walk through arbitrary nested groups or unrelated cash siblings', () => {
    const group = account(7, 'other_asset', 1);
    expect(settlementCandidates([brokerage, group, { ...holding, parent_account_id: 7 }, cash], 2)).toEqual([]);
  });
});
