import { expect, type Page, test } from '@playwright/test';
import { apiJSON, csrfTokenFor } from './support/api';
import { daysFromTodayISO, todayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';
import { expectNoAccessibilityViolations } from './support/a11y';

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
  await table.getByRole('row').filter({ hasText: 'USD -25.00' }).getByRole('button').click();
  await expect(page.getByText('forecast screen fixture', { exact: true })).toBeVisible();
  await expect(page.getByText('Posted', { exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Review transactions' })).toHaveAttribute('href', '/app/transactions');
  await page.getByRole('button', { name: 'Close details' }).click();

  await page.getByLabel('Combined currency').selectOption(String(currencyID));
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`reporting_currency_id=${currencyID}`));
  await expect(page).toHaveURL(/fx_method=constant_as_of/);
  await expect(page.getByText('Combined at constant FX in USD')).toBeVisible();

  await page.reload();
  await expect(page.getByLabel('Combined currency')).toHaveValue(String(currencyID));
  await expect(page.getByRole('table')).toBeVisible();

  await page.getByRole('table').getByRole('row').filter({ hasText: 'USD -25.00' }).getByRole('button').click();
  await expect(page.getByRole('button', { name: 'Close details' })).toBeVisible();
  await postForecastTransaction(page, daysFromTodayISO(1), account.id, expense.id, currencyID, '-100', '100');
  await page.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(page.getByText('The underlying records changed. Balances were refreshed; reopen a day to inspect current details.')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Close details' })).toHaveCount(0);

  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

test('forecast details round-trip through recurring draft generation and editing', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const name = `Forecast recurring ${suffix}`;
  const account = await createCashAccount(page, csrfToken, `${name} account`, currencyID);
  const categories = await apiJSON<{ categories: Array<{ id: number; category_type: string; allows_postings: boolean }> }>(page, 'GET', '/api/v1/categories');
  const expense = categories.categories.find((item) => item.category_type === 'expense' && item.allows_postings);
  if (!expense) throw new Error('expected seeded expense category');
  const template = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/recurring/templates', csrfToken, {
    name,
    enabled: true,
    frequency: 'daily',
    starts_on: todayISO(),
    max_occurrences: 1,
    lead_days: 0,
    description: name,
    postings: [
      { account_id: account.id, commodity_id: currencyID, quantity_value: '-1234', quantity_scale: 2 },
      { account_id: expense.id, commodity_id: currencyID, quantity_value: '1234', quantity_scale: 2 }
    ]
  }, [201]);
  const forecastURL = `/app/forecast?horizon_days=30&account_id=${account.id}&include_descendants=false`;

  try {
    await page.goto(forecastURL);
    const plannedRow = page.getByRole('table').getByRole('row').filter({ hasText: 'USD -12.34' }).first();
    await plannedRow.getByRole('button').click();
    await expect(page.getByText('Recurring template', { exact: true })).toBeVisible();
    await expect(page.getByText(name, { exact: true })).toBeVisible();
    await page.getByRole('region', { name: /Movements on/ }).getByRole('link', { name: 'Review recurring entries' }).click();

    await page.getByRole('button', { name: 'Templates', exact: true }).click();
    const templateCard = page.getByRole('article', { name, exact: true });
    await templateCard.getByRole('button', { name: 'Generate due drafts' }).click();
    const dueCard = page.getByRole('article', { name, exact: true });
    await dueCard.getByRole('button', { name: 'Edit draft' }).click();
    const edit = page.getByRole('dialog');
    await edit.getByLabel('Amount', { exact: true }).nth(0).fill('-15.67');
    await edit.getByLabel('Amount', { exact: true }).nth(1).fill('15.67');
    await edit.getByRole('button', { name: 'Save transaction', exact: true }).click();
    await expect(edit).not.toBeVisible();

    await page.goto(forecastURL);
    const changedRow = page.getByRole('table').getByRole('row').filter({ hasText: 'USD -15.67' }).first();
    await changedRow.getByRole('button').click();
    const changedDetails = page.getByRole('region', { name: /Movements on/ });
    await expect(changedDetails.getByText('Saved draft', { exact: true })).toBeVisible();
    await expect(changedDetails.getByText('USD -15.67', { exact: true })).toHaveCount(1);
    await expect(changedDetails.getByText(name, { exact: true })).toHaveCount(1);
  } finally {
    await apiJSON(page, 'POST', `/api/v1/recurring/templates/${template.id}/archive`, await csrfTokenFor(page), {});
  }
});

test('stale event cursor clears old details before refreshing balances', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const account = await createCashAccount(page, csrfToken, `Forecast stale ${Date.now()}`, currencyID);
  const categories = await apiJSON<{ categories: Array<{ id: number; code?: string; allows_postings: boolean }> }>(page, 'GET', '/api/v1/categories');
  const expense = categories.categories.find((item) => item.code === 'expense_food_groceries' && item.allows_postings);
  if (!expense) throw new Error('expected seeded expense category');
  const eventDate = daysFromTodayISO(1);
  await postForecastTransaction(page, eventDate, account.id, expense.id, currencyID, '-1000', '1000');

  await page.route('**/api/v1/forecasts/balance-events*', async (route) => {
    const url = new URL(route.request().url());
    if (url.searchParams.has('cursor')) {
      await route.fulfill({ status: 409, contentType: 'application/json', body: JSON.stringify({ error: { code: 'FORECAST_BASIS_CHANGED', message: 'changed' } }) });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        basis_token: url.searchParams.get('basis_token'),
        date: eventDate,
        total_count: 2,
        next_cursor: 'stale-cursor',
        items: [{
          key: 'mock-old-event', source: 'posted', source_date: eventDate, projected_date: eventDate,
          carried_forward: false, transaction_id: 1, version_id: 1, entry_id: 1,
          template_id: null, occurrence_id: null, occurrence_date: null,
          description: 'old event that must be cleared', payee_name: null,
          amounts: [{ account_id: account.id, commodity_id: currencyID, commodity_code: 'USD', quantity_value: '-1000', quantity_scale: 2 }]
        }]
      })
    });
  });

  await page.goto(`/app/forecast?horizon_days=30&account_id=${account.id}&include_descendants=false`);
  await page.getByRole('table').getByRole('row').filter({ hasText: 'USD -10.00' }).first().getByRole('button').click();
  await expect(page.getByText('old event that must be cleared', { exact: true })).toBeVisible();
  const loadMore = page.getByRole('button', { name: 'Load more movements' });
  let releaseRefresh!: () => void;
  const refreshResponse = new Promise<void>((resolve) => { releaseRefresh = resolve; });
  await page.route('**/api/v1/forecasts/balances*', async (route) => { await refreshResponse; await route.continue(); });
  await page.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(loadMore).toBeDisabled();
  releaseRefresh();
  await expect(page.getByRole('button', { name: 'Refresh', exact: true })).toBeEnabled();
  await page.unroute('**/api/v1/forecasts/balances*');
  await loadMore.click();
  await expect(page.getByText('The underlying records changed. Balances were refreshed; reopen a day to inspect current details.')).toBeVisible();
  await expect(page.getByText('old event that must be cleared', { exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Load more movements' })).toHaveCount(0);
});

test('forecast event details expose keyboard loading, empty and recoverable error states', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const account = await createCashAccount(page, csrfToken, `Forecast states ${Date.now()}`, currencyID);
  let releaseFirst!: () => void;
  const firstResponse = new Promise<void>((resolve) => { releaseFirst = resolve; });
  let requestCount = 0;
  await page.route('**/api/v1/forecasts/balance-events*', async (route) => {
    requestCount++;
    const url = new URL(route.request().url());
    if (requestCount === 1) {
      await firstResponse;
    } else if (requestCount <= 3) {
      await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: { code: 'RESOURCE_BUSY', message: 'busy' } }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ basis_token: url.searchParams.get('basis_token'), date: url.searchParams.get('date'), total_count: 0, next_cursor: null, items: [] }) });
  });

  await page.goto(`/app/forecast?horizon_days=30&account_id=${account.id}&include_descendants=false`);
  const dayButtons = page.getByRole('table').getByRole('button');
  const firstDay = dayButtons.nth(0);
  await firstDay.focus();
  await page.keyboard.press('Enter');
  await expect(firstDay).toBeFocused();
  await expect(page.getByText('Loading movement details…', { exact: true })).toBeVisible();
  releaseFirst();
  await expect(page.getByText('No included event affects this day and selected series.', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Close details' }).click();

  await dayButtons.nth(1).click();
  await expect(page.getByRole('button', { name: 'Retry details' })).toBeVisible();
  await page.getByRole('button', { name: 'Retry details' }).click();
  await expect(page.getByText('No included event affects this day and selected series.', { exact: true })).toBeVisible();

  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole('button', { name: 'Switch to dark theme' }).click();
  await expectNoAccessibilityViolations(page, 'forecast event details');
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
