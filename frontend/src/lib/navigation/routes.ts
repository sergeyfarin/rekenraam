import type { AttentionSourceID } from '#lib/api/attention.ts';
import type { RouteIcon } from './icons';
import { m } from '#lib/paraglide/messages.js';

/**
 * The one route registry (#436, docs/design/home-and-navigation.md → Pages and
 * routes). The header, each section's tab row, the phone menu, the settings
 * menu and side list, and document titles all read it, so a page cannot be
 * reachable from one and missing from another. `routes.test.ts` fails when a
 * `+page.svelte` route is missing from it.
 */

export type PageWidth = 'narrow' | 'standard' | 'fluid';

export type SectionID = 'home' | 'review' | 'ledger' | 'investments' | 'planning' | 'reports' | 'settings';

// Keep static message references so bundling can discard unrelated catalogs.
const titles = {
  attention_review: m.attention_review,
  nav_home: m.nav_home,
  nav_overview: m.nav_overview,
  accounts_title: m.accounts_title,
  transactions_page_title: m.transactions_page_title,
  categories_title: m.categories_title,
  import_title: m.import_title,
  import_dividend_title: m.import_dividend_title,
  reconcile_title: m.reconcile_title,
  quotes_instruments: m.quotes_instruments,
  nav_returns: m.nav_returns,
  nav_income: m.nav_income,
  investments_tab_gains: m.investments_tab_gains,
  nav_prices: m.nav_prices,
  budgets_title: m.budgets_title,
  recurring_title: m.recurring_title,
  forecast_nav: m.forecast_nav,
  reports_view_spending: m.reports_view_spending,
  reports_view_cashflow: m.reports_view_cashflow,
  reports_view_net_worth: m.reports_view_net_worth,
  reports_export_title: m.reports_export_title,
  settings_nav_general: m.settings_nav_general,
  settings_nav_preferences: m.settings_nav_preferences,
  settings_nav_currencies: m.settings_nav_currencies,
  settings_data_title: m.settings_data_title,
  settings_nav_security: m.settings_nav_security,
};
export type TitleKey = keyof typeof titles;

export type RouteEntry = {
  titleKey: TitleKey;
  icon: RouteIcon;
  /** Localized search terms; shared search consumes these in #457. */
  keywords: readonly TitleKey[];
  /**
   * The page column's width (#459): narrow about 720px for forms, standard
   * about 1200px, fluid up to the shell's 1600px for registers and lists. The
   * column keeps the shell's left edge, so a narrow page never re-centres.
   */
  width: PageWidth;
  /** The SvelteKit route id: `/app/ledger/accounts/[id]/register`. */
  route: string;
  section: SectionID;
  title: () => string;
  /** Listed in its section's tab row (or the settings side list and menu). */
  tab?: true;
  standalone?: true;
  attention?: readonly AttentionSourceID[];
  /** A detail page marks this tab current: an account register marks Accounts. */
  parent?: string;
  /**
   * Tabs sharing a query group carry the current query string between them, so
   * moving from Spending to Cashflow keeps the period, accounts and currency.
   */
  queryGroup?: 'reports';
  /** The page renders its own `PageHeader`; the shell adds no second h1. */
  ownHeader?: true;
  /** Kept only to redirect an old link (see `canonicalURL`); never listed. */
  legacy?: true;
};

export type Section = {
  id: Exclude<SectionID, 'home' | 'settings' | 'review'>;
  title: () => string;
  /** The section link opens this page, the first tab. */
  overview: string;
};

export const sections: readonly Section[] = [
  { id: 'ledger', title: () => m.nav_ledger(), overview: '/app/ledger' },
  { id: 'investments', title: () => m.investments_nav(), overview: '/app/investments' },
  { id: 'planning', title: () => m.nav_planning(), overview: '/app/planning' },
  { id: 'reports', title: () => m.reports_nav(), overview: '/app/reports' }
];

// Order within a section is tab order. The Ledger, Planning and Reports
// overviews are placeholders until #448 and #450; Accounts stays a Ledger tab
// until #448 folds it into the Ledger overview.
const definitions: readonly Omit<RouteEntry, 'title' | 'keywords'>[] = [
  { route: '/app/review', section: 'review', titleKey: 'attention_review', icon: 'scan-line', width: 'standard', standalone: true },
  { route: '/app', section: 'home', titleKey: 'nav_home', icon: 'house', width: 'fluid', ownHeader: true },

  { route: '/app/ledger', section: 'ledger', titleKey: 'nav_overview', icon: 'landmark', width: 'standard', tab: true },
  { route: '/app/ledger/accounts', section: 'ledger', titleKey: 'accounts_title', icon: 'landmark', width: 'standard', tab: true },
  { route: '/app/ledger/accounts/[id]/register', section: 'ledger', titleKey: 'accounts_title', icon: 'list', width: 'fluid', parent: '/app/ledger/accounts', ownHeader: true },
  { route: '/app/ledger/transactions', attention: ['transaction_review'], section: 'ledger', titleKey: 'transactions_page_title', icon: 'list', width: 'fluid', tab: true, ownHeader: true },
  { route: '/app/ledger/categories', section: 'ledger', titleKey: 'categories_title', icon: 'tags', width: 'standard', tab: true },
  { route: '/app/ledger/categories/[id]', section: 'ledger', titleKey: 'categories_title', icon: 'tags', width: 'fluid', parent: '/app/ledger/categories', ownHeader: true },
  { route: '/app/ledger/import', attention: ['imports', 'connections'], section: 'ledger', titleKey: 'import_title', icon: 'download', width: 'fluid', tab: true },
  { route: '/app/ledger/import/dividends', section: 'ledger', titleKey: 'import_dividend_title', icon: 'download', width: 'fluid', parent: '/app/ledger/import', ownHeader: true },
  { route: '/app/ledger/reconcile', section: 'ledger', titleKey: 'reconcile_title', icon: 'scan-line', width: 'fluid', tab: true },

  { route: '/app/investments', attention: ['suggestions'], section: 'investments', titleKey: 'nav_overview', icon: 'chart-no-axes-combined', width: 'standard', tab: true },
  { route: '/app/investments/instruments', section: 'investments', titleKey: 'quotes_instruments', icon: 'shapes', width: 'standard', tab: true },
  { route: '/app/investments/instruments/[id]', section: 'investments', titleKey: 'quotes_instruments', icon: 'shapes', width: 'standard', parent: '/app/investments/instruments' },
  { route: '/app/investments/returns', section: 'investments', titleKey: 'nav_returns', icon: 'trending-up', width: 'standard', tab: true, ownHeader: true },
  { route: '/app/investments/income', section: 'investments', titleKey: 'nav_income', icon: 'coins', width: 'standard', tab: true, ownHeader: true },
  { route: '/app/investments/gains', section: 'investments', titleKey: 'investments_tab_gains', icon: 'chart-line', width: 'standard', tab: true },
  { route: '/app/investments/prices', attention: ['prices', 'refresh'], section: 'investments', titleKey: 'nav_prices', icon: 'coins', width: 'standard', tab: true },
  { route: '/app/pricing', section: 'investments', titleKey: 'nav_prices', icon: 'coins', width: 'standard', legacy: true },

  { route: '/app/planning', section: 'planning', titleKey: 'nav_overview', icon: 'calendar-days', width: 'standard', tab: true },
  { route: '/app/planning/budgets', section: 'planning', titleKey: 'budgets_title', icon: 'wallet', width: 'standard', tab: true, ownHeader: true },
  { route: '/app/planning/recurring', attention: ['drafts', 'recurring_blocked'], section: 'planning', titleKey: 'recurring_title', icon: 'repeat', width: 'standard', tab: true },
  { route: '/app/planning/forecast', section: 'planning', titleKey: 'forecast_nav', icon: 'calendar-range', width: 'fluid', tab: true },

  { route: '/app/reports', section: 'reports', titleKey: 'nav_overview', icon: 'chart-pie', width: 'standard', tab: true },
  { route: '/app/reports/spending', section: 'reports', titleKey: 'reports_view_spending', icon: 'chart-pie', width: 'standard', tab: true, queryGroup: 'reports' },
  { route: '/app/reports/cashflow', section: 'reports', titleKey: 'reports_view_cashflow', icon: 'arrow-left-right', width: 'standard', tab: true, queryGroup: 'reports' },
  { route: '/app/reports/net-worth', section: 'reports', titleKey: 'reports_view_net_worth', icon: 'chart-line', width: 'standard', tab: true, queryGroup: 'reports' },
  { route: '/app/reports/export', section: 'reports', titleKey: 'reports_export_title', icon: 'download', width: 'standard', tab: true },

  { route: '/app/settings', section: 'settings', titleKey: 'settings_nav_general', icon: 'settings-2', width: 'standard', tab: true },
  { route: '/app/settings/preferences', section: 'settings', titleKey: 'settings_nav_preferences', icon: 'palette', width: 'standard', tab: true },
  { route: '/app/settings/currencies', section: 'settings', titleKey: 'settings_nav_currencies', icon: 'coins', width: 'standard', tab: true },
  { route: '/app/settings/security', section: 'settings', titleKey: 'settings_nav_security', icon: 'shield-check', width: 'standard', tab: true },
  { route: '/app/settings/data', section: 'settings', titleKey: 'settings_data_title', icon: 'database', width: 'standard', tab: true },

  // Settings moved pages (#446). They redirect before rendering; the stub
  // pages keep old bookmarks inside the app shell.
  { route: '/app/settings/appearance', section: 'settings', titleKey: 'settings_nav_preferences', icon: 'palette', width: 'standard', legacy: true },
  { route: '/app/settings/language', section: 'settings', titleKey: 'settings_nav_preferences', icon: 'palette', width: 'standard', legacy: true },
  { route: '/app/settings/trash', section: 'settings', titleKey: 'settings_data_title', icon: 'database', width: 'standard', legacy: true }
];

// Keep the old page families until old bookmarks no longer need them (#460).
// Accounts remains a tab until #448 folds its list into the Ledger overview;
// registers already have their lasting section-owned path.
const sectionRouteMoves = [
  ['/app/accounts', '/app/ledger/accounts'],
  ['/app/transactions', '/app/ledger/transactions'],
  ['/app/categories', '/app/ledger/categories'],
  ['/app/import', '/app/ledger/import'],
  ['/app/reconcile', '/app/ledger/reconcile'],
  ['/app/budgets', '/app/planning/budgets'],
  ['/app/recurring', '/app/planning/recurring'],
  ['/app/forecast', '/app/planning/forecast']
] as const;

const legacyDefinitions = definitions.flatMap((entry) => {
  const move = sectionRouteMoves.find(([, target]) => entry.route === target || entry.route.startsWith(`${target}/`));
  if (!move) return [];
  // Legacy paths only redirect, never appear as tabs or search results.
  return [{
    route: move[0] + entry.route.slice(move[1].length),
    section: entry.section,
    titleKey: entry.titleKey,
    icon: entry.icon,
    width: entry.width,
    legacy: true as const
  }];
});

export const routes: readonly RouteEntry[] = [...definitions, ...legacyDefinitions].map((entry) => ({
  ...entry,
  title: () => titles[entry.titleKey](),
  keywords: [entry.titleKey]
}));

const patterns = routes.map((entry) => ({
  entry,
  pattern: new RegExp(`^${entry.route.replace(/\[[^\]]+\]/g, '[^/]+')}/?$`)
}));

/** Resolve registry search aliases at render time in the active locale. */
export function routeSearchKeywords(entry: RouteEntry): string[] {
  return entry.keywords.map(key => titles[key]());
}

/** The registry entry for a pathname, or undefined outside the registry. */
export function routeFor(pathname: string): RouteEntry | undefined {
  return patterns.find(({ pattern }) => pattern.test(pathname))?.entry;
}

export function sectionTabs(section: SectionID): RouteEntry[] {
  return routes.filter((entry) => entry.section === section && entry.tab);
}

export function sectionFor(id: SectionID): Section | undefined {
  return sections.find((section) => section.id === id);
}

export type Crumb = { label: string; href: string };

/**
 * A detail page's breadcrumb: the tab it belongs to (*Accounts › Checking*).
 * The object hierarchy (*Categories › Housing › Energy*) extends it in #442.
 */
export function breadcrumbs(entry: RouteEntry | undefined): Crumb[] {
  const parent = entry?.parent ? routes.find((route) => route.route === entry.parent) : undefined;
  return parent ? [{ label: parent.title(), href: parent.route }] : [];
}

/** The tab a page marks current: itself, or the tab it is a detail of. */
export function currentTab(entry: RouteEntry | undefined): string | undefined {
  return entry?.parent ?? (entry?.tab ? entry.route : undefined);
}

/**
 * A tab's href from the current URL. Tabs in the same query group keep the
 * query string, so a report's period and filters survive a switch of report.
 */
export function tabHref(target: RouteEntry, current: RouteEntry | undefined, search: string): string {
  if (target.queryGroup && target.queryGroup === current?.queryGroup && search) {
    return `${target.route}${search}`;
  }
  return target.route;
}

const reportViews = new Set(['net-worth', 'spending', 'cashflow']);

/**
 * Where an old link belongs now, or null when the URL is already canonical.
 * Every parameter except the one the path now carries is kept, so a shared
 * link keeps its period, accounts, currency and saved run.
 */
export function canonicalURL(url: Pick<URL, 'href' | 'pathname' | 'search'>): URL | null {
  const params = new URLSearchParams(url.search);
  const move = (pathname: string, drop?: string) => {
    if (drop) params.delete(drop);
    const next = new URL(url.href);
    next.pathname = pathname;
    next.search = params.toString();
    return next;
  };
  const path = url.pathname.replace(/\/$/, '') || '/';

  if (routeFor(path)?.legacy) {
    const moved = sectionRouteMoves.find(([from]) => path === from || path.startsWith(`${from}/`));
    if (moved) return move(moved[1] + path.slice(moved[0].length));
  }

  if (path === '/app/pricing') return move('/app/investments/prices');

  // Settings consolidation (#446): appearance and language are one page, and
  // Trash is a view of Transactions. Keep every other parameter.
  if (path === '/app/settings/appearance' || path === '/app/settings/language') {
    return move('/app/settings/preferences');
  }
  if (path === '/app/settings/trash') {
    params.set('view', 'deleted');
    return move('/app/ledger/transactions');
  }
  // An old link that opened Export inside Settings → Data now belongs on its
  // own Reports page.
  if (path === '/app/settings/data') {
    if (params.get('view') === 'export' || params.get('export') === 'true') {
      params.delete('view');
      params.delete('export');
      return move('/app/reports/export');
    }
  }

  // `/app/reports` was the net-worth report; it is now the Reports overview.
  // A link carrying any report state is a report link, so it keeps going to
  // the report it named.
  if (path === '/app/reports' && params.size > 0) {
    const view = params.get('view') ?? '';
    return move(`/app/reports/${reportViews.has(view) ? view : 'net-worth'}`, 'view');
  }

  if (path === '/app/investments') {
    const tab = params.get('tab');
    if (tab === 'instruments') return move('/app/investments/instruments', 'tab');
    if (tab === 'gains') return move('/app/investments/gains', 'tab');
  }

  return null;
}
