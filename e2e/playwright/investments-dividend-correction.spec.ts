import { expect, test, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { ensureCurrency, readyForLedger } from './support/ledger';
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

  await page.goto(`/app/ledger/transactions?transaction_id=${dividend.id}`);
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
  await page.goto(`/app/ledger/transactions?transaction_id=${replacement}`);
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

  // R13 D2: an ex date recorded later by correction, so returns recognise the
  // income on it.
  test('a correction records the ex date a cash dividend was paid for', async ({ page }) => {
    const s = await setup(page, 'EXD');
    const paidOn = daysFromTodayISO(-10);
    const exDate = daysFromTodayISO(-20);
    const dividend = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/dividend', s.csrfToken, {
      transaction_date: paidOn, cash_account_id: s.cashID, cash_commodity_id: s.currencyID,
      income_account_id: s.incomeID, amount_value: '5000', amount_scale: 2, memo: `ACME ${s.suffix}`
    });

    await page.goto(`/app/ledger/transactions?transaction_id=${dividend.id}`);
    await page.getByRole('button', { name: 'Correct dividend…' }).click();
    const form = page.getByRole('dialog', { name: 'Correct this dividend' });
    const exDateField = form.getByLabel('Ex-dividend date');
    await expect(exDateField).toHaveValue('');
    await form.getByLabel('Reason for correction').fill('statement names the ex date');
    await exDateField.fill(exDate);
    await form.getByRole('button', { name: 'Review and replace' }).click();
    await expect(form).toBeHidden();

    const replacement = await chainEffective(page, dividend.id);
    expect(replacement).not.toBe(dividend.id);
    const chain = await apiJSON<{ effective_dividend: { ex_date?: string } }>(page, 'GET',
      `/api/v1/investments/transactions/${replacement}/correction-chain`);
    expect(chain.effective_dividend.ex_date).toBe(exDate);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  });

  // #395: the date, cash account and (with the account) the currency of a
  // cash dividend are correctable; the original is cancelled where it was.
  test('a correction moves a cash dividend to another date and cash account currency', async ({ page }) => {
    const s = await setup(page, 'MDV');
    const sek = await ensureCurrency(page, s.csrfToken, 'SEK', 'Swedish krona');
    const openedOn = daysFromTodayISO(-60);
    const broker = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', s.csrfToken, {
      name: `Broker SEK ${s.suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
      default_commodity_id: sek, allows_postings: true, opened_on: openedOn, effective_from: openedOn
    });
    const income = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', s.csrfToken, {
      name: `Dividend income any ${s.suffix}`, account_class: 'income', account_kind: 'income',
      allows_postings: true, opened_on: openedOn, effective_from: openedOn
    });
    const dividend = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/dividend', s.csrfToken, {
      transaction_date: daysFromTodayISO(-10), cash_account_id: s.cashID, cash_commodity_id: s.currencyID,
      income_account_id: income.id, amount_value: '5000', amount_scale: 2, memo: `ACME ${s.suffix}`
    });

    await page.goto(`/app/ledger/transactions?transaction_id=${dividend.id}`);
    await page.getByRole('button', { name: 'Correct dividend…' }).click();
    const form = page.getByRole('dialog', { name: 'Correct this dividend' });
    await expect(form.getByLabel('Payment date', { exact: true })).toHaveValue(daysFromTodayISO(-10));
    await form.getByLabel('Reason for correction').fill('paid into the SEK account two days earlier');
    await form.getByLabel('Payment date', { exact: true }).fill(daysFromTodayISO(-12));
    await form.getByLabel(/^Cash account/).selectOption(String(broker.id));
    await expect(form.getByText('(SEK)').first()).toBeVisible();
    await form.getByLabel(/^Dividend amount/).fill('48.00');
    await form.getByRole('button', { name: 'Review and replace' }).click();
    await expect(form).toBeHidden();

    const replacement = await chainEffective(page, dividend.id);
    expect(replacement).not.toBe(dividend.id);
    const chain = await apiJSON<{ effective_dividend: { event_date: string; cash_account_id: number; cash_commodity_id: number;
      amount_value: string; amount_scale: number; source_differences: string[] } }>(page, 'GET',
      `/api/v1/investments/transactions/${replacement}/correction-chain`);
    expect(chain.effective_dividend).toMatchObject({ event_date: daysFromTodayISO(-12), cash_account_id: broker.id,
      cash_commodity_id: sek, amount_value: '4800', amount_scale: 2, source_differences: [] });
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  });

  // The recorded cash account stays selected after it is archived, rather
  // than the picker silently falling back to another account, and a liability
  // the backend settlement role accepts (a margin line) is offered.
  test('a correction keeps an archived cash account and offers a margin line', async ({ page }) => {
    const s = await setup(page, 'ARC');
    const openedOn = daysFromTodayISO(-60);
    const margin = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', s.csrfToken, {
      name: `Margin line ${s.suffix}`, account_class: 'liability', account_kind: 'line_of_credit',
      default_commodity_id: s.currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
    });
    const dividend = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/dividend', s.csrfToken, {
      transaction_date: daysFromTodayISO(-10), cash_account_id: s.cashID, cash_commodity_id: s.currencyID,
      income_account_id: s.incomeID, amount_value: '5000', amount_scale: 2, memo: `ACME ${s.suffix}`
    });
    await apiJSON(page, 'POST', `/api/v1/accounts/${s.cashID}/close`, s.csrfToken, { closed_on: todayISO() });
    await apiJSON(page, 'POST', `/api/v1/accounts/${s.cashID}/archive`, s.csrfToken, { effective_from: todayISO() });

    await page.goto(`/app/ledger/transactions?transaction_id=${dividend.id}`);
    await page.getByRole('button', { name: 'Correct dividend…' }).click();
    const form = page.getByRole('dialog', { name: 'Correct this dividend' });
    const cashAccount = form.getByLabel(/^Cash account/);
    await expect(cashAccount).toHaveValue(String(s.cashID));
    await expect(cashAccount.locator(`option[value="${margin.id}"]`)).toHaveCount(1);
    await form.getByLabel('Reason for correction').fill('paid against the margin line');
    await cashAccount.selectOption(String(margin.id));
    await form.getByRole('button', { name: 'Review and replace' }).click();
    await expect(form).toBeHidden();

    const replacement = await chainEffective(page, dividend.id);
    expect(replacement).not.toBe(dividend.id);
    const chain = await apiJSON<{ effective_dividend: { cash_account_id: number } }>(page, 'GET',
      `/api/v1/investments/transactions/${replacement}/correction-chain`);
    expect(chain.effective_dividend.cash_account_id).toBe(margin.id);
  });

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
    // Scoped to realized gains: an unrealized row also shows its price date (#230).
    const saleRow = page
      .getByRole('region', { name: 'Realized gains', exact: true })
      .locator('tbody tr', { hasText: name })
      .filter({ hasText: daysFromTodayISO(-10) });
    await page.goto('/app/investments');
    await page.getByRole('link', { name: 'Gains', exact: true }).click();
    await expect(saleRow).toContainText('140.00');
    await clientNavigate(page, `/app/ledger/transactions?transaction_id=${reinvestment.transaction.id}`);
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
    await page.getByRole('link', { name: 'Gains', exact: true }).click();
    await expect(saleRow).toContainText('90.00');
    await expect(saleRow).not.toContainText('140.00');
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  });

  // #395: a reinvestment moves to another holding and a later date; the gain
  // review names the earlier sale it restates before anything is written.
  test('correcting a reinvested dividend date and holding restates the later sale', async ({ page }) => {
    const s = await setup(page, 'RMV');
    const openedOn = daysFromTodayISO(-60);
    const name = `Moved ${s.suffix}`;
    const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', s.csrfToken, {
      commodity_code: s.suffix.toUpperCase(), instrument_type: 'stock', display_name: name, symbol: s.suffix.toUpperCase(),
      quote_commodity_id: s.currencyID, trading_commodity_id: s.currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
    });
    const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', s.csrfToken, {
      instrument_id: instrument.id, name: `Moved holding ${s.suffix}`, opened_on: openedOn, effective_from: openedOn
    });
    const otherHolding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', s.csrfToken, {
      instrument_id: instrument.id, name: `Other broker ${s.suffix}`, opened_on: openedOn, effective_from: openedOn
    });
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

    await page.goto(`/app/ledger/transactions?transaction_id=${reinvestment.transaction.id}`);
    await page.getByRole('button', { name: 'Correct reinvested dividend…' }).click();
    const form = page.getByRole('dialog', { name: 'Correct this reinvested dividend' });
    await expect(form.getByLabel('Date', { exact: true })).toHaveValue(daysFromTodayISO(-30));
    await form.getByLabel('Reason for correction').fill('reinvested later, at the other broker');
    await form.getByLabel('Date', { exact: true }).fill(daysFromTodayISO(-15));
    await form.getByLabel('Holding account').selectOption(String(otherHolding.id));
    await form.getByRole('button', { name: 'Review and replace' }).click();

    // The sale now takes the bought shares: basis 10.00 → 100.00.
    const dialog = page.getByRole('alertdialog');
    await expect(dialog).toContainText(`Sale on ${daysFromTodayISO(-10)}: basis 10.00 → 100.00, gain 140.00 → 50.00`);
    await dialog.getByRole('button', { name: 'Accept changed gains' }).click();
    await expect(dialog).toBeHidden();
    await expect(form).toBeHidden();

    const replacement = await chainEffective(page, reinvestment.transaction.id);
    expect(replacement).not.toBe(reinvestment.transaction.id);
    const chain = await apiJSON<{ effective_reinvestment: { event_date: string; holding_account_id: number; commodity_id: number } }>(
      page, 'GET', `/api/v1/investments/transactions/${replacement}/correction-chain`);
    expect(chain.effective_reinvestment).toMatchObject({ event_date: daysFromTodayISO(-15), holding_account_id: otherHolding.id,
      commodity_id: instrument.commodity_id });
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  });
});
