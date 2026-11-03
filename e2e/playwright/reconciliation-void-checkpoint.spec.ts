import { expect, test } from './support/fixtures';
import { apiJSON } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';
import { daysFromTodayISO, todayISO } from './support/dates';

/**
 * #248: a wrongly finished statement is recovered from the reconcile screen.
 * Cancelling the void writes nothing; confirming it with a reason moves the
 * checkpoint to history, and the next reconciliation offers its posting
 * preselected for re-verification. Phone-sized screen.
 */

test.use({ viewport: { width: 390, height: 844 } });

type Checkpoint = { id: number; status: string; void_reason?: string };

test('a finished statement can be voided from the reconcile screen and reconciled again', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const day = todayISO();
  const movedOn = daysFromTodayISO(-2);
  const cash = await createCashAccount(page, csrfToken, `Void cash ${suffix}`, currencyID, movedOn);
  const income = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: `Void salary ${suffix}`, category_type: 'income'
  });
  const salary = await apiJSON<{ journal_entries: Array<{ postings: Array<{ id: number; account_id: number }> }> }>(
    page, 'POST', '/api/v1/transactions', csrfToken, {
      transaction_date: movedOn, status: 'posted', description: `Salary ${suffix}`,
      journal_entries: [{ entry_date: movedOn, postings: [
        { account_id: cash.id, commodity_id: currencyID, quantity_value: '20000', quantity_scale: 2 },
        { account_id: income.id, commodity_id: currencyID, quantity_value: '-20000', quantity_scale: 2 }
      ] }]
    });
  const salaryPosting = salary.journal_entries[0].postings.find((p) => p.account_id === cash.id)!;
  await apiJSON(page, 'POST', `/api/v1/accounts/${cash.id}/postings/reconciliation-status`, csrfToken, {
    posting_version_ids: [salaryPosting.id], status: 'cleared', cleared_on: day
  });
  const first = await apiJSON<{ id: number }>(page, 'POST', `/api/v1/accounts/${cash.id}/reconciliations/start`, csrfToken, {
    commodity_id: currencyID, source_kind: 'statement', statement_date: day,
    statement_balance_value: '20000', statement_balance_scale: 2
  });
  await apiJSON(page, 'POST', `/api/v1/reconciliations/${first.id}/finish`, csrfToken, { change_reason: 'wrong statement' });
  const checkpoints = async () =>
    (await apiJSON<{ checkpoints: Checkpoint[] }>(page, 'GET', `/api/v1/accounts/${cash.id}/reconciliation-checkpoints`)).checkpoints;

  await page.goto('/app/ledger/reconcile');
  await page.getByLabel('Account').selectOption({ label: cash.name });
  const voidButton = page.getByRole('button', { name: `Void the statement of ${day}` });
  await expect(voidButton).toBeVisible();

  // Cancelling writes nothing.
  await voidButton.click();
  const dialog = page.getByRole('alertdialog', { name: 'Void this statement?' });
  await expect(dialog).toContainText(day);
  await expect(dialog).toContainText('200.00');
  await expect(dialog).toContainText('offered again');
  await dialog.getByRole('button', { name: 'Cancel' }).click();
  await expect(dialog).toBeHidden();
  await expect(voidButton).toBeFocused();
  expect((await checkpoints()).map((c) => c.status)).toEqual(['active']);

  // A reason is required.
  await voidButton.click();
  await dialog.getByRole('button', { name: 'Void statement' }).click();
  await expect(dialog.getByText('Enter a reason to void this statement.')).toBeVisible();
  expect((await checkpoints()).map((c) => c.status)).toEqual(['active']);

  await dialog.getByLabel('Reason for voiding').fill(`Wrong balance ${suffix}`);
  await dialog.getByRole('button', { name: 'Void statement' }).click();
  await expect(dialog).toBeHidden();
  await expect(page.getByRole('status').filter({ hasText: `The statement of ${day} was voided.` })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'No longer current' })).toBeVisible();
  await expect(page.getByText(`Wrong balance ${suffix}`)).toBeVisible();
  await expect(page.getByText('This account has not been reconciled yet.')).toBeVisible();
  expect(await checkpoints()).toEqual([
    expect.objectContaining({ status: 'voided', void_reason: `Wrong balance ${suffix}` })
  ]);

  // The next reconciliation offers the posting again, preselected.
  await page.getByLabel('Statement closing date').fill(day);
  await page.getByLabel('Statement ending balance').fill('200.00');
  await page.getByRole('button', { name: 'Start reconciling' }).click();
  const reverify = page.getByRole('row').filter({ hasText: `Salary ${suffix}` });
  await expect(reverify.getByText('Re-verify')).toBeVisible();
  await expect(reverify.getByRole('checkbox')).toBeChecked();
  await expect(page.getByText('Balanced')).toBeVisible();
  // No void action while a session is open.
  await expect(page.getByRole('button', { name: /^Void the statement/ })).toHaveCount(0);
});
