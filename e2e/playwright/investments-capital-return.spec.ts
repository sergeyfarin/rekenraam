import { expect, test } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * T-148: a return of capital is recorded on a phone-sized screen, previewing
 * each lot's basis reduction and unresolved excess, then reversed from the
 * transaction detail.
 */

test.use({ viewport: { width: 390, height: 844 } });

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

test('a return of capital beyond the basis previews its excess, then is reversed on mobile', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const currencies = await apiJSON<{ currencies: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/currencies');
  const currencyCode = currencies.currencies.find((currency) => currency.id === currencyID)?.code ?? '';
  const suffix = `roc${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `ROC cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const name = `ROC Co ${suffix}`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: name, symbol: suffix.toUpperCase(),
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `ROC holding ${suffix}`, opened_on: openedOn, effective_from: openedOn
  });
  await apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, {
    transaction_date: daysFromTodayISO(-30), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: cash.id, quantity_value: '1', quantity_scale: 0, cash_amount_value: '700',
    cash_amount_scale: 2, cash_commodity_id: currencyID
  });

  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Return of capital', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Record a return of capital' });
  await dialog.getByLabel('Position').selectOption({ label: `ROC holding ${suffix} · ${name} · ${currencyCode}` });
  await dialog.getByLabel('Received into').selectOption({ label: `ROC cash ${suffix}` });
  await dialog.getByLabel('Effective date').fill(daysFromTodayISO(-10));
  await dialog.getByLabel('Payment date').fill(daysFromTodayISO(-5));
  await dialog.getByLabel(/Amount received/).fill('10.00');
  await dialog.getByRole('button', { name: 'Preview return' }).click();
  const preview = dialog.getByRole('region', { name: 'Cost basis reduced' });
  await expect(preview).toContainText('cost basis reduced by 7.00');
  await expect(preview).toContainText('3.00');
  await expect(preview).toContainText('never goes below zero');
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await dialog.getByRole('button', { name: 'Record return of capital' }).click();
  await expect(dialog).toBeHidden();

  const positionBasis = async () => {
    const positions = await apiJSON<{ positions: Array<{ account_id: number; remaining_cost_basis_value: string | null }> }>(
      page, 'GET', '/api/v1/investments/positions');
    return positions.positions.find((position) => position.account_id === holding.id)?.remaining_cost_basis_value ?? null;
  };
  await expect.poll(positionBasis).toBe('0');

  const transactions = await apiJSON<{ transactions: Array<{ id: number; transaction_date: string; description: string }> }>(
    page, 'GET', `/api/v1/transactions?account_id=${cash.id}`);
  const receipt = transactions.transactions.find((transaction) => transaction.transaction_date === daysFromTodayISO(-5));
  expect(receipt).toBeTruthy();
  await page.goto(`/app/transactions?transaction_id=${receipt!.id}`);
  await page.getByRole('button', { name: 'Reverse return of capital…' }).click();
  const reversal = page.getByRole('alertdialog');
  await reversal.getByLabel('Reason for reversal').fill('notice withdrawn');
  await reversal.getByRole('button', { name: 'Review and reverse' }).click();
  await expect(reversal).toBeHidden();
  await expect.poll(async () => {
    const value = await positionBasis();
    return value === null ? null : BigInt(value) > 0n;
  }).toBe(true);
});
