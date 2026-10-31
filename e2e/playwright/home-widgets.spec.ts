import { test, expect } from './support/owned-book';
import { apiJSON, csrfTokenFor } from './support/api';
import { createCashAccount, ensureCurrency, readyForLedger } from './support/ledger';
import { expectNoAccessibilityViolations } from './support/a11y';

// The default My Home widgets (#478). An owned book, because every widget is a
// book-wide summary.

type Category = { id: number; name?: string; builtin_key?: string; category_type: string };

test('My Home widgets come from the one Home read, drill down with filters, and Back restores the scroll', async ({ page }) => {
  const { currencyID } = await readyForLedger(page);
  let csrfToken = await csrfTokenFor(page);
  await apiJSON(page, 'POST', `/api/v1/currencies/${currencyID}/default`, csrfToken, undefined, [200]);
  csrfToken = await csrfTokenFor(page);
  const account = await createCashAccount(page, csrfToken, 'Widget checking', currencyID);
  const salary = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name: 'Widget salary', category_type: 'income' });
  // A starter category has no name of its own: Home labels it by its built-in key.
  const { categories } = await apiJSON<{ categories: Category[] }>(page, 'GET', '/api/v1/categories');
  const groceries = categories.find(category => category.builtin_key === 'expense.food.groceries');
  if (!groceries) throw new Error('expected the starter Groceries category');
  const { owner_today: today } = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
  const post = (categoryID: number, cents: number) => apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    status: 'posted', transaction_date: today, journal_entries: [{ entry_date: today, postings: [
      { account_id: account.id, commodity_id: currencyID, quantity_value: String(-cents), quantity_scale: 2 },
      { account_id: categoryID, commodity_id: currencyID, quantity_value: String(cents), quantity_scale: 2 }
    ] }]
  });
  await post(salary.id, -250000);
  await post(groceries.id, 4560);

  const reads: string[] = [];
  page.on('request', request => {
    const path = new URL(request.url()).pathname;
    if (path.startsWith('/api/v1/')) reads.push(path);
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/app');

  const spending = page.getByTestId('home-widget-spending');
  await expect(spending).toHaveAttribute('data-state', 'ready');
  // 45.60 is under 100, so Home keeps its minor units.
  await expect(spending.getByText(/45\.60/).first()).toBeVisible();
  const row = spending.getByRole('link', { name: 'Groceries' });
  await expect(row).toHaveAttribute('href', new RegExp(`category_id=${groceries.id}`));
  await expect(spending.getByRole('link', { name: /Open spending report/ })).toHaveAttribute('href', new RegExp(`start_date=${today.slice(0, 8)}01&end_date=${today}&bucket=month&reporting_currency_id=${currencyID}`));
  await expect(page.getByTestId('home-widget-accounts').getByRole('link', { name: 'Widget checking' })).toHaveAttribute('href', `/app/ledger/accounts/${account.id}/register`);
  await expect(page.getByTestId('home-widget-budgets')).toHaveAttribute('data-state', 'not_set_up');
  await expect(page.getByTestId('home-widget-recurring')).toHaveAttribute('data-state', 'not_set_up');
  await expect(page.getByTestId('home-widget-net_worth_trend').getByRole('img')).toBeVisible();

  // One Home read feeds every widget: no report, budget, balance or forecast call.
  expect(reads.filter(path => path === '/api/v1/home')).toHaveLength(1);
  expect(reads.filter(path => /^\/api\/v1\/(reports|budgets|ledger|forecasts|investments)\//.test(path))).toEqual([]);

  await row.click();
  await expect(page).toHaveURL(new RegExp(`/app/ledger/transactions\\?.*category_id=${groceries.id}`));
  await expect(page).toHaveURL(/date_basis=entry&status=posted/);
  await page.goBack();
  await expect(spending).toHaveAttribute('data-state', 'ready');

  // The last widget is below the fold on a phone: Back returns to it.
  const trendOpen = page.getByTestId('home-widget-net_worth_trend').getByRole('link', { name: /Open net worth report/ });
  await trendOpen.scrollIntoViewIfNeeded();
  const scrolled = await page.evaluate(() => scrollY);
  expect(scrolled).toBeGreaterThan(0);
  await trendOpen.click();
  await expect(page).toHaveURL(new RegExp(`/app/reports/net-worth\\?start_date=.*&end_date=${today}&bucket=month&reporting_currency_id=${currencyID}`));
  await page.goBack();
  await expect(page).toHaveURL(/\/app$/);
  await expect(page.getByTestId('home-widget-net_worth_trend')).toHaveAttribute('data-state', 'ready');
  await expect.poll(() => page.evaluate(() => scrollY)).toBe(scrolled);
  // Late content never displaces it.
  await page.waitForLoadState('networkidle');
  expect(await page.evaluate(() => scrollY)).toBe(scrolled);
});

for (const theme of ['light', 'dark'] as const) {
  test(`My Home widgets are accessible and reflow in ${theme}`, async ({ page }) => {
    const { currencyID } = await readyForLedger(page);
    await apiJSON(page, 'POST', `/api/v1/currencies/${currencyID}/default`, await csrfTokenFor(page), undefined, [200]);
    let token = await csrfTokenFor(page);
    // Spending in two currencies without a rate is the widest card: per-currency
    // totals instead of one (it once pushed a phone page sideways).
    const euro = await ensureCurrency(page, token, 'EUR', 'Euro');
    token = await csrfTokenFor(page);
    const dollars = await createCashAccount(page, token, 'Widget theme checking', currencyID);
    const euros = await createCashAccount(page, token, 'Widget theme euro account', euro);
    const { categories } = await apiJSON<{ categories: Category[] }>(page, 'GET', '/api/v1/categories');
    const groceries = categories.find(category => category.builtin_key === 'expense.food.groceries');
    if (!groceries) throw new Error('expected the starter Groceries category');
    const { owner_today: today } = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
    for (const [account, commodity, cents] of [[dollars.id, currencyID, 4560], [euros.id, euro, 500]]) {
      await apiJSON(page, 'POST', '/api/v1/transactions', token, {
        status: 'posted', transaction_date: today, journal_entries: [{ entry_date: today, postings: [
          { account_id: account, commodity_id: commodity, quantity_value: String(-cents), quantity_scale: 2 },
          { account_id: groceries.id, commodity_id: commodity, quantity_value: String(cents), quantity_scale: 2 }
        ] }]
      });
    }
    await page.evaluate(value => localStorage.setItem('rekenraam-theme', value), theme);
    await page.goto('/app');
    await expect(page.getByTestId('home-widget-accounts')).toHaveAttribute('data-state', 'partial');
    await expect(page.getByTestId('home-widget-spending')).toHaveAttribute('data-state', 'partial');
    await expectNoAccessibilityViolations(page, `home widgets ${theme} desktop`);
    await page.setViewportSize({ width: 390, height: 844 });
    await expectNoAccessibilityViolations(page, `home widgets ${theme} phone`);
    await page.setViewportSize({ width: 320, height: 720 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}

test('one widget that fails keeps its failure in its card', async ({ page }) => {
  await readyForLedger(page);
  await page.route('**/api/v1/home', async route => {
    const response = await route.fetch();
    const json = await response.json();
    json.widgets.budgets = { status: 'failed', summary: null };
    await route.fulfill({ response, json });
  });
  await page.goto('/app');
  const budgets = page.getByTestId('home-widget-budgets');
  await expect(budgets).toHaveAttribute('data-state', 'failed');
  await expect(budgets.getByRole('alert')).toHaveText('This summary could not be loaded.');
  await expect(page.getByTestId('home-widget-accounts')).not.toHaveAttribute('data-state', 'failed');
  await expect(page.getByTestId('home-headline')).toBeVisible();
});
