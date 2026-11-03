import { expect, test, type Page } from './support/fixtures';
import { apiExactText, apiJSON, csrfTokenFor, exactJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * #184: a cash-and-stock merger is entered on a phone-sized screen and
 * previewed with each lot's realized and carried basis and the cash result;
 * the transaction detail explains it, reverses or corrects it through the
 * shared gain review, and offers cash in lieu for the fraction it leaves.
 */

test.use({ viewport: { width: 390, height: 844 } });

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

async function setup(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = `mrg${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const cashName = `Merger cash ${suffix}`;
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: cashName, account_class: 'asset', account_kind: 'brokerage_cash',
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
  const target = await instrument(`${suffix}T`.toUpperCase(), `Target Co ${suffix}`);
  const acquirer = await instrument(`${suffix}A`.toUpperCase(), `Acquirer Co ${suffix}`);
  const buy = (daysAgo: number, quantity: string, amount: string) =>
    apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, {
      transaction_date: daysFromTodayISO(-daysAgo), commodity_id: target.commodity_id, holding_account_id: target.holdingID,
      cash_account_id: cash.id, quantity_value: quantity, quantity_scale: 0, cash_amount_value: amount,
      cash_amount_scale: 2, cash_commodity_id: currencyID, cost_basis_method: 'fifo'
    });
  const sellNew = (daysAgo: number, quantity: string, amount: string) =>
    apiJSON(page, 'POST', '/api/v1/investments/sell', csrfToken, {
      transaction_date: daysFromTodayISO(-daysAgo), commodity_id: acquirer.commodity_id,
      holding_account_id: acquirer.holdingID, cash_account_id: cash.id, quantity_value: quantity, quantity_scale: 0,
      cash_amount_value: amount, cash_amount_scale: 2, cash_commodity_id: currencyID, cost_basis_method: 'fifo'
    });
  // 1 new for 2 old and 30.00 of cash recovering 25 % of the basis, recorded directly.
  const merger = () =>
    apiJSON<{ transaction: { id: number } }>(page, 'POST', '/api/v1/investments/cash-and-stock-mergers', csrfToken, {
      effective_on: daysFromTodayISO(-10), payable_on: daysFromTodayISO(-8), holding_account_id: target.holdingID,
      destination_holding_account_id: acquirer.holdingID, commodity_id: target.commodity_id,
      destination_commodity_id: acquirer.commodity_id, ratio_numerator: 1, ratio_denominator: 2,
      cash_account_id: cash.id, currency_id: currencyID, cash_value: '3000', cash_scale: 2,
      cash_basis_fraction_value: '25', cash_basis_fraction_scale: 2, source_evidence: { reference: 'offer document' }
    });
  return { target, acquirer, cashName, cashAccountID: cash.id, currencyID, buy, sellNew, merger };
}

async function openForm(page: Page, s: Awaited<ReturnType<typeof setup>>, percent: string) {
  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Record cash-and-stock merger' }).click();
  await page.getByLabel('Effective date').fill(daysFromTodayISO(-10));
  await page.getByLabel('Cash payment date').fill(daysFromTodayISO(-8));
  await page.getByLabel('Holding to merge').selectOption({ label: `${s.target.holdingName} · ${s.target.name}` });
  await page.getByLabel('New instrument').selectOption({ label: s.acquirer.name });
  // The target holding is tied to the target, so the acquirer's own holding is suggested.
  await expect(page.getByLabel('Receive the new units in').locator('option:checked')).toHaveText(s.acquirer.holdingName);
  await page.getByLabel('New units received').fill('1');
  await page.getByLabel('For old units').fill('2');
  await page.getByLabel('Cash received in').selectOption({ label: s.cashName });
  await page.getByLabel('Total cash received').fill('30.00');
  await page.getByLabel('Cost basis recovered by the cash').fill(percent);
  await page.getByRole('button', { name: 'Preview merger' }).click();
}

test('a cash-and-stock merger previews the realized and carried basis and the detail explains it on mobile', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '10', '10000');
  await openForm(page, s, '25');

  const preview = page.getByRole('region', { name: 'Merger preview' });
  await expect(preview).toContainText(`10 ${s.target.name} become 5 ${s.acquirer.name} (1 new for 2 old) plus cash. The cash recovers 25% of the cost basis.`);
  await expect(preview).toContainText('Cash: 30.00');
  await expect(preview).toContainText('Basis realized: 25.00');
  await expect(preview).toContainText('Basis carried: 75.00');
  await expect(preview).toContainText('5.00');
  await page.getByRole('dialog').getByRole('button', { name: 'Record merger' }).click();
  await expect(page.getByRole('heading', { name: 'Record a cash-and-stock merger' })).toBeHidden();

  const listed = await apiJSON<{ lots: Array<{ source_transaction_id: number; remaining_quantity_value: string }> }>(
    page, 'GET', `/api/v1/investments/lots?account_id=${s.acquirer.holdingID}&commodity_id=${s.acquirer.commodity_id}`);
  expect(listed.lots.map((lot) => lot.remaining_quantity_value)).toEqual(['5']);
  await page.goto(`/app/ledger/transactions?transaction_id=${listed.lots[0].source_transaction_id}`);
  const detail = page.getByRole('group', { name: 'Cash-and-stock merger' });
  await expect(detail).toContainText(`${s.target.name} merged into ${s.acquirer.name} (1 new for 2 old)`);
  await expect(detail).toContainText(`Cash to ${s.cashName}`);
  await expect(detail).toContainText('Realized result on the cash');
  await expect(detail.getByRole('button', { name: 'Reverse merger…' })).toBeVisible();
  await expect(detail.getByRole('button', { name: 'Correct merger…' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Record cash in lieu…' })).toBeVisible();
});

test('a merger election typed with a decimal comma is read exactly', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '10', '10000');
  await openForm(page, s, '12,5');
  await expect(page.getByRole('region', { name: 'Merger preview' })).toContainText('Basis realized: 12.50');
});

// Correcting the election restates the merger's own cash result, which the
// user accepts from the shared gain review.
test('correcting a merger election restates its cash result on mobile', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '10', '10000');
  const merged = await s.merger();

  await page.goto(`/app/ledger/transactions?transaction_id=${merged.transaction.id}`);
  const detail = page.getByRole('group', { name: 'Cash-and-stock merger' });
  await detail.getByRole('button', { name: 'Correct merger…' }).click();
  const form = page.getByRole('dialog', { name: 'Correct this merger' });
  await expect(form.getByText(`${s.target.holdingName} · ${s.target.name}`)).toBeVisible();
  await expect(form.getByLabel('Cost basis recovered by the cash')).toHaveValue('25');
  await expect(form.getByLabel('Total cash received')).toHaveValue('30.00');
  await expect(form.getByLabel('Cash payment date')).toHaveValue(daysFromTodayISO(-8));
  await form.getByLabel('Reason for correction').fill('the tax rule is 50 %');
  await form.getByRole('button', { name: 'Preview merger' }).click();
  await expect(form.getByRole('alert')).toContainText(/./);
  await form.getByLabel('Cost basis recovered by the cash').fill('50');
  await form.getByRole('button', { name: 'Preview merger' }).click();
  await expect(form.getByRole('region', { name: 'Merger preview' })).toContainText('Basis realized: 50.00');
  await form.getByRole('button', { name: 'Correct merger' }).click();

  const review = page.getByRole('alertdialog');
  await expect(review).toContainText(`Merger cash on ${daysFromTodayISO(-10)} replaced: basis 25.00 → 50.00`);
  await review.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(form).toBeHidden();
  await expect.poll(async () => {
    const chain = await apiJSON<{ effective_cash_and_stock_merger?: { plan: { cash_basis_fraction_value: string; cash_basis_fraction_scale: number } } }>(
      page, 'GET', `/api/v1/investments/transactions/${merged.transaction.id}/correction-chain`);
    const plan = chain.effective_cash_and_stock_merger?.plan;
    return plan ? `${plan.cash_basis_fraction_value}/${plan.cash_basis_fraction_scale}` : '';
  }).toBe('5/1');
});

test('reversing a merger whose new units were sold is refused with a translated reason', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '10', '10000');
  const merged = await s.merger();
  await s.sellNew(5, '2', '5000');

  await page.goto(`/app/ledger/transactions?transaction_id=${merged.transaction.id}`);
  await page.getByRole('button', { name: 'Reverse merger…' }).click();
  await page.getByLabel('Reason for reversal').fill('no merger happened');
  await page.getByRole('button', { name: 'Review and reverse' }).click();
  await expect(page.getByRole('alertdialog')).toContainText('Reverse or correct that one first');
});

test('reversing an unsold merger returns the old lot and removes the cash result', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '10', '10000');
  const merged = await s.merger();

  await page.goto(`/app/ledger/transactions?transaction_id=${merged.transaction.id}`);
  await page.getByRole('button', { name: 'Reverse merger…' }).click();
  await page.getByLabel('Reason for reversal').fill('no merger happened');
  await page.getByRole('button', { name: 'Review and reverse' }).click();
  const review = page.getByRole('alertdialog');
  await expect(review).toContainText(`Merger cash on ${daysFromTodayISO(-10)} removed: basis 25.00`);
  await expect(review).not.toContainText('Sale on');
  await review.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(review).toBeHidden();
  await expect.poll(async () => {
    const lots = await apiJSON<{ lots: Array<{ status: string; remaining_quantity_value: string }> }>(
      page, 'GET', `/api/v1/investments/lots?account_id=${s.target.holdingID}&commodity_id=${s.target.commodity_id}`);
    return lots.lots.filter((lot) => lot.status === 'open').map((lot) => lot.remaining_quantity_value);
  }).toEqual(['10']);
  await expect.poll(async () => {
    const lots = await apiJSON<{ lots: Array<{ status: string }> }>(
      page, 'GET', `/api/v1/investments/lots?account_id=${s.acquirer.holdingID}&commodity_id=${s.acquirer.commodity_id}`);
    return lots.lots.filter((lot) => lot.status === 'open').length;
  }).toBe(0);
});

// #311: editing only the reference sends only the reference; the recorded
// evidence's other tokens survive exactly.
test('a merger correction of the reference preserves exact unedited source evidence', async ({ page }) => {
  const s = await setup(page);
  await s.buy(40, '10', '10000');
  const created = await apiExactText(page, 'POST', '/api/v1/investments/cash-and-stock-mergers', await csrfTokenFor(page), exactJSON({
    effective_on: daysFromTodayISO(-10), payable_on: daysFromTodayISO(-8), holding_account_id: s.target.holdingID,
    destination_holding_account_id: s.acquirer.holdingID, commodity_id: s.target.commodity_id,
    destination_commodity_id: s.acquirer.commodity_id, ratio_numerator: 1, ratio_denominator: 2,
    cash_account_id: s.cashAccountID, currency_id: s.currencyID, cash_value: '3000', cash_scale: 2,
    cash_basis_fraction_value: '25', cash_basis_fraction_scale: 2,
    source_evidence: { reference: 'offer document', revision: 'EXACT:9007199254740993' }
  }));
  const merger = JSON.parse(created) as { transaction: { id: number } };
  await page.goto(`/app/ledger/transactions?transaction_id=${merger.transaction.id}`);
  await page.getByRole('group', { name: 'Cash-and-stock merger' }).getByRole('button', { name: 'Correct merger…' }).click();
  const form = page.getByRole('dialog', { name: 'Correct this merger' });
  await expect(form.getByLabel('Broker or statement reference')).toHaveValue('offer document');
  await form.getByLabel('Reason for correction').fill('the offer document was amended');
  await form.getByLabel('Broker or statement reference').fill('amended offer document');
  await form.getByRole('button', { name: 'Preview merger' }).click();
  await expect(form.getByRole('region', { name: 'Merger preview' })).toBeVisible();
  await form.getByRole('button', { name: 'Correct merger' }).click();
  await expect(form).toBeHidden();
  const chain = await apiExactText(page, 'GET', `/api/v1/investments/transactions/${merger.transaction.id}/correction-chain`);
  expect(chain).toContain('"reference":"amended offer document"');
  expect(chain).toContain('"revision":9007199254740993');
});
