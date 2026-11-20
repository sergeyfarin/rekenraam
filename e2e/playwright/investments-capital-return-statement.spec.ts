import { test, expect, type Page } from './support/fixtures';
import AxeBuilder from '@axe-core/playwright';
import { apiJSON } from './support/api';
import { ensureCurrency, readyForLedger } from './support/ledger';
import { daysFromTodayISO } from './support/dates';

test.use({ viewport: { width: 390, height: 844 } });

type Fixture = { currency: number; cashID: number; suffix: number };

// One holding of 1 unit, basis 7.00 (or unknown), receiving a 10.00 return of capital.
async function position(page: Page, csrfToken: string, f: Fixture, code: string, unknown: boolean) {
  const openedOn = daysFromTodayISO(-60);
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: `${code}${f.suffix}`, instrument_type: 'stock', display_name: `ROC statement ${code} ${f.suffix}`, symbol: `${code}${f.suffix}`,
    quote_commodity_id: f.currency, trading_commodity_id: f.currency, quantity_scale: 0, price_scale: 2, effective_from: openedOn
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `ROC statement holding ${code} ${f.suffix}`, opened_on: openedOn, effective_from: openedOn
  });
  await apiJSON(page, 'POST', '/api/v1/investments/transfers/external/in', csrfToken, {
    effective_on: daysFromTodayISO(-40), holding_account_id: holding.id, commodity_id: instrument.commodity_id,
    quantity_value: '1', quantity_scale: 0, cost_commodity_id: f.currency, original_acquired_on: daysFromTodayISO(-40),
    ...(unknown ? { basis_knowledge: 'unknown' } : { carried_basis_value: '700', carried_basis_scale: 2 }),
    source_evidence: { statement: 'Capital-return period fixture' }
  });
  for (const offset of [-35, -2]) {
    await apiJSON(page, 'POST', '/api/v1/pricing/prices', csrfToken, {
      base_commodity_id: instrument.commodity_id, quote_commodity_id: f.currency, quote_type: 'manual', adjustment_basis: 'raw',
      price_value: 700, price_scale: 2, valuation_date: daysFromTodayISO(offset), is_manual: true
    });
  }
  return apiJSON<{ transaction: { id: number } }>(page, 'POST', '/api/v1/investments/return-of-capital', csrfToken, {
    effective_on: daysFromTodayISO(-10), payment_on: daysFromTodayISO(-5), holding_account_id: holding.id,
    commodity_id: instrument.commodity_id, cash_account_id: f.cashID, currency_id: f.currency,
    amount_value: '1000', amount_scale: 2, source_evidence: { notice: `ROC-${code}-238` }
  });
}

function statement(page: Page) {
  const live = page.getByTestId('gains-statement').first();
  const row = live.getByTestId('gains-statement-row').filter({ has: page.getByRole('heading', { name: 'DKK', exact: true }) });
  return { live, row, line: (name: string) => row.locator(`[data-line="${name}"]`) };
}

async function visit(page: Page, locale: string, tab: string) {
  await page.evaluate((value) => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
  await page.goto('/app/investments');
  await page.getByRole('link', { name: tab, exact: true }).click();
  const { live, row } = statement(page);
  await live.locator('input[type="date"]').first().fill(daysFromTodayISO(-30));
  await live.locator('form').getByRole('button').click();
  await expect(row).toBeVisible();
}

async function expectAccessibleAt390(page: Page) {
  expect((await new AxeBuilder({ page }).include('[data-testid="gains-statement"]').analyze()).violations).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
}

const locales = [
  ['en', 'Gains', 'excludes return-of-capital excess'],
  ['nl', 'Winsten', 'sluit het overschot bij kapitaalteruggave uit'],
  ['ru', 'Прибыль', 'не включает превышение возврата капитала']
] as const;

// The DKK statement totals every DKK position in the shared book, so the
// fixture is booked once and each step builds on the last: one test per locale
// (#251), run serially. A separate currency keeps this statement independent
// of other journeys.
test.describe.serial('period statements disclose capital-return excess and unknown coverage on mobile', () => {
  let fixture: Fixture;
  let knownID: number;
  let saved: { id: number; schema_version: number };

  test.beforeAll(async ({ browser, backend }) => {
    const context = await browser.newContext({ baseURL: backend.url });
    const page = await context.newPage();
    try {
      const { csrfToken } = await readyForLedger(page);
      const currency = await ensureCurrency(page, csrfToken, 'DKK', 'Danish Krone');
      const suffix = Date.now();
      const openedOn = daysFromTodayISO(-60);
      const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
        name: `ROC statement cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
        default_commodity_id: currency, allows_postings: true, opened_on: openedOn, effective_from: openedOn
      });
      fixture = { currency, cashID: cash.id, suffix };
      knownID = (await position(page, csrfToken, fixture, 'RKNOWN', false)).transaction.id;
    } finally {
      await context.close();
    }
  });

  for (const [locale, tab, scope] of locales) {
    test(`a known-basis receipt shows its excluded excess in ${locale}`, async ({ page }) => {
      await readyForLedger(page);
      await visit(page, locale, tab);
      const { live, line } = statement(page);
      await expect(line('total')).toContainText(scope);
      const decimal = new Intl.NumberFormat(locale, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
      await expect(line('total')).toContainText(decimal.format(7));
      await expect(line('roc-receipt')).toContainText(decimal.format(10));
      await expect(line('roc-reduction')).toContainText(decimal.format(7));
      await expect(line('roc-excess')).toContainText(decimal.format(3));
      await live.getByTestId('statement-capital-return-evidence').locator('summary').first().click();
      await expect(live.getByTestId('statement-capital-return-evidence')).toContainText('ROC-RKNOWN-238');
      await expectAccessibleAt390(page);
    });
  }

  test('a saved statement keeps its excess when the receipt is reversed and an unknown receipt arrives', async ({ page }) => {
    const { csrfToken } = await readyForLedger(page);
    await visit(page, 'en', 'Gains');
    const savedResponse = page.waitForResponse((res) => res.url().endsWith('/api/v1/investments/gains/runs') && res.request().method() === 'POST');
    await statement(page).live.getByRole('button', { name: 'Save period statement', exact: true }).click();
    saved = await (await savedResponse).json();
    expect(saved.schema_version).toBe(3);
    const frozen = page.getByTestId('saved-gains-run');
    await expect(frozen.getByTestId('gains-statement-row').filter({ has: page.getByRole('heading', { name: 'DKK', exact: true }) }).locator('[data-line="roc-excess"]')).toContainText('3.00');
    await apiJSON(page, 'POST', `/api/v1/investments/transactions/${knownID}/reverse-return-of-capital`, csrfToken, { reason: 'Notice withdrawn after saving' });
    await position(page, csrfToken, fixture, 'RUNKNOWN', true);
  });

  for (const [locale, tab, scope] of locales) {
    test(`an unknown-basis receipt never shows its excess as zero in ${locale}`, async ({ page }) => {
      await readyForLedger(page);
      await visit(page, locale, tab);
      const { line } = statement(page);
      await expect(line('total')).toContainText(scope);
      await expect(line('roc-receipt')).toContainText(new Intl.NumberFormat(locale, { minimumFractionDigits: 2 }).format(10));
      await expect(line('roc-excess')).not.toContainText(/0[.,]00/);
      await expect(line('roc-reduction')).not.toContainText(/0[.,]00/);
      if (locale === 'en') {
        await expect(line('roc-receipt')).toContainText('Receipts: 1; unresolved receipts: 1; unknown lot effects: 1.');
      }
      await expectAccessibleAt390(page);
    });
  }

  test('the saved statement is unchanged by the later corrections', async ({ page }) => {
    await readyForLedger(page);
    expect(await apiJSON(page, 'GET', `/api/v1/investments/gains/runs/${saved.id}`)).toEqual(saved);
  });
});
