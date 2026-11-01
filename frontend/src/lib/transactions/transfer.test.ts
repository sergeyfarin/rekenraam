import { describe, expect, it } from 'vitest';
import { transferEntry, transferFields, transferProblem, type TransferFields } from './transfer';

const account = (id: number, value: string, commodity = 1, scale = 2) => ({
  account_id: id, account_class: 'asset', commodity_id: commodity, quantity_value: value, quantity_scale: scale, line_key: `l${id}`
});
const fee = (value: string, commodity = 1) => ({
  account_id: 9, account_class: 'expense', commodity_id: commodity, quantity_value: value, quantity_scale: 2, line_key: 'lfee'
});
const trading = (value: string, commodity: number) => ({
  account_id: 99, account_class: 'equity', account_system_role: 'commodity_trading', commodity_id: commodity, quantity_value: value, quantity_scale: 2
});
const clearing = (value: string, commodity: number) => ({
  account_id: 98, account_class: 'asset', account_system_role: 'transfer_clearing', commodity_id: commodity, quantity_value: value, quantity_scale: 2
});

describe('transferFields', () => {
  it('reads a same-day transfer in one currency', () => {
    const fields = transferFields([{ entry_date: '2026-06-01', postings: [account(1, '-10000'), account(2, '10000')] }], '2026-06-01');
    expect(fields).toEqual({
      from: { accountID: 1, commodityID: 1, postedOn: '', lineKey: 'l1' },
      to: { accountID: 2, commodityID: 1, postedOn: '', lineKey: 'l2' },
      sent: { value: '10000', scale: 2 },
      received: { value: '10000', scale: 2 },
      fee: undefined
    });
  });

  it('keeps each side on its own day and drops the bridge', () => {
    const fields = transferFields([
      { entry_date: '2026-06-01', postings: [account(1, '-10000'), clearing('10000', 1)] },
      { entry_date: '2026-06-03', postings: [account(2, '10000'), clearing('-10000', 1)] }
    ], '2026-06-01');
    expect(fields?.from.postedOn).toBe('');
    expect(fields?.to.postedOn).toBe('2026-06-03');
  });

  it('reads an exchange with a fee on top, without its trading legs', () => {
    const fields = transferFields([{ entry_date: '2026-06-01', entry_kind: 'exchange', postings: [
      account(1, '-10250', 1), account(2, '10800', 2), fee('250', 1), trading('10000', 1), trading('-10800', 2)
    ] }], '2026-06-01');
    expect(fields?.sent).toEqual({ value: '10000', scale: 2 });
    expect(fields?.received).toEqual({ value: '10800', scale: 2 });
    expect(fields?.fee).toEqual({ accountID: 9, amount: { value: '250', scale: 2 }, paidBy: 'from', lineKey: 'lfee' });
  });

  it('reads a fee the arriving side pays', () => {
    const fields = transferFields([{ entry_date: '2026-06-01', entry_kind: 'exchange', postings: [
      account(1, '-10000', 1), account(2, '10700', 2), fee('100', 2), trading('10000', 1), trading('-10800', 2)
    ] }], '2026-06-01');
    expect(fields?.received).toEqual({ value: '10800', scale: 2 });
    expect(fields?.fee?.paidBy).toBe('to');
  });

  it('refuses anything else', () => {
    const one = (postings: object[]) => transferFields([{ entry_date: '2026-06-01', postings: postings as never }], '2026-06-01');
    expect(one([account(1, '-100'), fee('100')])).toBeNull();
    expect(one([account(1, '-100'), account(2, '50'), account(3, '50')])).toBeNull();
    expect(one([account(1, '-100'), account(2, '90'), fee('5'), fee('5')])).toBeNull();
    expect(one([account(1, '-100'), account(2, '100'), { ...fee('-1'), account_class: 'income' }])).toBeNull();
    expect(one([account(1, '-100'), account(2, '-100')])).toBeNull();
    expect(one([account(1, '-100'), account(2, '100'), trading('1', 1)])).toBeNull();
  });
});

describe('transferEntry', () => {
  const base: TransferFields = {
    from: { accountID: 1, commodityID: 1, postedOn: '' },
    to: { accountID: 2, commodityID: 1, postedOn: '' },
    sent: { value: '10000', scale: 2 },
    received: { value: '10000', scale: 2 }
  };

  it('sends one currency as an ordinary entry, fee on top of From', () => {
    expect(transferEntry({ ...base, fee: { accountID: 9, amount: { value: '25', scale: 1 }, paidBy: 'to' } })).toEqual({
      postings: [
        { line_key: undefined, account_id: 1, commodity_id: 1, quantity_value: '-10250', quantity_scale: 2 },
        { line_key: undefined, account_id: 2, commodity_id: 1, quantity_value: '10000', quantity_scale: 2 },
        { line_key: undefined, account_id: 9, commodity_id: 1, quantity_value: '25', quantity_scale: 1 }
      ]
    });
  });

  it('sends two currencies as an exchange with both amounts exact', () => {
    const entry = transferEntry({ ...base, to: { accountID: 2, commodityID: 2, postedOn: '' }, received: { value: '108', scale: 0 } });
    expect(entry.entry_kind).toBe('exchange');
    expect(entry.postings.map((p) => [p.quantity_value, p.quantity_scale])).toEqual([['-10000', 2], ['108', 0]]);
  });

  it('takes a fee the arriving side pays out of what arrived', () => {
    const entry = transferEntry({ ...base, to: { accountID: 2, commodityID: 2, postedOn: '' }, received: { value: '10800', scale: 2 }, fee: { accountID: 9, amount: { value: '100', scale: 2 }, paidBy: 'to' } });
    expect(entry.postings.map((p) => [p.account_id, p.commodity_id, p.quantity_value])).toEqual([[1, 1, '-10000'], [2, 2, '10700'], [9, 2, '100']]);
  });

  it('round-trips through transferFields', () => {
    const fields: TransferFields = { ...base, to: { accountID: 2, commodityID: 2, postedOn: '' }, received: { value: '10800', scale: 2 }, fee: { accountID: 9, amount: { value: '250', scale: 2 }, paidBy: 'from' } };
    const entry = transferEntry(fields);
    const loaded = transferFields([{ entry_date: '2026-06-01', entry_kind: entry.entry_kind, postings: entry.postings.map((p) => ({ ...p, account_class: p.account_id === 9 ? 'expense' : 'asset' })) }], '2026-06-01');
    expect(loaded?.sent).toEqual(fields.sent);
    expect(loaded?.received).toEqual(fields.received);
    expect(loaded?.fee?.amount).toEqual(fields.fee?.amount);
  });
});

describe('transferProblem', () => {
  const ok = { fromAccountID: 1, toAccountID: 2, sent: { value: '100', scale: 2 }, received: null, exchange: false, leavesOn: '2026-06-01', arrivesOn: '2026-06-02', fee: null, feePaidBy: 'from' as const };
  it('names each refusal before any write', () => {
    expect(transferProblem(ok)).toBeNull();
    expect(transferProblem({ ...ok, toAccountID: undefined })).toBe('missing_account');
    expect(transferProblem({ ...ok, toAccountID: 1 })).toBe('same_account');
    expect(transferProblem({ ...ok, sent: { value: '0', scale: 2 } })).toBe('missing_amount');
    expect(transferProblem({ ...ok, exchange: true })).toBe('missing_received');
    expect(transferProblem({ ...ok, arrivesOn: '2026-05-31' })).toBe('arrives_before_leaves');
    expect(transferProblem({ ...ok, fee: { value: '-1', scale: 2 } })).toBe('invalid_fee');
    expect(transferProblem({ ...ok, exchange: true, received: { value: '100', scale: 2 }, fee: { value: '100', scale: 2 }, feePaidBy: 'to' })).toBe('fee_exceeds_amount');
  });
});
