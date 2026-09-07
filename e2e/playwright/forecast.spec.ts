import { expect, type Page, test } from '@playwright/test';
import { apiJSON, csrfTokenFor } from './support/api';
import { daysFromTodayISO, todayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';

test('forecast screen shows exact recorded and future balances from one selected scope', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `Forecast checking ${suffix}`, currencyID);
  const categories = await apiJSON<{ categories: Array<{ id: number; code?: string; allows_postings: boolean }> }>(page, 'GET', '/api/v1/categories');
  const expense = categories.categories.find((item) => item.code === 'expense_food_groceries' && item.allows_postings);
  const income = categories.categories.find((item) => item.code === 'income_salary_wages' && item.allows_postings);
  if (!expense || !income) throw new Error('expected seeded forecast counterpart categories');

  await postForecastTransaction(page, todayISO(), account.id, income.id, currencyID, '10000', '-10000');
  await postForecastTransaction(page, daysFromTodayISO(1), account.id, expense.id, currencyID, '-2500', '2500');

  await page.goto(`/app/forecast?horizon_days=30&account_id=${account.id}&include_descendants=false`);
  await expect(page.getByRole('heading', { name: 'Projected balances' })).toBeVisible();
  await expect(page.getByRole('navigation').getByRole('link', { name: 'Forecast' })).toHaveAttribute('aria-current', 'page');

  const summary = page.getByRole('heading', { name: `${account.name} · USD` }).locator('..');
  await expect(summary).toContainText('USD 100.00');
  await expect(summary).toContainText('USD 75.00');
  const table = page.getByRole('table');
  await expect(table).toBeVisible();
  await expect(table).toContainText('USD -25.00');

  await page.getByLabel('Combined currency').selectOption(String(currencyID));
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`reporting_currency_id=${currencyID}`));
  await expect(page).toHaveURL(/fx_method=constant_as_of/);
  await expect(page.getByText('Combined at constant FX in USD')).toBeVisible();

  await page.reload();
  await expect(page.getByLabel('Combined currency')).toHaveValue(String(currencyID));
  await expect(page.getByRole('table')).toBeVisible();

  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

async function postForecastTransaction(
  page: Page,
  date: string,
  accountID: number,
  counterpartID: number,
  commodityID: number,
  accountValue: string,
  counterpartValue: string
) {
  await apiJSON(page, 'POST', '/api/v1/transactions', await csrfTokenFor(page), {
    status: 'posted',
    transaction_date: date,
    description: 'forecast screen fixture',
    journal_entries: [{
      entry_date: date,
      postings: [
        { account_id: accountID, commodity_id: commodityID, quantity_value: accountValue, quantity_scale: 2 },
        { account_id: counterpartID, commodity_id: commodityID, quantity_value: counterpartValue, quantity_scale: 2 }
      ]
    }]
  }, [201]);
}
