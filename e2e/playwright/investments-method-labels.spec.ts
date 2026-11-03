import { readFileSync } from 'node:fs';
import { expect, test, type Locator, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * #461: the sell and short-cover forms label cost-basis methods through the
 * same translated catalog as every other investment workflow, in their method
 * selects and in their previews.
 */

test.use({ viewport: { width: 390, height: 844 } });

const locale = 'nl';
const settings = JSON.parse(readFileSync(new URL(`../../frontend/messages/settings/${locale}.json`, import.meta.url), 'utf8'));
const methodLabels = [
  settings.transactions_investment_replace_method_fifo,
  settings.transactions_investment_replace_method_lifo,
  settings.transactions_investment_replace_method_average
];

function ago(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() - days);
  return todayISO(date);
}

async function position(page: Page, csrfToken: string, currencyID: number, label: string) {
  const suffix = `${label}${Date.now()}`;
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: ago(60), effective_from: ago(60)
  });
  const name = `Method ${suffix}`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: name, symbol: suffix.toUpperCase(),
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: ago(60)
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Holding ${suffix}`, opened_on: ago(60), effective_from: ago(60)
  });
  const trade = (path: string) => apiJSON(page, 'POST', `/api/v1/investments/${path}`, csrfToken, {
    transaction_date: ago(10), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: cash.id, quantity_value: '10', quantity_scale: 0, cash_amount_value: '10000',
    cash_amount_scale: 2, cash_commodity_id: currencyID
  });
  return { name, cashID: cash.id, holdingID: holding.id, trade };
}

async function fillTrade(form: Locator, prefix: string, p: Awaited<ReturnType<typeof position>>) {
  await form.locator(`#${prefix}-date`).fill(ago(2));
  await form.locator(`#${prefix}-instrument`).fill(p.name);
  await form.getByRole('option').filter({ hasText: p.name }).click();
  await form.locator(`#${prefix}-holding-account`).selectOption(String(p.holdingID));
  await form.locator(`#${prefix}-quantity`).fill('4');
  await form.locator(`#${prefix}-cash-account`).selectOption(String(p.cashID));
  await form.locator(`#${prefix}-cash-amount`).fill('50');
}

test(`sell and short-cover method labels are translated in ${locale} (#461)`, async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const long = await position(page, csrfToken, currencyID, 'long');
  await long.trade('buy');
  const short = await position(page, csrfToken, currencyID, 'short');
  await short.trade('short-sale');
  await page.evaluate((value) => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);

  await page.goto('/app/investments');
  await page.getByRole('button', { name: settings.investments_record_sell, exact: true }).click();
  let form = page.locator('dialog[open]');
  await fillTrade(form, 'sell', long);
  await expect(form.locator('#sell-method option')).toHaveText(methodLabels);
  await form.locator('#sell-method').selectOption('average_cost');
  await expect(form).toContainText(settings.investments_sell_preview_title);
  await expect(form).toContainText(settings.transactions_investment_replace_method_average);
  await expect(form).not.toContainText('Average cost');
  await page.keyboard.press('Escape');
  await expect(form).toBeHidden();

  await page.getByRole('button', { name: settings.investments_record_short_cover, exact: true }).click();
  form = page.locator('dialog[open]');
  await fillTrade(form, 'short-cover', short);
  await expect(form.locator('#short-cover-method option')).toHaveText(methodLabels);
  await form.locator('#short-cover-method').selectOption('average_cost');
  const preview = form.getByTestId('short-cover-preview');
  await expect(preview).toContainText(settings.transactions_investment_replace_method_average);
  await expect(preview).not.toContainText('Average cost');
});
