import { test, expect } from './support/owned-book';
import { apiJSON } from './support/api';
import { expectNoAccessibilityViolations } from './support/a11y';
import { createCashAccount, readyForLedger } from './support/ledger';
import { allLocales } from './support/locales';

for (const theme of ['light', 'dark'] as const) {
  test(`Planning and Reports navigate from the phone menu with figures and scope in ${theme} (#450)`, async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const { owner_today: today } = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
    const month = today.slice(0, 7);
    const checking = await createCashAccount(page, csrfToken, 'Overview checking', currencyID);
    const expense = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
      name: 'Overview expense', category_type: 'expense'
    });
    await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
      transaction_date: today, description: 'Overview purchase',
      journal_entries: [{ entry_date: today, postings: [
        { account_id: checking.id, commodity_id: currencyID, quantity_value: '-2125', quantity_scale: 2 },
        { account_id: expense.id, commodity_id: currencyID, quantity_value: '2125', quantity_scale: 2 }
      ] }]
    });
    await apiJSON(page, 'PUT', `/api/v1/budgets/targets/${expense.id}/${currencyID}`, csrfToken, {
      period_start: `${month}-01`, quantity_value: '5000', quantity_scale: 2, change_reason: 'Overview budget'
    });
    const saved = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/gains/runs', csrfToken, {
      report_type: 'statement', from: `${month}-01`, to: today
    });

    await page.evaluate((value) => localStorage.setItem('rekenraam-theme', value), theme);
    await page.setViewportSize({ width: 320, height: 870 });
    await page.goto('/app');
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
    const overviewReads: string[] = [];
    page.on('request', request => {
      const path = new URL(request.url()).pathname;
      if (/^\/api\/v1\/(planning|budgets|recurring|forecasts|reports|investments)\//.test(path)) overviewReads.push(path);
    });

    await page.getByRole('button', { name: 'Menu: Home' }).click();
    await page.locator('#shell-phone-menu').getByRole('link', { name: 'Planning' }).click();
    const planning = page.getByTestId('planning-overview');
    await expect(planning.getByText('USD 28.75', { exact: true })).toBeVisible();
    await expect(planning.getByText('42.5% of target spent')).toBeVisible();
    await expect(planning.getByText('Nothing due for review.')).toBeVisible();
    await expect(planning.getByText(/90-day estimate/)).toBeVisible();
    await expect(planning.getByText('USD -21.25', { exact: true })).toBeVisible();
    expect(overviewReads).toEqual(['/api/v1/planning/overview']);
    await expectNoAccessibilityViolations(page, `Planning overview ${theme} 320px`);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(320);
    const hide = page.getByRole('button', { name: 'Hide amounts', exact: true });
    await hide.click();
    await expect(planning.getByTestId('overview-amount')).toHaveText(['••••', '••••']);
    await expect(planning.getByText('42.5% of target spent')).toHaveCount(0);
    await hide.click();
    await planning.getByRole('link', { name: 'Budgets', exact: true }).click();
    await expect(page).toHaveURL(`/app/planning/budgets?month=${month}&currency_id=${currencyID}`);
    await expect(page.getByText('Overview expense', { exact: true })).toBeVisible();
    await page.goBack();
    await expect(planning.getByText('USD 28.75', { exact: true })).toBeVisible();

    overviewReads.length = 0;
    await page.getByRole('button', { name: 'Menu: Planning' }).click();
    await page.locator('#shell-phone-menu').getByRole('link', { name: 'Reports' }).click();
    const reports = page.getByTestId('reports-overview');
    await expect(reports.getByText('USD 21.25', { exact: true })).toBeVisible();
    await expect(reports.getByText('USD -21.25', { exact: true })).toHaveCount(2);
    expect(overviewReads).toEqual(['/api/v1/reports/overview']);
    await expectNoAccessibilityViolations(page, `Reports overview ${theme} 320px`);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(320);
    await hide.click();
    await expect(reports.getByTestId('overview-amount')).toHaveText(['••••', '••••', '••••']);
    await hide.click();
    await reports.getByRole('link', { name: 'Spending', exact: true }).click();
    await expect(page).toHaveURL(`/app/reports/spending?start_date=${month}-01&end_date=${today}&bucket=month&reporting_currency_id=${currencyID}`);
    await expect(page.getByRole('link', { name: 'Overview expense', exact: true })).toBeVisible();
    await page.goBack();
    await expect(reports.getByText('USD 21.25', { exact: true })).toBeVisible();
    await reports.locator(`a[href*="?run_id=${saved.id}"]`).click();
    await expect(page).toHaveURL(`/app/investments/gains?run_id=${saved.id}#saved-gains`);
    await expect(page.getByTestId('saved-gains-run').getByRole('link', { name: /Download/ })).toBeVisible();

    await page.goto(`/app/reports?view=cashflow&start_date=${month}-01&end_date=${today}&bucket=month#details`);
    await expect(page).toHaveURL(`/app/reports/cashflow?start_date=${month}-01&end_date=${today}&bucket=month#details`);
  });
}

test('a failed overview card keeps the other cards usable and can be retried (#450)', async ({ page }) => {
  await readyForLedger(page);
  await page.route('**/api/v1/planning/overview', async route => {
    const response = await route.fetch();
    const data = await response.json();
    data.budget = { failed: true, data: null };
    await route.fulfill({ response, json: data });
  }, { times: 1 });
  await page.goto('/app/planning');
  const overview = page.getByTestId('planning-overview');
  await expect(overview.getByRole('alert')).toHaveText('This summary could not be loaded.');
  await expect(overview.getByRole('link', { name: 'Forecast' })).toBeVisible();
  await expect(overview.getByText('Nothing due for review.')).toBeVisible();
  await overview.getByRole('button', { name: 'Try again' }).click();
  await expect(overview.getByText('No expense targets this month.')).toBeVisible();
  await expect(overview.getByRole('alert')).toHaveCount(0);
});

test('both overview layouts fit at 320px in every locale (#450)', async ({ page }) => {
  test.setTimeout(60_000);
  await readyForLedger(page);
  await page.setViewportSize({ width: 320, height: 870 });
  for (const locale of allLocales) {
    await page.evaluate(value => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    for (const path of ['planning', 'reports']) {
      await page.goto(`/app/${path}`);
      await expect(page.locator('html')).toHaveAttribute('lang', locale);
      await expect(page.getByTestId(`${path}-overview`).getByRole('heading').first()).toBeVisible();
      expect(await page.evaluate(() => document.documentElement.scrollWidth), `${path} in ${locale}`).toBe(320);
    }
  }
});
