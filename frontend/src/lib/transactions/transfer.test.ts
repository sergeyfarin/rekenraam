import { describe, expect, it } from 'vitest';
import { convertedAmounts, sameCurrencyAmount, sameCurrencyStatements, transferEntry, transferFields, transferProblem, type TransferFields } from './transfer';

const side = (account_id: number, commodity_id: number, quantity_value: string, posted_on: string) => ({
  account_id, commodity_id, quantity_value, quantity_scale: 2, posted_on, line_key: `l${account_id}`, memo: `memo ${account_id}`
});

describe('transferFields', () => {
  it('maps the server shape, keeping memos and line keys, days relative to the date', () => {
    const fields = transferFields({
      transaction_date: '2026-06-01',
      transfer: {
        sending: side(1, 1, '-10000', '2026-06-01'),
        receiving: side(2, 2, '10700', '2026-06-03'),
        fee: { account_id: 9, commodity_id: 2, quantity_value: '100', quantity_scale: 2, paid_by: 'receiving', line_key: 'lfee', memo: '' },
        conversion_side: 'sending',
        entry_memo: 'wire'
      }
    });
    expect(fields).toEqual({
      from: { accountID: 1, commodityID: 1, postedOn: '', lineKey: 'l1', memo: 'memo 1' },
      to: { accountID: 2, commodityID: 2, postedOn: '2026-06-03', lineKey: 'l2', memo: 'memo 2' },
      sent: { value: '10000', scale: 2 },
      received: { value: '10700', scale: 2 },
      fee: { accountID: 9, amount: { value: '100', scale: 2 }, paidBy: 'receiving', lineKey: 'lfee', memo: '' },
      conversionSide: 'sending',
      entryMemo: 'wire'
    });
  });

  it('is null without a server shape', () => {
    expect(transferFields({ transaction_date: '2026-06-01', transfer: undefined })).toBeNull();
  });
});

describe('amounts', () => {
  it('takes the fee back out of the side that paid it to show what converted', () => {
    expect(convertedAmounts({ sent: { value: '10000', scale: 2 }, received: { value: '10700', scale: 2 }, fee: { amount: { value: '100', scale: 2 }, paidBy: 'receiving' } }))
      .toEqual({ sent: { value: '10000', scale: 2 }, received: { value: '10800', scale: 2 } });
    expect(convertedAmounts({ sent: { value: '10250', scale: 2 }, received: { value: '10800', scale: 2 }, fee: { amount: { value: '25', scale: 1 }, paidBy: 'sending' } }))
      .toEqual({ sent: { value: '10000', scale: 2 }, received: { value: '10800', scale: 2 } });
    expect(convertedAmounts({ sent: { value: '100', scale: 2 }, received: { value: '100', scale: 2 }, fee: { amount: { value: '100', scale: 2 }, paidBy: 'sending' } })).toBeNull();
  });

  it('puts a one-currency fee on the paying statement, and reads the amount back', () => {
    const amount = { value: '5000', scale: 2 };
    expect(sameCurrencyStatements(amount, { amount: { value: '150', scale: 2 }, paidBy: 'sending' })).toEqual({ sent: { value: '5150', scale: 2 }, received: amount });
    expect(sameCurrencyStatements(amount, { amount: { value: '150', scale: 2 }, paidBy: 'receiving' })).toEqual({ sent: amount, received: { value: '4850', scale: 2 } });
    expect(sameCurrencyAmount({ sent: { value: '5150', scale: 2 }, fee: { accountID: 9, amount: { value: '150', scale: 2 }, paidBy: 'sending' } })).toEqual(amount);
    expect(sameCurrencyAmount({ sent: amount, fee: { accountID: 9, amount: { value: '150', scale: 2 }, paidBy: 'receiving' } })).toEqual(amount);
  });
});

describe('transferEntry', () => {
  const base: TransferFields = {
    from: { accountID: 1, commodityID: 1, postedOn: '', lineKey: 'a', memo: 'out' },
    to: { accountID: 2, commodityID: 2, postedOn: '2026-06-03', lineKey: 'b', memo: 'in' },
    sent: { value: '10000', scale: 2 },
    received: { value: '10700', scale: 2 },
    fee: { accountID: 9, amount: { value: '100', scale: 2 }, paidBy: 'receiving', lineKey: 'c', memo: '' },
    conversionSide: 'receiving',
    entryMemo: 'wire'
  };

  it('sends statement amounts, the fee on its payer\'s day, memos and the conversion side', () => {
    expect(transferEntry(base, '2026-06-01')).toEqual({
      entry_kind: 'exchange',
      conversion_side: 'receiving',
      memo: 'wire',
      postings: [
        { line_key: 'a', account_id: 1, commodity_id: 1, quantity_value: '-10000', quantity_scale: 2, memo: 'out', posted_on: undefined },
        { line_key: 'b', account_id: 2, commodity_id: 2, quantity_value: '10700', quantity_scale: 2, memo: 'in', posted_on: '2026-06-03' },
        { line_key: 'c', account_id: 9, commodity_id: 2, quantity_value: '100', quantity_scale: 2, memo: '', posted_on: '2026-06-03' }
      ]
    });
  });

  it('sends one currency as an ordinary entry, the conversion side left out', () => {
    const entry = transferEntry({ ...base, to: { ...base.to, commodityID: 1 }, fee: undefined, conversionSide: undefined }, '2026-06-01');
    expect(entry.entry_kind).toBeUndefined();
    expect(entry.conversion_side).toBeUndefined();
  });
});

describe('transferProblem', () => {
  const ok = { fromAccountID: 1, toAccountID: 2, sent: { value: '100', scale: 2 }, received: null, exchange: false, leavesOn: '2026-06-01', arrivesOn: '2026-06-02', fee: null, feePaidBy: 'sending' as const };
  it('names each refusal before any write', () => {
    expect(transferProblem(ok)).toBeNull();
    expect(transferProblem({ ...ok, toAccountID: undefined })).toBe('missing_account');
    expect(transferProblem({ ...ok, toAccountID: 1 })).toBe('same_account');
    expect(transferProblem({ ...ok, sent: { value: '0', scale: 2 } })).toBe('missing_amount');
    expect(transferProblem({ ...ok, exchange: true })).toBe('missing_received');
    expect(transferProblem({ ...ok, arrivesOn: '2026-05-31' })).toBe('arrives_before_leaves');
    expect(transferProblem({ ...ok, fee: { value: '-1', scale: 2 } })).toBe('invalid_fee');
    expect(transferProblem({ ...ok, fee: { value: '100', scale: 2 }, feePaidBy: 'receiving' })).toBe('fee_exceeds_amount');
    expect(transferProblem({ ...ok, exchange: true, received: { value: '100', scale: 2 }, fee: { value: '100', scale: 2 }, feePaidBy: 'sending' })).toBe('fee_exceeds_amount');
  });
});
