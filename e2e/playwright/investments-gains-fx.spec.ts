import { test, expect } from './support/fixtures';
import AxeBuilder from '@axe-core/playwright';
import { apiJSON } from './support/api';
import { ensureCurrency, readyForLedger } from './support/ledger';
import { daysFromTodayISO } from './support/dates';

test.use({ viewport: { width: 390, height: 844 } });

test.describe.serial('gains FX conversion', () => {
  test('gains convert with named dated FX methods and disclose incomplete coverage at 390px', async ({ page }) => {
    const { csrfToken } = await readyForLedger(page);
    const cad = await ensureCurrency(page, csrfToken, 'CAD', 'Canadian Dollar');
    const nzd = await ensureCurrency(page, csrfToken, 'NZD', 'New Zealand Dollar');
    const suffix = Date.now();
    const name = `FX investment ${suffix}`;
    const openedOn = daysFromTodayISO(-70);
    const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
      name: `FX cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
      default_commodity_id: cad, allows_postings: true, opened_on: openedOn, effective_from: openedOn
    });
    const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
      commodity_code: `FX${suffix}`, instrument_type: 'stock', display_name: name,
      symbol: `FX${suffix}`, quote_commodity_id: cad, trading_commodity_id: cad, quantity_scale: 0, price_scale: 2,
      effective_from: openedOn
    });
    const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
      instrument_id: instrument.id, name: `FX holding ${suffix}`, opened_on: openedOn, effective_from: openedOn
    });
    await apiJSON(page, 'POST', '/api/v1/investments/transfers/external/in', csrfToken, {
      effective_on: daysFromTodayISO(-40), holding_account_id: holding.id, commodity_id: instrument.commodity_id,
      quantity_value: '2', quantity_scale: 0, carried_basis_value: '4000', carried_basis_scale: 2,
      cost_commodity_id: cad, original_acquired_on: daysFromTodayISO(-60), source_evidence: { statement: 'FX test' }
    });
    await apiJSON(page, 'POST', '/api/v1/investments/sell', csrfToken, {
      transaction_date: daysFromTodayISO(-20), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
      cash_account_id: cash.id, quantity_value: '1', quantity_scale: 0, cash_amount_value: '5000', cash_amount_scale: 2,
      cash_commodity_id: cad, cost_basis_method: 'fifo'
    });
    await apiJSON(page, 'POST', '/api/v1/pricing/prices', csrfToken, {
      base_commodity_id: instrument.commodity_id, quote_commodity_id: cad, quote_type: 'close',
      adjustment_basis: 'raw', price_value: 6000, price_scale: 2, valuation_date: daysFromTodayISO(0)
    });
    const rate = (days: number, value: number) => apiJSON(page, 'POST', '/api/v1/pricing/prices', csrfToken, {
      base_commodity_id: cad, quote_commodity_id: nzd, quote_type: 'official_fixing', adjustment_basis: 'raw',
      price_value: value, price_scale: 2, valuation_date: daysFromTodayISO(-days)
    });
    await rate(60, 50);
    await rate(20, 80);
    await rate(8, 120); // beyond the default inclusive seven-day window

    const open = async () => {
      await page.goto('/app/investments');
      await page.getByRole('link', { name: 'Gains', exact: true }).click();
      await page.getByRole('combobox', { name: 'Convert gains to', exact: true }).selectOption(String(nzd));
    };
    await open();
    const controls = page.getByTestId('gains-fx-controls');
    const position = controls.locator(`[data-fx-label="${name}"]`);
    await expect(position).toContainText('Not available — incomplete coverage');
    await position.getByText('Rates and source amounts', { exact: true }).click();
    await expect(position).toContainText(`Nearest earlier observation: ${daysFromTodayISO(-8)}`);
    await expect(controls.getByTestId('gains-fx-results')).toContainText('live_raw_v1');

    await controls.getByLabel('Maximum FX age (days)').fill('8');
    await expect(position).toContainText('48.00'); // (60 − 20) × 1.2
    await controls.getByRole('combobox', { name: 'FX method', exact: true }).selectOption('historic_basis_valuation_close');
    await expect(position).toContainText('62.00'); // 60 × 1.2 − 20 × .5
    await position.getByText('Rates and source amounts', { exact: true }).click();
    await expect(position).toContainText(`requested ${daysFromTodayISO(-60)}`);
    await expect(position).toContainText('observation #');

    // The same selector configures the period statement without extra per-row requests.
    const statement = page.getByTestId('gains-statement');
    await statement.getByLabel('Period start', { exact: true }).fill(daysFromTodayISO(-30));
    await statement.getByRole('button', { name: 'Show statement' }).click();
    await expect(statement.getByTestId('gains-fx-results')).toContainText('historic_basis_valuation_close');
    // Original cost-currency figures are retained alongside the converted result.
    await expect(page.getByRole('region', { name: 'Unrealized gains', exact: true })).toContainText('CAD');
    expect((await new AxeBuilder({ page }).include('main').analyze()).violations).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);

    // Same-currency conversion works without any FX observation.
    await controls.getByRole('combobox', { name: 'Convert gains to', exact: true }).selectOption(String(cad));
    await expect(position).toContainText('40.00');
    await position.getByText('Rates and source amounts', { exact: true }).click();
    await expect(position).toContainText('Same currency: exact value');
  });

  // One test per locale (#251), over the CAD gains the journey booked.
  const locales = [
    ['nl', 'Winsten', 'Rapportagevaluta'],
    ['ru', 'Прибыль', 'Валюта отчёта']
  ] as const;
  for (const [locale, tab, title] of locales) {
    test(`gains FX controls fit 390px in ${locale}`, async ({ page }) => {
      const { csrfToken } = await readyForLedger(page);
      const cad = await ensureCurrency(page, csrfToken, 'CAD', 'Canadian Dollar');
      await page.evaluate((value) => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
      await page.goto('/app/investments');
      await page.getByRole('link', { name: tab, exact: true }).click();
      const translated = page.getByTestId('gains-fx-controls');
      await expect(translated.getByRole('heading', { name: title, exact: true })).toBeVisible();
      await translated.getByRole('combobox').first().selectOption(String(cad));
      await expect(translated.getByTestId('gains-fx-results')).toBeVisible();
      const layout = await page.evaluate(() => ({
        width: document.documentElement.scrollWidth,
        offenders: [...document.querySelectorAll('main *')].filter((node) => node.getBoundingClientRect().right > innerWidth)
          .slice(0, 6).map((node) => ({ tag: node.tagName, classes: node.className, right: node.getBoundingClientRect().right }))
      }));
      expect(layout.width, JSON.stringify({ locale, offenders: layout.offenders })).toBeLessThanOrEqual(390);
    });
  }
});
