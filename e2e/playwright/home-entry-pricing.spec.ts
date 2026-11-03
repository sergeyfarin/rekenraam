import { test, expect } from './support/owned-book';
import { apiJSON, csrfTokenFor } from './support/api';
import { todayISO } from './support/dates';
import { ensureCurrency, readyForLedger } from './support/ledger';

// The shared entry dialog saves over any page (#477). On Prices, the first
// posting in a currency no account defaults to creates the exchange rate that
// coverage needs; the page beneath must list it without a reload. An owned
// book, so this is the book's first exposure to that currency, entered on a
// multi-currency receivable whose default currency adds no pair of its own.

test('saving the first foreign-currency transaction over Prices lists the rate it needs', async ({ page }) => {
  const { currencyID } = await readyForLedger(page);
  let csrfToken = await csrfTokenFor(page);
  await apiJSON(page, 'POST', `/api/v1/currencies/${currencyID}/default`, csrfToken, undefined, [200]);
  csrfToken = await csrfTokenFor(page);
  const isk = await ensureCurrency(page, csrfToken, 'ISK', 'Icelandic króna');
  const account = await apiJSON<{ name: string }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: 'Reykjavik deposit owed', account_class: 'asset', account_kind: 'receivable', allows_postings: true
  });
  const category = await apiJSON<{ name: string }>(page, 'POST', '/api/v1/categories', csrfToken, { name: 'Travel meals', category_type: 'expense' });

  await page.goto('/app/investments/prices');
  await expect(page.getByRole('heading', { name: 'Stored price coverage', exact: true })).toBeVisible();
  const pair = page.locator('main').getByRole('button', { name: /^ISK in / });
  await expect(pair).toHaveCount(0);

  await page.locator('#main-content').focus();
  await page.keyboard.press('n');
  const dialog = page.getByRole('dialog', { name: 'Add transaction', exact: true });
  await dialog.getByLabel('Date', { exact: true }).fill(todayISO());
  await dialog.getByLabel('Description', { exact: true }).fill('Dinner in Reykjavik');
  await dialog.getByRole('combobox', { name: 'Account', exact: true }).selectOption({ label: account.name });
  await dialog.getByRole('combobox', { name: 'Currency', exact: true }).selectOption({ label: 'ISK' });
  await dialog.getByLabel('Amount', { exact: true }).fill('-4500');
  await dialog.getByPlaceholder('Search categories').fill(category.name);
  await dialog.locator('ul[role="listbox"] button[role="option"]').filter({ hasText: category.name }).first().click();
  const refreshed = page.waitForResponse(response => response.request().method() === 'GET' && new URL(response.url()).pathname === '/api/v1/pricing/workspace' && response.ok());
  await dialog.getByRole('button', { name: 'Add transaction', exact: true }).click();
  const workspace = await (await refreshed).json() as { coverage: Array<{ base_commodity_id: number; status: string }> };
  expect(workspace.coverage.find(item => item.base_commodity_id === isk)).toMatchObject({ status: 'missing' });
  await expect(dialog).toHaveCount(0);
  await expect(pair).toBeVisible();
});
