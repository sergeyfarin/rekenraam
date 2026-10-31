import { parseISO } from 'date-fns';
import type { Home, HomeCommodity, HomeWidgets } from '#lib/api/home.ts';
import { builtinCategoryLabel } from '#lib/categories/category-labels.ts';
import { formatHomeMoney, hiddenAmount, joinCommodityAmount } from '#lib/money/format.ts';
import { amountPrivacy } from '#lib/money/privacy.svelte.ts';
import { m } from '#lib/paraglide/messages.js';
import { seriesColumns } from '#lib/reports/report-series.ts';
import { spendingDrillDownHref, type SpendingRow } from '#lib/reports/spending.ts';

/**
 * The default My Home widgets (#478), as view models built from the one
 * composed Home read. Every figure is the canonical read's own (Go computed
 * it); this module only picks the words, links and Home rounding.
 */

export type WidgetID = keyof HomeWidgets;

/** The designed states (docs/design/home-and-navigation.md → Widgets). */
export type WidgetState = 'ready' | 'zero' | 'no_data' | 'not_set_up' | 'partial' | 'stale' | 'failed';

export type WidgetLink = { text: string; href: string };

export type WidgetRow = {
  key: string;
  label: string;
  href?: string;
  detail?: string;
  amount: string;
  /** Bar length as a fraction of the longest row, 0..1; absent when amounts are hidden. */
  bar?: number;
  /** Over budget: carried by the words too, never by colour alone. */
  over?: boolean;
};

export type WidgetView = {
  id: WidgetID;
  title: string;
  wide: boolean;
  state: WidgetState;
  /** The one key figure; null shows the state's text instead. */
  fact: string | null;
  /** Sign of a change figure, for its colour; 0 is neutral. */
  factSign?: number;
  /** The fact as read aloud when it differs from what is shown. */
  factLabel?: string;
  scope: string | null;
  rows: WidgetRow[];
  /** A count of what the rows leave out. */
  more: string | null;
  /** Points of a short chart, 0 (lowest) to 1 (highest); null is a gap. */
  chart: { points: (number | null)[]; label: string } | null;
  /** A not-set-up widget's first step. */
  action: WidgetLink | null;
  open: WidgetLink;
};

/** The default My Home order (design: Default My Home). */
export const defaultWidgetOrder: readonly WidgetID[] = ['spending', 'budgets', 'accounts', 'recurring', 'net_worth_trend'];

type Commodities = ReadonlyMap<number, HomeCommodity>;

function commodityMap(home: Home): Commodities {
  const map = new Map<number, HomeCommodity>();
  for (const commodity of home.commodities) map.set(commodity.commodity_id, commodity);
  if (home.reporting_currency) map.set(home.reporting_currency.commodity_id, home.reporting_currency);
  return map;
}

type Quantity = { commodity_id: number; quantity_value: string; quantity_scale: number };

function sign(value: string): number {
  return value.startsWith('-') ? -1 : /^0+$/.test(value) ? 0 : 1;
}

/** Home-rounded money with its symbol; a negative amount reads −€2,000. */
function money(quantity: Quantity, commodities: Commodities, locale: string, signed = false): string {
  if (amountPrivacy.hidden) return hiddenAmount;
  const commodity = commodities.get(quantity.commodity_id);
  const scale = commodity?.scale ?? quantity.quantity_scale;
  const value = quantity.quantity_value;
  const negative = value.startsWith('-');
  const magnitude = negative ? value.slice(1) : value;
  const shown = joinCommodityAmount(commodity ? commodity.symbol || commodity.code : '', formatHomeMoney(magnitude, quantity.quantity_scale, scale, locale));
  if (negative) return `−${shown}`;
  if (!signed) return shown;
  return `${sign(value) > 0 ? '+' : '±'}${shown}`;
}

function number(value: number, locale: string): string {
  return new Intl.NumberFormat(locale).format(value);
}

function day(date: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short' }).format(parseISO(date));
}

function range(start: string, end: string, locale: string, options: Intl.DateTimeFormatOptions): string {
  return new Intl.DateTimeFormat(locale, options).formatRange(parseISO(start), parseISO(end));
}

/** Bar lengths against the longest row, exact to the last digit (BigInt). */
function bars(quantities: (Quantity | null)[]): (number | undefined)[] {
  if (amountPrivacy.hidden) return quantities.map(() => undefined);
  const known = quantities.map((quantity, index) => ({ quantity, index })).filter((item): item is { quantity: Quantity; index: number } => item.quantity !== null);
  const columns = seriesColumns(known.map(({ quantity, index }) => ({ key: String(index), label: '', quantityValue: quantity.quantity_value, quantityScale: quantity.quantity_scale })));
  const result: (number | undefined)[] = quantities.map(() => undefined);
  columns.forEach((column, position) => (result[known[position].index] = Math.max(0, column.ratio)));
  return result;
}

function categoryLabel(category: { name: string; code: string; builtin_key: string | null }): string {
  return category.name.trim() || (category.builtin_key ? builtinCategoryLabel(category.builtin_key) : category.code.trim()) || m.reports_spending_unnamed();
}

function failed(id: WidgetID, title: string, open: WidgetLink, wide = false): WidgetView {
  return { id, title, wide, state: 'failed', fact: null, scope: m.home_widget_failed(), rows: [], more: null, chart: null, action: null, open };
}

function noData(id: WidgetID, title: string, open: WidgetLink, wide = false): WidgetView {
  return { id, title, wide, state: 'no_data', fact: m.home_widget_no_data(), scope: m.home_widget_no_data_copy(), rows: [], more: null, chart: null, action: null, open };
}

function reportQuery(start: string, end: string, home: Home): string {
  const params = new URLSearchParams({ start_date: start, end_date: end, bucket: 'month' });
  if (home.reporting_currency) params.set('reporting_currency_id', String(home.reporting_currency.commodity_id));
  return params.toString();
}

export function spendingWidget(home: Home, locale: string): WidgetView {
  const title = m.home_spending_title();
  const widget = home.widgets.spending;
  const monthStart = `${home.as_of.slice(0, 8)}01`;
  const open = { text: m.home_spending_open(), href: `/app/reports/spending?${reportQuery(widget.summary?.start_date ?? monthStart, widget.summary?.end_date ?? home.as_of, home)}` };
  if (home.empty_book) return noData('spending', title, open);
  const summary = widget.summary;
  if (widget.status === 'failed' || !summary) return failed('spending', title, open);
  const commodities = commodityMap(home);
  const currency = home.reporting_currency?.code ?? '';
  const period = range(summary.start_date, summary.end_date, locale, { day: 'numeric', month: 'short' });
  const rowAmounts = summary.categories.map(category => category.converted ?? (category.totals.length === 1 ? category.totals[0] : null));
  const lengths = bars(rowAmounts);
  const rows = summary.categories.map((category, index): WidgetRow => {
    const amount = rowAmounts[index];
    const drillDown: SpendingRow = {
      key: String(category.category_id), categoryID: category.category_id, code: category.code, name: category.name, commodityID: 0,
      quantityValue: '0', quantityScale: 0, unattributed: false, converted: false,
      drillDown: { start_date: summary.start_date, end_date: summary.end_date, category_id: category.category_id, account_ids: [] }
    };
    return {
      key: String(category.category_id),
      label: categoryLabel(category),
      href: spendingDrillDownHref(drillDown, 'category', 'spending'),
      amount: amount ? money(amount, commodities, locale) : category.totals.map(total => money(total, commodities, locale)).join(' · '),
      bar: lengths[index]
    };
  });
  const rest = summary.category_count - summary.categories.length;
  const more = rest > 0 ? m.home_spending_more({ count: rest, n: number(rest, locale) }) : null;
  const base = { id: 'spending' as const, title, wide: false, rows, more, chart: null, action: null, open };
  if (summary.total) {
    const zero = summary.categories.length === 0 && sign(summary.total.quantity_value) === 0;
    return { ...base, state: zero ? 'zero' : 'ready', fact: money(summary.total, commodities, locale), scope: zero ? m.home_spending_zero() : m.home_spending_scope({ period, currency }) };
  }
  if (summary.commodity_totals.length === 0) {
    return { ...base, state: 'zero', fact: null, scope: m.home_spending_zero() };
  }
  // No total in one currency: each currency's exact total, never a partial sum.
  return {
    ...base,
    state: 'partial',
    fact: summary.commodity_totals.map(total => money(total, commodities, locale)).join(' · '),
    scope: home.reporting_currency ? m.home_spending_partial({ currency }) : m.home_widget_no_currency()
  };
}

export function budgetsWidget(home: Home, locale: string): WidgetView {
  const widget = home.widgets.budgets;
  const monthDate = widget.summary?.period_start ?? `${home.as_of.slice(0, 8)}01`;
  const title = m.home_budgets_title({ month: new Intl.DateTimeFormat(locale, { month: 'long' }).format(parseISO(monthDate)) });
  const params = new URLSearchParams({ month: monthDate.slice(0, 7) });
  if (widget.summary?.commodity_id) params.set('currency_id', String(widget.summary.commodity_id));
  const open = { text: m.home_budgets_open(), href: `/app/planning/budgets?${params.toString()}` };
  if (home.empty_book) return noData('budgets', title, open);
  const summary = widget.summary;
  if (widget.status === 'failed' || !summary) return failed('budgets', title, open);
  if (!summary.commodity_id || !summary.total) {
    return { id: 'budgets', title, wide: false, state: 'not_set_up', fact: m.home_budgets_not_set_up(), scope: null, rows: [], more: null, chart: null, action: { text: m.home_budgets_set_up(), href: open.href }, open };
  }
  const commodities = commodityMap(home);
  const total = summary.total;
  const left = sign(total.remaining.quantity_value) >= 0;
  const remaining = money({ ...total.remaining, quantity_value: total.remaining.quantity_value.replace(/^-/, '') }, commodities, locale);
  const scopeParts = [m.home_budgets_spent({ actual: money(total.actual, commodities, locale), target: money(total.target, commodities, locale) })];
  if (summary.other_commodity_count > 0) scopeParts.push(m.home_budgets_other_currencies({ count: summary.other_commodity_count, n: number(summary.other_commodity_count, locale) }));
  const rows = summary.categories.map((category): WidgetRow => {
    const over = sign(category.amount.remaining.quantity_value) < 0;
    return {
      key: String(category.category_id),
      label: categoryLabel(category),
      amount: m.home_budgets_row({ actual: money(category.amount.actual, commodities, locale), target: money(category.amount.target, commodities, locale) }),
      detail: over ? m.home_budgets_row_over({ amount: money({ ...category.amount.remaining, quantity_value: category.amount.remaining.quantity_value.slice(1) }, commodities, locale) }) : undefined,
      over
    };
  });
  const rest = summary.category_count - summary.categories.length;
  return {
    id: 'budgets', title, wide: false, state: 'ready',
    fact: left ? m.home_budgets_left({ amount: remaining }) : m.home_budgets_over({ amount: remaining }),
    scope: scopeParts.join(' · '), rows,
    more: rest > 0 ? m.home_budgets_more({ count: rest, n: number(rest, locale) }) : null,
    chart: null, action: null, open
  };
}

export function accountsWidget(home: Home, locale: string): WidgetView {
  const title = m.home_accounts_title();
  const open = { text: m.home_accounts_open(), href: '/app/ledger/accounts' };
  if (home.empty_book) return noData('accounts', title, open);
  const widget = home.widgets.accounts;
  const summary = widget.summary;
  if (widget.status === 'failed' || !summary) return failed('accounts', title, open);
  const commodities = commodityMap(home);
  const rows = summary.accounts.map((account): WidgetRow => {
    // A balance in one currency shows in that currency, as its register does;
    // anything else (a brokerage with holdings) shows its value, or says it has none.
    const only = account.balances.length === 1 ? account.balances[0] : null;
    const native = only && commodities.get(only.commodity_id)?.kind === 'currency' ? only : null;
    const shown = native ?? account.value;
    return {
      key: String(account.account_id),
      label: account.name.trim() || account.code.trim() || m.accounts_unnamed_account({ id: account.account_id }),
      href: `/app/ledger/accounts/${account.account_id}/register`,
      amount: shown ? money(shown, commodities, locale) : account.balances.length === 0 ? money({ commodity_id: home.reporting_currency?.commodity_id ?? 0, quantity_value: '0', quantity_scale: 0 }, commodities, locale) : m.home_accounts_no_value()
    };
  });
  const rest = summary.open_count - summary.accounts.length;
  const partial = summary.accounts.some(account => account.value === null && account.balances.length > 0);
  return {
    id: 'accounts', title, wide: false,
    state: partial ? 'partial' : 'ready',
    fact: m.home_accounts_count({ count: summary.open_count, n: number(summary.open_count, locale) }),
    scope: m.home_accounts_scope({ date: day(summary.as_of, locale) }),
    rows, more: rest > 0 ? m.home_accounts_more({ count: rest, n: number(rest, locale) }) : null,
    chart: null, action: null, open
  };
}

export function recurringWidget(home: Home, locale: string): WidgetView {
  const title = m.home_recurring_title();
  const widget = home.widgets.recurring;
  const summary = widget.summary;
  // Drafts waiting lead to the Due list; otherwise the schedules themselves.
  const open = { text: m.home_recurring_open(), href: summary && summary.draft_count + summary.blocked_count > 0 ? '/app/planning/recurring' : '/app/planning/recurring?tab=templates' };
  if (home.empty_book) return noData('recurring', title, open);
  if (widget.status === 'failed' || !summary) return failed('recurring', title, open);
  if (summary.active_count === 0 && summary.draft_count === 0) {
    return { id: 'recurring', title, wide: false, state: 'not_set_up', fact: m.home_recurring_not_set_up(), scope: null, rows: [], more: null, chart: null, action: { text: m.home_recurring_set_up(), href: '/app/planning/recurring?tab=templates' }, open };
  }
  const commodities = commodityMap(home);
  const next = summary.upcoming[0];
  const scope = [m.home_recurring_active({ count: summary.active_count, n: number(summary.active_count, locale) })];
  if (summary.draft_count > 0) scope.push(m.home_recurring_drafts({ count: summary.draft_count, n: number(summary.draft_count, locale) }));
  return {
    id: 'recurring', title, wide: false, state: 'ready',
    fact: next ? m.home_recurring_next({ name: next.name, date: day(next.next_due_on, locale) }) : m.home_recurring_none_due(),
    scope: scope.join(' · '),
    rows: summary.upcoming.map(item => ({ key: String(item.template_id), label: item.name, detail: day(item.next_due_on, locale), amount: item.amounts.map(amount => money(amount, commodities, locale)).join(' · ') })),
    more: null, chart: null, action: null, open
  };
}

/** Each point's height between the series' lowest and highest value, exactly. */
export function trendHeights(values: (Quantity | null)[]): (number | null)[] {
  const known = values.filter((value): value is Quantity => value !== null);
  if (known.length === 0) return values.map(() => null);
  const scale = Math.max(...known.map(value => value.quantity_scale));
  const exact = (value: Quantity) => BigInt(value.quantity_value) * 10n ** BigInt(scale - value.quantity_scale);
  const all = known.map(exact);
  const low = all.reduce((a, b) => (b < a ? b : a));
  const high = all.reduce((a, b) => (b > a ? b : a));
  const span = high - low;
  const precision = 1_000_000n;
  return values.map(value => {
    if (value === null) return null;
    if (span === 0n) return 0.5;
    return Number(((exact(value) - low) * precision) / span) / Number(precision);
  });
}

export function netWorthTrendWidget(home: Home, locale: string): WidgetView {
  const title = m.home_trend_title();
  const widget = home.widgets.net_worth_trend;
  const summary = widget.summary;
  const open = { text: m.home_trend_open(), href: `/app/reports/net-worth?${reportQuery(summary?.start_date ?? home.start_boundary, summary?.end_date ?? home.as_of, home)}` };
  if (home.empty_book) return noData('net_worth_trend', title, open, true);
  if (widget.status === 'failed' || !summary) return failed('net_worth_trend', title, open, true);
  const base = { id: 'net_worth_trend' as const, title, wide: true, rows: [], more: null, open };
  if (!home.reporting_currency || summary.points.length === 0) {
    return { ...base, state: 'not_set_up', fact: null, scope: m.home_widget_no_currency(), chart: null, action: { text: m.home_widget_set_currency(), href: '/app/settings/currencies' } };
  }
  const commodities = commodityMap(home);
  const currency = home.reporting_currency.code;
  const period = range(summary.start_date, summary.end_date, locale, { month: 'short', year: 'numeric' });
  const last = summary.points.at(-1)?.value ?? null;
  if (!last) {
    return { ...base, state: 'partial', fact: m.home_trend_unavailable(), scope: m.home_trend_unavailable_copy(), chart: null, action: null };
  }
  const heights = trendHeights(summary.points.map(point => point.value));
  const first = summary.points[0].value;
  const chart = amountPrivacy.hidden ? null : {
    points: heights,
    label: m.home_trend_chart_label({ start: first ? money(first, commodities, locale) : '—', end: money(last, commodities, locale) })
  };
  const stale = home.headlines.find(headline => headline.id === 'net_worth')?.stale.length ?? 0;
  const scope = [m.home_trend_scope({ period, currency })];
  if (stale > 0) scope.push(m.home_trend_stale({ count: stale, n: number(stale, locale) }));
  const change = summary.change;
  return {
    ...base,
    state: stale > 0 ? 'stale' : 'ready',
    fact: change ? money(change, commodities, locale, true) : money(last, commodities, locale),
    factSign: change && !amountPrivacy.hidden ? sign(change.quantity_value) : 0,
    factLabel: change ? m.home_trend_change_label({ amount: money(change, commodities, locale, true), date: day(summary.start_date, locale) }) : undefined,
    scope: scope.join(' · '),
    chart, action: null
  };
}

const builders: Record<WidgetID, (home: Home, locale: string) => WidgetView> = {
  spending: spendingWidget,
  budgets: budgetsWidget,
  accounts: accountsWidget,
  recurring: recurringWidget,
  net_worth_trend: netWorthTrendWidget
};

/** The default My Home, in its fixed order (personalization is #438). */
export function homeWidgets(home: Home, locale: string): WidgetView[] {
  return defaultWidgetOrder.map(id => builders[id](home, locale));
}

/** Titles for the loading state, so each card holds its place. */
export function widgetTitles(): { id: WidgetID; title: string; wide: boolean }[] {
  return [
    { id: 'spending', title: m.home_spending_title(), wide: false },
    { id: 'budgets', title: m.budgets_title(), wide: false },
    { id: 'accounts', title: m.home_accounts_title(), wide: false },
    { id: 'recurring', title: m.home_recurring_title(), wide: false },
    { id: 'net_worth_trend', title: m.home_trend_title(), wide: true }
  ];
}
