import { expect, type Page, test } from '@playwright/test';
import { apiJSON, csrfTokenFor } from './support/api';
import { todayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';

test('spending report ranks categories, nets refunds, and ignores transfers', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const today = todayISO();

  const checking = await createCashAccount(page, csrfToken, `Reports checking ${suffix}`, currencyID);
  const savings = await createCashAccount(page, csrfToken, `Reports savings ${suffix}`, currencyID);

  const categories = await apiJSON<{
    categories: Array<{ id: number; code?: string; allows_postings: boolean }>;
  }>(page, 'GET', '/api/v1/categories');
  const category = (code: string) => {
    const found = categories.categories.find((item) => item.code === code && item.allows_postings);
    if (!found) throw new Error(`seeded category ${code} not found`);
    return found.id;
  };
  const groceries = category('expense_food_groceries');
  const fuel = category('expense_transport_fuel');

  // 80.00 groceries, then a 30.00 refund against the same category: the report
  // must show 50.00, not 110.00.
  await postTransaction(page, today, checking.id, groceries, currencyID, 8000);
  await postTransaction(page, today, checking.id, groceries, currencyID, -3000);
  // 60.00 fuel, so groceries (50.00) must rank below it.
  await postTransaction(page, today, checking.id, fuel, currencyID, 6000);
  // A transfer between the user's own accounts has no category posting and must
  // never appear as spending.
  await postTransfer(page, today, checking.id, savings.id, currencyID, 25000);

  await page.goto(`/app/reports?view=spending&group_by=category&start_date=${today}&end_date=${today}&bucket=month`);

  const table = page.getByRole('table');
  await expect(table).toBeVisible();
  const rows = table.locator('tbody tr');
  await expect(rows).toHaveCount(2);

  // Fuel 60.00 outranks the net 50.00 of groceries.
  await expect(rows.nth(0)).toContainText('Fuel');
  await expect(rows.nth(0)).toContainText('60.00');
  await expect(rows.nth(1)).toContainText('Groceries');
  await expect(rows.nth(1)).toContainText('50.00');

  // Neither the refund nor the transfer produced a row of its own.
  await expect(table).not.toContainText('110.00');
  await expect(table).not.toContainText('250.00');

  // Switching the dimension changes only group_by and keeps the date range.
  await page.getByRole('button', { name: 'Payee', exact: true }).click();
  await expect(page).toHaveURL(/group_by=payee/);
  await expect(page).toHaveURL(new RegExp(`start_date=${today}`));

  // Income is a separate direction, reported as a positive magnitude.
  await page.getByRole('button', { name: 'Category', exact: true }).click();
  await page.getByRole('button', { name: 'Income', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Where money came from' })).toBeVisible();
});

async function postTransaction(
  page: Page,
  date: string,
  accountID: number,
  categoryID: number,
  commodityID: number,
  cents: number
) {
  const csrfToken = await csrfTokenFor(page);
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    status: 'posted',
    transaction_date: date,
    description: 'reports fixture',
    journal_entries: [
      {
        entry_date: date,
        postings: [
          { account_id: accountID, commodity_id: commodityID, quantity_value: String(-cents), quantity_scale: 2 },
          { account_id: categoryID, commodity_id: commodityID, quantity_value: String(cents), quantity_scale: 2 }
        ]
      }
    ]
  }, [201]);
}

async function postTransfer(
  page: Page,
  date: string,
  fromAccountID: number,
  toAccountID: number,
  commodityID: number,
  cents: number
) {
  const csrfToken = await csrfTokenFor(page);
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    status: 'posted',
    transaction_date: date,
    description: 'reports transfer fixture',
    journal_entries: [
      {
        entry_date: date,
        postings: [
          { account_id: fromAccountID, commodity_id: commodityID, quantity_value: String(-cents), quantity_scale: 2 },
          { account_id: toAccountID, commodity_id: commodityID, quantity_value: String(cents), quantity_scale: 2 }
        ]
      }
    ]
  }, [201]);
}
