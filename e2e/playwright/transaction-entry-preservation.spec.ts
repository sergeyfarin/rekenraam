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
