import { expect, test } from './support/owned-book';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { daysFromTodayISO, todayISO } from './support/dates';
import { expectNoAccessibilityViolations } from './support/a11y';
import type { AllocationResponse } from '../../frontend/src/lib/api/allocation';

test.use({ viewport: { width: 390, height: 844 } });

test('[acceptance] investment pool includes deposits, excludes college savings, and preserves targets and revision history on a phone', async ({ page }) => {
  test.slow(); // This journey covers entry, stale edits, two themes and locale re-entry.
  const { csrfToken, currencyID } = await readyForLedger(page);
  const opened = daysFromTodayISO(-30);
  const start = daysFromTodayISO(-10);
  const end = todayISO();
  const income = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name: 'Allocation funding', category_type: 'income' });
  async function account(name: string, kind: string, cents: string) {
    const created = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, { name, account_class: 'asset', account_kind: kind, default_commodity_id: currencyID, allows_postings: true, opened_on: opened, effective_from: opened });
    await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, { transaction_date: opened, journal_entries: [{ entry_date: opened, postings: [
      { account_id: created.id, commodity_id: currencyID, quantity_value: cents, quantity_scale: 2 },
      { account_id: income.id, commodity_id: currencyID, quantity_value: `-${cents}`, quantity_scale: 2 }
    ] }] });
    return created;
  }
  const checking = await account('Pool settlement', 'checking', '100000');
  const deposit = await account('Raisin fixed deposit', 'term_deposit', '40000');
  const college = await account('College savings outside pool', 'savings', '500000');
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, { commodity_code: 'POOL', instrument_type: 'etf', display_name: 'Pool equity ETF', quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 0, price_scale: 2, effective_from: opened });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, { instrument_id: instrument.id, name: 'Pool holding', opened_on: opened, effective_from: opened });
  await apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, { transaction_date: daysFromTodayISO(-12), commodity_id: instrument.commodity_id, holding_account_id: holding.id, cash_account_id: checking.id, quantity_value: '10', quantity_scale: 0, cash_amount_value: '100000', cash_amount_scale: 2, cash_commodity_id: currencyID, cost_basis_method: 'fifo' });
  await apiJSON(page, 'POST', '/api/v1/pricing/prices', csrfToken, { base_commodity_id: instrument.commodity_id, quote_commodity_id: currencyID, quote_type: 'close', adjustment_basis: 'raw', price_value: 6000, price_scale: 2, valuation_date: end, is_manual: true });
  const url = `/app/investments/allocation?from=${start}&to=${end}&reporting_currency_id=${currencyID}`;
  await page.goto(url);
  await expect(page.getByRole('heading', { name: 'Allocation', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Edit pool and targets', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Edit pool and targets' });
  // Exact membership, not cash-kind guesses: the deposit is in, college is out.
  for (const box of await dialog.getByRole('checkbox').all()) await box.uncheck();
  await dialog.getByRole('checkbox', { name: 'Pool settlement', exact: true }).check();
  await dialog.getByRole('checkbox', { name: 'Raisin fixed deposit', exact: true }).check();
  await dialog.getByRole('checkbox', { name: 'Pool holding', exact: true }).check();
  await dialog.getByRole('combobox', { name: 'Add a target', exact: true }).selectOption(String(instrument.commodity_id));
  await dialog.getByRole('button', { name: 'Add', exact: true }).click();
  await dialog.getByLabel('Pool equity ETF · Target', { exact: true }).fill('50');
  await dialog.getByRole('combobox', { name: 'Add a target', exact: true }).selectOption('0');
  await dialog.getByRole('button', { name: 'Add', exact: true }).click();
  await dialog.getByLabel('Cash and deposits · Target', { exact: true }).fill('50');
  await expect(dialog.getByText('Target total: 100%', { exact: true })).toBeVisible();
  await expectNoAccessibilityViolations(page, 'Allocation editor on a phone');
  await dialog.getByRole('button', { name: 'Save pool and targets', exact: true }).click();
  await expect(dialog).toBeHidden();
  await expect(page.getByTestId('allocation-total')).toContainText('1,000.00');
  const table = page.getByTestId('allocation-table');
  await expect(table.getByRole('row').filter({ hasText: 'Pool equity ETF' })).toContainText('+10.00 pp');
  await expect(table.getByRole('row').filter({ hasText: 'Cash and deposits' })).toContainText('-10.00 pp');
  await page.getByText(/^3 included · [0-9]+ excluded accounts$/).click();
  await expect(page.getByText('College savings outside pool', { exact: true })).toBeVisible();
  await expect(page.getByText(/Total net worth can also include college savings/)).toBeVisible();
  await expect(page.getByTestId('allocation-chart').getByRole('img')).toHaveCount(2);
  await page.getByLabel('Show saved-change markers').check();
  await expect(page.getByTestId('allocation-chart').getByRole('button', { name: /Saved change/ })).toHaveCount(1);
  await page.getByTestId('allocation-chart').getByRole('button', { name: /Saved change/ }).click();
  await expect(page.locator('#allocation-history').locator('..').getByText('Previous: No target', { exact: false }).first()).toBeVisible();
  await expectNoAccessibilityViolations(page, 'Allocation on a phone');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: '/tmp/411-allocation-phone.png', fullPage: true });

  // A newer saved revision must not replace the edit's fields or token when
  // a focus-triggered query refresh updates the page beneath its modal.
  await page.getByRole('button', { name: 'Edit pool and targets', exact: true }).click();
  await dialog.getByLabel('Pool equity ETF · Target', { exact: true }).fill('55');
  await dialog.getByLabel('Cash and deposits · Target', { exact: true }).fill('45');
  const current = await apiJSON<AllocationResponse>(page, 'GET', `/api/v1/investments/allocation?from=${start}&to=${end}&reporting_currency_id=${currencyID}`);
  await apiJSON(page, 'PUT', '/api/v1/investments/allocation', csrfToken, { expected_revision: current.current.id, account_ids: current.current.account_ids, targets: [{ commodity_id: instrument.commodity_id, units: 600000 }, { commodity_id: 0, units: 400000 }] });
  const refresh = page.waitForResponse(r => r.request().method() === 'GET' && r.url().includes('/api/v1/investments/allocation'));
  await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')));
  await refresh;
  await expect(dialog.getByLabel('Pool equity ETF · Target', { exact: true })).toHaveValue('55');
  const refused = page.waitForResponse(r => r.request().method() === 'PUT' && r.url().includes('/api/v1/investments/allocation'));
  await dialog.getByRole('button', { name: 'Save pool and targets', exact: true }).click();
  expect((await refused).status()).toBe(409);
  await expect(dialog.getByRole('alert')).toBeVisible();
  await expect(dialog.getByLabel('Pool equity ETF · Target', { exact: true })).toHaveValue('55');
  page.once('dialog', d => d.accept());
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();

  await page.evaluate(() => localStorage.setItem('rekenraam-theme', 'dark'));
  await page.reload();
  await expect(page.getByTestId('allocation-total')).toContainText('1,000.00');
  await expect(table.getByRole('row').filter({ hasText: 'Pool equity ETF' })).toContainText('0.00 pp');
  await expectNoAccessibilityViolations(page, 'Allocation in dark theme');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.screenshot({ path: '/tmp/411-allocation-desktop.png', fullPage: true });

  // Locale-aware editing through the real consumer, rather than just a parser.
  await page.evaluate(() => localStorage.setItem('PARAGLIDE_LOCALE', 'nl'));
  await page.reload();
  await page.getByRole('button', { name: 'Vermogen en doelen wijzigen', exact: true }).click();
  const dutch = page.getByRole('dialog');
  await dutch.getByLabel('Pool equity ETF · Doel', { exact: true }).fill('60,0001');
  await dutch.getByLabel('Geld en deposito’s · Doel', { exact: true }).fill('39,9999');
  await dutch.getByRole('button', { name: 'Vermogen en doelen opslaan', exact: true }).click();
  await expect(dutch).toBeHidden();
  const final = await apiJSON<AllocationResponse>(page, 'GET', '/api/v1/investments/allocation');
  expect(final.current.targets.find(t => t.commodity_id === instrument.commodity_id)?.units).toBe(600001);
  expect(final.current.account_ids).toContain(deposit.id);
  expect(final.current.account_ids).not.toContain(college.id);
});
