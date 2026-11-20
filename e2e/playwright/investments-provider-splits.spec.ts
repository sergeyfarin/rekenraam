import { randomUUID } from 'node:crypto';
import process from 'node:process';
import { expect, test } from './support/fixtures';
import { expectNoAccessibilityViolations } from './support/a11y';
import { apiJSON } from './support/api';
import { daysFromTodayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';

test.use({ viewport: { width: 390, height: 844 } });
test.skip(Boolean(process.env.E2E_BASE_URL), 'needs the local Massive stub');

test('[acceptance] a Massive split is reviewed and posted once on a phone', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const reference = new Date();
  const openedOn = daysFromTodayISO(-60, reference);
  const splitDate = daysFromTodayISO(-10, reference);
  const symbol = `E2ESPLIT${randomUUID().replaceAll('-', '').slice(0, 12).toUpperCase()}`;
  const name = `Split issuer ${symbol}`;
  const stubURL = process.env.E2E_TRADING212_STUB_URL ?? `http://127.0.0.1:${process.env.TRADING212_STUB_PORT ?? '16890'}`;
  const registered = await fetch(`${stubURL}/__control/splits/${symbol}`, {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ execution_date: splitDate })
  });
  expect(registered.status).toBe(204);

  const cash = await createCashAccount(page, csrfToken, `Cash ${symbol}`, currencyID, openedOn);
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: symbol, instrument_type: 'stock', display_name: name, symbol, mic: 'XNAS',
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 0, price_scale: 2, effective_from: openedOn
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Holding ${symbol}`, opened_on: openedOn, effective_from: openedOn
  });
  await apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, {
    transaction_date: daysFromTodayISO(-30, reference), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: cash.id, cash_commodity_id: currencyID, quantity_value: '10', quantity_scale: 0,
    cash_amount_value: '100000', cash_amount_scale: 2
  });

  await page.goto('/app/investments/instruments');
  await page.getByRole('button', { name: `${name} (${symbol})`, exact: true }).click();
  await page.getByLabel('Massive ticker').fill(symbol);
  await page.getByRole('button', { name: 'Save instrument', exact: true }).click();
  await expect(page.getByText('Saved.', { exact: true })).toBeVisible();

  await page.goto('/app/investments');
  await page.getByRole('link', { name: /^Suggestions/ }).click();
  await page.getByRole('button', { name: 'Check Massive splits', exact: true }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Split check queued' })).toBeVisible();
  const card = page.getByTestId('event-suggestion').filter({ hasText: name });
  await expect(card).toContainText('4 new for 1 old', { timeout: 30_000 });
  await expect(card.locator('time')).toHaveAttribute('datetime', splitDate);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await expectNoAccessibilityViolations(page, 'Massive split suggestion on a phone');

  await card.getByRole('button', { name: 'Accept', exact: true }).click();
  const settled = page.getByRole('listitem').filter({ hasText: name }).filter({ hasText: 'Accepted' });
  await expect(settled).toContainText('4 new for 1 old');
  await expect(settled.getByRole('link', { name: /^Open split #/ })).toBeVisible();
  await page.getByRole('button', { name: 'Check Massive splits', exact: true }).click();
  await expect(settled).toHaveCount(1);
  await expect(card).toHaveCount(0);
});
