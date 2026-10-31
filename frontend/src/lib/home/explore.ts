import { parseISO } from 'date-fns';
import type { Home, HomeValuationInput } from '#lib/api/home.ts';
import type { AttentionState } from '#lib/attention/model.ts';
import { assetTypeText, formatReturn, formatWeight, returnDirection, returnReasonText } from '#lib/investments/returns-model.ts';
import { m } from '#lib/paraglide/messages.js';
import { valuationHref, valuationText } from './briefing.ts';
import {
  commodityMap, day, failed, money, noData, number, range, reportQuery, sign,
  type Quantity, type WidgetID, type WidgetLink, type WidgetRow, type WidgetView
} from './widgets.ts';

/**
 * More to explore (#479): every widget that is not on My Home, at real size,
 * by category. Each card reads data Home already has: the composed Home read,
 * or the shared attention read the shell loads anyway, so mounting the section
 * starts no request and no returns or forecast calculation.
 */

export type ExploreCategoryID = 'everyday' | 'wealth' | 'planning' | 'data';

export const exploreCategories: readonly { id: ExploreCategoryID; widgets: readonly WidgetID[] }[] = [
  { id: 'everyday', widgets: ['cashflow'] },
  { id: 'wealth', widgets: ['holdings', 'returns', 'allocation', 'income'] },
  { id: 'planning', widgets: ['forecast'] },
  { id: 'data', widgets: ['imports', 'prices', 'backups'] }
];

export function categoryTitle(id: ExploreCategoryID): string {
  return { everyday: m.home_explore_everyday, wealth: m.home_explore_wealth, planning: m.home_explore_planning, data: m.home_explore_data }[id]();
}

function view(id: WidgetID, title: string, open: WidgetLink, fields: Partial<WidgetView> & Pick<WidgetView, 'state' | 'fact'>): WidgetView {
  return { id, title, wide: false, scope: null, rows: [], more: null, chart: null, action: null, open, ...fields };
}

function monthTitle(date: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, { month: 'long' }).format(parseISO(date));
}

function joined(quantities: Quantity[], home: Home, locale: string): string {
  const commodities = commodityMap(home);
  return quantities.map(quantity => money(quantity, commodities, locale)).join(' · ');
}

export function cashflowWidget(home: Home, locale: string): WidgetView {
  const widget = home.widgets.cashflow;
  const summary = widget.summary;
  const start = summary?.start_date ?? `${home.as_of.slice(0, 8)}01`;
  const end = summary?.end_date ?? home.as_of;
  const title = m.home_cashflow_title({ month: monthTitle(start, locale) });
  const open = { text: m.home_cashflow_open(), href: `/app/reports/cashflow?${reportQuery(start, end, home)}` };
  if (home.empty_book) return noData('cashflow', title, open);
  if (widget.status === 'failed' || !summary) return failed('cashflow', title, open);
  if (summary.account_count === 0) {
    return view('cashflow', title, open, { state: 'not_set_up', fact: m.home_cashflow_no_accounts(), action: { text: m.home_cashflow_add_account(), href: '/app/ledger/accounts' } });
  }
  const commodities = commodityMap(home);
  const period = range(summary.start_date, summary.end_date, locale, { day: 'numeric', month: 'short' });
  const accounts = m.home_cashflow_accounts({ count: summary.account_count, n: number(summary.account_count, locale) });
  const moved = summary.inflow.length > 0 || summary.outflow.length > 0 || summary.transfer_net.some(total => sign(total.quantity_value) !== 0);
  if (!moved) return view('cashflow', title, open, { state: 'zero', fact: null, scope: m.home_cashflow_zero() });

  const converted = home.reporting_currency && summary.converted_operating_net ? summary : null;
  if (converted && converted.converted_operating_net && converted.converted_inflow && converted.converted_outflow) {
    const net = converted.converted_operating_net;
    const rows: WidgetRow[] = [
      { key: 'in', label: m.home_cashflow_in(), amount: money(converted.converted_inflow, commodities, locale) },
      { key: 'out', label: m.home_cashflow_out(), amount: money(converted.converted_outflow, commodities, locale) }
    ];
    if (converted.converted_transfer_net && sign(converted.converted_transfer_net.quantity_value) !== 0) {
      rows.push({ key: 'transfers', label: m.home_cashflow_transfers(), amount: money(converted.converted_transfer_net, commodities, locale, true) });
    }
    return view('cashflow', title, open, {
      state: 'ready',
      fact: money(net, commodities, locale, true),
      factSign: sign(net.quantity_value),
      factLabel: m.home_cashflow_net_label({ amount: money(net, commodities, locale, true) }),
      scope: [period, accounts, home.reporting_currency?.code ?? ''].join(' · '),
      rows
    });
  }
  // Not every posting converts: each measure in each currency, never a partial sum.
  const rows: WidgetRow[] = [
    { key: 'in', label: m.home_cashflow_in(), amount: joined(summary.inflow, home, locale) || '—' },
    { key: 'out', label: m.home_cashflow_out(), amount: joined(summary.outflow, home, locale) || '—' }
  ];
  return view('cashflow', title, open, {
    state: 'partial',
    fact: summary.operating_net.map(total => money(total, commodities, locale, true)).join(' · ') || null,
    scope: home.reporting_currency ? m.home_cashflow_partial({ currency: home.reporting_currency.code }) : m.home_widget_no_currency(),
    rows
  });
}

function priceStale(home: Home): HomeValuationInput[] {
  return home.headlines.find(headline => headline.id === 'net_worth')?.stale ?? [];
}

export function holdingsWidget(home: Home, locale: string): WidgetView {
  const title = m.home_holdings_title();
  const open = { text: m.home_holdings_open(), href: '/app/investments' };
  if (home.empty_book) return noData('holdings', title, open);
  const widget = home.widgets.holdings;
  const summary = widget.summary;
  if (widget.status === 'failed' || !summary) return failed('holdings', title, open);
  if (summary.position_count === 0) {
    return view('holdings', title, open, { state: 'not_set_up', fact: m.home_holdings_none(), action: { text: m.home_holdings_set_up(), href: '/app/investments' } });
  }
  const commodities = commodityMap(home);
  const rows = summary.holdings.map((row): WidgetRow => {
    const instrument = row.balances.find(balance => commodities.get(balance.commodity_id)?.kind !== 'currency') ?? row.balances[0];
    const commodity = instrument ? commodities.get(instrument.commodity_id) : undefined;
    return {
      key: String(row.account_id),
      label: commodity?.code ?? m.accounts_unnamed_account({ id: row.account_id }),
      href: instrument ? `/app/investments/instruments/${instrument.commodity_id}` : undefined,
      detail: row.brokerage_name.trim() || undefined,
      amount: row.value ? money(row.value, commodities, locale) : home.reporting_currency ? m.home_holdings_no_price() : m.home_accounts_no_value()
    };
  });
  // Without a default currency nothing has a value, which is not a missing price.
  if (!home.reporting_currency) {
    return view('holdings', title, open, {
      state: 'partial', rows, scope: m.home_widget_no_currency(),
      fact: m.home_holdings_count({ count: summary.position_count, n: number(summary.position_count, locale) })
    });
  }
  const stale = priceStale(home).filter(input => input.kind === 'price').length;
  const scope = [m.home_holdings_scope({ date: day(summary.as_of, locale) })];
  if (summary.unpriced_count > 0) scope.push(m.home_holdings_unpriced({ count: summary.unpriced_count, n: number(summary.unpriced_count, locale) }));
  else if (stale > 0) scope.push(m.home_trend_stale({ count: stale, n: number(stale, locale) }));
  const rest = summary.position_count - summary.holdings.length;
  return view('holdings', title, open, {
    state: summary.unpriced_count > 0 ? 'partial' : stale > 0 ? 'stale' : 'ready',
    fact: m.home_holdings_count({ count: summary.position_count, n: number(summary.position_count, locale) }),
    scope: scope.join(' · '),
    rows,
    more: rest > 0 ? m.home_holdings_more({ count: rest, n: number(rest, locale) }) : null
  });
}

function returnsOpen(from: string | null, to: string | null): WidgetLink {
  const params = new URLSearchParams();
  if (from && to) {
    params.set('from', from);
    params.set('to', to);
  }
  const query = params.toString();
  return { text: m.home_returns_open(), href: `/app/investments/returns${query ? `?${query}` : ''}` };
}

function savedOn(capturedAt: string, locale: string): string {
  return m.home_returns_saved({ date: new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(new Date(capturedAt)) });
}

/** No saved run: Home never calculates a return, so it says how to get one. */
function notSaved(id: WidgetID, title: string, open: WidgetLink): WidgetView {
  return view(id, title, open, { state: 'not_set_up', fact: m.home_returns_not_saved(), scope: m.home_returns_not_saved_copy(), action: { text: m.home_returns_calculate(), href: '/app/investments/returns' } });
}

export function returnsWidget(home: Home, locale: string): WidgetView {
  const title = m.home_returns_title();
  const widget = home.widgets.returns;
  const summary = widget.summary;
  const open = returnsOpen(summary?.from ?? null, summary?.to ?? null);
  if (home.empty_book) return noData('returns', title, open);
  if (widget.status === 'failed' || !summary) return failed('returns', title, open);
  if (summary.run_id === null || !summary.time_weighted || !summary.captured_at || !summary.from || !summary.to) return notSaved('returns', title, open);
  const period = range(summary.from, summary.to, locale, { day: 'numeric', month: 'short', year: 'numeric' });
  const scope = [m.home_returns_scope({ period }), savedOn(summary.captured_at, locale)].join(' · ');
  const figure = summary.time_weighted;
  const badge = m.home_returns_saved_badge();
  if (figure.value === null) {
    const formatDate = (date: string) => day(date, locale);
    return view('returns', title, open, { state: 'partial', fact: m.home_trend_unavailable(), scope: `${returnReasonText(figure.reason, figure.date, formatDate)} · ${scope}`, badge });
  }
  const annualized = summary.time_weighted_annualized?.value;
  const direction = returnDirection(figure.value);
  return view('returns', title, open, {
    state: 'ready',
    fact: formatReturn(figure.value, locale),
    factSign: direction === 'up' ? 1 : direction === 'down' ? -1 : 0,
    scope,
    rows: annualized ? [{ key: 'annualized', label: m.home_returns_annualized(), amount: formatReturn(annualized, locale) }] : [],
    badge
  });
}

export function allocationWidget(home: Home, locale: string): WidgetView {
  const title = m.home_allocation_title();
  const widget = home.widgets.allocation;
  const summary = widget.summary;
  const open = returnsOpen(home.widgets.returns.summary?.from ?? null, summary?.date ?? null);
  if (home.empty_book) return noData('allocation', title, open);
  if (widget.status === 'failed' || !summary) return failed('allocation', title, open);
  if (summary.run_id === null || !summary.captured_at || !summary.date) return notSaved('allocation', title, open);
  const badge = m.home_returns_saved_badge();
  const scope = [m.home_allocation_scope({ date: day(summary.date, locale) }), savedOn(summary.captured_at, locale)];
  if (summary.reason) {
    return view('allocation', title, open, { state: 'partial', fact: m.home_trend_unavailable(), scope: `${returnReasonText(summary.reason, summary.date, date => day(date, locale))} · ${scope.join(' · ')}`, badge });
  }
  const commodities = commodityMap(home);
  const label = (row: (typeof summary.rows)[number]) => row.side === 'short' ? m.home_allocation_short({ type: assetTypeText(row.asset_type) }) : assetTypeText(row.asset_type);
  const rows = summary.rows.map((row): WidgetRow => ({
    key: `${row.asset_type}-${row.side}`,
    label: label(row),
    amount: row.weight !== null ? formatWeight(row.weight, locale) : row.value ? money(row.value, commodities, locale) : '—'
  }));
  if (summary.basis === 'known_only') scope.push(m.home_allocation_unknown({ count: summary.unknown_count, n: number(summary.unknown_count, locale) }));
  const first = summary.rows[0];
  const rest = summary.row_count - summary.rows.length;
  return view('allocation', title, open, {
    state: summary.rows.length === 0 ? 'zero' : summary.basis === 'known_only' ? 'partial' : 'ready',
    fact: first ? (first.weight !== null ? m.home_allocation_fact({ type: label(first), weight: formatWeight(first.weight, locale) }) : label(first)) : null,
    scope: scope.join(' · '),
    rows,
    more: rest > 0 ? m.home_allocation_more({ count: rest, n: number(rest, locale) }) : null,
    badge
  });
}

export function incomeWidget(home: Home, locale: string): WidgetView {
  const title = m.home_income_title();
  const open = { text: m.home_income_open(), href: '/app/investments/income' };
  if (home.empty_book) return noData('income', title, open);
  const widget = home.widgets.income;
  const summary = widget.summary;
  if (widget.status === 'failed' || !summary) return failed('income', title, open);
  if (summary.instrument_count === 0 && summary.unassigned_count === 0) {
    return view('income', title, open, { state: 'zero', fact: null, scope: m.home_income_zero() });
  }
  const commodities = commodityMap(home);
  const instruments = m.home_income_instruments({ count: summary.instrument_count, n: number(summary.instrument_count, locale) });
  if (summary.unassigned_count === 0) {
    return view('income', title, open, { state: 'ready', fact: instruments, scope: m.home_income_scope() });
  }
  // Unassigned cash is cash, not gross income, until the owner assigns it.
  return view('income', title, open, {
    state: 'ready',
    fact: m.home_income_to_assign({ count: summary.unassigned_count, n: number(summary.unassigned_count, locale) }),
    warning: true,
    scope: summary.instrument_count > 0 ? `${instruments} · ${m.home_income_unassigned_copy()}` : m.home_income_unassigned_copy(),
    rows: summary.unassigned.map(item => ({
      key: String(item.cash.commodity_id),
      label: m.home_income_unassigned_cash(),
      detail: m.home_income_payments({ count: item.item_count, n: number(item.item_count, locale) }),
      amount: money(item.cash, commodities, locale)
    }))
  });
}

/** The forecast is calculated on its page (#440 brings it here); the card
 * says what it projects from, read from the recurring summary Home has. */
export function forecastWidget(home: Home, locale: string): WidgetView {
  const title = m.home_forecast_title();
  const open = { text: m.home_forecast_open(), href: '/app/planning/forecast' };
  if (home.empty_book) return noData('forecast', title, open);
  const badge = m.home_forecast_badge();
  const recurring = home.widgets.recurring.summary;
  if (recurring && recurring.active_count === 0) {
    return view('forecast', title, open, { state: 'not_set_up', fact: m.home_forecast_not_set_up(), scope: m.home_forecast_not_set_up_copy(), action: { text: m.home_recurring_set_up(), href: '/app/planning/recurring?tab=templates' }, badge });
  }
  return view('forecast', title, open, {
    state: 'ready',
    fact: m.home_forecast_fact(),
    scope: recurring ? m.home_forecast_scope({ count: recurring.active_count, n: number(recurring.active_count, locale) }) : m.home_forecast_scope_unknown(),
    badge
  });
}

export function importsWidget(home: Home, locale: string, attention: AttentionState): WidgetView {
  const title = m.home_imports_title();
  const open = { text: m.home_imports_open(), href: '/app/ledger/import' };
  if (home.empty_book) return noData('imports', title, open);
  const source = attention.data?.sources.find(item => item.id === 'imports');
  if (attention.isError || source?.state === 'error') return { ...failed('imports', title, open), retry: attention.refetch };
  if (!source || source.count === null) return view('imports', title, open, { state: 'ready', fact: m.home_imports_checking() });
  if (source.count === 0) return view('imports', title, open, { state: 'zero', fact: null, scope: m.home_imports_zero() });
  return view('imports', title, open, {
    state: 'ready',
    fact: m.home_imports_review({ count: source.count, n: number(source.count, locale) }),
    warning: true,
    scope: m.home_imports_scope()
  });
}

function valuationRows(inputs: HomeValuationInput[]): WidgetRow[] {
  return inputs.slice(0, 3).map(input => ({ key: `${input.kind}-${input.commodity_id}`, label: input.code, detail: input.kind === 'rate' ? m.home_prices_rate() : m.home_prices_price(), amount: '' }));
}

/** The prices and rates net worth uses, as the briefing's status line names
 * them, linking to the same filtered Prices list. */
export function pricesWidget(home: Home, locale: string): WidgetView {
  const title = m.home_prices_title();
  const headline = home.headlines.find(item => item.id === 'net_worth');
  const missing = headline?.missing ?? [];
  const stale = headline?.stale ?? [];
  const open = {
    text: m.home_prices_open(),
    href: missing.length ? valuationHref(missing, 'missing') : stale.length ? valuationHref(stale, 'stale') : '/app/investments/prices'
  };
  if (home.empty_book) return noData('prices', title, open);
  if (!home.reporting_currency) {
    return view('prices', title, open, { state: 'not_set_up', fact: null, scope: m.home_widget_no_currency(), action: { text: m.home_widget_set_currency(), href: '/app/settings/currencies' } });
  }
  const more = (inputs: HomeValuationInput[]) => inputs.length > 3 ? m.home_prices_more({ count: inputs.length - 3, n: number(inputs.length - 3, locale) }) : null;
  if (missing.length) {
    return view('prices', title, open, { state: 'partial', fact: valuationText(missing, 'missing', locale), warning: true, scope: m.home_prices_scope(), rows: valuationRows(missing), more: more(missing) });
  }
  if (stale.length) {
    return view('prices', title, open, { state: 'stale', fact: valuationText(stale, 'stale', locale), warning: true, scope: m.home_prices_scope(), rows: valuationRows(stale), more: more(stale) });
  }
  return view('prices', title, open, { state: 'ready', fact: m.home_prices_current(), scope: m.home_prices_scope() });
}

/** Backups protect an empty book too, so this card never shows *no data*. */
export function backupsWidget(home: Home, locale: string): WidgetView {
  const title = m.home_backups_title();
  const open = { text: m.home_backups_open(), href: '/app/settings/data' };
  const widget = home.widgets.backups;
  const summary = widget.summary;
  if (widget.status === 'failed' || !summary) return failed('backups', title, open);
  const instant = (value: string) => new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
  if (!summary.enabled) {
    return view('backups', title, open, { state: 'not_set_up', fact: m.home_backups_off(), action: { text: m.home_backups_set_up(), href: '/app/settings/data' } });
  }
  const next = summary.next_run_at ? m.home_backups_next({ date: instant(summary.next_run_at) }) : null;
  const last = summary.last_success_at ? m.home_backups_last({ date: instant(summary.last_success_at) }) : null;
  if (summary.latest_status === 'failed') {
    return view('backups', title, open, {
      state: 'ready', fact: m.home_backups_failed(), warning: true,
      scope: [summary.latest_will_retry ? m.home_backups_retrying() : m.home_backups_retry_copy(), last].filter(Boolean).join(' · ')
    });
  }
  if (summary.latest_status === 'running' || summary.latest_status === 'pending') {
    return view('backups', title, open, { state: 'ready', fact: m.home_backups_running(), scope: last });
  }
  return view('backups', title, open, { state: 'ready', fact: last ?? m.home_backups_none(), scope: next });
}

const builders: Partial<Record<WidgetID, (home: Home, locale: string, attention: AttentionState) => WidgetView>> = {
  cashflow: cashflowWidget,
  holdings: holdingsWidget,
  returns: returnsWidget,
  allocation: allocationWidget,
  income: incomeWidget,
  forecast: forecastWidget,
  imports: importsWidget,
  prices: pricesWidget,
  backups: backupsWidget
};

export type ExploreGroup = { id: ExploreCategoryID; title: string; widgets: WidgetView[] };

export function exploreWidgets(home: Home, locale: string, attention: AttentionState): ExploreGroup[] {
  return exploreCategories.map(category => ({
    id: category.id,
    title: categoryTitle(category.id),
    widgets: category.widgets.flatMap(id => builders[id]?.(home, locale, attention) ?? [])
  }));
}

/** Titles for the loading state, so each card holds its place. */
export function exploreTitles(): { id: ExploreCategoryID; title: string; widgets: { id: WidgetID; title: string }[] }[] {
  const titles: Partial<Record<WidgetID, () => string>> = {
    cashflow: m.home_cashflow_label, holdings: m.home_holdings_title, returns: m.home_returns_title, allocation: m.home_allocation_title,
    income: m.home_income_title, forecast: m.home_forecast_title, imports: m.home_imports_title, prices: m.home_prices_title, backups: m.home_backups_title
  };
  return exploreCategories.map(category => ({
    id: category.id,
    title: categoryTitle(category.id),
    widgets: category.widgets.map(id => ({ id, title: titles[id]?.() ?? '' }))
  }));
}
