import { test, expect } from './support/owned-book';
import { apiJSON, csrfTokenFor } from './support/api';
import { createCashAccount, ensureCurrency, readyForLedger } from './support/ledger';
import { expectNoAccessibilityViolations } from './support/a11y';

// #482: one reconciliation out-of-date window, set in Settings › General,
// read by the account list and Home's Reconciliation widget alike. An owned
// book, because the window is a book-wide setting.

function shiftDays(isoDate: string, days: number): string {
  const date = new Date(`${isoDate}T00:00:00Z`);
  date.setUTCDate(date.getUTCDate() + days);
  return date.toISOString().slice(0, 10);
}

test('the reconciliation window drives the account list and Home together', async ({ page }) => {
  const { currencyID } = await readyForLedger(page);
  const { owner_today: today } = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
  const movedOn = shiftDays(today, -5);
  const statementDate = shiftDays(today, -3);
  let csrfToken = await csrfTokenFor(page);
  const reconciled = await createCashAccount(page, csrfToken, 'Window reconciled', currencyID, movedOn);
  const never = await createCashAccount(page, csrfToken, 'Window never', currencyID, movedOn);
  await createCashAccount(page, csrfToken, 'Window empty', currencyID, movedOn);
  const income = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name: 'Window salary', category_type: 'income' });
  for (const account of [reconciled, never]) {
    await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
      transaction_date: movedOn, status: 'posted',
      journal_entries: [{ entry_date: movedOn, postings: [
        { account_id: account.id, commodity_id: currencyID, quantity_value: '10000', quantity_scale: 2 },
        { account_id: income.id, commodity_id: currencyID, quantity_value: '-10000', quantity_scale: 2 }
      ] }]
    });
  }
  const session = await apiJSON<{ id: number; candidates: Array<{ posting_id: number }> }>(page, 'POST', `/api/v1/accounts/${reconciled.id}/reconciliations/start`, csrfToken, {
    commodity_id: currencyID, source_kind: 'statement', statement_date: statementDate, statement_balance_value: '10000', statement_balance_scale: 2
  });
  await apiJSON(page, 'PATCH', `/api/v1/reconciliations/${session.id}`, csrfToken, { posting_version_ids: session.candidates.map(candidate => candidate.posting_id) });
  await apiJSON(page, 'POST', `/api/v1/reconciliations/${session.id}/finish`, csrfToken, { change_reason: 'statement verified' });
  const shown = new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeZone: 'UTC' }).format(new Date(`${statementDate}T00:00:00Z`));

  const row = (name: string) => page.locator('article').filter({ hasText: name });
  await page.goto('/app/ledger');
  await expect(row('Window reconciled').getByTestId('account-reconciliation-age')).toHaveText(`Reconciled to ${shown}`);
  await expect(row('Window never').getByTestId('account-reconciliation-age')).toHaveText('Never reconciled');
  await expect(row('Window empty').getByTestId('account-reconciliation-age')).toHaveText('Nothing to reconcile');

  await page.goto('/app');
  const widget = page.getByTestId('explore-widget-reconciliation');
  await expect(widget).toHaveAttribute('data-state', 'ready');
  await expect(widget).toContainText('1 of 2 accounts reconciled');
  await expect(widget).toContainText('Within the last 60 days');

  await page.goto('/app/settings');
  const days = page.getByLabel('Out of date after (days)');
  await expect(days).toHaveValue('60');
  await days.fill('0');
  await page.getByTestId('reconciliation-settings').getByRole('button', { name: 'Save' }).click();
  await expect(page.getByText('Enter a whole number of days from 1 to 3650.')).toBeVisible();
  await expect(days).toHaveAttribute('aria-invalid', 'true');
  await expectNoAccessibilityViolations(page, 'reconciliation settings invalid');
  await days.fill('2');
  await page.getByTestId('reconciliation-settings').getByRole('button', { name: 'Save' }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Reconciliation setting saved.' })).toBeVisible();
  csrfToken = await csrfTokenFor(page);
  expect((await apiJSON<{ out_of_date_after_days: number }>(page, 'GET', '/api/v1/reconciliation/policy')).out_of_date_after_days).toBe(2);

  await page.goto('/app/ledger');
  await expect(row('Window reconciled').getByTestId('account-reconciliation-age')).toHaveText(`Out of date: reconciled to ${shown}`);
  await expect(row('Window empty').getByTestId('account-reconciliation-age')).toHaveText('Nothing to reconcile');
  await expectNoAccessibilityViolations(page, 'account list reconciliation states');

  await page.goto('/app');
  await expect(widget).toContainText('0 of 2 accounts reconciled');
  await expect(widget).toContainText('Within the last 2 days');
  await widget.getByRole('link', { name: 'Window never' }).click();
  await expect(page).toHaveURL(new RegExp(`/app/ledger/reconcile\\?account=${never.id}&commodity=${currencyID}$`));
});

// #594 review: a brokerage's cash account is folded into the brokerage row,
// so its judgement is named there; Ledger and Home agree.
test('a brokerage row names the reconciliation state of its folded cash account', async ({ page }) => {
  const { currencyID } = await readyForLedger(page);
  const { owner_today: today } = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
  const movedOn = shiftDays(today, -5);
  const csrfToken = await csrfTokenFor(page);
  const broker = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: 'Folded broker', account_class: 'asset', account_kind: 'brokerage', default_commodity_id: currencyID, allows_postings: true, opened_on: movedOn
  });
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: 'Folded broker cash', account_class: 'asset', account_kind: 'brokerage_cash', default_commodity_id: currencyID, allows_postings: true,
    parent_account_id: broker.id, opened_on: movedOn
  });
  const income = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name: 'Folded interest', category_type: 'income' });
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_date: movedOn, status: 'posted',
    journal_entries: [{ entry_date: movedOn, postings: [
      { account_id: cash.id, commodity_id: currencyID, quantity_value: '500', quantity_scale: 2 },
      { account_id: income.id, commodity_id: currencyID, quantity_value: '-500', quantity_scale: 2 }
    ] }]
  });

  await page.goto('/app/ledger');
  const row = page.locator('article').filter({ hasText: 'Folded broker' });
  await expect(row.getByTestId('account-folded-reconciliation-age')).toHaveText('Folded broker cash: Never reconciled');
  await expect(row.getByTestId('account-reconciliation-age')).toHaveCount(0);
  await expectNoAccessibilityViolations(page, 'folded reconciliation state');

  await page.goto('/app');
  await expect(page.getByTestId('explore-widget-reconciliation')).toContainText('Folded broker cash · Never reconciled');
});

// #594 review: an account judged in two currencies. Home names each on a
// 320px phone without clipping, and its link opens Reconcile on the currency
// it flagged, even for an account with no default currency.
test('a currency Home flags can be reconciled from its link, and fits a phone', async ({ page }) => {
  const { currencyID } = await readyForLedger(page);
  const { owner_today: today } = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
  const movedOn = shiftDays(today, -5);
  let csrfToken = await csrfTokenFor(page);
  const euro = await ensureCurrency(page, csrfToken, 'EUR', 'Euro');
  csrfToken = await csrfTokenFor(page);
  const receivable = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: 'Two-currency receivable', account_class: 'asset', account_kind: 'receivable', allows_postings: true, opened_on: movedOn
  });
  const income = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name: 'Two-currency fees', category_type: 'income' });
  for (const commodity of [currencyID, euro]) {
    await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
      transaction_date: movedOn, status: 'posted',
      journal_entries: [{ entry_date: movedOn, postings: [
        { account_id: receivable.id, commodity_id: commodity, quantity_value: '700', quantity_scale: 2 },
        { account_id: income.id, commodity_id: commodity, quantity_value: '-700', quantity_scale: 2 }
      ] }]
    });
  }
  const session = await apiJSON<{ id: number; candidates: Array<{ posting_id: number }> }>(page, 'POST', `/api/v1/accounts/${receivable.id}/reconciliations/start`, csrfToken, {
    commodity_id: currencyID, source_kind: 'statement', statement_date: movedOn, statement_balance_value: '700', statement_balance_scale: 2
  });
  await apiJSON(page, 'PATCH', `/api/v1/reconciliations/${session.id}`, csrfToken, { posting_version_ids: session.candidates.map(candidate => candidate.posting_id) });
  await apiJSON(page, 'POST', `/api/v1/reconciliations/${session.id}/finish`, csrfToken, { change_reason: 'statement verified' });

  await page.setViewportSize({ width: 320, height: 720 });
  await page.goto('/app');
  const widget = page.getByTestId('explore-widget-reconciliation');
  await expect(widget).toContainText('0 of 1 account reconciled');
  const text = widget.getByTestId('home-widget-row-text').filter({ hasText: 'Two-currency receivable' });
  await expect(text).toContainText('EUR: Never reconciled');
  await expect(text).toContainText('USD: Reconciled to');
  // Every state is on screen: the row wraps rather than clipping its finding.
  expect(await text.evaluate(element => element.scrollWidth - element.clientWidth)).toBeLessThanOrEqual(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);

  await text.getByRole('link', { name: 'Two-currency receivable' }).click();
  await expect(page).toHaveURL(new RegExp(`/app/ledger/reconcile\\?account=${receivable.id}&commodity=${euro}$`));
  const currency = page.getByLabel('Currency');
  await expect(currency.locator('option:checked')).toHaveText('EUR');
  await expect(page.getByText('This account has no default currency.')).toHaveCount(0);
  await page.getByLabel('Statement closing date').fill(movedOn);
  await page.getByLabel('Statement ending balance').fill('7.00');
  await expect(page.getByRole('button', { name: 'Start reconciling' })).toBeEnabled();
  await expectNoAccessibilityViolations(page, 'reconcile currency choice');
});

// #594 review: the statement's currency is fixed for the working copy. A
// refresh after another currency changes state never moves a typed balance
// to another currency, and a failed lookup is an error with a retry, not a
// request to change the account.
test('a statement in progress keeps its currency through a refresh, and a failed lookup offers a retry', async ({ page }) => {
  const { currencyID } = await readyForLedger(page);
  const { owner_today: today } = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
  const movedOn = shiftDays(today, -5);
  let csrfToken = await csrfTokenFor(page);
  const euro = await ensureCurrency(page, csrfToken, 'EUR', 'Euro');
  csrfToken = await csrfTokenFor(page);
  const receivable = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: 'Locked receivable', account_class: 'asset', account_kind: 'receivable', allows_postings: true, opened_on: movedOn
  });
  const income = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name: 'Locked fees', category_type: 'income' });
  for (const commodity of [currencyID, euro]) {
    await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
      transaction_date: movedOn, status: 'posted',
      journal_entries: [{ entry_date: movedOn, postings: [
        { account_id: receivable.id, commodity_id: commodity, quantity_value: '300', quantity_scale: 2 },
        { account_id: income.id, commodity_id: commodity, quantity_value: '-300', quantity_scale: 2 }
      ] }]
    });
  }
  const currencies = await apiJSON<{ currencies: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/currencies');
  const idFor = (code: string) => currencies.currencies.find(currency => currency.code === code)!.id;

  await page.goto(`/app/ledger/reconcile?account=${receivable.id}`);
  const currency = page.getByLabel('Currency');
  const first = (await currency.locator('option:checked').textContent())!.trim();
  expect(['EUR', 'USD']).toContain(first);
  await page.getByLabel('Statement closing date').fill(movedOn);
  await page.getByLabel('Statement ending balance').fill('3.00');

  // The first currency is reconciled elsewhere, which reorders the lookup.
  const session = await apiJSON<{ id: number; candidates: Array<{ posting_id: number }> }>(page, 'POST', `/api/v1/accounts/${receivable.id}/reconciliations/start`, csrfToken, {
    commodity_id: idFor(first), source_kind: 'statement', statement_date: movedOn, statement_balance_value: '300', statement_balance_scale: 2
  });
  await apiJSON(page, 'PATCH', `/api/v1/reconciliations/${session.id}`, csrfToken, { posting_version_ids: session.candidates.map(candidate => candidate.posting_id) });
  await apiJSON(page, 'POST', `/api/v1/reconciliations/${session.id}/finish`, csrfToken, { change_reason: 'statement verified' });
  const refetched = page.waitForResponse(response => response.url().endsWith(`/api/v1/accounts/${receivable.id}/reconciliation`) && response.ok());
  await page.clock.setFixedTime(new Date(Date.now() + 20_000));
  await page.evaluate(() => {
    window.dispatchEvent(new Event('offline'));
    window.dispatchEvent(new Event('online'));
  });
  await refetched;
  await expect(currency.locator('option:checked')).toHaveText(first);
  await expect(page.getByLabel('Statement ending balance')).toHaveValue('3.00');
  await expect(page.getByRole('button', { name: 'Start reconciling' })).toBeEnabled();

  // A failed lookup says so and retries; it never asks to change the account.
  let fail = true;
  await page.route(`**/api/v1/accounts/${receivable.id}/reconciliation`, route =>
    fail ? route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: { code: 'INTERNAL_ERROR', message: 'test refusal' } }) }) : route.continue());
  await page.clock.resume();
  await page.goto(`/app/ledger/reconcile?account=${receivable.id}`);
  await expect(page.locator('#reconcile-commodity-error')).toBeVisible();
  await expect(page.getByText('This account has no default currency.')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Start reconciling' })).toBeDisabled();
  await expectNoAccessibilityViolations(page, 'reconcile currency lookup failed');
  fail = false;
  await page.getByRole('button', { name: 'Try again' }).click();
  await expect(page.getByLabel('Currency')).toBeVisible();
  await expect(page.locator('#reconcile-commodity-error')).toHaveCount(0);
});
