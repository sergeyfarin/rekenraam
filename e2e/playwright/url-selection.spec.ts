import { expect, test, type Page } from './support/fixtures';
import { apiJSON, csrfTokenFor } from './support/api';
import { readyForLedger, ensureCurrency } from './support/ledger';
import { seedReferenceBook } from './support/reference-book';
import { goHomeInApp } from './support/session';

// #326: run against the production binary, including a fresh document for
// reload/shared links and real browser history for Back. Filter tweaks must
// replace the current history entry rather than making extra Back stops.
async function revisit(page: Page, assertSelection: (page: Page) => Promise<void>, destination = '/app/ledger/transactions') {
  const selectedURL = page.url();
  // Exercise SvelteKit's in-app navigation and popstate restoration too.
  await goHomeInApp(page);
  await expect(page).toHaveURL((url) => url.pathname === '/app');
  await page.goBack();
  await expect(page).toHaveURL(selectedURL);
  await assertSelection(page);
  await page.reload();
  await assertSelection(page);
  await page.goto(destination);
  if (destination.includes('transaction_id=')) await expect(page.getByRole('button', { name: 'Close', exact: true }).first()).toBeVisible();
  await page.goBack();
  await expect(page).toHaveURL(selectedURL);
  await assertSelection(page);
  const shared = await page.context().newPage();
  try {
    await shared.goto(selectedURL);
    await assertSelection(shared);
  } finally {
    await shared.close();
  }
}

for (const width of [1280, 390]) {
  test.describe(`URL selections at ${width}px`, () => {
    test.use({ viewport: { width, height: 900 } });

    test('budget month and currency survive reload, Back and sharing', async ({ page }) => {
      const { csrfToken } = await readyForLedger(page);
      const eur = await ensureCurrency(page, csrfToken, 'EUR', 'Euro');
      const usd = await ensureCurrency(page, await csrfTokenFor(page), 'USD', 'US Dollar');
      await page.goto('/app/planning/budgets?highlight=kept');
      await expect(page.getByRole('combobox', { name: 'Currency', exact: true }).getByRole('option', { name: 'USD', exact: true })).toHaveCount(1);
      const historyLength = await page.evaluate(() => history.length);
      await page.getByLabel('Month', { exact: true }).fill('2026-01');
      await page.getByRole('combobox', { name: 'Currency', exact: true }).selectOption(String(eur));
      await page.getByRole('combobox', { name: 'Currency', exact: true }).selectOption(String(usd));
      const assertSelection = async (target: Page) => {
        await expect(target.getByLabel('Month', { exact: true })).toHaveValue('2026-01');
        await expect(target.getByRole('combobox', { name: 'Currency', exact: true })).toHaveValue(String(usd));
        await expect(target).toHaveURL((url) => url.searchParams.get('month') === '2026-01'
          && url.searchParams.get('currency_id') === String(usd) && url.searchParams.get('highlight') === 'kept');
      };
      await assertSelection(page);
      expect(await page.evaluate(() => history.length)).toBe(historyLength);
      await revisit(page, assertSelection);
    });

    test('recurring tab survives reload, Back and sharing and Due clears it', async ({ page }) => {
      await readyForLedger(page);
      await page.goto('/app/planning/recurring?highlight=kept');
      await expect(page.getByRole('button', { name: 'Due for review', exact: true })).toHaveAttribute('aria-pressed', 'true');
      const historyLength = await page.evaluate(() => history.length);
      await page.getByRole('button', { name: 'Templates', exact: true }).click();
      const assertSelection = async (target: Page) => {
        await expect(target.getByRole('button', { name: 'Templates', exact: true })).toHaveAttribute('aria-pressed', 'true');
        await expect(target).toHaveURL((url) => url.searchParams.get('tab') === 'templates' && url.searchParams.get('highlight') === 'kept');
      };
      await assertSelection(page);
      expect(await page.evaluate(() => history.length)).toBe(historyLength);
      await revisit(page, assertSelection);
      await page.getByRole('button', { name: 'Due for review', exact: true }).click();
      await expect(page).toHaveURL((url) => !url.searchParams.has('tab'));
      await page.reload();
      await expect(page.getByRole('button', { name: 'Due for review', exact: true })).toHaveAttribute('aria-pressed', 'true');
    });

    for (const screen of ['register', 'category', 'instrument'] as const) {
      test(`${screen} filters survive reload, linked transaction Back and sharing`, async ({ page }) => {
        const { ids, suffix } = await seedReferenceBook(page);
        const instrument = screen === 'instrument';
        const description = instrument ? `URL dividend ${suffix}` : 'Duplicate groceries';
        if (instrument) {
          await apiJSON(page, 'POST', '/api/v1/investments/dividend', await csrfTokenFor(page), {
            transaction_date: '2026-02-21', commodity_id: ids.get('REF'), cash_account_id: ids.get('usd_checking'),
            cash_commodity_id: ids.get('USD'), income_account_id: ids.get('dividends'),
            amount_value: '100', amount_scale: 2, memo: description
          });
        }
        const path = screen === 'register' ? `/app/ledger/accounts/${ids.get('checking')}/register`
          : screen === 'category' ? `/app/ledger/categories/${ids.get('groceries')}`
          : `/app/investments/instruments/${ids.get('REF')}`;
        const requests: URL[] = [];
        const endpoint = screen === 'register' ? `/api/v1/accounts/${ids.get('checking')}/register` : '/api/v1/transactions';
        page.on('request', (request) => {
          const url = new URL(request.url());
          if (request.method() === 'GET' && url.pathname === endpoint) requests.push(url);
        });
        // Unrelated query scope must never replace the route's scope.
        await page.goto(`${path}?highlight=kept&account_id=999999&category_id=999999&commodity_id=999999`);
        await expect(page.getByRole('combobox', { name: 'Status', exact: true })).toBeVisible();
        const historyLength = await page.evaluate(() => history.length);
        const status = instrument ? 'posted' : 'voided';
        const from = instrument ? '2026-02-01' : '2026-01-01';
        const to = instrument ? '2026-02-28' : '2026-01-31';
        await page.getByRole('combobox', { name: 'Status', exact: true }).selectOption(status);
        await page.getByLabel('From', { exact: true }).fill(from);
        await page.getByLabel('To', { exact: true }).fill(to);
        if (screen !== 'register') {
          await page.getByRole('searchbox', { name: 'Search', exact: true }).fill('no-match');
          await page.getByRole('searchbox', { name: 'Search', exact: true }).fill(description);
        }
        const assertSelection = async (target: Page) => {
          await expect(target.getByRole('combobox', { name: 'Status', exact: true })).toHaveValue(status);
          await expect(target.getByLabel('From', { exact: true })).toHaveValue(from);
          await expect(target.getByLabel('To', { exact: true })).toHaveValue(to);
          if (screen !== 'register') await expect(target.getByRole('searchbox', { name: 'Search', exact: true })).toHaveValue(description);
          await expect(target.getByRole('row').filter({ hasText: description })).toHaveCount(1);
          expect(new URL(target.url()).searchParams.get('highlight')).toBe('kept');
        };
        await assertSelection(page);
        expect(await page.evaluate(() => history.length)).toBe(historyLength);
        await expect.poll(() => requests.at(-1)?.searchParams.get('before_date')).toBe(to);
        const query = requests.at(-1)!.searchParams;
        expect(query.get('status')).toBe(status);
        expect(query.get('after_date')).toBe(from);
        if (screen === 'category') expect(query.get('category_id')).toBe(String(ids.get('groceries')));
        if (instrument) expect(query.get('commodity_id')).toBe(String(ids.get('REF')));
        expect(query.get('account_id')).not.toBe('999999');
        expect(query.get('category_id')).not.toBe('999999');
        const transactions = await apiJSON<{ transactions: Array<{ id: number }> }>(page, 'GET', `/api/v1/transactions?${query}`);
        const linkedID = screen === 'register' ? ids.get('t13') : transactions.transactions[0]?.id;
        expect(linkedID).toBeTruthy();
        await revisit(page, assertSelection, `/app/ledger/transactions?transaction_id=${linkedID}`);

        await page.getByLabel('From', { exact: true }).fill('');
        await page.getByLabel('To', { exact: true }).fill('');
        if (screen !== 'register') {
          await page.getByRole('searchbox', { name: 'Search', exact: true }).fill('');
          await page.getByRole('combobox', { name: 'Status', exact: true }).selectOption('');
        }
        await expect(page).toHaveURL((url) => !url.searchParams.has('after_date') && !url.searchParams.has('before_date')
          && !url.searchParams.has('q') && (screen === 'register' || !url.searchParams.has('status')));
        await page.reload();
        await expect(page.getByLabel('From', { exact: true })).toHaveValue('');
        await expect(page.getByLabel('To', { exact: true })).toHaveValue('');
      });
    }
  });
}

test('budget saves refresh the displayed target for both default and URL-selected months', async ({ page }) => {
  const { csrfToken } = await readyForLedger(page);
  const name = `URL budget target ${Date.now()}`;
  await apiJSON(page, 'POST', '/api/v1/categories', csrfToken, { name, category_type: 'expense' });
  await page.goto('/app/planning/budgets');
  const form = page.locator('form').filter({ has: page.getByLabel(`Target for ${name}`, { exact: true }) });
  await form.getByLabel(`Target for ${name}`, { exact: true }).fill('12');
  await form.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(form.locator('p').last()).toContainText('12');
  await expect(page).toHaveURL((url) => !url.searchParams.has('month'));
  await page.getByLabel('Month', { exact: true }).fill('2020-01');
  await expect(form.locator('p').last()).not.toContainText('12');
  await form.getByLabel(`Target for ${name}`, { exact: true }).fill('24');
  await form.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(form.locator('p').last()).toContainText('24');
  await page.reload();
  await expect(form.locator('p').last()).toContainText('24');
});

test('malformed shared selections fall back without sending impossible dates to the API (#377)', async ({ page }) => {
  const { ids } = await seedReferenceBook(page);
  const failures: string[] = [];
  page.on('response', (response) => {
    if (response.url().includes('/api/v1/') && response.status() >= 400) failures.push(response.url());
  });
  for (const path of [
    '/app/ledger/transactions', `/app/ledger/accounts/${ids.get('checking')}/register`,
    `/app/ledger/categories/${ids.get('groceries')}`, `/app/investments/instruments/${ids.get('REF')}`
  ]) {
    await page.goto(`${path}?after_date=2026-02-31&before_date=2026-13-01&status=invalid`);
    await expect(page.getByLabel('From', { exact: true })).toHaveValue('');
    await expect(page.getByLabel('To', { exact: true })).toHaveValue('');
    await expect(page.getByRole('row').nth(1)).toBeVisible();
  }
  await page.goto('/app/planning/budgets?month=2026-13&currency_id=-1');
  await expect(page.getByLabel('Month', { exact: true })).toHaveValue(/^\d{4}-(0[1-9]|1[0-2])$/);
  await expect(page.getByRole('combobox', { name: 'Currency', exact: true })).not.toHaveValue('0');
  await page.goto('/app/planning/recurring?tab=unknown');
  await expect(page.getByRole('button', { name: 'Due for review', exact: true })).toHaveAttribute('aria-pressed', 'true');
  expect(failures).toEqual([]);
});
