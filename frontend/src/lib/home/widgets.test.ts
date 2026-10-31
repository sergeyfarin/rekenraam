import { afterEach, describe, expect, it } from 'vitest';
import type { Home, HomeWidgets } from '#lib/api/home.ts';
import { amountPrivacy } from '#lib/money/privacy.svelte.ts';
import { accountsWidget, budgetsWidget, homeWidgets, netWorthTrendWidget, recurringWidget, spendingWidget, trendHeights } from './widgets.ts';

const EUR = 1;
const USD = 2;
const TEST = 3;
const q = (value: string, commodity = EUR, scale = 2) => ({ commodity_id: commodity, quantity_value: value, quantity_scale: scale, normal_quantity_value: value });
const amount = (value: string, commodity = EUR) => ({ commodity_id: commodity, quantity_value: value, quantity_scale: 2 });

function widgets(overrides: Partial<HomeWidgets> = {}): HomeWidgets {
  return {
    spending: {
      status: 'ok',
      summary: {
        start_date: '2026-10-01', end_date: '2026-10-15', total: q('167000'), commodity_totals: [q('167000')], category_count: 4,
        categories: [
          { category_id: 11, code: 'RENT', name: 'Rent', builtin_key: null, converted: q('120000'), totals: [q('120000')] },
          { category_id: 12, code: '', name: '', builtin_key: 'expense.food.groceries', converted: q('35000'), totals: [q('35000')] },
          { category_id: 13, code: 'DINE', name: 'Dining out', builtin_key: null, converted: q('8000'), totals: [q('8000')] }
        ]
      }
    },
    budgets: {
      status: 'ok',
      summary: {
        period_start: '2026-10-01', period_end: '2026-10-31', commodity_id: EUR, other_commodity_count: 1, category_count: 4,
        total: { commodity_id: EUR, target: amount('175000'), actual: amount('167000'), remaining: amount('8000') },
        categories: [
          { category_id: 13, code: 'DINE', name: 'Dining out', builtin_key: null, amount: { commodity_id: EUR, target: amount('5000'), actual: amount('8000'), remaining: amount('-3000') } },
          { category_id: 11, code: 'RENT', name: 'Rent', builtin_key: null, amount: { commodity_id: EUR, target: amount('120000'), actual: amount('120000'), remaining: amount('0') } }
        ]
      }
    },
    accounts: {
      status: 'ok',
      summary: {
        as_of: '2026-10-15', open_count: 5,
        accounts: [
          { account_id: 21, code: 'CARD', name: 'Card', account_class: 'liability', account_kind: 'credit_card', balances: [q('-200000')], value: q('-200000') },
          { account_id: 22, code: 'USD', name: 'Dollar savings', account_class: 'asset', account_kind: 'savings', balances: [q('100000', USD)], value: q('90000') },
          { account_id: 23, code: 'BRK', name: 'Brokerage', account_class: 'asset', account_kind: 'brokerage', balances: [q('10', TEST, 0)], value: null }
        ]
      }
    },
    recurring: {
      status: 'ok',
      summary: {
        today: '2026-10-15', active_count: 4, draft_count: 2, blocked_count: 0,
        upcoming: [{ template_id: 31, name: 'Energy', next_due_on: '2026-10-20', amounts: [q('14200')] }]
      }
    },
    net_worth_trend: {
      status: 'ok',
      summary: {
        start_date: '2025-10-31', end_date: '2026-10-15', change: q('130000'),
        points: [{ date: '2025-10-31', value: q('0') }, { date: '2026-09-30', value: q('100000') }, { date: '2026-10-15', value: q('130000') }]
      }
    },
    ...overrides
  };
}

function home(overrides: Partial<HomeWidgets> = {}, extra: Partial<Home> = {}): Home {
  return {
    as_of: '2026-10-15', start_boundary: '2026-09-30', empty_book: false,
    reporting_currency: { commodity_id: EUR, code: 'EUR', kind: 'currency', symbol: '€', scale: 2 },
    headlines: [{ id: 'net_worth', account_count: 5, value: q('130000'), start_value: q('100000'), change: q('30000'), unavailable: null, missing: [], stale: [] }],
    widgets: widgets(overrides),
    commodities: [
      { commodity_id: EUR, code: 'EUR', kind: 'currency', symbol: '€', scale: 2 },
      { commodity_id: USD, code: 'USD', kind: 'currency', symbol: '$', scale: 2 },
      { commodity_id: TEST, code: 'TEST', kind: 'security', symbol: 'TEST', scale: 0 }
    ],
    ...extra
  };
}

afterEach(() => { amountPrivacy.hidden = false; });

describe('My Home', () => {
  it('lists the five default widgets in order, the trend wide', () => {
    const views = homeWidgets(home(), 'en');
    expect(views.map(view => view.id)).toEqual(['spending', 'budgets', 'accounts', 'recurring', 'net_worth_trend']);
    expect(views.map(view => view.wide)).toEqual([false, false, false, false, true]);
  });

  it('shows no data rather than zeros in an empty book', () => {
    for (const view of homeWidgets(home({}, { empty_book: true }), 'en')) {
      expect(view.state).toBe('no_data');
      expect(view.rows).toEqual([]);
      expect(view.open.href).toMatch(/^\/app\//);
    }
  });

  it('keeps one failed summary in its own card', () => {
    const views = homeWidgets(home({ budgets: { status: 'failed', summary: null } }), 'en');
    expect(views.find(view => view.id === 'budgets')?.state).toBe('failed');
    expect(views.filter(view => view.state === 'failed')).toHaveLength(1);
  });
});

describe('spending', () => {
  it('shows the month total Home-rounded and the first categories with drill-downs', () => {
    const view = spendingWidget(home(), 'en');
    expect(view.state).toBe('ready');
    expect(view.fact).toBe('€1,670');
    expect(view.scope).toBe('Oct 1\u2009–\u200915 · all accounts · EUR');
    expect(view.rows.map(row => [row.label, row.amount])).toEqual([['Rent', '€1,200'], ['Groceries', '€350'], ['Dining out', '€80.00']]);
    expect(view.rows[0].bar).toBe(1);
    expect(view.rows[2].href).toBe('/app/ledger/transactions?after_date=2026-10-01&before_date=2026-10-15&date_basis=entry&status=posted&category_id=13');
    expect(view.more).toBe('+1 more category');
    expect(view.open.href).toBe('/app/reports/spending?start_date=2026-10-01&end_date=2026-10-15&bucket=month&reporting_currency_id=1');
  });

  it('shows each currency exactly when the total cannot convert, never a partial sum', () => {
    const view = spendingWidget(home({ spending: { status: 'ok', summary: { ...widgets().spending.summary!, total: null, commodity_totals: [q('167000'), q('2000', USD)] } } }), 'en');
    expect(view.state).toBe('partial');
    expect(view.fact).toBe('€1,670 · $20.00');
  });

  it('is zero before anything is spent', () => {
    const view = spendingWidget(home({ spending: { status: 'ok', summary: { ...widgets().spending.summary!, total: q('0'), commodity_totals: [], categories: [], category_count: 0 } } }), 'en');
    expect(view.state).toBe('zero');
    expect(view.scope).toBe('Nothing spent yet this month');
  });

  it('hides amounts and bars while amounts are hidden', () => {
    amountPrivacy.hidden = true;
    const view = spendingWidget(home(), 'en');
    expect(view.fact).toBe('••••');
    expect(view.rows.every(row => row.bar === undefined)).toBe(true);
  });
});

describe('budgets', () => {
  it('leads with what is left, names overspending in words and links the month and currency', () => {
    const view = budgetsWidget(home(), 'en');
    expect(view.title).toBe('Budgets · October');
    expect(view.fact).toBe('€80.00 left');
    expect(view.scope).toBe('€1,670 of €1,750 spent · also 1 other currency');
    expect(view.rows[0]).toMatchObject({ label: 'Dining out', amount: '€80.00 of €50.00', detail: '€30.00 over', over: true });
    expect(view.rows[1].over).toBe(false);
    expect(view.more).toBe('+2 more budgets');
    expect(view.open.href).toBe('/app/planning/budgets?month=2026-10&currency_id=1');
  });

  it('says over budget when the month is over', () => {
    const summary = widgets().budgets.summary!;
    const view = budgetsWidget(home({ budgets: { status: 'ok', summary: { ...summary, total: { ...summary.total!, remaining: amount('-12000') } } } }), 'en');
    expect(view.fact).toBe('€120 over budget');
  });

  it('offers to set a budget when nothing is budgeted', () => {
    const view = budgetsWidget(home({ budgets: { status: 'ok', summary: { ...widgets().budgets.summary!, commodity_id: null, total: null, categories: [], category_count: 0 } } }), 'en');
    expect(view.state).toBe('not_set_up');
    expect(view.action?.href).toBe('/app/planning/budgets?month=2026-10');
  });
});

describe('accounts', () => {
  it('shows a currency balance in its own currency and anything else by value', () => {
    const view = accountsWidget(home(), 'en');
    expect(view.fact).toBe('5 open accounts');
    expect(view.scope).toBe('Largest balances · Oct 15');
    expect(view.rows.map(row => row.amount)).toEqual(['−€2,000', '$1,000', 'No value']);
    expect(view.rows[1].href).toBe('/app/ledger/accounts/22/register');
    expect(view.state).toBe('partial');
    expect(view.more).toBe('+2 more accounts');
  });
});

describe('recurring', () => {
  it('names the next schedule and counts drafts, linking to the Due list', () => {
    const view = recurringWidget(home(), 'en');
    expect(view.fact).toBe('Next: Energy on Oct 20');
    expect(view.scope).toBe('4 active schedules · 2 drafts to review');
    expect(view.rows[0]).toMatchObject({ label: 'Energy', detail: 'Oct 20', amount: '€142' });
    expect(view.open.href).toBe('/app/planning/recurring');
  });

  it('is not set up without schedules', () => {
    const view = recurringWidget(home({ recurring: { status: 'ok', summary: { today: '2026-10-15', active_count: 0, draft_count: 0, blocked_count: 0, upcoming: [] } } }), 'en');
    expect(view.state).toBe('not_set_up');
    expect(view.action?.href).toBe('/app/planning/recurring?tab=templates');
  });
});

describe('net worth trend', () => {
  it('shows the signed change over the series with a chart and the report link', () => {
    const view = netWorthTrendWidget(home(), 'en');
    expect(view.fact).toBe('+€1,300');
    expect(view.factSign).toBe(1);
    expect(view.factLabel).toBe('+€1,300 since Oct 31');
    expect(view.scope).toBe('Oct 2025\u2009–\u2009Oct 2026 · month-end values · EUR');
    expect(view.chart?.points).toEqual([0, 100000 / 130000, 1].map(value => Math.floor(value * 1_000_000) / 1_000_000));
    expect(view.open.href).toBe('/app/reports/net-worth?start_date=2025-10-31&end_date=2026-10-15&bucket=month&reporting_currency_id=1');
  });

  it('is unavailable, never partial, when today has no value', () => {
    const summary = widgets().net_worth_trend.summary!;
    const view = netWorthTrendWidget(home({ net_worth_trend: { status: 'ok', summary: { ...summary, change: null, points: [...summary.points.slice(0, 2), { date: '2026-10-15', value: null }] } } }), 'en');
    expect(view.fact).toBe('Unavailable');
    expect(view.chart).toBeNull();
  });

  it('counts out-of-date values as stale', () => {
    const extra = { headlines: [{ id: 'net_worth' as const, account_count: 5, value: q('130000'), start_value: null, change: null, unavailable: null, missing: [], stale: [{ commodity_id: TEST, code: 'TEST', kind: 'price' as const }] }] };
    const view = netWorthTrendWidget(home({}, extra), 'en');
    expect(view.state).toBe('stale');
    expect(view.scope).toContain('1 value out of date');
  });

  it('asks for a default currency without one', () => {
    const view = netWorthTrendWidget(home({ net_worth_trend: { status: 'ok', summary: { start_date: '2025-10-31', end_date: '2026-10-15', change: null, points: [] } } }, { reporting_currency: null }), 'en');
    expect(view.state).toBe('not_set_up');
    expect(view.action?.href).toBe('/app/settings/currencies');
  });

  it('drops the sign colour and the chart while amounts are hidden', () => {
    amountPrivacy.hidden = true;
    const view = netWorthTrendWidget(home(), 'en');
    expect(view.fact).toBe('••••');
    expect(view.factSign).toBe(0);
    expect(view.chart).toBeNull();
  });
});

describe('trend heights', () => {
  it('scales between the lowest and highest point exactly and keeps gaps', () => {
    expect(trendHeights([q('-500'), null, q('1500'), q('500')])).toEqual([0, null, 1, 0.5]);
    expect(trendHeights([q('7'), q('7')])).toEqual([0.5, 0.5]);
    // Beyond Number's exact range the order still holds.
    expect(trendHeights([q('90071992547409930'), q('90071992547409931'), q('90071992547409932')])).toEqual([0, 0.5, 1]);
  });
});
