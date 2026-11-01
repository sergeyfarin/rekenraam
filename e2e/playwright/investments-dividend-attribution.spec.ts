import { expect, test, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { daysFromTodayISO, todayISO } from './support/dates';
import { expectNoAccessibilityViolations } from './support/a11y';
import { readFileSync } from 'node:fs';
import { translationLocales } from './support/locales';

test.use({ viewport: { width: 390, height: 844 } });
test.describe.configure({ mode: 'parallel' });

type Chain = { effective_transaction_id: number; effective_dividend: { commodity_id?: number; withholding_only: boolean; amount_value: string; amount_scale: number } };

async function cents(page: Page, accountID: number): Promise<string> {
  const ledger = await apiJSON<{ transactions: Array<{ journal_entries: Array<{ postings: Array<{ account_id: number; quantity_value: string; quantity_scale: number }> }> }> }>(
    page, 'GET', `/api/v1/transactions?account_id=${accountID}`);
  let total = 0n;
  for (const transaction of ledger.transactions) for (const entry of transaction.journal_entries) for (const posting of entry.postings) {
    if (posting.account_id === accountID) total += BigInt(posting.quantity_value) * 100n / 10n ** BigInt(posting.quantity_scale);
  }
  return total.toString();
}

for (const theme of ['light', 'dark']) {
  test(`dividend submit is fully opaque as soon as it becomes enabled in ${theme}`, async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const copy = JSON.parse(readFileSync(new URL('../../frontend/messages/settings/ru.json', import.meta.url), 'utf8')) as Record<string, string>;
    const account = async (accountClass: string, kind: string) => apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
      name: `Opacity ${kind} ${Date.now()}`, account_class: accountClass, account_kind: kind,
      default_commodity_id: currencyID, allows_postings: true, opened_on: '2021-01-01'
    });
    const cash = await account('asset', 'brokerage_cash');
    const income = await account('income', 'income');
    await page.evaluate(({ theme }) => {
      localStorage.setItem('PARAGLIDE_LOCALE', 'ru');
      localStorage.setItem('rekenraam-theme', theme);
    }, { theme });
    await page.goto('/app/investments');
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
    await page.getByRole('button', { name: copy.investments_record_dividend, exact: true }).click();
    const entry = page.getByRole('dialog', { name: copy.investments_dividend_title, exact: true });
    await entry.locator('#div-cash-account').selectOption(String(cash.id));
    await entry.locator('#div-income-account').selectOption(String(income.id));
    await expect(entry.locator('button[type="submit"]')).toBeDisabled();

    // Change the last required field and measure in the same browser task,
    // after Svelte's microtask flush. A Playwright round trip can miss the
    // first frames of the old 150 ms disabled-opacity transition (#416).
    const enabled = await entry.evaluate(async dialog => {
      const submit = dialog.querySelector<HTMLButtonElement>('button[type="submit"]')!;
      const amount = dialog.querySelector<HTMLInputElement>('#div-amount')!;
      const disabledOpacity = getComputedStyle(submit).opacity;
      amount.value = '100';
      amount.dispatchEvent(new Event('input', { bubbles: true }));
      await Promise.resolve();
      return { disabledOpacity, disabled: submit.disabled, opacity: getComputedStyle(submit).opacity };
    });
    expect(enabled).toEqual({ disabledOpacity: '0.5', disabled: false, opacity: '1' });
    await expectNoAccessibilityViolations(page, `enabled dividend entry in ${theme}`);
  });
}

for (const locale of translationLocales) {
  for (const shape of ['dividend', 'embedded_tax', 'separate_tax']) {
  test(`${shape} payer entry, assignment and clearing work in ${locale}`, async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const copy = JSON.parse(readFileSync(new URL(`../../frontend/messages/settings/${locale}.json`, import.meta.url), 'utf8')) as Record<string, string>;
    const reference = new Date();
    const date = todayISO(reference);
    const opened = daysFromTodayISO(-30, reference);
    const suffix = `payer${locale}${Date.now()}`;
    const account = async (name: string, accountClass: string, kind: string) => apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
      name: `${name} ${suffix}`, account_class: accountClass, account_kind: kind, default_commodity_id: currencyID,
      allows_postings: true, opened_on: opened, effective_from: opened
    });
    const cash = await account('Cash', 'asset', 'brokerage_cash');
    const income = await account('Income', 'income', 'income');
    const tax = await account('Tax', 'expense', 'expense');
    const instrument = await apiJSON<{ commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
      commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: `Payer ${suffix}`, symbol: suffix.toUpperCase(),
      quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 0, price_scale: 2, effective_from: opened
    });
    await page.evaluate(value => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    const originalIDs: number[] = [];
    {
      const separate = shape === 'separate_tax';
      await page.goto('/app/investments');
      await page.getByRole('button', { name: separate ? copy.investments_record_withholding : copy.investments_record_dividend, exact: true }).click();
      const entry = page.getByRole('dialog', { name: separate ? copy.investments_withholding_title : copy.investments_dividend_title, exact: true });
      await expect(entry.locator('#div-instrument')).toHaveValue('');
      await expect(entry).toContainText(copy.investments_dividend_payer_help);
      await entry.locator('#div-date').fill(date);
      await entry.locator('#div-cash-account').selectOption(String(cash.id));
      await entry.locator('#div-amount').fill(separate ? '2' : '100');
      if (separate) await entry.locator('#div-withholding-account').selectOption(String(tax.id));
      else await entry.locator('#div-income-account').selectOption(String(income.id));
      if (shape === 'embedded_tax') {
        await entry.getByRole('checkbox').check();
        await entry.locator('#div-withholding').fill('15');
        await entry.locator('#div-withholding-account').selectOption(String(tax.id));
      }
      await expectNoAccessibilityViolations(page, `optional ${shape} payer entry`);
      const saved = page.waitForResponse(response => response.url().endsWith('/api/v1/investments/dividend') && response.request().method() === 'POST');
      await entry.getByRole('button', { name: separate ? copy.investments_record_withholding : copy.investments_dividend_submit, exact: true }).click();
      const created = await (await saved).json() as { id: number };
      await expect(entry).toBeHidden();
      originalIDs.push(created.id);
      let current = created.id;
      for (const clear of [false, true]) {
        await page.goto(`/app/ledger/transactions?transaction_id=${current}`);
        await page.getByRole('button', { name: separate ? copy.investments_withholding_correct_action : copy.transactions_investment_replace_dividend_action, exact: true }).click();
        const form = page.getByRole('dialog', { name: separate ? copy.investments_withholding_correct_title : copy.transactions_investment_replace_dividend_title, exact: true });
        await form.locator('#dividend-correction-payer').selectOption(clear ? '' : String(instrument.commodity_id));
        await form.getByLabel(copy.transactions_investment_replace_reason, { exact: true }).fill(clear ? 'Payer unknown' : 'Statement identifies payer');
        await expectNoAccessibilityViolations(page, `${shape} payer correction`);
        const replaced = page.waitForResponse(response => response.url().endsWith('/replace-dividend') && response.request().method() === 'POST');
        await form.getByRole('button', { name: copy.transactions_investment_replace_submit, exact: true }).click();
        const response = await replaced;
        expect(response.ok()).toBeTruthy();
        expect(response.request().postDataJSON().replacement.commodity_id).toBe(clear ? null : instrument.commodity_id);
        const result = await response.json() as { replacement_transaction: { id: number } };
        await expect(form).toBeHidden();
        current = result.replacement_transaction.id;
        const chain = await apiJSON<Chain>(page, 'GET', `/api/v1/investments/transactions/${created.id}/correction-chain`);
        expect(chain.effective_dividend.commodity_id).toBe(clear ? undefined : instrument.commodity_id);
        expect(chain.effective_dividend.withholding_only).toBe(separate);
        expect(BigInt(chain.effective_dividend.amount_value) * 100n / 10n ** BigInt(chain.effective_dividend.amount_scale)).toBe(separate ? 200n : 10000n);
      }
    }
    expect(await cents(page, cash.id)).toBe(shape === 'separate_tax' ? '-200' : shape === 'embedded_tax' ? '8500' : '10000');
    expect(await cents(page, income.id)).toBe(shape === 'separate_tax' ? '0' : '-10000');
    expect(await cents(page, tax.id)).toBe(shape === 'separate_tax' ? '200' : shape === 'embedded_tax' ? '1500' : '0');
    const coverage = await apiJSON<{ unassigned: Array<{ transaction_id: number; gross_state: string; withholding_state: string }> }>(page, 'GET', '/api/v1/investments/income-coverage');
    for (const id of originalIDs) {
      const chain = await apiJSON<Chain>(page, 'GET', `/api/v1/investments/transactions/${id}/correction-chain`);
      expect(coverage.unassigned.some(item => item.transaction_id === chain.effective_transaction_id)).toBeTruthy();
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  });
}
}
