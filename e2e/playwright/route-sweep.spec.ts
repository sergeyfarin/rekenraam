import { readdirSync } from 'node:fs';
import path from 'node:path';
import { expect, repoRoot, test as base, type Page } from './support/fixtures';
import { seedReferenceBook } from './support/reference-book';
import { accessibilityViolations, type AxeViolation } from './support/a11y';
import { ensureBrowserSession } from './support/session';
import { observePageRequests, waitForPageReady } from './support/page-readiness';
import { allLocales } from './support/locales';

// Worker scope preserves one seed per book while letting Playwright schedule
// each parallel sweep test independently instead of bundling beforeAll groups.
const test = base.extend<object, { routeBook: Map<string, number> }>({
  routeBook: [async ({ browser, backend }, use) => {
    const page = await browser.newPage({ baseURL: backend.url });
    const { ids } = await seedReferenceBook(page);
    await page.close();
    await use(ids);
  }, { scope: 'worker', timeout: 180_000 }]
});

// #278 — every route, both themes, phone width and 200% zoom.
//
// The R3a accessibility spec checks the journeys the roadmap names. This sweep
// visits every route page on a seeded reference book (so lists, trees and
// figures render, not only their empty states) and, for each, runs axe in the
// light and dark themes at desktop and 390 px, checks that the page never
// scrolls sideways at desktop, 390 px or a 640 px viewport (a 1280 px window
// at 200% zoom), and that the route's data failing renders a translated error state
// rather than an endless spinner or a blank page.
//
// A pass means no machine-checkable defect on the state each route reaches
// here; it does not certify every state of every screen.

// `data: false` routes have no data of their own to fail: appearance and
// language fetch nothing, and the overview reads only /health, which the shell
// gate needs too. The Settings overview shows a failed status inline in each
// value rather than as one alert (#329).
type Route = {
  name: string;
  /** The SvelteKit route id this entry covers, for the coverage check below. */
  route: string;
  path: (ids: Map<string, number>) => string;
  data: boolean;
  inlineErrors?: true;
};

const routes: Route[] = [
  { name: 'review', route: '/app/review', path: () => '/app/review', data: true },
  { name: 'home', route: '/app', path: () => '/app', data: false },
  { name: 'ledger overview', route: '/app/ledger', path: () => '/app/ledger', data: false },
  { name: 'accounts', route: '/app/ledger/accounts', path: () => '/app/ledger/accounts', data: true },
  { name: 'account register', route: '/app/ledger/accounts/[id]/register', path: (ids) => `/app/ledger/accounts/${ids.get('checking')}/register`, data: true },
  { name: 'categories', route: '/app/ledger/categories', path: () => '/app/ledger/categories', data: true },
  { name: 'category detail', route: '/app/ledger/categories/[id]', path: (ids) => `/app/ledger/categories/${ids.get('groceries')}`, data: true },
  { name: 'transactions', route: '/app/ledger/transactions', path: () => '/app/ledger/transactions', data: true },
  { name: 'reconcile', route: '/app/ledger/reconcile', path: () => '/app/ledger/reconcile', data: true },
  { name: 'import', route: '/app/ledger/import', path: () => '/app/ledger/import', data: true },
  { name: 'dividend repair', route: '/app/ledger/import/dividends', path: () => '/app/ledger/import/dividends', data: true },
  { name: 'investments', route: '/app/investments', path: () => '/app/investments', data: true },
  { name: 'instruments', route: '/app/investments/instruments', path: () => '/app/investments/instruments', data: true },
  { name: 'instrument', route: '/app/investments/instruments/[id]', path: (ids) => `/app/investments/instruments/${ids.get('REF')}`, data: true },
  { name: 'income coverage', route: '/app/investments/income', path: () => '/app/investments/income', data: true },
  { name: 'returns', route: '/app/investments/returns', path: () => '/app/investments/returns', data: true },
  { name: 'gains', route: '/app/investments/gains', path: () => '/app/investments/gains', data: true },
  { name: 'prices', route: '/app/investments/prices', path: () => '/app/investments/prices', data: true },
  { name: 'planning overview', route: '/app/planning', path: () => '/app/planning', data: false },
  { name: 'budgets', route: '/app/planning/budgets', path: () => '/app/planning/budgets', data: true },
  { name: 'recurring', route: '/app/planning/recurring', path: () => '/app/planning/recurring', data: true },
  { name: 'forecast', route: '/app/planning/forecast', path: () => '/app/planning/forecast', data: true },
  { name: 'reports overview', route: '/app/reports', path: () => '/app/reports', data: false },
  { name: 'spending', route: '/app/reports/spending', path: () => `/app/reports/spending?${reportRange}`, data: true },
  { name: 'cashflow', route: '/app/reports/cashflow', path: () => `/app/reports/cashflow?${reportRange}`, data: true },
  { name: 'net worth', route: '/app/reports/net-worth', path: () => `/app/reports/net-worth?${reportRange}`, data: true },
  { name: 'reports export', route: '/app/reports/export', path: () => '/app/reports/export', data: true },
  { name: 'settings general', route: '/app/settings', path: () => '/app/settings', data: true, inlineErrors: true },
  { name: 'preferences', route: '/app/settings/preferences', path: () => '/app/settings/preferences', data: false },
  { name: 'currencies', route: '/app/settings/currencies', path: () => '/app/settings/currencies', data: true },
  { name: 'security', route: '/app/settings/security', path: () => '/app/settings/security', data: true },
  { name: 'data and backups', route: '/app/settings/data', path: () => '/app/settings/data', data: true }
];

// Old paths only redirect (#436/#460); shell-navigation tests the browser
// transition and routes.test.ts checks every parameter and fragment.
const legacyRoutes = new Set([
  '/app/pricing', '/app/accounts', '/app/accounts/[id]/register',
  '/app/transactions', '/app/categories', '/app/categories/[id]',
  '/app/import', '/app/import/dividends', '/app/reconcile',
  '/app/budgets', '/app/recurring', '/app/forecast',
  '/app/settings/appearance', '/app/settings/language', '/app/settings/trash'
]);

const reportRange = 'start_date=2026-01-01&end_date=2026-03-31&bucket=month';

const viewports = {
  desktop: { width: 1280, height: 900 },
  phone: { width: 390, height: 844 },
  zoom200: { width: 640, height: 450 }
} as const;

// The shell's own gate: failing these leaves the install gate, not the route.
const shellEndpoints = /\/api\/v1\/(health|setup\/status|auth\/session)(\?|$)/;

/**
 * Waits until the route itself has rendered and stopped fetching. Each test
 * has its own browser context, so a lost session would otherwise land every
 * visit on the sign-in screen and audit that 72 times instead.
 */
async function settle(page: Page, path: string) {
  await expect(page).toHaveURL((url) => url.pathname === new URL(path, url).pathname);
  await expect(page.locator('main h1, main h2').first()).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole('heading', { name: 'Sign in to continue' })).toHaveCount(0);
  // #378: a generic not-found heading cannot certify the seeded instrument.
  if (path.startsWith('/app/investments/instruments/')) {
    await expect(page.getByRole('heading', { name: /Reference Corp/ })).toBeVisible();
    await expect(page.locator('main').getByRole('searchbox')).toBeVisible();
  }
  await waitForPageReady(page);
}

async function setTheme(page: Page, theme: 'light' | 'dark') {
  await page.evaluate((name) => window.localStorage.setItem('rekenraam-theme', name), theme);
}

/** How far the page scrolls sideways; 0 means it fits its viewport. */
async function horizontalOverflow(page: Page): Promise<number> {
  return page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
}

function describeViolations(violations: AxeViolation[]): string {
  return violations.map((v) => `[${v.impact ?? 'unknown'}] ${v.id}: ${v.targets.join(' | ')}`).join('; ');
}

/** Every `+page.svelte` under frontend/src/routes/app, as a route id. */
function pageRoutes(dir = path.join(repoRoot, 'frontend/src/routes/app')): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return pageRoutes(full);
    if (entry.name !== '+page.svelte') return [];
    return ['/' + path.relative(path.join(repoRoot, 'frontend/src/routes'), dir).split(path.sep).join('/')];
  });
}

// A page added without a sweep entry would never be audited (#436).
base('the sweep covers every app page', () => {
  const swept = new Set(routes.map((route) => route.route));
  const missing = pageRoutes().filter((route) => !swept.has(route) && !legacyRoutes.has(route));
  expect(missing, `pages missing from the route sweep: ${missing.join(', ')}`).toEqual([]);
});

test.describe('route sweep (#278)', () => {
  // Each check takes minutes; spread them over workers so the push smoke
  // suite does not wait on one file. Each worker seeds its own book.
  test.describe.configure({ mode: 'parallel' });
  test.beforeEach(async ({ page }) => { observePageRequests(page); });

  for (const theme of ['light', 'dark'] as const) {
    test(`every route passes axe and fits the phone in the ${theme} theme`, async ({ page, routeBook: ids }) => {
      test.setTimeout(600_000);
      await ensureBrowserSession(page);
      await setTheme(page, theme);
      const failures: string[] = [];

      for (const route of routes) {
        for (const [size, viewport] of Object.entries(viewports)) {
          await test.step(`route: ${route.name} / ${size}`, async () => {
            await page.setViewportSize(viewport);
            await page.goto(route.path(ids));
            await settle(page, route.path(ids));
            await expect(page.locator('html')).toHaveAttribute('data-theme', theme);

            const violations = await accessibilityViolations(page);
            if (violations.length > 0) failures.push(`${route.name} @ ${size}: ${describeViolations(violations)}`);
            // Desktop too: the categories toolbar scrolled 118px sideways at
            // 1280px once its row layout applied (#340).
            const overflow = await horizontalOverflow(page);
            if (overflow > 0) failures.push(`${route.name} @ ${size}: page scrolls ${overflow}px sideways`);
          });
        }
      }

      expect(failures, failures.join('\n')).toEqual([]);
    });
  }

  test('every route masks read-only financial figures (#456)', async ({ page, routeBook: ids }) => {
    test.setTimeout(240_000);
    await ensureBrowserSession(page);
    await page.evaluate(() => localStorage.setItem('rekenraam-hide-amounts', 'true'));
    let masked = 0;
    for (const route of routes) {
      await test.step(route.name, async () => {
        await page.goto(route.path(ids));
        await settle(page, route.path(ids));
        await expect(page.getByRole('button', { name: 'Hide amounts', exact: true })).toHaveAttribute('aria-pressed', 'true');
        const evidence = await page.locator('main').evaluate((main) => {
          const walker = document.createTreeWalker(main, NodeFilter.SHOW_TEXT);
          const leaks: string[] = [];
          let masks = 0;
          while (walker.nextNode()) {
            const node = walker.currentNode;
            if (node.parentElement?.closest('input, textarea, select, script, style')) continue;
            const text = node.textContent?.trim() ?? '';
            if (text.includes('••••')) masks++;
            // Read-only money cells and SVG labels must not retain decimal digits.
            if (node.parentElement?.closest('.tabular-nums, .font-mono, svg') && /\d[,.]\d{2}/.test(text) && !text.includes('%')) leaks.push(text);
          }
          return { leaks, masks };
        });
        expect(evidence.leaks, route.name).toEqual([]);
        masked += evidence.masks;
      });
    }
    expect(masked).toBeGreaterThan(20); // Populated financial reads, never an empty-page pass.
  });

  // Longer German, French and Russian copy is what breaks a phone layout, and
  // the declared page language is what a screen reader reads it in.
  test('every route fits the phone and declares its language in every locale', async ({ page, routeBook: ids }) => {
    test.setTimeout(600_000);
    await ensureBrowserSession(page);
    await setTheme(page, 'light');
    await page.setViewportSize(viewports.phone);
    const failures: string[] = [];

    for (const locale of allLocales.filter(locale => locale !== 'en')) {
      await page.evaluate((name) => window.localStorage.setItem('PARAGLIDE_LOCALE', name), locale);
      for (const route of routes) {
        await test.step(`route: ${route.name} / ${locale}`, async () => {
          await page.goto(route.path(ids));
          await settle(page, route.path(ids));
          const lang = await page.locator('html').getAttribute('lang');
          if (lang !== locale) failures.push(`${route.name} in ${locale}: html lang is ${lang}`);
          const overflow = await horizontalOverflow(page);
          if (overflow > 0) failures.push(`${route.name} in ${locale}: page scrolls ${overflow}px sideways`);
          const violations = await accessibilityViolations(page);
          if (violations.length > 0) failures.push(`${route.name} in ${locale}: ${describeViolations(violations)}`);
          // Longer copy also widens desktop toolbars.
          await page.setViewportSize(viewports.desktop);
          const desktopOverflow = await horizontalOverflow(page);
          if (desktopOverflow > 0) failures.push(`${route.name} in ${locale} @ desktop: page scrolls ${desktopOverflow}px sideways`);
          await page.setViewportSize(viewports.phone);
        });
      }
    }
    await page.evaluate(() => window.localStorage.setItem('PARAGLIDE_LOCALE', 'en'));

    expect(failures, failures.join('\n')).toEqual([]);
  });

  test('every data route shows an accessible error state when its data fails', async ({ page, routeBook: ids }) => {
    test.setTimeout(600_000);
    await ensureBrowserSession(page);
    await setTheme(page, 'light');
    await page.setViewportSize(viewports.phone);
    await page.route('**/api/v1/**', async (route) => {
      const request = route.request();
      if (request.method() !== 'GET' || shellEndpoints.test(request.url())) return route.continue();
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: { code: 'INTERNAL_ERROR', message: 'route sweep failure' } })
      });
    });
    const failures: string[] = [];

    for (const route of routes.filter((r) => r.data && !r.inlineErrors)) {
      await test.step(`route: ${route.name} / error`, async () => {
        await page.goto(route.path(ids));
        await expect(page).toHaveURL((url) => url.pathname === new URL(route.path(ids), url).pathname);
        const alert = page.getByRole('alert').first();
        // The alert normally appears in ~1.4s. A 4-vCPU CI runner can starve
        // this worker while the sub-view sweep runs axe (one run saw the
        // failure snapshot itself take 10.1s), so keep the wait bounded but
        // long enough that a stalled renderer is not reported as a stuck state.
        // Twenty seconds also keeps a systematic failure across all 21 routes
        // inside the test's 600s budget, so the collected messages survive.
        const shown = await alert.waitFor({ state: 'visible', timeout: 20_000 }).then(() => true, () => false);
        if (!shown) {
          const shownText = (await page.locator('main').innerText()).replace(/\s+/g, ' ').split('AUTHENTICATED WORKSPACE').pop()!.slice(0, 700);
          failures.push(`${route.name}: no error alert when its data fails; shows: ${shownText}`);
          return;
        }
        if (await page.getByText('route sweep failure').count()) {
          failures.push(`${route.name}: shows the raw server message instead of translated copy`);
        }
        const violations = await accessibilityViolations(page);
        if (violations.length > 0) failures.push(`${route.name} error state: ${describeViolations(violations)}`);
        const overflow = await horizontalOverflow(page);
        if (overflow > 0) failures.push(`${route.name} error state: page scrolls ${overflow}px sideways`);
      });
    }

    expect(failures, failures.join('\n')).toEqual([]);
  });
});
