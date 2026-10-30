import { expect, test, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { todayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';
import { expectNoAccessibilityViolations } from './support/a11y';

/**
 * #186: investment activity is entered where the user is working. A holding
 * account's register and an instrument's view offer the investment record
 * forms with the holding and instrument already chosen, on a phone-sized
 * screen, and neither offers the generic editor that would write an
 * investment-linked posting.
 */

test.use({ viewport: { width: 390, height: 844 } });

async function setup(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const name = `Context Co ${suffix}`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: `CX${suffix}`, instrument_type: 'stock', display_name: name, symbol: `CX${suffix}`,
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 0, price_scale: 2, effective_from: todayISO()
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Context holding ${suffix}`, opened_on: todayISO(), effective_from: todayISO()
  });
  const cash = await createCashAccount(page, csrfToken, `Context cash ${suffix}`, currencyID);
  return { csrfToken, currencyID, suffix, name, instrument, holding, cash };
}

async function openActivity(page: Page, menuLabel: string, activityLabel: string) {
  await page.getByRole('button', { name: menuLabel }).click();
  await page.getByRole('button', { name: activityLabel, exact: true }).click();
}

test('a holding register records a pre-filled buy and opens it in the investment detail', async ({ page }) => {
  const { suffix, name, instrument, holding, cash } = await setup(page);
  const memo = `Context buy ${suffix}`;

  await page.goto(`/app/ledger/accounts/${holding.id}/register`);
  await expect(page.getByRole('heading', { name: `Context holding ${suffix}` })).toBeVisible();
  // The investment entry point replaces the generic one; wait for it so the
  // absence below is checked on a rendered register, not a loading one.
  await expect(page.getByRole('button', { name: 'Record investment activity' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Add transaction' })).toHaveCount(0);

  await openActivity(page, 'Record investment activity', 'Record buy');
  const dialog = page.getByRole('dialog');
  await expect(dialog.locator('#buy-instrument')).toHaveValue(name);
  await expect(dialog.locator('#buy-holding-account')).toHaveValue(String(holding.id));
  await dialog.locator('#buy-quantity').fill('5');
  await dialog.locator('#buy-cash-account').selectOption(String(cash.id));
  await dialog.locator('#buy-cash-amount').fill('100.00');
  await dialog.locator('#buy-memo').fill(memo);
  await dialog.getByRole('button', { name: 'Record buy' }).click();
  await expect(dialog).toBeHidden();

  const listed = await apiJSON<{ transactions: Array<{ description: string }> }>(
    page, 'GET', `/api/v1/transactions?commodity_id=${instrument.commodity_id}`);
  expect(listed.transactions.map((item) => item.description)).toEqual([memo]);

  // The register lists it without a reload, and the row opens the investment
  // detail with its correction and reversal history, not the generic editor.
  await page.getByText(memo).first().click();
  await expect(page.locator('#investment-correction-heading')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Edit transaction' })).toHaveCount(0);
});

test('an instrument view lists its activity and pre-fills the instrument and its only holding', async ({ page }) => {
  const { csrfToken, currencyID, suffix, name, instrument, holding, cash } = await setup(page);
  const memo = `Context seed buy ${suffix}`;
  await apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, {
    transaction_date: todayISO(), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: cash.id, quantity_value: '3', quantity_scale: 0,
    cash_amount_value: '6000', cash_amount_scale: 2, cash_commodity_id: currencyID, memo
  });

  await page.goto('/app/investments');
  await page.getByRole('link', { name: 'Instruments', exact: true }).click();
  await page.getByRole('link', { name: `View activity for ${name}` }).click();
  await expect(page).toHaveURL(new RegExp(`/app/investments/instruments/${instrument.commodity_id}$`));
  await expect(page.getByRole('heading', { name: new RegExp(name) })).toBeVisible();
  await expect(page.getByText(memo)).toBeVisible();
  await expect(page.getByText('+3')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Add transaction' })).toHaveCount(0);

  await page.getByRole('button', { name: 'Record investment activity' }).click();
  await expectNoAccessibilityViolations(page, 'instrument view investment entry');
  await page.getByRole('button', { name: 'Record sell', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.locator('#sell-instrument')).toHaveValue(name);
  await expect(dialog.locator('#sell-holding-account')).toHaveValue(String(holding.id));
});

const contextLocales: Array<[string, string, string]> = [
  ['en', 'Record investment activity', 'Back to investments'],
  ['de', 'Wertpapiervorgang erfassen', 'Zurück zur Geldanlage'],
  ['es', 'Registrar operación de inversión', 'Volver a inversiones'],
  ['fr', 'Enregistrer une opération de placement', 'Retour aux investissements'],
  ['nl', 'Beleggingsactiviteit vastleggen', 'Terug naar beleggingen'],
  ['ru', 'Записать инвестиционную операцию', 'Назад к инвестициям']
];
for (const [locale, menu, back] of contextLocales) {
  test(`investment entry in context is translated in ${locale}`, async ({ page }) => {
    const { holding, instrument } = await setup(page);
    await page.evaluate((value) => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    await page.goto(`/app/ledger/accounts/${holding.id}/register`);
    await expect(page.getByRole('button', { name: menu })).toBeVisible();
    await page.goto(`/app/investments/instruments/${instrument.commodity_id}`);
    await expect(page.getByRole('button', { name: menu })).toBeVisible();
    await expect(page.getByRole('link', { name: back })).toBeVisible();
  });
}

test('instrument activity includes a cash dividend with withholding and retains it after correction', async ({ page }) => {
  const { csrfToken, currencyID, suffix, instrument } = await setup(page);
  const cash = await createCashAccount(page, csrfToken, `Payer cash ${suffix}`, currencyID);
  const category = async (account_class: 'income' | 'expense') => apiJSON<{ id: number }>(
    page, 'POST', '/api/v1/accounts', csrfToken, {
      name: `Payer ${account_class} ${suffix}`, account_class, account_kind: account_class,
      default_commodity_id: currencyID, allows_postings: true, opened_on: todayISO(), effective_from: todayISO()
    });
  const income = await category('income');
  const withholding = await category('expense');
  const memo = `Payer dividend ${suffix}`;
  const dividend = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/dividend', csrfToken, {
    commodity_id: instrument.commodity_id, transaction_date: todayISO(),
    cash_account_id: cash.id, cash_commodity_id: currencyID, income_account_id: income.id,
    amount_value: '10000', amount_scale: 2, withholding_value: '1500', withholding_scale: 2,
    withholding_account_id: withholding.id, memo
  });
  await page.goto(`/app/investments/instruments/${instrument.commodity_id}`);
  // The dividend is listed here, so the old "appears on the cash register
  // instead" note must not come back on the loaded activity list.
  await expect(page.getByText(memo, { exact: true })).toBeVisible();
  await expect(page.getByText('Cash dividends move no units', { exact: false })).toHaveCount(0);
  await page.getByText(memo, { exact: true }).click();
  await page.getByRole('button', { name: 'Correct dividend…' }).click();
  const form = page.getByRole('dialog', { name: 'Correct this dividend' });
  await expect(form.getByLabel('Dividend amount')).toHaveValue('100.00');
  await expect(form.locator('#dividend-correction-withholding')).toHaveValue('15.00');
  await form.locator('#dividend-correction-withholding').fill('20.00');
  await form.getByLabel('Reason for correction').fill('correct withholding');
  await form.getByRole('button', { name: 'Review and replace' }).click();
  await expect(form).toBeHidden();
  const chain = await apiJSON<{
    effective_transaction_id: number;
    effective_dividend: { commodity_id: number; withholding_value: string };
  }>(page, 'GET', `/api/v1/investments/transactions/${dividend.id}/correction-chain`);
  expect(chain.effective_dividend.commodity_id).toBe(instrument.commodity_id);
  expect(chain.effective_dividend.withholding_value).toBe('2000');
  await page.goto(`/app/investments/instruments/${instrument.commodity_id}`);
  const activity = await apiJSON<{ transactions: Array<{ id: number }> }>(
    page, 'GET', `/api/v1/transactions?commodity_id=${instrument.commodity_id}`);
  expect(activity.transactions).toHaveLength(3);
  expect(activity.transactions.map((tx) => tx.id)).toContain(chain.effective_transaction_id);
  await expect(page.getByText(memo, { exact: true })).toHaveCount(3);
  await expectNoAccessibilityViolations(page, 'instrument cash dividend activity');
});


test('an instrument records, corrects and reverses separately paid withholding tax on a phone', async ({ page }) => {
  const { csrfToken, currencyID, suffix, instrument, cash, name } = await setup(page);
  const tax = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Separate tax ${suffix}`, account_class: 'expense', account_kind: 'expense',
    default_commodity_id: currencyID, allows_postings: true, opened_on: todayISO(), effective_from: todayISO()
  });
  const memo = `Separate withholding ${suffix}`;
  await page.goto(`/app/investments/instruments/${instrument.commodity_id}`);
  await openActivity(page, 'Record investment activity', 'Record withholding tax');
  const entry = page.getByRole('dialog', { name: 'Record separate withholding tax' });
  await expect(entry.locator('#div-instrument')).toHaveValue(name);
  await expect(entry.locator('#div-income-account')).toHaveCount(0);
  await entry.locator('#div-cash-account').selectOption(String(cash.id));
  await entry.getByLabel('Withholding amount').fill('15.00');
  await entry.locator('#div-withholding-account').selectOption(String(tax.id));
  await entry.getByLabel('Memo').fill(memo);
  await expectNoAccessibilityViolations(page, 'separate withholding entry');
  await entry.getByRole('button', { name: 'Record withholding tax' }).click();
  await expect(entry).toBeHidden();
  await page.getByText(memo, { exact: true }).click();
  await page.getByRole('button', { name: 'Correct withholding tax…' }).click();
  const form = page.getByRole('dialog', { name: 'Correct this withholding tax' });
  await expect(form.getByLabel('Withholding amount')).toHaveValue('15.00');
  await form.getByLabel('Withholding amount').fill('20.00');
  await form.getByLabel('Reason for correction').fill('statement tax correction');
  await form.getByRole('button', { name: 'Review and replace' }).click();
  await expect(form).toBeHidden();
  const activity = await apiJSON<{ transactions: Array<{ id: number; correction_of_transaction_id?: number }> }>(
    page, 'GET', `/api/v1/transactions?commodity_id=${instrument.commodity_id}`);
  expect(activity.transactions).toHaveLength(3);
  const original = activity.transactions.find((tx) => tx.correction_of_transaction_id === undefined)!;
  const chain = await apiJSON<{ effective_transaction_id: number; effective_dividend: { withholding_only: boolean } }>(
    page, 'GET', `/api/v1/investments/transactions/${original.id}/correction-chain`);
  expect(chain.effective_dividend.withholding_only).toBe(true);
  await page.goto(`/app/ledger/transactions?transaction_id=${chain.effective_transaction_id}`);
  await page.getByRole('button', { name: 'Reverse withholding tax…' }).click();
  const reversal = page.getByRole('alertdialog', { name: 'Reverse this withholding tax' });
  await reversal.getByLabel('Reason for reversal').fill('duplicate tax');
  await reversal.getByRole('button', { name: 'Review and reverse' }).click();
  await expect(reversal).toBeHidden();
  const reversed = await apiJSON<{ effective_transaction_id: number | null }>(page, 'GET',
    `/api/v1/investments/transactions/${original.id}/correction-chain`);
  expect(reversed.effective_transaction_id).toBeNull();
});
