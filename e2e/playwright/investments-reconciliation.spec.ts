import { expect, test, type Page } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { monthStartISO, todayISO } from './support/dates';

/**
 * A trade cannot be dated before the instrument's commodity exists, and the
 * commodity is created during the run — so the reconciled period is put *ahead*
 * of the trade rather than the trade behind the period. Same guard, same code
 * path: the trade lands below the checkpoint's lock floor either way.
 */
function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

/**
 * T-53: a backdated investment trade must be able to proceed deliberately.
 *
 * The guard itself is covered by Go tests. What only a browser can prove is
 * that the user is shown *which* reconciliation the trade would invalidate and
 * gets to decide — an override the user cannot see the consequences of is
 * barely better than no override at all.
 */
test('a backdated buy names the reconciliation it would invalidate, then proceeds', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const openedOn = monthStartISO();
  const statementDate = daysFromTodayISO(7);

  // The account has to pre-date the trade: PostingAccountRule resolves the
  // account version as-of the entry date, so an account created today cannot
  // carry a posting dated earlier for any reason, reconciliation or not.
  const cash = await apiJSON<{ id: number; name: string }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Recon brokerage cash ${suffix}`,
    account_class: 'asset',
    account_kind: 'brokerage_cash',
    default_commodity_id: currencyID,
    allows_postings: true,
    opened_on: openedOn,
    effective_from: openedOn
  });

  const instrument = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: `RC${suffix}`,
    instrument_type: 'stock',
    display_name: `Recon Equity ${suffix}`,
    symbol: `RC${suffix}`,
    quote_commodity_id: currencyID,
    trading_commodity_id: currencyID,
    quantity_scale: 3,
    price_scale: 2,
    effective_from: openedOn
  });
  const holding = await apiJSON<{ id: number; name: string }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id,
    name: `Recon holding ${suffix}`,
    opened_on: openedOn,
    effective_from: openedOn
  });

  await recordBuy(page, {
    instrument: `Recon Equity ${suffix}`,
    holdingID: holding.id,
    cashID: cash.id,
    date: todayISO(),
    quantity: '2.000',
    cost: '20.00'
  });
  await expect(page.getByText(`Recon Equity ${suffix}`).first()).toBeVisible();

  // Reconcile the cash account through the statement date. The buy's cash leg
  // is the only posting on it, so the closing balance is the money that left.
  await page.goto('/app/reconcile');
  await page.getByLabel('Account').selectOption({ label: cash.name });
  await page.getByLabel('Statement closing date').fill(statementDate);
  await page.getByLabel('Statement ending balance').fill('-20.00');
  await page.getByRole('button', { name: 'Start reconciling' }).click();
  await page.getByRole('checkbox').first().check();
  await expect(page.getByText('Balanced')).toBeVisible();
  await page.getByLabel('Reason').fill(`Recon ${suffix}`);
  await page.getByRole('button', { name: 'Finish reconciliation' }).click();
  await expect(page.getByText('Reconciliation complete')).toBeVisible();

  // A second buy on the same cash account, dated inside the reconciled period.
  await recordBuy(page, {
    instrument: `Recon Equity ${suffix}`,
    holdingID: holding.id,
    cashID: cash.id,
    date: todayISO(),
    quantity: '1.000',
    cost: '11.00'
  });

  // The warning must name the account and its statement date, not merely say
  // that something will break.
  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText('This change affects a reconciled period');
  await expect(dialog).toContainText(cash.name);
  await expect(dialog).toContainText(statementDate);

  // Cancelling leaves the ledger alone.
  await dialog.getByRole('button', { name: 'Go back' }).click();
  await expect(dialog).toBeHidden();

  const before = await activeCheckpointCount(page, cash.id);
  expect(before).toBe(1);

  await page.getByRole('button', { name: 'Record buy' }).last().click();
  await expect(page.getByRole('alertdialog')).toBeVisible();
  await page.getByRole('alertdialog').getByRole('button', { name: 'Proceed and invalidate' }).click();

  // The trade lands, and the checkpoint it warned about is no longer claiming
  // the account is reconciled.
  await expect(page.getByRole('alertdialog')).toBeHidden();
  await expect.poll(() => activeCheckpointCount(page, cash.id)).toBe(0);
});

/**
 * T-114: an earlier buy can revise the gain of a sale already recorded, with no
 * reconciliation involved. The user must see the old and new gain and accept
 * them; the issue's FIFO case moves the sale's gain from 50.00 to 140.00.
 */
test('a backdated buy discloses the earlier sale gain it revises before proceeding', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const openedOn = daysFromTodayISO(-30);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Gain brokerage cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: `GN${suffix}`, instrument_type: 'stock', display_name: `Gain Equity ${suffix}`,
    symbol: `GN${suffix}`, quote_commodity_id: currencyID, trading_commodity_id: currencyID,
    quantity_scale: 3, price_scale: 2, effective_from: openedOn
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Gain holding ${suffix}`, opened_on: openedOn, effective_from: openedOn
  });
  const trade = (date: string, quantity: string, cash_amount_value: string) => ({
    transaction_date: date, commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: cash.id, quantity_value: quantity, quantity_scale: 0,
    cash_amount_value, cash_amount_scale: 2, cash_commodity_id: currencyID
  });
  await apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, trade(daysFromTodayISO(-10), '10', '20000'));
  await apiJSON(page, 'POST', '/api/v1/investments/sell', csrfToken, trade(daysFromTodayISO(-5), '5', '15000'));

  await recordBuy(page, {
    instrument: `Gain Equity ${suffix}`, holdingID: holding.id, cashID: cash.id,
    date: daysFromTodayISO(-20), quantity: '10.000', cost: '20.00'
  });
  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText('This changes gains on earlier sales');
  await expect(dialog).toContainText('50.00 → 140.00');

  await dialog.getByRole('button', { name: 'Accept changed gains' }).click();
  await expect(dialog).toBeHidden();
  await expect.poll(async () => {
    const gains = await apiJSON<{ realized: Array<{ commodity_id: number; realized_gain_value: string; realized_gain_scale: number }> }>(
      page, 'GET', '/api/v1/investments/gains');
    const sale = gains.realized.find((gain) => gain.commodity_id === instrument.commodity_id);
    // Replay may store the gain at a wider scale; compare the exact value.
    return sale ? BigInt(sale.realized_gain_value) * 100n === 14000n * 10n ** BigInt(sale.realized_gain_scale) : false;
  }).toBe(true);
});

async function activeCheckpointCount(page: Page, accountID: number): Promise<number> {
  const body = await apiJSON<{ checkpoints: Array<{ status: string }> }>(
    page,
    'GET',
    `/api/v1/accounts/${accountID}/reconciliation-checkpoints`
  );
  return body.checkpoints.filter((checkpoint) => checkpoint.status === 'active').length;
}

async function recordBuy(
  page: Page,
  options: { instrument: string; holdingID: number; cashID: number; date: string; quantity: string; cost: string }
) {
  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Record buy' }).click();
  await page.getByLabel('Date').fill(options.date);
  await page.getByLabel('Instrument').fill(options.instrument);
  await page.getByRole('option').filter({ hasText: options.instrument }).click();
  await page.getByLabel('Holding account').selectOption(String(options.holdingID));
  await page.getByLabel('Quantity').fill(options.quantity);
  await page.getByLabel('Cash account').selectOption(String(options.cashID));
  await page.getByLabel('Total cost').fill(options.cost);
  await page.getByRole('button', { name: 'Record buy' }).last().click();
}
