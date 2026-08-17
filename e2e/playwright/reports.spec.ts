import { expect, type Page, test } from '@playwright/test';
import { todayISO } from './support/dates';

test('spending report ranks categories, nets refunds, and ignores transfers', async ({ page }) => {
  const csrfToken = await readyForLedger(page);
  const suffix = Date.now();
  const today = todayISO();

  const currencies = await apiJSON<{ currencies: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/currencies');
  const usd = currencies.currencies.find((currency) => currency.code === 'USD') ?? currencies.currencies[0];

  const checking = await createCashAccount(page, csrfToken, `Reports checking ${suffix}`, usd.id);
  const savings = await createCashAccount(page, csrfToken, `Reports savings ${suffix}`, usd.id);

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
  await postTransaction(page, today, checking.id, groceries, usd.id, 8000);
  await postTransaction(page, today, checking.id, groceries, usd.id, -3000);
  // 60.00 fuel, so groceries (50.00) must rank below it.
  await postTransaction(page, today, checking.id, fuel, usd.id, 6000);
  // A transfer between the user's own accounts has no category posting and must
  // never appear as spending.
  await postTransfer(page, today, checking.id, savings.id, usd.id, 25000);

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

async function readyForLedger(page: Page): Promise<string> {
  await ensureBrowserSession(page);
  let csrfToken = await csrfTokenFor(page);
  await apiJSON(page, 'POST', '/api/v1/setup/system-accounts', csrfToken, undefined, [201, 409]);
  csrfToken = await csrfTokenFor(page);
  await apiJSON(page, 'POST', '/api/v1/setup/categories', csrfToken, undefined, [201, 409]);
  return csrfTokenFor(page);
}

async function createCashAccount(
  page: Page,
  csrfToken: string,
  name: string,
  currencyID: number
): Promise<{ id: number; name: string }> {
  return apiJSON(page, 'POST', '/api/v1/accounts', csrfToken, {
    name,
    account_class: 'asset',
    account_kind: 'checking',
    default_commodity_id: currencyID,
    allows_postings: true
  });
}

async function ensureBrowserSession(page: Page) {
  await page.goto('/');
  const freshHeading = page.getByRole('heading', { name: 'Create the owner account' });
  const loginHeading = page.getByRole('heading', { name: 'Sign in to continue' });
  const currencyHeading = page.getByRole('heading', { name: 'Choose default currencies' });
  const overviewHeading = page.getByRole('heading', { name: 'Overview' });
  await expect(freshHeading.or(loginHeading).or(currencyHeading).or(overviewHeading)).toBeVisible({ timeout: 10_000 });
  if (await freshHeading.isVisible()) {
    await page.getByLabel('Username').fill('owner');
    await page.getByLabel('Password').fill('test-password');
    await page.getByRole('button', { name: 'Create owner' }).click();
  } else if (await loginHeading.isVisible()) {
    await page.getByLabel('Username').fill('owner');
    await page.getByLabel('Password').fill('test-password');
    await page.getByRole('button', { name: 'Sign in' }).click();
  }
  await expect(currencyHeading.or(overviewHeading)).toBeVisible({ timeout: 10_000 });
  if (await currencyHeading.isVisible()) {
    await page.getByRole('button', { name: /USD/ }).first().click();
    await page.getByRole('button', { name: 'Save currencies' }).click();
  }
  await expect(page).toHaveURL(/\/app$/);
}

async function csrfTokenFor(page: Page): Promise<string> {
  const session = await apiJSON<{ authenticated: boolean; csrf_token?: string }>(page, 'GET', '/api/v1/auth/session');
  if (!session.authenticated || !session.csrf_token) throw new Error('expected authenticated session with CSRF token');
  return session.csrf_token;
}

async function apiJSON<T = unknown>(
  page: Page,
  method: 'GET' | 'POST',
  path: string,
  csrfTokenOrExpected?: string | number[],
  body?: unknown,
  expectedStatuses: number[] = method === 'POST' ? [200, 201] : [200]
): Promise<T> {
  const csrfToken = typeof csrfTokenOrExpected === 'string' ? csrfTokenOrExpected : undefined;
  const statuses = Array.isArray(csrfTokenOrExpected) ? csrfTokenOrExpected : expectedStatuses;
  const response = await page.evaluate(
    async ({ method, path, csrfToken, body }) => {
      const headers: Record<string, string> = {};
      if (csrfToken) headers['X-CSRF-Token'] = csrfToken;
      if (body !== undefined) headers['Content-Type'] = 'application/json';
      const res = await fetch(path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
      return { status: res.status, text: await res.text() };
    },
    { method, path, csrfToken, body }
  );
  if (!statuses.includes(response.status)) throw new Error(`${method} ${path} failed with ${response.status}: ${response.text}`);
  return response.status === 204 || response.text === '' ? (undefined as T) : (JSON.parse(response.text) as T);
}
