import { expect, test, type Page } from './support/fixtures';
import { seedReferenceBook } from './support/reference-book';
import { accessibilityViolations, type AxeViolation } from './support/a11y';
import { ensureBrowserSession } from './support/session';

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
type Route = { name: string; path: (ids: Map<string, number>) => string; data: boolean; inlineErrors?: true };

const routes: Route[] = [
  { name: 'overview', path: () => '/app', data: false },
  { name: 'accounts', path: () => '/app/accounts', data: true },
  { name: 'account register', path: (ids) => `/app/accounts/${ids.get('checking')}/register`, data: true },
  { name: 'categories', path: () => '/app/categories', data: true },
  { name: 'category detail', path: (ids) => `/app/categories/${ids.get('groceries')}`, data: true },
  { name: 'transactions', path: () => '/app/transactions', data: true },
  { name: 'reconcile', path: () => '/app/reconcile', data: true },
  { name: 'import', path: () => '/app/import', data: true },
  { name: 'dividend repair', path: () => '/app/import/dividends', data: true },
  { name: 'investments', path: () => '/app/investments', data: true },
  { name: 'instrument', path: (ids) => `/app/investments/instruments/${ids.get('REF')}`, data: true },
  { name: 'income coverage', path: () => '/app/investments/income', data: true },
  { name: 'pricing', path: () => '/app/pricing', data: true },
  { name: 'reports', path: () => '/app/reports?view=net-worth&start_date=2026-01-01&end_date=2026-03-31&bucket=month', data: true },
  { name: 'budgets', path: () => '/app/budgets', data: true },
  { name: 'recurring', path: () => '/app/recurring', data: true },
  { name: 'forecast', path: () => '/app/forecast', data: true },
  { name: 'settings', path: () => '/app/settings', data: true, inlineErrors: true },
  { name: 'appearance', path: () => '/app/settings/appearance', data: false },
  { name: 'currencies', path: () => '/app/settings/currencies', data: true },
  { name: 'language', path: () => '/app/settings/language', data: false },
  { name: 'security', path: () => '/app/settings/security', data: true },
  { name: 'data', path: () => '/app/settings/data', data: true },
  { name: 'trash', path: () => '/app/settings/trash', data: true }
];

const viewports = {
  desktop: { width: 1280, height: 900 },
  phone: { width: 390, height: 844 },
  zoom200: { width: 640, height: 450 }
} as const;

// The shell's own gate: failing these leaves the install gate, not the route.
const shellEndpoints = /\/api\/v1\/(health|setup\/status|auth\/session|recurring\/summary)(\?|$)/;

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
  await page.waitForLoadState('networkidle');
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

test.describe('route sweep (#278)', () => {
  // Each check takes minutes; spread them over workers so the push smoke
  // suite does not wait on one file. Each worker seeds its own book.
  test.describe.configure({ mode: 'parallel' });
  let ids = new Map<string, number>();

  // Seeded once per worker; each check is its own test so one failing check
  // does not hide the others.
  test.beforeAll(async ({ browser, backend }) => {
    test.setTimeout(180_000);
    const page = await browser.newPage({ baseURL: backend.url });
    ({ ids } = await seedReferenceBook(page));
    await page.close();
  });

  for (const theme of ['light', 'dark'] as const) {
    test(`every route passes axe and fits the phone in the ${theme} theme`, async ({ page }) => {
      test.setTimeout(600_000);
      await ensureBrowserSession(page);
      await setTheme(page, theme);
      const failures: string[] = [];

      for (const route of routes) {
        for (const [size, viewport] of Object.entries(viewports)) {
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
        }
      }

      expect(failures, failures.join('\n')).toEqual([]);
    });
  }

  // Longer German, French and Russian copy is what breaks a phone layout, and
  // the declared page language is what a screen reader reads it in.
  test('every route fits the phone and declares its language in every locale', async ({ page }) => {
    test.setTimeout(600_000);
    await ensureBrowserSession(page);
    await setTheme(page, 'light');
    await page.setViewportSize(viewports.phone);
    const failures: string[] = [];

    for (const locale of ['es', 'fr', 'nl', 'de', 'ru']) {
      await page.evaluate((name) => window.localStorage.setItem('PARAGLIDE_LOCALE', name), locale);
      for (const route of routes) {
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
      }
    }
    await page.evaluate(() => window.localStorage.setItem('PARAGLIDE_LOCALE', 'en'));

    expect(failures, failures.join('\n')).toEqual([]);
  });

  test('every data route shows an accessible error state when its data fails', async ({ page }) => {
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
      await page.goto(route.path(ids));
      await expect(page).toHaveURL((url) => url.pathname === new URL(route.path(ids), url).pathname);
      const alert = page.getByRole('alert').first();
      const shown = await alert.waitFor({ state: 'visible', timeout: 10_000 }).then(() => true, () => false);
      if (!shown) {
        const shownText = (await page.locator('main').innerText()).replace(/\s+/g, ' ').split('AUTHENTICATED WORKSPACE').pop()!.slice(0, 700);
        failures.push(`${route.name}: no error alert when its data fails; shows: ${shownText}`);
        continue;
      }
      if (await page.getByText('route sweep failure').count()) {
        failures.push(`${route.name}: shows the raw server message instead of translated copy`);
      }
      const violations = await accessibilityViolations(page);
      if (violations.length > 0) failures.push(`${route.name} error state: ${describeViolations(violations)}`);
      const overflow = await horizontalOverflow(page);
      if (overflow > 0) failures.push(`${route.name} error state: page scrolls ${overflow}px sideways`);
    }

    expect(failures, failures.join('\n')).toEqual([]);
  });
});
