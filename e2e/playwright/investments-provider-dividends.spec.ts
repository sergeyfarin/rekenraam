import type { Page } from '@playwright/test';
import { expect, test } from './support/fixtures';
import process from 'node:process';
import { randomUUID } from 'node:crypto';
import { apiJSON } from './support/api';
import { readyForLedger, createCashAccount } from './support/ledger';
import { daysFromTodayISO } from './support/dates';
import { expectNoAccessibilityViolations } from './support/a11y';
import { setProviderDividendDate } from './support/provider-dividends';

// #111: a provider dividend announcement is staged through the durable queue
// and posts only after the owner completes it from the statement, on a phone.
test.use({ viewport: { width: 390, height: 844 } });
test.skip(Boolean(process.env.E2E_BASE_URL), 'needs the local market-data stub for Twelve Data dividends');

async function heldProviderListing(page: Page, prefix: string, side: 'long' | 'short' = 'long') {
  const { csrfToken, currencyID } = await readyForLedger(page);
  // Capture one host-calendar reference for every date in this scenario. The
  // stub gets the chosen ex-date explicitly, even across midnight/DST (#386).
  const referenceDate = new Date();
  const suffix = randomUUID().replaceAll('-', '').slice(0, 13);
  const openedOn = daysFromTodayISO(-60, referenceDate);
  const exDate = daysFromTodayISO(-10, referenceDate);
  const paymentDate = daysFromTodayISO(-3, referenceDate);
  const cash = await createCashAccount(page, csrfToken, `Provider cash ${suffix}`, currencyID, openedOn);
  const income = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Provider income ${suffix}`, account_class: 'income', account_kind: 'income',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const symbol = `${prefix}${suffix}`;
  await setProviderDividendDate(symbol, exDate);
  const name = `Provider payer ${suffix}`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: symbol, instrument_type: 'stock', display_name: name, symbol, mic: 'XNYS',
    identifiers: { twelve_data_symbol: symbol },
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 0, price_scale: 2, effective_from: openedOn
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Provider holding ${suffix}`, opened_on: openedOn, effective_from: openedOn
  });
  await apiJSON(page, 'POST', side === 'long' ? '/api/v1/investments/buy' : '/api/v1/investments/short-sale', csrfToken, {
    transaction_date: daysFromTodayISO(-30, referenceDate), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: cash.id, quantity_value: '10', quantity_scale: 0, cash_amount_value: '100000',
    cash_amount_scale: 2, cash_commodity_id: currencyID
  });
  const sources = await apiJSON<{ sources: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/pricing/sources');
  const twelveData = sources.sources.find((source) => source.code === 'twelve_data');
  if (!twelveData) throw new Error('expected the Twelve Data source');
  await apiJSON(page, 'POST', '/api/v1/pricing/source-assignments', csrfToken, {
    commodity_id: instrument.commodity_id, quote_commodity_id: currencyID, source_id: twelveData.id, effective_from: openedOn
  });

  return { csrfToken, currencyID, cash, income, instrument, holding, name, openedOn, exDate, paymentDate };
}

test('[acceptance] a provider dividend is reviewed, completed and posted through the native command', async ({ page }) => {
  const { cash, income, instrument, name, exDate, paymentDate } = await heldProviderListing(page, 'E2EDIV');
  await page.goto('/app/investments');
  await page.getByRole('link', { name: /^Suggestions/ }).click();
  await page.getByRole('button', { name: 'Check provider dividends', exact: true }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Dividend check queued for listings:' })).toBeVisible();

  const card = page.getByTestId('event-suggestion').filter({ hasText: name });
  await expect(card).toBeVisible({ timeout: 30_000 });
  await expect(card.getByText(`Ex-date ${exDate}`, { exact: true })).toBeVisible();
  await expect(card.getByText('Reported in the listing currency USD; the payment currency is not stated.')).toBeVisible();
  await expect(card).toContainText('You supply: payment date, cash account, amount received, payment currency');
  // Nothing posts until the owner completes it: no Accept on an incomplete proposal.
  await expect(card.getByRole('button', { name: 'Complete and post', exact: true })).toBeVisible();
  await expect(card.getByRole('button', { name: 'Accept', exact: true })).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await expectNoAccessibilityViolations(page, 'Provider dividend suggestion on a phone');

  await card.getByRole('button', { name: 'Complete and post', exact: true }).click();
  const form = card.getByRole('form', { name: 'Dividend from your statement' });
  await expect(form.getByLabel('Payment date')).toHaveValue(exDate);
  await form.getByLabel('Payment date').fill(paymentDate);
  await form.getByLabel('Cash account').selectOption(String(cash.id));
  await form.getByLabel(/^Income account/).selectOption(String(income.id));
  // 10 units × 0.50 in the cash currency pre-fills the gross for confirmation.
  await expect(form.getByLabel(/Gross amount received/)).toHaveValue('5.00');
  await expect(form.getByText('Pre-filled from the estimate; check it against your statement.')).toBeVisible();
  await form.getByLabel(/Gross amount received/).fill('4.80');
  await expectNoAccessibilityViolations(page, 'Provider dividend completion on a phone');
  await form.getByRole('button', { name: 'Post dividend', exact: true }).click();

  const settled = page.getByRole('listitem').filter({ hasText: name }).filter({ hasText: 'Accepted' });
  await expect(settled).toBeVisible();
  await expect(card).toHaveCount(0);

  await page.goto(`/app/investments/instruments/${instrument.commodity_id}`);
  await expect(page.getByText(/^Gross income: .*4\.80/)).toBeVisible();
});

test('a provider revision of a staged dividend is disclosed without reopening the decision', async ({ page }) => {
  // #254: the stub revises E2EDIVREV listings from 0.50 to 0.55 on the second fetch.
  const { name, exDate } = await heldProviderListing(page, 'E2EDIVREV');
  await page.goto('/app/investments');
  await page.getByRole('link', { name: /^Suggestions/ }).click();
  const check = page.getByRole('button', { name: 'Check provider dividends', exact: true });
  await check.click();
  const card = page.getByTestId('event-suggestion').filter({ hasText: name });
  await expect(card).toBeVisible({ timeout: 30_000 });
  await expect(card.getByText(`Ex-date ${exDate}`, { exact: true })).toBeVisible();
  await expect(card.getByTestId('event-suggestion-revisions')).toHaveCount(0);

  await check.click();
  const notice = card.getByTestId('event-suggestion-revisions');
  await expect(notice).toBeVisible({ timeout: 30_000 });
  await expect(notice).toContainText('Provider revised this announcement');
  await expect(notice).toContainText(/0\.5 USD → 0\.55 USD/);
  await expect(notice).toContainText('The suggestion keeps the first announcement; complete it from your statement.');
  await expect(page.getByTestId('event-suggestion').filter({ hasText: name })).toHaveCount(1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await expectNoAccessibilityViolations(page, 'Provider dividend revision on a phone');

  await card.getByRole('button', { name: 'Ignore', exact: true }).click();
  const settled = page.getByRole('listitem').filter({ hasText: name }).filter({ hasText: 'Ignored' });
  await expect(settled).toBeVisible();
  await expect(settled.getByTestId('event-suggestion-revisions')).toContainText('Your decision and any recorded dividend are unchanged.');
});

test('a short-only provider dividend links the payment in lieu the owner already recorded', async ({ page }) => {
  // #258: the book is short at the ex-date, so the suggestion is a payment in
  // lieu; the owner recorded it by hand and links it instead of posting twice.
  const { csrfToken, currencyID, cash, instrument, holding, name, openedOn, exDate, paymentDate } = await heldProviderListing(page, 'E2EDIVPIL', 'short');
  const expense = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Borrow costs ${name}`, account_class: 'expense', account_kind: 'expense',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const payment = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/payment-in-lieu', csrfToken, {
    holding_account_id: holding.id, commodity_id: instrument.commodity_id, ex_date: exDate,
    transaction_date: paymentDate, cash_account_id: cash.id, cash_commodity_id: currencyID,
    expense_account_id: expense.id, amount_value: '500', amount_scale: 2
  });

  await page.goto('/app/investments');
  await page.getByRole('link', { name: /^Suggestions/ }).click();
  await page.getByRole('button', { name: 'Check provider dividends', exact: true }).click();
  const card = page.getByTestId('event-suggestion').filter({ hasText: name });
  await expect(card).toBeVisible({ timeout: 30_000 });
  await expect(card.getByText(`Ex-date ${exDate}`, { exact: true })).toBeVisible();
  await expect(card.getByRole('note')).toContainText('payment in lieu');
  const candidate = card.getByTestId('event-suggestion-link-candidate');
  await expect(candidate).toHaveCount(1);
  await expect(candidate).toContainText(paymentDate);
  await expect(candidate).toContainText('5');
  await expect(candidate).toContainText(`Provider holding`);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await expectNoAccessibilityViolations(page, 'Provider payment in lieu link candidate on a phone');

  await candidate.getByRole('button', { name: 'Link', exact: true }).click();
  const settled = page.getByRole('listitem').filter({ hasText: name }).filter({ hasText: 'Linked' });
  await expect(settled).toBeVisible();
  await expect(card).toHaveCount(0);
  const suggestions = await apiJSON<{ suggestions: Array<{ status: string; linked_transaction_id?: number; instrument_id?: number }> }>(
    page, 'GET', '/api/v1/investments/event-suggestions');
  const linked = suggestions.suggestions.find((s) => s.instrument_id === instrument.id);
  expect(linked?.status).toBe('linked');
  expect(linked?.linked_transaction_id).toBe(payment.id);
});

test('a repaired dividend shows source mismatch and keeps its original provider evidence', async ({ page }) => {
  const s = await heldProviderListing(page, 'E2EDIVPAYER');
  await page.goto('/app/investments');
  await page.getByRole('link', { name: /^Suggestions/ }).click();
  await page.getByRole('button', { name: 'Check provider dividends', exact: true }).click();
  const card = page.getByTestId('event-suggestion').filter({ hasText: s.name });
  await expect(card).toBeVisible({ timeout: 30_000 });
  const suggestions = await apiJSON<{ suggestions: Array<{ id: number; instrument_id: number }> }>(page, 'GET', '/api/v1/investments/event-suggestions');
  const suggestion = suggestions.suggestions.find(item => item.instrument_id === s.instrument.id)!;
  const accepted = await apiJSON<{ generated_transaction_id: number }>(page, 'POST', `/api/v1/investments/event-suggestions/${suggestion.id}/accept`, s.csrfToken, {
    completion: { transaction_date: s.paymentDate, cash_account_id: s.cash.id, cash_commodity_id: s.currencyID,
      income_account_id: s.income.id, amount_value: '480', amount_scale: 2 }
  });
  const other = await apiJSON<{ commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', s.csrfToken, {
    commodity_code: `OTHER${s.instrument.id}`, instrument_type: 'stock', display_name: 'Different payer',
    quote_commodity_id: s.currencyID, trading_commodity_id: s.currencyID, quantity_scale: 0, price_scale: 2, effective_from: s.openedOn
  });
  const replacement = await apiJSON<{ replacement_transaction: { id: number } }>(page, 'POST', `/api/v1/investments/transactions/${accepted.generated_transaction_id}/replace-dividend`, s.csrfToken, {
    reason: 'Different payer on statement', replacement: { transaction_date: s.paymentDate, commodity_id: other.commodity_id,
      cash_account_id: s.cash.id, cash_commodity_id: s.currencyID, income_account_id: s.income.id, amount_value: '480', amount_scale: 2 }
  });
  await page.goto(`/app/ledger/transactions?transaction_id=${replacement.replacement_transaction.id}`);
  await expect(page.getByTestId('investment-attribution-diagnostic')).toContainText('These details differ from the linked provider evidence');
  await expectNoAccessibilityViolations(page, 'dividend source mismatch');
  await page.getByRole('button', { name: 'Correct dividend…', exact: true }).click();
  const form = page.getByRole('dialog', { name: 'Correct this dividend', exact: true });
  await form.locator('#dividend-correction-payer').selectOption('');
  await form.getByLabel('Reason for correction', { exact: true }).fill('Payer is unknown');
  const saved = page.waitForResponse(response => response.url().endsWith('/replace-dividend') && response.request().method() === 'POST');
  await form.getByRole('button', { name: 'Review and replace', exact: true }).click();
  const cleared = await (await saved).json() as { replacement_transaction: { id: number } };
  await expect(form).toBeHidden();
  await page.goto(`/app/ledger/transactions?transaction_id=${cleared.replacement_transaction.id}`);
  await expect(page.getByTestId('investment-attribution-diagnostic')).toContainText('Some attribution context is missing');
  await expect(page.getByTestId('investment-attribution-diagnostic')).not.toContainText('These details differ');
  await page.goto('/app/investments');
  await page.getByRole('link', { name: /^Suggestions/ }).click();
  const history = page.getByRole('listitem').filter({ hasText: s.name }).filter({ hasText: 'Accepted' });
  await expect(history.getByTestId('settled-event-evidence')).toContainText('Announced per share');
  await expect(history.getByTestId('settled-event-evidence')).toContainText('0.5');
  await expect(history.locator(`a[href="/app/ledger/transactions?transaction_id=${accepted.generated_transaction_id}"]`)).toBeVisible();
  await expect(history).not.toContainText('Different payer');
  const coverage = await apiJSON<{ unassigned: Array<{ transaction_id: number; cash: { value: string; scale: number } }> }>(page, 'GET', '/api/v1/investments/income-coverage');
  const item = coverage.unassigned.find(row => row.transaction_id === cleared.replacement_transaction.id)!;
  expect(BigInt(item.cash.value) * 100n / 10n ** BigInt(item.cash.scale)).toBe(480n);
});
