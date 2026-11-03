import { expect, test } from './support/fixtures';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';
import { expectNoAccessibilityViolations } from './support/a11y';
import { requireTrading212Stub, setTrading212Fills } from './support/trading212';

/**
 * #245: a pending unsupported provider fill is linked, on a phone, to a short
 * sale the owner recorded natively. The link posts nothing, and a re-fetch of
 * the same fill is a duplicate. The fill type is synthetic: this exercises
 * the owner-driven path, not undocumented provider short semantics (#176).
 */

requireTrading212Stub();
test.use({ viewport: { width: 390, height: 844 } });

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

test('[acceptance] an unsupported provider row links to a recorded short sale on a phone without posting twice', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const currencies = await apiJSON<{ currencies: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/currencies');
  const currency = currencies.currencies.find((candidate) => candidate.id === currencyID)!.code;
  const suffix = `SHL${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const tradeDate = daysFromTodayISO(-5);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Short link cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const ticker = `${suffix}_US_EQ`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: ticker, instrument_type: 'stock', display_name: `Short link ${suffix}`, symbol: ticker,
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 0, price_scale: 2, effective_from: openedOn
  });
  const holdingName = `Short link holding ${suffix}`;
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: holdingName, opened_on: openedOn, effective_from: openedOn
  });
  await apiJSON(page, 'POST', '/api/v1/investments/short-sale', csrfToken, {
    transaction_date: tradeDate, commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: cash.id, quantity_value: '10', quantity_scale: 0, cash_amount_value: '100000',
    cash_amount_scale: 2, cash_commodity_id: currencyID
  });
  const activity = () => apiJSON<{ transactions: Array<{ id: number }> }>(page, 'GET', `/api/v1/transactions?commodity_id=${instrument.commodity_id}`);
  expect((await activity()).transactions).toHaveLength(1);

  const apiKey = `e2e-${suffix}`;
  await setTrading212Fills(apiKey, { ticker, isin: '', currency }, [
    { id: 1, side: 'SELL', quantity: '10', price: '100', netValue: '1000', date: tradeDate, type: 'SYNTHETIC_SHORT' }
  ]);
  const connection = `Short link broker ${suffix}`;
  await apiJSON(page, 'POST', '/api/v1/import-connections', csrfToken, {
    source: 'trading212', display_name: connection, api_key: apiKey, cash_account_id: cash.id
  });
  const fetchRows = async (action: 'Import' | 'Re-fetch full history') => {
    await page.goto('/app/ledger/import');
    await page.getByRole('row').filter({ hasText: connection }).getByRole('button', { name: action, exact: true }).click();
    await expect(page.getByRole('button', { name: 'Commit to ledger' })).toBeVisible({ timeout: 15_000 });
  };
  await fetchRows('Import');

  await page.getByRole('button', { name: 'Link to recorded short', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Provider evidence', exact: true })).toBeVisible();
  await expect(page.getByText('SELL · SYNTHETIC_SHORT', { exact: false })).toBeVisible();
  const choice = page.getByRole('radio', { name: new RegExp(holdingName) });
  await expect(choice).toBeEnabled();
  await choice.check();
  const submit = page.getByRole('button', { name: 'Link row', exact: true });
  await expect(submit).toBeInViewport();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await expectNoAccessibilityViolations(page, 'Short link review on a phone');
  await submit.click();
  await expect(page.getByText('Linked to recorded short', { exact: true })).toBeVisible();
  expect((await activity()).transactions).toHaveLength(1);

  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('Import complete')).toBeVisible();
  await fetchRows('Re-fetch full history');
  await expect(page.getByRole('button', { name: 'Link to recorded short', exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('0 transactions committed')).toBeVisible();
  expect((await activity()).transactions).toHaveLength(1);
});
