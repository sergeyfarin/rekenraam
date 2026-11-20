import { test, expect, type Page } from './support/fixtures';
import AxeBuilder from '@axe-core/playwright';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO, daysFromTodayISO } from './support/dates';

test.use({ viewport: { width: 390, height: 844 } });

// Two holding accounts of one instrument: an internal transfer moves half of
// account 0's lot into account 1, then a write-off disposes of one unit.
async function setup(page: Page, locale: string) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = `${locale.toUpperCase()}${Date.now()}`;
  const name = `Person pool ${suffix}`;
  const opened = daysFromTodayISO(-60);
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: `PP${suffix}`, instrument_type: 'stock', display_name: name, symbol: `PP${suffix}`,
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 0, price_scale: 2, effective_from: opened
  });
  const accounts: number[] = [];
  const lots: number[] = [];
  for (const [index, basis] of ['4000', '12000'].entries()) {
    const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
      instrument_id: instrument.id, name: `${name} ${index}`, opened_on: opened, effective_from: opened
    });
    accounts.push(holding.id);
    const inbound = await apiJSON<{ lot_id: number }>(page, 'POST', '/api/v1/investments/transfers/external/in', csrfToken, {
      effective_on: daysFromTodayISO(-40 + index), holding_account_id: holding.id, commodity_id: instrument.commodity_id,
      quantity_value: '4', quantity_scale: 0, carried_basis_value: basis, carried_basis_scale: 2,
      cost_commodity_id: currencyID, original_acquired_on: daysFromTodayISO(-40 + index), source_evidence: { statement: 'Pooled method test' }
    });
    lots.push(inbound.lot_id);
  }
  await apiJSON(page, 'POST', '/api/v1/investments/transfers/internal', csrfToken, {
    effective_on: daysFromTodayISO(-30), source_account_id: accounts[0], destination_account_id: accounts[1],
    commodity_id: instrument.commodity_id, cost_commodity_id: currencyID,
    lot_allocations: [{ lot_id: lots[0], quantity_value: '2', quantity_scale: 0 }]
  });
  await apiJSON(page, 'POST', '/api/v1/investments/write-off', csrfToken, {
    transaction_date: daysFromTodayISO(-10), holding_account_id: accounts[0], commodity_id: instrument.commodity_id,
    cost_commodity_id: currencyID, quantity_value: '1', quantity_scale: 0, cost_basis_method: 'fifo', reason: 'Pool test'
  });
  await apiJSON(page, 'POST', '/api/v1/pricing/prices', csrfToken, {
    base_commodity_id: instrument.commodity_id, quote_commodity_id: currencyID, quote_type: 'manual', adjustment_basis: 'raw',
    price_value: 3000, price_scale: 2, valuation_date: todayISO(), is_manual: true
  });
  return name;
}

const note = 'Owner context <s104 / ACB / PMP>'; // plain text, never a rule selector or markup
const locales = [
  ['en', 'Gains', 'Person-pooled average cost', 'Jurisdiction note (optional)'],
  ['nl', 'Winsten', 'Gemiddelde kostprijs per persoon', 'Jurisdictienotitie (optioneel)'],
  ['ru', 'Прибыль', 'Средняя стоимость по всем счетам владельца', 'Примечание о юрисдикции (необязательно)']
] as const;

// One test per locale (#250): six passes in one body outgrew the default
// budget under full-suite load. Each test runs in a fresh browser context, so
// the locale it stores never leaks into the next spec.
for (const [locale, tab, method, noteLabel] of locales) {
  test(`person-pooled gains preserve internal transfers and operational basis at 390px in ${locale}`, async ({ page }) => {
    const name = await setup(page, locale);
    await page.evaluate((value) => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    await page.goto('/app/investments');
    await page.getByRole('link', { name: tab, exact: true }).click();
    const comparison = page.getByTestId('alternative-gains');
    await comparison.getByRole('combobox').selectOption('person_pooled_average');
    await expect(comparison.getByRole('heading')).toContainText(method);
    await comparison.getByLabel(noteLabel, { exact: true }).fill(note);
    const request = page.waitForResponse((res) => new URL(res.url()).searchParams.get('jurisdiction_note') === note);
    await comparison.locator('form button').click();
    const result = await (await request).json();
    expect(result.alternative.profile.jurisdiction_note).toBe(note);
    await expect(comparison.locator('p').filter({ hasText: note })).toBeVisible();
    const positions = comparison.getByRole('row').filter({ hasText: name });
    await expect(positions).toHaveCount(2);
    // Account 1: operational basis 10, pool basis 20, market 30.
    await expect(positions.first()).toContainText(/20[,.]00/);
    await expect(positions.first()).toContainText(/10[,.]00/);
    // Account 2: operational basis 140, pooled basis 120, market 180.
    await expect(positions.last()).toContainText(/40[,.]00/);
    await expect(positions.last()).toContainText(/60[,.]00/);
    expect((await new AxeBuilder({ page }).include('main').analyze()).violations).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  });
}
