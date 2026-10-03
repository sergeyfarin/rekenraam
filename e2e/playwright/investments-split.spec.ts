import { expect, test, type Page } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * T-122: a split is entered on a phone-sized screen, previewed with its exact
 * per-lot effect, and — when dated before a later sale — committed only after
 * the user accepts the sale gain it revises.
 */

test.use({ viewport: { width: 390, height: 844 } });

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

async function setup(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = `split${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Split cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const name = `Split Co ${suffix}`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: name, symbol: suffix.toUpperCase(),
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
  });
  const holdingName = `Split holding ${suffix}`;
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: holdingName, opened_on: openedOn, effective_from: openedOn
  });
  const trade = (side: 'buy' | 'sell', daysAgo: number, quantity: string, amount: string) =>
    apiJSON(page, 'POST', `/api/v1/investments/${side}`, csrfToken, {
      transaction_date: daysFromTodayISO(-daysAgo), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
      cash_account_id: cash.id, quantity_value: quantity, quantity_scale: 0, cash_amount_value: amount,
      cash_amount_scale: 2, cash_commodity_id: currencyID, cost_basis_method: 'fifo'
    });
  return { commodityID: instrument.commodity_id, holdingLabel: `${holdingName} · ${name}`, trade };
}

async function gainsFor(page: Page, commodityID: number): Promise<string[]> {
  const gains = await apiJSON<{ realized: Array<{ commodity_id: number; realized_gain_value: string; realized_gain_scale: number }> }>(
    page, 'GET', '/api/v1/investments/gains');
  return gains.realized
    .filter((gain) => gain.commodity_id === commodityID)
    .map((gain) => ((BigInt(gain.realized_gain_value) * 100n) / 10n ** BigInt(gain.realized_gain_scale)).toString());
}

test('a backdated split previews its lot effect and revises the later sale gain on mobile', async ({ page }) => {
  const s = await setup(page);
  await s.trade('buy', 30, '10', '10000'); // 10.00 per share
  await s.trade('sell', 5, '5', '10000'); // basis 50.00, gain 50.00

  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Record split' }).click();
  await page.getByLabel('Holding', { exact: true }).selectOption({ label: s.holdingLabel });
  await page.getByLabel('Split effective date').fill(daysFromTodayISO(-20));
  await page.getByLabel('New shares').fill('2');
  await page.getByLabel('For old shares').fill('1');
  await page.getByRole('button', { name: 'Preview split' }).click();

  const preview = page.getByRole('region', { name: 'What changes' });
  await expect(preview).toContainText('10 → 20');
  await expect(preview).toContainText('Holding change: 10');
  await expect(preview).toContainText('Later sales are recalculated');
  await page.getByRole('button', { name: 'Record split' }).last().click();

  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText(`Sale on ${daysFromTodayISO(-5)}: basis 50.00 → 25.00, gain 50.00 → 75.00`);
  await dialog.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(dialog).toBeHidden();
  await expect.poll(() => gainsFor(page, s.commodityID)).toEqual(['7500']);
});

test('an unrepresentable reverse split is refused with a translated reason', async ({ page }) => {
  const s = await setup(page);
  await s.trade('buy', 30, '10', '10000');
  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Record split' }).click();
  await page.getByLabel('Holding', { exact: true }).selectOption({ label: s.holdingLabel });
  await page.getByLabel('Split effective date').fill(daysFromTodayISO(-20));
  await page.getByLabel('New shares').fill('1');
  await page.getByLabel('For old shares').fill('7');
  await page.getByRole('button', { name: 'Preview split' }).click();
  await expect(page.getByRole('alert')).toContainText('Splits are never rounded');
});
