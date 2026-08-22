import { expect, test } from '@playwright/test';
import { apiJSON, csrfTokenFor } from './support/api';
import { todayISO } from './support/dates';
import { createCashAccount, expectSavedTransaction, readyForLedger } from './support/ledger';

test('creates a simple transaction through the browser form', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const accountName = `E2E Checking ${Date.now()}`;
  const description = `E2E Coffee ${Date.now()}`;
  const payeeName = `E2E Cafe ${Date.now()}`;

  await createCashAccount(page, csrfToken, accountName, currencyID);

  await page.goto('/app/transactions');
  await page.getByRole('button', { name: 'New transaction' }).click();

  await expect(page.getByRole('heading', { name: 'Add transaction' })).toBeVisible();
  const form = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Add transaction' }) });
  await form.getByLabel('Date').fill(todayISO());
  await form.getByLabel('Description').fill(description);
  await form.getByLabel('Account').selectOption({ label: accountName });
  await form.getByLabel('Amount').fill('-4.50');
  await form.getByPlaceholder('Search categories').click();
  await form.locator('ul[role="listbox"] button[role="option"]').first().click();
  // The payee goes last: its suggestion list is also a `listbox`, so filling it
  // earlier would leave two open at once and the category pick could land in
  // the wrong one.
  await form.getByRole('textbox', { name: 'Payee' }).fill(payeeName);
  await form.getByRole('button', { name: 'Add transaction' }).click();

  // A payee nobody has recorded is not saved as free text: creating the record
  // is confirmed first, so the transaction can group in the spending report.
  await expect(form.getByText(`No payee is named “${payeeName}” yet.`)).toBeVisible();
  await form.getByRole('button', { name: `Create “${payeeName}”` }).click();

  await expectSavedTransaction(page, description);

  const payees = await apiJSON<{ payees: Array<{ id: number; name: string }> }>(page, 'GET', '/api/v1/payees');
  const created = payees.payees.find((payee) => payee.name === payeeName);
  expect(created).toBeDefined();

  const linked = await apiJSON<{ transactions: Array<{ payee_id?: number }> }>(
    page,
    'GET',
    `/api/v1/transactions?payee_id=${created?.id}`
  );
  expect(linked.transactions).toHaveLength(1);
});

test('offers an existing payee before creating a near-duplicate', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const accountName = `Fuzzy Checking ${Date.now()}`;
  const description = `Fuzzy groceries ${Date.now()}`;
  const suffix = Date.now();
  const payeeName = `Market Hall ${suffix}`;

  await createCashAccount(page, csrfToken, accountName, currencyID);
  const payee = await apiJSON<{ id: number; name: string }>(
    page,
    'POST',
    '/api/v1/payees',
    await csrfTokenFor(page),
    { name: payeeName },
    [201]
  );

  await page.goto('/app/transactions');
  await page.getByRole('button', { name: 'New transaction' }).click();
  const form = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Add transaction' }) });

  await form.getByLabel('Date').fill(todayISO());
  await form.getByLabel('Description').fill(description);
  await form.getByLabel('Account').selectOption({ label: accountName });
  await form.getByLabel('Amount').fill('-9.99');
  await form.getByPlaceholder('Search categories').click();
  await form.locator('ul[role="listbox"] button[role="option"]').first().click();
  // A typo, which is the case this exists for: a substring search would find
  // nothing here and the user would create a second, near-duplicate payee.
  await form.getByRole('textbox', { name: 'Payee' }).fill(`Markt Hall ${suffix}`);
  await form.getByRole('button', { name: 'Add transaction' }).click();

  await expect(form.getByText('Did you mean one of these?')).toBeVisible();
  await form.getByRole('button', { name: payeeName, exact: true }).click();

  await expectSavedTransaction(page, description);

  // The existing payee was reused; no near-duplicate was created.
  const payees = await apiJSON<{ payees: Array<{ id: number; name: string }> }>(page, 'GET', '/api/v1/payees');
  expect(payees.payees.filter((item) => item.name.includes(String(suffix)))).toHaveLength(1);

  const linked = await apiJSON<{ transactions: Array<{ payee_id?: number }> }>(
    page,
    'GET',
    `/api/v1/transactions?payee_id=${payee.id}`
  );
  expect(linked.transactions).toHaveLength(1);
});
