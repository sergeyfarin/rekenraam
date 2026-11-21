import { describe, expect, it } from 'vitest';
import { builtinCategoryLabel } from '#lib/categories/category-labels.ts';
import { m } from '#lib/paraglide/messages.js';
import { accountDisplayName, accountLabelByID } from './account-labels.ts';

// The unnamed starter cash account as the accounts API returns it.
const starter = { id: 9, name: '', code: 'cash:USD', metadata: { source: 'starter_account_seed', starter_key: 'cash', currency_code: 'USD' } };
const cashUSD = m.accounts_starter_cash_name({ currency: 'USD' });

describe('accountDisplayName', () => {
  it('labels the unnamed starter cash account by its localized name, never its code', () => {
    expect(cashUSD).toBe('Cash USD');
    expect(accountDisplayName(starter)).toBe(cashUSD);
  });

  it('labels an unnamed starter category in a picker by its built-in label, and a renamed one by its name', () => {
    const dividends = { id: 12, name: '', code: 'income_dividends',
      metadata: { category: { type: 'income', is_starter: true, builtin_key: 'income.dividends' } } };
    expect(builtinCategoryLabel('income.dividends')).toBe('Dividends');
    expect(accountDisplayName(dividends)).toBe(builtinCategoryLabel('income.dividends'));
    expect(accountDisplayName({ ...dividends, name: 'Payouts' })).toBe('Payouts');
  });

  it('keeps an owner-given name, then the code, then the account number', () => {
    expect(accountDisplayName({ ...starter, name: 'Wallet' })).toBe('Wallet');
    expect(accountDisplayName({ id: 4, name: '', code: 'HOLD' })).toBe('HOLD');
    expect(accountDisplayName({ id: 4 })).toBe(m.accounts_unnamed_account({ id: 4 }));
  });
});

describe('accountLabelByID', () => {
  const accounts = [{ id: 1, name: 'Checking', code: 'CHK' }, starter];

  it('labels a listed account by number or form value', () => {
    expect(accountLabelByID(accounts, 9)).toBe(cashUSD);
    expect(accountLabelByID(accounts, '9')).toBe(cashUSD);
    expect(accountLabelByID(accounts, 1)).toBe('Checking');
  });

  it('leaves an unlisted or missing account to the screen', () => {
    expect(accountLabelByID(accounts, 2)).toBeUndefined();
    expect(accountLabelByID(undefined, 9)).toBeUndefined();
    expect(accountLabelByID(accounts, undefined)).toBeUndefined();
    expect(accountLabelByID(accounts, '')).toBeUndefined();
  });
});
