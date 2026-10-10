import { expect, test, type Page } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * T-145: holdings arrive from another broker without a stated cost basis. On
 * a phone-sized screen the transfer is recorded with basis marked unknown, and
 * a later sale's gain is shown as unresolved rather than as a number.
 */

test.use({ viewport: { width: 390, height: 844 } });

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

test('an unknown-basis transfer in leaves a later sale gain unresolved', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const currencies = await apiJSON<{ currencies: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/currencies');
  const currencyCode = currencies.currencies.find((currency) => currency.id === currencyID)?.code ?? '';
  const suffix = `unk${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Broker cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const name = `Unknown Co ${suffix}`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: name, symbol: suffix.toUpperCase(),
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
  });
  const holdingName = `New broker ${suffix}`;
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: holdingName, opened_on: openedOn, effective_from: openedOn
  });

  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Transfer in investments' }).click();
  await page.getByLabel('Transfer date in this book').fill(daysFromTodayISO(-30));
  await page.getByLabel('Instrument').selectOption({ label: `${name} (${suffix.toUpperCase()})` });
  await page.getByLabel('Holding account').selectOption({ label: holdingName });
  await page.getByLabel('Quantity').fill('2');
  await page.getByRole('checkbox', { name: 'Cost basis unknown' }).check();
  await expect(page.getByLabel('Carried cost basis')).toBeDisabled();
  await page.getByLabel('Basis currency').selectOption({ label: currencyCode });
  await page.getByRole('button', { name: 'Record transfer' }).click();
  await expect(page.getByRole('heading', { name: 'Transfer in investments' })).toBeHidden();

  await apiJSON(page, 'POST', '/api/v1/investments/sell', csrfToken, {
    transaction_date: daysFromTodayISO(-10), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: cash.id, quantity_value: '1', quantity_scale: 0, cash_amount_value: '5000',
    cash_amount_scale: 2, cash_commodity_id: currencyID, cost_basis_method: 'fifo'
  });

  await page.getByRole('button', { name: 'Gains', exact: true }).click();
  const realized = page.getByRole('row').filter({ hasText: name });
  await expect(realized.filter({ hasText: 'Unresolved — cost basis unknown' })).toHaveCount(1);
  await expect(page.getByText(/Unresolved: 1 with unknown cost basis/)).toBeVisible();
});

test('an unknown-basis holding transfers out without a cost basis entry', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const currencies = await apiJSON<{ currencies: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/currencies');
  const currencyCode = currencies.currencies.find((currency) => currency.id === currencyID)?.code ?? '';
  const suffix = `uout${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const name = `Unknown Out Co ${suffix}`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: name, symbol: suffix.toUpperCase(),
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
  });
  const holdingName = `Unknown out holding ${suffix}`;
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: holdingName, opened_on: openedOn, effective_from: openedOn
  });
  await apiJSON(page, 'POST', '/api/v1/investments/transfers/external/in', csrfToken, {
    effective_on: daysFromTodayISO(-30), holding_account_id: holding.id, commodity_id: instrument.commodity_id,
    quantity_value: '3', quantity_scale: 0, basis_knowledge: 'unknown', cost_commodity_id: currencyID
  });

  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Transfer out investments' }).click();
  const dialog = page.getByRole('dialog', { name: 'Transfer investments out' });
  await dialog.getByLabel('Transfer date in this book').fill(daysFromTodayISO(-10));
  await dialog.getByLabel('Source position').selectOption({ label: `${holdingName} · ${name} · ${currencyCode}` });
  await dialog.getByRole('textbox', { name: /Quantity to move/ }).fill('2');
  await dialog.getByRole('button', { name: 'Preview transfer' }).click();

  const preview = dialog.getByRole('region', { name: 'What leaves the book' });
  await expect(preview).toContainText('Cost basis transferred out: unknown');
  await expect(preview).toContainText('No cost basis entry is recorded until the basis is resolved.');
  await expect(preview).toContainText('basis unknown');
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await dialog.getByRole('button', { name: 'Record transfer' }).click();
  await expect(dialog).toBeHidden();
  await expect.poll(async () => {
    const positions = await apiJSON<{ positions: Array<{ commodity_id: number; quantity_value: string }> }>(
      page, 'GET', '/api/v1/investments/positions');
    return positions.positions.find((position) => position.commodity_id === instrument.commodity_id)?.quantity_value;
  }).toBe('1');
});

// Two units arrive from another broker with unknown basis 30 days ago, and
// one is sold for 50.00 ten days ago, so its gain is unresolved.
async function seedUnknownArrivalWithSale(page: Page, prefix: string) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = `${prefix}${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Resolve cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: `Resolve Co ${suffix}`, symbol: suffix.toUpperCase(),
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Resolve holding ${suffix}`, opened_on: openedOn, effective_from: openedOn
  });
  const arrived = await apiJSON<{ transaction: { id: number } }>(page, 'POST', '/api/v1/investments/transfers/external/in', csrfToken, {
    effective_on: daysFromTodayISO(-30), holding_account_id: holding.id, commodity_id: instrument.commodity_id,
    quantity_value: '2', quantity_scale: 0, basis_knowledge: 'unknown', cost_commodity_id: currencyID
  });
  await apiJSON(page, 'POST', '/api/v1/investments/sell', csrfToken, {
    transaction_date: daysFromTodayISO(-10), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: cash.id, quantity_value: '1', quantity_scale: 0, cash_amount_value: '5000',
    cash_amount_scale: 2, cash_commodity_id: currencyID, cost_basis_method: 'fifo'
  });
  return { csrfToken, instrument, arrived };
}

// The realized gain on the instrument, in cents, or its basis knowledge
// while unresolved.
async function realizedGainCents(page: Page, commodityID: number): Promise<string | undefined> {
  const gains = await apiJSON<{ realized: Array<{ commodity_id: number; basis_knowledge: string; realized_gain_value: string | null; realized_gain_scale: number | null }> }>(
    page, 'GET', '/api/v1/investments/gains');
  const gain = gains.realized.find((entry) => entry.commodity_id === commodityID);
  if (!gain || gain.realized_gain_value === null || gain.realized_gain_scale === null) return gain?.basis_knowledge;
  return ((BigInt(gain.realized_gain_value) * 100n) / 10n ** BigInt(gain.realized_gain_scale)).toString();
}

test('a sourced statement resolves an unknown basis and its sale gain on mobile', async ({ page }) => {
  const { instrument, arrived } = await seedUnknownArrivalWithSale(page, 'res');

  await page.goto(`/app/transactions?transaction_id=${arrived.transaction.id}`);
  await page.getByRole('button', { name: 'Resolve cost basis…' }).click();
  const form = page.getByRole('dialog', { name: 'Resolve cost basis' });
  await form.getByLabel(/^Total cost basis/).fill('80.00');
  await form.getByLabel('Broker or statement reference (optional)').fill('2020 statement');
  await form.getByLabel('Reason').fill('old broker statement found');
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await form.getByRole('button', { name: 'Review and resolve' }).click();

  // The sale gain moves from unresolved to known: 50.00 − 40.00.
  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText('40.00');
  await dialog.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(form).toBeHidden();
  await expect.poll(() => realizedGainCents(page, instrument.commodity_id)).toBe('1000');
  await expect(page.getByRole('button', { name: 'Resolve cost basis…' })).toHaveCount(0);
});

// #168: a mistyped statement total is corrected, then the resolution is
// withdrawn; every resolution stays listed with its standing.
test('a sourced basis resolution is corrected and withdrawn on mobile', async ({ page }) => {
  const { csrfToken, instrument, arrived } = await seedUnknownArrivalWithSale(page, 'cor');
  const resolvePath = `/api/v1/investments/transactions/${arrived.transaction.id}/resolve-basis`;
  const resolution = { basis_value: '8000', basis_scale: 2, reason: 'old broker statement found' };
  const impact = await apiJSON<{ gain_impact: { acknowledgement: string } }>(
    page, 'POST', `${resolvePath}/reconciliation-impact`, csrfToken, resolution);
  await apiJSON(page, 'POST', resolvePath, csrfToken,
    { ...resolution, gain_impact_acknowledgement: impact.gain_impact.acknowledgement });

  await page.goto(`/app/transactions?transaction_id=${arrived.transaction.id}`);
  await page.getByRole('button', { name: 'Correct cost basis…' }).click();
  const form = page.getByRole('dialog', { name: 'Correct cost basis' });
  await expect(form.getByLabel(/^Total cost basis/)).toHaveValue('80.00');
  await form.getByLabel(/^Total cost basis/).fill('60.00');
  await form.getByLabel('Reason').fill('statement total was mistyped');
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await form.getByRole('button', { name: 'Review and correct' }).click();

  // The sold unit's basis moves from 40.00 to 30.00.
  const review = page.getByRole('alertdialog');
  await expect(review).toContainText('30.00');
  await review.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(form).toBeHidden();
  await expect.poll(() => realizedGainCents(page, instrument.commodity_id)).toBe('2000');
  // Saving closes the detail; reopen it to read the resolution history.
  await page.goto(`/app/transactions?transaction_id=${arrived.transaction.id}`);
  const history = page.getByRole('group', { name: 'Cost basis resolutions' });
  await expect(history.getByRole('listitem')).toHaveCount(2);
  await expect(history.getByRole('listitem').first()).toContainText('Superseded');
  await expect(history.getByRole('listitem').last()).toContainText('Current');

  await page.getByRole('button', { name: 'Withdraw cost basis' }).click();
  const withdraw = page.getByRole('alertdialog', { name: 'Withdraw the cost basis' });
  await withdraw.getByLabel('Reason for reversal').fill('statement belonged to another account');
  await withdraw.getByRole('button', { name: 'Review and reverse' }).click();
  await page.getByRole('alertdialog').getByRole('button', { name: 'Accept changed gains' }).click();
  await expect.poll(() => realizedGainCents(page, instrument.commodity_id)).toBe('unknown');
  await page.goto(`/app/transactions?transaction_id=${arrived.transaction.id}`);
  await expect(page.getByRole('button', { name: 'Resolve cost basis…' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Withdraw cost basis' })).toHaveCount(0);
  await expect(history.getByRole('listitem').last()).toContainText('Reversed');
  await expect(history.getByRole('listitem').last()).toContainText('Withdrawn: statement belonged to another account');
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
});
