import { expect, test, type Page } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';
import { requireTrading212Stub, setTrading212Fills, type StubFill, type StubInstrument } from './support/trading212';

/**
 * T-128: browser evidence for the two gain-review families the Go service
 * tests already pin — imported acquisitions and Trading 212 source
 * revisions. Fills come from the provider stub through the real fetch
 * worker; every later fetch uses the connection's own Refresh, which is
 * incremental, so each scenario only revises or adds fills at or after the
 * saved cursor, as the provider would.
 */

requireTrading212Stub();

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

type Broker = {
  csrfToken: string;
  currencyID: number;
  cashID: number;
  apiKey: string;
  connection: string;
  instrument: StubInstrument;
};

/** A cash account, a stub account with these fills and a connection to it. */
async function broker(page: Page, label: string, fills: StubFill[]): Promise<Broker> {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const currencies = await apiJSON<{ currencies: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/currencies');
  const currency = currencies.currencies.find((candidate) => candidate.id === currencyID)!.code;
  const suffix = `${label}${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `T212 cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const instrument = {
    ticker: `${suffix.toUpperCase()}_US_EQ`,
    isin: `XS${String(Date.now()).slice(-10)}`,
    currency
  };
  const apiKey = `e2e-${suffix}`;
  await setTrading212Fills(apiKey, instrument, fills);
  const connection = `Stub broker ${suffix}`;
  await apiJSON(page, 'POST', '/api/v1/import-connections', csrfToken, {
    source: 'trading212', display_name: connection, api_key: apiKey, cash_account_id: cash.id
  });
  return { csrfToken, currencyID, cashID: cash.id, apiKey, connection, instrument };
}

/**
 * Starts the connection's fetch from the import page ("Import" the first
 * time, "Refresh" after) and waits for the fetched rows.
 */
async function fetchFromConnection(page: Page, b: Broker, action: 'Import' | 'Refresh') {
  await page.goto('/app/import');
  await page.getByRole('row').filter({ hasText: b.connection }).getByRole('button', { name: action, exact: true }).click();
  await expect(page.getByRole('button', { name: 'Commit to ledger' })).toBeVisible({ timeout: 15_000 });
}

/** The holding the importer created for this broker's instrument. */
async function importedHolding(page: Page, b: Broker): Promise<{ commodityID: number; holdingID: number }> {
  const instruments = await apiJSON<{ instruments: Array<{ symbol?: string; commodity_id: number }> }>(page, 'GET', '/api/v1/investments/instruments');
  const commodityID = instruments.instruments.find((instrument) => instrument.symbol === b.instrument.ticker)!.commodity_id;
  const positions = await apiJSON<{ positions: Array<{ account_id: number; commodity_id: number }> }>(page, 'GET', '/api/v1/investments/positions');
  return { commodityID, holdingID: positions.positions.find((position) => position.commodity_id === commodityID)!.account_id };
}

/**
 * A sale entered by hand on the imported holding under LIFO, so a purchase
 * the provider reports later — but before the sale — changes its basis.
 */
async function manualLIFOSale(page: Page, b: Broker, daysAgo: number, quantity: string, cash: string) {
  const { commodityID, holdingID } = await importedHolding(page, b);
  await apiJSON(page, 'POST', '/api/v1/investments/sell', b.csrfToken, {
    transaction_date: daysFromTodayISO(-daysAgo), commodity_id: commodityID, holding_account_id: holdingID,
    cash_account_id: b.cashID, quantity_value: quantity, quantity_scale: 0, cash_amount_value: cash,
    cash_amount_scale: 2, cash_commodity_id: b.currencyID, cost_basis_method: 'lifo'
  });
}

/** Exact realized gains for this broker's instrument, as value/100 strings. */
async function gainsFor(page: Page, b: Broker): Promise<string[]> {
  const { commodityID } = await importedHolding(page, b);
  const gains = await apiJSON<{ realized: Array<{ commodity_id: number; realized_gain_value: string; realized_gain_scale: number }> }>(
    page, 'GET', '/api/v1/investments/gains');
  return gains.realized
    .filter((gain) => gain.commodity_id === commodityID)
    .map((gain) => ((BigInt(gain.realized_gain_value) * 100n) / 10n ** BigInt(gain.realized_gain_scale)).toString())
    .sort();
}

const buy = (id: number, daysAgo: number, quantity: string, price: string, net: string): StubFill =>
  ({ id, side: 'BUY', quantity, price, netValue: net, date: daysFromTodayISO(-daysAgo) });
const sell = (id: number, daysAgo: number, quantity: string, price: string, net: string): StubFill =>
  ({ id, side: 'SELL', quantity, price, netValue: net, date: daysFromTodayISO(-daysAgo) });

test('an imported purchase that changes a sale gain is reviewed before it commits', async ({ page }) => {
  const first = buy(1, 30, '10', '20', '-200');
  const b = await broker(page, 'ACQ', [first]);
  await fetchFromConnection(page, b, 'Import');
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('Import complete')).toBeVisible();
  await manualLIFOSale(page, b, 10, '5', '15000');
  await expect.poll(() => gainsFor(page, b)).toEqual(['5000']);

  // The provider reports a cheaper purchase made before the sale.
  await setTrading212Fills(b.apiKey, b.instrument, [first, buy(2, 20, '10', '2', '-20')]);
  await fetchFromConnection(page, b, 'Refresh');
  await page.getByRole('button', { name: 'Commit to ledger' }).click();

  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText('This changes gains on earlier sales');
  await expect(dialog).toContainText(`Sale on ${daysFromTodayISO(-10)}: basis 100.00 → 10.00, gain 50.00 → 140.00`);
  await dialog.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(dialog).toBeHidden();
  await expect(page.getByText('Import complete')).toBeVisible();
  await expect(page.getByText(/were not committed yet/)).toHaveCount(0);
  await expect.poll(() => gainsFor(page, b)).toEqual(['14000']);
});

test.describe('on a phone', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('a purchase changed by an earlier row in the same run is reviewed again from the result', async ({ page }) => {
    const first = buy(1, 30, '10', '20', '-200');
    const b = await broker(page, 'HELD', [first]);
    await fetchFromConnection(page, b, 'Import');
    await page.getByRole('button', { name: 'Commit to ledger' }).click();
    await expect(page.getByText('Import complete')).toBeVisible();
    await manualLIFOSale(page, b, 10, '5', '15000');

    // Two purchases before the sale. Each is previewed against the current
    // ledger; committing the earlier one changes the later one's gain set.
    await setTrading212Fills(b.apiKey, b.instrument, [first, buy(2, 20, '10', '1', '-10'), buy(3, 15, '10', '2', '-20')]);
    await fetchFromConnection(page, b, 'Refresh');
    await page.getByRole('button', { name: 'Commit to ledger' }).click();

    const dialog = page.getByRole('alertdialog');
    await expect(dialog).toContainText(`Sale on ${daysFromTodayISO(-10)}: basis 100.00 → 5.00, gain 50.00 → 145.00`);
    await expect(dialog).toContainText(`Sale on ${daysFromTodayISO(-10)}: basis 100.00 → 10.00, gain 50.00 → 140.00`);
    await dialog.getByRole('button', { name: 'Accept changed gains' }).click();
    await expect(dialog).toBeHidden();

    await expect(page.getByText('1 imported purchases would change gains on earlier sales and were not committed yet.')).toBeVisible();
    await expect.poll(() => gainsFor(page, b)).toEqual(['14500']);
    await page.getByRole('button', { name: 'Review changed gains' }).click();
    await expect(dialog).toContainText('The affected gains changed since your review.');
    await expect(dialog).toContainText(`Sale on ${daysFromTodayISO(-10)}: basis 5.00 → 10.00, gain 145.00 → 140.00`);
    await dialog.getByRole('button', { name: 'Accept changed gains' }).click();
    await expect(dialog).toBeHidden();
    await expect(page.getByText(/were not committed yet/)).toHaveCount(0);
    await expect.poll(() => gainsFor(page, b)).toEqual(['14000']);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  });

  test('a revised sale from the provider is corrected only after accepting its changed gain', async ({ page }) => {
    const purchase = buy(1, 30, '10', '20', '-200');
    const b = await broker(page, 'REV', [purchase, sell(2, 10, '5', '30', '150')]);
    await fetchFromConnection(page, b, 'Import');
    await page.getByRole('button', { name: 'Commit to ledger' }).click();
    await expect(page.getByText('Import complete')).toBeVisible();
    await expect.poll(() => gainsFor(page, b)).toEqual(['5000']);

    // The provider revises the sale's proceeds; it is the newest fill, so the
    // incremental refresh fetches it again.
    await setTrading212Fills(b.apiKey, b.instrument, [purchase, sell(2, 10, '5', '50', '250')]);
    await fetchFromConnection(page, b, 'Refresh');
    await page.getByRole('button', { name: 'Correct sale from source' }).click();
    await page.getByLabel('Reason for correction').fill('broker revised proceeds');
    await page.getByRole('button', { name: 'Preview reconciliation impact' }).click();

    await expect(page.getByText(`Sale on ${daysFromTodayISO(-10)} replaced: basis 100.00 → 100.00, gain 50.00 → 150.00`)).toBeVisible();
    const apply = page.getByRole('button', { name: 'Apply source correction' });
    await expect(apply).toBeDisabled();
    await page.getByRole('checkbox', { name: 'Accept changed gains' }).check();
    await expect(apply).toBeEnabled();
    await apply.click();
    await expect(page.getByText('Source trade corrected')).toBeVisible();
    await expect.poll(() => gainsFor(page, b)).toEqual(['15000']);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  });
});
