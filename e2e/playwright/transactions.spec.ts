import { expect, test } from '@playwright/test';
import { todayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';

test('creates a simple transaction through the browser form', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const accountName = `E2E Checking ${Date.now()}`;
  const description = `E2E Coffee ${Date.now()}`;

  await createCashAccount(page, csrfToken, accountName, currencyID);

  await page.goto('/app/transactions');
  await page.getByRole('button', { name: 'New transaction' }).click();

  await expect(page.getByRole('heading', { name: 'Add transaction' })).toBeVisible();
  const form = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Add transaction' }) });
  await form.getByLabel('Date').fill(todayISO());
  await form.getByLabel('Payee').fill('E2E Cafe');
  await form.getByLabel('Description').fill(description);
  await form.getByLabel('Account').selectOption({ label: accountName });
  await form.getByLabel('Amount').fill('-4.50');
  await form.getByPlaceholder('Search categories').click();
  await form.locator('ul[role="listbox"] button[role="option"]').first().click();
  await form.getByRole('button', { name: 'Add transaction' }).click();

  await expect(page.getByText(description)).toBeVisible();
});
