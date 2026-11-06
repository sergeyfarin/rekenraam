import { openInvestmentActivity } from './support/investment-entry';
import { expect, test } from './support/fixtures';
import { readyForLedger, ensureCurrency } from './support/ledger';
import { apiJSON } from './support/api';

// Handlers that read a fetched response can still be running when the page
// closes (a refetch after the last assertion); without this the test fails
// with "Response has been disposed" (#236).
test.afterEach(async ({ page }) => {
  await page.unrouteAll({ behavior: 'ignoreErrors' });
});

test.use({ timezoneId: 'Pacific/Honolulu', viewport: { width: 390, height: 844 } });

test('[acceptance] server year controls investment entry, gains and unchosen discovery dates', async ({ page }) => {
  await readyForLedger(page);
  await page.clock.setFixedTime(new Date('2026-12-31T23:30:00Z'));
  await page.route('**/api/v1/auth/session', async route => {
    const response = await route.fetch();
    await route.fulfill({ response, json: { ...await response.json(), owner_today: '2027-01-01' } });
  });
  for (const [name, dateLabel] of [
    ['Record sell', 'Date'],
    ['Record dividend', 'Payment date'],
    ['Record short sale', 'Date']
  ] as const) {
    await page.goto('/app/investments');
    await openInvestmentActivity(page, name);
    const form = page.locator('form').filter({ has: page.getByRole('heading', { name, exact: true }) });
    await expect(form.getByLabel(dateLabel, { exact: true })).toHaveValue('2027-01-01');
  }
  await page.goto('/app/investments/gains');
  // The portfolio tab is driven by local UI state.
  await page.getByRole('link', { name: 'Gains', exact: true }).click();
  await expect(page.getByLabel('From', { exact: true })).toHaveValue('2027-01-01');
  await expect(page.getByLabel('To', { exact: true })).toHaveValue('2027-12-31');
  await page.goto('/app/investments');
  const discovery = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/investments/dated-holdings');
  await openInvestmentActivity(page, 'Record split');
  expect(new URL((await discovery).url()).searchParams.get('as_of')).toBe('2027-01-01');
  await expect(page.getByLabel('Split effective date')).toHaveValue('2027-01-01');
  const dated = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/investments/dated-holdings' && new URL(response.url()).searchParams.get('as_of') === '2020-02-29');
  await page.getByLabel('Split effective date').fill('2020-02-29');
  expect((await dated).status()).toBe(200);
});

test('[acceptance] currency provider assignment uses the shared server date', async ({ page }) => {
  const { csrfToken } = await readyForLedger(page);
  await ensureCurrency(page, csrfToken, 'EUR', 'Euro');
  await page.clock.setFixedTime(new Date('2020-02-29T23:30:00Z'));
  const session = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
  await page.goto('/app/settings/currencies');
  const row = page.locator('tbody tr').filter({ has: page.getByText('EUR', { exact: true }) });
  const provider = row.locator('select').first();
  await expect(provider).toBeVisible();
  const sourceID = await provider.locator('option').nth(1).getAttribute('value');
  const response = page.waitForResponse(response => ['POST', 'PATCH'].includes(response.request().method()) && new URL(response.url()).pathname.startsWith('/api/v1/pricing/source-assignments'));
  await provider.selectOption(sourceID!);
  const saved = await response;
  expect([200, 201]).toContain(saved.status());
  expect(saved.request().postDataJSON().effective_from).toBe(session.owner_today);
  expect((await saved.json()).effective_from).toBe(session.owner_today);
});

test('[acceptance] pricing captures its editable date while refreshed server today updates new dialogs', async ({ page }) => {
  await readyForLedger(page);
  let ownerDate = '2027-01-01';
  await page.route('**/api/v1/auth/session', async route => {
    const response = await route.fetch();
    await route.fulfill({ response, json: { ...await response.json(), owner_today: ownerDate } });
  });
  await page.goto('/app/investments/prices');
  const add = page.getByRole('button', { name: 'Add manual observation', exact: true });
  await add.click();
  const date = page.locator('#price-date');
  await expect(date).toHaveValue(ownerDate);
  await date.fill('2020-02-29');
  await page.locator('#price-amount').fill('1.25');
  ownerDate = '2027-01-02';
  const refreshedWorkspace = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/pricing/workspace' && new URL(response.url()).searchParams.get('as_of') === ownerDate);
  await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')));
  expect((await refreshedWorkspace).status()).toBe(200);
  await expect(date).toHaveValue('2020-02-29');
  await expect(page.locator('#price-amount')).toHaveValue('1.25');
  await page.getByRole('dialog').last().getByRole('button', { name: 'Cancel', exact: true }).click();
  await add.click();
  await expect(date).toHaveValue(ownerDate);
});
