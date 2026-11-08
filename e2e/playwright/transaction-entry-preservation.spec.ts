import { readFileSync } from 'node:fs';
import { expect, test, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { createCashAccount, ensureCurrency, readyForLedger } from './support/ledger';
import { daysFromTodayISO, todayISO } from './support/dates';
import { expectNoAccessibilityViolations, focusIsInside } from './support/a11y';
import { openInvestmentActivity } from './support/investment-entry';

// #508: New transaction keeps one shared context (main date, memo, holding,
// instrument, settlement choice) and a working copy per activity for as long
// as the entry is open. Buy ↔ Sell carry quantity and unit price only; each
// keeps its own charges and recomputes its own cash. Switching never posts,
// a review belongs to the request it reviewed, and closing asks only when
// something entered would be lost.

async function setup(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const date = daysFromTodayISO(-10);
  const checking = await createCashAccount(page, csrfToken, `Keep bank ${suffix}`, currencyID, date);
  const broker = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Keep broker ${suffix}`, account_class: 'asset', account_kind: 'brokerage',
    default_commodity_id: currencyID, allows_postings: false, opened_on: date, effective_from: date
  });
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: `KP${suffix}`, instrument_type: 'stock', display_name: `Keep shares ${suffix}`,
    quantity_scale: 0, price_scale: 2, quote_commodity_id: currencyID, effective_from: date
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Keep holding ${suffix}`, parent_account_id: broker.id,
    opened_on: date, effective_from: date
  });
  const cash = await apiJSON<{ id: number; name: string }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Keep broker cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    parent_account_id: broker.id, default_commodity_id: currencyID,
    allows_postings: true, opened_on: date, effective_from: date
  });
  return { csrfToken, currencyID, checking, broker, holding, cash, instrument, suffix };
}

/** What switching must never change: the ledger's transactions and the holding's lots. */
async function ledgerState(page: Page, holdingID: number) {
  const transactions = await apiJSON<{ transactions: unknown[] }>(page, 'GET', '/api/v1/transactions?limit=200');
  const lots = await apiJSON<unknown>(page, 'GET', `/api/v1/investments/lots?account_id=${holdingID}`);
  return JSON.stringify([transactions.transactions, lots]);
}

/** Every API write the page sends; previews and impact reads are simulated and rolled back. */
function recordWrites(page: Page): string[] {
  const writes: string[] = [];
  page.on('request', (request) => {
    const url = new URL(request.url());
    if (request.method() !== 'GET' && url.pathname.startsWith('/api/v1/')) writes.push(`${request.method()} ${url.pathname}`);
  });
  return writes;
}
const simulated = (write: string) => /\/preview$|reconciliation-impact$/.test(write);

test('journeys 4, 5 and 7: Buy → Sell → Buy keeps the context, carries quantity and unit price, and keeps each side’s charges', async ({ page }) => {
  const s = await setup(page);
  const before = await ledgerState(page, s.holding.id);
  const writes = recordWrites(page);
  await page.goto(`/app/ledger/accounts/${s.holding.id}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  const entry = page.getByRole('region', { name: 'New transaction', exact: true });
  const tradeDate = daysFromTodayISO(-3);
  const memo = `Keep memo ${s.suffix}`;

  await openInvestmentActivity(page, 'Buy');
  let form = page.getByRole('dialog').filter({ has: page.locator('#buy-quantity') });
  await form.getByLabel('Date', { exact: true }).fill(tradeDate);
  await form.locator('#buy-memo').fill(memo);
  await form.locator('#buy-quantity').fill('10');
  await expect(form.locator('#buy-cash-account')).toHaveValue(String(s.cash.id));
  await form.locator('#buy-unit-price').fill('12.50');
  await expect(form.locator('#buy-cash-amount')).toHaveValue('125.00');
  await expect(form.locator('#buy-cash-amount')).toHaveAttribute('readonly', '');
  await form.getByText('Exact trade amounts', { exact: true }).click();
  await form.getByLabel('Record gross amount and charges').check();
  await form.getByRole('button', { name: 'Add charge', exact: true }).click();
  await form.getByLabel('Amount', { exact: true }).fill('2');
  await expect(form.locator('#trade-gross-buy')).toHaveValue('125.00');
  // A buy pays the commission on top.
  await expect(form.locator('#buy-cash-amount')).toHaveValue('127.00');
  await form.getByRole('button', { name: 'Cancel', exact: true }).click();

  await openInvestmentActivity(page, 'Sell');
  form = page.getByRole('dialog').filter({ has: page.locator('#sell-quantity') });
  await expect(form.getByLabel('Date', { exact: true })).toHaveValue(tradeDate);
  await expect(form.locator('#sell-memo')).toHaveValue(memo);
  await expect(form.locator('#sell-holding-account')).toHaveValue(String(s.holding.id));
  await expect(form.locator('#sell-quantity')).toHaveValue('10');
  await expect(form.locator('#sell-unit-price')).toHaveValue('12.50');
  // Buy's commission and its net are not Sell's: Sell starts with no charges.
  await expect(form.getByLabel('Record gross amount and charges')).not.toBeChecked();
  await expect(form.locator('#sell-cash-amount')).toHaveValue('125.00');
  // The buy was never saved, so there is nothing to sell: the preview refuses.
  await expect(form.locator('#sell-preview-error')).toBeVisible();
  await expect(form.getByRole('button', { name: 'Confirm sell', exact: true })).toBeDisabled();
  const sellDate = daysFromTodayISO(-2);
  await form.getByLabel('Date', { exact: true }).fill(sellDate);
  await form.getByText('Exact trade amounts', { exact: true }).click();
  await form.getByLabel('Record gross amount and charges').check();
  await form.getByRole('button', { name: 'Add charge', exact: true }).click();
  await form.getByLabel('Amount', { exact: true }).fill('1');
  // A sale deducts its commission from the proceeds.
  await expect(form.locator('#sell-cash-amount')).toHaveValue('124.00');
  await form.getByRole('button', { name: 'Cancel', exact: true }).click();

  await openInvestmentActivity(page, 'Buy');
  form = page.getByRole('dialog').filter({ has: page.locator('#buy-quantity') });
  // Journey 5: the date changed in Sell is the date Buy shows.
  await expect(form.getByLabel('Date', { exact: true })).toHaveValue(sellDate);
  await expect(form.locator('#buy-quantity')).toHaveValue('10');
  await expect(form.locator('#buy-unit-price')).toHaveValue('12.50');
  await expect(form.getByLabel('Record gross amount and charges')).toBeChecked();
  await expect(form.getByLabel('Amount', { exact: true })).toHaveValue('2');
  await expect(form.locator('#buy-cash-amount')).toHaveValue('127.00');
  // Buy ↔ Sell keep the price: a new one recomputes Buy's cash with its own charge.
  await form.locator('#buy-unit-price').fill('13');
  await expect(form.locator('#buy-cash-amount')).toHaveValue('132.00');
  await form.getByRole('button', { name: 'Cancel', exact: true }).click();

  // The cash tabs see the same date and memo, and the Activity is remembered.
  await entry.getByRole('tab', { name: 'Withdrawal', exact: true }).click();
  await expect(entry.getByLabel('Date', { exact: true })).toHaveValue(sellDate);
  await expect(entry.getByLabel('Description', { exact: true })).toHaveValue(memo);
  await entry.getByRole('tab', { name: 'Investment', exact: true }).click();
  await expect(page.locator('#investment-activity')).toHaveValue('buy');
  await openInvestmentActivity(page, 'Sell');
  await expect(page.locator('#sell-unit-price')).toHaveValue('13');
  await expect(page.locator('#sell-cash-amount')).toHaveValue('129.00');
  await page.getByRole('dialog').filter({ has: page.locator('#sell-quantity') }).getByRole('button', { name: 'Cancel', exact: true }).click();

  // Switching mode, activity and account posts nothing.
  expect(writes.filter((write) => !simulated(write))).toEqual([]);
  expect(await ledgerState(page, s.holding.id)).toBe(before);
});

test('journey 8: a review belongs to the request it reviewed; saving after a change asks again', async ({ page }) => {
  const s = await setup(page);
  await apiJSON(page, 'POST', '/api/v1/investments/buy', s.csrfToken, {
    transaction_date: daysFromTodayISO(-5), commodity_id: s.instrument.commodity_id, holding_account_id: s.holding.id,
    cash_account_id: s.cash.id, quantity_value: '2', quantity_scale: 0,
    cash_amount_value: '2000', cash_amount_scale: 2, cash_commodity_id: s.currencyID
  });
  await page.goto('/app/ledger/reconcile');
  await page.getByLabel('Account', { exact: true }).selectOption(String(s.cash.id));
  await page.getByLabel('Statement closing date').fill(todayISO());
  await page.getByLabel('Statement ending balance').fill('-20.00');
  await page.getByRole('button', { name: 'Start reconciling' }).click();
  await page.getByRole('checkbox').first().check();
  await page.getByLabel('Reason', { exact: true }).fill(`Keep statement ${s.suffix}`);
  await page.getByRole('button', { name: 'Finish reconciliation' }).click();
  await expect(page.getByText('Reconciliation complete')).toBeVisible();

  const impacts: string[] = [];
  const recorded: Record<string, unknown>[] = [];
  page.on('request', (request) => {
    const path = new URL(request.url()).pathname;
    if (request.method() !== 'POST') return;
    if (path.endsWith('reconciliation-impact')) impacts.push(path);
    if (path === '/api/v1/investments/buy') recorded.push(request.postDataJSON() as Record<string, unknown>);
  });

  await page.goto(`/app/ledger/accounts/${s.holding.id}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  await openInvestmentActivity(page, 'Buy');
  let form = page.getByRole('dialog').filter({ has: page.locator('#buy-quantity') });
  await form.getByLabel('Date', { exact: true }).fill(daysFromTodayISO(-1));
  await form.locator('#buy-quantity').fill('1');
  await form.locator('#buy-unit-price').fill('10');
  await form.getByRole('button', { name: 'Record buy', exact: true }).click();
  const review = page.getByRole('alertdialog');
  await expect(review).toContainText(s.cash.name);
  await review.getByRole('button', { name: 'Go back' }).click();
  await expect(review).toBeHidden();
  await form.getByRole('button', { name: 'Cancel', exact: true }).click();

  // Another activity in between; the Buy comes back with its fields, not its review.
  await openInvestmentActivity(page, 'Sell');
  await page.getByRole('dialog').filter({ has: page.locator('#sell-quantity') }).getByRole('button', { name: 'Cancel', exact: true }).click();
  await openInvestmentActivity(page, 'Buy');
  form = page.getByRole('dialog').filter({ has: page.locator('#buy-quantity') });
  await expect(page.getByRole('alertdialog')).toHaveCount(0);
  await form.getByRole('button', { name: 'Record buy', exact: true }).click();
  await expect(review).toContainText(s.cash.name);
  expect(impacts).toHaveLength(2);
  await review.getByRole('button', { name: 'Go back' }).click();

  // A review of one account never authorizes another: paying from the
  // unreconciled bank asks nothing and overrides nothing.
  await form.locator('#buy-cash-account').selectOption(String(s.checking.id));
  await form.getByRole('button', { name: 'Record buy', exact: true }).click();
  await expect(form).toBeHidden();
  expect(impacts).toHaveLength(3);
  expect(recorded).toHaveLength(1);
  expect(recorded[0].cash_account_id).toBe(s.checking.id);
  expect(recorded[0].reconciliation_override_checkpoint_ids).toBeUndefined();
  const checkpoints = await apiJSON<{ checkpoints: Array<{ status: string }> }>(page, 'GET', `/api/v1/accounts/${s.cash.id}/reconciliation-checkpoints`);
  expect(checkpoints.checkpoints.filter((checkpoint) => checkpoint.status === 'active')).toHaveLength(1);
});

test('while an investment saves, tabs and activity are locked', async ({ page }) => {
  const s = await setup(page);
  let release!: () => void;
  const held = new Promise<void>((resolve) => { release = resolve; });
  await page.route('**/api/v1/investments/buy', async (route) => {
    await held;
    await route.continue();
  });
  await page.goto(`/app/ledger/accounts/${s.holding.id}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  const entry = page.getByRole('region', { name: 'New transaction', exact: true });
  await openInvestmentActivity(page, 'Buy');
  const form = page.getByRole('dialog').filter({ has: page.locator('#buy-quantity') });
  await form.locator('#buy-quantity').fill('1');
  await form.locator('#buy-unit-price').fill('10');
  await form.getByRole('button', { name: 'Record buy', exact: true }).click();
  await expect(form.getByRole('button', { name: 'Recording…' })).toBeVisible();
  for (const tab of ['Withdrawal', 'Deposit', 'Transfer', 'Investment']) {
    await expect(entry.getByRole('tab', { name: tab, exact: true })).toBeDisabled();
  }
  await expect(page.locator('#investment-activity')).toBeDisabled();
  release();
  await expect(form).toBeHidden();
});

test('closing or leaving asks only when entered work would be lost', async ({ page }) => {
  const s = await setup(page);
  await page.goto(`/app/ledger/accounts/${s.checking.id}/register`);
  const entry = page.getByRole('region', { name: 'New transaction', exact: true });
  const discard = page.getByRole('alertdialog', { name: 'Discard this transaction?' });

  // Nothing entered: Cancel closes at once.
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  await entry.getByRole('button', { name: 'Cancel', exact: true }).last().click();
  await expect(entry).toBeHidden();
  await expect(discard).toHaveCount(0);

  // An amount entered: Cancel asks, and Keep editing keeps it.
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  await entry.getByLabel('Amount', { exact: true }).fill('12.34');
  await entry.getByRole('button', { name: 'Cancel', exact: true }).last().click();
  await expect(discard).toBeVisible();
  await expect(discard.getByRole('button', { name: 'Keep editing' })).toBeFocused();
  await discard.getByRole('button', { name: 'Keep editing' }).click();
  await expect(entry.getByLabel('Amount', { exact: true })).toHaveValue('12.34');

  // Leaving the page asks too; Keep editing stays here.
  await page.getByRole('link', { name: 'Home', exact: true }).first().click();
  await expect(discard).toBeVisible();
  await discard.getByRole('button', { name: 'Keep editing' }).click();
  await expect(page).toHaveURL(new RegExp(`/accounts/${s.checking.id}/register`));
  await page.getByRole('link', { name: 'Home', exact: true }).first().click();
  await discard.getByRole('button', { name: 'Discard', exact: true }).click();
  await expect(page).toHaveURL(/\/app\/?$/);

  // An investment working copy alone is work too; Escape in the shared dialog asks.
  await page.getByRole('button', { name: 'New transaction', exact: true }).first().click();
  const dialog = page.getByRole('dialog', { name: 'New transaction', exact: true });
  await dialog.getByRole('tab', { name: 'Investment', exact: true }).click();
  await openInvestmentActivity(page, 'Split');
  const split = page.locator('form').filter({ has: page.locator('#split-date') });
  await split.locator('#split-new-units').fill('2');
  await split.getByRole('button', { name: 'Cancel', exact: true }).click();
  await expect(split).toBeHidden();
  await page.keyboard.press('Escape');
  await expect(discard).toBeVisible();
  await discard.getByRole('button', { name: 'Discard', exact: true }).click();
  await expect(dialog).toBeHidden();
});

for (const theme of ['light', 'dark']) {
  test(`journey 17: unit price and discard on a phone in ${theme}`, async ({ page }) => {
    const s = await setup(page);
    await page.setViewportSize({ width: 390, height: 844 });
    await page.evaluate((value) => localStorage.setItem('rekenraam-theme', value), theme);
    await page.goto(`/app/ledger/accounts/${s.holding.id}/register`);
    await page.getByRole('button', { name: 'New transaction', exact: true }).click();
    await openInvestmentActivity(page, 'Short sale');
    const form = page.getByRole('dialog').filter({ has: page.locator('#short-open-quantity') });
    await form.locator('#short-open-quantity').fill('3');
    await form.locator('#short-open-unit-price').fill('1.005');
    await expect(form.getByRole('status').filter({ hasText: 'more decimals' })).toContainText('3.015');
    await form.locator('#short-open-unit-price').fill('1.50');
    await expect(form.locator('#short-open-cash-amount')).toHaveValue('4.50');
    await expectNoAccessibilityViolations(page, `unit price ${theme}`);
    await form.getByRole('button', { name: 'Cancel', exact: true }).click();
    // Short cover carries the short pair's quantity and price, never Buy's.
    await openInvestmentActivity(page, 'Short cover');
    await expect(page.locator('#short-cover-quantity')).toHaveValue('3');
    await expect(page.locator('#short-cover-unit-price')).toHaveValue('1.50');
    await page.getByRole('dialog').filter({ has: page.locator('#short-cover-quantity') }).getByRole('button', { name: 'Cancel', exact: true }).click();
    await openInvestmentActivity(page, 'Buy');
    await expect(page.locator('#buy-quantity')).toHaveValue('');
    await page.getByRole('dialog').filter({ has: page.locator('#buy-quantity') }).getByRole('button', { name: 'Cancel', exact: true }).click();

    await page.getByRole('region', { name: 'New transaction', exact: true }).getByRole('button', { name: 'Cancel', exact: true }).last().click();
    const discard = page.getByRole('alertdialog', { name: 'Discard this transaction?' });
    await expect(discard).toBeVisible();
    await expect.poll(() => focusIsInside(discard)).toBe(true);
    await expectNoAccessibilityViolations(page, `discard ${theme}`);
    await page.keyboard.press('Escape');
    await expect(discard).toBeHidden();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  });
}

const catalog = (locale: string) => ({
  ...JSON.parse(readFileSync(new URL('../../frontend/messages/settings/en.json', import.meta.url), 'utf8')),
  ...JSON.parse(readFileSync(new URL(`../../frontend/messages/settings/${locale}.json`, import.meta.url), 'utf8'))
}) as Record<string, string>;

for (const locale of ['en', 'nl', 'ru', 'de', 'es', 'fr']) {
  test(`journey 17: discard confirmation in ${locale}`, async ({ page }) => {
    const s = await setup(page);
    const messages = catalog(locale);
    await page.evaluate((value) => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    await page.goto(`/app/ledger/accounts/${s.checking.id}/register`);
    await page.getByRole('button', { name: messages.transactions_form_create_title, exact: true }).click();
    const entry = page.getByRole('region', { name: messages.transactions_form_create_title, exact: true });
    await entry.getByLabel(messages.transactions_field_description, { exact: true }).fill('x');
    await entry.getByRole('button', { name: messages.transactions_form_cancel, exact: true }).last().click();
    const discard = page.getByRole('alertdialog', { name: messages.transactions_entry_discard_title });
    await expect(discard).toBeVisible();
    await expect(discard.getByRole('button', { name: messages.transactions_entry_discard_keep })).toBeFocused();
    await discard.getByRole('button', { name: messages.transactions_entry_discard_confirm, exact: true }).click();
    await expect(entry).toBeHidden();
  });
}

test('a phone panel’s Escape keeps a filled entry until the discard is confirmed (#521 review)', async ({ page }) => {
  const s = await setup(page);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`/app/ledger/accounts/${s.checking.id}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  const entry = page.getByRole('region', { name: 'New transaction', exact: true });
  await entry.getByLabel('Amount', { exact: true }).fill('12.34');
  await page.keyboard.press('Escape');
  const discard = page.getByRole('alertdialog', { name: 'Discard this transaction?' });
  await expect(discard).toBeVisible();
  await expect(entry.getByLabel('Amount', { exact: true })).toHaveValue('12.34');
  await page.keyboard.press('Escape');
  await expect(discard).toBeHidden();
  await expect(entry.getByLabel('Amount', { exact: true })).toHaveValue('12.34');
  await page.keyboard.press('Escape');
  await discard.getByRole('button', { name: 'Discard', exact: true }).click();
  await expect(entry).toBeHidden();
});

test('an explicitly chosen settlement account counts as entered work (#521 review)', async ({ page }) => {
  const s = await setup(page);
  await page.goto(`/app/ledger/accounts/${s.holding.id}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  await openInvestmentActivity(page, 'Buy');
  const form = page.getByRole('dialog').filter({ has: page.locator('#buy-quantity') });
  await form.locator('#buy-cash-account').selectOption(String(s.checking.id));
  await form.getByRole('button', { name: 'Cancel', exact: true }).click();
  const entry = page.getByRole('region', { name: 'New transaction', exact: true });
  await entry.getByRole('button', { name: 'Cancel', exact: true }).last().click();
  await expect(page.getByRole('alertdialog', { name: 'Discard this transaction?' })).toBeVisible();
});

test('a multi-currency position selection survives closing and reopening its activity (#521 review)', async ({ page }) => {
  const s = await setup(page);
  const eur = await ensureCurrency(page, s.csrfToken, 'EUR', 'Euro');
  const eurCash = await createCashAccount(page, s.csrfToken, `EUR cash ${s.suffix}`, eur, daysFromTodayISO(-10));
  for (const [cash, currency] of [[s.cash.id, s.currencyID], [eurCash.id, eur]]) {
    await apiJSON(page, 'POST', '/api/v1/investments/buy', s.csrfToken, {
      transaction_date: daysFromTodayISO(-5), commodity_id: s.instrument.commodity_id, holding_account_id: s.holding.id,
      cash_account_id: cash, quantity_value: '2', quantity_scale: 0,
      cash_amount_value: '2000', cash_amount_scale: 2, cash_commodity_id: currency
    });
  }
  await page.goto(`/app/ledger/accounts/${s.holding.id}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  await openInvestmentActivity(page, 'Shares out');
  const form = page.getByRole('dialog').filter({ has: page.locator('#external-transfer-out-source') });
  const chosen = `${s.holding.id}:${s.instrument.commodity_id}:${eur}`;
  await form.locator('#external-transfer-out-source').selectOption(chosen);
  await form.getByRole('button', { name: 'Cancel', exact: true }).click();
  await openInvestmentActivity(page, 'Shares out');
  await expect(page.locator('#external-transfer-out-source')).toHaveValue(chosen);
});

test('a cost currency chosen in Shares in is the one Reinvested dividend starts on (#521 review)', async ({ page }) => {
  const s = await setup(page);
  const eur = await ensureCurrency(page, s.csrfToken, 'EUR', 'Euro');
  await page.goto(`/app/ledger/accounts/${s.holding.id}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  await openInvestmentActivity(page, 'Shares in');
  const form = page.getByRole('dialog').filter({ has: page.locator('#transfer-in-currency') });
  await form.locator('#transfer-in-currency').selectOption(String(eur));
  await form.getByRole('button', { name: 'Cancel', exact: true }).click();
  await openInvestmentActivity(page, 'Reinvested dividend');
  await expect(page.locator('#div-cost-currency')).toHaveValue(String(eur));
});

test('account selections are locked while the investment save is pending (#521 review)', async ({ page }) => {
  const s = await setup(page);
  let release!: () => void;
  const held = new Promise<void>((resolve) => { release = resolve; });
  await page.route('**/api/v1/investments/buy', async (route) => {
    await held;
    await route.continue();
  });
  await page.goto(`/app/ledger/accounts/${s.holding.id}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  await openInvestmentActivity(page, 'Buy');
  const form = page.getByRole('dialog').filter({ has: page.locator('#buy-quantity') });
  await form.locator('#buy-quantity').fill('1');
  await form.locator('#buy-unit-price').fill('10');
  await form.getByRole('button', { name: 'Record buy', exact: true }).click();
  await expect(form.getByRole('button', { name: 'Recording…' })).toBeVisible();
  try {
    await expect(form.locator('#buy-cash-account')).toBeDisabled();
    await expect(form.locator('#buy-holding-account')).toBeDisabled();
  } finally {
    release();
  }
});

test('an income account chosen alone is work to lose (#521 re-review)', async ({ page }) => {
  const s = await setup(page);
  await page.goto(`/app/ledger/accounts/${s.holding.id}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  await openInvestmentActivity(page, 'Cash dividend');
  const form = page.getByRole('dialog').filter({ has: page.locator('#div-income-account') });
  const value = await form.locator('#div-income-account option').evaluateAll(options => options.map(o => (o as HTMLOptionElement).value).find(v => v !== '')!);
  expect(value).toBeTruthy();
  await form.locator('#div-income-account').selectOption(value);
  await form.getByRole('button', { name: 'Cancel', exact: true }).click();
  const entry = page.getByRole('region', { name: 'New transaction', exact: true });
  await entry.getByRole('button', { name: 'Cancel', exact: true }).last().click();
  await expect(page.getByRole('alertdialog', { name: 'Discard this transaction?' })).toBeVisible();
});

async function seedBuy(s: Awaited<ReturnType<typeof setup>>, page: Page) {
  await apiJSON(page, 'POST', '/api/v1/investments/buy', s.csrfToken, {
    transaction_date: daysFromTodayISO(-5), commodity_id: s.instrument.commodity_id, holding_account_id: s.holding.id,
    cash_account_id: s.cash.id, quantity_value: '2', quantity_scale: 0,
    cash_amount_value: '2000', cash_amount_scale: 2, cash_commodity_id: s.currencyID
  });
}

test('undoing the only typed change leaves an automatic position clean (#521 re-review)', async ({ page }) => {
  const s = await setup(page);
  await seedBuy(s, page);
  await page.goto(`/app/ledger/accounts/${s.holding.id}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  await openInvestmentActivity(page, 'Shares out');
  const form = page.getByRole('dialog').filter({ has: page.locator('#external-transfer-out-source') });
  await expect(form.locator('#external-transfer-out-source')).not.toHaveValue('');
  await form.locator('#external-transfer-out-memo').fill('temporary');
  await form.locator('#external-transfer-out-memo').fill('');
  await form.getByRole('button', { name: 'Cancel', exact: true }).click();
  const entry = page.getByRole('region', { name: 'New transaction', exact: true });
  await entry.getByRole('button', { name: 'Cancel', exact: true }).last().click();
  await expect(entry).toBeHidden();
  await expect(page.getByRole('alertdialog', { name: 'Discard this transaction?' })).toHaveCount(0);
});

test('a selected position invalidated by a new main date stays identified (#521 re-review)', async ({ page }) => {
  const s = await setup(page);
  await seedBuy(s, page);
  await page.goto(`/app/ledger/accounts/${s.holding.id}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  await openInvestmentActivity(page, 'Shares out');
  const form = page.getByRole('dialog').filter({ has: page.locator('#external-transfer-out-source') });
  const chosen = `${s.holding.id}:${s.instrument.commodity_id}:${s.currencyID}`;
  await expect(form.locator('#external-transfer-out-source')).toHaveValue(chosen);
  await form.getByRole('button', { name: 'Cancel', exact: true }).click();
  await openInvestmentActivity(page, 'Buy');
  const trade = page.getByRole('dialog').filter({ has: page.locator('#buy-quantity') });
  await trade.locator('#buy-date').fill(daysFromTodayISO(-7));
  await trade.getByRole('button', { name: 'Cancel', exact: true }).click();
  await openInvestmentActivity(page, 'Shares out');
  await expect(page.locator('#external-transfer-out-date')).toHaveValue(daysFromTodayISO(-7));
  await expect(page.locator('#external-transfer-out-source')).toHaveValue(chosen);
  await expect(form.getByRole('alert')).toBeVisible();
});

test('a posting-date-only change counts as unsaved work (#521 re-review)', async ({ page }) => {
  const s = await setup(page);
  await page.goto(`/app/ledger/accounts/${s.checking.id}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  const entry = page.getByRole('region', { name: 'New transaction', exact: true });
  await entry.getByRole('button', { name: 'Posted on another day' }).click();
  await entry.locator('input[aria-describedby="transaction-posted-on-hint"]').fill(daysFromTodayISO(-1));
  await entry.getByRole('button', { name: 'Cancel', exact: true }).last().click();
  await expect(page.getByRole('alertdialog', { name: 'Discard this transaction?' })).toBeVisible();
});

async function start(page: Page, accountID: number) {
  await page.goto(`/app/ledger/accounts/${accountID}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  return page.getByRole('region', { name: 'New transaction', exact: true });
}

async function cancelEntry(page: Page) {
  const entry = page.getByRole('region', { name: 'New transaction', exact: true });
  await entry.getByRole('button', { name: 'Cancel', exact: true }).last().click();
  await expect(page.getByRole('alertdialog', { name: 'Discard this transaction?' })).toBeVisible();
}

async function position(s: Awaited<ReturnType<typeof setup>>, page: Page, currency = s.currencyID, cash = s.cash.id) {
  await apiJSON(page, 'POST', '/api/v1/investments/buy', s.csrfToken, {
    transaction_date: daysFromTodayISO(-5), commodity_id: s.instrument.commodity_id, holding_account_id: s.holding.id,
    cash_account_id: cash, quantity_value: '2', quantity_scale: 0,
    cash_amount_value: '2000', cash_amount_scale: 2, cash_commodity_id: currency
  });
}

test('(#521 third review) choosing an instrument through autocomplete counts as owner work', async ({ page }) => {
  const s = await setup(page);
  const entry = await start(page, s.checking.id);
  await entry.getByRole('tab', { name: 'Investment', exact: true }).click();
  await openInvestmentActivity(page, 'Buy');
  const form = page.getByRole('dialog').filter({ has: page.locator('#buy-quantity') });
  await form.locator('#buy-instrument').fill(`KP${s.suffix}`);
  await form.getByRole('listbox').getByRole('option').click();
  await expect(form.locator('#buy-instrument')).toHaveValue(`Keep shares ${s.suffix}`);
  await form.getByRole('button', { name: 'Cancel', exact: true }).click();
  await cancelEntry(page);
});

test('(#521 third review) an explicitly chosen cash entry currency counts as owner work', async ({ page }) => {
  const s = await setup(page);
  const receivable = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', s.csrfToken, {
    name: `Review receivable ${s.suffix}`, account_class: 'asset', account_kind: 'receivable', allows_postings: true
  });
  const entry = await start(page, receivable.id);
  await entry.getByRole('combobox', { name: 'Currency', exact: true }).selectOption(String(s.currencyID));
  await cancelEntry(page);
});

test('(#521 third review) an explicit cost currency stays owner work after automatic position restoration', async ({ page }) => {
  const s = await setup(page);
  const eur = await ensureCurrency(page, s.csrfToken, 'EUR', 'Euro');
  const eurCash = await createCashAccount(page, s.csrfToken, `EUR cash ${s.suffix}`, eur, daysFromTodayISO(-10));
  await position(s, page);
  await position(s, page, eur, eurCash.id);
  await start(page, s.holding.id);
  await openInvestmentActivity(page, 'Shares in');
  const incoming = page.getByRole('dialog').filter({ has: page.locator('#transfer-in-currency') });
  await incoming.locator('#transfer-in-currency').selectOption(String(eur));
  await incoming.getByRole('button', { name: 'Cancel', exact: true }).click();
  await openInvestmentActivity(page, 'Shares out');
  const outgoing = page.getByRole('dialog').filter({ has: page.locator('#external-transfer-out-source') });
  await expect(outgoing.locator('#external-transfer-out-source')).toHaveValue(`${s.holding.id}:${s.instrument.commodity_id}:${eur}`);
  await outgoing.getByRole('button', { name: 'Cancel', exact: true }).click();
  await cancelEntry(page);
});

test('(#521 third review) merger cost currency is shared with a subsequent reinvestment', async ({ page }) => {
  const s = await setup(page);
  await position(s, page);
  const eur = await ensureCurrency(page, s.csrfToken, 'EUR', 'Euro');
  const eurCash = await createCashAccount(page, s.csrfToken, `Merger EUR cash ${s.suffix}`, eur, daysFromTodayISO(-10));
  await position(s, page, eur, eurCash.id);
  const target = await apiJSON<{ commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', s.csrfToken, {
    commodity_code: `NEW${s.suffix}`, instrument_type: 'stock', display_name: `Target ${s.suffix}`,
    quantity_scale: 0, price_scale: 2, quote_commodity_id: s.currencyID, effective_from: daysFromTodayISO(-10)
  });
  await start(page, s.holding.id);
  await openInvestmentActivity(page, 'Cash-and-stock merger');
  const form = page.getByRole('dialog').filter({ has: page.locator('#merger-new-instrument') });
  await form.locator('#merger-new-instrument').selectOption(String(target.commodity_id));
  await form.locator('#merger-currency').selectOption(String(eur));
  await form.getByRole('button', { name: 'Cancel', exact: true }).click();
  await openInvestmentActivity(page, 'Reinvested dividend');
  await expect(page.locator('#div-cost-currency')).toHaveValue(String(eur));
});

test('(#521 third review) keyboard instrument selection counts as owner work', async ({ page }) => {
  const s = await setup(page);
  const entry = await start(page, s.checking.id);
  await entry.getByRole('tab', { name: 'Investment', exact: true }).click();
  await openInvestmentActivity(page, 'Buy');
  const form = page.getByRole('dialog').filter({ has: page.locator('#buy-quantity') });
  const input = form.locator('#buy-instrument');
  await input.fill(`KP${s.suffix}`);
  const choice = form.getByRole('listbox').getByRole('option');
  await expect(choice).toBeVisible();
  await input.press('Tab');
  await expect(choice).toBeFocused();
  await choice.press('Enter');
  await expect(input).toHaveValue(`Keep shares ${s.suffix}`);
  await form.getByRole('button', { name: 'Cancel', exact: true }).click();
  await cancelEntry(page);
});

test('(#521 third review) unavailable lot allocations remain visible rather than becoming a valid subset', async ({ page }) => {
  const s = await setup(page);
  await apiJSON(page, 'POST', '/api/v1/investments/buy', s.csrfToken, {
    transaction_date: daysFromTodayISO(-8), commodity_id: s.instrument.commodity_id, holding_account_id: s.holding.id,
    cash_account_id: s.cash.id, quantity_value: '2', quantity_scale: 0,
    cash_amount_value: '2000', cash_amount_scale: 2, cash_commodity_id: s.currencyID
  });
  await position(s, page);
  await start(page, s.holding.id);
  await openInvestmentActivity(page, 'Shares out');
  const form = page.getByRole('dialog').filter({ has: page.locator('#external-transfer-out-source') });
  const choices = form.locator('input[id^="external-transfer-out-lot-"]');
  await expect(choices).toHaveCount(2);
  await choices.nth(0).fill('1');
  await choices.nth(1).fill('1');
  await form.getByRole('button', { name: 'Cancel', exact: true }).click();
  await openInvestmentActivity(page, 'Buy');
  const trade = page.getByRole('dialog').filter({ has: page.locator('#buy-quantity') });
  await trade.locator('#buy-date').fill(daysFromTodayISO(-6));
  await trade.getByRole('button', { name: 'Cancel', exact: true }).click();
  await openInvestmentActivity(page, 'Shares out');
  await expect(choices).toHaveCount(1);
  await expect(choices).toHaveValue('1');
  await expect(form.getByRole('alert')).toContainText('not used');
  await expect(form.getByRole('button', { name: 'Preview transfer', exact: true })).toBeDisabled();
  // Removing them is the owner's explicit decision; only then does the preview run.
  await form.getByRole('button', { name: 'Remove those quantities', exact: true }).click();
  await expect(form.getByRole('alert')).toHaveCount(0);
  const [request] = await Promise.all([
    page.waitForRequest((r) => r.method() === 'POST' && new URL(r.url()).pathname === '/api/v1/investments/transfers/external/out/preview'),
    form.getByRole('button', { name: 'Preview transfer', exact: true }).click()
  ]);
  expect(request.postDataJSON().lot_allocations).toHaveLength(1);
});

test('(#521 third review) all eight position pickers reopen safely with an unavailable position', async ({ page }) => {
  const s = await setup(page);
  await position(s, page);
  await start(page, s.holding.id);
  await openInvestmentActivity(page, 'Shares out');
  const initial = page.getByRole('dialog').filter({ has: page.locator('#external-transfer-out-source') });
  await expect(initial.locator('#external-transfer-out-source')).not.toHaveValue('');
  await initial.getByRole('button', { name: 'Cancel', exact: true }).click();
  await openInvestmentActivity(page, 'Buy');
  const trade = page.getByRole('dialog').filter({ has: page.locator('#buy-quantity') });
  await trade.locator('#buy-date').fill(daysFromTodayISO(-11));
  await trade.getByRole('button', { name: 'Cancel', exact: true }).click();
  for (const [activity, id, cost] of [
    ['Shares out', 'external-transfer-out-source', true],
    ['Move shares between holdings', 'internal-transfer-source', true],
    ['Return of capital', 'capital-return-position', true],
    ['Write-off', 'write-off-holding', true],
    ['Share exchange', 'exchange-holding', false],
    ['Spin-off', 'spin-off-holding', false],
    ['Cash-and-stock merger', 'merger-holding', false],
    ['Split', 'split-holding', false]
  ] as const) {
    await openInvestmentActivity(page, activity);
    // Write-off is inline, the others are dialogs: scope by the form itself.
    const form = page.locator('form').filter({ has: page.locator(`#${id}`) });
    const key = `${s.holding.id}:${s.instrument.commodity_id}${cost ? `:${s.currencyID}` : ''}`;
    await expect(form.locator(`#${id}`)).toHaveValue(key);
    await expect(form.getByRole('alert')).toContainText('not available');
    await form.getByRole('button', { name: 'Cancel', exact: true }).click();
  }
});

test('(#521 fourth review) a shared cost currency the merger holding lacks stays chosen and blocks preview', async ({ page }) => {
  const s = await setup(page);
  await position(s, page);
  const opened = daysFromTodayISO(-10);
  const eur = await ensureCurrency(page, s.csrfToken, 'EUR', 'Euro');
  const target = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', s.csrfToken, {
    commodity_code: `NEW${s.suffix}`, instrument_type: 'stock', display_name: `Target ${s.suffix}`,
    quantity_scale: 0, price_scale: 2, quote_commodity_id: s.currencyID, effective_from: opened
  });
  await apiJSON(page, 'POST', '/api/v1/investments/holding-accounts', s.csrfToken, {
    instrument_id: target.id, name: `Target holding ${s.suffix}`, parent_account_id: s.broker.id, opened_on: opened, effective_from: opened
  });
  // A cash account in each currency, so only the currency itself can block the preview.
  const eurCash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', s.csrfToken, {
    name: `Euro cash ${s.suffix}`, account_class: 'asset', account_kind: 'brokerage_cash', parent_account_id: s.broker.id,
    default_commodity_id: eur, allows_postings: true, opened_on: opened, effective_from: opened
  });
  await start(page, s.holding.id);
  await openInvestmentActivity(page, 'Shares in');
  const incoming = page.getByRole('dialog').filter({ has: page.locator('#transfer-in-currency') });
  await incoming.locator('#transfer-in-currency').selectOption(String(eur));
  await incoming.getByRole('button', { name: 'Cancel', exact: true }).click();
  await openInvestmentActivity(page, 'Cash-and-stock merger');
  const form = page.getByRole('dialog').filter({ has: page.locator('#merger-new-instrument') });
  await form.locator('#merger-date').fill(daysFromTodayISO(-1));
  await form.locator('#merger-payable').fill(daysFromTodayISO(-1));
  await form.locator('#merger-new-instrument').selectOption(String(target.commodity_id));
  await expect(form.locator('#merger-currency')).toHaveValue(String(eur));
  await expect(form.getByRole('alert')).toContainText('no lots with a cost basis in EUR');
  await form.locator('#merger-new-units').fill('1');
  await form.locator('#merger-old-units').fill('2');
  await form.locator('#merger-cash-account').selectOption(String(eurCash.id));
  await form.locator('#merger-cash').fill('3.00');
  await form.locator('#merger-percent').fill('25');
  const preview = form.getByRole('button', { name: 'Preview merger', exact: true });
  await expect(preview).toBeDisabled();
  await form.locator('#merger-currency').selectOption(String(s.currencyID));
  await expect(form.locator('#merger-currency-unavailable')).toHaveCount(0);
  await form.locator('#merger-cash-account').selectOption(String(s.cash.id));
  await expect(preview).toBeEnabled();
});

test('(#521 fourth review) return of capital ignores restored allocations while specific lots are off', async ({ page }) => {
  const s = await setup(page);
  await apiJSON(page, 'POST', '/api/v1/investments/buy', s.csrfToken, {
    transaction_date: daysFromTodayISO(-8), commodity_id: s.instrument.commodity_id, holding_account_id: s.holding.id,
    cash_account_id: s.cash.id, quantity_value: '2', quantity_scale: 0,
    cash_amount_value: '2000', cash_amount_scale: 2, cash_commodity_id: s.currencyID
  });
  await position(s, page);
  await start(page, s.holding.id);
  await openInvestmentActivity(page, 'Return of capital');
  const form = page.getByRole('dialog').filter({ has: page.locator('#capital-return-position') });
  await form.locator('#capital-return-payment').fill(todayISO());
  await form.locator('#capital-return-amount').fill('1');
  await form.getByRole('checkbox').check();
  await form.getByRole('button', { name: 'Preview return', exact: true }).click();
  const lots = form.locator('input[id^="capital-return-lot-"]');
  await expect(lots).toHaveCount(2);
  await lots.nth(0).fill('1');
  await lots.nth(1).fill('1');
  await form.getByRole('button', { name: 'Cancel', exact: true }).click();
  await openInvestmentActivity(page, 'Buy');
  const trade = page.getByRole('dialog').filter({ has: page.locator('#buy-quantity') });
  await trade.locator('#buy-date').fill(daysFromTodayISO(-6));
  await trade.getByRole('button', { name: 'Cancel', exact: true }).click();
  await openInvestmentActivity(page, 'Return of capital');
  await form.getByRole('button', { name: 'Preview return', exact: true }).click();
  await expect(lots).toHaveCount(1);
  await expect(form.getByRole('alert')).toBeVisible();
  await form.getByRole('checkbox').uncheck();
  await expect(form.getByRole('button', { name: 'Preview return', exact: true })).toBeEnabled();
});


test('(#521 fourth review) a forced close leaves the discard confirmation usable above its parent', async ({ page }) => {
  await setup(page);
  await page.goto('/app');
  await page.getByRole('button', { name: 'New transaction', exact: true }).first().click();
  const dialog = page.getByRole('dialog', { name: 'New transaction', exact: true });
  await dialog.getByLabel('Description', { exact: true }).fill('Keep this entry');
  await dialog.evaluate((el: HTMLDialogElement) => el.close());
  const discard = page.getByRole('alertdialog', { name: 'Discard this transaction?' });
  await expect(discard).toBeVisible();
  await discard.getByRole('button', { name: 'Keep editing', exact: true }).click({ trial: true, timeout: 5000 });
  await expect(discard.getByRole('button', { name: 'Keep editing', exact: true })).toBeFocused();
  await discard.getByRole('button', { name: 'Keep editing', exact: true }).click();
  await expect(discard).toHaveCount(0);
  await expect(dialog.getByLabel('Description', { exact: true })).toHaveValue('Keep this entry');
});

/** A second holding of the same instrument whose only position is costed in EUR, and a merger target. */
async function euroMerger(s: Awaited<ReturnType<typeof setup>>, page: Page) {
  await position(s, page);
  const eur = await ensureCurrency(page, s.csrfToken, 'EUR', 'Euro');
  const eurCash = await createCashAccount(page, s.csrfToken, `EUR cash ${s.suffix}`, eur, daysFromTodayISO(-10));
  const euroHolding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', s.csrfToken, {
    instrument_id: s.instrument.id, name: `EUR holding ${s.suffix}`, parent_account_id: s.broker.id,
    opened_on: daysFromTodayISO(-10), effective_from: daysFromTodayISO(-10)
  });
  await apiJSON(page, 'POST', '/api/v1/investments/buy', s.csrfToken, {
    transaction_date: daysFromTodayISO(-5), commodity_id: s.instrument.commodity_id, holding_account_id: euroHolding.id,
    cash_account_id: eurCash.id, quantity_value: '2', quantity_scale: 0,
    cash_amount_value: '2000', cash_amount_scale: 2, cash_commodity_id: eur
  });
  const target = await apiJSON<{ commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', s.csrfToken, {
    commodity_code: `NEW${s.suffix}`, instrument_type: 'stock', display_name: `Target ${s.suffix}`,
    quantity_scale: 0, price_scale: 2, quote_commodity_id: s.currencyID, effective_from: daysFromTodayISO(-10)
  });
  return {
    eur, target,
    usdKey: `${s.holding.id}:${s.instrument.commodity_id}`,
    eurKey: `${euroHolding.id}:${s.instrument.commodity_id}`,
    euroHoldingID: euroHolding.id
  };
}

test('(#521 fifth review) changing the merger holding keeps an explicitly chosen cash currency', async ({ page }) => {
  const s = await setup(page);
  const e = await euroMerger(s, page);
  await start(page, e.euroHoldingID);
  await openInvestmentActivity(page, 'Cash-and-stock merger');
  const form = page.getByRole('dialog').filter({ has: page.locator('#merger-new-instrument') });
  await form.locator('#merger-new-instrument').selectOption(String(e.target.commodity_id));
  await form.locator('#merger-currency').selectOption(String(e.eur));
  await form.locator('#merger-cash').fill('10');
  await form.locator('#merger-holding').selectOption(e.usdKey);
  await expect(form.locator('#merger-cash')).toHaveValue('10');
  await expect(form.locator('#merger-currency')).toHaveValue(String(e.eur));
  await expect(form.locator('#merger-currency-unavailable')).toContainText('EUR');
  await expect(form.getByRole('button', { name: 'Preview merger', exact: true })).toBeDisabled();
});

test('(#521 fifth review) the default merger currency follows the holding until an amount is entered in it', async ({ page }) => {
  const s = await setup(page);
  const e = await euroMerger(s, page);
  await start(page, s.holding.id);
  await openInvestmentActivity(page, 'Cash-and-stock merger');
  const form = page.getByRole('dialog').filter({ has: page.locator('#merger-new-instrument') });
  await form.locator('#merger-holding').selectOption(e.usdKey);
  await expect(form.locator('#merger-currency')).toHaveValue(String(s.currencyID));
  await form.locator('#merger-holding').selectOption(e.eurKey);
  await expect(form.locator('#merger-currency')).toHaveValue(String(e.eur));
  await form.locator('#merger-holding').selectOption(e.usdKey);
  await expect(form.locator('#merger-currency')).toHaveValue(String(s.currencyID));
  await form.locator('#merger-cash').fill('10');
  await form.locator('#merger-holding').selectOption(e.eurKey);
  await expect(form.locator('#merger-currency')).toHaveValue(String(s.currencyID));
  await expect(form.locator('#merger-currency-unavailable')).toContainText('USD');
});

async function addHolding(page: Page, s: Awaited<ReturnType<typeof setup>>, instrumentID: number, name: string) {
  return apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', s.csrfToken, {
    instrument_id: instrumentID, name: `${name} ${s.suffix}`, parent_account_id: s.broker.id,
    opened_on: daysFromTodayISO(-10), effective_from: daysFromTodayISO(-10)
  });
}
async function addBuy(page: Page, s: Awaited<ReturnType<typeof setup>>, holdingID: number, cashID: number, currencyID: number) {
  return apiJSON(page, 'POST', '/api/v1/investments/buy', s.csrfToken, {
    transaction_date: daysFromTodayISO(-5), commodity_id: s.instrument.commodity_id, holding_account_id: holdingID,
    cash_account_id: cashID, quantity_value: '2', quantity_scale: 0,
    cash_amount_value: '2000', cash_amount_scale: 2, cash_commodity_id: currencyID
  });
}
async function targetInstrument(page: Page, s: Awaited<ReturnType<typeof setup>>) {
  return apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', s.csrfToken, {
    commodity_code: `NEW${s.suffix}`, instrument_type: 'stock', display_name: `Target ${s.suffix}`,
    quantity_scale: 0, price_scale: 2, quote_commodity_id: s.currencyID, effective_from: daysFromTodayISO(-10)
  });
}

test('(#521 sixth review) a restored merger amount keeps the currency it was entered in', async ({ page }) => {
  const s = await setup(page);
  await position(s, page);
  const eur = await ensureCurrency(page, s.csrfToken, 'EUR', 'Euro');
  const eurHolding = await addHolding(page, s, s.instrument.id, 'Euro holding');
  const eurCash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', s.csrfToken, {
    name: `Euro cash ${s.suffix}`, account_class: 'asset', account_kind: 'brokerage_cash', parent_account_id: s.broker.id,
    default_commodity_id: eur, allows_postings: true, opened_on: daysFromTodayISO(-10), effective_from: daysFromTodayISO(-10)
  });
  await addBuy(page, s, eurHolding.id, eurCash.id, eur);
  const target = await targetInstrument(page, s);
  await addHolding(page, s, target.id, 'Target holding');
  await start(page, s.holding.id);
  await openInvestmentActivity(page, 'Cash-and-stock merger');
  const form = page.getByRole('dialog').filter({ has: page.locator('#merger-new-instrument') });
  await form.locator('#merger-new-instrument').selectOption(String(target.commodity_id));
  await expect(form.locator('#merger-currency')).toHaveValue(String(s.currencyID));
  await form.locator('#merger-cash').fill('10');
  await form.getByRole('button', { name: 'Cancel', exact: true }).click();
  await openInvestmentActivity(page, 'Shares out');
  const outgoing = page.getByRole('dialog').filter({ has: page.locator('#external-transfer-out-source') });
  await outgoing.locator('#external-transfer-out-source').selectOption(`${eurHolding.id}:${s.instrument.commodity_id}:${eur}`);
  await outgoing.getByRole('button', { name: 'Cancel', exact: true }).click();
  await openInvestmentActivity(page, 'Cash-and-stock merger');
  await expect(form.locator('#merger-holding')).toHaveValue(`${eurHolding.id}:${s.instrument.commodity_id}`);
  // Keep the old money's unit identified, or ask for the amount again.
  await expect(form.locator('#merger-cash')).toHaveValue('10');
  await expect(form.locator('#merger-currency')).toHaveValue(String(s.currencyID));
  await expect(form.locator('#merger-currency-unavailable')).toContainText('USD');
});

test('(#521 sixth review) a chosen receiving holding survives a source change and is checked, never replaced', async ({ page }) => {
  const s = await setup(page);
  await position(s, page);
  const other = await addHolding(page, s, s.instrument.id, 'Other USD holding');
  await addBuy(page, s, other.id, s.cash.id, s.currencyID);
  const target = await targetInstrument(page, s);
  await addHolding(page, s, target.id, 'Target A');
  await addHolding(page, s, target.id, 'Target B');
  await start(page, s.holding.id);
  await openInvestmentActivity(page, 'Cash-and-stock merger');
  const form = page.getByRole('dialog').filter({ has: page.locator('#merger-new-instrument') });
  await form.locator('#merger-new-instrument').selectOption(String(target.commodity_id));
  const destination = form.locator('#merger-destination');
  const initial = await destination.inputValue();
  const chosen = await destination.locator('option').evaluateAll((options, current) => options.map(o => (o as HTMLOptionElement).value).find(v => v !== '' && v !== 'same' && v !== current)!, initial);
  expect(chosen).toBeTruthy();
  await destination.selectOption(chosen);
  await form.locator('#merger-holding').selectOption(`${other.id}:${s.instrument.commodity_id}`);
  await expect(destination).toHaveValue(chosen);
  await expect(form.locator('#merger-destination-unavailable')).toHaveCount(0);

  // A new instrument the chosen holding cannot hold leaves it named, not replaced.
  const alternative = await apiJSON<{ commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', s.csrfToken, {
    commodity_code: `ALT${s.suffix}`, instrument_type: 'stock', display_name: `Alternative ${s.suffix}`,
    quantity_scale: 0, price_scale: 2, quote_commodity_id: s.currencyID, effective_from: daysFromTodayISO(-10)
  });
  await start(page, s.holding.id);
  await openInvestmentActivity(page, 'Cash-and-stock merger');
  await form.locator('#merger-new-instrument').selectOption(String(target.commodity_id));
  await destination.selectOption(chosen);
  await form.locator('#merger-new-instrument').selectOption(String(alternative.commodity_id));
  await expect(destination).toHaveValue(chosen);
  await expect(form.locator('#merger-destination-unavailable')).toBeVisible();
  await expect(form.getByRole('button', { name: 'Preview merger', exact: true })).toBeDisabled();
});

async function euroCash(page: Page, s: Awaited<ReturnType<typeof setup>>) {
  const eur = await ensureCurrency(page, s.csrfToken, 'EUR', 'Euro');
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', s.csrfToken, {
    name: `EUR cash ${s.suffix}`, account_class: 'asset', account_kind: 'brokerage_cash', parent_account_id: s.broker.id,
    default_commodity_id: eur, allows_postings: true, opened_on: daysFromTodayISO(-10), effective_from: daysFromTodayISO(-10)
  });
  return { eur, cash };
}
async function buyAt(page: Page, s: Awaited<ReturnType<typeof setup>>, date: string, cashID: number, currency: number) {
  await apiJSON(page, 'POST', '/api/v1/investments/buy', s.csrfToken, {
    transaction_date: date, commodity_id: s.instrument.commodity_id, holding_account_id: s.holding.id,
    cash_account_id: cashID, quantity_value: '2', quantity_scale: 0,
    cash_amount_value: '2000', cash_amount_scale: 2, cash_commodity_id: currency
  });
}

test('(#521 seventh review) a merger amount keeps its currency when a date change reprices the holding', async ({ page }) => {
  const s = await setup(page);
  const e = await euroCash(page, s);
  await buyAt(page, s, daysFromTodayISO(-8), s.cash.id, s.currencyID);
  await apiJSON(page, 'POST', '/api/v1/investments/sell', s.csrfToken, {
    transaction_date: daysFromTodayISO(-6), commodity_id: s.instrument.commodity_id, holding_account_id: s.holding.id,
    cash_account_id: s.cash.id, quantity_value: '2', quantity_scale: 0,
    cash_amount_value: '2000', cash_amount_scale: 2, cash_commodity_id: s.currencyID, cost_basis_method: 'fifo'
  });
  await buyAt(page, s, daysFromTodayISO(-5), e.cash.id, e.eur);
  const target = await apiJSON<{ commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', s.csrfToken, {
    commodity_code: `NEW${s.suffix}`, instrument_type: 'stock', display_name: `Target ${s.suffix}`,
    quantity_scale: 0, price_scale: 2, quote_commodity_id: s.currencyID, effective_from: daysFromTodayISO(-10)
  });
  await start(page, s.holding.id);
  await openInvestmentActivity(page, 'Cash-and-stock merger');
  const form = page.getByRole('dialog').filter({ has: page.locator('#merger-new-instrument') });
  await form.locator('#merger-new-instrument').selectOption(String(target.commodity_id));
  await form.locator('#merger-date').fill(daysFromTodayISO(-7));
  await expect(form.locator('#merger-currency')).toHaveValue(String(s.currencyID));
  await form.locator('#merger-cash').fill('10');
  const response = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/investments/dated-holdings' && new URL(r.url()).searchParams.get('as_of') === daysFromTodayISO(1));
  await form.locator('#merger-date').fill(daysFromTodayISO(1));
  const loaded = await response;
  expect(loaded.ok()).toBe(true);
  const body = await loaded.json();
  expect(body.holdings.some((h: { account_id: number; cost_commodity_id: number; quantity_value: string }) => h.account_id === s.holding.id && h.cost_commodity_id === e.eur && BigInt(h.quantity_value) > 0n)).toBe(true);
  await expect(form.locator('#merger-currency-unavailable')).toContainText('USD');
  await expect(form.locator('#merger-cash')).toHaveValue('10');
  await expect(form.locator('#merger-currency')).toHaveValue(String(s.currencyID));
});

test('(#521 seventh review) a trade total is cleared, not relabelled, when restored under another cash currency', async ({ page }) => {
  const s = await setup(page);
  const e = await euroCash(page, s);
  await start(page, s.holding.id);
  await openInvestmentActivity(page, 'Buy');
  const buy = page.getByRole('dialog').filter({ has: page.locator('#buy-quantity') });
  await buy.locator('#buy-cash-account').selectOption(String(s.cash.id));
  await buy.locator('#buy-quantity').fill('1');
  await buy.locator('#buy-cash-amount').fill('10');
  await buy.getByRole('button', { name: 'Cancel', exact: true }).click();
  await openInvestmentActivity(page, 'Sell');
  const sell = page.getByRole('dialog').filter({ has: page.locator('#sell-quantity') });
  await sell.locator('#sell-cash-account').selectOption(String(e.cash.id));
  await sell.getByRole('button', { name: 'Cancel', exact: true }).click();
  await openInvestmentActivity(page, 'Buy');
  await expect(buy.locator('#buy-cash-account')).toHaveValue(String(e.cash.id));
  await expect(buy.locator('#buy-cash-amount')).toHaveValue('');
  await expect(buy.getByText('The cash account’s currency changed')).toBeVisible();

  // Inside the form too: a total typed for one account is cleared when another currency is chosen.
  await buy.locator('#buy-cash-amount').fill('12');
  await buy.locator('#buy-cash-account').selectOption(String(s.cash.id));
  await expect(buy.locator('#buy-cash-amount')).toHaveValue('');
  await buy.locator('#buy-cash-amount').fill('12');
  await expect(buy.getByText('The cash account’s currency changed')).toBeHidden();
});

test('(#521 seventh review) a unit price carried to the pair is cleared under another cash currency', async ({ page }) => {
  const s = await setup(page);
  const e = await euroCash(page, s);
  await start(page, s.holding.id);
  await openInvestmentActivity(page, 'Buy');
  const buy = page.getByRole('dialog').filter({ has: page.locator('#buy-quantity') });
  await buy.locator('#buy-cash-account').selectOption(String(s.cash.id));
  await buy.locator('#buy-quantity').fill('1');
  await buy.locator('#buy-unit-price').fill('5');
  await expect(buy.locator('#buy-cash-amount')).toHaveValue('5.00');
  await buy.getByRole('button', { name: 'Cancel', exact: true }).click();
  await openInvestmentActivity(page, 'Sell');
  const sell = page.getByRole('dialog').filter({ has: page.locator('#sell-quantity') });
  await expect(sell.locator('#sell-unit-price')).toHaveValue('5');
  await sell.locator('#sell-cash-account').selectOption(String(e.cash.id));
  await expect(sell.locator('#sell-unit-price')).toHaveValue('');
  await expect(sell.locator('#sell-quantity')).toHaveValue('1');
  await expect(sell.getByText('The cash account’s currency changed')).toBeVisible();
});
