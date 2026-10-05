import { describe, expect, it } from 'vitest';
import { transactionTitle, type SystemLabel } from './transaction-title';

const label = (code: SystemLabel) => `label:${code}`;

describe('transactionTitle', () => {
  it('prefers the payee, then the description', () => {
    expect(transactionTitle({ payee_name: 'Shop', description: 'Lunch' }, label, '—')).toBe('Shop');
    expect(transactionTitle({ payee_name: '', description: 'Lunch' }, label, '—')).toBe('Lunch');
  });

  it('localizes a system label when the journal has no user text', () => {
    expect(transactionTitle({ description: '', system_label: 'split_adjustment' }, label, '—'))
      .toBe('label:split_adjustment');
    expect(transactionTitle({ description: '', system_label: 'transfer_bridge' }, label, '—'))
      .toBe('label:transfer_bridge');
  });

  it('keeps user text ahead of a system label', () => {
    expect(transactionTitle({ description: 'Note', system_label: 'split_adjustment' }, label, '—')).toBe('Note');
  });

  it('falls back when nothing names the transaction', () => {
    expect(transactionTitle({ payee_name: null, description: '' }, label, '—')).toBe('—');
  });
});
