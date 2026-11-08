import { test, expect } from './support/owned-book';
import { apiJSON } from './support/api';
import { seedBulkAccounts } from './support/bulk-accounts';
import { createCashAccount, readyForLedger } from './support/ledger';

/**
 * #518: the account list is cursor-paged at 2,000 accounts. A register reached
 * directly must still name its account and seed New transaction, and the
 * editor's Account picker must offer an account that sorts past the first page.
 */

test('a register and its account picker reach an account beyond the first account page', async ({ page, ownedBookDatabasePath }) => {
  test.setTimeout(120_000);
  const { csrfToken, currencyID } = await readyForLedger(page);
  // Seeded names sort before the target's, so it lands on the second page.
  seedBulkAccounts(ownedBookDatabasePath, 'Aa bulk', 2100);
  const target = await createCashAccount(page, csrfToken, 'Zz catalog target', currencyID);

  const first = await apiJSON<{ accounts: Array<{ id: number }>; next_cursor: string | null }>(page, 'GET', '/api/v1/accounts');
  expect(first.accounts).toHaveLength(2000);
  expect(first.next_cursor).not.toBeNull();
  expect(first.accounts.some((account) => account.id === target.id)).toBe(false);

  await page.goto(`/app/ledger/accounts/${target.id}/register`);
  const header = page.locator('main header').first();
  await expect(header.getByRole('heading', { level: 1, name: target.name })).toBeVisible();
  await header.getByRole('button', { name: 'New transaction', exact: true }).click();
  const entry = page.getByRole('region', { name: 'New transaction', exact: true });
  await expect(entry.getByLabel('Account').locator('option:checked')).toHaveText(target.name);
});
