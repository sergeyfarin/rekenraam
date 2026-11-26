import type { Locator, Page } from '@playwright/test';
import { expect, test } from './support/owned-book';
import { apiJSON } from './support/api';
import { seedBulkPayees } from './support/bulk-payees';
import { createCashAccount, readyForLedger } from './support/ledger';

const suggestionPath = '/api/v1/payees/suggestions';

async function openExpense(page: Page, account: string, category: string) {
  await page.getByRole('list', { name: 'Shortcuts', exact: true }).getByRole('button', { name: 'New transaction', exact: true }).click();
  const entry = page.getByRole('dialog', { name: 'New transaction', exact: true });
  await entry.getByRole('combobox', { name: 'Account', exact: true }).selectOption({ label: account });
  await entry.getByLabel('Amount', { exact: true }).fill('4.50');
  await entry.getByRole('combobox', { name: 'Category', exact: true }).fill(category);
  await entry.getByRole('listbox', { name: 'Category', exact: true }).getByText(category, { exact: true }).click();
  return entry;
}

function saveButton(entry: Locator) {
  return entry.getByRole('button', { name: 'Save transaction', exact: true });
}

async function submitDespiteDisabledButton(entry: Locator) {
  const form = entry.locator('form');
  const dispatched = await form.evaluate((element) => {
    const formElement = element as HTMLFormElement;
    let submitted = false;
    const observe = () => { submitted = true; };
    formElement.addEventListener('submit', observe);
    formElement.requestSubmit();
    formElement.removeEventListener('submit', observe);
    return submitted;
  });
  expect(dispatched).toBe(true);
  await expect(form).toHaveAttribute('aria-busy', 'false');
}

async function expectLinkedSave(page: Page, entry: Locator, payeeID: number, action: Locator) {
  const saved = page.waitForResponse((response) =>
    new URL(response.url()).pathname === '/api/v1/transactions' && response.request().method() === 'POST'
  );
  await action.click();
  const response = await saved;
  expect(response.status()).toBe(201);
  const transaction = await response.json() as { id: number; payee_id: number };
  expect(transaction.payee_id).toBe(payeeID);
  await expect(entry).toHaveCount(0);
  return transaction.id;
}

test.describe('on a touch phone', () => {
  test.use({ hasTouch: true, viewport: { width: 390, height: 844 } });

  test('keyboard and touch select Vendor 2000 beyond the legacy payee list', async ({ page, ownedBookDatabasePath }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const account = await createCashAccount(page, csrfToken, 'Payee lookup checking', currencyID);
    const category = await apiJSON<{ name: string }>(page, 'POST', '/api/v1/categories', csrfToken, {
      name: 'Payee lookup expense', category_type: 'expense'
    });
    await apiJSON(page, 'POST', '/api/v1/payees', csrfToken, { name: 'Zz existing payee' });
    seedBulkPayees(ownedBookDatabasePath, 2000);
    const target = await apiJSON<{ id: number; name: string }>(page, 'POST', '/api/v1/payees', csrfToken, { name: 'Vendor 2000' });
    await page.goto('/app');

    const transactionIDs: number[] = [];
    for (const selection of ['keyboard', 'touch'] as const) {
      const entry = await openExpense(page, account.name, category.name);
      const field = entry.getByRole('combobox', { name: 'Payee', exact: true });
      await field.fill('vendor 2000');
      const option = entry.getByRole('option', { name: target.name, exact: true });
      await expect(option).toBeVisible();
      if (selection === 'touch') {
        await option.tap();
      } else {
        await field.press('ArrowDown');
        await expect(option).toHaveAttribute('aria-selected', 'true');
        await field.press('Enter');
      }
      await expect(field).toHaveValue(target.name);
      await expect(option).toHaveCount(0);
      transactionIDs.push(await expectLinkedSave(page, entry, target.id, saveButton(entry)));
    }

    const nearMatch = await openExpense(page, account.name, category.name);
    await nearMatch.getByRole('combobox', { name: 'Payee', exact: true }).fill('Vendr 2000');
    await saveButton(nearMatch).click();
    const confirmation = nearMatch.getByRole('group', { name: 'No payee is named “Vendr 2000” yet.' });
    await expect(confirmation).toBeVisible();
    transactionIDs.push(await expectLinkedSave(page, nearMatch, target.id, confirmation.getByRole('button', { name: target.name, exact: true })));

    await apiJSON(page, 'POST', `/api/v1/payees/${target.id}/archive`, csrfToken, {});
    await page.reload();
    const entry = await openExpense(page, account.name, category.name);
    await entry.getByRole('combobox', { name: 'Payee', exact: true }).fill(target.name);
    await expect(entry.getByRole('listbox', { name: 'Payee' })).toBeVisible();
    await expect(entry.getByRole('option', { name: target.name, exact: true })).toHaveCount(0);
    for (const id of transactionIDs) {
      const saved = await apiJSON(page, 'GET', `/api/v1/transactions/${id}`);
      expect(saved).toMatchObject({ payee_id: target.id, payee_name: target.name });
    }

    const longName = 'V'.repeat(200);
    await apiJSON(page, 'POST', '/api/v1/payees', csrfToken, { name: longName });
    await entry.getByRole('combobox', { name: 'Payee', exact: true }).fill(longName);
    const longOption = entry.getByRole('option', { name: longName, exact: true });
    await expect(longOption).toBeVisible();
    const { scrollWidth, clientWidth } = await longOption.evaluate((option) => ({
      scrollWidth: option.scrollWidth,
      clientWidth: option.clientWidth
    }));
    expect(scrollWidth).toBeLessThanOrEqual(clientWidth);
  });
});

test('pending and failed payee lookups stop a new-payee confirmation until retry succeeds', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const account = await createCashAccount(page, csrfToken, 'Lookup retry checking', currencyID);
  const category = await apiJSON<{ name: string }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: 'Lookup retry expense', category_type: 'expense'
  });
  const payee = await apiJSON<{ id: number; name: string }>(page, 'POST', '/api/v1/payees', csrfToken, { name: 'Market Hall' });
  await page.goto('/app');
  const entry = await openExpense(page, account.name, category.name);
  const field = entry.getByRole('combobox', { name: 'Payee', exact: true });
  await field.fill(payee.name);
  const previousOption = entry.getByRole('option', { name: payee.name, exact: true });
  await expect(previousOption).toBeVisible();

  const typedName = 'Markt Hall';
  let releaseFailure!: () => void;
  const failure = new Promise<void>((resolve) => { releaseFailure = resolve; });
  await page.route(`**${suggestionPath}?*`, async (route) => {
    if (new URL(route.request().url()).searchParams.get('q') !== typedName) return route.fallback();
    await failure;
    await route.fulfill({ status: 503, json: { error: { code: 'RESOURCE_BUSY', message: 'Try again' } } });
  }, { times: 1 });
  const request = page.waitForRequest((request) => new URL(request.url()).pathname === suggestionPath);
  const writes: string[] = [];
  page.on('request', (request) => {
    const path = new URL(request.url()).pathname;
    if (request.method() === 'POST' && ['/api/v1/transactions', '/api/v1/payees'].includes(path)) writes.push(path);
  });
  await field.fill(typedName);
  await request;

  const confirmation = entry.getByRole('group', { name: `No payee is named “${typedName}” yet.` });
  try {
    await expect(entry.getByText('Searching payees…')).toBeVisible();
    await expect(previousOption).toHaveCount(0);
    await expect(saveButton(entry)).toBeDisabled();
    await submitDespiteDisabledButton(entry);
    await expect(confirmation).toHaveCount(0);
  } finally {
    releaseFailure();
  }

  await expect(entry.getByText('Could not search payees.', { exact: true })).toBeVisible();
  await expect(saveButton(entry)).toBeDisabled();
  await submitDespiteDisabledButton(entry);
  await expect(confirmation).toHaveCount(0);
  expect(writes).toEqual([]);

  await entry.getByRole('button', { name: 'Retry', exact: true }).click();
  await saveButton(entry).click();
  await expect(confirmation.getByRole('button', { name: payee.name, exact: true })).toBeVisible();
  await expectLinkedSave(page, entry, payee.id, confirmation.getByRole('button', { name: payee.name, exact: true }));
  expect(writes).toEqual(['/api/v1/transactions']);
});
