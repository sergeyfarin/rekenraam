import { expect, type Page, test } from '@playwright/test';
import { expectSavedTransaction, todayISO } from './support';

test('creates a simple transaction through the browser form', async ({ page }) => {
  await ensureBrowserSession(page);
  const csrfToken = await seedTransactionPrerequisites(page);
  const accountName = `E2E Checking ${Date.now()}`;
  const description = `E2E Coffee ${Date.now()}`;

  const currencies = await apiJSON<{ currencies: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/currencies');
  const usd = currencies.currencies.find((currency) => currency.code === 'USD') ?? currencies.currencies[0];
  expect(usd).toBeTruthy();

  await apiJSON(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: accountName,
    account_class: 'asset',
    account_kind: 'checking',
    default_commodity_id: usd.id,
    allows_postings: true
  });

  await page.goto('/app/transactions');
  await page.getByRole('button', { name: 'New transaction' }).click();

  await expect(page.getByRole('heading', { name: 'Add transaction' })).toBeVisible();
  const form = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Add transaction' }) });
  await form.getByLabel('Date').fill(todayISO());
  await form.getByLabel('Payee').fill('E2E Cafe');
  await form.getByLabel('Description').fill(description);
  await form.getByLabel('Account').selectOption({ label: accountName });
  await form.getByLabel('Amount').fill('-4.50');
  await form.getByPlaceholder('Search categories').click();
  await form.locator('ul[role="listbox"] button[role="option"]').first().click();
  await form.getByRole('button', { name: 'Add transaction' }).click();

  await expectSavedTransaction(page, description);
});

async function seedTransactionPrerequisites(page: Page): Promise<string> {
  let csrfToken = await csrfTokenFor(page);
  await apiJSON(page, 'POST', '/api/v1/setup/system-accounts', csrfToken, undefined, [201, 409]);
  csrfToken = await csrfTokenFor(page);
  await apiJSON(page, 'POST', '/api/v1/setup/categories', csrfToken, undefined, [201, 409]);

  return csrfTokenFor(page);
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
  await expect(overviewHeading).toBeVisible();
}

async function csrfTokenFor(page: Page): Promise<string> {
  const session = await apiJSON<{ authenticated: boolean; csrf_token?: string }>(page, 'GET', '/api/v1/auth/session');
  if (!session.authenticated || !session.csrf_token) {
    throw new Error('expected authenticated session with CSRF token');
  }
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
      if (csrfToken) {
        headers['X-CSRF-Token'] = csrfToken;
      }
      if (body !== undefined) {
        headers['Content-Type'] = 'application/json';
      }
      const res = await fetch(path, {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body)
      });
      return { status: res.status, text: await res.text() };
    },
    { method, path, csrfToken, body }
  );

  if (!statuses.includes(response.status)) {
    throw new Error(`${method} ${path} failed with ${response.status}: ${response.text}`);
  }
  if (response.status === 204 || response.text === '') {
    return undefined as T;
  }
  return JSON.parse(response.text) as T;
}
