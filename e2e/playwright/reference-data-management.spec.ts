import { expect, test } from './support/fixtures';
import { apiJSON } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';

// #269 — account and category management at phone width. The API suites prove
// the rules; this proves an owner can reach and complete the ordinary
// management journeys on a 390 px screen without horizontal scrolling.

async function expectNoHorizontalScroll(page: import('@playwright/test').Page): Promise<void> {
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
}

test('[acceptance] accounts can be created, renamed and closed at 390 px', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const name = `Mobile savings ${suffix}`;
  const renamed = `Mobile reserve ${suffix}`;
  // Parent options are inside the select's wrapping label. A partial "Name"
  // label lookup also matches that select when an account contains "name".
  // Seed the collision here so this journey never depends on worker history.
  await createCashAccount(page, csrfToken, `Name collision ${suffix}`, currencyID);

  await page.goto('/app/ledger/accounts');
  await page.getByRole('button', { name: 'Add account' }).first().click();
  let form = page.locator('form').filter({ has: page.getByRole('button', { name: 'Create account' }) });
  await form.getByLabel('Name', { exact: true }).fill(name);
  await form.getByLabel('Account type').selectOption('savings');
  await form.getByLabel('Currency').selectOption(String(currencyID));
  await expectNoHorizontalScroll(page);
  await form.getByRole('button', { name: 'Create account' }).click();
  await expect(page.getByText(name).first()).toBeVisible();
  await expectNoHorizontalScroll(page);

  await page.getByRole('button', { name: `Edit ${name}` }).click();
  form = page.locator('form').filter({ has: page.getByRole('button', { name: 'Save account' }) });
  await form.getByLabel('Name', { exact: true }).fill(renamed);
  await form.getByRole('button', { name: 'Save account' }).click();
  await expect(page.getByText(renamed).first()).toBeVisible();

  const accounts = await apiJSON<{ accounts: Array<{ id: number; name?: string; status: string }> }>(
    page,
    'GET',
    '/api/v1/accounts?include_archived=true'
  );
  const saved = accounts.accounts.find((account) => account.name === renamed);
  expect(saved?.status).toBe('active');
  const versions = await apiJSON<{ accounts: Array<{ name?: string }> }>(
    page,
    'GET',
    `/api/v1/accounts/${saved?.id}/versions`
  );
  expect(versions.accounts.map((version) => version.name)).toEqual([renamed, name]);

  await page.getByRole('button', { name: `Close ${renamed}` }).click();
  const dialog = page.getByRole('dialog');
  if (await dialog.isVisible().catch(() => false)) {
    await dialog.getByRole('button', { name: /close/i }).last().click();
  }
  await expect
    .poll(async () => {
      const current = await apiJSON<{ status: string }>(page, 'GET', `/api/v1/accounts/${saved?.id}`);
      return current.status;
    })
    .toBe('closed');
  await expectNoHorizontalScroll(page);
});

test('[acceptance] categories can be created under a parent and archived at 390 px', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await readyForLedger(page);
  const name = `Mobile hobby ${Date.now()}`;

  await page.goto('/app/ledger/categories');
  await page.getByRole('button', { name: 'Add category' }).first().click();
  const form = page.locator('form').filter({ has: page.getByRole('button', { name: 'Create category' }) });
  await form.getByLabel('Name', { exact: true }).fill(name);
  await form.getByLabel('Type').selectOption('expense');
  await form.getByLabel('Parent').selectOption({ label: 'Personal' });
  await expectNoHorizontalScroll(page);
  await form.getByRole('button', { name: 'Create category' }).click();
  await expect(page.getByText(name).first()).toBeVisible();

  const categories = await apiJSON<{
    categories: Array<{ id: number; name?: string; code?: string; parent_category_id?: number; status: string }>;
  }>(page, 'GET', '/api/v1/categories');
  const created = categories.categories.find((category) => category.name === name);
  const personal = categories.categories.find((category) => category.code === 'expense_personal');
  expect(created?.parent_category_id).toBe(personal?.id);

  await page.getByRole('button', { name: `Archive ${name}` }).click();
  const dialog = page.getByRole('dialog');
  if (await dialog.isVisible().catch(() => false)) {
    await dialog.getByRole('button', { name: /archive/i }).last().click();
  }
  await expect
    .poll(async () => {
      const current = await apiJSON<{ status: string }>(page, 'GET', `/api/v1/categories/${created?.id}`);
      return current.status;
    })
    .toBe('archived');
  await expectNoHorizontalScroll(page);
});
