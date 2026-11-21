import { test, expect } from './support/fixtures';
import AxeBuilder from '@axe-core/playwright';
import { readFile } from 'node:fs/promises';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { daysFromTodayISO, todayISO } from './support/dates';

test.use({ viewport: { width: 390, height: 844 } });

test.describe.serial('saved gains reports', () => {
  // The journey saves this report; the locale tests below reopen it.
  let gainsRun: { id: number; inputs: { profile: { knowledge_cutoff: string } } };

  test('saved gains and statements keep their figures after a sale reversal at 390px', async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const suffix = Date.now();
    const openedOn = daysFromTodayISO(-60);
    const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
      name: `Saved report cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
      default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
    });
    const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
      commodity_code: `SAV${suffix}`, instrument_type: 'stock', display_name: `Saved report ${suffix}`, symbol: `SAV${suffix}`,
      quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 0, price_scale: 2, effective_from: openedOn
    });
    const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
      instrument_id: instrument.id, name: `Saved report holding ${suffix}`, opened_on: openedOn, effective_from: openedOn
    });
    await apiJSON(page, 'POST', '/api/v1/investments/transfers/external/in', csrfToken, {
      effective_on: daysFromTodayISO(-40), holding_account_id: holding.id, commodity_id: instrument.commodity_id,
      quantity_value: '2', quantity_scale: 0, carried_basis_value: '10000', carried_basis_scale: 2,
      cost_commodity_id: currencyID, original_acquired_on: daysFromTodayISO(-40), source_evidence: { statement: 'Saved gains run fixture' }
    });
    const sale = await apiJSON<{ transaction: { id: number } }>(page, 'POST', '/api/v1/investments/sell', csrfToken, {
      transaction_date: daysFromTodayISO(-10), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
      cash_account_id: cash.id, quantity_value: '1', quantity_scale: 0, cash_amount_value: '8000', cash_amount_scale: 2,
      cash_commodity_id: currencyID, cost_basis_method: 'fifo'
    });
    await apiJSON(page, 'POST', '/api/v1/pricing/prices', csrfToken, {
      base_commodity_id: instrument.commodity_id, quote_commodity_id: currencyID, quote_type: 'manual', adjustment_basis: 'raw',
      price_value: 9000, price_scale: 2, valuation_date: daysFromTodayISO(-5), is_manual: true
    });

    await page.goto('/app/investments');
    await page.getByRole('link', { name: 'Gains', exact: true }).click();
    await page.getByLabel('From', { exact: true }).fill(daysFromTodayISO(-35));
    await page.getByLabel('To', { exact: true }).fill(todayISO());
    await page.getByRole('button', { name: 'Apply', exact: true }).click();

    const gainsResponse = page.waitForResponse((res) => res.url().endsWith('/api/v1/investments/gains/runs') && res.request().method() === 'POST');
    await page.getByRole('button', { name: 'Save gains report', exact: true }).click();
    gainsRun = await (await gainsResponse).json();
    expect(gainsRun.id).toBeGreaterThan(0);
    const saved = page.getByTestId('saved-gains-run');
    await expect(saved).toContainText(`Saved report #${gainsRun.id}`);
    const position = saved.locator('p').filter({ hasText: `Instrument #${instrument.commodity_id} · Account #${holding.id}` });
    await expect(position).toHaveCount(2);
    await expect(position.first().locator('..')).toContainText('30.00');
    await expect(position.last().locator('..')).toContainText('40.00');

    const statement = page.getByTestId('gains-statement');
    await statement.getByLabel('Period start').fill(daysFromTodayISO(-30));
    await statement.getByRole('button', { name: 'Show statement' }).click();
    const statementResponse = page.waitForResponse((res) => res.url().endsWith('/api/v1/investments/gains/runs') && res.request().method() === 'POST');
    await statement.getByRole('button', { name: 'Save period statement', exact: true }).click();
    const statementRun = await (await statementResponse).json();
    await expect(saved).toContainText(`Saved report #${statementRun.id}`);
    await expect(saved.getByTestId('gains-statement')).toBeVisible();
    await expect(saved.getByLabel('Period start')).toHaveCount(0);

    const reversalPath = `/api/v1/investments/transactions/${sale.transaction.id}/reverse-sale`;
    const reason = { reason: 'Broker canceled this fill after the report was saved' };
    const preview = await apiJSON<{ gain_impact: { acknowledgement: string } }>(page, 'POST', `${reversalPath}/reconciliation-impact`, csrfToken, reason);
    await apiJSON(page, 'POST', reversalPath, csrfToken, { ...reason, gain_impact_acknowledgement: preview.gain_impact.acknowledgement });
    const retrieved = await apiJSON(page, 'GET', `/api/v1/investments/gains/runs/${gainsRun.id}`);
    expect(retrieved).toEqual(gainsRun);
    expect(await apiJSON(page, 'GET', `/api/v1/investments/gains/runs/${statementRun.id}`)).toEqual(statementRun);
    const live = await apiJSON<{ realized: Array<{ transaction_id: number }> }>(page, 'GET', '/api/v1/investments/gains');
    expect(live.realized.some((row) => row.transaction_id === sale.transaction.id)).toBe(false);
  });

  // One test per locale (#251).
  const locales = [
    ['en', 'Gains', 'Saved gains reports'], ['nl', 'Winsten', 'Opgeslagen winstoverzichten'],
    ['ru', 'Прибыль', 'Сохранённые отчёты о прибыли']
  ] as const;
  for (const [locale, tab, title] of locales) {
    test(`a saved gains report reopens unchanged at 390px in ${locale}`, async ({ page }) => {
      await readyForLedger(page);
      await page.evaluate((value) => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
      await page.goto('/app/investments');
      await page.getByRole('link', { name: tab, exact: true }).click();
      const list = page.getByTestId('gains-runs');
      await expect(list.getByRole('heading', { name: title, exact: true })).toBeVisible();
      await list.getByRole('button', { name: new RegExp(`^#?${gainsRun.id} ·|^№${gainsRun.id} ·`) }).click();
      const saved = page.getByTestId('saved-gains-run');
      await expect(saved).toContainText(gainsRun.inputs.profile.knowledge_cutoff);
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
      if (locale === 'en') {
        const downloadEvent = page.waitForEvent('download');
        await saved.getByRole('link', { name: 'Download saved report (JSON)' }).click();
        const download = await downloadEvent;
        expect(download.suggestedFilename()).toBe(`rekenraam-gains-run-${gainsRun.id}.json`);
        const path = await download.path();
        expect(JSON.parse(await readFile(path!, 'utf8'))).toEqual(gainsRun);
        expect((await new AxeBuilder({ page }).include('main').analyze()).violations).toEqual([]);
      }
    });
  }
});
