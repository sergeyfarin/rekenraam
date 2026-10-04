import { expect, test, type Page } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * T-118: a write-off is corrected and then reversed from the transaction
 * detail. The Go tests pin the replay contract; this proves the localized
 * form and review work at a phone viewport and the position follows.
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

  test('correcting and then reversing a write-off restores the holding', async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const suffix = `wof${Date.now()}`;
    const openedOn = daysFromTodayISO(-60);
    const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
      name: `Write-off cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
      default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
    });
    const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
      commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: `Write-off ${suffix}`, symbol: suffix.toUpperCase(),
      quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
    });
    const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
      instrument_id: instrument.id, name: `Write-off holding ${suffix}`, opened_on: openedOn, effective_from: openedOn
    });
    await apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, {
      transaction_date: daysFromTodayISO(-30), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
      cash_account_id: cash.id, quantity_value: '10', quantity_scale: 0, cash_amount_value: '10000',
      cash_amount_scale: 2, cash_commodity_id: currencyID
    });
    const written = await apiJSON<{ transaction: { id: number } }>(page, 'POST', '/api/v1/investments/write-off', csrfToken, {
      transaction_date: daysFromTodayISO(-10), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
      quantity_value: '10', quantity_scale: 0, reason: 'delisted', cost_basis_method: 'fifo'
    });
    await expect.poll(() => held(page, holding.id, instrument.commodity_id)).toBe('0');

    await page.goto(`/app/transactions?transaction_id=${written.transaction.id}`);
    await page.getByRole('button', { name: 'Correct write-off…' }).click();
    const form = page.getByRole('dialog', { name: 'Correct this write-off' });
    await expect(form.getByLabel('Quantity')).toHaveValue('10');
    await expect(form.getByLabel('Why the holding is worthless')).toHaveValue('delisted');
    await form.getByLabel('Reason for correction').fill('only four shares were delisted');
    await form.getByLabel('Quantity').fill('4');
    await form.getByRole('button', { name: 'Review and replace' }).click();

    // The write-off's own loss is restated: basis 100.00 becomes 40.00.
    const review = page.getByRole('alertdialog');
    await expect(review).toContainText(`on ${daysFromTodayISO(-10)} replaced: basis 100.00 → 40.00, gain -100.00 → -40.00`);
    await review.getByRole('button', { name: 'Accept changed gains' }).click();
    await expect(review).toBeHidden();
    await expect.poll(() => held(page, holding.id, instrument.commodity_id)).toBe('6');

    const chain = await apiJSON<{ effective_transaction_id: number | null }>(page, 'GET',
      `/api/v1/investments/transactions/${written.transaction.id}/correction-chain`);
    expect(chain.effective_transaction_id).not.toBeNull();
    await page.goto(`/app/transactions?transaction_id=${chain.effective_transaction_id}`);
    await page.getByRole('button', { name: 'Reverse write-off…' }).click();
    const dialog = page.getByRole('alertdialog');
    await expect(dialog).toContainText('Reverse this write-off');
    await dialog.getByLabel('Reason for reversal').fill('the fund reopened');
    await dialog.getByRole('button', { name: 'Review and reverse' }).click();
    await expect(dialog).toContainText('removed: basis 40.00, gain -40.00');
    await dialog.getByRole('button', { name: 'Accept changed gains' }).click();
    await expect(dialog).toBeHidden();
    await expect.poll(() => held(page, holding.id, instrument.commodity_id)).toBe('10');
  });
});
