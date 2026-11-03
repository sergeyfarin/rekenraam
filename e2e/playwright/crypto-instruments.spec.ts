import { expect, test } from './support/fixtures';
import { apiJSON } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';
import { expectNoAccessibilityViolations } from './support/a11y';

test.use({ viewport: { width: 390, height: 844 } });

test('crypto instrument, holding, explicit quotes and CSV lots work on a phone', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = `crypto${Date.now()}`;
  const name = `Bitcoin ${suffix}`;
  const date = todayISO();
  const cash = await createCashAccount(page, csrfToken, `Crypto cash ${suffix}`, currencyID, date);

  await page.goto('/app/investments/instruments');
  await page.getByRole('button', { name: 'Add instrument', exact: true }).click();
  let form = page.getByRole('form', { name: 'Add instrument', exact: true });
  await form.getByLabel('Name', { exact: true }).fill(name);
  await form.getByLabel('Commodity code').fill(suffix.toUpperCase());
  await form.getByLabel('Symbol', { exact: true }).fill('BTC');
  await expect(form.getByLabel('Instrument type')).toHaveValue('crypto');
  await expect(form.getByLabel('Quantity decimal places')).toHaveValue('24');
  await form.getByLabel('Quote currency').selectOption(String(currencyID));
  await form.getByLabel('CoinGecko coin ID', { exact: false }).fill('bitcoin');
  await expectNoAccessibilityViolations(page, 'crypto instrument entry');
  await form.getByRole('button', { name: 'Save instrument' }).click();
  await expect(page.getByRole('status', { name: '' }).filter({ hasText: 'Saved.' })).toBeVisible();

  const instruments = await apiJSON<{ instruments: Array<{ id: number; commodity_id: number; commodity_code: string }> }>(page, 'GET', '/api/v1/investments/instruments');
  const instrument = instruments.instruments.find(i => i.commodity_code === suffix.toUpperCase())!;
  expect(instrument).toBeTruthy();
  form = page.getByRole('form', { name: 'Create holding account', exact: true });
  await form.getByLabel('Name', { exact: true }).fill(`Crypto holding ${suffix}`);
  await form.getByLabel('Opened on').fill(date);
  const holdingResponse = page.waitForResponse(response => response.request().method() === 'POST' && response.url().endsWith('/api/v1/investments/holding-accounts'));
  await form.getByRole('button', { name: 'Create holding account' }).click();
  expect((await holdingResponse).status()).toBe(201);
  const accounts = await apiJSON<{ accounts: Array<{ id: number; name: string }> }>(page, 'GET', '/api/v1/accounts');
  const holding = accounts.accounts.find(a => a.name === `Crypto holding ${suffix}`)!;
  expect(holding).toBeTruthy();

  await page.getByRole('combobox', { name: 'Quote provider', exact: true }).selectOption({ label: 'CoinGecko' });
  await page.getByRole('button', { name: 'Enable quotes', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Disable quotes', exact: true })).toHaveCount(1);
  // Keep e2e hermetic: don't contact live providers without operator credentials.
  await page.getByRole('button', { name: 'Disable quotes', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Disable quotes', exact: true })).toHaveCount(0);

  await page.goto('/app/ledger/import');
  await page.getByLabel('Crypto CSV', { exact: true }).check();
  await page.getByRole('combobox', { name: 'Holding account', exact: true }).selectOption(String(holding.id));
  await page.getByRole('combobox', { name: 'Cash account', exact: true }).selectOption(String(cash.id));
  await page.locator('input[type=file]').setInputFiles({ name: `${suffix}.csv`, mimeType: 'text/csv', buffer: Buffer.from(`date,side,quantity,amount,external_ref,memo\n${date},BUY,1.000000000000000000000001,100.00,buy-${suffix},Crypto buy\n${date},SELL,0.000000000000000000000001,1.00,sell-${suffix},Crypto sell\n`) });
  await page.getByRole('button', { name: 'Upload & preview' }).click();
  await expect(page.getByText('Record buy: 1.000000000000000000000001 units')).toBeVisible();
  await expect(page.getByText('Record sell: 0.000000000000000000000001 units')).toBeVisible();
  await expectNoAccessibilityViolations(page, 'crypto CSV preview');
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('Import complete', { exact: true })).toBeVisible();
  const lots = await apiJSON<{ lots: Array<{ remaining_quantity_value: string; remaining_quantity_scale: number }> }>(page, 'GET', `/api/v1/investments/lots?account_id=${holding.id}&commodity_id=${instrument.commodity_id}`);
  expect(lots.lots).toHaveLength(1);
  expect(lots.lots[0].remaining_quantity_value).toBe('1000000000000000000000000');
  expect(lots.lots[0].remaining_quantity_scale).toBe(24);
  await page.goto('/app/investments');
  await expect(page.getByRole('button', { name: `View lots for ${name}` })).toBeVisible();
});
