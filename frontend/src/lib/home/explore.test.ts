import { afterEach, describe, expect, it } from 'vitest';
import type { AttentionSource } from '#lib/api/attention.ts';
import type { AttentionState } from '#lib/attention/model.ts';
import { amountPrivacy } from '#lib/money/privacy.svelte.ts';
import {
  allocationWidget, backupsWidget, cashflowWidget, exploreCategories, exploreWidgets, forecastWidget, holdingsWidget, importsWidget,
  incomeWidget, pricesWidget, returnsWidget
} from './explore.ts';
import { home, q, TEST, USD, widgets } from './home-fixture.ts';
import { defaultWidgetOrder } from './widgets.ts';

function attention(sources: AttentionSource[], overrides: Partial<AttentionState> = {}): AttentionState {
  return {
    isPending: false, isError: false, isFetching: false, refetch: () => {},
    data: { checked_at: '2026-10-15T08:00:00Z', total_known: 0, complete: true, has_failures: false, sources },
    ...overrides
  };
}

const imports = (count: number | null, overrides: Partial<AttentionSource> = {}): AttentionSource =>
  ({ id: 'imports', unit: 'batch', state: 'ready', status: '', count, failed: false, ...overrides });

afterEach(() => {
  amountPrivacy.hidden = false;
});

describe('More to explore', () => {
  it('shows every other widget once, by category, none of them on My Home', () => {
    const groups = exploreWidgets(home(), 'en', attention([imports(0)]));
    expect(groups.map(group => group.title)).toEqual(['Everyday', 'Wealth & investments', 'Planning', 'Ledger & data']);
    const ids = groups.flatMap(group => group.widgets.map(widget => widget.id));
    expect(ids).toEqual(['cashflow', 'holdings', 'returns', 'allocation', 'income', 'forecast', 'imports', 'prices', 'backups']);
    expect(ids.filter(id => (defaultWidgetOrder as readonly string[]).includes(id))).toEqual([]);
    expect(exploreCategories.flatMap(category => category.widgets)).toEqual(ids);
  });

  it('shows no data in an empty book, except backups, which protect it anyway', () => {
    for (const group of exploreWidgets(home({}, { empty_book: true }), 'en', attention([imports(0)]))) {
      for (const widget of group.widgets) expect(widget.state).toBe(widget.id === 'backups' ? 'ready' : 'no_data');
    }
  });

  it('keeps a failed summary in its own card', () => {
    const groups = exploreWidgets(home({ holdings: { status: 'failed', summary: null } }), 'en', attention([imports(0)]));
    const failed = groups.flatMap(group => group.widgets).filter(widget => widget.state === 'failed');
    expect(failed.map(widget => widget.id)).toEqual(['holdings']);
  });
});

describe('cashflow', () => {
  it('nets money in and out, transfers apart, with the report link', () => {
    const view = cashflowWidget(home(), 'en');
    expect(view.title).toBe('Cashflow · October');
    expect(view.state).toBe('ready');
    expect(view.fact).toBe('+€830');
    expect(view.factSign).toBe(1);
    expect(view.factLabel).toBe('+€830 money in minus money out');
    expect(view.scope).toBe('Oct 1 – 15 · 2 cash accounts · EUR');
    expect(view.rows.map(row => [row.label, row.amount])).toEqual([['Money in', '€2,500'], ['Money out', '€1,670'], ['Transfers, net', '−€300']]);
    expect(view.open.href).toBe('/app/reports/cashflow?start_date=2026-10-01&end_date=2026-10-15&bucket=month&reporting_currency_id=1');
  });

  it('keeps transfers in their own currency when only they cannot convert', () => {
    // Money in and out convert; a $300 move to a brokerage has no rate.
    const summary = { ...widgets().cashflow.summary!, transfer_net: [q('-30000', USD)], converted_transfer_net: null };
    const view = cashflowWidget(home({ cashflow: { status: 'ok', summary } }), 'en');
    expect(view.state).toBe('partial');
    expect(view.fact).toBe('+€830');
    expect(view.rows.at(-1)).toEqual({ key: 'transfers', label: 'Transfers, net', amount: '−$300' });
    expect(view.scope).toBe('Oct 1\u2009–\u200915 · 2 cash accounts · EUR · transfers not converted to EUR');
  });

  // $300 out at €0.90 and back at €0.80: $0 in dollars, −€30 converted.
  it('keeps transfers that net to zero in their own currency', () => {
    const quiet = { ...widgets().cashflow.summary!, inflow: [], outflow: [], operating_net: [], converted_inflow: q('0'), converted_outflow: q('0'), converted_operating_net: q('0') };
    const atTwoRates = cashflowWidget(home({ cashflow: { status: 'ok', summary: { ...quiet, transfer_net: [q('0', USD)], converted_transfer_net: q('-3000') } } }), 'en');
    expect(atTwoRates.state).toBe('ready');
    expect(atTwoRates.rows.at(-1)).toEqual({ key: 'transfers', label: 'Transfers, net', amount: '−€30.00' });

    const atOneRate = cashflowWidget(home({ cashflow: { status: 'ok', summary: { ...quiet, transfer_net: [q('0', USD)], converted_transfer_net: q('0') } } }), 'en');
    expect(atOneRate.state).toBe('zero');

    // No rate: the exact $0 is shown and the card is not complete.
    const summary = { ...widgets().cashflow.summary!, transfer_net: [q('0', USD)], converted_transfer_net: null };
    const unconverted = cashflowWidget(home({ cashflow: { status: 'ok', summary } }), 'en');
    expect(unconverted.state).toBe('partial');
    expect(unconverted.rows.at(-1)?.key).toBe('transfers');
    expect(unconverted.rows.at(-1)?.amount).toMatch(/\$0\.00/);
    expect(unconverted.scope).toContain('transfers not converted to EUR');
    expect(cashflowWidget(home({ cashflow: { status: 'ok', summary: { ...quiet, transfer_net: [q('0', USD)], converted_transfer_net: null } } }), 'en').state).toBe('partial');
  });

  it('keeps each currency exact when anything cannot convert', () => {
    const summary = { ...widgets().cashflow.summary!, inflow: [q('250000'), q('5000', USD)], operating_net: [q('83000'), q('5000', USD)], converted_inflow: null, converted_operating_net: null };
    const view = cashflowWidget(home({ cashflow: { status: 'ok', summary } }), 'en');
    expect(view.state).toBe('partial');
    expect(view.fact).toBe('+€830 · +$50.00');
    expect(view.rows[0].amount).toBe('€2,500 · $50.00');
    expect(view.rows.at(-1)?.amount).toBe('−€300');
    expect(view.scope).toBe('Not every amount converts to EUR; totals by currency');
  });

  it('says when nothing moved or there is no cash account', () => {
    const quiet = { ...widgets().cashflow.summary!, inflow: [], outflow: [], transfer_net: [], operating_net: [], converted_inflow: q('0'), converted_outflow: q('0'), converted_transfer_net: q('0'), converted_operating_net: q('0') };
    expect(cashflowWidget(home({ cashflow: { status: 'ok', summary: quiet } }), 'en').state).toBe('zero');
    const none = cashflowWidget(home({ cashflow: { status: 'ok', summary: { ...quiet, account_count: 0 } } }), 'en');
    expect(none.state).toBe('not_set_up');
    expect(none.action?.href).toBe('/app/ledger/accounts');
  });

  it('masks amounts while they are hidden', () => {
    amountPrivacy.hidden = true;
    const view = cashflowWidget(home(), 'en');
    expect(view.fact).not.toContain('830');
    expect(view.rows.map(row => row.amount).join(' ')).not.toMatch(/\d/);
  });
});

describe('holdings', () => {
  it('lists the largest positions with their instrument and brokerage', () => {
    const view = holdingsWidget(home(), 'en');
    expect(view.state).toBe('ready');
    expect(view.fact).toBe('4 positions');
    expect(view.scope).toBe('Largest by value · Oct 15');
    expect(view.rows).toEqual([{ key: '41', label: 'TEST', href: `/app/investments/instruments/${TEST}`, detail: 'Brokerage', amount: '€600' }]);
    expect(view.more).toBe('+3 more positions');
  });

  it('is partial with an unpriced position and stale with an out-of-date price', () => {
    const summary = widgets().holdings.summary!;
    const unpriced = holdingsWidget(home({ holdings: { status: 'ok', summary: { ...summary, unpriced_count: 1, holdings: [{ ...summary.holdings[0], value: null }] } } }), 'en');
    expect(unpriced.state).toBe('partial');
    expect(unpriced.rows[0].amount).toBe('No price');
    expect(unpriced.scope).toBe('Largest by value · Oct 15 · 1 without a price');

    const missing = holdingsWidget(home({ holdings: { status: 'ok', summary: { ...summary, unpriced_count: 1, missing: [{ commodity_id: TEST, code: 'TEST', kind: 'price' }], holdings: [{ ...summary.holdings[0], value: null }] } } }), 'en');
    expect(missing.scope).toBe('Largest by value · Oct 15 · Price missing for TEST');
  });

  it('is stale when a rate behind a foreign holding is out of date, though its price is current', () => {
    const summary = { ...widgets().holdings.summary!, stale: [{ commodity_id: USD, code: 'USD', kind: 'rate' as const }] };
    const view = holdingsWidget(home({ holdings: { status: 'ok', summary } }), 'en');
    expect(view.state).toBe('stale');
    expect(view.scope).toBe('Largest by value · Oct 15 · Exchange rate for USD out of date');
  });

  it('does not blame prices when there is no default currency', () => {
    const summary = { ...widgets().holdings.summary!, unpriced_count: 4, holdings: [{ ...widgets().holdings.summary!.holdings[0], value: null }] };
    const view = holdingsWidget(home({ holdings: { status: 'ok', summary } }, { reporting_currency: null }), 'en');
    expect(view.state).toBe('partial');
    expect(view.scope).toBe('Set a default currency to see this in one currency.');
    expect(view.rows[0].amount).toBe('No value');
  });

  it('offers the first step without positions', () => {
    const view = holdingsWidget(home({ holdings: { status: 'ok', summary: { as_of: '2026-10-15', position_count: 0, unpriced_count: 0, holdings: [], missing: [], stale: [] } } }), 'en');
    expect(view.state).toBe('not_set_up');
    expect(view.action?.text).toBe('Record a buy');
  });
});

describe('returns and allocation', () => {
  it('shows the latest saved run as frozen, labelled as a saved run', () => {
    const view = returnsWidget(home(), 'en');
    expect(view.state).toBe('ready');
    expect(view.fact).toBe('+7.40%');
    expect(view.badge).toBe('Saved run');
    expect(view.scope).toBe('Time-weighted · Jan 1 – Oct 13, 2026 · saved Oct 14, 2026');
    expect(view.rows).toEqual([]);
    // The saved run itself, not its period: a live read would use another scope.
    expect(view.open.href).toBe('/app/investments/returns?run_id=7');
    expect(allocationWidget(home(), 'en').open.href).toBe('/app/investments/returns?run_id=7');
  });

  it('never calculates without a saved run', () => {
    const empty = { run_id: null, captured_at: null, from: null, to: null, account_scope: null, account_count: 0, time_weighted: null, time_weighted_annualized: null };
    const view = returnsWidget(home({ returns: { status: 'ok', summary: empty } }), 'en');
    expect(view.state).toBe('not_set_up');
    expect(view.fact).toBe('No saved run yet');
    expect(view.action?.href).toBe('/app/investments/returns');
    const allocation = allocationWidget(home({ allocation: { status: 'ok', summary: { run_id: null, captured_at: null, date: null, reason: null, basis: null, unknown_count: 0, rows: [], row_count: 0 } } }), 'en');
    expect(allocation.state).toBe('not_set_up');
  });

  it('says why a saved return is unavailable', () => {
    const summary = { ...widgets().returns.summary!, time_weighted: { value: null, reason: 'valuation_unavailable' as const, date: '2026-03-31' } };
    const view = returnsWidget(home({ returns: { status: 'ok', summary } }), 'en');
    expect(view.state).toBe('partial');
    expect(view.fact).toBe('Unavailable');
    expect(view.scope).toContain('Mar 31');
  });

  it('leads allocation with the largest asset type', () => {
    const view = allocationWidget(home(), 'en');
    expect(view.state).toBe('ready');
    expect(view.fact).toBe('Stock 60.00%');
    expect(view.rows.map(row => [row.label, row.amount])).toEqual([['Stock', '60.00%'], ['Cash', '40.00%']]);
    expect(view.scope).toBe('By asset type · Oct 13 · saved Oct 14, 2026');

    const known = allocationWidget(home({ allocation: { status: 'ok', summary: { ...widgets().allocation.summary!, basis: 'known_only', unknown_count: 2 } } }), 'en');
    expect(known.state).toBe('partial');
    expect(known.scope).toContain('2 balances without a value');
  });
});

describe('dividend income', () => {
  it('counts instruments with confirmed income', () => {
    const view = incomeWidget(home(), 'en');
    expect(view.fact).toBe('Income from 2 instruments');
    expect(view.scope).toBe('Confirmed dividends, all time');
  });

  it('leads with payments to assign, as cash, never income', () => {
    const view = incomeWidget(home({ income: { status: 'ok', summary: { instrument_count: 0, unassigned_count: 3, unassigned: [{ cash: q('9640'), item_count: 3 }] } } }), 'en');
    expect(view.fact).toBe('3 payments to assign');
    expect(view.warning).toBe(true);
    expect(view.rows).toEqual([{ key: '1', label: 'Unassigned cash', detail: '3 payments', amount: '€96.40' }]);
    expect(view.scope).toBe('unassigned cash is not income until you assign it');
  });

  it('is zero without dividends', () => {
    expect(incomeWidget(home({ income: { status: 'ok', summary: { instrument_count: 0, unassigned_count: 0, unassigned: [] } } }), 'en').state).toBe('zero');
  });
});

describe('forecast', () => {
  it('is labelled an estimate and calculates nothing', () => {
    const view = forecastWidget(home(), 'en');
    expect(view.badge).toBe('Estimate');
    expect(view.fact).toBe('Next 90 days');
    expect(view.scope).toBe('Projected balances from 4 active schedules, calculated when you open it');
    expect(view.chart).toBeNull();
  });

  it('asks for a schedule when nothing recurs', () => {
    const view = forecastWidget(home({ recurring: { status: 'ok', summary: { today: '2026-10-15', active_count: 0, draft_count: 0, blocked_count: 0, upcoming: [] } } }), 'en');
    expect(view.state).toBe('not_set_up');
    expect(view.action?.href).toBe('/app/planning/recurring?tab=templates');
  });
});

describe('imports', () => {
  it('reads the shared attention count', () => {
    const view = importsWidget(home(), 'en', attention([imports(2)]));
    expect(view.fact).toBe('2 batches to review');
    expect(view.warning).toBe(true);
    expect(importsWidget(home(), 'en', attention([imports(0)])).state).toBe('zero');
  });

  it('is checking while unknown and fails with its own retry', () => {
    expect(importsWidget(home(), 'en', attention([], { data: undefined, isPending: true })).fact).toBe('Checking…');
    let retried = false;
    const view = importsWidget(home(), 'en', attention([imports(null, { state: 'error' })], { refetch: () => (retried = true) }));
    expect(view.state).toBe('failed');
    view.retry?.();
    expect(retried).toBe(true);
  });
});

describe('prices', () => {
  it('names what net worth is missing and links where the briefing does', () => {
    const base = home();
    const missing = [{ commodity_id: TEST, code: 'TEST', kind: 'price' as const }, { commodity_id: USD, code: 'USD', kind: 'rate' as const }];
    const view = pricesWidget({ ...base, headlines: [{ ...base.headlines[0], missing }] }, 'en');
    expect(view.state).toBe('partial');
    expect(view.fact).toBe('2 values missing');
    expect(view.rows.map(row => [row.label, row.detail])).toEqual([['TEST', 'price'], ['USD', 'exchange rate']]);
    expect(view.open.href).toBe('/app/investments/prices?status=unusable');
  });

  it('counts out-of-date prices and is up to date otherwise', () => {
    const base = home();
    const stale = pricesWidget({ ...base, headlines: [{ ...base.headlines[0], stale: [{ commodity_id: TEST, code: 'TEST', kind: 'price' }] }] }, 'en');
    expect(stale.state).toBe('stale');
    expect(stale.fact).toBe('Price for TEST out of date');
    expect(stale.open.href).toBe('/app/investments/prices?status=stale');
    expect(pricesWidget(base, 'en').fact).toBe('Up to date');
    expect(pricesWidget({ ...base, reporting_currency: null }, 'en').state).toBe('not_set_up');
  });
});

describe('backups', () => {
  const summary = widgets().backups.summary!;
  it('shows the last backup', () => {
    const view = backupsWidget(home(), 'en');
    expect(view.state).toBe('ready');
    expect(view.fact).toMatch(/^Backed up Oct 15, 2026/);
    expect(view.scope).toMatch(/^Next Oct 16, 2026/);
  });

  it('says a failure and whether it will retry', () => {
    const failed = backupsWidget(home({ backups: { status: 'ok', summary: { ...summary, latest_status: 'failed', latest_will_retry: true } } }), 'en');
    expect(failed.fact).toBe('Last backup failed');
    expect(failed.warning).toBe(true);
    expect(failed.scope).toMatch(/^It will try again · Backed up/);
  });

  it('is not set up while automatic backups are off', () => {
    const view = backupsWidget(home({ backups: { status: 'ok', summary: { ...summary, enabled: false } } }), 'en');
    expect(view.state).toBe('not_set_up');
    expect(view.action?.text).toBe('Turn on backups');
    const none = backupsWidget(home({ backups: { status: 'ok', summary: { ...summary, last_success_at: null, latest_status: null } } }), 'en');
    expect(none.fact).toBe('No backup yet');
  });
});
