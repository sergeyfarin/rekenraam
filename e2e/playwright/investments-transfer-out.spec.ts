import { expect, test, type Page } from './support/fixtures';
import { apiExactText, apiJSON, exactJSON } from './support/api';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * T-142: part of a lot leaves the book on a phone-sized screen. The preview
 * shows the basis the server computes from the lot — not the broker's figure
 * — and the commit posts it as a labelled "Cost basis transferred out" entry.
 */

test.use({ viewport: { width: 390, height: 844 } });

function daysFromTodayISO(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return todayISO(date);
}

async function setup(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const currencies = await apiJSON<{ currencies: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/currencies');
  const currencyCode = currencies.currencies.find((currency) => currency.id === currencyID)?.code ?? '';
  const suffix = `out${Date.now()}`;
  const openedOn = daysFromTodayISO(-60);
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Out cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: openedOn, effective_from: openedOn
  });
  const name = `Out Co ${suffix}`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: name, symbol: suffix.toUpperCase(),
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: openedOn
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Out holding ${suffix}`, opened_on: openedOn, effective_from: openedOn
  });
  await apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, {
    transaction_date: daysFromTodayISO(-30), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: cash.id, quantity_value: '3', quantity_scale: 0, cash_amount_value: '3000',
    cash_amount_scale: 2, cash_commodity_id: currencyID
  });
  return { commodityID: instrument.commodity_id, holdingID: holding.id,
    sourceLabel: `Out holding ${suffix} · ${name} · ${currencyCode}` };
}

test('part of a lot transfers out of the book with its computed cost basis on mobile', async ({ page }) => {
  const s = await setup(page);
  const transferDate = daysFromTodayISO(-10);
  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Transfer out investments' }).click();
  const dialog = page.getByRole('dialog', { name: 'Transfer investments out' });
  await dialog.getByLabel('Transfer date in this book').fill(transferDate);
  await dialog.getByLabel('Source position').selectOption({ label: s.sourceLabel });
  await dialog.getByRole('textbox', { name: /Quantity to move/ }).fill('2');
  await dialog.getByLabel('Basis reported by the broker (optional)').fill('19.50');
  await dialog.getByRole('button', { name: 'Preview transfer' }).click();

  // Two of three shares bought for 30.00 carry 20.00, whatever the broker says.
  const preview = dialog.getByRole('region', { name: 'What leaves the book' });
  await expect(preview).toContainText('Cost basis transferred out: 20.00');
  await expect(preview).toContainText('separate entry labelled');
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  const recorded = page.waitForResponse((response) => response.request().method() === 'POST'
    && new URL(response.url()).pathname === '/api/v1/investments/transfers/external/out');
  await dialog.getByRole('button', { name: 'Record transfer' }).click();
  const response = await recorded;
  expect(response.status()).toBe(201);
  const transfer = (await response.json()) as { transaction: { id: number } };
  await expect(dialog).toBeHidden();

  await expect.poll(async () => {
    const positions = await apiJSON<{ positions: Array<{ account_id: number; commodity_id: number; quantity_value: string }> }>(
      page, 'GET', '/api/v1/investments/positions');
    return positions.positions.find((position) =>
      position.account_id === s.holdingID && position.commodity_id === s.commodityID)?.quantity_value ?? null;
  }).toBe('1');
  await expectOwnTransferBridge(page, transfer.transaction.id, transferDate, '2000');
});

// #406: the worker's book is shared by every spec it runs, so the bridge is
// found through this transfer's own operation, never by scanning a list page
// that later-dated entries from other specs can fill.
type ListedTransaction = { id: number; system_label?: string;
  metadata: { transfer_bridge_of_operation_id?: number; caused_by_operation_id?: number } | null;
  journal_entries: Array<{ postings: Array<{ quantity_value: string; quantity_scale: number }> }> };

async function expectOwnTransferBridge(page: Page, transferID: number, transferDate: string, basisCents: string) {
  const chain = await apiJSON<{ root_operation_id: number }>(page, 'GET',
    `/api/v1/investments/transactions/${transferID}/correction-chain`);
  const onDate: ListedTransaction[] = [];
  let cursor: string | null = null;
  do {
    const query: string = `after_date=${transferDate}&before_date=${transferDate}&kind=investment&limit=200`
      + (cursor ? `&cursor=${encodeURIComponent(cursor)}` : '');
    const listed = await apiJSON<{ transactions: ListedTransaction[]; next_cursor: string | null }>(
      page, 'GET', `/api/v1/transactions?${query}`);
    onDate.push(...listed.transactions);
    cursor = listed.next_cursor;
  } while (cursor);
  const bridges = onDate.filter((transaction) =>
    transaction.metadata?.transfer_bridge_of_operation_id === chain.root_operation_id
    && transaction.metadata?.caused_by_operation_id === undefined);
  expect(bridges).toHaveLength(1);
  const bridge = bridges[0];
  expect(bridge.system_label).toBe('transfer_bridge');
  // Exact at whatever scale the basis is stored: value / 10^scale = ±basis.
  const postings = bridge.journal_entries.flatMap((entry) => entry.postings);
  const signs = postings.map((posting) => {
    const value = BigInt(posting.quantity_value) * BigInt(100);
    const basis = BigInt(basisCents) * BigInt(10) ** BigInt(posting.quantity_scale);
    return value === basis ? 1 : value === -basis ? -1 : 0;
  }).sort();
  expect(signs).toEqual([-1, 1]);

  await page.goto(`/app/ledger/transactions?transaction_id=${bridge.id}`);
  await expect(page.getByRole('heading', { level: 2, name: 'Cost basis transferred out' })).toBeVisible();
}

// T-144: an outbound transfer is corrected, then the correction is reversed,
// from the transaction detail on a phone.
test('an outbound transfer is corrected and then reversed on mobile', async ({ page }) => {
  const { csrfToken } = await readyForLedger(page);
  const s = await setup(page);
  const positions = await apiJSON<{ positions: Array<{ account_id: number; cost_commodity_id: number }> }>(
    page, 'GET', '/api/v1/investments/positions');
  const costID = positions.positions.find((position) => position.account_id === s.holdingID)!.cost_commodity_id;
  const lots = await apiJSON<{ lots: Array<{ id: number }> }>(page, 'GET',
    `/api/v1/investments/lots?account_id=${s.holdingID}&commodity_id=${s.commodityID}`);
  // #312: evidence goes as raw text so its exact tokens reach the server.
  const out = JSON.parse(await apiExactText(page, 'POST', '/api/v1/investments/transfers/external/out', csrfToken, exactJSON({
    effective_on: daysFromTodayISO(-10), source_account_id: s.holdingID, commodity_id: s.commodityID,
    cost_commodity_id: costID, lot_allocations: [{ lot_id: lots.lots[0].id, quantity_value: '2', quantity_scale: 0 }],
    source_evidence: { reference: 'broker letter', revision: 'EXACT:9007199254740993' }
  }))) as { transaction: { id: number } };
  const held = async () => {
    const result = await apiJSON<{ positions: Array<{ account_id: number; commodity_id: number; quantity_value: string }> }>(
      page, 'GET', '/api/v1/investments/positions');
    return result.positions.find((position) =>
      position.account_id === s.holdingID && position.commodity_id === s.commodityID)?.quantity_value ?? '0';
  };

  await page.goto(`/app/ledger/transactions?transaction_id=${out.transaction.id}`);
  await page.getByRole('button', { name: 'Correct transfer…' }).click();
  const form = page.getByRole('dialog', { name: 'Correct this transfer out' });
  const lot = form.getByLabel(/^Lot #/);
  await expect(lot).toHaveValue('2');
  await expect(form.getByLabel('Destination holding account')).toHaveCount(0);
  await form.getByLabel('Reason for correction').fill('only one share left');
  await lot.fill('1');
  await form.getByRole('button', { name: 'Review and replace' }).click();
  await expect(form).toBeHidden();
  await expect.poll(held).toBe('2');
  // The form sends no evidence, so the recorded tokens survive exactly.
  const chainText = await apiExactText(page, 'GET', `/api/v1/investments/transactions/${out.transaction.id}/correction-chain`);
  expect(chainText).toContain('"revision":9007199254740993');
  expect(chainText).toContain('"reference":"broker letter"');

  const chain = JSON.parse(chainText) as { effective_transaction_id: number };
  await page.goto(`/app/ledger/transactions?transaction_id=${chain.effective_transaction_id}`);
  await page.getByRole('button', { name: 'Reverse transfer…' }).click();
  const dialog = page.getByRole('alertdialog');
  await dialog.getByLabel('Reason for reversal').fill('the transfer was cancelled');
  await dialog.getByRole('button', { name: 'Review and reverse' }).click();
  await expect(dialog).toBeHidden();
  await expect.poll(held).toBe('3');
});
