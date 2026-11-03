import { expect, test, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { expectFocusTrapped } from './support/a11y';
import { createCashAccount, readyForLedger } from './support/ledger';
import { daysFromTodayISO, todayISO } from './support/dates';

/**
 * #327: every modal dialog is the shared native one. Opened from the keyboard,
 * it takes focus, keeps Tab and Shift+Tab inside, closes on Escape unless a
 * write is in flight, and hands focus back to the control that opened it.
 * Covers the reason dialog, the reconciliation override it hands over to, the
 * Deleted restore override and an investment correction. Phone-sized screen.
 */

test.use({ viewport: { width: 390, height: 844 } });

test.afterEach(async ({ page }) => {
  await page.unrouteAll({ behavior: 'ignoreErrors' });
});

/** A spend reconciled on today's statement; `deleted` soft-deletes it first, so a restore needs an override. */
async function seedReconciled(page: Page, label: string, deleted = false) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const checking = await createCashAccount(page, csrfToken, `${label} checking ${suffix}`, currencyID);
  const category = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: `${label} groceries ${suffix}`, category_type: 'expense'
  });
  const day = todayISO();
  const spend = (description: string) =>
    apiJSON<{ id: number; journal_entries: Array<{ postings: Array<{ id: number; account_id: number }> }> }>(
      page, 'POST', '/api/v1/transactions', csrfToken, {
        transaction_date: day, status: 'posted', description,
        journal_entries: [{ entry_date: day, postings: [
          { account_id: checking.id, commodity_id: currencyID, quantity_value: '-1000', quantity_scale: 2 },
          { account_id: category.id, commodity_id: currencyID, quantity_value: '1000', quantity_scale: 2 }
        ] }]
      });
  const subject = await spend(`${label} subject ${suffix}`);
  let covered = subject;
  if (deleted) {
    await apiJSON(page, 'POST', `/api/v1/transactions/${subject.id}/soft-delete`, csrfToken, { change_reason: 'duplicate' });
    covered = await spend(`${label} kept ${suffix}`);
  }
  const posting = covered.journal_entries[0].postings.find((p) => p.account_id === checking.id)!;
  await apiJSON(page, 'POST', `/api/v1/accounts/${checking.id}/postings/reconciliation-status`, csrfToken, {
    posting_version_ids: [posting.id], status: 'cleared', cleared_on: day
  });
  const session = await apiJSON<{ id: number }>(page, 'POST', `/api/v1/accounts/${checking.id}/reconciliations/start`, csrfToken, {
    commodity_id: currencyID, source_kind: 'statement', statement_date: day,
    statement_balance_value: '-1000', statement_balance_scale: 2
  });
  await apiJSON(page, 'POST', `/api/v1/reconciliations/${session.id}/finish`, csrfToken, { change_reason: 'matched statement' });
  return { checking, subject, description: `${label} subject ${suffix}` };
}

test('the void dialog and its reconciliation override keep and return focus', async ({ page }) => {
  const s = await seedReconciled(page, 'Focus');
  await page.goto(`/app/ledger/accounts/${s.checking.id}/register`);
  await page.getByRole('row').filter({ hasText: s.description }).click();

  const trigger = page.getByRole('button', { name: 'Void…' });
  await trigger.focus();
  await page.keyboard.press('Enter');
  const reasonDialog = page.getByRole('dialog', { name: 'Void transaction' });
  await expect(reasonDialog).toBeVisible();
  await expect(reasonDialog.getByLabel('Reason')).toBeFocused();
  await expectFocusTrapped(page, reasonDialog, 'void reason');
  await page.keyboard.press('Escape');
  await expect(reasonDialog).toHaveCount(0);
  await expect(trigger).toBeFocused();

  // The reason dialog hands over to the override; closing that returns focus
  // to the control that started the flow, not to <body>.
  await page.keyboard.press('Enter');
  await reasonDialog.getByLabel('Reason').fill('duplicate charge');
  await reasonDialog.getByRole('button', { name: 'Void', exact: true }).click();
  const warning = page.getByRole('alertdialog', { name: 'This change affects a reconciled period' });
  await expect(warning).toBeVisible();
  // Destructive confirmation: focus starts on the safe choice.
  await expect(warning.getByRole('button', { name: 'Go back' })).toBeFocused();
  await expectFocusTrapped(page, warning, 'void override');
  await page.keyboard.press('Escape');
  await expect(warning).toHaveCount(0);
  await expect(trigger).toBeFocused();
});

test('the Deleted restore override keeps focus, ignores Escape while restoring, and returns focus', async ({ page }) => {
  const s = await seedReconciled(page, 'Trash focus', true);

  await page.goto('/app/ledger/transactions?view=deleted');
  const trigger = page.getByRole('row').filter({ hasText: s.description }).getByRole('button', { name: /^Restore/ });
  await trigger.focus();
  await page.keyboard.press('Enter');
  const warning = page.getByRole('alertdialog', { name: 'Restore and invalidate reconciliation?' });
  await expect(warning.getByRole('listitem')).not.toHaveCount(0);
  await expect(warning.getByRole('button', { name: 'Restore anyway' })).not.toBeFocused();
  await expectFocusTrapped(page, warning, 'Deleted restore');
  await page.keyboard.press('Escape');
  await expect(warning).toHaveCount(0);
  await expect(trigger).toBeFocused();

  // While the restore is in flight Escape is ignored.
  let release!: () => void;
  const held = new Promise<void>((resolve) => { release = resolve; });
  await page.route(`**/api/v1/transactions/${s.subject.id}/restore`, async (route) => {
    await held;
    await route.continue();
  });
  await page.keyboard.press('Enter');
  await warning.getByRole('button', { name: 'Restore anyway' }).click();
  await expect(warning.getByRole('button', { name: 'Restoring…' })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(warning).toBeVisible();
  release();
  await expect(warning).toHaveCount(0);
});

test('an investment correction dialog keeps and returns focus', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = `dlg${Date.now()}`;
  // Yesterday holds whatever the runner zone is against the owner's day.
  const day = daysFromTodayISO(-1);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Dialog cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: day, effective_from: day
  });
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: `Dialog ${suffix}`, symbol: suffix.toUpperCase(),
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: day
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Dialog broker ${suffix}`, opened_on: day, effective_from: day
  });
  const buy = await apiJSON<{ transaction: { id: number } }>(page, 'POST', '/api/v1/investments/buy', csrfToken, {
    transaction_date: day, commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: cash.id, quantity_value: '10', quantity_scale: 0, cash_amount_value: '10000',
    cash_amount_scale: 2, cash_commodity_id: currencyID, cost_basis_method: 'fifo'
  });

  await page.goto(`/app/ledger/transactions?transaction_id=${buy.transaction.id}`);
  const trigger = page.getByRole('button', { name: 'Correct buy…' });
  await trigger.focus();
  await page.keyboard.press('Enter');
  const dialog = page.getByRole('dialog', { name: 'Correct this buy' });
  await expect(dialog.getByLabel('Reason for correction')).toBeVisible();
  await expectFocusTrapped(page, dialog, 'buy correction', 30);
  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
  await expect(trigger).toBeFocused();
});
