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
  // A 1:1 exchange into the new instrument's own holding, recorded directly.
  const exchange = () =>
    apiJSON<{ transaction: { id: number } }>(page, 'POST', '/api/v1/investments/share-exchanges', csrfToken, {
      effective_on: daysFromTodayISO(-10), holding_account_id: old.holdingID,
      destination_holding_account_id: successor.holdingID, commodity_id: old.commodity_id,
      destination_commodity_id: successor.commodity_id, ratio_numerator: 1, ratio_denominator: 1
    });
  const sellNew = (daysAgo: number, quantity: string, amount: string) =>
    apiJSON(page, 'POST', '/api/v1/investments/sell', csrfToken, {
      transaction_date: daysFromTodayISO(-daysAgo), commodity_id: successor.commodity_id,
      holding_account_id: successor.holdingID, cash_account_id: cash.id, quantity_value: quantity, quantity_scale: 0,
      cash_amount_value: amount, cash_amount_scale: 2, cash_commodity_id: currencyID, cost_basis_method: 'fifo'
    });
  return { old, successor, buy, exchange, sellNew };
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
  await expect(detail.getByRole('button', { name: 'Correct exchange…' })).toBeVisible();
  await expect(detail.getByRole('button', { name: 'Reverse exchange…' })).toBeVisible();
});

test('an exchange leaving an unrepresentable fraction is refused with a translated reason', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '10', '10000');
  await openForm(page, s, '1', '7');
  await expect(page.getByRole('alert')).toContainText('Exchanges are never rounded');
});

// #179: correcting an exchange's ratio on a phone restates the later sale of
// the new units, which the user accepts from the shared gain review.
test('correcting an exchange ratio revises the later sale gain on mobile', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '10', '10000');
  const exchanged = await s.exchange();
  await s.sellNew(5, '5', '10000'); // FIFO: 5 of 10 new units, basis 50.00

  await page.goto(`/app/transactions?transaction_id=${exchanged.transaction.id}`);
  await page.getByRole('button', { name: 'Correct exchange…' }).click();
  const form = page.getByRole('dialog', { name: 'Correct this share exchange' });
  await expect(form.getByLabel('New units received')).toHaveValue('1');
  await expect(form.getByText(`${s.old.holdingName} · ${s.old.name}`)).toBeVisible();
  await form.getByLabel('Reason for correction').fill('the merger was 2 for 1');
  await form.getByLabel('New units received').fill('2');
  await form.getByRole('button', { name: 'Preview exchange' }).click();
  await expect(form.getByRole('region', { name: 'Exchange preview' })).toContainText(`10 ${s.old.name} → 20 ${s.successor.name}`);
  await form.getByRole('button', { name: 'Correct exchange' }).click();

  const review = page.getByRole('alertdialog');
  await expect(review).toContainText(`Sale on ${daysFromTodayISO(-5)}: basis 50.00 → 25.00`);
  await review.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(form).toBeHidden();
  await expect.poll(async () => {
    const chain = await apiJSON<{ effective_share_exchange?: { plan: { ratio_numerator: number; ratio_denominator: number } } }>(
      page, 'GET', `/api/v1/investments/transactions/${exchanged.transaction.id}/correction-chain`);
    const plan = chain.effective_share_exchange?.plan;
    return plan ? `${plan.ratio_numerator}:${plan.ratio_denominator}` : '';
  }).toBe('2:1');
});

test('reversing an exchange whose new units were sold is refused with a translated reason', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '10', '10000');
  const exchanged = await s.exchange();
  await s.sellNew(5, '5', '10000');

  await page.goto(`/app/transactions?transaction_id=${exchanged.transaction.id}`);
  await page.getByRole('button', { name: 'Reverse exchange…' }).click();
  await page.getByLabel('Reason for reversal').fill('no merger happened');
  await page.getByRole('button', { name: 'Review and reverse' }).click();
  await expect(page.getByRole('alertdialog')).toContainText('Reverse or correct that one first');
});

test('reversing an unsold exchange restores the old holding', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '10', '10000');
  const exchanged = await s.exchange();

  await page.goto(`/app/transactions?transaction_id=${exchanged.transaction.id}`);
  await page.getByRole('button', { name: 'Reverse exchange…' }).click();
  await page.getByLabel('Reason for reversal').fill('no merger happened');
  await page.getByRole('button', { name: 'Review and reverse' }).click();
  await expect(page.getByRole('alertdialog')).toBeHidden();
  await expect.poll(async () => {
    const lots = await apiJSON<{ lots: Array<{ status: string; remaining_quantity_value: string }> }>(
      page, 'GET', `/api/v1/investments/lots?account_id=${s.old.holdingID}&commodity_id=${s.old.commodity_id}`);
    return lots.lots.filter((lot) => lot.status === 'open').map((lot) => lot.remaining_quantity_value);
  }).toEqual(['10']);
});
