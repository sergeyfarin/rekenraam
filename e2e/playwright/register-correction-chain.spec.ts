import { expect, test, type Page } from '@playwright/test';
import { apiJSON } from './support/api';
import { expectNoAccessibilityViolations } from './support/a11y';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * T-120: the register keeps a correction's original, reversal and replacement
 * as separate rows and groups them with an expandable explanation. The Go tests
 * pin the read model; this proves the explanation is reachable and accessible
 * on a phone, by pointer and by keyboard, without opening the row.
 */

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

async function correctedDividendRegister(page: Page): Promise<string> {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = `RCC${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Chain cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const income = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Chain income ${suffix}`, account_class: 'income', account_kind: 'income',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const dividend = {
    transaction_date: daysFromTodayISO(-10), cash_account_id: cash.id, cash_commodity_id: currencyID,
    income_account_id: income.id, amount_value: '5000', amount_scale: 2, memo: `ACME ${suffix}`
  };
  const original = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/dividend', csrfToken, dividend);
  await apiJSON(page, 'POST', `/api/v1/investments/transactions/${original.id}/replace-dividend`, csrfToken, {
    reason: 'statement shows 55.00', replacement: { ...dividend, amount_value: '5500' }
  });
  return `/app/accounts/${cash.id}/register`;
}

test.describe('on a phone', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('a corrected dividend is grouped in the register with its net effect', async ({ page }) => {
    await page.goto(await correctedDividendRegister(page));
    const histories = page.getByText('Correction history (3 entries)');
    await expect(histories).toHaveCount(3);
    const badges = page.locator('summary');
    await expect(badges.getByText('Corrected', { exact: true })).toBeVisible();
    await expect(badges.getByText('Reversal', { exact: true })).toBeVisible();
    await expect(badges.getByText('Correction', { exact: true })).toBeVisible();

    // Opening the explanation does not open the row.
    const url = page.url();
    await histories.first().click();
    await expect(page.getByText('Reason: statement shows 55.00').first()).toBeVisible();
    await expect(page.getByText(/Net effect on this account: .*55\.00/).first()).toBeVisible();
    expect(page.url()).toBe(url);
    await expectNoAccessibilityViolations(page, 'register with an expanded correction chain');

    // The keyboard toggles it the same way: Enter on the summary, not the row.
    const summary = histories.last().locator('xpath=ancestor::summary');
    await summary.focus();
    await page.keyboard.press('Enter');
    const disclosure = summary.locator('xpath=ancestor::details');
    await expect(disclosure).toHaveJSProperty('open', true);
    await expect(disclosure.getByText('Reason: statement shows 55.00').first()).toBeVisible();
    expect(page.url()).toBe(url);
  });
});

