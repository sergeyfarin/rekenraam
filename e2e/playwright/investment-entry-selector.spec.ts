import { expect, test, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';
import { daysFromTodayISO, todayISO } from './support/dates';
import { expectNoAccessibilityViolations, focusIsInside } from './support/a11y';
import { openInvestmentActivity } from './support/investment-entry';

async function setup(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const date = daysFromTodayISO(-10);
  const checking = await createCashAccount(page, csrfToken, `Entry bank ${suffix}`, currencyID, date);
  const other = await createCashAccount(page, csrfToken, `Entry other ${suffix}`, currencyID, date);
  const broker = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Entry broker ${suffix}`, account_class: 'asset', account_kind: 'brokerage',
    default_commodity_id: currencyID, allows_postings: false, opened_on: date, effective_from: date
  });
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: `ES${suffix}`, instrument_type: 'stock', display_name: `Entry shares ${suffix}`,
    quantity_scale: 0, price_scale: 2, quote_commodity_id: currencyID, effective_from: date
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Entry holding ${suffix}`, parent_account_id: broker.id,
    opened_on: date, effective_from: date
  });
  const cash = await apiJSON<{ id: number; name: string }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Entry broker cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    parent_account_id: broker.id, default_commodity_id: currencyID,
    allows_postings: true, opened_on: date, effective_from: date
  });
  return { csrfToken, currencyID, checking, other, broker, holding, cash, instrument, suffix, date };
}

for (const theme of ['light', 'dark']) {
  test(`journeys 6 and 10: bank context carries facts and requires a choice against brokerage cash in ${theme}`, async ({ page }) => {
    const s = await setup(page);
    await page.setViewportSize({ width: 390, height: 844 });
    await page.evaluate(value => localStorage.setItem('rekenraam-theme', value), theme);
    await page.goto(`/app/ledger/accounts/${s.checking.id}/register`);
    await page.getByRole('button', { name: 'New transaction', exact: true }).click();
    const entry = page.getByRole('region', { name: 'New transaction', exact: true });
    const date = daysFromTodayISO(-2);
    const memo = `Entry context ${s.suffix}`;
    await entry.getByLabel('Date', { exact: true }).fill(date);
    await entry.getByLabel('Description', { exact: true }).fill(memo);
    await entry.getByRole('tab', { name: 'Investment', exact: true }).click();
    await openInvestmentActivity(page, 'Buy');
    const buy = page.getByRole('dialog').filter({ has: page.locator('#buy-cash-account') });
    await expect(buy.getByLabel('Date', { exact: true })).toHaveValue(date);
    await expect(buy.locator('#buy-memo')).toHaveValue(memo);
    await expect(buy.getByLabel('Pay from', { exact: true })).toHaveValue(String(s.checking.id));
    await buy.getByLabel('Holding account', { exact: true }).selectOption(String(s.holding.id));
    await expect(buy.getByLabel('Pay from', { exact: true })).toHaveValue('');
    await buy.getByLabel('Pay from', { exact: true }).selectOption(String(s.other.id));
    const refreshed = page.waitForResponse(response => response.url().includes('/investments/entry-accounts?as_of=' + daysFromTodayISO(-3)));
    await buy.getByLabel('Date', { exact: true }).fill(daysFromTodayISO(-3));
    expect((await refreshed).ok()).toBe(true);
    await buy.getByLabel('Holding account', { exact: true }).selectOption('');
    await buy.getByLabel('Holding account', { exact: true }).selectOption(String(s.holding.id));
    await expect(buy.getByLabel('Pay from', { exact: true })).toHaveValue(String(s.other.id));
    await expectNoAccessibilityViolations(page, `investment buy ${theme}`);
    await buy.getByRole('button', { name: 'Cancel', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Continue', exact: true })).toBeFocused();

    await openInvestmentActivity(page, 'Sell');
    await expect(page.getByLabel('Pay into', { exact: true })).toHaveValue(String(s.checking.id));
    await page.getByRole('dialog').filter({ has: page.locator('#sell-cash-account') }).getByRole('button', { name: 'Cancel', exact: true }).click();
    await openInvestmentActivity(page, 'Split');
    const split = page.getByRole('dialog').filter({ has: page.locator('#split-date') });
    await expect(split.getByRole('combobox', { name: /Cash account|Pay from|Pay into/ })).toHaveCount(0);
    await split.getByRole('button', { name: 'Cancel', exact: true }).click();
    await openInvestmentActivity(page, 'Reinvested dividend');
    const reinvest = page.getByRole('dialog').filter({ has: page.locator('#div-cost-currency') });
    await expect(reinvest.getByLabel('Cost currency')).toBeVisible();
    await expect(reinvest.locator('#div-cash-account')).toHaveCount(0);
    await expect(reinvest.locator('#div-memo')).toHaveValue(memo);
  });
}

test('journeys 2 and 9: brokerage and holding contexts share entry and name their sole cash suggestion', async ({ page }) => {
  const s = await setup(page);
  await page.goto(`/app/ledger/accounts/${s.holding.id}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  await openInvestmentActivity(page, 'Buy');
  await expect(page.locator('#buy-holding-account')).toHaveValue(String(s.holding.id));
  await expect(page.locator('#buy-cash-account')).toHaveValue(String(s.cash.id));
  await expect(page.locator('#buy-cash-account option:checked')).toHaveText(s.cash.name);
  await page.getByRole('dialog').last().getByRole('button', { name: 'Cancel', exact: true }).click();
  const entry = page.getByRole('region', { name: 'New transaction', exact: true });
  await entry.getByRole('tab', { name: 'Withdrawal', exact: true }).click();
  await expect(entry.getByRole('combobox', { name: 'Account', exact: true }).locator('option:checked')).toHaveText(`Entry holding ${s.suffix}`);
  await expect(entry.getByText('Choose a cash account, or use the Investment tab.', { exact: false })).toBeVisible();
  await page.goto(`/app/ledger/accounts/${s.broker.id}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  await expect(page.getByRole('region', { name: 'New transaction', exact: true }).getByRole('tab')).toHaveText(['Withdrawal', 'Deposit', 'Transfer', 'Investment']);
  await page.getByRole('tab', { name: 'Investment', exact: true }).click();
  await openInvestmentActivity(page, 'Buy');
  await expect(page.locator('#buy-cash-account')).toHaveValue(String(s.cash.id));
});

for (const [locale, activity, prefix, group, next] of [
  ['en', 'Activity', 'buy', 'Trades', 'Continue'],
  ['nl', 'Activiteit', 'kop', 'Handel', 'Doorgaan'],
  ['ru', 'Операция', 'покуп', 'Сделки', 'Продолжить'],
  ['de', 'Activity', 'buy', 'Trades', 'Continue'],
  ['es', 'Activity', 'buy', 'Trades', 'Continue'],
  ['fr', 'Activity', 'buy', 'Trades', 'Continue']
]) {
  test(`journey 17: native grouped selector supports localized keyboard entry in ${locale}`, async ({ page }) => {
    await readyForLedger(page);
    await page.setViewportSize({ width: 390, height: 844 });
    await page.evaluate(value => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    await page.goto('/app/investments');
    // Portfolio keeps the shared New transaction name in the selected locale.
    await page.locator('#main-content button').filter({ hasText: /New transaction|Nieuwe transactie|Новая транзакция|Neue Transaktion|Nueva transacción|Nouvelle transaction/ }).click();
    // The portfolio action explicitly selects Investment.
    const selector = page.getByRole('combobox', { name: activity, exact: true });
    await expect(selector.locator('optgroup')).toHaveCount(4);
    await expect(selector.locator(`optgroup[label="${group}"]`)).toHaveCount(1);
    await expect(selector.locator('option')).toHaveCount(18);
    await selector.focus();
    await page.keyboard.press('ArrowDown');
    await expect(selector).toBeFocused();
    await page.keyboard.type(prefix);
    await expect(selector).toHaveValue('buy');
    await expectNoAccessibilityViolations(page, `activity selector ${locale}`);
    await expect.poll(() => focusIsInside(page.getByRole('dialog'))).toBe(true);
    await expect(page.getByRole('button', { name: next, exact: true })).toBeEnabled();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  });
}

test('dated choices show loading and retry without calling a prefilling holding invalid', async ({ page }) => {
  const s = await setup(page);
  await page.goto(`/app/ledger/accounts/${s.holding.id}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  let release!: () => void;
  const blocked = new Promise<void>(resolve => { release = resolve; });
  let fail = true;
  let reads = 0;
  await page.route('**/api/v1/investments/entry-accounts?*', async route => {
    reads++;
    await blocked;
    if (fail) await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: { code: 'RESOURCE_BUSY', message: 'test refusal' } }) });
    else await route.continue();
  });
  await openInvestmentActivity(page, 'Buy');
  const form = page.getByRole('dialog').last();
  await expect(form.getByText('Loading accounts', { exact: true })).toBeVisible();
  expect(reads).toBe(1); // main and cash share one dated request, regardless of account count
  await expect(form.getByText('This holding cannot receive this activity on the selected date.', { exact: false })).toHaveCount(0);
  release();
  const retry = form.getByRole('button', { name: 'Retry accounts', exact: true });
  await expect(retry).toBeVisible({ timeout: 15_000 });
  await expect(form.getByRole('button', { name: 'Record buy', exact: true })).toBeDisabled();
  fail = false;
  await retry.click();
  await expect(form.locator('#buy-holding-account')).toHaveValue(String(s.holding.id));
  await expect(form.locator('#buy-cash-account')).toHaveValue(String(s.cash.id));
  await expect(retry).toBeHidden();
  await expectNoAccessibilityViolations(page, 'investment entry recovered account choices');
});

test('reinvestment waits for refreshed dated accounts and recovers after an error', async ({ page }) => {
  const s = await setup(page);
  await page.goto(`/app/ledger/accounts/${s.holding.id}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  await openInvestmentActivity(page, 'reinvested');
  const form = page.locator('dialog[aria-labelledby="reinvested-dividend-title"]');
  await expect(form.locator('#div-holding-account')).toHaveValue(String(s.holding.id));
  await form.locator('#div-cost-currency').selectOption(String(s.currencyID));
  await form.locator('#div-quantity').fill('1');
  await form.locator('#div-amount').fill('10');
  const submit = form.getByRole('button', { name: 'Record reinvested dividend', exact: true });
  await expect(submit).toBeEnabled();

  let release!: () => void;
  const blocked = new Promise<void>(resolve => { release = resolve; });
  let fail = true;
  await page.route('**/api/v1/investments/entry-accounts?*', async route => {
    await blocked;
    if (fail) await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: { code: 'RESOURCE_BUSY', message: 'test refusal' } }) });
    else await route.continue();
  });
  // Make the successful cached snapshot stale, then use the normal reconnect
  // refresh. Its holdings remain available while the refresh is pending/failed.
  await page.clock.setFixedTime(new Date(Date.now() + 20_000));
  await page.evaluate(() => {
    window.dispatchEvent(new Event('offline'));
    window.dispatchEvent(new Event('online'));
  });
  try {
    await expect(form.getByText('Loading accounts', { exact: true })).toBeVisible();
    await expect(submit).toBeDisabled();
  } finally {
    release();
  }
  const retry = form.getByRole('button', { name: 'Retry accounts', exact: true });
  await expect(retry).toBeVisible({ timeout: 15_000 });
  await expect(form.locator('#div-holding-account')).toHaveValue(String(s.holding.id));
  await expect(submit).toBeDisabled();
  fail = false;
  await retry.click();
  await expect(retry).toBeHidden();
  await expect(submit).toBeEnabled();
  await expect(form.locator('#div-quantity')).toHaveValue('1');
  await expect(form.locator('#div-amount')).toHaveValue('10');
});
