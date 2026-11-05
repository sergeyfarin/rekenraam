import { expect, test, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { ensureCurrency, readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';
import { expectNoAccessibilityViolations } from './support/a11y';
import { readFileSync } from 'node:fs';
import { test as ownedBookTest } from './support/owned-book';
import { translationLocales } from './support/locales';

/**
 * #255: a short holder's payment in lieu of a dividend is recorded, corrected
 * and reversed on a phone-sized screen. It is charged to an expense account
 * and never changes the short position.
 */

test.use({ viewport: { width: 390, height: 844 } });

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

async function setup(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = `pil${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `PIL cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const expense = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Borrow costs ${suffix}`, account_class: 'expense', account_kind: 'expense',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const name = `Lent Co ${suffix}`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: name, symbol: suffix.toUpperCase(),
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Short holding ${suffix}`, opened_on: openedOn, effective_from: openedOn
  });
  await apiJSON(page, 'POST', '/api/v1/investments/short-sale', csrfToken, {
    transaction_date: daysFromTodayISO(-30), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: cash.id, quantity_value: '10', quantity_scale: 0, cash_amount_value: '100000',
    cash_amount_scale: 2, cash_commodity_id: currencyID
  });
  return { csrfToken, currencyID, name, cashID: cash.id, expenseID: expense.id, holdingID: holding.id, commodityID: instrument.commodity_id };
}

async function expenseBalance(page: Page, accountID: number): Promise<string> {
  const ledger = await apiJSON<{ transactions: Array<{ status: string; journal_entries: Array<{ postings: Array<{ account_id: number; quantity_value: string; quantity_scale: number }> }> }> }>(
    page, 'GET', `/api/v1/transactions?account_id=${accountID}`);
  let cents = 0n;
  for (const transaction of ledger.transactions) {
    for (const entry of transaction.journal_entries) {
      for (const posting of entry.postings) {
        if (posting.account_id === accountID) cents += (BigInt(posting.quantity_value) * 100n) / 10n ** BigInt(posting.quantity_scale);
      }
    }
  }
  return cents.toString();
}

test('a payment in lieu is recorded, corrected and reversed on mobile', async ({ page }) => {
  const s = await setup(page);
  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Record payment in lieu' }).click();
  const form = page.getByRole('dialog', { name: 'Payment in lieu of dividend' });
  await form.getByLabel('Short holding').selectOption(String(s.holdingID));
  await form.getByLabel('Instrument').selectOption(String(s.commodityID));
  await form.getByLabel('Ex-dividend date').fill(daysFromTodayISO(-10));
  await form.getByLabel('Payment date').fill(daysFromTodayISO(-8));
  await form.getByLabel('Cash account').selectOption(String(s.cashID));
  await form.getByLabel(/Amount owed/).fill('5.00');
  await form.getByLabel('Expense account').selectOption(String(s.expenseID));
  await expectNoAccessibilityViolations(page, 'payment in lieu form');
  await form.getByRole('button', { name: 'Record payment' }).click();
  await expect(form).toBeHidden();
  await expect.poll(() => expenseBalance(page, s.expenseID)).toBe('500');
  // The short position is unchanged: still ten units owed.
  await expect(page.getByRole('button', { name: new RegExp(s.name) })).toContainText('-10');

  const ledger = await apiJSON<{ transactions: Array<{ id: number }> }>(page, 'GET', `/api/v1/transactions?account_id=${s.expenseID}`);
  await page.goto(`/app/ledger/transactions?transaction_id=${ledger.transactions[0].id}`);
  await page.getByRole('button', { name: 'Correct payment in lieu…' }).click();
  const correction = page.getByRole('dialog', { name: 'Correct payment in lieu' });
  await expect(correction.getByLabel(/Amount owed/)).toHaveValue('5.00');
  await correction.getByLabel('Reason for correction').fill('broker charged 6.00');
  await correction.getByLabel(/Amount owed/).fill('6.00');
  await correction.getByRole('button', { name: 'Save correction' }).click();
  await expect(correction).toBeHidden();
  await expect.poll(() => expenseBalance(page, s.expenseID)).toBe('600');

  const chain = await apiJSON<{ effective_transaction_id: number }>(page, 'GET',
    `/api/v1/investments/transactions/${ledger.transactions[0].id}/correction-chain`);
  await page.goto(`/app/ledger/transactions?transaction_id=${chain.effective_transaction_id}`);
  await page.getByRole('button', { name: 'Reverse payment in lieu…' }).click();
  await page.getByLabel('Reason for reversal').fill('charged in error');
  await page.getByRole('button', { name: 'Review and reverse' }).click();
  await expect.poll(() => expenseBalance(page, s.expenseID)).toBe('0');
});

// #395: a correction moves the payment to another date and a cash account in
// another currency; the original is cancelled where it was posted.
test('a payment in lieu correction changes its date, cash account and currency on mobile', async ({ page }) => {
  const s = await setup(page);
  const nok = await ensureCurrency(page, s.csrfToken, 'NOK', 'Norwegian krone');
  const openedOn = daysFromTodayISO(-60);
  const broker = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', s.csrfToken, {
    name: `PIL broker NOK ${Date.now()}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: nok, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const anyExpense = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', s.csrfToken, {
    name: `Borrow costs any ${Date.now()}`, account_class: 'expense', account_kind: 'expense',
    allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const payment = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/payment-in-lieu', s.csrfToken, {
    holding_account_id: s.holdingID, commodity_id: s.commodityID, ex_date: daysFromTodayISO(-10),
    transaction_date: daysFromTodayISO(-8), cash_account_id: s.cashID, cash_commodity_id: s.currencyID,
    expense_account_id: anyExpense.id, amount_value: '500', amount_scale: 2
  });

  await page.goto(`/app/ledger/transactions?transaction_id=${payment.id}`);
  await page.getByRole('button', { name: 'Correct payment in lieu…' }).click();
  const correction = page.getByRole('dialog', { name: 'Correct payment in lieu' });
  await expect(correction.getByLabel('Payment date')).toHaveValue(daysFromTodayISO(-8));
  await correction.getByLabel('Reason for correction').fill('the broker charged it in NOK a day later');
  await correction.getByLabel('Payment date').fill(daysFromTodayISO(-7));
  await correction.getByLabel('Cash account').selectOption(String(broker.id));
  await expect(correction.getByText('(NOK)').first()).toBeVisible();
  await correction.getByLabel(/Amount owed/).fill('4.80');
  await correction.getByRole('button', { name: 'Save correction' }).click();
  await expect(correction).toBeHidden();

  await expect.poll(async () => (await apiJSON<{ effective_transaction_id: number | null }>(page, 'GET',
    `/api/v1/investments/transactions/${payment.id}/correction-chain`)).effective_transaction_id).not.toBe(payment.id);
  const chain = await apiJSON<{ effective_payment_in_lieu: { event_date: string; cash_account_id: number; cash_commodity_id: number;
    amount_value: string; amount_scale: number; ex_date: string | null } }>(page, 'GET',
    `/api/v1/investments/transactions/${payment.id}/correction-chain`);
  expect(chain.effective_payment_in_lieu).toMatchObject({ event_date: daysFromTodayISO(-7), cash_account_id: broker.id,
    cash_commodity_id: nok, amount_value: '480', amount_scale: 2, ex_date: daysFromTodayISO(-10) });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
});

test('unsupported short attribution leaves recorded cash effective with a warning', async ({ page }) => {
  const s = await setup(page);
  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Record payment in lieu' }).click();
  const form = page.getByRole('dialog', { name: 'Payment in lieu of dividend' });
  await form.getByLabel('Short holding').selectOption(String(s.holdingID));
  await form.getByLabel('Instrument').selectOption(String(s.commodityID));
  // The owner knows the cash charge; historical short support is advisory.
  await form.getByLabel('Ex-dividend date').fill(daysFromTodayISO(-40));
  await form.getByLabel('Payment date').fill(daysFromTodayISO(-38));
  await form.getByLabel('Cash account').selectOption(String(s.cashID));
  await form.getByLabel(/Amount owed/).fill('5.00');
  await form.getByLabel('Expense account').selectOption(String(s.expenseID));
  await form.getByRole('button', { name: 'Record payment' }).click();
  await expect(form).toBeHidden();
  await expect.poll(() => expenseBalance(page, s.expenseID)).toBe('500');
  const ledger = await apiJSON<{ transactions: Array<{ id: number }> }>(page, 'GET', `/api/v1/transactions?account_id=${s.expenseID}`);
  await page.goto(`/app/ledger/transactions?transaction_id=${ledger.transactions[0].id}`);
  await expect(page.getByTestId('investment-attribution-diagnostic')).toContainText('No short units');
});

// #389's mixed-severity rendering uses explicit response fixtures. Real owner
// entry/repair/clearing is exercised separately below under #388.
for (const locale of translationLocales) {
  test(`attribution warnings and unavailable coverage stay visible in ${locale}`, async ({ page }) => {
    const s = await setup(page);
    const payment = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/payment-in-lieu', s.csrfToken, {
      holding_account_id: s.holdingID, commodity_id: s.commodityID,
      ex_date: daysFromTodayISO(-20), transaction_date: daysFromTodayISO(-18),
      cash_account_id: s.cashID, cash_commodity_id: s.currencyID, expense_account_id: s.expenseID,
      amount_value: '5000', amount_scale: 2
    });
    const copy = JSON.parse(readFileSync(new URL(`../../frontend/messages/settings/${locale}.json`, import.meta.url), 'utf8')) as Record<string, string>;
    const warning = { transaction_id: payment.id, reason: 'no_short_at_entitlement_close' };
    const failed = locale !== 'en';
    await page.route('**/api/v1/maintenance/self-check', async route => {
      await route.fulfill({ json: { has_run: true, read_only: true, latest_run: {
        id: 1, trigger: 'manual', status: failed ? 'failed' : 'warning',
        failed_check_count: failed ? 1 : 0, warning_check_count: 1, incomplete_check_count: 1,
        error_summary: '', started_at: '2026-10-23T00:00:00Z', finished_at: '2026-10-23T00:00:01Z',
        results: [{ check_id: 'investment_attribution', status: 'warning', finding_count: 0,
          sample: [], warning_count: 1, warning_sample: [warning], unavailable_count: 1,
          unavailable_sample: [payment.id], summary: '', explanation: '', next_step: '' },
          ...(failed ? [{ check_id: 'entry_balance', status: 'failed', finding_count: 1,
            sample: [payment.id], warning_count: 0, warning_sample: [], unavailable_count: 0,
            unavailable_sample: [], summary: 'Entry balance', explanation: '', next_step: '' }] : [])]
      } } });
    });
    await page.route(`**/api/v1/investments/transactions/${payment.id}/correction-chain`, async route => {
      const response = await route.fetch();
      const chain = await response.json();
      chain.effective_payment_in_lieu.attribution = { status: 'warning', reasons: [warning.reason], unavailable: false };
      await route.fulfill({ response, json: chain });
    });
    await page.evaluate(value => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    await page.goto('/app/settings/data');
    const summary = page.getByTestId('selfcheck-summary');
    await expect(summary).toContainText(copy.settings_data_selfcheck_warning.replace('{count}', '1'));
    await expect(summary).toContainText(copy.settings_data_selfcheck_failed_unavailable.replace('{count}', '1'));
    await expect(page.getByTestId('selfcheck-warnings')).toHaveText(copy.settings_data_selfcheck_warnings.replace('{count}', '1'));
    await expect(page.getByTestId('selfcheck-unavailable')).toHaveText(copy.settings_data_selfcheck_unavailable.replace('{count}', '1'));
    const link = page.locator(`a[href="/app/ledger/transactions?transaction_id=${payment.id}"]`).first();
    await expect(link).toHaveAccessibleName(copy.investments_attribution_payment_link.replace('{id}', String(payment.id)));
    await expectNoAccessibilityViolations(page);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
    await link.click();
    await expect(page.getByTestId('investment-attribution-diagnostic')).toContainText(copy.investments_attribution_no_short);
    await expect(page.getByTestId('investment-attribution-diagnostic')).toContainText(copy.investments_attribution_review);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  });
}

// The journey ends on the book-wide self-check, so it owns its book (#398):
// the cleared payment is the run's only unavailable record, and the run's
// sample, count and link name it whatever other specs have written.
for (const locale of translationLocales) {
  ownedBookTest(`optional payment attribution is recorded, repaired and cleared in ${locale}`, async ({ page }) => {
    const s = await setup(page);
    const copy = JSON.parse(readFileSync(new URL(`../../frontend/messages/settings/${locale}.json`, import.meta.url), 'utf8')) as Record<string, string>;
    await page.evaluate(value => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    await page.goto('/app/investments');
    await page.getByRole('button', { name: copy.investments_record_payment_in_lieu, exact: true }).click();
    const entry = page.getByRole('dialog', { name: copy.investments_payment_in_lieu_title, exact: true });
    await expect(entry.locator('#pil-ex-date')).toHaveValue('');
    await entry.locator('#pil-cash').selectOption(String(s.cashID));
    await entry.locator('#pil-expense').selectOption(String(s.expenseID));
    await entry.locator('#pil-amount').fill('5');
    const createdResponse = page.waitForResponse(response => response.url().endsWith('/api/v1/investments/payment-in-lieu') && response.request().method() === 'POST');
    await entry.getByRole('button', { name: copy.investments_payment_in_lieu_submit, exact: true }).click();
    const created = await (await createdResponse).json() as { id: number };
    await expect(entry).toBeHidden();
    await page.goto(`/app/ledger/transactions?transaction_id=${created.id}`);
    await expect(page.getByTestId('investment-attribution-diagnostic')).toContainText(copy.investments_attribution_unavailable);
    const replace = async (clear: boolean) => {
      await page.getByRole('button', { name: copy.investments_payment_in_lieu_correct_action, exact: true }).click();
      const form = page.getByRole('dialog', { name: copy.investments_payment_in_lieu_correct_title, exact: true });
      await form.locator('#pil-holding').selectOption(clear ? '' : String(s.holdingID));
      if (!clear) await expect(form.locator('#pil-instrument')).toHaveValue('');
      await form.locator('#pil-instrument').selectOption(clear ? '' : String(s.commodityID));
      await form.locator('#pil-ex-date').fill(clear ? '' : daysFromTodayISO(-10));
      await form.getByLabel(copy.investments_payment_in_lieu_correction_reason, { exact: true }).fill(clear ? 'Details unknown' : 'Statement attribution');
      await expectNoAccessibilityViolations(page, 'payment attribution correction');
      const saved = page.waitForResponse(response => response.url().endsWith('/replace-payment-in-lieu') && response.request().method() === 'POST');
      await form.getByRole('button', { name: copy.investments_payment_in_lieu_correct_submit, exact: true }).click();
      const replacement = await (await saved).json() as { replacement_transaction: { id: number } };
      // The workspace closes the detail after its list refresh completes.
      await expect(page.locator('aside[aria-label]')).toBeHidden();
      return replacement.replacement_transaction.id;
    };
    const supported = await replace(false);
    await page.goto(`/app/ledger/transactions?transaction_id=${supported}`);
    await expect(page.getByRole('button', { name: copy.investments_payment_in_lieu_correct_action, exact: true })).toBeVisible();
    await expect(page.getByTestId('investment-attribution-diagnostic')).toBeHidden();
    const cleared = await replace(true);
    await page.goto(`/app/ledger/transactions?transaction_id=${cleared}`);
    await expect(page.getByTestId('investment-attribution-diagnostic')).toContainText(copy.investments_attribution_unavailable);
    await expect.poll(() => expenseBalance(page, s.expenseID)).toBe('500');
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
    await page.goto('/app/settings/data');
    const ran = page.waitForResponse(response => response.url().endsWith('/api/v1/maintenance/self-check') && response.request().method() === 'POST');
    await page.getByTestId('selfcheck-run').click();
    const run = await (await ran).json() as { results: Array<{ check_id: string; unavailable_count: number; unavailable_sample: number[] }> };
    const attribution = run.results.find(row => row.check_id === 'investment_attribution');
    expect(attribution?.unavailable_count).toBe(1);
    expect(attribution?.unavailable_sample).toEqual([cleared]);
    await expect(page.getByTestId('selfcheck-unavailable')).toHaveText(copy.settings_data_selfcheck_unavailable.replace('{count}', '1'));
    await page.locator(`a[href="/app/ledger/transactions?transaction_id=${cleared}"]`).click();
    await expect(page.getByTestId('investment-attribution-diagnostic')).toContainText(copy.investments_attribution_unavailable);
  });
}

// #398: self-check samples stop at 20 records in operation order, so in a book
// with earlier unrelated findings a new record's own sample entry is not
// guaranteed. Saturate an owned book on purpose: the count still covers the
// journey's payment, the sample does not reach it, and the per-transaction
// diagnostic — which shared-book specs assert instead — still names it.
ownedBookTest('a saturated self-check sample leaves the per-transaction diagnostic owned', async ({ page }) => {
  const s = await setup(page);
  const unattributed = (days: number) => apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/payment-in-lieu', s.csrfToken, {
    transaction_date: daysFromTodayISO(days), cash_account_id: s.cashID, cash_commodity_id: s.currencyID,
    expense_account_id: s.expenseID, amount_value: '100', amount_scale: 2
  });
  const unrelated: number[] = [];
  for (let index = 0; index < 21; index++) unrelated.push((await unattributed(-25)).id);
  const own = await unattributed(-5);

  await page.goto('/app/settings/data');
  const ran = page.waitForResponse(response => response.url().endsWith('/api/v1/maintenance/self-check') && response.request().method() === 'POST');
  await page.getByTestId('selfcheck-run').click();
  const run = await (await ran).json() as { results: Array<{ check_id: string; unavailable_count: number; unavailable_sample: number[] }> };
  const attribution = run.results.find(row => row.check_id === 'investment_attribution');
  expect(attribution?.unavailable_count).toBe(22);
  expect(attribution?.unavailable_sample).toEqual(unrelated.slice(0, 20));
  await expect(page.getByTestId('selfcheck-unavailable')).toHaveText('Not verified: 22 items with unknown information.');
  await expect(page.locator(`a[href="/app/ledger/transactions?transaction_id=${own.id}"]`)).toHaveCount(0);

  await page.goto(`/app/ledger/transactions?transaction_id=${own.id}`);
  await expect(page.getByTestId('investment-attribution-diagnostic')).toContainText('Some attribution context is missing');
});
