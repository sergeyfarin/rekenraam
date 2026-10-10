import { expect, test, type Page } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * #180: a spin-off is entered on a phone-sized screen, previewed with the
 * exact basis each parent lot gives up and keeps, and the new lots name the
 * parent and keep its original acquisition dates.
 */

test.use({ viewport: { width: 390, height: 844 } });

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

async function setup(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = `spin${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Spin-off cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
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
  const parent = await instrument(`${suffix}P`.toUpperCase(), `Parent Co ${suffix}`);
  const spun = await instrument(`${suffix}S`.toUpperCase(), `Spin Co ${suffix}`);
  const buy = (daysAgo: number, quantity: string, amount: string) =>
    apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, {
      transaction_date: daysFromTodayISO(-daysAgo), commodity_id: parent.commodity_id, holding_account_id: parent.holdingID,
      cash_account_id: cash.id, quantity_value: quantity, quantity_scale: 0, cash_amount_value: amount,
      cash_amount_scale: 2, cash_commodity_id: currencyID, cost_basis_method: 'fifo'
    });
  const sell = (instrument: typeof parent, daysAgo: number, quantity: string, amount: string) =>
    apiJSON(page, 'POST', '/api/v1/investments/sell', csrfToken, {
      transaction_date: daysFromTodayISO(-daysAgo), commodity_id: instrument.commodity_id,
      holding_account_id: instrument.holdingID, cash_account_id: cash.id, quantity_value: quantity, quantity_scale: 0,
      cash_amount_value: amount, cash_amount_scale: 2, cash_commodity_id: currencyID, cost_basis_method: 'fifo'
    });
  // A 1:1 spin-off of 10 % into the new instrument's own holding, recorded directly.
  const spinOff = () =>
    apiJSON<{ transaction: { id: number } }>(page, 'POST', '/api/v1/investments/spin-offs', csrfToken, {
      effective_on: daysFromTodayISO(-10), holding_account_id: parent.holdingID,
      destination_holding_account_id: spun.holdingID, commodity_id: parent.commodity_id,
      destination_commodity_id: spun.commodity_id, ratio_numerator: 1, ratio_denominator: 1,
      basis_fraction_value: '1', basis_fraction_scale: 1, source_evidence: { ex_date: daysFromTodayISO(-12) }
    });
  return { parent, spun, buy, sell, spinOff };
}

async function openForm(page: Page, s: Awaited<ReturnType<typeof setup>>, newUnits: string, parentUnits: string, percent: string) {
  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Record spin-off' }).click();
  await page.getByLabel('Distribution date').fill(daysFromTodayISO(-10));
  await page.getByLabel('Parent holding').selectOption({ label: `${s.parent.holdingName} · ${s.parent.name}` });
  await page.getByLabel('Distributed instrument').selectOption({ label: s.spun.name });
  // The parent holding is tied to the parent, so the new instrument's own holding is suggested.
  await expect(page.getByLabel('Receive the new units in').locator('option:checked')).toHaveText(s.spun.holdingName);
  await page.getByLabel('New units received').fill(newUnits);
  await page.getByLabel('Per parent units held').fill(parentUnits);
  await page.getByLabel('Cost basis moved to the new instrument').fill(percent);
  await page.getByRole('button', { name: 'Preview spin-off' }).click();
}

test('a spin-off previews the exact basis split and the new lots name the parent on mobile', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '10', '10000');
  await s.buy(30, '10', '30000');
  // One new unit for every two held; 20 % of the basis moves.
  await openForm(page, s, '1', '2', '20');

  const preview = page.getByRole('region', { name: 'Spin-off preview' });
  await expect(preview).toContainText(`10 ${s.spun.name} distributed on ${s.parent.name} (1 new for 2 held). 20% of the cost basis moves.`);
  await expect(preview).toContainText(`10 ${s.parent.name} kept → 5 ${s.spun.name}`);
  await expect(preview).toContainText('Basis moved: 20.00');
  await expect(preview).toContainText('Parent keeps: 80.00');
  await expect(preview).toContainText('Basis moved: 60.00');
  await expect(preview).toContainText('320.00');
  await expect(preview).toContainText('No gain or loss is realized');
  await page.getByRole('dialog').getByRole('button', { name: 'Record spin-off' }).click();
  await expect(page.getByRole('heading', { name: 'Record a spin-off' })).toBeHidden();

  await page.getByRole('button', { name: `View lots for ${s.spun.name}` }).click();
  const lots = page.getByRole('complementary', { name: 'Lot detail' });
  await expect(lots.getByText(`Spun off from ${s.parent.name}`)).toHaveCount(2);

  const listed = await apiJSON<{ lots: Array<{ source_transaction_id: number }> }>(
    page, 'GET', `/api/v1/investments/lots?account_id=${s.spun.holdingID}&commodity_id=${s.spun.commodity_id}`);
  await page.goto(`/app/transactions?transaction_id=${listed.lots[0].source_transaction_id}`);
  const detail = page.getByRole('group', { name: 'Spin-off' });
  await expect(detail).toContainText(`${s.spun.name} spun off from ${s.parent.name}`);
  await expect(detail).toContainText('Total basis moved');
});

test('a spin-off percentage typed with a decimal comma is read exactly', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '10', '10000');
  await openForm(page, s, '1', '1', '12,5');
  await expect(page.getByRole('region', { name: 'Spin-off preview' })).toContainText('Basis moved: 12.50');
});

test('a spin-off leaving an unrepresentable fraction is refused with a translated reason', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '1', '10000');
  await openForm(page, s, '1', '7', '10');
  await expect(page.getByRole('alert')).toContainText('Spin-offs are never rounded');
});

// #183: a spin-off dated behind a later parent sale is admitted by replay; the
// restated gain is disclosed in the shared review before anything is written.
test('a backdated spin-off revises the later parent sale through the gain review', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '10', '10000');
  await s.sell(s.parent, 5, '5', '10000'); // basis 50.00
  await openForm(page, s, '1', '1', '20');
  await expect(page.getByRole('region', { name: 'Spin-off preview' })).toContainText(`10 ${s.parent.name} kept → 10 ${s.spun.name}`);
  await page.getByRole('dialog').getByRole('button', { name: 'Record spin-off' }).click();

  const review = page.getByRole('alertdialog');
  await expect(review).toContainText(`Sale on ${daysFromTodayISO(-5)}: basis 50.00 → 40.00`);
  await review.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(page.getByRole('heading', { name: 'Record a spin-off' })).toBeHidden();
  await expect.poll(async () => {
    const lots = await apiJSON<{ lots: Array<{ remaining_quantity_value: string }> }>(
      page, 'GET', `/api/v1/investments/lots?account_id=${s.spun.holdingID}&commodity_id=${s.spun.commodity_id}`);
    return lots.lots.map((lot) => lot.remaining_quantity_value);
  }).toEqual(['10']);
});

// #183: correcting a spin-off's percentage on a phone restates the later
// parent sale, which the user accepts from the shared gain review.
test('correcting a spin-off percentage revises the later parent sale on mobile', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '10', '10000');
  const spun = await s.spinOff();
  await s.sell(s.parent, 5, '5', '10000'); // basis 45.00 after 10 % moved

  await page.goto(`/app/transactions?transaction_id=${spun.transaction.id}`);
  const detail = page.getByRole('group', { name: 'Spin-off' });
  await expect(detail.getByRole('button', { name: 'Reverse spin-off…' })).toBeVisible();
  await detail.getByRole('button', { name: 'Correct spin-off…' }).click();
  const form = page.getByRole('dialog', { name: 'Correct this spin-off' });
  await expect(form.getByText(`${s.parent.holdingName} · ${s.parent.name}`)).toBeVisible();
  await expect(form.getByLabel('Cost basis moved to the new instrument')).toHaveValue('10');
  await expect(form.getByLabel('New units received')).toHaveValue('1');
  await expect(form.getByLabel('Ex-date (optional)')).toHaveValue(daysFromTodayISO(-12));
  await form.getByLabel('Reason for correction').fill('the issuer published 30 %');
  await form.getByRole('button', { name: 'Preview spin-off' }).click();
  await expect(form.getByRole('alert')).toContainText(/./);
  await form.getByLabel('Cost basis moved to the new instrument').fill('30');
  await form.getByRole('button', { name: 'Preview spin-off' }).click();
  await expect(form.getByRole('region', { name: 'Spin-off preview' })).toContainText('Basis moved: 30.00');
  await form.getByRole('button', { name: 'Correct spin-off' }).click();

  const review = page.getByRole('alertdialog');
  await expect(review).toContainText(`Sale on ${daysFromTodayISO(-5)}: basis 45.00 → 35.00`);
  await review.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(form).toBeHidden();
  await expect.poll(async () => {
    const chain = await apiJSON<{ effective_spin_off?: { plan: { basis_fraction_value: string; basis_fraction_scale: number } } }>(
      page, 'GET', `/api/v1/investments/transactions/${spun.transaction.id}/correction-chain`);
    const plan = chain.effective_spin_off?.plan;
    return plan ? `${plan.basis_fraction_value}/${plan.basis_fraction_scale}` : '';
  }).toBe('3/1');
});

test('reversing a spin-off whose new units were sold is refused with a translated reason', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '10', '10000');
  const spun = await s.spinOff();
  await s.sell(s.spun, 5, '5', '1000');

  await page.goto(`/app/transactions?transaction_id=${spun.transaction.id}`);
  await page.getByRole('button', { name: 'Reverse spin-off…' }).click();
  await page.getByLabel('Reason for reversal').fill('no spin-off happened');
  await page.getByRole('button', { name: 'Review and reverse' }).click();
  await expect(page.getByRole('alertdialog')).toContainText('Reverse or correct that one first');
});

test('reversing an unsold spin-off gives the parent its basis back', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '10', '10000');
  const spun = await s.spinOff();

  await page.goto(`/app/transactions?transaction_id=${spun.transaction.id}`);
  await page.getByRole('button', { name: 'Reverse spin-off…' }).click();
  await page.getByLabel('Reason for reversal').fill('no spin-off happened');
  await page.getByRole('button', { name: 'Review and reverse' }).click();
  await expect(page.getByRole('alertdialog')).toBeHidden();
  await expect.poll(async () => {
    const lots = await apiJSON<{ lots: Array<{ status: string; remaining_cost_basis_value: string; remaining_cost_basis_scale: number }> }>(
      page, 'GET', `/api/v1/investments/lots?account_id=${s.parent.holdingID}&commodity_id=${s.parent.commodity_id}`);
    return lots.lots.filter((lot) => lot.status === 'open').length;
  }).toBe(1);
  await expect.poll(async () => {
    const lots = await apiJSON<{ lots: Array<{ status: string }> }>(
      page, 'GET', `/api/v1/investments/lots?account_id=${s.spun.holdingID}&commodity_id=${s.spun.commodity_id}`);
    return lots.lots.filter((lot) => lot.status === 'open').length;
  }).toBe(0);
});
