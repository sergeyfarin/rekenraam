import { expect, test, type Page } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * T-126: every existing replaying workflow shows the committed-sale gains it
 * would change and commits only after the user accepts exactly that set. The
 * Go tests pin the contract; these prove the user is shown it and can proceed.
 */

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

type Position = { csrfToken: string; currencyID: number; cashID: number; holdingID: number; commodityID: number; name: string };

async function position(page: Page, label: string): Promise<Position> {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = `${label}${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Gain review cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const name = `Gain Review ${suffix}`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: name, symbol: suffix.toUpperCase(),
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Gain review holding ${suffix}`, opened_on: openedOn, effective_from: openedOn
  });
  return { csrfToken, currencyID, cashID: cash.id, holdingID: holding.id, commodityID: instrument.commodity_id, name };
}

async function trade(page: Page, p: Position, side: 'buy' | 'sell', daysAgo: number, quantity: string, cash: string) {
  return apiJSON<{ transaction: { id: number } }>(page, 'POST', `/api/v1/investments/${side}`, p.csrfToken, {
    transaction_date: daysFromTodayISO(-daysAgo), commodity_id: p.commodityID, holding_account_id: p.holdingID,
    cash_account_id: p.cashID, quantity_value: quantity, quantity_scale: 0, cash_amount_value: cash,
    cash_amount_scale: 2, cash_commodity_id: p.currencyID, cost_basis_method: 'fifo'
  });
}

/** Exact realized gains for this instrument, as value/100 strings. */
async function gainsFor(page: Page, commodityID: number): Promise<string[]> {
  const gains = await apiJSON<{ realized: Array<{ commodity_id: number; realized_gain_value: string; realized_gain_scale: number }> }>(
    page, 'GET', '/api/v1/investments/gains');
  return gains.realized
    .filter((gain) => gain.commodity_id === commodityID)
    .map((gain) => {
      const cents = (BigInt(gain.realized_gain_value) * 100n) / 10n ** BigInt(gain.realized_gain_scale);
      return cents.toString();
    })
    .sort();
}

test('reversing a sale shows the removed gain and the later sale it revises', async ({ page }) => {
  const p = await position(page, 'rev');
  await trade(page, p, 'buy', 30, '10', '10000'); // 10.00 per share
  await trade(page, p, 'buy', 25, '10', '30000'); // 30.00 per share
  const first = await trade(page, p, 'sell', 20, '8', '16000');
  await trade(page, p, 'sell', 15, '4', '8000');

  await page.goto(`/app/transactions?transaction_id=${first.transaction.id}`);
  await page.getByRole('button', { name: 'Reverse sale…' }).click();
  await page.getByLabel('Reason for reversal').fill('entered twice');
  await page.getByRole('button', { name: 'Review and reverse' }).click();

  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText('This changes gains on earlier sales');
  await expect(dialog).toContainText(`Sale on ${daysFromTodayISO(-20)} removed: basis 80.00, gain 80.00`);
  await expect(dialog).toContainText(`Sale on ${daysFromTodayISO(-15)}: basis 80.00 → 40.00, gain 0.00 → 40.00`);
  await dialog.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(dialog).toBeHidden();
  await expect.poll(() => gainsFor(page, p.commodityID)).toEqual(['4000']);
});

test('correcting a buy shows the later sale gain it changes before replacing', async ({ page }) => {
  const p = await position(page, 'buy');
  const bought = await trade(page, p, 'buy', 20, '10', '20000');
  await trade(page, p, 'sell', 10, '5', '15000');

  await page.goto(`/app/transactions?transaction_id=${bought.transaction.id}`);
  await page.getByRole('button', { name: 'Correct buy…' }).click();
  await page.getByLabel('Reason for correction').fill('broker corrected the price');
  await page.getByLabel('Total cost').fill('20.00');
  await page.getByRole('button', { name: 'Review and replace' }).click();

  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText(`Sale on ${daysFromTodayISO(-10)}: basis 100.00 → 10.00, gain 50.00 → 140.00`);
  await dialog.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(dialog).toBeHidden();
  await expect.poll(() => gainsFor(page, p.commodityID)).toEqual(['14000']);
});

test('a backdated reinvested dividend shows the sale gain it revises', async ({ page }) => {
  const p = await position(page, 'div');
  await trade(page, p, 'buy', 10, '10', '20000');
  await trade(page, p, 'sell', 5, '5', '15000');

  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Record reinvested dividend' }).click();
  await page.getByLabel('Date').fill(daysFromTodayISO(-20));
  await page.getByLabel('Instrument').fill(p.name);
  await page.getByRole('option').filter({ hasText: p.name }).click();
  await page.getByLabel('Holding account').selectOption(String(p.holdingID));
  await page.getByLabel('Currency account (for valuation)').selectOption(String(p.cashID));
  await page.getByLabel('Quantity').fill('10');
  await page.getByLabel('Dividend amount').fill('20.00');
  await page.getByLabel(/Income account/).selectOption({ index: 1 });
  await page.getByRole('button', { name: 'Record reinvested dividend' }).last().click();

  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText(`Sale on ${daysFromTodayISO(-5)}: basis 100.00 → 10.00, gain 50.00 → 140.00`);
  await dialog.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(dialog).toBeHidden();
  await expect.poll(() => gainsFor(page, p.commodityID)).toEqual(['14000']);
});

test('reversing a buy shows the later sale gain it changes', async ({ page }) => {
  const p = await position(page, 'rvb');
  const cheap = await trade(page, p, 'buy', 30, '10', '10000'); // 10.00 per share
  await trade(page, p, 'buy', 25, '10', '30000'); // 30.00 per share
  await trade(page, p, 'sell', 20, '5', '25000');

  await page.goto(`/app/transactions?transaction_id=${cheap.transaction.id}`);
  await page.getByRole('button', { name: 'Reverse buy…' }).click();
  await page.getByLabel('Reason for reversal').fill('duplicate import');
  await page.getByRole('button', { name: 'Review and reverse' }).click();

  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText(`Sale on ${daysFromTodayISO(-20)}: basis 50.00 → 150.00, gain 200.00 → 100.00`);
  await dialog.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(dialog).toBeHidden();
  await expect.poll(() => gainsFor(page, p.commodityID)).toEqual(['10000']);
});

test('correcting a sale shows its replaced gain before replacing', async ({ page }) => {
  const p = await position(page, 'rps');
  await trade(page, p, 'buy', 20, '10', '20000');
  const sold = await trade(page, p, 'sell', 10, '5', '15000');

  await page.goto(`/app/transactions?transaction_id=${sold.transaction.id}`);
  await page.getByRole('button', { name: 'Correct sale…' }).click();
  await page.getByLabel('Reason for correction').fill('broker corrected the fill');
  await page.getByLabel(/^Proceeds/).fill('250.00');
  await page.getByRole('button', { name: 'Review and replace' }).click();

  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText(`Sale on ${daysFromTodayISO(-10)} replaced: basis 100.00 → 100.00, gain 50.00 → 150.00`);
  await dialog.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(dialog).toBeHidden();
  await expect.poll(() => gainsFor(page, p.commodityID)).toEqual(['15000']);
});
