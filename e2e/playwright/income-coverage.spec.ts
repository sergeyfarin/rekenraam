import { expect, test, type Page } from './support/fixtures';
import process from 'node:process';
import { apiJSON } from './support/api';
import { readyForLedger, createCashAccount } from './support/ledger';
import { todayISO } from './support/dates';
import { expectNoAccessibilityViolations } from './support/a11y';
import { requireTrading212Stub, setTrading212Dividends } from './support/trading212';
import { linkLegacyCashDividend } from './support/legacy-dividend';

// #244: income attribution coverage keeps cash-only and payer-less payments
// visible beside known figures, and routes each to its optional repair.
test.use({ viewport: { width: 390, height: 844 } });

async function ledger(page: Page, label: string) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = `${label}${Date.now()}`;
  const cash = await createCashAccount(page, csrfToken, `Coverage cash ${suffix}`, currencyID);
  const income = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Coverage income ${suffix}`, account_class: 'income', account_kind: 'income',
    default_commodity_id: currencyID, allows_postings: true, opened_on: todayISO(), effective_from: todayISO()
  });
  const ticker = `COV${suffix}`;
  const instrument = await apiJSON<{ commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: ticker, instrument_type: 'stock', display_name: `Coverage payer ${suffix}`, symbol: ticker,
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 0, price_scale: 2, effective_from: todayISO()
  });
  return { csrfToken, currencyID, cash, income, instrument, ticker, suffix };
}

function noHorizontalScroll(page: Page) {
  return page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
}

test('[acceptance] a dividend without payer stays usable and gets its payer through native correction on a phone', async ({ page }) => {
  const f = await ledger(page, 'NAMED');
  const dividend = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/dividend', f.csrfToken, {
    transaction_date: todayISO(), cash_account_id: f.cash.id, cash_commodity_id: f.currencyID,
    income_account_id: f.income.id, amount_value: '4321', amount_scale: 2, memo: `Unassigned ${f.suffix}`
  });

  await page.goto(`/app/investments/instruments/${f.instrument.commodity_id}`);
  await expect(page.getByText('No recorded dividend income for this instrument.', { exact: true })).toBeVisible();
  await expect(page.getByText(/^Not included: \d+ unassigned payments in the book/)).toBeVisible();

  await page.goto('/app/investments/income');
  const card = page.getByRole('listitem').filter({ hasText: '43.21' }).filter({ hasText: 'Dividend without payer' });
  await expect(card).toBeVisible();
  await expect(card.getByText('Unassigned', { exact: true })).toBeVisible();
  await expect(card.getByText('None withheld (recorded)', { exact: true })).toBeVisible();
  expect(await noHorizontalScroll(page)).toBe(true);
  await expectNoAccessibilityViolations(page, 'Income coverage on a phone');

  await card.getByRole('link', { name: 'Open payment to correct its payer', exact: true }).click();
  await page.waitForURL(`**/app/ledger/transactions?transaction_id=${dividend.id}`);
  await page.getByRole('button', { name: 'Correct dividend…' }).click();
  const form = page.getByRole('dialog', { name: 'Correct this dividend' });
  await expect(form.getByRole('combobox', { name: 'Paying instrument', exact: true })).toHaveValue('');
  await form.getByRole('combobox', { name: 'Paying instrument', exact: true }).selectOption(String(f.instrument.commodity_id));
  await form.getByLabel('Reason for correction').fill('statement names the payer');
  await form.getByRole('button', { name: 'Review and replace' }).click();
  await expect(form).toBeHidden();

  await page.goto(`/app/investments/instruments/${f.instrument.commodity_id}`);
  await expect(page.getByText(/^Gross income: .*43\.21/)).toBeVisible();
  await expect(page.getByText('· None withheld (recorded)')).toBeVisible();
});

// The locale passes reuse the journey's imported cash, so they run serially
// after it, one test per locale (#251).
test.describe.serial('imported cash payments', () => {
  requireTrading212Stub();
  test.skip(Boolean(process.env.E2E_BASE_URL), 'historical source-link fixture needs the disposable local e2e database');

  test('[acceptance] imported cash with unresolved gross and tax is disclosed beside its instrument, never as income', async ({ page }) => {
    const f = await ledger(page, 'CASH');
    const apiKey = `coverage-${f.suffix}`;
    await setTrading212Dividends(apiKey, { ticker: f.ticker, isin: '', currency: 'USD' }, [{ reference: `coverage-${f.suffix}`, amount: 12.34, date: todayISO() }]);
    const connectionName = `Coverage broker ${f.suffix}`;
    await apiJSON(page, 'POST', '/api/v1/import-connections', f.csrfToken, {
      source: 'trading212', display_name: connectionName, api_key: apiKey, cash_account_id: f.cash.id
    });
    await page.goto('/app/ledger/import');
    await page.getByRole('row').filter({ hasText: connectionName }).getByRole('button', { name: 'Import', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Commit to ledger' })).toBeVisible({ timeout: 15_000 });
    const reviews = await apiJSON<{ rows: Array<{ batch_id: number; row_id: number; state: string; resolution: { account_id?: number } }> }>(page, 'GET', '/api/v1/import-dividends');
    const row = reviews.rows.find(r => r.state === 'pending' && r.resolution.account_id === f.cash.id)!;
    expect(row).toBeTruthy();
    const legacy = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/transactions', f.csrfToken, {
      transaction_date: todayISO(), description: 'Historical cash dividend',
      journal_entries: [{ entry_date: todayISO(), entry_kind: 'ordinary', postings: [
        { account_id: f.cash.id, commodity_id: f.currencyID, quantity_value: '1234', quantity_scale: 2 },
        { account_id: f.income.id, commodity_id: f.currencyID, quantity_value: '-1234', quantity_scale: 2 }
      ] }]
    });
    linkLegacyCashDividend(row.row_id, legacy.id, f.cash.id);

    await page.goto(`/app/investments/instruments/${f.instrument.commodity_id}`);
    await expect(page.getByText(/^No recorded gross income/)).toBeVisible();
    await expect(page.getByText(/^1 imported payments naming this instrument \(.*12\.34 cash\) are not classified/)).toBeVisible();
    expect(await noHorizontalScroll(page)).toBe(true);
    await expectNoAccessibilityViolations(page, 'Instrument income summary on a phone');

    await page.goto('/app/investments/income');
    const card = page.getByRole('listitem').filter({ hasText: '12.34' }).filter({ hasText: 'Imported cash payment' })
      .filter({ hasText: f.ticker });
    await expect(card.getByText('Unknown — only the cash received is known', { exact: true })).toBeVisible();
    await expect(card.getByText('Unknown — not assumed to be zero', { exact: true })).toBeVisible();
    await expect(card.getByText(new RegExp(`Source names Coverage payer ${f.suffix}`))).toBeVisible();
    await expect(card.getByText(new RegExp(`Source evidence: ${f.ticker}`))).toBeVisible();
    await card.getByRole('link', { name: 'Review gross, tax and payer', exact: true }).click();
    await page.waitForURL(`**/app/ledger/import/dividends?batch_id=${row.batch_id}`);
    await expect(page.getByRole('heading', { name: 'Repair previously imported cash payment', exact: true })).toBeVisible();
  });

  // #246: the period statement discloses the same cash beside income, never in it.
  const locales = [
    ['en', 'Gains', 'Unclassified imported dividend cash'],
    ['nl', 'Winsten', 'Niet-geclassificeerde geïmporteerde dividendontvangsten'],
    ['de', 'Gewinne', 'Nicht klassifizierte importierte Dividendenzahlungen'],
    ['fr', 'Plus-values', 'Encaissements de dividendes importés non classés'],
    ['es', 'Ganancias', 'Efectivo de dividendos importados sin clasificar'],
    ['ru', 'Прибыль', 'Неклассифицированные импортированные дивидендные поступления']
  ] as const;
  for (const [locale, tab, label] of locales) {
    test(`[acceptance] the period statement discloses unclassified imported cash beside income in ${locale}`, async ({ page }) => {
      await readyForLedger(page);
      await page.evaluate((value) => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
      await page.goto('/app/investments');
      await page.getByRole('link', { name: tab, exact: true }).click();
      const statement = page.getByTestId('gains-statement').first();
      const disclosure = statement.locator('[data-line="unclassified-dividends"]');
      await expect(disclosure).toContainText(label);
      await expect(disclosure.getByRole('link')).toHaveAttribute('href', '/app/investments/income');
      expect(await noHorizontalScroll(page)).toBe(true);
      await expectNoAccessibilityViolations(page, `Statement dividend disclosure (${locale})`);
    });
  }
});
