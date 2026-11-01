import { readdirSync } from 'node:fs';
import { join, relative, sep } from 'node:path';
import { describe, expect, it } from 'vitest';
import { breadcrumbs, canonicalURL, currentTab, routeFor, routeSearchKeywords, routes, sectionTabs, sections, tabHref } from './routes';

const routesDir = join(import.meta.dirname, '../../routes');

/** Every `+page.svelte` under src/routes/app as a SvelteKit route id. */
function pageRoutes(dir = join(routesDir, 'app')): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return pageRoutes(path);
    if (entry.name !== '+page.svelte') return [];
    const route = relative(routesDir, dir).split(sep).join('/');
    return [`/${route}`];
  });
}

function url(path: string): URL {
  return new URL(path, 'https://rekenraam.test');
}

describe('route registry', () => {
  it('lists every /app page and nothing that does not exist', () => {
    const pages = pageRoutes().sort();
    const registered = routes.map((entry) => entry.route).sort();
    expect(registered).toEqual(pages);
  });

  it('gives a detail page the tab it belongs to as its breadcrumb', () => {
    expect(breadcrumbs(routeFor('/app/ledger/accounts/7/register'))).toEqual([{ label: 'Accounts', href: '/app/ledger/accounts' }]);
    expect(breadcrumbs(routeFor('/app/ledger/categories/3'))).toEqual([{ label: 'Categories', href: '/app/ledger/categories' }]);
    expect(breadcrumbs(routeFor('/app/ledger/transactions'))).toEqual([]);
  });

  it('opens each section on its own first tab', () => {
    for (const section of sections) {
      const [first] = sectionTabs(section.id);
      expect(first?.route).toBe(section.overview);
      expect(routeFor(section.overview)?.section).toBe(section.id);
    }
  });

  it('keeps the old page names as search aliases for Preferences', () => {
    const keywords = routeSearchKeywords(routeFor('/app/settings/preferences')!);
    expect(keywords).toEqual(expect.arrayContaining(['Preferences', 'Language', 'Appearance']));
  });

  it('lists the five settings pages in the agreed order', () => {
    expect(sectionTabs('settings').map((entry) => entry.route)).toEqual([
      '/app/settings',
      '/app/settings/preferences',
      '/app/settings/currencies',
      '/app/settings/security',
      '/app/settings/data'
    ]);
  });

  it('keeps a detail page parent inside its own section', () => {
    for (const entry of routes.filter((item) => item.parent)) {
      const parent = routeFor(entry.parent!);
      expect(parent?.tab, entry.route).toBe(true);
      expect(parent?.section, entry.route).toBe(entry.section);
    }
  });

  it('never lists a legacy route as a tab', () => {
    expect(routes.filter((entry) => entry.legacy && entry.tab)).toEqual([]);
  });

  it('keeps each section page under its section path', () => {
    for (const entry of routes.filter((item) => !item.legacy && ['ledger', 'planning'].includes(item.section))) {
      expect(entry.route.startsWith(`/app/${entry.section}`), entry.route).toBe(true);
    }
  });

  it('gives every route localized search and presentation metadata', () => {
    for (const entry of routes) {
      expect(entry.titleKey, entry.route).toBeTruthy();
      expect(entry.title(), entry.route).not.toBe('');
      expect(entry.icon, entry.route).toBeDefined();
      expect(entry.keywords, entry.route).toContain(entry.titleKey);
      expect(['narrow', 'standard', 'fluid'], entry.route).toContain(entry.width);
      expect(entry.legacy || entry.route === '/app' || entry.tab || entry.parent || entry.standalone, entry.route).toBeTruthy();
    }
  });

  it('matches dynamic segments and marks the parent tab current', () => {
    expect(routeFor('/app/ledger/accounts/12/register')?.route).toBe('/app/ledger/accounts/[id]/register');
    expect(currentTab(routeFor('/app/ledger/accounts/12/register'))).toBe('/app/ledger/accounts');
    expect(currentTab(routeFor('/app/investments/instruments/7'))).toBe('/app/investments/instruments');
    expect(routeFor('/app/ledger/accounts/12')).toBeUndefined();
    expect(routeFor('/app/reports/spending/')?.route).toBe('/app/reports/spending');
  });

  it('carries report filters between reports and nowhere else', () => {
    const spending = routeFor('/app/reports/spending');
    const cashflow = routeFor('/app/reports/cashflow')!;
    const overview = routeFor('/app/reports')!;
    expect(tabHref(cashflow, spending, '?start_date=2026-03-01&account_id=4')).toBe(
      '/app/reports/cashflow?start_date=2026-03-01&account_id=4'
    );
    expect(tabHref(overview, spending, '?start_date=2026-03-01')).toBe('/app/reports');
    expect(tabHref(cashflow, routeFor('/app/planning/forecast'), '?horizon_days=30')).toBe('/app/reports/cashflow');
  });
});

describe('legacy links', () => {
  // Include detail routes and duplicates/empty/encoded/unknown parameters: a
  // redirect must preserve the entire URL state, not a known filter allowlist.
  it.each([
    ['/app/accounts', '/app/ledger/accounts'],
    ['/app/accounts/12/register', '/app/ledger/accounts/12/register'],
    ['/app/transactions', '/app/ledger/transactions'],
    ['/app/categories', '/app/ledger/categories'],
    ['/app/categories/7', '/app/ledger/categories/7'],
    ['/app/import', '/app/ledger/import'],
    ['/app/import/dividends', '/app/ledger/import/dividends'],
    ['/app/reconcile', '/app/ledger/reconcile'],
    ['/app/budgets', '/app/planning/budgets'],
    ['/app/recurring', '/app/planning/recurring'],
    ['/app/forecast', '/app/planning/forecast'],
    ['/app/settings/appearance', '/app/settings/preferences'],
    ['/app/settings/language', '/app/settings/preferences']
  ])('preserves every parameter and fragment from %s', (from, to) => {
    for (const suffix of ['', '/']) {
      const original = url(`${from}${suffix}?account_id=1&account_id=2&empty=&q=a%2Bb+%26+c&view=drafts&future=unknown#detail`);
      const next = canonicalURL(original)!;
      expect(routeFor(from)?.legacy).toBe(true);
      expect(next.pathname).toBe(to);
      expect([...next.searchParams]).toEqual([...original.searchParams]);
      expect(next.hash).toBe('#detail');
      expect(canonicalURL(next)).toBeNull();
      expect(routeFor(next.pathname)?.legacy).toBeUndefined();
    }
  });

  it('gives every legacy entry an existing canonical destination', () => {
    for (const entry of routes.filter((item) => item.legacy)) {
      const next = canonicalURL(url(entry.route.replace('[id]', '12')));
      expect(next, entry.route).not.toBeNull();
      expect(routeFor(next!.pathname)?.legacy, entry.route).toBeUndefined();
      expect(routeFor(next!.pathname), entry.route).toBeDefined();
    }
  });

  it('moves Trash to the Deleted transaction view without losing URL state', () => {
    const next = canonicalURL(url('/app/settings/trash?empty=&future=a%2Bb#detail'))!;
    expect(next.pathname).toBe('/app/ledger/transactions');
    expect([...next.searchParams]).toEqual([
      ['empty', ''],
      ['future', 'a+b'],
      ['view', 'deleted']
    ]);
    expect(next.hash).toBe('#detail');
    expect(canonicalURL(next)).toBeNull();
  });

  it('moves an old export deep link to Reports → Export', () => {
    for (const from of [
      '/app/settings/data?view=export&q=kept',
      '/app/settings/data?export=true&q=kept'
    ]) {
      const next = canonicalURL(url(from))!;
      expect(`${next.pathname}${next.search}`).toBe('/app/reports/export?q=kept');
      expect(canonicalURL(next)).toBeNull();
    }
    // The Data and backups page itself stays where it is.
    expect(canonicalURL(url('/app/settings/data'))).toBeNull();
  });

  it.each([
    ['/app/pricing?base_commodity_id=3&quote_commodity_id=1', '/app/investments/prices?base_commodity_id=3&quote_commodity_id=1'],
    [
      '/app/reports?view=spending&group_by=payee&account_id=1&account_id=2&reporting_currency_id=5',
      '/app/reports/spending?group_by=payee&account_id=1&account_id=2&reporting_currency_id=5'
    ],
    ['/app/reports?view=cashflow&start_date=2026-03-01&end_date=2026-03-31', '/app/reports/cashflow?start_date=2026-03-01&end_date=2026-03-31'],
    // The old page defaulted to net worth, so a link without a view meant it.
    ['/app/reports?start_date=2020-01-01&end_date=2020-02-29&bucket=day', '/app/reports/net-worth?start_date=2020-01-01&end_date=2020-02-29&bucket=day'],
    ['/app/reports?view=unknown&run_id=4', '/app/reports/net-worth?run_id=4'],
    ['/app/investments?tab=instruments', '/app/investments/instruments'],
    ['/app/investments?tab=gains&run_id=9', '/app/investments/gains?run_id=9']
  ])('%s reaches %s', (from, to) => {
    const next = canonicalURL(url(from));
    expect(next && `${next.pathname}${next.search}`).toBe(to);
  });

  it('keeps the hash', () => {
    expect(canonicalURL(url('/app/pricing#form'))?.hash).toBe('#form');
  });

  it.each([
    '/app/reports',
    '/app/reports/spending?view=spending',
    '/app/investments',
    '/app/investments?tab=splits&split_operation_id=4',
    '/app/investments?tab=suggestions',
    '/app/ledger/transactions?view=drafts',
    '/app/ledger/transactions?view=deleted',
    '/app/settings',
    '/app/settings/preferences',
    '/app/settings/data',
    '/app/reports/export',
    '/app/accounts/12',
    '/app/import/unknown',
    '/app/forecasting'
  ])('leaves %s alone', (path) => {
    expect(canonicalURL(url(path))).toBeNull();
  });
});
