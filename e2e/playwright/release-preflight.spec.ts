import { expect, type Page, test } from '@playwright/test';
import { apiJSON } from './support/api';
import { monthStartISO, todayISO, todayQIF } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';

test.describe.serial('release preflight journeys', () => {
  test('enters a balanced split transfer', async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const suffix = Date.now();
    const checking = await createCashAccount(page, csrfToken, `Preflight checking ${suffix}`, currencyID);
    const savings = await createCashAccount(page, csrfToken, `Preflight savings ${suffix}`, currencyID);

    await page.goto('/app/transactions');
    await page.getByRole('button', { name: 'New transaction' }).click();
    const form = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Add transaction' }) });
    await form.getByLabel('Date').fill(todayISO());
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

    await expect(transactionTable(page).getByText(`Preflight transfer ${suffix}`)).toBeVisible();
  });

  test('reconciles a selected posting to zero', async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const suffix = Date.now();
    const account = await createCashAccount(page, csrfToken, `Preflight reconcile ${suffix}`, currencyID);
    await createSimpleTransaction(page, account.id, `Preflight reconcile entry ${suffix}`, '10.00');

    await page.goto('/app/reconcile');
    await page.getByLabel('Account').selectOption({ label: account.name });
    await page.getByLabel('Statement closing date').fill(todayISO());
    await page.getByLabel('Statement ending balance').fill('10.00');
    await page.getByRole('button', { name: 'Start reconciling' }).click();

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
      buffer: Buffer.from(`!Type:Bank\nD${todayQIF()}\nT-12.34\nPPreflight QIF Payee\nMPreflight QIF memo\n^\n`)
    });
    await page.getByRole('button', { name: 'Upload & preview' }).click();
    await expect(page.getByLabel('Target account')).toBeVisible();
    await page.getByLabel('Target account').selectOption(String(account.id));
    await page.getByLabel('Currency').selectOption(String(currencyID));
    await page.getByLabel('Default category').selectOption({ index: 1 });
    await page.getByRole('button', { name: 'Apply to all rows' }).click();
    await page.getByRole('button', { name: 'Commit to ledger' }).click();
    await expect(page.getByText('Import complete')).toBeVisible();
    await expect(page.getByText('1 transactions committed')).toBeVisible();
  });

  test('records a buy, previews a sell, and commits it', async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const suffix = Date.now();
    const cash = await createCashAccount(page, csrfToken, `Preflight brokerage cash ${suffix}`, currencyID);
    const instrument = await apiJSON<{ id: number; display_name: string }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
      commodity_code: `PF${suffix}`,
      instrument_type: 'stock',
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
      opened_on: monthStartISO()
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
    await form.getByLabel('Date').fill(todayISO());
    await form.getByLabel('Description').fill(`Preflight mobile entry ${suffix}`);
    await form.getByLabel('Account').selectOption(String(account.id));
    await form.getByLabel('Amount').fill('-3.50');
    await form.getByPlaceholder('Search categories').click();
    await form.locator('ul[role="listbox"] button[role="option"]').first().click();
    await form.getByRole('button', { name: 'Add transaction' }).click();
    await expect(transactionTable(page).getByText(`Preflight mobile entry ${suffix}`)).toBeVisible();
  });
});

/**
 * The transaction list. Saving a transaction also opens the detail panel, so
 * an unscoped text locator matches the row, the panel heading and the panel
 * field alike — scope list assertions to the table.
 */
function transactionTable(page: Page) {
  return page.locator('[data-transaction-table]');
}

async function createSimpleTransaction(page: Page, accountID: number, description: string, amount: string) {
  await page.goto('/app/transactions');
  await page.getByRole('button', { name: 'New transaction' }).click();
  const form = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Add transaction' }) });
  await form.getByLabel('Date').fill(todayISO());
  await form.getByLabel('Description').fill(description);
  await form.getByLabel('Account').selectOption(String(accountID));
  await form.getByLabel('Amount').fill(amount);
  await form.getByPlaceholder('Search categories').click();
  await form.locator('ul[role="listbox"] button[role="option"]').first().click();
  await form.getByRole('button', { name: 'Add transaction' }).click();
  await expect(transactionTable(page).getByText(description)).toBeVisible();
}
