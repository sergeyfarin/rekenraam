import { test, expect } from './support/owned-book';
import { apiJSON, csrfTokenFor } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';

// The unnamed starter cash account (code cash:USD) reads "Cash USD" on every
// screen that names it, as Ledger does (#597 sweep, bug class 13). Each screen
// below built its own `name || code` label, or a bare name that drew a blank
// option, before the sweep. An owned book: no other spec can rename the
// starter account under it. The last step reconciles the starter account and
// checks a reconciliation confirmation names it too.

test('every screen names the unnamed starter cash account Cash USD', async ({ page }) => {
  const { currencyID } = await readyForLedger(page);
  const csrfToken = await csrfTokenFor(page);
  const checking = await createCashAccount(page, csrfToken, 'Label checking', currencyID);
  const { accounts } = await apiJSON<{ accounts: Array<{ id: number; code?: string; name?: string }> }>(page, 'GET', '/api/v1/accounts');
  const starter = accounts.find(item => item.code === 'cash:USD');
  if (!starter) throw new Error('expected the starter USD cash account');
  expect(starter.name ?? '').toBe('');
  const { owner_today: today } = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
  const move = await apiJSON<{ id: number; journal_entries: Array<{ postings: Array<{ id: number; account_id: number }> }> }>(page, 'POST', '/api/v1/transactions', csrfToken, {
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

  // A reconciliation confirmation names the checkpoint's account. The API sent
  // a composed account_label, cash:USD, which every confirmation showed as is.
  const starterPosting = move.journal_entries[0].postings.find(posting => posting.account_id === starter.id);
  if (!starterPosting) throw new Error('expected the starter posting');
  await apiJSON(page, 'POST', `/api/v1/accounts/${starter.id}/postings/reconciliation-status`, csrfToken, {
    posting_version_ids: [starterPosting.id], status: 'cleared', cleared_on: today
  });
  const session = await apiJSON<{ id: number }>(page, 'POST', `/api/v1/accounts/${starter.id}/reconciliations/start`, csrfToken, {
    commodity_id: currencyID, source_kind: 'statement', statement_date: today, statement_balance_value: '2500', statement_balance_scale: 2
  });
  await apiJSON(page, 'POST', `/api/v1/reconciliations/${session.id}/finish`, csrfToken, { change_reason: 'matched statement' });
  await page.goto(`/app/ledger/accounts/${starter.id}/register`);
  await page.getByRole('row').filter({ hasText: 'Starter label move' }).click();
  await page.getByRole('button', { name: 'Void…' }).click();
  await page.getByLabel('Reason').fill('duplicate');
  await page.getByRole('dialog', { name: 'Void transaction' }).getByRole('button', { name: 'Void', exact: true }).click();
  const warning = page.getByRole('alertdialog', { name: 'This change affects a reconciled period' });
  await expect(warning.getByRole('listitem')).toHaveCount(1);
  await expect(warning.getByRole('listitem')).toContainText('Cash USD (USD) reconciled through');
  await warning.getByRole('button', { name: 'Go back' }).click();
  await expect(warning).toHaveCount(0);
});
