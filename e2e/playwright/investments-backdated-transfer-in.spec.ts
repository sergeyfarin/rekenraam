import { expect, test } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * T-117: holdings from an old broker are entered after the current broker's
 * sale on a phone-sized screen. The transfer is admitted behind that sale, and
 * the sale gain it revises is confirmed before recording.
 */

test.use({ viewport: { width: 390, height: 844 } });

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

test('a backdated transfer-in shows the sale gain it revises before recording', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const currencies = await apiJSON<{ currencies: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/currencies');
  const currencyCode = currencies.currencies.find((currency) => currency.id === currencyID)?.code ?? '';
  const suffix = `xin${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Broker cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const name = `Moved Co ${suffix}`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: name, symbol: suffix.toUpperCase(),
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
  });
  const holdingName = `Current broker ${suffix}`;
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: holdingName, opened_on: openedOn, effective_from: openedOn
  });
  for (const [side, daysAgo, amount] of [['buy', 20, '8000'], ['sell', 10, '10000']] as const) {
    await apiJSON(page, 'POST', `/api/v1/investments/${side}`, csrfToken, {
      transaction_date: daysFromTodayISO(-daysAgo), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
      cash_account_id: cash.id, quantity_value: '2', quantity_scale: 0, cash_amount_value: amount,
      cash_amount_scale: 2, cash_commodity_id: currencyID, cost_basis_method: 'fifo'
    });
  }

  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Transfer in investments' }).click();
  await page.getByLabel('Transfer date in this book').fill(daysFromTodayISO(-30));
  await page.getByLabel('Instrument').selectOption({ label: `${name} (${suffix.toUpperCase()})` });
  await page.getByLabel('Holding account').selectOption({ label: holdingName });
  await page.getByLabel('Quantity').fill('2');
  await page.getByLabel('Carried cost basis').fill('30.00');
  await page.getByLabel('Basis currency').selectOption({ label: currencyCode });
  await page.getByLabel('Original acquisition date (optional)').fill('2020-03-01');
  await page.getByRole('button', { name: 'Record transfer' }).click();

  // FIFO now sells the 2020 shares: basis 80.00 → 30.00, gain 20.00 → 70.00.
  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText(`Sale on ${daysFromTodayISO(-10)}: basis 80.00 → 30.00, gain 20.00 → 70.00`);
  await dialog.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(dialog).toBeHidden();
  await expect.poll(async () => {
    const gains = await apiJSON<{ realized: Array<{ commodity_id: number; realized_gain_value: string; realized_gain_scale: number }> }>(
      page, 'GET', '/api/v1/investments/gains');
    return gains.realized.filter((gain) => gain.commodity_id === instrument.commodity_id)
      .map((gain) => ((BigInt(gain.realized_gain_value) * 100n) / 10n ** BigInt(gain.realized_gain_scale)).toString());
  }).toEqual(['7000']);
});
