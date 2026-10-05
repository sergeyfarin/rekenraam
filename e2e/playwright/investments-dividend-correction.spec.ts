import { expect, test, type Page } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * T-115: a posted cash dividend or reinvested dividend is corrected from the
 * transaction detail. The Go tests pin the ledger contract; these prove the
 * user can reach, review and complete each correction, including on a phone.
 */

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

type Setup = { csrfToken: string; currencyID: number; cashID: number; incomeID: number; suffix: string };

async function setup(page: Page, label: string): Promise<Setup> {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = `${label}${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Dividend cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const income = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Dividend income ${suffix}`, account_class: 'income', account_kind: 'income',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  return { csrfToken, currencyID, cashID: cash.id, incomeID: income.id, suffix };
}

// Navigate through the SvelteKit router, as an in-app link does: the query
// cache survives, unlike page.goto's full reload.
async function clientNavigate(page: Page, href: string): Promise<void> {
  await page.evaluate((target) => {
    const link = document.createElement('a');
    link.href = target;
    document.getElementById('app-root')!.appendChild(link);
    link.click();
    link.remove();
  }, href);
  await page.waitForURL((url) => `${url.pathname}${url.search}` === href);
}

async function chainEffective(page: Page, transactionID: number): Promise<number | null> {
  const chain = await apiJSON<{ effective_transaction_id: number | null }>(page, 'GET',
    `/api/v1/investments/transactions/${transactionID}/correction-chain`);
  return chain.effective_transaction_id;
}

test('a cash dividend is corrected, then the correction is reversed', async ({ page }) => {
  const s = await setup(page, 'CDV');
  const dividend = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/dividend', s.csrfToken, {
    transaction_date: daysFromTodayISO(-10), cash_account_id: s.cashID, cash_commodity_id: s.currencyID,
    income_account_id: s.incomeID, amount_value: '5000', amount_scale: 2, memo: `ACME ${s.suffix}`
  });

  await page.goto(`/app/transactions?transaction_id=${dividend.id}`);
  await page.getByRole('button', { name: 'Correct dividend…' }).click();
  const form = page.getByRole('dialog', { name: 'Correct this dividend' });
  await expect(form.getByLabel('Dividend amount')).toHaveValue('50.00');
  const submit = form.getByRole('button', { name: 'Review and replace' });
  await expect(submit).toBeDisabled();
  await form.getByLabel('Reason for correction').fill('statement shows 55.00');
  await form.getByLabel('Dividend amount').fill('55.00');
  await submit.click();
  await expect(form).toBeHidden();

  const replacement = await chainEffective(page, dividend.id);
  expect(replacement).not.toBeNull();
  expect(replacement).not.toBe(dividend.id);
  await page.goto(`/app/transactions?transaction_id=${replacement}`);
  await page.getByRole('button', { name: 'Reverse dividend…' }).click();
  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText('Reverse this dividend');
  await dialog.getByLabel('Reason for reversal').fill('paid in error');
  await dialog.getByRole('button', { name: 'Review and reverse' }).click();
  await expect(dialog).toBeHidden();
  await expect.poll(() => chainEffective(page, dividend.id)).toBeNull();
});

test.describe('on a phone', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('correcting a reinvested dividend shows the later sale gain it changes', async ({ page }) => {
    const s = await setup(page, 'RDV');
    const openedOn = daysFromTodayISO(-60);
    const name = `Reinvest ${s.suffix}`;
    const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', s.csrfToken, {
      commodity_code: s.suffix.toUpperCase(), instrument_type: 'stock', display_name: name, symbol: s.suffix.toUpperCase(),
      quote_commodity_id: s.currencyID, trading_commodity_id: s.currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
    });
    const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', s.csrfToken, {
      instrument_id: instrument.id, name: `Reinvest holding ${s.suffix}`, opened_on: openedOn, effective_from: openedOn
    });
    // 10 reinvested shares for 20.00, then 10 bought for 200.00; a FIFO sale
    // of 5 for 150.00 uses reinvested shares: basis 10.00, gain 140.00.
    const reinvestment = await apiJSON<{ transaction: { id: number } }>(page, 'POST', '/api/v1/investments/reinvested-dividend', s.csrfToken, {
      transaction_date: daysFromTodayISO(-30), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
      income_account_id: s.incomeID, quantity_value: '10', quantity_scale: 0, amount_value: '2000', amount_scale: 2,
      cash_commodity_id: s.currencyID
    });
    const trade = (side: 'buy' | 'sell', daysAgo: number, quantity: string, cash: string) =>
      apiJSON(page, 'POST', `/api/v1/investments/${side}`, s.csrfToken, {
        transaction_date: daysFromTodayISO(-daysAgo), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
        cash_account_id: s.cashID, quantity_value: quantity, quantity_scale: 0, cash_amount_value: cash,
        cash_amount_scale: 2, cash_commodity_id: s.currencyID, cost_basis_method: 'fifo'
      });
    await trade('buy', 20, '10', '20000');
    await trade('sell', 10, '5', '15000');

    // T-138: the gains read is cached before the correction and must not
    // survive it.
    const saleRow = page.locator('tbody tr', { hasText: name }).filter({ hasText: daysFromTodayISO(-10) });
    await page.goto('/app/investments');
    await page.getByRole('button', { name: 'Gains', exact: true }).click();
    await expect(saleRow).toContainText('140.00');
    await clientNavigate(page, `/app/transactions?transaction_id=${reinvestment.transaction.id}`);
    await page.getByRole('button', { name: 'Correct reinvested dividend…' }).click();
    const form = page.getByRole('dialog', { name: 'Correct this reinvested dividend' });
    await expect(form.getByLabel('Quantity')).toHaveValue('10');
    await form.getByLabel('Reason for correction').fill('broker restated the reinvestment');
    await form.getByLabel('Quantity').fill('4');
    await form.getByLabel('Dividend amount').fill('40.00');
    await form.getByRole('button', { name: 'Review and replace' }).click();

    const dialog = page.getByRole('alertdialog');
    await expect(dialog).toContainText('This changes gains on earlier sales');
    await expect(dialog).toContainText(`Sale on ${daysFromTodayISO(-10)}: basis 10.00 → 60.00, gain 140.00 → 90.00`);
    await dialog.getByRole('button', { name: 'Accept changed gains' }).click();
    await expect(dialog).toBeHidden();
    await expect(form).toBeHidden();
    await expect.poll(async () => {
      const gains = await apiJSON<{ realized: Array<{ commodity_id: number; realized_gain_value: string; realized_gain_scale: number }> }>(
        page, 'GET', '/api/v1/investments/gains');
      return gains.realized.filter((gain) => gain.commodity_id === instrument.commodity_id)
        .map((gain) => ((BigInt(gain.realized_gain_value) * 100n) / 10n ** BigInt(gain.realized_gain_scale)).toString());
    }).toEqual(['9000']);
    await clientNavigate(page, '/app/investments');
    await page.getByRole('button', { name: 'Gains', exact: true }).click();
    await expect(saleRow).toContainText('90.00');
    await expect(saleRow).not.toContainText('140.00');
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  });
});
