import { expect, test, type Page } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * T-116: a buy correction can move the trade to another date and holding
 * account. The Go tests pin the replay contract; this proves the form offers
 * the fields, shows the gain the move restates, and both positions update.
 */

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

type Positions = Array<{ account_id: number; commodity_id: number; quantity_value: string; quantity_scale: number }>;

async function heldIn(page: Page, accountID: number, commodityID: number): Promise<string> {
  const { positions } = await apiJSON<{ positions: Positions }>(page, 'GET', '/api/v1/investments/positions');
  const position = positions.find((p) => p.account_id === accountID && p.commodity_id === commodityID);
  if (!position) return '0';
  return (BigInt(position.quantity_value) / 10n ** BigInt(position.quantity_scale)).toString();
}

test.describe('on a phone', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('moving a buy to another date and holding restates the source sale', async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const suffix = `mov${Date.now()}`;
    const openedOn = daysFromTodayISO(-60);
    const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
      name: `Move cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
      default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
    });
    const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
      commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: `Move ${suffix}`, symbol: suffix.toUpperCase(),
      quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
    });
    const holding = (name: string) => apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
      instrument_id: instrument.id, name: `${name} ${suffix}`, opened_on: openedOn, effective_from: openedOn
    });
    const source = await holding('Wrong broker');
    const target = await holding('Right broker');
    const trade = (side: 'buy' | 'sell', daysAgo: number, quantity: string, amount: string) =>
      apiJSON<{ transaction: { id: number } }>(page, 'POST', `/api/v1/investments/${side}`, csrfToken, {
        transaction_date: daysFromTodayISO(-daysAgo), commodity_id: instrument.commodity_id, holding_account_id: source.id,
        cash_account_id: cash.id, quantity_value: quantity, quantity_scale: 0, cash_amount_value: amount,
        cash_amount_scale: 2, cash_commodity_id: currencyID, cost_basis_method: 'fifo'
      });
    // Ten at 10.00 then ten at 30.00; the FIFO sale of five for 150.00 uses
    // the cheap lot (basis 50.00). Moving that lot away leaves the 30.00 lot.
    const misbooked = await trade('buy', 30, '10', '10000');
    await trade('buy', 25, '10', '30000');
    await trade('sell', 20, '5', '15000');

    await page.goto(`/app/transactions?transaction_id=${misbooked.transaction.id}`);
    await page.getByRole('button', { name: 'Correct buy…' }).click();
    await page.getByLabel('Reason for correction').fill('bought at the other broker a day earlier');
    await page.getByLabel('Date').fill(daysFromTodayISO(-31));
    await page.getByLabel('Holding account').selectOption(String(target.id));
    await page.getByRole('button', { name: 'Review and replace' }).click();

    const dialog = page.getByRole('alertdialog');
    await expect(dialog).toContainText(`Sale on ${daysFromTodayISO(-20)}: basis 50.00 → 150.00, gain 100.00 → 0.00`);
    await dialog.getByRole('button', { name: 'Accept changed gains' }).click();
    await expect(dialog).toBeHidden();
    await expect.poll(() => heldIn(page, target.id, instrument.commodity_id)).toBe('10');
    await expect.poll(() => heldIn(page, source.id, instrument.commodity_id)).toBe('5');
  });
});
