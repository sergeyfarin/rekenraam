import { expect, test } from './support/fixtures';
import { apiJSON } from './support/api';
import { todayISO } from './support/dates';
import { createCashAccount, ensureCurrency, readyForLedger } from './support/ledger';

// An open editor holds the owner's unsaved input. Reference data refetched
// behind it (here: a record added from another tab, picked up on network
// reconnect) must not reset the form.

test('a new-account draft survives a currency refetch while the editor is open', async ({ page }) => {
  const { csrfToken } = await readyForLedger(page);
  const draft = `Refetch draft ${Date.now()}`;

  await page.clock.install();
  await page.goto('/app/ledger/accounts');
  await page.getByRole('button', { name: 'Add account' }).first().click();
  const form = page.locator('form').filter({ has: page.getByRole('button', { name: 'Create account' }) });
  const nameField = form.getByLabel('Name', { exact: true });
  await nameField.fill(draft);
  await form.getByLabel('Account type').selectOption('savings');

  await ensureCurrency(page, csrfToken, 'CHF', 'Swiss franc');
  // Past the query staleTime, then a reconnect: TanStack refetches stale queries.
  await page.clock.fastForward('00:45');
  const refetched = page.waitForResponse(
    (response) => new URL(response.url()).pathname === '/api/v1/currencies' && response.ok()
  );
  await page.context().setOffline(true);
  await page.context().setOffline(false);
  const body = (await (await refetched).json()) as { currencies: Array<{ code: string }> };
  expect(body.currencies.map((currency) => currency.code)).toContain('CHF');

  await expect(nameField).toHaveValue(draft);
  await expect(form.getByLabel('Account type')).toHaveValue('savings');
});

test('a transaction edit survives a category refetch while the editor is open', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const accountName = `Refetch wallet ${suffix}`;
  const description = `Refetch lunch ${suffix}`;
  const edited = `${description} edited`;
  const account = await createCashAccount(page, csrfToken, accountName, currencyID);
  const category = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: `Refetch meals ${suffix}`, category_type: 'expense'
  });
  const today = todayISO();
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_date: today, status: 'posted', description,
    journal_entries: [{ entry_date: today, postings: [
      { account_id: account.id, commodity_id: currencyID, quantity_value: '-1250', quantity_scale: 2 },
      { account_id: category.id, commodity_id: currencyID, quantity_value: '1250', quantity_scale: 2 }
    ] }]
  });

  await page.clock.install();
  await page.goto('/app/ledger/transactions');
  await page.getByRole('row').filter({ hasText: description }).getByRole('button', { name: 'Edit', exact: true }).click();
  const edit = page.getByRole('complementary', { name: 'Actions' });
  await expect(edit.getByRole('heading', { name: 'Edit transaction' })).toBeVisible();
  const descriptionField = edit.getByLabel('Description');
  await descriptionField.fill(edited);

  await apiJSON(page, 'POST', '/api/v1/categories', csrfToken, { name: `Refetch other ${suffix}`, category_type: 'expense' });
  await page.clock.fastForward('00:45');
  const refetched = page.waitForResponse(
    (response) => new URL(response.url()).pathname === '/api/v1/categories' && response.ok()
  );
  await page.context().setOffline(true);
  await page.context().setOffline(false);
  await refetched;

  await expect(descriptionField).toHaveValue(edited);
});
