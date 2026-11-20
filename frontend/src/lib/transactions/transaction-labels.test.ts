import { describe, expect, it } from 'vitest';
import { builtinCategoryLabel } from '#lib/categories/category-labels.ts';
import { m } from '#lib/paraglide/messages.js';
import { resolveAccountLabel, systemRoleLabel, type PostingResponse } from './transaction-labels.ts';

const posting = {
  id: 1, posting_line_id: 1, line_key: 'a', line_seq: 1, account_id: 9, account_class: 'asset', commodity_id: 1, commodity_code: 'USD',
  quantity_value: '100', quantity_scale: 2, memo: '', reconciliation_status: 'uncleared', metadata: {}, tag_ids: [],
  account_name: '', account_code: 'cash:USD', account_system_role: null, account_builtin_key: null,
  account_starter_key: 'cash', account_currency_code: 'USD'
} as PostingResponse;

describe('resolveAccountLabel', () => {
  it('labels a posting on the unnamed starter cash account as Ledger does (#597 sweep)', () => {
    expect(resolveAccountLabel(posting)).toBe(m.accounts_starter_cash_name({ currency: 'USD' }));
  });

  it('prefers the system role, then the built-in category, then the name', () => {
    expect(resolveAccountLabel({ ...posting, account_system_role: 'transfer_clearing' })).toBe(systemRoleLabel('transfer_clearing'));
    expect(resolveAccountLabel({ ...posting, account_starter_key: null, account_builtin_key: 'expense.food.groceries' }))
      .toBe(builtinCategoryLabel('expense.food.groceries'));
    expect(resolveAccountLabel({ ...posting, account_name: 'Wallet' })).toBe('Wallet');
  });

  it('falls back to the code, then the account number', () => {
    expect(resolveAccountLabel({ ...posting, account_starter_key: null, account_currency_code: null })).toBe('cash:USD');
    expect(resolveAccountLabel({ ...posting, account_name: null, account_code: null, account_starter_key: null })).toBe(m.accounts_unnamed_account({ id: 9 }));
  });
});
