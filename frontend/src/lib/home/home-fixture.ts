import type { Home, HomeWidgets } from '#lib/api/home.ts';

/** A Home read for unit tests: every figure is hand-picked, so a view model's output can be checked by eye. */

export const EUR = 1;
export const USD = 2;
export const TEST = 3;
export const q = (value: string, commodity = EUR, scale = 2) => ({ commodity_id: commodity, quantity_value: value, quantity_scale: scale, normal_quantity_value: value });
export const amount = (value: string, commodity = EUR) => ({ commodity_id: commodity, quantity_value: value, quantity_scale: 2 });

export function widgets(overrides: Partial<HomeWidgets> = {}): HomeWidgets {
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
    cashflow: {
      status: 'ok',
      summary: {
        start_date: '2026-10-01', end_date: '2026-10-15', cash_scope: 'default_liquid_cash', account_count: 2,
        inflow: [q('250000')], outflow: [q('167000')], transfer_net: [q('-30000')], operating_net: [q('83000')],
        converted_inflow: q('250000'), converted_outflow: q('167000'), converted_transfer_net: q('-30000'), converted_operating_net: q('83000')
      }
    },
    holdings: {
      status: 'ok',
      summary: {
        as_of: '2026-10-15', position_count: 4, unpriced_count: 0,
        holdings: [{ account_id: 41, brokerage_id: 23, brokerage_name: 'Brokerage', balances: [q('10', TEST, 0)], value: q('60000') }]
      }
    },
    returns: {
      status: 'ok',
      summary: {
        run_id: 7, captured_at: '2026-10-14T09:00:00Z', from: '2026-01-01', to: '2026-10-13', account_scope: 'default_investment_accounts', account_count: 1,
        time_weighted: { value: '0.074000', reason: null, date: null }, time_weighted_annualized: { value: null, reason: 'period_shorter_than_year', date: null }
      }
    },
    allocation: {
      status: 'ok',
      summary: {
        run_id: 7, captured_at: '2026-10-14T09:00:00Z', date: '2026-10-13', reason: null, basis: 'closing_value', unknown_count: 0, row_count: 2,
        rows: [{ asset_type: 'stock', side: 'long', value: q('60000'), weight: '0.600000' }, { asset_type: 'cash', side: 'long', value: q('40000'), weight: '0.400000' }]
      }
    },
    income: { status: 'ok', summary: { instrument_count: 2, unassigned_count: 0, unassigned: [] } },
    backups: { status: 'ok', summary: { enabled: true, next_run_at: '2026-10-16T01:15:00Z', last_success_at: '2026-10-15T01:15:05Z', latest_status: 'completed', latest_will_retry: false } },
    ...overrides
  };
}

export function home(overrides: Partial<HomeWidgets> = {}, extra: Partial<Home> = {}): Home {
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
    consistent: true,
    ...extra
  };
}
