import { expect, test, type Page } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * T-119: an internal transfer is corrected and reversed from the transaction
 * detail. The Go tests pin the replay contract; this proves the localized
 * actions work at a phone viewport and both holdings follow.
 */

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

type Positions = Array<{ account_id: number; commodity_id: number; quantity_value: string; quantity_scale: number }>;

async function held(page: Page, accountID: number, commodityID: number): Promise<string> {
  const { positions } = await apiJSON<{ positions: Positions }>(page, 'GET', '/api/v1/investments/positions');
  const position = positions.find((p) => p.account_id === accountID && p.commodity_id === commodityID);
  if (!position) return '0';
  return (BigInt(position.quantity_value) / 10n ** BigInt(position.quantity_scale)).toString();
}

test.describe('on a phone', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  async function seedTransfer(page: Page, prefix: string) {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const suffix = `${prefix}${Date.now()}`;
    const openedOn = daysFromTodayISO(-60);
    const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
      name: `Transfer cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
      default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
    });
    const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
      commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: `Transfer ${suffix}`, symbol: suffix.toUpperCase(),
      quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
    });
    const source = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
      instrument_id: instrument.id, name: `Transfer source ${suffix}`, opened_on: openedOn, effective_from: openedOn
    });
    const destination = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
      instrument_id: instrument.id, name: `Transfer destination ${suffix}`, opened_on: openedOn, effective_from: openedOn
    });
    const bought = await apiJSON<{ lot_id: number }>(page, 'POST', '/api/v1/investments/buy', csrfToken, {
      transaction_date: daysFromTodayISO(-30), commodity_id: instrument.commodity_id, holding_account_id: source.id,
      cash_account_id: cash.id, quantity_value: '10', quantity_scale: 0, cash_amount_value: '10000',
      cash_amount_scale: 2, cash_commodity_id: currencyID
    });
    const moved = await apiJSON<{ transaction: { id: number } }>(page, 'POST', '/api/v1/investments/transfers/internal', csrfToken, {
      effective_on: daysFromTodayISO(-10), source_account_id: source.id, destination_account_id: destination.id,
      commodity_id: instrument.commodity_id, cost_commodity_id: currencyID,
      lot_allocations: [{ lot_id: bought.lot_id, quantity_value: '4', quantity_scale: 0 }]
    });
    await expect.poll(() => held(page, destination.id, instrument.commodity_id)).toBe('4');
    return { source, destination, instrument, moved };
  }

  test('reversing an internal transfer returns the units to the source holding', async ({ page }) => {
    const { source, destination, instrument, moved } = await seedTransfer(page, 'xrv');
    await page.goto(`/app/transactions?transaction_id=${moved.transaction.id}`);
    await page.getByRole('button', { name: 'Reverse transfer…' }).click();
    const dialog = page.getByRole('alertdialog');
    await expect(dialog).toContainText('Reverse this transfer');
    await dialog.getByLabel('Reason for reversal').fill('moved to the wrong account');
    await dialog.getByRole('button', { name: 'Review and reverse' }).click();
    await expect(dialog).toBeHidden();
    await expect.poll(() => held(page, source.id, instrument.commodity_id)).toBe('10');
    await expect.poll(() => held(page, destination.id, instrument.commodity_id)).toBe('0');
    await expect(page.getByRole('button', { name: 'Reverse transfer…' })).toHaveCount(0);
  });

  test('correcting an internal transfer moves the corrected quantity', async ({ page }) => {
    const { source, destination, instrument, moved } = await seedTransfer(page, 'xcr');
    await page.goto(`/app/transactions?transaction_id=${moved.transaction.id}`);
    await page.getByRole('button', { name: 'Correct transfer…' }).click();
    const form = page.getByRole('dialog', { name: 'Correct this transfer' });
    const lot = form.getByLabel(/^Lot #/);
    await expect(lot).toHaveValue('4');
    await form.getByLabel('Reason for correction').fill('the statement shows six units');
    await lot.fill('6');
    await form.getByRole('button', { name: 'Review and replace' }).click();
    await expect(form).toBeHidden();
    await expect.poll(() => held(page, destination.id, instrument.commodity_id)).toBe('6');
    await expect.poll(() => held(page, source.id, instrument.commodity_id)).toBe('4');
  });
});
