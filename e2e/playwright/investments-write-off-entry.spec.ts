import { expect, test, type Page } from './support/fixtures';
import AxeBuilder from '@axe-core/playwright';
import { readFileSync } from 'node:fs';
import { apiJSON } from './support/api';
import { readyForLedger, ensureCurrency } from './support/ledger';
import { todayISO } from './support/dates';
import { useDarkTheme } from './support/session';
import { translationLocales } from './support/locales';

test.use({ viewport: { width: 390, height: 844 } });

function ago(days: number) {
  const date = new Date();
  date.setDate(date.getDate() - days);
  return todayISO(date);
}
function messages(locale = 'en') {
  return JSON.parse(readFileSync(new URL(`../../frontend/messages/settings/${locale}.json`, import.meta.url), 'utf8'));
}
async function setup(page: Page, prefix: string) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = `${prefix}${Date.now()}`;
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: ago(60), effective_from: ago(60)
  });
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: suffix, symbol: suffix.toUpperCase(),
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: ago(60)
  });
  const holding = await apiJSON<{ id: number; name: string }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Holding ${suffix}`, opened_on: ago(60), effective_from: ago(60)
  });
  const trade = (side: 'buy' | 'sell', days: number, quantity: string, amount: string) =>
    apiJSON<{ allocations: Array<{ lot_id: number }> }>(page, 'POST', `/api/v1/investments/${side}`, csrfToken, {
      transaction_date: ago(days), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
      cash_account_id: cash.id, quantity_value: quantity, quantity_scale: 0,
      cash_amount_value: amount, cash_amount_scale: 2, cash_commodity_id: currencyID, cost_basis_method: 'fifo'
    });
  return { csrfToken, currencyID, cash, instrument, holding, trade, name: suffix, key: `${holding.id}:${instrument.commodity_id}:${currencyID}` };
}
type Fixture = Awaited<ReturnType<typeof setup>>;
type Gain = { disposal_date: string; commodity_id: number; realized_gain_value: string | null; realized_gain_scale: number | null };
async function gains(page: Page, s: Fixture) {
  const result = await apiJSON<{ realized: Gain[] }>(page, 'GET', '/api/v1/investments/gains');
  return result.realized.filter(gain => gain.commodity_id === s.instrument.commodity_id).sort((a,b) => a.disposal_date.localeCompare(b.disposal_date)).map(gain => gain.realized_gain_value === null || gain.realized_gain_scale === null ? null :
      (BigInt(gain.realized_gain_value) * 100n / 10n ** BigInt(gain.realized_gain_scale)).toString());
}
async function open(page: Page, s: Fixture, days = 0, locale = 'en') {
  const m = messages(locale);
  await page.evaluate(value => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
  await page.goto('/app/investments');
  await page.getByRole('button', { name: m.investments_record_write_off, exact: true }).click();
  const dialog = page.getByRole('dialog', { name: m.investments_write_off_title });
  await dialog.locator('#write-off-date').fill(ago(days));
  await dialog.locator('#write-off-holding').selectOption(s.key);
  await dialog.locator('#write-off-reason').fill('Issuer liquidated');
  return dialog;
}

for (const locale of translationLocales) {
  test(`[acceptance] known-basis write-off has localized loading, empty, error and success states at 390 px in ${locale}`, async ({ page }) => {
    const s = await setup(page, `wo${locale}`);
    await s.trade('buy', 40, '3', '3000');
    const m = messages(locale);
    await page.evaluate(value => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    await page.goto('/app/investments');
    if (locale === 'en') await useDarkTheme(page);
    const action = page.getByRole('button', { name: m.investments_record_write_off, exact: true });
    await action.focus();
    await page.keyboard.press('Enter');
    const dialog = page.getByRole('dialog', { name: m.investments_write_off_title });
    await expect(dialog.locator('#write-off-date')).toBeFocused();
    await expect(dialog).toContainText(m.investments_write_off_choose_date);
    // Before any position exists, the date remains editable for recovery. The
    // worker's book is shared with other specs (the #266 reference book holds
    // positions from 2020), so the empty date precedes every seeded holding.
    await dialog.locator('#write-off-date').fill('2001-01-02');
    await expect(dialog).toContainText(m.investments_write_off_empty);

    let release!: () => void;
    const wait = new Promise<void>(resolve => { release = resolve; });
    await page.route('**/api/v1/investments/dated-holdings?*', async route => {
      await wait;
      await route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ error: { code: 'VALIDATION_FAILED', message: 'test refusal' } }) });
    }, { times: 2 });
    await dialog.locator('#write-off-date').fill(ago(10));
    await expect(dialog).toContainText(m.investments_loading);
    release();
    await expect(dialog.getByRole('alert')).toBeVisible();
    await dialog.locator('#write-off-date').fill(todayISO());
    await dialog.locator('#write-off-holding').selectOption(s.key);
    // Editable amounts parse in the active locale (#100).
    await dialog.locator('#write-off-quantity').fill(new Intl.NumberFormat(locale).format(1.25));
    const previewButton = dialog.getByRole('button', { name: m.investments_write_off_preview, exact: true });
    await expect(previewButton).toBeDisabled(); // mandatory cause
    await dialog.locator('#write-off-reason').fill('Issuer liquidated');
    await previewButton.focus();
    await page.keyboard.press('Enter');
    const result = dialog.getByRole('region', { name: m.investments_write_off_preview_title });
    await expect(result).toContainText(new Intl.NumberFormat(locale, { minimumFractionDigits: 2 }).format(-12.5));
    expect(await gains(page, s)).toEqual([]); // previews did not post
    expect((await new AxeBuilder({ page }).include('dialog').analyze()).violations).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);

    // Editing a reviewed field invalidates that preview.
    await dialog.locator('#write-off-reason').fill('Issuer permanently liquidated');
    await expect(result).toBeHidden();
    await previewButton.click();
    const responsePromise = page.waitForResponse(response => response.url().endsWith('/investments/write-off') && response.request().method() === 'POST');
    await dialog.getByRole('button', { name: m.investments_record_write_off, exact: true }).focus();
    await page.keyboard.press('Enter');
    const response = await responsePromise;
    expect(response.status()).toBe(201);
    const payload = response.request().postDataJSON();
    expect(payload).not.toHaveProperty('cash_account_id');
    expect(payload).not.toHaveProperty('cash_amount_value');
    const saved = await response.json();
    const postings = saved.transaction.journal_entries.flatMap((entry: { postings: Array<{ account_id: number }> }) => entry.postings);
    expect(postings).toHaveLength(2);
    expect(postings.some((posting: { account_id: number }) => posting.account_id === s.cash.id)).toBe(false);
    await expect(dialog).toBeHidden();
    await expect(action).toBeFocused();
    await expect(page.getByRole('status').filter({ hasText: m.investments_write_off_success })).toBeVisible();
    expect(await gains(page, s)).toEqual(['-1250']);
    // The already mounted portfolio reflects the invalidated holding read.
    await expect(page.locator('tbody tr').filter({ hasText: s.name })).toContainText(new Intl.NumberFormat(locale).format(1.75));
  });
}

test('[acceptance] specific-lot write-off uses the elected dated lot at 390 px', async ({ page }) => {
  const s = await setup(page, 'wols');
  await s.trade('buy', 40, '2', '2000');
  await s.trade('buy', 30, '2', '4000');
  const dialog = await open(page, s, 20);
  await dialog.locator('#write-off-quantity').fill('1');
  await dialog.locator('#write-off-method').selectOption('specific_lot');
  const lots = dialog.locator('input[id^="write-off-lot-"]');
  await expect(lots).toHaveCount(2);
  await lots.last().fill('0.5');
  await dialog.getByRole('button', { name: 'Preview write-off', exact: true }).click();
  await expect(dialog.getByRole('alert')).toContainText('must equal');
  await lots.last().fill('1');
  await dialog.getByRole('button', { name: 'Preview write-off', exact: true }).click();
  await expect(dialog.getByRole('region', { name: 'Write-off preview' })).toContainText('-20.00');
  await dialog.getByRole('button', { name: 'Record write-off', exact: true }).click();
  await expect(dialog).toBeHidden();
  expect(await gains(page, s)).toEqual(['-2000']);
});

test('[acceptance] backdated write-off discloses later gains and recovers a stale acknowledgement at 390 px', async ({ page }) => {
  const s = await setup(page, 'wobd');
  await s.trade('buy', 40, '3', '3000');
  await s.trade('buy', 30, '3', '4500');
  await s.trade('sell', 5, '3', '6000');
  const dialog = await open(page, s, 20);
  await dialog.locator('#write-off-quantity').fill('2');
  await dialog.getByRole('button', { name: 'Preview write-off', exact: true }).click();
  await expect(dialog.getByRole('region', { name: 'Write-off preview' })).toContainText('-20.00');
  await dialog.getByRole('button', { name: 'Record write-off', exact: true }).click();
  const review = page.getByRole('alertdialog');
  await expect(review).toContainText('basis 30.00 → 40.00');
  await expect(review).toContainText('gain 30.00 → 20.00');
  await expect(review.getByRole('button', { name: 'Go back' })).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(review).toBeHidden();
  await expect(dialog.getByRole('button', { name: 'Record write-off', exact: true })).toBeFocused();
  expect(await gains(page, s)).toEqual(['3000']);

  // Change the accepted token to prove the real writer refuses it, then let
  // the form recover by showing the current preview and accepting anew.
  await page.route('**/api/v1/investments/write-off', async route => {
    const body = route.request().postDataJSON();
    await route.continue({ postData: JSON.stringify({ ...body, gain_impact_acknowledgement: 'stale-test-token' }) });
  }, { times: 1 });
  await dialog.getByRole('button', { name: 'Record write-off', exact: true }).click();
  await review.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(review.getByRole('status')).toBeVisible();
  expect(await gains(page, s)).toEqual(['3000']);
  await review.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(dialog).toBeHidden();
  expect(await gains(page, s)).toEqual(['-2000', '2000']);
});

test('[acceptance] a write-off of a holding sold since names the refused dependency at 390 px', async ({ page }) => {
  const s = await setup(page, 'worf');
  await s.trade('buy', 40, '3', '3000');
  await s.trade('sell', 5, '3', '6000');
  const dialog = await open(page, s, 20);
  await expect(dialog.locator('#write-off-available')).toContainText('3');
  await dialog.locator('#write-off-quantity').fill('2');
  const refused = page.waitForResponse(response => response.url().endsWith('/write-off/preview'));
  await dialog.getByRole('button', { name: 'Preview write-off', exact: true }).click();
  expect((await (await refused).json()).error.code).toBe('INVESTMENT_SALE_DEPENDENCY');
  await expect(dialog.getByRole('alert')).toContainText('later sale');
  await expect(dialog.getByRole('button', { name: 'Record write-off', exact: true })).toHaveCount(0);
  expect(await gains(page, s)).toEqual(['3000']);
  await page.keyboard.press('Escape');
  await expect(dialog).toBeHidden();
});

test('[acceptance] write-off reviews and explicitly invalidates a holding checkpoint at 390 px', async ({ page }) => {
  const s = await setup(page, 'worc');
  await s.trade('buy', 40, '3', '3000');
  await page.goto('/app/ledger/reconcile');
  await page.getByLabel('Account', { exact: true }).selectOption(String(s.holding.id));
  await page.getByLabel('Statement closing date').fill(todayISO());
  await page.getByLabel('Statement ending balance').fill('3');
  await page.getByRole('button', { name: 'Start reconciling' }).click();
  await page.getByRole('checkbox').first().check();
  await page.getByLabel('Reason', { exact: true }).fill('Synthetic holding statement');
  await page.getByRole('button', { name: 'Finish reconciliation' }).click();
  await expect(page.getByText('Reconciliation complete')).toBeVisible();
  const checkpointCount = async () => {
    const result = await apiJSON<{ checkpoints: Array<{ status: string }> }>(page, 'GET', `/api/v1/accounts/${s.holding.id}/reconciliation-checkpoints`);
    return result.checkpoints.filter(checkpoint => checkpoint.status === 'active').length;
  };
  const dialog = await open(page, s, 10);
  await dialog.locator('#write-off-quantity').fill('1');
  await dialog.getByRole('button', { name: 'Preview write-off', exact: true }).click();
  await dialog.getByRole('button', { name: 'Record write-off', exact: true }).click();
  const review = page.getByRole('alertdialog');
  await expect(review).toContainText(s.holding.name);
  await review.getByRole('button', { name: 'Go back' }).click();
  expect(await checkpointCount()).toBe(1);
  expect(await gains(page, s)).toEqual([]);
  await dialog.getByRole('button', { name: 'Record write-off', exact: true }).click();
  await review.getByRole('button', { name: 'Proceed and invalidate' }).click();
  await expect(dialog).toBeHidden();
  expect(await checkpointCount()).toBe(0);
});

for (const locale of translationLocales) {
  test(`[acceptance] unknown-basis write-off stays unresolved at 390 px in ${locale}`, async ({ page }) => {
    const s = await setup(page, `woun${locale}`);
    await apiJSON(page, 'POST', '/api/v1/investments/transfers/external/in', s.csrfToken, {
      effective_on: ago(40), holding_account_id: s.holding.id, commodity_id: s.instrument.commodity_id,
      quantity_value: '2', quantity_scale: 0, cost_commodity_id: s.currencyID, basis_knowledge: 'unknown'
    });
    const m = messages(locale);
    const dialog = await open(page, s, 0, locale);
    await dialog.locator('#write-off-quantity').fill('2');
    await dialog.getByRole('button', { name: m.investments_write_off_preview, exact: true }).click();
    await expect(dialog.getByRole('region', { name: m.investments_write_off_preview_title })).toContainText(m.investments_write_off_unresolved_help);
    await dialog.getByRole('button', { name: m.investments_record_write_off, exact: true }).click();
    await expect(dialog).toBeHidden();
    expect(await gains(page, s)).toEqual([null]);
  });
}

test('[acceptance] mixed-currency write-off consumes the selected dated position at 390 px', async ({ page }) => {
  const s = await setup(page, 'womc');
  await s.trade('buy', 40, '3', '3000');
  const eurID = await ensureCurrency(page, s.csrfToken, 'EUR', 'Euro');
  await apiJSON(page, 'POST', '/api/v1/investments/transfers/external/in', s.csrfToken, {
    effective_on: ago(30), holding_account_id: s.holding.id, commodity_id: s.instrument.commodity_id,
    quantity_value: '2', quantity_scale: 0, cost_commodity_id: eurID, basis_knowledge: 'known', carried_basis_value: '2000', carried_basis_scale: 2
  });
  const dialog = await open(page, s, 20);
  await dialog.locator('#write-off-quantity').fill('1');
  const preview = page.waitForResponse(response => response.url().endsWith('/write-off/preview'));
  await dialog.getByRole('button', { name: 'Preview write-off', exact: true }).click();
  const response = await preview;
  expect(response.status()).toBe(200);
  expect(response.request().postDataJSON().cost_commodity_id).toBe(s.currencyID);
  const body = await response.json();
  expect(body.disposal_decision.cost_commodity_id).toBe(s.currencyID);
  await expect(dialog.getByRole('region', { name: 'Write-off preview' })).toContainText('-10.00 USD');
  expect(await gains(page, s)).toEqual([]);
  const commit = page.waitForResponse(response => response.url().endsWith('/write-off'));
  await dialog.getByRole('button', { name: 'Record write-off', exact: true }).click();
  expect((await commit).request().postDataJSON().cost_commodity_id).toBe(s.currencyID);
  await expect(dialog).toBeHidden();
  expect(await gains(page, s)).toEqual(['-1000']);
  const dated = await apiJSON<{ holdings: Array<{ account_id: number; commodity_id: number; cost_commodity_id: number; quantity_value: string }> }>(page, 'GET', `/api/v1/investments/dated-holdings?as_of=${ago(20)}`);
  const positions = dated.holdings.filter(position => position.account_id === s.holding.id && position.commodity_id === s.instrument.commodity_id);
  expect(positions.find(position => position.cost_commodity_id === s.currencyID)?.quantity_value).toBe('2');
  expect(positions.find(position => position.cost_commodity_id === eurID)?.quantity_value).toBe('2');
  // Select the other position, including its own specific-lot election.
  const euroDialog = await open(page, s, 10);
  await euroDialog.locator('#write-off-holding').selectOption(`${s.holding.id}:${s.instrument.commodity_id}:${eurID}`);
  await euroDialog.locator('#write-off-quantity').fill('1');
  await euroDialog.locator('#write-off-method').selectOption('specific_lot');
  await euroDialog.locator('input[id^="write-off-lot-"]').fill('1');
  await euroDialog.getByRole('button', { name: 'Preview write-off', exact: true }).click();
  await expect(euroDialog.getByRole('region', { name: 'Write-off preview' })).toContainText('-10.00 EUR');
  expect(await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)).toBe(false);
  const accessibility = await new AxeBuilder({ page }).include('dialog[open]').analyze();
  expect(accessibility.violations).toEqual([]);
  await euroDialog.getByRole('button', { name: 'Record write-off', exact: true }).click();
  await expect(euroDialog).toBeHidden();
  expect(await gains(page, s)).toEqual(['-1000', '-1000']);
});
