import { expect, test, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { createCashAccount, expectSavedTransaction, readyForLedger } from './support/ledger';
import { daysFromTodayISO, todayISO } from './support/dates';
import { openInvestmentActivity } from './support/investment-entry';

// #660: a create whose response is lost must not be recorded twice when the
// owner clicks Save again. The form sends one Idempotency-Key with every retry
// of one entry; the server answers the repeat with the stored result. Each
// case lets the first POST reach the server, drops its response, saves again
// and counts the records through the API.

const networkError = 'The server could not be reached. Check the connection and try again.';

/** Forwards every POST to path, and drops the first response after the server answered it. */
async function loseFirstResponse(page: Page, path: string) {
  const answered: Array<{ status: number; replayed: string | undefined; key: string | null }> = [];
  await page.route(`**${path}`, async (route) => {
    if (route.request().method() !== 'POST') return route.continue();
    const key = await route.request().headerValue('Idempotency-Key');
    const response = await route.fetch();
    answered.push({ status: response.status(), replayed: response.headers()['idempotent-replayed'], key });
    if (answered.length === 1) return route.abort('failed');
    return route.fulfill({ response });
  });
  return answered;
}

async function transactionCount(page: Page, accountID: number): Promise<number> {
  const body = await apiJSON<{ transactions: unknown[] }>(page, 'GET', `/api/v1/transactions?account_id=${accountID}&limit=200`);
  return body.transactions.length;
}

test('a transaction saved again after its response was lost is recorded once; a second entry is a second record', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `Lost response ${suffix}`, currencyID);
  const description = `Lost response coffee ${suffix}`;
  const answered = await loseFirstResponse(page, '/api/v1/transactions');

  const enter = async () => {
    await page.getByRole('button', { name: 'New transaction', exact: true }).click();
    const form = page.getByRole('region', { name: 'New transaction', exact: true });
    await form.getByLabel('Date').fill(todayISO());
    await form.getByLabel('Description').fill(description);
    await form.getByLabel('Account').selectOption({ label: account.name });
    await form.getByLabel('Amount').fill('4.50');
    await form.getByPlaceholder('Search categories').click();
    await form.locator('ul[role="listbox"] button[role="option"]').first().click();
    return form;
  };

  await page.goto('/app/ledger/transactions');
  const form = await enter();
  await form.getByRole('button', { name: 'Save transaction', exact: true }).click();
  await expect(form.getByText(networkError)).toBeVisible();
  await form.getByRole('button', { name: 'Save transaction', exact: true }).click();
  await expectSavedTransaction(page, description);

  expect(answered.map((answer) => [answer.status, answer.replayed])).toEqual([[201, undefined], [201, 'true']]);
  expect(answered[0].key).toBeTruthy();
  expect(answered[1].key).toBe(answered[0].key);
  expect(await transactionCount(page, account.id)).toBe(1);

  // The same coffee again, entered deliberately: a new entry, a new key.
  await page.unrouteAll({ behavior: 'wait' });
  await page.goto('/app/ledger/transactions');
  const again = await enter();
  const second = page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/transactions');
  await again.getByRole('button', { name: 'Save transaction', exact: true }).click();
  const response = await second;
  expect(response.status()).toBe(201);
  expect(await response.request().headerValue('Idempotency-Key')).not.toBe(answered[0].key);
  await expect.poll(() => transactionCount(page, account.id)).toBe(2);
});

async function investmentSetup(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const date = daysFromTodayISO(-10);
  const instrument = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: `LR${suffix}`, instrument_type: 'stock', display_name: `Lost response shares ${suffix}`,
    quantity_scale: 0, price_scale: 2, quote_commodity_id: currencyID, effective_from: date
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Lost response holding ${suffix}`, opened_on: date, effective_from: date
  });
  const account = (name: string, accountClass: string, kind: string) => apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `${name} ${suffix}`, account_class: accountClass, account_kind: kind, default_commodity_id: currencyID,
    allows_postings: true, opened_on: date, effective_from: date
  });
  const cash = await account('Lost response cash', 'asset', 'brokerage_cash');
  const income = await account('Lost response income', 'income', 'income');
  return { instrument: `Lost response shares ${suffix}`, holding, cash, income };
}

test('a buy, a sell and a dividend saved again after a lost response are each recorded once', async ({ page }) => {
  const s = await investmentSetup(page);
  const tradeDate = daysFromTodayISO(-2);
  const lots = async () => (await apiJSON<{ lots: unknown[] }>(page, 'GET', `/api/v1/investments/lots?account_id=${s.holding.id}`)).lots.length;

  await page.goto('/app/investments');

  // Buy.
  let answered = await loseFirstResponse(page, '/api/v1/investments/buy');
  await openInvestmentActivity(page, 'Buy');
  let form = page.locator('form').filter({ has: page.locator('#buy-quantity') });
  await form.locator('#buy-date').fill(tradeDate);
  await form.locator('#buy-instrument').fill(s.instrument);
  await page.getByRole('option').filter({ hasText: s.instrument }).click();
  await form.locator('#buy-holding-account').selectOption(String(s.holding.id));
  await form.locator('#buy-quantity').fill('4');
  await form.locator('#buy-cash-account').selectOption(String(s.cash.id));
  await form.locator('#buy-unit-price').fill('10');
  await form.getByRole('button', { name: 'Record buy', exact: true }).click();
  await expect(form.getByText(networkError)).toBeVisible();
  await form.getByRole('button', { name: 'Record buy', exact: true }).click();
  await expect(page.locator('#buy-quantity')).toBeHidden();
  expect(answered.map((answer) => [answer.status, answer.replayed])).toEqual([[201, undefined], [201, 'true']]);
  expect(await transactionCount(page, s.cash.id)).toBe(1);
  expect(await lots()).toBe(1);
  await page.unrouteAll({ behavior: 'wait' });

  // Sell every unit: writing it again would be refused for want of lots, so
  // only the stored result can answer the second save.
  answered = await loseFirstResponse(page, '/api/v1/investments/sell');
  await openInvestmentActivity(page, 'Sell');
  form = page.locator('form').filter({ has: page.locator('#sell-quantity') });
  await form.locator('#sell-date').fill(daysFromTodayISO(-1));
  await form.locator('#sell-instrument').fill(s.instrument);
  await page.getByRole('option').filter({ hasText: s.instrument }).click();
  await form.locator('#sell-holding-account').selectOption(String(s.holding.id));
  await form.locator('#sell-quantity').fill('4');
  await form.locator('#sell-cash-account').selectOption(String(s.cash.id));
  await form.locator('#sell-unit-price').fill('12');
  const confirm = form.getByRole('button', { name: 'Confirm sell', exact: true });
  await expect(confirm).toBeEnabled();
  await confirm.click();
  await expect(form.getByText(networkError)).toBeVisible();
  await expect(confirm).toBeEnabled();
  await confirm.click();
  await expect(page.locator('#sell-quantity')).toBeHidden();
  expect(answered.map((answer) => [answer.status, answer.replayed])).toEqual([[201, undefined], [201, 'true']]);
  expect(await transactionCount(page, s.cash.id)).toBe(2);
  await page.unrouteAll({ behavior: 'wait' });

  // Cash dividend.
  answered = await loseFirstResponse(page, '/api/v1/investments/dividend');
  await openInvestmentActivity(page, 'dividend');
  form = page.locator('form').filter({ has: page.locator('#div-amount') });
  await form.locator('#div-date').fill(tradeDate);
  await form.locator('#div-cash-account').selectOption(String(s.cash.id));
  await form.locator('#div-income-account').selectOption(String(s.income.id));
  await form.locator('#div-amount').fill('3.25');
  const record = form.getByRole('button', { name: 'Record dividend', exact: true });
  await record.click();
  await expect(form.getByText(networkError)).toBeVisible();
  await record.click();
  await expect(page.locator('#div-amount')).toBeHidden();
  expect(answered.map((answer) => [answer.status, answer.replayed])).toEqual([[201, undefined], [201, 'true']]);
  expect(await transactionCount(page, s.cash.id)).toBe(3);
});
