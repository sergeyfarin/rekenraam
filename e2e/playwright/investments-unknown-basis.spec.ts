import { expect, test } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * T-145: holdings arrive from another broker without a stated cost basis. On
 * a phone-sized screen the transfer is recorded with basis marked unknown, and
 * a later sale's gain is shown as unresolved rather than as a number.
 */

test.use({ viewport: { width: 390, height: 844 } });

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

test('an unknown-basis transfer in leaves a later sale gain unresolved', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const currencies = await apiJSON<{ currencies: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/currencies');
  const currencyCode = currencies.currencies.find((currency) => currency.id === currencyID)?.code ?? '';
  const suffix = `unk${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Broker cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const name = `Unknown Co ${suffix}`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: name, symbol: suffix.toUpperCase(),
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
  });
  const holdingName = `New broker ${suffix}`;
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: holdingName, opened_on: openedOn, effective_from: openedOn
  });

  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Transfer in investments' }).click();
  await page.getByLabel('Transfer date in this book').fill(daysFromTodayISO(-30));
  await page.getByLabel('Instrument').selectOption({ label: `${name} (${suffix.toUpperCase()})` });
  await page.getByLabel('Holding account').selectOption({ label: holdingName });
  await page.getByLabel('Quantity').fill('2');
  await page.getByRole('checkbox', { name: 'Cost basis unknown' }).check();
  await expect(page.getByLabel('Carried cost basis')).toBeDisabled();
  await page.getByLabel('Basis currency').selectOption({ label: currencyCode });
  await page.getByRole('button', { name: 'Record transfer' }).click();
  await expect(page.getByRole('heading', { name: 'Transfer in investments' })).toBeHidden();

  await apiJSON(page, 'POST', '/api/v1/investments/sell', csrfToken, {
    transaction_date: daysFromTodayISO(-10), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: cash.id, quantity_value: '1', quantity_scale: 0, cash_amount_value: '5000',
    cash_amount_scale: 2, cash_commodity_id: currencyID, cost_basis_method: 'fifo'
  });

  await page.getByRole('button', { name: 'Gains', exact: true }).click();
  const realized = page.getByRole('row').filter({ hasText: name });
  await expect(realized.filter({ hasText: 'Unresolved — cost basis unknown' })).toHaveCount(1);
  await expect(page.getByText(/Unresolved: 1 with unknown cost basis/)).toBeVisible();
});
