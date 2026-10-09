import { expect, test, type Page } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';
import { expectNoAccessibilityViolations } from './support/a11y';

/**
 * #173/#174: a named short sale is opened and covered on a phone-sized screen.
 * The holding shows as a short with its owed units negative, the cover shows
 * its result before it is confirmed, and the gains report labels the short.
 * A holding that is long refuses a short sale with a translated reason.
 */

test.use({ viewport: { width: 390, height: 844 } });

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

async function setup(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = `short${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const cashName = `Short cash ${suffix}`;
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: cashName, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const name = `Short Co ${suffix}`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: name, symbol: suffix.toUpperCase(),
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
  });
  const holdingName = `Short holding ${suffix}`;
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: holdingName, opened_on: openedOn, effective_from: openedOn
  });
  const short = (side: 'short-sale' | 'short-cover', daysAgo: number, quantity: string, amount: string) =>
    apiJSON<{ transaction: { id: number } }>(page, 'POST', `/api/v1/investments/${side}`, csrfToken, {
      transaction_date: daysFromTodayISO(-daysAgo), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
      cash_account_id: cash.id, quantity_value: quantity, quantity_scale: 0, cash_amount_value: amount,
      cash_amount_scale: 2, cash_commodity_id: currencyID, ...(side === 'short-cover' ? { cost_basis_method: 'fifo' } : {})
    });
  const buy = (quantity: string, amount: string) =>
    apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, {
      transaction_date: daysFromTodayISO(-10), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
      cash_account_id: cash.id, quantity_value: quantity, quantity_scale: 0, cash_amount_value: amount,
      cash_amount_scale: 2, cash_commodity_id: currencyID
    });
  return { name, cashID: cash.id, holdingID: holding.id, commodityID: instrument.commodity_id, buy, short };
}

async function fillShortForm(page: Page, s: Awaited<ReturnType<typeof setup>>, date: string, quantity: string, amount: string) {
  const form = page.getByRole('dialog');
  await form.getByLabel('Date').fill(date);
  await form.getByLabel('Instrument').fill(s.name);
  await form.getByRole('option').filter({ hasText: s.name }).click();
  await form.getByLabel('Holding account').selectOption(String(s.holdingID));
  await form.getByLabel(/Units (borrowed and sold|bought back)/).fill(quantity);
  await form.getByLabel('Cash account').selectOption(String(s.cashID));
  await form.getByLabel(/Proceeds received|Cost to cover/).fill(amount);
  return form;
}

test('a short sale is opened and covered on mobile with its result shown first', async ({ page }) => {
  const s = await setup(page);
  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Record short sale' }).click();
  let form = await fillShortForm(page, s, daysFromTodayISO(-10), '10', '100.00');
  await expect(form).toContainText('not an ordinary sale');
  await form.getByRole('button', { name: 'Confirm short sale' }).click();
  await expect(form).toBeHidden();

  const row = page.getByRole('button', { name: new RegExp(s.name) });
  await expect(row.getByTestId('position-side-short')).toHaveText('Short');
  await expect(row).toContainText('-10');

  await page.getByRole('button', { name: 'Record short cover' }).click();
  form = await fillShortForm(page, s, daysFromTodayISO(-2), '10', '70.00');
  await expect(form.getByTestId('short-cover-result')).toContainText('+30.00');
  await expectNoAccessibilityViolations(page, 'short cover form with preview');
  await form.getByRole('button', { name: 'Confirm cover' }).click();
  await expect(form).toBeHidden();
  await expect(page.getByRole('button', { name: new RegExp(s.name) })).toHaveCount(0);

  await page.getByRole('button', { name: 'Gains' }).click();
  const realized = page.getByRole('row').filter({ hasText: s.name });
  await expect(realized.getByTestId('position-side-short')).toBeVisible();
  await expect(realized).toContainText('30.00');
});

test('a holding that is long refuses a short sale with a translated reason', async ({ page }) => {
  const s = await setup(page);
  await s.buy('5', '5000');
  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Record short sale' }).click();
  const form = await fillShortForm(page, s, daysFromTodayISO(-2), '1', '10.00');
  await form.getByRole('button', { name: 'Confirm short sale' }).click();
  await expect(form.getByRole('alert')).toContainText('cannot be long and short of the same instrument');
});

async function shortResults(page: Page, commodityID: number): Promise<string[]> {
  const gains = await apiJSON<{ realized: Array<{ commodity_id: number; realized_gain_value: string; realized_gain_scale: number }> }>(
    page, 'GET', '/api/v1/investments/gains');
  return gains.realized
    .filter((gain) => gain.commodity_id === commodityID)
    .map((gain) => ((BigInt(gain.realized_gain_value) * 100n) / 10n ** BigInt(gain.realized_gain_scale)).toString());
}

test('a cover is corrected and then reversed from its transaction on mobile (#175)', async ({ page }) => {
  const s = await setup(page);
  await s.short('short-sale', 20, '10', '10000');
  const cover = await s.short('short-cover', 10, '10', '7000');

  await page.goto(`/app/transactions?transaction_id=${cover.transaction.id}`);
  await page.getByRole('button', { name: 'Correct short cover…' }).click();
  const form = page.getByRole('dialog', { name: 'Correct this short cover' });
  await expect(form.getByLabel(/Cost to cover/)).toHaveValue('70.00');
  await form.getByLabel('Reason for correction').fill('paid 60.00');
  await form.getByLabel(/Cost to cover/).fill('60.00');
  await form.getByRole('button', { name: 'Review and replace' }).click();
  let review = page.getByRole('alertdialog');
  await expect(review).toBeVisible();
  await review.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(form).toBeHidden();
  await expect.poll(() => shortResults(page, s.commodityID)).toEqual(['4000']);

  const replacement = await apiJSON<{ effective_transaction_id: number }>(page, 'GET',
    `/api/v1/investments/transactions/${cover.transaction.id}/correction-chain`);
  await page.goto(`/app/transactions?transaction_id=${replacement.effective_transaction_id}`);
  await page.getByRole('button', { name: 'Reverse short cover…' }).click();
  await page.getByLabel('Reason for reversal').fill('cover was not executed');
  await page.getByRole('button', { name: 'Review and reverse' }).click();
  review = page.getByRole('alertdialog');
  await review.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(review).toBeHidden();
  await expect.poll(() => shortResults(page, s.commodityID)).toEqual([]);
});
