import { expect, test, type Page } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * T-142: part of a lot leaves the book on a phone-sized screen. The preview
 * shows the basis the server computes from the lot — not the broker's figure
 * — and the commit posts it as a labelled "Cost basis transferred out" entry.
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
  const suffix = `out${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Out cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const name = `Out Co ${suffix}`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: name, symbol: suffix.toUpperCase(),
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Out holding ${suffix}`, opened_on: openedOn, effective_from: openedOn
  });
  await apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, {
    transaction_date: daysFromTodayISO(-30), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: cash.id, quantity_value: '3', quantity_scale: 0, cash_amount_value: '3000',
    cash_amount_scale: 2, cash_commodity_id: currencyID
  });
  return { commodityID: instrument.commodity_id, holdingID: holding.id,
    sourceLabel: `Out holding ${suffix} · ${name} · ${currencyCode}` };
}

test('part of a lot transfers out of the book with its computed cost basis on mobile', async ({ page }) => {
  const s = await setup(page);
  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Transfer out investments' }).click();
  const dialog = page.getByRole('dialog', { name: 'Transfer investments out' });
  await dialog.getByLabel('Transfer date in this book').fill(daysFromTodayISO(-10));
  await dialog.getByLabel('Source position').selectOption({ label: s.sourceLabel });
  await dialog.getByRole('textbox', { name: /Quantity to move/ }).fill('2');
  await dialog.getByLabel('Basis reported by the broker (optional)').fill('19.50');
  await dialog.getByRole('button', { name: 'Preview transfer' }).click();

  // Two of three shares bought for 30.00 carry 20.00, whatever the broker says.
  const preview = dialog.getByRole('region', { name: 'What leaves the book' });
  await expect(preview).toContainText('Cost basis transferred out: 20.00');
  await expect(preview).toContainText('separate entry labelled');
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await dialog.getByRole('button', { name: 'Record transfer' }).click();
  await expect(dialog).toBeHidden();

  await expect.poll(async () => {
    const positions = await apiJSON<{ positions: Array<{ account_id: number; commodity_id: number; quantity_value: string }> }>(
      page, 'GET', '/api/v1/investments/positions');
    return positions.positions.find((position) =>
      position.account_id === s.holdingID && position.commodity_id === s.commodityID)?.quantity_value ?? null;
  }).toBe('1');
  await page.goto('/app/transactions');
  await expect(page.getByText('Cost basis transferred out').first()).toBeVisible();
});

// T-144: an outbound transfer is corrected, then the correction is reversed,
// from the transaction detail on a phone.
test('an outbound transfer is corrected and then reversed on mobile', async ({ page }) => {
  const { csrfToken } = await readyForLedger(page);
  const s = await setup(page);
  const positions = await apiJSON<{ positions: Array<{ account_id: number; cost_commodity_id: number }> }>(
    page, 'GET', '/api/v1/investments/positions');
  const costID = positions.positions.find((position) => position.account_id === s.holdingID)!.cost_commodity_id;
  const lots = await apiJSON<{ lots: Array<{ id: number }> }>(page, 'GET',
    `/api/v1/investments/lots?account_id=${s.holdingID}&commodity_id=${s.commodityID}`);
  const out = await apiJSON<{ transaction: { id: number } }>(page, 'POST', '/api/v1/investments/transfers/external/out', csrfToken, {
    effective_on: daysFromTodayISO(-10), source_account_id: s.holdingID, commodity_id: s.commodityID,
    cost_commodity_id: costID, lot_allocations: [{ lot_id: lots.lots[0].id, quantity_value: '2', quantity_scale: 0 }]
  });
  const held = async () => {
    const result = await apiJSON<{ positions: Array<{ account_id: number; commodity_id: number; quantity_value: string }> }>(
      page, 'GET', '/api/v1/investments/positions');
    return result.positions.find((position) =>
      position.account_id === s.holdingID && position.commodity_id === s.commodityID)?.quantity_value ?? '0';
  };

  await page.goto(`/app/transactions?transaction_id=${out.transaction.id}`);
  await page.getByRole('button', { name: 'Correct transfer…' }).click();
  const form = page.getByRole('dialog', { name: 'Correct this transfer out' });
  await expect(form.getByLabel('Destination holding account')).toHaveCount(0);
  const lot = form.getByLabel(/^Lot #/);
  await expect(lot).toHaveValue('2');
  await form.getByLabel('Reason for correction').fill('only one share left');
  await lot.fill('1');
  await form.getByRole('button', { name: 'Review and replace' }).click();
  await expect(form).toBeHidden();
  await expect.poll(held).toBe('2');

  const chain = await apiJSON<{ effective_transaction_id: number }>(page, 'GET',
    `/api/v1/investments/transactions/${out.transaction.id}/correction-chain`);
  await page.goto(`/app/transactions?transaction_id=${chain.effective_transaction_id}`);
  await page.getByRole('button', { name: 'Reverse transfer…' }).click();
  const dialog = page.getByRole('alertdialog');
  await dialog.getByLabel('Reason for reversal').fill('the transfer was cancelled');
  await dialog.getByRole('button', { name: 'Review and reverse' }).click();
  await expect(dialog).toBeHidden();
  await expect.poll(held).toBe('3');
});
