import { expect, test } from '@playwright/test';
import { apiJSON } from './support/api';
import { todayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';
import { ensureBrowserSession } from './support/session';
import {
  expectFocusIsNotOrphaned,
  expectKeyboardReachable,
  expectNoAccessibilityViolations
} from './support/a11y';

// R3a — core-workflow accessibility regression coverage.
//
// Smoke checks over the journeys the roadmap names: setup/auth, transaction
// entry, reconciliation, reports, and import. A failure means something
// regressed; a pass does not mean the app is accessible. Mobile journeys stay
// in the broader suite, as the roadmap says.

test('[acceptance] the sign-in screen is usable without a mouse', async ({ page }) => {
  await page.goto('/');
  // Either the first-run or the sign-in form, depending on run order: both are
  // the first thing a new user meets, and both must pass.
  await expect(page.getByRole('heading').first()).toBeVisible();

  await expectNoAccessibilityViolations(page, 'entry screen');
  await expectKeyboardReachable(page, 'entry screen', ['input:', 'button:']);
});

test('[acceptance] the overview and its navigation are accessible', async ({ page }) => {
  await ensureBrowserSession(page);
  await page.goto('/app');
  await expect(page.getByRole('heading', { name: 'Overview' }).first()).toBeVisible();

  await expectNoAccessibilityViolations(page, 'overview');

  // Navigation must be reachable by keyboard, or the app has one screen.
  await expectKeyboardReachable(page, 'overview navigation', ['a:']);
});

test('[acceptance] transaction entry is accessible and operable by keyboard', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  await createCashAccount(page, csrfToken, `A11y checking ${Date.now()}`, currencyID);

  await page.goto('/app/transactions');
  await expect(page.getByRole('heading').first()).toBeVisible();

  // Entry is a panel on the transactions screen rather than its own route, so
  // the check is of the screen with the form open — which is where a keyboard
  // user actually meets it.
  const newTransaction = page.getByRole('button', { name: /new transaction/i }).first();
  if (await newTransaction.isVisible().catch(() => false)) {
    await newTransaction.click();
  }

  await expectNoAccessibilityViolations(page, 'transaction entry');
  await expectKeyboardReachable(page, 'transaction entry', ['input:', 'button:']);
});

test('[acceptance] the transactions list is accessible', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  // Opened early enough for the past-dated posting below: this book is shared
  // within a run, and today's totals belong to the reports specs.
  const account = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `A11y list ${Date.now()}`,
    account_class: 'asset',
    account_kind: 'checking',
    default_commodity_id: currencyID,
    allows_postings: true,
    opened_on: '2021-01-01'
  });
  const categories = await apiJSON<{
    categories: Array<{ id: number; code?: string; allows_postings: boolean }>;
  }>(page, 'GET', '/api/v1/categories');
  const groceries = categories.categories.find(
    (item) => item.code === 'expense_food_groceries' && item.allows_postings
  );
  if (!groceries) throw new Error('seeded groceries category not found');

  // Booked in the past: the reports specs assert on today's totals in this
  // shared database.
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_date: '2021-05-06',
    journal_entries: [
      {
        entry_date: '2021-05-06',
        postings: [
          { account_id: account.id, quantity_value: '-1500', quantity_scale: 2, commodity_id: currencyID },
          { account_id: groceries.id, quantity_value: '1500', quantity_scale: 2, commodity_id: currencyID }
        ]
      }
    ]
  });

  await page.goto('/app/transactions');
  await expect(page.getByRole('table').first()).toBeVisible();

  await expectNoAccessibilityViolations(page, 'transactions list');
});

test('[acceptance] the reconcile screen is accessible', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  await createCashAccount(page, csrfToken, `A11y reconcile ${Date.now()}`, currencyID);

  await page.goto('/app/reconcile');
  await expect(page.getByRole('heading').first()).toBeVisible();

  await expectNoAccessibilityViolations(page, 'reconcile');
});

test('[acceptance] the budget screen is accessible and usable on mobile', async ({ page }) => {
  await readyForLedger(page);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/app/budgets');
  await expect(page.getByRole('heading', { name: 'Budgets' }).first()).toBeVisible();
  await expectNoAccessibilityViolations(page, 'budgets');
  await expectKeyboardReachable(page, 'budgets', ['input:', 'select:', 'button:']);
});

test('[acceptance] every report view is accessible', async ({ page }) => {
  await readyForLedger(page);
  const today = todayISO();

  for (const view of ['net-worth', 'spending', 'cashflow']) {
    await page.goto(`/app/reports?view=${view}&start_date=${today}&end_date=${today}&bucket=month`);
    await expect(page.getByRole('heading').first()).toBeVisible();
    await expectNoAccessibilityViolations(page, `reports: ${view}`);
  }
});

// A control may render below the contrast floor only while it is inactive,
// which is the exemption the rules grant. Fading one *while enabling it* takes
// the exemption away a frame or two before the fade finishes, and an axe run
// that lands in that gap fails — which is how this arrived: the report views
// case above failed intermittently under load on the spending view, where the
// reporting-currency select is the only select on the page, caught already
// operable at 60% opacity (4.37:1 against the panel, against a 4.5:1 floor).
//
// This select is the one control in the app that both animates its disabled
// fade and clears that state on its own, with no click behind it — which is
// what lets a page-load accessibility check land in the gap. Sampling every
// frame makes the check deterministic; waiting for the query to settle would
// only move the race somewhere a test cannot see it.
test('the reporting-currency select is never operable before its fade finishes', async ({ page }) => {
  await readyForLedger(page);
  const today = todayISO();

  // Hold the currencies response long enough that the select is observably
  // disabled first, so the sampler is already running when the state flips.
  await page.route('**/api/v1/currencies*', async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 1500));
    await route.continue();
  });

  await page.goto(`/app/reports?view=spending&start_date=${today}&end_date=${today}&bucket=month`);
  await expect(page.getByRole('heading').first()).toBeVisible();

  const faded = await page.evaluate(async () => {
    const select = document.querySelector('select') as HTMLSelectElement | null;
    if (!select) return 'no select on the spending report';
    if (!select.disabled) return 'the select was never disabled, so this proves nothing';
    const start = performance.now();
    return await new Promise<string | null>((resolve) => {
      const frame = () => {
        const opacity = Number(getComputedStyle(select).opacity);
        const elapsed = performance.now() - start;
        if (!select.disabled && opacity < 1) return resolve(`enabled at opacity ${opacity}`);
        // Half a second past the flip is well clear of a 150ms transition.
        if (!select.disabled && elapsed > 500) return resolve(null);
        if (elapsed > 10_000) return resolve('the select never became enabled');
        requestAnimationFrame(frame);
      };
      frame();
    });
  });

  expect(
    faded,
    `the reporting-currency select was operable before its fade finished: ${faded}`
  ).toBeNull();
});

test('[acceptance] the import screen is accessible', async ({ page }) => {
  await ensureBrowserSession(page);

  await page.goto('/app/import');
  await expect(page.getByRole('heading').first()).toBeVisible();

  await expectNoAccessibilityViolations(page, 'import');
});

test('[acceptance] the data screen is accessible and keeps focus on navigation', async ({ page }) => {
  await ensureBrowserSession(page);

  await page.goto('/app/settings');
  await expect(page.getByRole('heading').first()).toBeVisible();
  await expectNoAccessibilityViolations(page, 'settings index');

  await page.goto('/app/settings/data');
  await expect(page.getByTestId('data-settings')).toBeVisible();
  await expectNoAccessibilityViolations(page, 'settings: data');
  await expectFocusIsNotOrphaned(page, 'settings: data after navigation');
});
