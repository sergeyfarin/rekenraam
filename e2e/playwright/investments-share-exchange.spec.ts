import { expect, test, type Page } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * #178: a share exchange is entered on a phone-sized screen, previewed with its
 * exact lots and carried basis, and the new lots keep their original dates.
 */

test.use({ viewport: { width: 390, height: 844 } });

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

async function setup(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = `exch${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Exchange cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const instrument = async (code: string, name: string) => {
    const created = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
      commodity_code: code, instrument_type: 'stock', display_name: name, symbol: code,
      quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
    });
    const holdingName = `${name} holding`;
    const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
      instrument_id: created.id, name: holdingName, opened_on: openedOn, effective_from: openedOn
    });
    return { ...created, name, holdingID: holding.id, holdingName };
  };
  const old = await instrument(`${suffix}A`.toUpperCase(), `Old Co ${suffix}`);
  const successor = await instrument(`${suffix}B`.toUpperCase(), `New Co ${suffix}`);
  const buy = (daysAgo: number, quantity: string, amount: string) =>
    apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, {
      transaction_date: daysFromTodayISO(-daysAgo), commodity_id: old.commodity_id, holding_account_id: old.holdingID,
      cash_account_id: cash.id, quantity_value: quantity, quantity_scale: 0, cash_amount_value: amount,
      cash_amount_scale: 2, cash_commodity_id: currencyID, cost_basis_method: 'fifo'
    });
  return { old, successor, buy };
}

async function openForm(page: Page, s: Awaited<ReturnType<typeof setup>>, newUnits: string, oldUnits: string) {
  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Record share exchange' }).click();
  await page.getByLabel('Effective date').fill(daysFromTodayISO(-10));
  await page.getByLabel('Holding to exchange').selectOption({ label: `${s.old.holdingName} · ${s.old.name}` });
  await page.getByLabel('New instrument').selectOption({ label: s.successor.name });
  // The old holding is tied to the old instrument, so its own holding is suggested.
  await expect(page.getByLabel('Receive the new units in').locator('option:checked')).toHaveText(s.successor.holdingName);
  await page.getByLabel('New units received').fill(newUnits);
  await page.getByLabel('For old units').fill(oldUnits);
  await page.getByRole('button', { name: 'Preview exchange' }).click();
}

test('a share exchange previews exact lots and the new lots keep their original dates on mobile', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '10', '10000');
  await s.buy(30, '10', '30000');
  await openForm(page, s, '3', '2');

  const preview = page.getByRole('region', { name: 'Exchange preview' });
  await expect(preview).toContainText(`20 ${s.old.name} become 30 ${s.successor.name} (3 new for 2 old)`);
  await expect(preview).toContainText(`10 ${s.old.name} → 15 ${s.successor.name}`);
  await expect(preview).toContainText('Basis carried: 100.00');
  await expect(preview).toContainText('400.00');
  await expect(preview).toContainText('No gain or loss is realized');
  await page.getByRole('button', { name: 'Record exchange' }).click();
  await expect(page.getByRole('heading', { name: 'Record a share exchange' })).toBeHidden();

  await page.getByRole('button', { name: `View lots for ${s.successor.name}` }).click();
  const lots = page.getByRole('complementary', { name: 'Lot detail' });
  await expect(lots.getByText(`Exchanged from ${s.old.name}`)).toHaveCount(2);
  await expect(lots.getByText('Acquired', { exact: true })).toHaveCount(2);

  const listed = await apiJSON<{ lots: Array<{ source_transaction_id: number }> }>(
    page, 'GET', `/api/v1/investments/lots?account_id=${s.successor.holdingID}&commodity_id=${s.successor.commodity_id}`);
  await page.goto(`/app/transactions?transaction_id=${listed.lots[0].source_transaction_id}`);
  const detail = page.getByRole('group', { name: 'Share exchange' });
  await expect(detail).toContainText(`${s.old.name} → ${s.successor.name}: 3 new for 2 old`);
  await expect(detail).toContainText('cannot be corrected or reversed yet');
});

test('an exchange leaving an unrepresentable fraction is refused with a translated reason', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '10', '10000');
  await openForm(page, s, '1', '7');
  await expect(page.getByRole('alert')).toContainText('Exchanges are never rounded');
});
