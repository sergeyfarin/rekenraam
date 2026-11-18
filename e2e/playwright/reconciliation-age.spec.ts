import { test, expect } from './support/owned-book';
import { apiJSON, csrfTokenFor } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';
import { expectNoAccessibilityViolations } from './support/a11y';

// #482: one reconciliation out-of-date window, set in Settings › General,
// read by the account list and Home's Reconciliation widget alike. An owned
// book, because the window is a book-wide setting.

function shiftDays(isoDate: string, days: number): string {
  const date = new Date(`${isoDate}T00:00:00Z`);
  date.setUTCDate(date.getUTCDate() + days);
  return date.toISOString().slice(0, 10);
}

test('the reconciliation window drives the account list and Home together', async ({ page }) => {
  const { currencyID } = await readyForLedger(page);
  const { owner_today: today } = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
  const movedOn = shiftDays(today, -5);
  const statementDate = shiftDays(today, -3);
  let csrfToken = await csrfTokenFor(page);
  const reconciled = await createCashAccount(page, csrfToken, 'Window reconciled', currencyID, movedOn);
  const never = await createCashAccount(page, csrfToken, 'Window never', currencyID, movedOn);
  await createCashAccount(page, csrfToken, 'Window empty', currencyID, movedOn);
  const income = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name: 'Window salary', category_type: 'income' });
  for (const account of [reconciled, never]) {
    await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
      transaction_date: movedOn, status: 'posted',
      journal_entries: [{ entry_date: movedOn, postings: [
        { account_id: account.id, commodity_id: currencyID, quantity_value: '10000', quantity_scale: 2 },
        { account_id: income.id, commodity_id: currencyID, quantity_value: '-10000', quantity_scale: 2 }
      ] }]
    });
  }
  const session = await apiJSON<{ id: number; candidates: Array<{ posting_id: number }> }>(page, 'POST', `/api/v1/accounts/${reconciled.id}/reconciliations/start`, csrfToken, {
    commodity_id: currencyID, source_kind: 'statement', statement_date: statementDate, statement_balance_value: '10000', statement_balance_scale: 2
  });
  await apiJSON(page, 'PATCH', `/api/v1/reconciliations/${session.id}`, csrfToken, { posting_version_ids: session.candidates.map(candidate => candidate.posting_id) });
  await apiJSON(page, 'POST', `/api/v1/reconciliations/${session.id}/finish`, csrfToken, { change_reason: 'statement verified' });
  const shown = new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeZone: 'UTC' }).format(new Date(`${statementDate}T00:00:00Z`));

  const row = (name: string) => page.locator('article').filter({ hasText: name });
  await page.goto('/app/ledger');
  await expect(row('Window reconciled').getByTestId('account-reconciliation-age')).toHaveText(`Reconciled to ${shown}`);
  await expect(row('Window never').getByTestId('account-reconciliation-age')).toHaveText('Never reconciled');
  await expect(row('Window empty').getByTestId('account-reconciliation-age')).toHaveText('Nothing to reconcile');

  await page.goto('/app');
  const widget = page.getByTestId('explore-widget-reconciliation');
  await expect(widget).toHaveAttribute('data-state', 'ready');
  await expect(widget).toContainText('1 of 2 accounts reconciled');
  await expect(widget).toContainText('Within the last 60 days');

  await page.goto('/app/settings');
  const days = page.getByLabel('Out of date after (days)');
  await expect(days).toHaveValue('60');
  await days.fill('0');
  await page.getByTestId('reconciliation-settings').getByRole('button', { name: 'Save' }).click();
  await expect(page.getByText('Enter a whole number of days from 1 to 3650.')).toBeVisible();
  await expect(days).toHaveAttribute('aria-invalid', 'true');
  await expectNoAccessibilityViolations(page, 'reconciliation settings invalid');
  await days.fill('2');
  await page.getByTestId('reconciliation-settings').getByRole('button', { name: 'Save' }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Reconciliation setting saved.' })).toBeVisible();
  csrfToken = await csrfTokenFor(page);
  expect((await apiJSON<{ out_of_date_after_days: number }>(page, 'GET', '/api/v1/reconciliation/policy')).out_of_date_after_days).toBe(2);

  await page.goto('/app/ledger');
  await expect(row('Window reconciled').getByTestId('account-reconciliation-age')).toHaveText(`Out of date: reconciled to ${shown}`);
  await expect(row('Window empty').getByTestId('account-reconciliation-age')).toHaveText('Nothing to reconcile');
  await expectNoAccessibilityViolations(page, 'account list reconciliation states');

  await page.goto('/app');
  await expect(widget).toContainText('0 of 2 accounts reconciled');
  await expect(widget).toContainText('Within the last 2 days');
  await widget.getByRole('link', { name: 'Window never' }).click();
  await expect(page).toHaveURL(new RegExp(`/app/ledger/reconcile\\?account=${never.id}$`));
});
