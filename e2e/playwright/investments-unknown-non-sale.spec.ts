import { expect, test } from './support/fixtures';
import AxeBuilder from '@axe-core/playwright';
import { readFileSync } from 'node:fs';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';
import { translationLocales } from './support/locales';

test.use({ viewport: { width: 390, height: 844 } });

function ago(days: number) {
  const date = new Date();
  date.setDate(date.getDate() - days);
  return todayISO(date);
}

for (const locale of translationLocales) {
  const settings = JSON.parse(readFileSync(new URL(`../../frontend/messages/settings/${locale}.json`, import.meta.url), 'utf8'));
  const app = JSON.parse(readFileSync(new URL(`../../frontend/messages/app/${locale}.json`, import.meta.url), 'utf8'));

  test(`unresolved cash in lieu and return of capital stay usable on mobile in ${locale}`, async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const suffix = `un${locale}${Date.now()}`;
    const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
      name: `Cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
      default_commodity_id: currencyID, allows_postings: true, opened_on: ago(60), effective_from: ago(60)
    });
    const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
      commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: suffix, symbol: suffix.toUpperCase(),
      quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: ago(60)
    });
    const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
      instrument_id: instrument.id, name: `Holding ${suffix}`, opened_on: ago(60), effective_from: ago(60)
    });
    await apiJSON(page, 'POST', '/api/v1/investments/transfers/external/in', csrfToken, {
      effective_on: ago(40), holding_account_id: holding.id, commodity_id: instrument.commodity_id,
      quantity_value: '2', quantity_scale: 0, cost_commodity_id: currencyID, basis_knowledge: 'unknown'
    });
    const split = await apiJSON<{ transaction: { id: number } }>(page, 'POST', '/api/v1/investments/splits', csrfToken, {
      effective_on: ago(30), holding_account_id: holding.id, commodity_id: instrument.commodity_id, ratio_numerator: 5, ratio_denominator: 4
    });
    await page.evaluate((value) => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    await page.goto(`/app/ledger/transactions?transaction_id=${split.transaction.id}`);
    await page.getByRole('button', { name: settings.investments_cash_in_lieu_entry_action, exact: true }).click();
    const cil = page.locator('dialog[open]').filter({ has: page.locator('#cil-quantity') });
    // Editable amounts parse in the active locale (#100).
    await cil.locator('#cil-quantity').fill(new Intl.NumberFormat(locale).format(0.5));
    await cil.locator('#cil-payment').fill(ago(25));
    await cil.locator('#cil-cash').selectOption(String(cash.id));
    await cil.locator('#cil-proceeds').fill('8');
    await cil.getByRole('button', { name: settings.investments_cash_in_lieu_preview, exact: true }).click();
    const cilPreview = cil.getByRole('region', { name: settings.investments_cash_in_lieu_preview_title });
    await expect(cilPreview).toContainText(app.investments_basis_unknown);
    await expect(cilPreview).toContainText(app.investments_gain_unresolved);
    expect((await new AxeBuilder({ page }).include('dialog[open]').analyze()).violations).toEqual([]);
    await cil.getByRole('button', { name: settings.investments_cash_in_lieu_submit, exact: true }).click();
    await expect(cil).toBeHidden();

    await page.goto('/app/investments');
    await page.getByRole('button', { name: settings.investments_record_capital_return, exact: true }).click();
    const roc = page.locator('dialog[open]').filter({ has: page.locator('#capital-return-position') });
    await roc.locator('#capital-return-position').selectOption(`${holding.id}:${instrument.commodity_id}:${currencyID}`);
    await roc.locator('#capital-return-cash').selectOption(String(cash.id));
    await roc.locator('#capital-return-effective').fill(ago(20));
    await roc.locator('#capital-return-payment').fill(ago(15));
    await roc.locator('#capital-return-amount').fill('10');
    await roc.getByRole('button', { name: settings.investments_capital_return_preview, exact: true }).click();
    const rocPreview = roc.getByRole('region', { name: settings.investments_capital_return_preview_title });
    await expect(rocPreview).toContainText(settings.investments_capital_return_effect_unknown.split('{currency}')[1]);
    expect((await new AxeBuilder({ page }).include('dialog[open]').analyze()).violations).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
    await roc.getByRole('button', { name: settings.investments_capital_return_submit, exact: true }).click();
    await expect(roc).toBeHidden();
  });
}
