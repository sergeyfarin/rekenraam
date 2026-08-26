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

test('[acceptance] every report view is accessible', async ({ page }) => {
  await readyForLedger(page);
  const today = todayISO();

  for (const view of ['net-worth', 'spending', 'cashflow']) {
    await page.goto(`/app/reports?view=${view}&start_date=${today}&end_date=${today}&bucket=month`);
    await expect(page.getByRole('heading').first()).toBeVisible();
    await expectNoAccessibilityViolations(page, `reports: ${view}`);
  }
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
