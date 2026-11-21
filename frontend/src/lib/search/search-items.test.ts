import { describe, expect, it } from 'vitest';
import type { SearchCatalogItem } from '#lib/api/search.ts';
import { builtinCategoryLabel } from '#lib/categories/category-labels.ts';
import { m } from '#lib/paraglide/messages.js';
import { searchItems } from './search-items.ts';

const base: SearchCatalogItem = { kind: 'account', id: 9, name: '', code: 'cash:USD', builtin_key: '', starter_key: 'cash', currency_code: 'USD',
  status: 'active', account_kind: 'cash' };

function title(item: SearchCatalogItem): string | undefined {
  return searchItems([item], false).find((result) => result.id === `${item.kind}:${item.id}`)?.title;
}

describe('search catalog titles', () => {
  it('titles the unnamed starter cash account as Ledger does (#597 sweep)', () => {
    expect(title(base)).toBe(m.accounts_starter_cash_name({ currency: 'USD' }));
  });

  it('keeps account names and codes, category keys and instrument names', () => {
    expect(title({ ...base, name: 'Wallet' })).toBe('Wallet');
    expect(title({ ...base, starter_key: null, currency_code: null, code: 'CHK' })).toBe('CHK');
    expect(title({ ...base, kind: 'category', id: 3, code: 'expense_food_groceries', builtin_key: 'expense.food.groceries', starter_key: null, currency_code: null }))
      .toBe(builtinCategoryLabel('expense.food.groceries'));
    expect(title({ ...base, kind: 'instrument', id: 5, name: 'Acme', code: 'ACME USD', starter_key: null, currency_code: null })).toBe('Acme');
  });
});
