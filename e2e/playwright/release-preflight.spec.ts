import { expect, type Page, test } from '@playwright/test';

test.describe.serial('release preflight journeys', () => {
  test('enters a balanced split transfer', async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const suffix = Date.now();
    const checking = await createCashAccount(page, csrfToken, `Preflight checking ${suffix}`, currencyID);
    const savings = await createCashAccount(page, csrfToken, `Preflight savings ${suffix}`, currencyID);

    await page.goto('/app/transactions');
    await page.getByRole('button', { name: 'New transaction' }).click();
    const form = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Add transaction' }) });
    await form.getByLabel('Date').fill('2026-07-08');
    await form.getByLabel('Description').fill(`Preflight transfer ${suffix}`);
    await form.getByRole('button', { name: 'Use split entry' }).click();

    const accounts = form.getByLabel('Account');
    const amounts = form.getByLabel('Amount');
    await accounts.nth(0).selectOption({ label: checking.name });
    await amounts.nth(0).fill('-25.00');
    await accounts.nth(1).selectOption({ label: savings.name });
    await amounts.nth(1).fill('25.00');
    await expect(form.getByText('Balanced')).toBeVisible();
    await form.getByRole('button', { name: 'Add transaction' }).click();

    await expect(page.getByText(`Preflight transfer ${suffix}`)).toBeVisible();
  });

  test('reconciles a selected posting to zero', async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const suffix = Date.now();
    const account = await createCashAccount(page, csrfToken, `Preflight reconcile ${suffix}`, currencyID);
    await createSimpleTransaction(page, csrfToken, account.id, currencyID, `Preflight reconcile entry ${suffix}`, '10.00');

    await page.goto('/app/reconcile');
    await page.getByLabel('Account').selectOption({ label: account.name });
    await page.getByLabel('Statement date').fill('2026-07-08');
    await page.getByLabel('Statement ending balance').fill('10.00');
    await page.getByRole('button', { name: 'Start reconciliation' }).click();

    await page.getByRole('checkbox').first().check();
    await expect(page.getByText('Balanced')).toBeVisible();
    await page.getByLabel('Reason').fill('Preflight zero reconciliation');
    await page.getByRole('button', { name: 'Finish reconciliation' }).click();
    await expect(page.getByText('Reconciliation complete')).toBeVisible();
  });

  test('previews and commits a QIF import', async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const suffix = Date.now();
    const account = await createCashAccount(page, csrfToken, `Preflight import ${suffix}`, currencyID);

    await page.goto('/app/import');
    await page.locator('input[type="file"]').setInputFiles({
      name: 'preflight.qif',
      mimeType: 'application/qif',
      buffer: Buffer.from('!Type:Bank\nD07/08/2026\nT-12.34\nPPreflight QIF Payee\nMPreflight QIF memo\n^\n')
    });
    await page.getByRole('button', { name: 'Upload & preview' }).click();
    await expect(page.getByText('Preview import')).toBeVisible();
    await page.getByLabel('Target account').selectOption(String(account.id));
    await page.getByLabel('Currency').selectOption(String(currencyID));
    await page.getByRole('button', { name: 'Apply to all rows' }).click();
    await page.getByRole('button', { name: 'Commit to ledger' }).click();
    await expect(page.getByText('Import complete')).toBeVisible();
    await expect(page.getByText('1 transaction committed.')).toBeVisible();
  });

  test('records a buy, previews a sell, and commits it', async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const suffix = Date.now();
    const cash = await createCashAccount(page, csrfToken, `Preflight brokerage cash ${suffix}`, currencyID);
    const instrument = await apiJSON<{ id: number; display_name: string }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
      commodity_code: `PF${suffix}`,
      instrument_type: 'equity',
      display_name: `Preflight Equity ${suffix}`,
      symbol: `PF${suffix}`,
      quote_commodity_id: currencyID,
      trading_commodity_id: currencyID,
      quantity_scale: 3,
      price_scale: 2
    });
    const holding = await apiJSON<{ id: number; name: string }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
      instrument_id: instrument.id,
      name: `Preflight holding ${suffix}`,
      opened_on: '2026-07-01'
    });

    await page.goto('/app/investments');
    await page.getByRole('button', { name: 'Record buy' }).click();
    await page.getByLabel('Instrument').fill(`Preflight Equity ${suffix}`);
    await page.getByRole('option').filter({ hasText: `Preflight Equity ${suffix}` }).click();
    await page.getByLabel('Holding account').selectOption(String(holding.id));
    await page.getByLabel('Quantity').fill('2.000');
    await page.getByLabel('Cash account').selectOption(String(cash.id));
    await page.getByLabel('Total cost').fill('20.00');
    await page.getByRole('button', { name: 'Record buy' }).last().click();
    await expect(page.getByText(`Preflight Equity ${suffix}`)).toBeVisible();

    await page.getByRole('button', { name: 'Record sell' }).click();
    await page.getByLabel('Instrument').fill(`Preflight Equity ${suffix}`);
    await page.getByRole('option').filter({ hasText: `Preflight Equity ${suffix}` }).click();
    await page.getByLabel('Holding account').selectOption(String(holding.id));
    await page.getByLabel('Quantity').fill('1.000');
    await page.getByLabel('Cash account').selectOption(String(cash.id));
    await page.getByLabel('Proceeds').fill('12.00');
    await expect(page.getByText('Sell preview')).toBeVisible();
    await page.getByRole('button', { name: 'Confirm sell' }).click();
    await expect(page.getByText(`Preflight Equity ${suffix}`)).toBeVisible();
  });

  test('supports transaction entry at a mobile viewport', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    const { csrfToken, currencyID } = await readyForLedger(page);
    const suffix = Date.now();
    const account = await createCashAccount(page, csrfToken, `Preflight mobile ${suffix}`, currencyID);

    await page.goto('/app/transactions');
    await page.getByRole('button', { name: 'New transaction' }).click();
    const form = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Add transaction' }) });
    await form.getByLabel('Date').fill('2026-07-08');
    await form.getByLabel('Description').fill(`Preflight mobile entry ${suffix}`);
    await form.getByLabel('Account').selectOption(String(account.id));
    await form.getByLabel('Amount').fill('-3.50');
    await form.getByPlaceholder('Search categories').click();
    await form.locator('ul[role="listbox"] button[role="option"]').first().click();
    await form.getByRole('button', { name: 'Add transaction' }).click();
    await expect(page.getByText(`Preflight mobile entry ${suffix}`)).toBeVisible();
  });
});

async function readyForLedger(page: Page): Promise<{ csrfToken: string; currencyID: number }> {
  await ensureBrowserSession(page);
  let csrfToken = await csrfTokenFor(page);
  await apiJSON(page, 'POST', '/api/v1/setup/system-accounts', csrfToken, undefined, [201, 409]);
  csrfToken = await csrfTokenFor(page);
  await apiJSON(page, 'POST', '/api/v1/setup/categories', csrfToken, undefined, [201, 409]);
  const currencies = await apiJSON<{ currencies: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/currencies');
  const currency = currencies.currencies.find((item) => item.code === 'USD') ?? currencies.currencies[0];
  if (!currency) throw new Error('expected a configured currency');
  return { csrfToken: await csrfTokenFor(page), currencyID: currency.id };
}

async function createCashAccount(page: Page, csrfToken: string, name: string, currencyID: number): Promise<{ id: number; name: string }> {
  return apiJSON(page, 'POST', '/api/v1/accounts', csrfToken, {
    name,
    account_class: 'asset',
    account_kind: 'checking',
    default_commodity_id: currencyID,
    allows_postings: true
  });
}

async function createSimpleTransaction(page: Page, csrfToken: string, accountID: number, currencyID: number, description: string, amount: string) {
  await page.goto('/app/transactions');
  await page.getByRole('button', { name: 'New transaction' }).click();
  const form = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Add transaction' }) });
  await form.getByLabel('Date').fill('2026-07-08');
  await form.getByLabel('Description').fill(description);
  await form.getByLabel('Account').selectOption(String(accountID));
  await form.getByLabel('Amount').fill(amount);
  await form.getByPlaceholder('Search categories').click();
  await form.locator('ul[role="listbox"] button[role="option"]').first().click();
  await form.getByRole('button', { name: 'Add transaction' }).click();
  await expect(page.getByText(description)).toBeVisible();
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
  return response.status === 204 || response.text === '' ? undefined as T : JSON.parse(response.text) as T;
}
