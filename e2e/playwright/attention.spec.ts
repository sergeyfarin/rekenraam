import { test, expect } from './support/owned-book';
import { apiJSON } from './support/api';
import { createCashAccount, ensureCurrency, readyForLedger } from './support/ledger';
import { expectNoAccessibilityViolations } from './support/a11y';

const review = (page: import('@playwright/test').Page) => page.locator('header').getByRole('link', { name: /^Review / });

test('posted non-default currency exposes missing FX in pricing and Review, then recovers with a manual rate', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  // Keep the fixture provider-free even if queued coverage runs during it.
  const { sources } = await apiJSON<{ sources: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/pricing/sources');
  const manual = sources.find(source => source.code === 'manual')!;
  await apiJSON(page, 'PUT', '/api/v1/pricing/policy', csrfToken, {
    base_commodity_id: currencyID, default_source_id: manual.id, refresh_enabled: false,
    change_reason: 'Synthetic manual-only coverage fixture'
  });
  const foreignCode = currencyID === (await ensureCurrency(page, csrfToken, 'USD', 'US Dollar')) ? 'EUR' : 'USD';
  const foreign = await ensureCurrency(page, csrfToken, foreignCode, foreignCode);
  const { currencies } = await apiJSON<{ currencies: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/currencies');
  const quoteCode = currencies.find(currency => currency.id === currencyID)!.code;
  const first = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: 'FX receivable', account_class: 'asset', account_kind: 'receivable', allows_postings: true
  });
  const second = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: 'FX payable', account_class: 'liability', account_kind: 'payable', allows_postings: true
  });
  const { owner_today: today } = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    status: 'posted', transaction_date: today, journal_entries: [{ postings: [
      { account_id: first.id, commodity_id: foreign, quantity_value: '10000', quantity_scale: 2 },
      { account_id: second.id, commodity_id: foreign, quantity_value: '-10000', quantity_scale: 2 }
    ] }]
  });
  await page.goto('/app/investments/prices');
  const pair = page.locator('li').filter({ has: page.getByRole('button', { name: `${foreignCode} in ${quoteCode}`, exact: true }) });
  await expect(pair).toContainText('No usable observation');
  await review(page).click();
  const rates = page.locator('li').filter({ has: page.getByRole('link', { name: 'Missing or out-of-date exchange rates', exact: true }) });
  await expect(rates).toContainText('1');
  await apiJSON(page, 'POST', '/api/v1/pricing/manual', csrfToken, {
    base_commodity_id: foreign, quote_commodity_id: currencyID, valuation_date: today,
    price_value: '125', price_scale: 2, base_quantity_value: '1', base_quantity_scale: 0,
    reason: 'Synthetic FX coverage recovery'
  });
  await page.getByRole('button', { name: 'Retry', exact: true }).click();
  await expect(rates).toContainText('0');
  await page.goto('/app/investments/prices');
  await expect(pair).toContainText('Within age limit');
});

test('real draft count reaches header, Home and tab, and updates after discard', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const first = await createCashAccount(page, csrfToken, 'Attention cash', currencyID);
  const second = await createCashAccount(page, csrfToken, 'Attention savings', currencyID);
  const session = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
  const template = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/recurring/templates', csrfToken, {
    name: 'Attention transfer', frequency: 'daily', starts_on: session.owner_today, lead_days: 0,
    postings: [
      { account_id: first.id, commodity_id: currencyID, quantity_value: '-100', quantity_scale: 2 },
      { account_id: second.id, commodity_id: currencyID, quantity_value: '100', quantity_scale: 2 }
    ]
  });
  await apiJSON(page, 'POST', `/api/v1/recurring/templates/${template.id}/run-now`, csrfToken, {});
  await page.goto('/app');
  const attention = await apiJSON<{ total_known: number }>(page, 'GET', '/api/v1/attention');
  expect(attention.total_known).toBe(1);
  await expect(review(page)).toContainText('1');
  await expect(review(page)).toHaveAccessibleDescription('Items to review: 1');
  await expect(page.getByTestId('home-status').getByRole('link', { name: /1 draft to review/ })).toHaveAttribute('href', '/app/review');
  await review(page).click();
  await expect(page).toHaveURL('/app/review');
  await expect(page.getByRole('link', { name: 'Draft transactions', exact: true })).toBeVisible();
  await page.getByRole('link', { name: 'Draft transactions', exact: true }).click();
  await expect(page.getByRole('navigation', { name: 'Planning pages' }).getByRole('link', { name: /^Recurring/ })).toContainText('1');
  await page.getByRole('article', { name: 'Attention transfer', exact: true }).getByRole('button', { name: 'Discard draft', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Confirm', exact: true }).click();
  await expect(review(page)).toContainText('0');
  await expect(page.getByRole('navigation', { name: 'Planning pages' }).getByRole('link', { name: 'Recurring', exact: true })).toBeVisible();
});

for (const theme of ['light', 'dark'] as const) {
  test(`shared attention reads, retry and phone accessibility in ${theme}`, async ({ page }) => {
    await readyForLedger(page);
    await page.evaluate(value => localStorage.setItem('rekenraam-theme', value), theme);
    let requests = 0;
    let broken = false;
    await page.route('**/api/v1/attention', async route => {
      requests++;
      await route.fulfill(broken ? {
        status: 500, json: { error: { code: 'INTERNAL_ERROR', message: 'Unavailable' } }
      } : {
        json: { checked_at: '2026-10-30T04:00:00Z', total_known: 0, complete: true, has_failures: false, sources: [] }
      });
    });
    const recurringReads: string[] = [];
    page.on('request', request => { if (request.url().includes('/api/v1/recurring/summary')) recurringReads.push(request.url()); });
    await page.goto('/app');
    await expect(review(page)).toContainText('0');
    expect(requests).toBe(1);
    await review(page).click();
    await expect(page.getByText('All clear for the available checks.', { exact: true })).toBeVisible();
    expect(requests).toBe(1); // Header, Home and Review share the shell observer.
    expect(recurringReads).toEqual([]);
    broken = true;
    await page.getByRole('button', { name: 'Retry', exact: true }).click();
    await expect(review(page)).toContainText('!');
    await expect(page.getByRole('alert')).toContainText('Could not check');
    await expect(page.getByText('All clear for the available checks.', { exact: true })).toHaveCount(0);
    await page.setViewportSize({ width: 320, height: 844 });
    await expectNoAccessibilityViolations(page, `attention error ${theme} phone`);
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
    broken = false;
    await page.getByRole('button', { name: 'Retry', exact: true }).click();
    await expect(review(page)).toContainText('0');
    await expect(page.getByText('All clear for the available checks.', { exact: true })).toBeVisible();
    await expectNoAccessibilityViolations(page, `attention success ${theme} phone`);
  });
}

test('a failed source remains visible even beside known counts and running work', async ({ page }) => {
  await readyForLedger(page);
  await page.route('**/api/v1/attention', route => route.fulfill({ json: {
    checked_at: '2026-10-30T04:00:00Z', total_known: 2, complete: false, has_failures: true,
    sources: [
      { id: 'imports', unit: 'batch', state: 'ready', status: '', count: 2, failed: false },
      { id: 'backups', unit: 'latest_run', state: 'error', status: '', count: null, failed: false, error_code: 'INTERNAL_ERROR' },
      { id: 'self_check', unit: 'latest_run', state: 'loading', status: 'running', count: null, failed: false }
    ]
  } }));
  await page.goto('/app/review');
  await expect(review(page)).toHaveAccessibleName('Review !');
  await expect(review(page)).toHaveAccessibleDescription('2 known items · a check failed');
  await expect(page.locator('li').filter({ has: page.getByRole('link', { name: 'Import batches to review', exact: true }) })).toContainText('2');
  await expect(page.getByText('Could not check this source.', { exact: false })).toBeVisible();
  await expect(page.getByText('Work is running;', { exact: false })).toBeVisible();
  await expect(page.getByText('All clear for the available checks.', { exact: true })).toHaveCount(0);
});
