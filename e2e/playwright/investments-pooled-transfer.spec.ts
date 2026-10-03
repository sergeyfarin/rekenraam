import { expect, test, type Page } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * T-123: an average-cost holding is moved on a phone-sized screen by quantity.
 * The preview shows the pool-rate basis the commit then carries.
 */

test.use({ viewport: { width: 390, height: 844 } });

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

async function setup(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const currencies = await apiJSON<{ currencies: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/currencies');
  const currencyCode = currencies.currencies.find((currency) => currency.id === currencyID)?.code ?? '';
  const suffix = `pool${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Pool cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const name = `Pool Co ${suffix}`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: name, symbol: suffix.toUpperCase(),
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
  });
  const holding = (label: string, method?: string) => apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `${label} ${suffix}`, opened_on: openedOn, effective_from: openedOn,
    ...(method ? { cost_basis_method: method } : {})
  });
  const source = await holding('Pooled source', 'average_cost');
  const destination = await holding('Pooled destination');
  for (const [daysAgo, amount] of [[30, '1000'], [20, '2000']] as const) {
    await apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, {
      transaction_date: daysFromTodayISO(-daysAgo), commodity_id: instrument.commodity_id, holding_account_id: source.id,
      cash_account_id: cash.id, quantity_value: '1', quantity_scale: 0, cash_amount_value: amount,
      cash_amount_scale: 2, cash_commodity_id: currencyID
    });
  }
  return {
    commodityID: instrument.commodity_id, destinationID: destination.id,
    sourceLabel: `Pooled source ${suffix} · ${name} · ${currencyCode}`, destinationLabel: `Pooled destination ${suffix}`
  };
}

test('an average-cost holding moves by quantity at the previewed pool rate on mobile', async ({ page }) => {
  const s = await setup(page);
  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Move between holdings' }).click();
  await page.getByLabel('Transfer date in this book').fill(daysFromTodayISO(-10));
  await page.getByLabel('Source position').selectOption({ label: s.sourceLabel });
  await page.getByLabel('Destination holding account').selectOption({ label: s.destinationLabel });
  await expect(page.getByText('This position uses average cost.')).toBeVisible();
  await page.getByRole('textbox', { name: /Quantity to move/ }).fill('1');
  await page.getByRole('button', { name: 'Preview transfer' }).click();

  // The first lot was bought at 10.00, but it carries the pool's 15.00.
  const preview = page.getByRole('region', { name: 'What moves' });
  await expect(preview).toContainText('1 units, basis 15.00');
  await expect(preview).toContainText('not a complete tax calculation');
  await page.getByRole('button', { name: 'Record transfer' }).click();
  await expect(preview).toBeHidden();

  await expect.poll(async () => {
    const positions = await apiJSON<{ positions: Array<{ account_id: number; commodity_id: number;
      quantity_value: string; remaining_cost_basis_value: string | null; remaining_cost_basis_scale: number | null }> }>(
      page, 'GET', '/api/v1/investments/positions');
    const moved = positions.positions.find((position) =>
      position.account_id === s.destinationID && position.commodity_id === s.commodityID);
    if (!moved || moved.remaining_cost_basis_value === null || moved.remaining_cost_basis_scale === null) return null;
    return [moved.quantity_value,
      ((BigInt(moved.remaining_cost_basis_value) * 100n) / 10n ** BigInt(moved.remaining_cost_basis_scale)).toString()];
  }).toEqual(['1', '1500']);
});
