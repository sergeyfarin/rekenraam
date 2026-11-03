import { expect, test, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { daysFromTodayISO, todayISO } from './support/dates';
import { createCashAccount, expectSavedTransaction, readyForLedger } from './support/ledger';

/**
 * #270 (area 3 of #260): the phone journey for entry and reconciliation. A
 * transaction is entered in the account register's form and the statement is
 * reconciled on the reconcile screen, both at 390 px without horizontal
 * scrolling. The figures are hand-derived: 250.00 opening less 12.40 = 237.60.
 */

test.use({ viewport: { width: 390, height: 844 } });

async function expectNoHorizontalScroll(page: Page): Promise<void> {
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
}

type Checkpoint = { status: string; statement_balance: { quantity_value: string; quantity_scale: number } };

test('[acceptance] a transaction is entered and its statement reconciled at 390 px', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const day = todayISO();
  const openedOn = daysFromTodayISO(-3);
  const account = await createCashAccount(page, csrfToken, `Phone checking ${suffix}`, currencyID, openedOn);
  const salary = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: `Phone salary ${suffix}`, category_type: 'income'
  });
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_date: openedOn, description: `Phone opening ${suffix}`,
    journal_entries: [{ entry_date: openedOn, postings: [
      { account_id: account.id, commodity_id: currencyID, quantity_value: '25000', quantity_scale: 2 },
      { account_id: salary.id, commodity_id: currencyID, quantity_value: '-25000', quantity_scale: 2 }
    ] }]
  });

  // Entry: the register's form starts in this account.
  const description = `Phone groceries ${suffix}`;
  await page.goto(`/app/ledger/accounts/${account.id}/register`);
  await expect(page.getByRole('heading', { name: account.name })).toBeVisible();
  await expectNoHorizontalScroll(page);
  await page.getByRole('button', { name: 'Add transaction' }).click();
  const form = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Add transaction' }) });
  await expect(form.getByLabel('Account').locator('option:checked')).toHaveText(account.name);
  await form.getByLabel('Date').fill(day);
  await form.getByLabel('Description').fill(description);
  await form.getByLabel('Amount').fill('-12.40');
  await form.getByPlaceholder('Search categories').click();
  await form.locator('ul[role="listbox"] button[role="option"]').first().click();
  await expectNoHorizontalScroll(page);
  await form.getByRole('button', { name: 'Add transaction' }).click();
  await expectSavedTransaction(page, description);
  await page.getByRole('button', { name: 'Close' }).first().click();
  await expect(page.getByText(description)).toBeVisible();
  await expectNoHorizontalScroll(page);

  const balances = await apiJSON<{ accounts: Array<{ account_id: number; direct_balances: Array<{ quantity_value: string; quantity_scale: number }> }> }>(
    page, 'GET', `/api/v1/ledger/account-balances?as_of=${day}`);
  const balance = balances.accounts.find((row) => row.account_id === account.id)!.direct_balances[0];
  expect([balance.quantity_value, balance.quantity_scale]).toEqual(['23760', 2]);

  // Reconciliation: both postings are uncleared, so both are ticked by hand.
  await page.goto('/app/ledger/reconcile');
  await page.getByLabel('Account').selectOption({ label: account.name });
  await page.getByLabel('Statement closing date').fill(day);
  await page.getByLabel('Statement ending balance').fill('237.60');
  await page.getByRole('button', { name: 'Start reconciling' }).click();
  for (const text of [`Phone opening ${suffix}`, description]) {
    await page.getByRole('row').filter({ hasText: text }).getByRole('checkbox').check();
  }
  await expect(page.getByText('Balanced')).toBeVisible();
  await expectNoHorizontalScroll(page);
  await page.getByLabel('Reason').fill(`Phone statement ${suffix}`);
  await page.getByRole('button', { name: 'Finish reconciliation' }).click();
  await expect(page.getByText('Reconciliation complete')).toBeVisible();
  await expectNoHorizontalScroll(page);

  await expect.poll(async () =>
    (await apiJSON<{ checkpoints: Checkpoint[] }>(page, 'GET', `/api/v1/accounts/${account.id}/reconciliation-checkpoints`))
      .checkpoints.map((c) => [c.status, c.statement_balance.quantity_value])
  ).toEqual([['active', '23760']]);
});
