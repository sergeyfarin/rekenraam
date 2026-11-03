import { expect, test, type Page } from './support/fixtures';
import { apiExactText, apiJSON, csrfTokenFor, exactJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * T-122: a split is entered on a phone-sized screen, previewed with its exact
 * per-lot effect, and — when dated before a later sale — committed only after
 * the user accepts the sale gain it revises. T-129/T-136: a backdated purchase
 * before a split shows a labelled adjustment, and a posted split is corrected
 * or reversed from its transaction.
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
  const split = (daysAgo: number, numerator: number, denominator: number) =>
    apiJSON<{ transaction: { id: number } }>(page, 'POST', '/api/v1/investments/splits', csrfToken, {
      effective_on: daysFromTodayISO(-daysAgo), holding_account_id: holding.id, commodity_id: instrument.commodity_id,
      ratio_numerator: numerator, ratio_denominator: denominator
    });
  return { cashID: cash.id, currencyID, csrfToken, commodityID: instrument.commodity_id, holdingID: holding.id, holdingLabel: `${holdingName} · ${name}`, trade, split };
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

test('a split adjustment is labelled and correcting the split revises the later sale gain', async ({ page }) => {
  const s = await setup(page);
  await s.trade('buy', 30, '10', '10000'); // 10.00 per share
  const split = await s.split(20, 2, 1);
  await s.trade('sell', 5, '10', '10000'); // FIFO: 10 of 20 split shares, basis 50.00, gain 50.00
  await s.trade('buy', 25, '2', '2000'); // entitled to the split: an adjustment of +2

  await page.goto(`/app/ledger/accounts/${s.holdingID}/register`);
  await expect(page.getByText('Split adjustment')).toBeVisible();

  await page.goto(`/app/ledger/transactions?transaction_id=${split.transaction.id}`);
  await page.getByRole('button', { name: 'Correct split…' }).click();
  const form = page.getByRole('dialog', { name: 'Correct this split' });
  await expect(form.getByLabel('New shares')).toHaveValue('2');
  await form.getByLabel('Reason for correction').fill('it was a 3-for-1');
  await form.getByLabel('New shares').fill('3');
  await form.getByRole('button', { name: 'Preview split' }).click();
  await expect(form.getByRole('region', { name: 'What changes' })).toContainText('Holding change: 24');
  await form.getByRole('button', { name: 'Correct split' }).click();

  const review = page.getByRole('alertdialog');
  await expect(review).toContainText(`Sale on ${daysFromTodayISO(-5)}: basis 50.00 → 33.33`);
  await review.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(form).toBeHidden();
  // Basis is a third of 100.00, kept at its finer allocation scale; the
  // helper truncates the 66.6667 gain to cents.
  await expect.poll(() => gainsFor(page, s.commodityID)).toEqual(['6666']);
});

test('reversing a split a later sale depends on is refused with a translated reason', async ({ page }) => {
  const s = await setup(page);
  await s.trade('buy', 30, '10', '10000');
  const split = await s.split(20, 2, 1);
  await s.trade('sell', 5, '15', '15000'); // only possible after the split

  await page.goto(`/app/ledger/transactions?transaction_id=${split.transaction.id}`);
  await page.getByRole('button', { name: 'Reverse split…' }).click();
  await page.getByLabel('Reason for reversal').fill('no split happened');
  await page.getByRole('button', { name: 'Review and reverse' }).click();
  await expect(page.getByRole('alertdialog')).toContainText('would no longer be possible after this split');
});


test('an empty holding records a zero-delta split and a later backdated acquisition applies it on mobile', async ({ page }) => {
  const s = await setup(page);
  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Record split' }).click();
  await page.getByLabel('Holding', { exact: true }).selectOption({ label: s.holdingLabel });
  await page.getByLabel('Split effective date').fill(daysFromTodayISO(-20));
  await page.getByLabel('New shares').fill('2');
  await page.getByLabel('For old shares').fill('1');
  await page.getByRole('button', { name: 'Preview split' }).click();
  await expect(page.getByRole('region', { name: 'What changes' })).toContainText('Holding change: 0');
  const response = page.waitForResponse((r) => r.url().endsWith('/api/v1/investments/splits') && r.request().method() === 'POST');
  await page.getByRole('button', { name: 'Record split' }).last().click();
  const saved = await (await response).json();
  expect(saved.transaction).toBeNull();
  expect(saved.operation_id).toBeGreaterThan(0);
  await s.trade('buy', 30, '10', '10000');
  await page.goto(`/app/ledger/accounts/${s.holdingID}/register`);
  await expect(page.getByText('Split adjustment')).toBeVisible();
});

test('a journal-free split is inspected, corrected, settled and reversed by operation on mobile', async ({ page }) => {
  const s = await setup(page);
  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Record split' }).click();
  await page.getByLabel('Holding', { exact: true }).selectOption({ label: s.holdingLabel });
  await page.getByLabel('Split effective date').fill(daysFromTodayISO(-20));
  await page.getByLabel('New shares').fill('2');
  await page.getByLabel('For old shares').fill('1');
  await page.getByRole('button', { name: 'Preview split' }).click();
  await expect(page.getByRole('region', { name: 'What changes' })).toContainText('Holding change: 0');
  await page.getByRole('button', { name: 'Record split' }).last().click();
  await page.getByRole('link', { name: 'Splits', exact: true }).click();
  const active = page.getByRole('article').filter({ hasText: s.holdingLabel.split(' · ')[0] }).filter({ hasText: 'Active' });
  await expect(active).toContainText('Holding change: 0');
  await active.getByRole('button', { name: 'Correct split…' }).click();
  const correction = page.getByRole('dialog', { name: 'Correct this split' });
  await correction.getByLabel('Reason for correction').fill('sourced 3-for-2');
  await correction.getByLabel('New shares').fill('3');
  await correction.getByLabel('For old shares').fill('2');
  await correction.getByRole('button', { name: 'Preview split' }).click();
  await correction.getByRole('button', { name: 'Correct split', exact: true }).click();
  await expect(correction).toBeHidden();
  await expect(active).toContainText('3:2');
  await s.trade('buy', 30, '1', '10000');
  await page.reload();
  await page.getByRole('link', { name: 'Splits', exact: true }).click();
  await expect(active).toContainText('Holding change: 0.5');
  await active.getByRole('button', { name: 'Record cash in lieu', exact: true }).click();
  const settlement = page.getByRole('dialog', { name: 'Record cash in lieu', exact: true });
  // The same settlement form used from a posted split supports operation identity.
  await settlement.getByLabel('Received into', { exact: true }).selectOption(String(s.cashID));
  await settlement.getByLabel('Settlement currency', { exact: true }).selectOption(String(s.currencyID));
  await settlement.getByLabel('Fraction disposed').fill('0.5');
  await settlement.getByLabel('Cash proceeds').fill('8');
  await settlement.getByRole('button', { name: 'Preview cash in lieu' }).click();
  const posted = page.waitForResponse(r => r.url().endsWith('/api/v1/investments/cash-in-lieu') && r.request().method() === 'POST');
  await settlement.getByRole('button', { name: 'Record cash in lieu', exact: true }).click();
  await expect(settlement).toBeHidden();
  await expect(active.getByRole('button', { name: 'Reverse split…' })).toBeEnabled();
  const cash = await (await posted).json();
  await page.goto(`/app/ledger/transactions?transaction_id=${cash.transaction.id}`);
  await page.getByRole('button', {name:'Correct cash in lieu',exact:true}).click();
  const cashCorrection=page.getByRole('dialog',{name:'Correct cash in lieu',exact:true});
  await cashCorrection.getByLabel('Reason for correction').fill('broker proceeds revised');
  await cashCorrection.getByLabel('Cost-basis method').selectOption('specific_lot');
  await cashCorrection.getByLabel(/quantity to dispose \(available:/).fill('0.5');
  await cashCorrection.getByLabel('Cash proceeds').fill('8.50');
  await cashCorrection.getByRole('button',{name:'Preview cash in lieu',exact:true}).click();
  const correctedResponse=page.waitForResponse(r=>r.url().endsWith('/replace-cash-in-lieu') && r.request().method()==='POST');
  await cashCorrection.getByRole('button',{name:'Save corrected cash in lieu'}).click();
  await page.getByRole('button',{name:'Accept changed gains'}).click();
  const corrected=await(await correctedResponse).json();
  await expect(cashCorrection).toBeHidden();
  await page.goto(`/app/ledger/transactions?transaction_id=${corrected.replacement.transaction.id}`);
  await page.getByRole('button', { name: 'Reverse cash in lieu…', exact: true }).click();
  await page.getByLabel('Reason for reversal').fill('fraction payment withdrawn');
  await page.getByRole('button', { name: 'Review and reverse' }).click();
  const reversedResponse=page.waitForResponse(r=>r.url().endsWith('/reverse-cash-in-lieu') && r.request().method()==='POST');
  await page.getByRole('button', { name: 'Accept changed gains' }).click();
  expect((await reversedResponse).status()).toBe(201);
  await expect(page.getByRole('alertdialog')).toBeHidden();
  await page.goto('/app/investments?tab=splits');
  await active.getByRole('button', { name: 'Reverse split…' }).click();
  const reversal = page.getByRole('dialog', { name: 'Reverse split…', exact: true });
  await reversal.getByLabel('Reason for reversal').fill('withdraw split');
  await reversal.getByRole('button', { name: 'Review and reverse' }).click();
  await expect(reversal).toBeHidden();
  await expect(page.getByRole('article').filter({hasText:s.holdingLabel.split(' · ')[0]}).first()).toContainText(/· Reversed/);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
});

// #312: the correction form pre-fills the recorded reference and sends no
// evidence when only the ratio changes, so the recorded tokens survive
// exactly instead of becoming {}.
test('a split ratio correction preserves exact unedited source evidence', async ({ page }) => {
  const s = await setup(page);
  await s.trade('buy', 30, '10', '10000');
  const created = await apiExactText(page, 'POST', '/api/v1/investments/splits', await csrfTokenFor(page), exactJSON({
    effective_on: daysFromTodayISO(-20), holding_account_id: s.holdingID, commodity_id: s.commodityID,
    ratio_numerator: 2, ratio_denominator: 1,
    source_evidence: { reference: 'issuer notice', revision: 'EXACT:9007199254740993', nested: { rate: 'EXACT:0.1000000000000000055511151231257827' } }
  }));
  const transactionID = (JSON.parse(created) as { transaction: { id: number } }).transaction.id;
  await page.goto(`/app/ledger/transactions?transaction_id=${transactionID}`);
  await page.getByRole('button', { name: 'Correct split…' }).click();
  const form = page.getByRole('dialog', { name: 'Correct this split' });
  await expect(form.getByLabel('Broker or statement reference')).toHaveValue('issuer notice');
  await form.getByLabel('Reason for correction').fill('it was a 3-for-1');
  await form.getByLabel('New shares').fill('3');
  await form.getByRole('button', { name: 'Preview split' }).click();
  await expect(form.getByRole('region', { name: 'What changes' })).toContainText('Holding change: 20');
  await form.getByRole('button', { name: 'Correct split' }).click();
  await expect(form).toBeHidden();
  // Read the response text, never a rounded JavaScript number.
  const chain = await apiExactText(page, 'GET', `/api/v1/investments/transactions/${transactionID}/correction-chain`);
  expect(chain).toContain('"ratio_numerator":3');
  expect(chain).toContain('"revision":9007199254740993');
  expect(chain).toContain('"rate":0.1000000000000000055511151231257827');
  expect(chain).toContain('"reference":"issuer notice"');
});
