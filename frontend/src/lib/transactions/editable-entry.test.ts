import { describe, expect, it } from 'vitest';
import { simpleFormKeeps, splitEntryCarry, splitEntryRequest, type EditableEntry, type SplitPosting } from './editable-entry';

const posting = (account_id: number, quantity_value: string, extra: Partial<EditableEntry['postings'][number]> = {}) => ({
  line_key: `l${account_id}`, account_id, commodity_id: 1, quantity_value, quantity_scale: 2, memo: '', metadata: {}, tag_ids: [], ...extra
});
const entry = (postings: EditableEntry['postings'], extra: Partial<EditableEntry> = {}): EditableEntry => ({ entry_kind: 'ordinary', memo: '', metadata: {}, postings, ...extra });
const accounts = new Set([1, 2]);
const isAccount = (id: number) => accounts.has(id);

const split = (accountID: number, value: string, extra: Partial<SplitPosting> = {}): SplitPosting => ({
  accountID, commodityID: 1, value, scale: 2, memo: '', lineKey: `l${accountID}`, isAccount: accounts.has(accountID), postedOn: '', ...extra
});

describe('simpleFormKeeps', () => {
  it('admits one account and one category with nothing the simple form drops', () => {
    expect(simpleFormKeeps(entry([posting(1, '-500'), posting(9, '500')]), isAccount)).toBe(true);
  });
  it('declines a memo, a tag, a category day or a non-ordinary entry', () => {
    expect(simpleFormKeeps(entry([posting(1, '-500', { memo: 'ref' }), posting(9, '500')]), isAccount)).toBe(false);
    expect(simpleFormKeeps(entry([posting(1, '-500'), posting(9, '500', { tag_ids: [3] })]), isAccount)).toBe(false);
    expect(simpleFormKeeps(entry([posting(1, '-500', { posted_on: '2026-06-02' }), posting(9, '500', { posted_on: '2026-06-01' })]), isAccount)).toBe(false);
    expect(simpleFormKeeps(entry([posting(1, '-500'), posting(9, '500')], { entry_kind: 'transfer_leg' }), isAccount)).toBe(false);
  });
});

describe('splitEntryRequest', () => {
  it('sends what it carries back unchanged', () => {
    const carry = splitEntryCarry(entry([], { entry_kind: 'transfer_leg', memo: 'wire', metadata: { source: 'x' }, fee_paid_by: 'sending' }));
    const request = splitEntryRequest([
      split(1, '-10100', { postedOn: '2026-06-02', memo: 'sender reference' }),
      split(2, '10000', { postedOn: '2026-06-03' }),
      split(9, '100', { followsDay: '2026-06-02', tagIDs: [4], metadata: { k: 1 } })
    ], carry, '2026-06-01', false);
    expect(request).toEqual({
      entry_date: '2026-06-01', entry_kind: 'transfer_leg', memo: 'wire', metadata: { source: 'x' }, fee_paid_by: 'sending',
      postings: [
        { line_key: 'l1', account_id: 1, posted_on: '2026-06-02', commodity_id: 1, quantity_value: '-10100', quantity_scale: 2, memo: 'sender reference', metadata: undefined, tag_ids: undefined },
        { line_key: 'l2', account_id: 2, posted_on: '2026-06-03', commodity_id: 1, quantity_value: '10000', quantity_scale: 2, memo: '', metadata: undefined, tag_ids: undefined },
        // With a payer the server dates the fee, so a moved payer day moves it.
        { line_key: 'l9', account_id: 9, posted_on: undefined, commodity_id: 1, quantity_value: '100', quantity_scale: 2, memo: '', metadata: { k: 1 }, tag_ids: [4] }
      ]
    });
  });

  it('keeps a category day only while an account in its currency still posts that day', () => {
    const legs = [split(1, '-10100', { postedOn: '2026-06-02' }), split(2, '10000', { postedOn: '2026-06-03' }), split(9, '100', { followsDay: '2026-06-02' })];
    expect(splitEntryRequest(legs, {}, '2026-06-01', false).postings![2].posted_on).toBe('2026-06-02');
    const moved = [{ ...legs[0], postedOn: '2026-06-04' }, legs[1], legs[2]];
    expect(splitEntryRequest(moved, {}, '2026-06-01', false).postings![2].posted_on).toBeUndefined();
  });

  it('drops a fee payer and a conversion side the split no longer fits', () => {
    const carry = { feePaidBy: 'receiving' as const, conversionSide: 'receiving' as const };
    const refund = splitEntryRequest([split(1, '-10000'), split(2, '11000'), split(9, '-1000')], carry, '2026-06-01', false);
    expect(refund.fee_paid_by).toBeUndefined();
    expect(refund.conversion_side).toBeUndefined();
    const exchange = splitEntryRequest([split(1, '-10000'), split(2, '10800', { commodityID: 2 })], carry, '2026-06-01', false);
    expect(exchange.entry_kind).toBe('exchange');
    expect(exchange.conversion_side).toBe('receiving');
  });
});
