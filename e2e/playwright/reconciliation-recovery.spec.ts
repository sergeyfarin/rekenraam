import { expect, test } from './support/fixtures';
import { apiJSON } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';
import { daysFromTodayISO, todayISO } from './support/dates';

/**
 * #240: after an override invalidates a checkpoint, a new reconciliation of
 * the same statement period must reach the corrected ledger balance. The
 * once-reconciled posting is offered again, preselected and marked for
 * re-verification, and the invalidated checkpoint stays visible as history.
 * Phone-sized screen.
 */

test.use({ viewport: { width: 390, height: 844 } });

type Checkpoint = { id: number; status: string; statement_balance: { quantity_value: string } };

test('an invalidated statement can be reconciled again to the corrected balance', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const day = todayISO();
  // Both movements are dated before the statement so the late one falls
  // inside the reconciled period.
  const movedOn = daysFromTodayISO(-2);
  const cash = await createCashAccount(page, csrfToken, `Recovery cash ${suffix}`, currencyID, movedOn);
  const income = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: `Recovery dividends ${suffix}`, category_type: 'income'
  });
  const move = (description: string, cents: number, extra: Record<string, unknown> = {}) =>
    apiJSON<{ journal_entries: Array<{ postings: Array<{ id: number; account_id: number }> }> }>(
      page, 'POST', '/api/v1/transactions', csrfToken, {
        transaction_date: movedOn, status: 'posted', description, ...extra,
        journal_entries: [{ entry_date: movedOn, postings: [
          { account_id: cash.id, commodity_id: currencyID, quantity_value: String(cents), quantity_scale: 2 },
          { account_id: income.id, commodity_id: currencyID, quantity_value: String(-cents), quantity_scale: 2 }
        ] }]
      });
  const checkpoints = async () =>
    (await apiJSON<{ checkpoints: Checkpoint[] }>(page, 'GET', `/api/v1/accounts/${cash.id}/reconciliation-checkpoints`)).checkpoints;

  // Reconcile the dividend through the API.
  const dividend = await move(`Dividend ${suffix}`, 10000);
  const dividendPosting = dividend.journal_entries[0].postings.find((p) => p.account_id === cash.id)!;
  await apiJSON(page, 'POST', `/api/v1/accounts/${cash.id}/postings/reconciliation-status`, csrfToken, {
    posting_version_ids: [dividendPosting.id], status: 'cleared', cleared_on: day
  });
  const first = await apiJSON<{ id: number }>(page, 'POST', `/api/v1/accounts/${cash.id}/reconciliations/start`, csrfToken, {
    commodity_id: currencyID, source_kind: 'statement', statement_date: day,
    statement_balance_value: '10000', statement_balance_scale: 2
  });
  await apiJSON(page, 'POST', `/api/v1/reconciliations/${first.id}/finish`, csrfToken, { change_reason: 'matched statement' });

  // Withholding entered late, with the explicit override of the reviewed checkpoint (#296).
  const reviewed = (await checkpoints()).map((c) => c.id);
  await move(`Withholding ${suffix}`, -1500, {
    reconciliation_override: true, reconciliation_override_checkpoint_ids: reviewed, change_reason: 'withholding entered late'
  });
  expect((await checkpoints()).map((c) => c.status)).toEqual(['invalidated']);

  await page.goto('/app/ledger/reconcile');
  await page.getByLabel('Account').selectOption({ label: cash.name });
  await page.getByLabel('Statement closing date').fill(day);
  await page.getByLabel('Statement ending balance').fill('85.00');
  await page.getByRole('button', { name: 'Start reconciling' }).click();

  const reverify = page.getByRole('row').filter({ hasText: `Dividend ${suffix}` });
  await expect(reverify.getByText('Re-verify')).toBeVisible();
  await expect(reverify.getByRole('checkbox')).toBeChecked();
  await page.getByRole('row').filter({ hasText: `Withholding ${suffix}` }).getByRole('checkbox').check();
  await expect(page.getByText('Balanced')).toBeVisible();
  await page.getByLabel('Reason').fill(`Recovery ${suffix}`);
  await page.getByRole('button', { name: 'Finish reconciliation' }).click();
  await expect(page.getByText('Reconciliation complete')).toBeVisible();

  // The old checkpoint remains inspectable as history; the new one is active.
  await expect(page.getByRole('heading', { name: 'No longer current' })).toBeVisible();
  await expect(page.getByText('Invalidated')).toBeVisible();
  await expect.poll(async () =>
    (await checkpoints()).filter((c) => c.status === 'active').map((c) => c.statement_balance.quantity_value)
  ).toEqual(['8500']);
});
