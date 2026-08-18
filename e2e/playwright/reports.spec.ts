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

test('report filters narrow the result and travel in the URL', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const today = todayISO();

  const checking = await createCashAccount(page, csrfToken, `Filter checking ${suffix}`, currencyID);
  const savings = await createCashAccount(page, csrfToken, `Filter savings ${suffix}`, currencyID);

  const categories = await apiJSON<{
    categories: Array<{ id: number; code?: string; allows_postings: boolean }>;
  }>(page, 'GET', '/api/v1/categories');
  const category = (code: string) => {
    const found = categories.categories.find((item) => item.code === code && item.allows_postings);
    if (!found) throw new Error(`seeded category ${code} not found`);
    return found.id;
  };

  // Checking spends 110.00 across two categories; savings spends 90.00. Every
  // assertion below can tell a filtered total from an unfiltered one.
  await postTransaction(page, today, checking.id, category('expense_food_groceries'), currencyID, 4000);
  await postTransaction(page, today, checking.id, category('expense_transport_fuel'), currencyID, 7000);
  await postTransaction(page, today, savings.id, category('expense_housing_rent_mortgage'), currencyID, 9000);

  await page.goto(`/app/reports?view=spending&group_by=category&start_date=${today}&end_date=${today}&bucket=month`);

  const table = page.getByRole('table');
  await expect(table.locator('tbody tr')).toHaveCount(3);

  // Selecting one category narrows the report and writes the dimension into the
  // URL, so the link reproduces exactly this result.
  await openFilter(page, 'Categories');
  await page.getByRole('checkbox', { name: 'Groceries', exact: true }).check();

  await expect(page).toHaveURL(new RegExp(`category_id=${category('expense_food_groceries')}`));
  await expect(table.locator('tbody tr')).toHaveCount(1);
  await expect(table).toContainText('Groceries');
  await expect(table).not.toContainText('Fuel');

  // The active selection is named on the page, so a shared link explains its
  // own scope without opening a control.
  await expect(page.getByRole('region', { name: 'Filters' })).toContainText('Groceries');

  // A reload proves the URL alone carries the filter — nothing lives in memory.
  await page.reload();
  await expect(page.getByRole('table').locator('tbody tr')).toHaveCount(1);

  // Net worth cannot express a category, so that control is not offered there;
  // the selection survives in the URL for the trip back.
  await page.getByRole('button', { name: 'Net worth', exact: true }).click();
  await expect(page.getByRole('region', { name: 'Filters' })).not.toContainText('Categories');
  await expect(page).toHaveURL(/category_id=/);

  await page.getByRole('button', { name: 'Spending', exact: true }).click();
  await expect(page.getByRole('table').locator('tbody tr')).toHaveCount(1);

  // Clearing drops the dimension from the URL rather than writing it empty.
  await page.getByRole('button', { name: 'Clear all filters' }).click();
  await expect(page).not.toHaveURL(/category_id=/);
  await expect(page.getByRole('table').locator('tbody tr')).toHaveCount(3);

  // The account dimension applies to both reports, and its descendant toggle
  // only appears once there is a selection to expand.
  await expect(page.getByRole('checkbox', { name: 'Include sub-accounts of the selected accounts' })).toHaveCount(0);
  await openFilter(page, 'Accounts');
  await page.getByRole('checkbox', { name: `Filter checking ${suffix}`, exact: true }).check();
  await expect(page).toHaveURL(new RegExp(`account_id=${checking.id}`));
  await page.getByRole('checkbox', { name: 'Include sub-accounts of the selected accounts' }).check();
  await expect(page).toHaveURL(/include_descendants=true/);

  // The account filter must reach the net-worth calculation, not just the URL:
  // checking alone is -110.00 where both accounts together are -200.00.
  await page.getByRole('button', { name: 'Net worth', exact: true }).click();
  const netWorth = page.getByRole('table');
  await expect(netWorth).toContainText('-110.00');
  await expect(netWorth).not.toContainText('-200.00');
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

/** Opens one filter disclosure by its visible label. */
async function openFilter(page: Page, label: string) {
  const disclosure = page.locator('details').filter({ has: page.getByText(label, { exact: true }) });
  await disclosure.locator('summary').click();
}
