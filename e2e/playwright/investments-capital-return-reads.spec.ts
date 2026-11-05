import { expect, test } from './support/fixtures';
import AxeBuilder from '@axe-core/playwright';
import { readFileSync } from 'node:fs';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';
import { scopedDomTrace } from '../trace-options';
import { translationLocales } from './support/locales';

test.use({
  viewport: { width: 390, height: 844 },
  // #414's original stall predates failure tracing. Keep DOM snapshots
  // and browser network evidence for this small spec so the next occurrence
  // can distinguish a missing control, actionability wait and lost dialog.
  // Other journeys retain the lighter global trace; trace screenshots stay off.
  // E2E_FULL_TRACE=1 upgrades every spec, including this one, to a full trace.
  trace: scopedDomTrace
});
function ago(days: number) { const date = new Date(); date.setDate(date.getDate() - days); return todayISO(date); }

for (const locale of translationLocales) {
  const settings = JSON.parse(readFileSync(new URL(`../../frontend/messages/settings/${locale}.json`, import.meta.url), 'utf8'));
  test(`recorded capital-return excess, revisions and unknown results in ${locale} on mobile`, async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const suffix = `rc${locale}${Date.now()}`;
    const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
      name: suffix, account_class: 'asset', account_kind: 'brokerage_cash', default_commodity_id: currencyID,
      allows_postings: true, opened_on: ago(60), effective_from: ago(60)
    });
    const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
      commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: suffix, symbol: suffix.toUpperCase(),
      quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: ago(60)
    });
    const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
      instrument_id: instrument.id, name: `Holding ${suffix}`, opened_on: ago(60), effective_from: ago(60)
    });
    const terms = { transaction_date: ago(40), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
      cash_account_id: cash.id, quantity_value: '1', quantity_scale: 0, cash_amount_value: '700', cash_amount_scale: 2, cash_commodity_id: currencyID };
    const buy = await apiJSON<{ transaction: { id: number } }>(page, 'POST', '/api/v1/investments/buy', csrfToken, terms);
    const receiptTerms = { holding_account_id: holding.id, commodity_id: instrument.commodity_id, cash_account_id: cash.id, currency_id: currencyID,
      effective_on: ago(30), payment_on: ago(20), amount_value: '1000', amount_scale: 2, source_evidence: { reference: suffix, precise: '9007199254740993' } };
    const receipt = await apiJSON<{ transaction: { id: number } }>(page, 'POST', '/api/v1/investments/return-of-capital', csrfToken, receiptTerms);
    await page.evaluate((value) => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    await page.goto(`/app/ledger/transactions?transaction_id=${receipt.transaction.id}`);
    const history = page.getByRole('region', { name: settings.investments_roc_history });
    let card = history.getByRole('article').filter({ hasText: settings.investments_roc_current });
    await expect(card).toContainText(settings.investments_roc_excess);
    await expect(card).toContainText(settings.investments_roc_decision_note);
    await expect(card).toContainText('3');
    await card.locator('summary').click();
    await expect(card).toContainText('9007199254740993');
    await expect(card).toContainText(suffix);
    expect((await new AxeBuilder({ page }).include('article').analyze()).violations).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);

    const correctedBuy = await apiJSON<{ replacement: { transaction: { id: number } } }>(page, 'POST', `/api/v1/investments/transactions/${buy.transaction.id}/replace-buy`, csrfToken, {
      reason: 'source basis corrected', replacement: { ...terms, cash_amount_value: '500' }
    });
    await page.reload();
    await expect(history.getByRole('article')).toHaveCount(2);
    card = history.getByRole('article').filter({ hasText: settings.investments_roc_current });
    await expect(card).toContainText('5');
    await expect(history.getByRole('article').filter({ hasText: settings.investments_roc_historical })).toContainText('3');
    await apiJSON(page, 'POST', `/api/v1/investments/transactions/${correctedBuy.replacement.transaction.id}/replace-buy`, csrfToken, {
      reason: 'verified original basis', replacement: { ...terms, cash_amount_value: '1200' }
    });
    await page.reload();
    await expect(history.getByRole('article')).toHaveCount(3);
    await expect(history.getByRole('article').filter({ hasText: settings.investments_roc_current })).toContainText(settings.investments_roc_no_excess);
    await page.goto('/app/investments');
    await page.getByRole('link', { name: settings.investments_tab_gains, exact: true }).click();
    const summary = page.getByRole('region', { name: settings.investments_roc_summary_title });
    await expect(summary.getByRole('article').filter({ hasText: suffix })).toHaveCount(1);
    await expect(summary.getByRole('article').filter({ hasText: suffix })).toContainText(settings.investments_roc_no_excess);
    await test.step('load the capital-return transaction for reversal', async () => {
      await page.goto(`/app/ledger/transactions?transaction_id=${receipt.transaction.id}`);
    });
    await test.step('click the capital-return reversal action', async () => {
      await page.getByRole('button', { name: settings.transactions_investment_reverse_capital_return_action, exact: true }).click();
    });
    const reversal = page.getByRole('alertdialog');
    await test.step('enter the capital-return reversal reason', async () => {
      await reversal.locator('input').fill('notice withdrawn');
    });
    // Stable control id avoids tying financial verification to translated text.
    await test.step('submit the capital-return reversal', async () => {
      await reversal.getByRole('button').last().click();
    });
    await test.step('wait for the capital-return reversal dialog to close', async () => {
      await expect(reversal).toBeHidden();
    });
    await page.goto(`/app/ledger/transactions?transaction_id=${receipt.transaction.id}`);
    await expect(history.getByRole('article')).toHaveCount(3);
    await expect(history).not.toContainText(settings.investments_roc_current);

    const unknown = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
      instrument_id: instrument.id, name: `Unknown ${suffix}`, opened_on: ago(60), effective_from: ago(60)
    });
    await apiJSON(page, 'POST', '/api/v1/investments/transfers/external/in', csrfToken, {
      effective_on: ago(40), holding_account_id: unknown.id, commodity_id: instrument.commodity_id,
      quantity_value: '2', quantity_scale: 0, cost_commodity_id: currencyID, basis_knowledge: 'unknown'
    });
    await apiJSON(page, 'POST', '/api/v1/investments/return-of-capital', csrfToken, { ...receiptTerms, holding_account_id: unknown.id });
    await page.goto('/app/investments');
    await page.getByRole('link', { name: settings.investments_tab_gains, exact: true }).click();
    const unknownCard = summary.getByRole('article').filter({ hasText: `Unknown ${suffix}` });
    await expect(unknownCard).toContainText(settings.investments_roc_unresolved);
    await expect(summary.getByRole('article').filter({ hasText: `Holding ${suffix}` })).toHaveCount(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  });
}

test('return-of-capital summary exposes loading, error and empty states', async ({ page }) => {
  await readyForLedger(page);
  let finishLoading!: () => void;
  const loading = new Promise<void>((resolve) => { finishLoading = resolve; });
  await page.route('**/api/v1/investments/gains?*', async (route) => {
    await loading;
    await route.fulfill({ status: 200, json: { currencies: [], realized: [], unrealized: [], realized_totals: [], capital_returns: [] } });
  });
  await page.goto('/app/investments');
  await page.getByRole('link', { name: 'Gains', exact: true }).click();
  const summary = page.getByRole('region', { name: 'Return-of-capital results' });
  await expect(summary.getByRole('status')).toBeVisible();
  finishLoading();
  await expect(summary).toContainText('No effective return-of-capital receipts');
  await page.unrouteAll({ behavior: 'wait' });
  await page.route('**/api/v1/investments/gains?*', (route) => route.fulfill({ status: 500, json: { error: { code: 'INTERNAL_ERROR', message: 'failure' } } }));
  await page.reload();
  await page.getByRole('link', { name: 'Gains', exact: true }).click();
  await expect(summary.getByRole('alert')).toBeVisible({ timeout: 15000 });
});
