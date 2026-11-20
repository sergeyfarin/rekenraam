import { test, expect } from './support/owned-book';
import { apiJSON, csrfTokenFor } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';

// The unnamed starter cash account (code cash:USD) reads "Cash USD" on every
// screen that names it, as Ledger does (#597 sweep, bug class 13). Each screen
// below built its own `name || code` label, or a bare name that drew a blank
// option, before the sweep. An owned book: no other spec can rename the
// starter account under it.

test('every screen names the unnamed starter cash account Cash USD', async ({ page }) => {
  const { currencyID } = await readyForLedger(page);
  const csrfToken = await csrfTokenFor(page);
  const checking = await createCashAccount(page, csrfToken, 'Label checking', currencyID);
  const { accounts } = await apiJSON<{ accounts: Array<{ id: number; code?: string; name?: string }> }>(page, 'GET', '/api/v1/accounts');
  const starter = accounts.find(item => item.code === 'cash:USD');
  if (!starter) throw new Error('expected the starter USD cash account');
  expect(starter.name ?? '').toBe('');
  const { owner_today: today } = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    status: 'posted', transaction_date: today, description: 'Starter label move', journal_entries: [{ entry_date: today, postings: [
      { account_id: checking.id, commodity_id: currencyID, quantity_value: '-2500', quantity_scale: 2 },
      { account_id: starter.id, commodity_id: currencyID, quantity_value: '2500', quantity_scale: 2 }
    ] }]
  });

  // The register Home's Accounts widget links to: its heading.
  await page.goto(`/app/ledger/accounts/${starter.id}/register`);
  await expect(page.getByRole('heading', { level: 1, name: 'Cash USD', exact: true })).toBeVisible();

  // The transactions list names it through the posting read, and its
  // account filter offers it.
  await page.goto('/app/ledger/transactions');
  await expect(page.getByRole('row').filter({ hasText: 'Starter label move' })).toContainText('Cash USD');
  await expect(page.locator('option', { hasText: /^Cash USD$/ })).toHaveCount(1);

  // Reconciliation's account choice.
  await page.goto('/app/ledger/reconcile');
  await expect(page.locator('option', { hasText: /^Cash USD$/ })).toHaveCount(1);

  // Budgets' accounts panel.
  await page.goto('/app/planning/budgets');
  await expect(page.getByRole('main').getByText('Cash USD', { exact: true })).toBeVisible();

  // Forecast's account options.
  await page.goto('/app/planning/forecast?horizon_days=30');
  await expect(page.getByRole('checkbox', { name: 'Cash USD', exact: true })).toHaveCount(1);

  // Allocation's pool editor lists cash accounts.
  await page.goto(`/app/investments/allocation?from=${today}&to=${today}&reporting_currency_id=${currencyID}`);
  await page.getByRole('button', { name: 'Edit pool and targets', exact: true }).click();
  const pool = page.getByRole('dialog', { name: 'Edit pool and targets' });
  await expect(pool.getByRole('checkbox', { name: 'Cash USD', exact: true })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(pool).toBeHidden();

  // Search.
  await page.locator('.shell-bar').getByRole('button', { name: /^Search Ctrl K$/ }).click();
  await page.getByRole('combobox', { name: 'Search', exact: true }).fill('Cash USD');
  await expect(page.getByRole('option', { name: /Cash USD/ }).first()).toBeVisible();
});
