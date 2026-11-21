import { test, expect } from './support/owned-book';
import AxeBuilder from '@axe-core/playwright';
import { apiJSON } from './support/api';
import { ensureCurrency, readyForLedger } from './support/ledger';
import { daysFromTodayISO } from './support/dates';

test.use({ viewport: { width: 390, height: 844 } });

// #231 (R18 slice 2): the period statement reports opening and closing
// unrealized, realized and income per currency, and shows an absent figure
// with its reason instead of zero. The statement is a book-wide read, so each
// case runs in a book of its own (support/owned-book.ts, #398): no other spec's
// income or trades can change the totals it asserts.
test.describe('period statement', () => {
  test('the period statement splits a result per currency and never shows an absent figure as zero at 390px', async ({ page }) => {
    const { csrfToken } = await readyForLedger(page);
    const chf = await ensureCurrency(page, csrfToken, 'CHF', 'Swiss Franc');
    const suffix = Date.now();
    const openedOn = daysFromTodayISO(-60);
    const account = (name: string, accountClass: string, kind: string) =>
      apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
        name: `${name} ${suffix}`, account_class: accountClass, account_kind: kind,
        default_commodity_id: chf, allows_postings: true, opened_on: openedOn, effective_from: openedOn
      });
    const cash = await account('Statement cash', 'asset', 'brokerage_cash');
    const income = await account('Statement income', 'income', 'income');
    const instrument = async (code: string) => {
      const created = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
        commodity_code: `${code}${suffix}`, instrument_type: 'stock', display_name: `Statement ${code} ${suffix}`,
        symbol: `${code}${suffix}`, quote_commodity_id: chf, trading_commodity_id: chf, quantity_scale: 0, price_scale: 2,
        effective_from: openedOn
      });
      const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
        instrument_id: created.id, name: `Statement ${code} holding ${suffix}`, opened_on: openedOn, effective_from: openedOn
      });
      return { commodityID: created.commodity_id, holdingID: holding.id };
    };
    const transferIn = (target: { commodityID: number; holdingID: number }, quantity: string, basis: string) =>
      apiJSON(page, 'POST', '/api/v1/investments/transfers/external/in', csrfToken, {
        effective_on: daysFromTodayISO(-40), holding_account_id: target.holdingID, commodity_id: target.commodityID,
        quantity_value: quantity, quantity_scale: 0, carried_basis_value: basis, carried_basis_scale: 2,
        cost_commodity_id: chf, original_acquired_on: daysFromTodayISO(-40), source_evidence: { statement: 'Period statement test' }
      });
    const price = (target: { commodityID: number }, daysAgo: number, value: number) =>
      apiJSON(page, 'POST', '/api/v1/pricing/prices', csrfToken, {
        base_commodity_id: target.commodityID, quote_commodity_id: chf, quote_type: 'close',
        adjustment_basis: 'raw', price_value: value, price_scale: 2, valuation_date: daysFromTodayISO(-daysAgo), is_manual: false
      });

    // Priced throughout: 2 units for 100.00, priced 30.00 before the period;
    // 1 sold for 80.00 inside it; the other priced 70.00 near its end.
    const priced = await instrument('PST');
    await transferIn(priced, '2', '10000');
    await price(priced, 35, 3000);
    await apiJSON(page, 'POST', '/api/v1/investments/sell', csrfToken, {
      transaction_date: daysFromTodayISO(-20), commodity_id: priced.commodityID, holding_account_id: priced.holdingID,
      cash_account_id: cash.id, quantity_value: '1', quantity_scale: 0, cash_amount_value: '8000', cash_amount_scale: 2,
      cash_commodity_id: chf, cost_basis_method: 'fifo'
    });
    await price(priced, 2, 7000);
    // First priced 12.00 fifteen days ago: unpriced before that.
    const late = await instrument('LST');
    await transferIn(late, '1', '1000');
    await price(late, 15, 1200);
    await apiJSON(page, 'POST', '/api/v1/investments/dividend', csrfToken, {
      transaction_date: daysFromTodayISO(-10), cash_account_id: cash.id, cash_commodity_id: chf,
      income_account_id: income.id, amount_value: '500', amount_scale: 2, memo: `Statement dividend ${suffix}`
    });

    await page.goto('/app/investments');
    await page.getByRole('link', { name: 'Gains', exact: true }).click();
    const statement = page.getByTestId('gains-statement');
    const showPeriod = async (fromDaysAgo: number) => {
      await statement.getByLabel('Period start').fill(daysFromTodayISO(-fromDaysAgo));
      await statement.getByRole('button', { name: 'Show statement' }).click();
    };
    const card = statement.getByRole('article', { name: 'Statement in CHF' });
    const line = (key: string) => card.locator(`[data-line="${key}"]`);

    // Opening the day before 30 days ago: the late instrument has no price yet.
    await showPeriod(30);
    await expect(line('opening')).toContainText('Not available');
    await expect(line('opening')).toContainText('No price: 1');
    await expect(line('opening')).toContainText('Positions: 2');
    await expect(line('realized')).toContainText('30.00'); // 80.00 − 50.00
    await expect(line('realized')).toContainText('Disposals: 1');
    await expect(line('income')).toContainText('5.00');
    await expect(line('closing')).toContainText('22.00'); // (70.00 − 50.00) + (12.00 − 10.00)
    await expect(line('closing')).toContainText('Stale prices: 1');
    await expect(line('change')).toContainText('Not available');
    await expect(line('total')).toContainText('Needs every figure above');
    await expect(line('total')).not.toContainText('0.00');

    // A period opening after the late instrument's first price is complete.
    await showPeriod(12);
    await expect(line('opening')).toContainText('32.00'); // (80.00 − 50.00) + (12.00 − 10.00)
    await expect(line('realized')).toContainText('Disposals: 0');
    await expect(line('change')).toContainText(/[−-]10\.00/);
    await expect(line('total')).toContainText(/[−-]5\.00/); // 0 + 5.00 − 10.00

    await statement.getByLabel('Period end').fill(daysFromTodayISO(1));
    await expect(statement.getByText('the end date cannot be after today')).toBeVisible();
    await expect(statement.getByRole('button', { name: 'Show statement' })).toBeDisabled();

    expect((await new AxeBuilder({ page }).include('main').analyze()).violations).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  });

  // One test per locale (#251), each in its own book with one dividend so the
  // statement has a row to translate.
  const locales = [
    ['nl', 'Winsten', 'Periodeoverzicht'],
    ['ru', 'Прибыль', 'Отчёт за период']
  ] as const;
  for (const [locale, tab, title] of locales) {
    test(`the period statement is translated in ${locale}`, async ({ page }) => {
      const { csrfToken, currencyID } = await readyForLedger(page);
      const openedOn = daysFromTodayISO(-60);
      const account = (name: string, accountClass: string, kind: string) =>
        apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
          name, account_class: accountClass, account_kind: kind, default_commodity_id: currencyID,
          allows_postings: true, opened_on: openedOn, effective_from: openedOn
        });
      const cash = await account('Statement cash', 'asset', 'brokerage_cash');
      const income = await account('Statement income', 'income', 'income');
      await apiJSON(page, 'POST', '/api/v1/investments/dividend', csrfToken, {
        transaction_date: daysFromTodayISO(-10), cash_account_id: cash.id, cash_commodity_id: currencyID,
        income_account_id: income.id, amount_value: '500', amount_scale: 2
      });
      await page.evaluate((value) => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
      await page.goto('/app/investments');
      await page.getByRole('link', { name: tab, exact: true }).click();
      await expect(page.getByRole('heading', { name: title })).toBeVisible();
      await expect(page.getByTestId('gains-statement-row').first()).toBeVisible();
    });
  }
});
