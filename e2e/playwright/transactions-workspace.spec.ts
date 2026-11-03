import { expect, test, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { todayISO } from './support/dates';
import { createCashAccount, expectSavedTransaction, readyForLedger } from './support/ledger';

/**
 * #185: the account register and a category's transactions share the main
 * screen's workspace, so a transaction can be entered where the user is
 * working, starting from that account or category, on a phone-sized screen.
 */

test.use({ viewport: { width: 390, height: 844 } });

async function addForm(page: Page) {
  await page.getByRole('button', { name: 'Add transaction' }).click();
  await expect(page.getByRole('heading', { name: 'Add transaction' })).toBeVisible();
  return page.locator('form').filter({ has: page.getByRole('heading', { name: 'Add transaction' }) });
}

test('a transaction entered in an account register starts in that account', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `Workspace checking ${suffix}`, currencyID);
  const description = `Register groceries ${suffix}`;

  await page.goto(`/app/ledger/accounts/${account.id}/register`);
  await expect(page.getByRole('heading', { name: account.name })).toBeVisible();
  const form = await addForm(page);
  await expect(form.getByLabel('Account').locator('option:checked')).toHaveText(account.name);
  await form.getByLabel('Date').fill(todayISO());
  await form.getByLabel('Description').fill(description);
  await form.getByLabel('Amount').fill('-12.40');
  await form.getByPlaceholder('Search categories').click();
  await form.locator('ul[role="listbox"] button[role="option"]').first().click();
  await form.getByRole('button', { name: 'Add transaction' }).click();
  await expectSavedTransaction(page, description);

  // The register lists it without a reload.
  await page.getByRole('button', { name: 'Close' }).first().click();
  await expect(page.getByText(description)).toBeVisible();
});

test('a transaction entered in a category starts in that category', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `Workspace wallet ${suffix}`, currencyID);
  const category = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: `Workspace hobbies ${suffix}`, category_type: 'expense'
  });
  const description = `Category paints ${suffix}`;

  await page.goto(`/app/ledger/categories/${category.id}`);
  await expect(page.getByRole('heading', { name: `Workspace hobbies ${suffix}` })).toBeVisible();
  const form = await addForm(page);
  await expect(form.getByText(`Workspace hobbies ${suffix}`)).toBeVisible();
  await form.getByLabel('Date').fill(todayISO());
  await form.getByLabel('Description').fill(description);
  await form.getByLabel('Account').selectOption({ label: account.name });
  await form.getByLabel('Amount').fill('-30.00');
  await form.getByRole('button', { name: 'Add transaction' }).click();
  await expectSavedTransaction(page, description);

  const listed = await apiJSON<{ transactions: Array<{ description: string }> }>(
    page, 'GET', `/api/v1/transactions?category_id=${category.id}`);
  expect(listed.transactions.map((item) => item.description)).toContain(description);
});

// A holding account's own entry is covered by investment-entry-context.spec.ts (#186).
