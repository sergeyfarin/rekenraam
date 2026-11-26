import { describe, expect, it } from 'vitest';
import type { HomeLayout } from '#lib/api/home.ts';
import { home } from './home-fixture.ts';
import { sameFigureScopes } from './layout-model.ts';

function configuredLayout(): HomeLayout {
  const layout = home().personalization.layout;
  layout.account_id = 21;
  layout.account_set = { name: 'Cash accounts', account_ids: [21, 22] };
  layout.widget_options.spending = { account_ids: [23] };
  return layout;
}

describe('sameFigureScopes', () => {
  it('reuses figures across presentation changes and a switch to an already valued headline', () => {
    const before = configuredLayout();
    const after = structuredClone(before);
    after.headline = 'account_set';
    after.account_set!.name = 'Everyday accounts';
    after.shortcuts.reverse();
    after.widgets.reverse();
    after.widget_options.spending!.size = 'wide';
    after.widget_options.cashflow = { period: 'this_month' };
    after.show_review = !after.show_review;
    expect(sameFigureScopes(before, after)).toBe(true);
  });

  it.each<[string, (layout: HomeLayout) => void]>([
    ['a different headline account', layout => { layout.account_id = 22; }],
    ['removing the headline account', layout => { delete layout.account_id; }],
    ['different named set membership', layout => { layout.account_set!.account_ids = [21, 23]; }],
    ['removing the named set', layout => { delete layout.account_set; }],
    ['a different Spending account', layout => { layout.widget_options.spending!.account_ids = [24]; }],
    ['returning Spending to all accounts', layout => { layout.widget_options.spending!.account_ids = []; }],
    ['a different Cashflow period', layout => { layout.widget_options.cashflow = { period: 'last_month' }; }]
  ])('refreshes figures for %s', (_, change) => {
    const before = configuredLayout();
    const after = structuredClone(before);
    change(after);
    expect(sameFigureScopes(before, after)).toBe(false);
    expect(sameFigureScopes(after, before)).toBe(false);
  });
});
