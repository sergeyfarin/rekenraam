import { expect, test, type Page } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * #166: split, return-of-capital, outbound and internal transfer entry select
 * holdings and lots as they stood on the chosen date, including holdings a
 * later sale closed. Commit still runs the real writer, gain review and
 * checkpoints; an internal transfer behind later history replays (#167).
 */

test.use({ viewport: { width: 390, height: 844 } });

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

async function setup(page: Page, prefix: string) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const currencies = await apiJSON<{ currencies: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/currencies');
  const currencyCode = currencies.currencies.find((currency) => currency.id === currencyID)?.code ?? '';
  const suffix = `${prefix}${Date.now()}`;
  const openedOn = daysFromTodayISO(-90);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Hist cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const name = `Hist Co ${suffix}`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: name, symbol: suffix.toUpperCase(),
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
  });
  const holdingName = `Hist holding ${suffix}`;
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: holdingName, opened_on: openedOn, effective_from: openedOn
  });
  const destinationName = `Hist destination ${suffix}`;
  await apiJSON(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: destinationName, opened_on: openedOn, effective_from: openedOn
  });
  const trade = (side: 'buy' | 'sell', daysAgo: number, quantity: string, amount: string) =>
    apiJSON(page, 'POST', `/api/v1/investments/${side}`, csrfToken, {
      transaction_date: daysFromTodayISO(-daysAgo), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
      cash_account_id: cash.id, quantity_value: quantity, quantity_scale: 0, cash_amount_value: amount,
      cash_amount_scale: 2, cash_commodity_id: currencyID, cost_basis_method: 'fifo'
    });
  return {
    commodityID: instrument.commodity_id, trade, cashLabel: `Hist cash ${suffix}`,
    holdingLabel: `${holdingName} · ${name}`, positionLabel: `${holdingName} · ${name} · ${currencyCode}`,
    destinationLabel: destinationName
  };
}

async function gainsFor(page: Page, commodityID: number): Promise<string[]> {
  const gains = await apiJSON<{ realized: Array<{ commodity_id: number; realized_gain_value: string; realized_gain_scale: number }> }>(
    page, 'GET', '/api/v1/investments/gains');
  return gains.realized
    .filter((gain) => gain.commodity_id === commodityID)
    .map((gain) => ((BigInt(gain.realized_gain_value) * 100n) / 10n ** BigInt(gain.realized_gain_scale)).toString());
}

test('a split of a holding sold since is entered by its date on mobile', async ({ page }) => {
  const s = await setup(page, 'hsp');
  await s.trade('buy', 40, '2', '2000'); // 10.00 per share
  await s.trade('sell', 10, '2', '3000'); // gain 10.00; the holding is closed today

  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Record split' }).click();
  const holding = page.getByLabel('Holding', { exact: true });
  await expect(holding.locator('option', { hasText: s.holdingLabel })).toHaveCount(0);
  await page.getByLabel('Split effective date').fill(daysFromTodayISO(-20));
  await expect(holding.locator('option', { hasText: s.holdingLabel })).toHaveCount(1);
  await holding.selectOption({ label: s.holdingLabel });
  await page.getByLabel('New shares').fill('2');
  await page.getByLabel('For old shares').fill('1');
  await page.getByRole('button', { name: 'Preview split' }).click();
  await expect(page.getByRole('region', { name: 'What changes' })).toContainText('2 → 4');
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await page.getByRole('button', { name: 'Record split' }).last().click();
  // The later sale still sells two shares, now half the split holding.
  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText('basis 20.00 → 10.00');
  await dialog.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(dialog).toBeHidden();
  await expect.poll(() => gainsFor(page, s.commodityID)).toEqual(['2000']);
});

test('a return of capital on a holding sold since is entered by its date on mobile', async ({ page }) => {
  const s = await setup(page, 'hrc');
  await s.trade('buy', 40, '2', '2000');
  await s.trade('sell', 10, '2', '3000');

  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Return of capital', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Record a return of capital' });
  await dialog.getByLabel('Effective date').fill(daysFromTodayISO(-20));
  await dialog.getByLabel('Position').selectOption({ label: s.positionLabel });
  await dialog.getByLabel('Received into').selectOption({ label: s.cashLabel });
  await dialog.getByLabel('Payment date').fill(daysFromTodayISO(-15));
  await dialog.getByLabel(/Amount received/).fill('4.00');
  await dialog.getByRole('button', { name: 'Preview return' }).click();
  await expect(dialog.getByRole('region', { name: 'Cost basis reduced' })).toContainText('cost basis reduced by 4.00');
  await dialog.getByRole('button', { name: 'Record return of capital' }).click();
  const review = page.getByRole('alertdialog');
  await expect(review).toContainText('basis 20.00 → 16.00');
  await review.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(dialog).toBeHidden();
  await expect.poll(() => gainsFor(page, s.commodityID)).toEqual(['1400']);
});

test('an outbound transfer from a lot consumed since lists that lot by its date', async ({ page }) => {
  const s = await setup(page, 'hto');
  await s.trade('buy', 40, '3', '3000'); // 10.00 per share
  await s.trade('buy', 30, '3', '4500'); // 15.00 per share
  await s.trade('sell', 5, '3', '6000'); // FIFO: the first lot, basis 30.00

  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Transfer out investments' }).click();
  const dialog = page.getByRole('dialog', { name: 'Transfer investments out' });
  await dialog.getByLabel('Transfer date in this book').fill(daysFromTodayISO(-20));
  await dialog.getByLabel('Source position').selectOption({ label: s.positionLabel });
  const lots = dialog.getByRole('textbox', { name: /Quantity to move/ });
  await expect(lots).toHaveCount(2);
  await lots.first().fill('2');
  await dialog.getByRole('button', { name: 'Preview transfer' }).click();
  await expect(dialog.getByRole('region', { name: 'What leaves the book' })).toContainText('Cost basis transferred out: 20.00');
  await dialog.getByRole('button', { name: 'Record transfer' }).click();
  // The later FIFO sale now takes one share of the first lot and two of the second.
  const review = page.getByRole('alertdialog');
  await expect(review).toContainText('basis 30.00 → 40.00');
  await review.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(dialog).toBeHidden();
  await expect.poll(() => gainsFor(page, s.commodityID)).toEqual(['2000']);
});

test('a dated outbound transfer that a later sale cannot survive is refused by name', async ({ page }) => {
  const s = await setup(page, 'htd');
  await s.trade('buy', 40, '3', '3000');
  await s.trade('sell', 5, '3', '6000'); // needs all three shares

  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Transfer out investments' }).click();
  const dialog = page.getByRole('dialog', { name: 'Transfer investments out' });
  await dialog.getByLabel('Transfer date in this book').fill(daysFromTodayISO(-20));
  await dialog.getByLabel('Source position').selectOption({ label: s.positionLabel });
  await dialog.getByRole('textbox', { name: /Quantity to move/ }).fill('2');
  await dialog.getByRole('button', { name: 'Preview transfer' }).click();
  await expect(dialog.getByRole('alert')).toBeVisible();
  await expect.poll(() => gainsFor(page, s.commodityID)).toEqual(['3000']);
});

test('an internal move from a lot consumed since is entered by its date on mobile', async ({ page }) => {
  const s = await setup(page, 'hti');
  await s.trade('buy', 40, '3', '3000'); // 10.00 per share
  await s.trade('buy', 30, '3', '4500'); // 15.00 per share
  await s.trade('sell', 5, '3', '6000'); // FIFO: the first lot, basis 30.00

  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Move between holdings' }).click();
  await page.getByLabel('Transfer date in this book').fill(daysFromTodayISO(-20));
  await page.getByLabel('Source position').selectOption({ label: s.positionLabel });
  await page.getByLabel('Destination holding account').selectOption({ label: s.destinationLabel });
  const lots = page.getByRole('textbox', { name: /Quantity to move/ });
  await expect(lots).toHaveCount(2);
  await lots.first().fill('2');
  await page.getByRole('button', { name: 'Preview transfer' }).click();
  await expect(page.getByRole('region', { name: 'What moves' })).toContainText('basis 20.00');
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await page.getByRole('button', { name: 'Record transfer' }).click();
  // The later FIFO sale now takes one share of the first lot and two of the second.
  const review = page.getByRole('alertdialog');
  await expect(review).toContainText('basis 30.00 → 40.00');
  await review.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(review).toBeHidden();
  await expect.poll(() => gainsFor(page, s.commodityID)).toEqual(['2000']);
});

test('a dated internal move that a later sale cannot survive is refused by name', async ({ page }) => {
  const s = await setup(page, 'hty');
  await s.trade('buy', 40, '3', '3000');
  await s.trade('sell', 5, '3', '6000'); // needs all three shares

  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Move between holdings' }).click();
  await page.getByLabel('Transfer date in this book').fill(daysFromTodayISO(-20));
  await page.getByLabel('Source position').selectOption({ label: s.positionLabel });
  await page.getByLabel('Destination holding account').selectOption({ label: s.destinationLabel });
  await page.getByRole('textbox', { name: /Quantity to move/ }).fill('2');
  await page.getByRole('button', { name: 'Preview transfer' }).click();
  await expect(page.locator('#internal-transfer-form-error')).toBeVisible();
  await expect.poll(() => gainsFor(page, s.commodityID)).toEqual(['3000']);
});
