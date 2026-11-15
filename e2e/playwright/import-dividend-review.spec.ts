import { expect, test, type Page } from './support/fixtures';
import process from 'node:process';
import { apiJSON } from './support/api';
import { readyForLedger, createCashAccount } from './support/ledger';
import { todayISO } from './support/dates';
import { expectNoAccessibilityViolations } from './support/a11y';
import { requireTrading212Stub, setTrading212Dividends } from './support/trading212';
import { linkLegacyCashDividend } from './support/legacy-dividend';

requireTrading212Stub();
test.use({ viewport: { width: 390, height: 844 } });

async function dividendSource(page: Page, payments = 1, fetch = true) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const ticker = `DIV${suffix}`;
  const cash = await createCashAccount(page, csrfToken, `Dividend cash ${suffix}`, currencyID);
  const category = (kind: 'income' | 'expense') => apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Dividend ${kind} ${suffix}`, account_class: kind, account_kind: kind,
    default_commodity_id: currencyID, allows_postings: true, opened_on: todayISO(), effective_from: todayISO()
  });
  const income = await category('income');
  const tax = await category('expense');
  const instrument = await apiJSON<{ commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: ticker, instrument_type: 'stock', display_name: `Dividend payer ${suffix}`, symbol: ticker,
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 0, price_scale: 2, effective_from: todayISO()
  });
  const apiKey = `dividend-${suffix}`;
  await setTrading212Dividends(apiKey, { ticker, isin: '', currency: 'USD' },
    Array.from({ length: payments }, (_, i) => ({ reference: `dividend-${suffix}-${i}`, amount: 85 + i, date: todayISO() })));
  const connectionName = `Dividend broker ${suffix}`;
  await apiJSON(page, 'POST', '/api/v1/import-connections', csrfToken, {
    source: 'trading212', display_name: connectionName, api_key: apiKey, cash_account_id: cash.id
  });
  const fetchRows = async (action: 'Import' | 'Re-fetch full history') => {
    await page.goto('/app/ledger/import?view=connections');
    await page.getByRole('row').filter({ hasText: connectionName }).getByRole('button', { name: action, exact: true }).click();
    await expect(page.getByRole('button', { name: 'Commit to ledger' })).toBeVisible({ timeout: 15_000 });
  };
  const activity = () => apiJSON<{ transactions: Array<{ id: number }> }>(page, 'GET', `/api/v1/transactions?commodity_id=${instrument.commodity_id}`);
  if (fetch) await fetchRows('Import');
  return { csrfToken, currencyID, cash, income, tax, instrument, connectionName, fetchRows, activity };
}

test('[acceptance] imported dividend waits for owner facts and records its known payer without defaults on a phone', async ({ page }) => {
  const f = await dividendSource(page);
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('1 dividend payments remain in review.')).toBeVisible();
  expect((await f.activity()).transactions).toHaveLength(0);
  await page.getByRole('link', { name: 'Review dividends and repair attribution', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Pending dividend', exact: true })).toBeVisible();
  await expect(page.getByLabel('Dividend amount', { exact: true })).toHaveValue('');
  await expect(page.getByLabel('Withholding amount', { exact: true })).toHaveValue('');
  await expect(page.getByRole('combobox', { name: 'Instrument', exact: true })).toHaveValue(String(f.instrument.commodity_id));
  await page.getByRole('combobox', { name: 'Income account', exact: true }).selectOption(String(f.income.id));
  await page.getByRole('combobox', { name: 'Withholding account', exact: true }).selectOption(String(f.tax.id));
  await page.getByLabel('Dividend amount', { exact: true }).fill('100.00');
  await page.getByLabel('Withholding amount', { exact: true }).fill('15.00');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await expectNoAccessibilityViolations(page, 'Imported dividend review on a phone');
  await page.getByRole('button', { name: 'Record reviewed dividend', exact: true }).click();
  await expect(page.getByText('Dividend recorded.', { exact: true })).toBeVisible();
  expect((await f.activity()).transactions).toHaveLength(1);
  await f.fetchRows('Re-fetch full history');
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('0 transactions committed')).toBeVisible();
  expect((await f.activity()).transactions).toHaveLength(1);
});

test('a dividend recorded in the Dividends view refreshes the Files batch and the Dividends count (#567 review)', async ({ page }) => {
  const f = await dividendSource(page);
  const batch = new URL(page.url()).searchParams.get('batch_id')!;
  const pendingDividends = async () => (await apiJSON<{ sources: Array<{ id: string; dividend_review_count?: number }> }>(page, 'GET', '/api/v1/attention'))
    .sources.find(source => source.id === 'imports')?.dividend_review_count ?? 0;
  const label = (count: number) => (count ? `Dividends (${count})` : 'Dividends');
  const before = await pendingDividends();
  expect(before).toBeGreaterThan(0);
  await expect(page.getByRole('button', { name: label(before), exact: true })).toBeVisible();
  await page.getByRole('button', { name: label(before), exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Pending dividend', exact: true })).toBeVisible();
  await page.getByRole('combobox', { name: 'Income account', exact: true }).selectOption(String(f.income.id));
  await page.getByRole('combobox', { name: 'Withholding account', exact: true }).selectOption(String(f.tax.id));
  await page.getByLabel('Dividend amount', { exact: true }).fill('100.00');
  await page.getByLabel('Withholding amount', { exact: true }).fill('15.00');
  await page.getByRole('button', { name: 'Record reviewed dividend', exact: true }).click();
  await expect(page.getByText('Dividend recorded.', { exact: true })).toBeVisible();
  // The count follows the write at once, not at the next attention poll.
  const after = await pendingDividends();
  expect(after).toBe(before - 1);
  await expect(page.getByRole('button', { name: label(after), exact: true })).toBeVisible();
  // Its only row is now recorded, so the batch is committed: Files re-reads
  // it and offers no commit for the stale working copy. While that read is
  // in flight, the stale copy cannot be committed.
  let release!: () => void;
  let started!: () => void;
  const held = new Promise<void>(resolve => { release = resolve; });
  const requested = new Promise<void>(resolve => { started = resolve; });
  await page.route(`**/api/v1/imports/${batch}?*`, async route => { started(); await held; await route.continue(); });
  await page.getByRole('button', { name: 'Files', exact: true }).click();
  await requested;
  try {
    await expect(page.getByRole('button', { name: 'Commit to ledger' })).toBeDisabled();
    // The reconciliation override saves the working copy, so it waits too.
    await expect(page.getByRole('checkbox', { name: 'Override reconciliation guards', exact: true })).toBeDisabled();
  } finally { release(); }
  expect(new URL(page.url()).searchParams.get('batch_id')).toBe(batch);
  await expect(page.getByRole('button', { name: 'Commit to ledger' })).toHaveCount(0);
  expect((await f.activity()).transactions).toHaveLength(1);
});

test('a failed Files re-read keeps the working copy, holds writes and retries the re-read (#567 review)', async ({ page }) => {
  const f = await dividendSource(page, 2);
  const batch = new URL(page.url()).searchParams.get('batch_id')!;
  const commit = page.getByRole('button', { name: 'Commit to ledger' });
  const override = page.getByRole('checkbox', { name: 'Override reconciliation guards', exact: true });
  await expect(commit).toBeEnabled();
  await page.getByRole('button', { name: /^Dividends/ }).click();
  const first = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Pending dividend', exact: true }) }).first();
  await first.getByRole('combobox', { name: 'Income account', exact: true }).selectOption(String(f.income.id));
  await first.getByRole('combobox', { name: 'Withholding account', exact: true }).selectOption(String(f.tax.id));
  await first.getByLabel('Dividend amount', { exact: true }).fill('100.00');
  await first.getByLabel('Withholding amount', { exact: true }).fill('15.00');
  await first.getByRole('button', { name: 'Record reviewed dividend', exact: true }).click();
  await expect(page.getByText('Dividend recorded.', { exact: true })).toBeVisible();

  // The re-read fails: the working copy stays, with no restore from scratch,
  // and every batch write waits for a re-read that lands.
  let failures = 0;
  const reads: string[] = [];
  await page.route(`**/api/v1/imports/${batch}?*`, async route => {
    reads.push(route.request().url());
    if (failures++ === 0) await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: { code: 'SERVICE_UNAVAILABLE', message: 'Try again' } }) });
    else await route.continue();
  });
  await page.getByRole('button', { name: 'Files', exact: true }).click();
  const retry = page.getByRole('button', { name: 'Retry', exact: true });
  await expect(retry).toBeVisible();
  await expect(page.getByRole('button', { name: 'Import another file', exact: true })).toBeEnabled();
  await expect(commit).toBeVisible();
  await expect(commit).toBeDisabled();
  await expect(override).toBeDisabled();
  expect(reads).toHaveLength(1);
  // The hold reaches the Dividends view: its save would write the same batch.
  await page.getByRole('button', { name: /^Dividends/ }).click();
  await expect(page.getByRole('button', { name: 'Record reviewed dividend', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: 'Files', exact: true }).click();
  await expect(retry).toBeVisible();

  // Retry re-reads the batch; one payment is still pending, so commit returns.
  await retry.click();
  await expect(retry).toHaveCount(0);
  await expect(commit).toBeEnabled();
  await expect(override).toBeEnabled();
  expect(reads).toHaveLength(2);
  expect(new URL(page.url()).searchParams.get('batch_id')).toBe(batch);
  await expectNoAccessibilityViolations(page, 'Files after a retried re-read');
});

async function recordFirstDividend(page: Page, f: Awaited<ReturnType<typeof dividendSource>>) {
  const form = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Pending dividend', exact: true }) }).first();
  await form.getByRole('combobox', { name: 'Income account', exact: true }).selectOption(String(f.income.id));
  await form.getByRole('combobox', { name: 'Withholding account', exact: true }).selectOption(String(f.tax.id));
  await form.getByLabel('Dividend amount', { exact: true }).fill('100.00');
  await form.getByLabel('Withholding amount', { exact: true }).fill('15.00');
  await form.getByRole('button', { name: 'Record reviewed dividend', exact: true }).click();
}

test('a Dividends save in flight keeps holding Files writes after a view change, until it lands (#567 review)', async ({ page }) => {
  const f = await dividendSource(page, 2);
  const batch = new URL(page.url()).searchParams.get('batch_id')!;
  const commit = page.getByRole('button', { name: 'Commit to ledger' });
  const override = page.getByRole('checkbox', { name: 'Override reconciliation guards', exact: true });
  await expect(commit).toBeEnabled();
  let release!: () => void;
  let started!: () => void;
  const held = new Promise<void>(resolve => { release = resolve; });
  const requested = new Promise<void>(resolve => { started = resolve; });
  await page.route(`**/api/v1/imports/${batch}/rows/*/review-dividend`, async route => { started(); await held; await route.continue(); });
  await page.getByRole('button', { name: /^Dividends/ }).click();
  await recordFirstDividend(page, f);
  await requested;
  // The hidden Dividends view pauses its reads while Files shows.
  const queueReads: string[] = [];
  page.on('request', request => { if (new URL(request.url()).pathname === '/api/v1/import-dividends') queueReads.push(request.url()); });
  try {
    // Leaving Dividends does not drop the save: Files still waits for it.
    await page.getByRole('button', { name: 'Files', exact: true }).click();
    await expect(commit).toBeVisible();
    await expect(commit).toBeDisabled();
    await expect(override).toBeDisabled();
  } finally { release(); }
  // The save lands, Files re-reads the batch, and the remaining payment can
  // be committed again.
  await expect(commit).toBeEnabled();
  await expect(override).toBeEnabled();
  expect((await f.activity()).transactions).toHaveLength(1);
  expect(queueReads).toHaveLength(0);
  // Back on Dividends, the queue invalidated by the save is read again.
  await page.getByRole('button', { name: /^Dividends/ }).click();
  await expect(page.getByText('Dividend recorded.', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Record reviewed dividend', exact: true })).toBeEnabled();
  expect(queueReads.length).toBeGreaterThan(0);
});

test('Import another file leaves a batch whose re-read keeps failing (#567 review)', async ({ page }) => {
  const f = await dividendSource(page, 2);
  const batch = new URL(page.url()).searchParams.get('batch_id')!;
  await page.getByRole('button', { name: /^Dividends/ }).click();
  await recordFirstDividend(page, f);
  await expect(page.getByText('Dividend recorded.', { exact: true })).toBeVisible();
  await page.route(`**/api/v1/imports/${batch}?*`, route => route.fulfill({
    status: 503, contentType: 'application/json', body: JSON.stringify({ error: { code: 'SERVICE_UNAVAILABLE', message: 'Try again' } })
  }));
  await page.getByRole('button', { name: 'Files', exact: true }).click();
  const retry = page.getByRole('button', { name: 'Retry', exact: true });
  await expect(retry).toBeVisible();
  await retry.click();
  await expect(retry).toBeVisible();
  await page.getByRole('button', { name: 'Import another file', exact: true }).click();
  // The batch stays as it is on the server; Files starts a new import with
  // nothing held.
  await expect(page.getByText('Upload a file', { exact: true })).toBeVisible();
  await expect(retry).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Commit to ledger' })).toHaveCount(0);
  expect(new URL(page.url()).searchParams.get('batch_id')).toBeNull();
  await page.unroute(`**/api/v1/imports/${batch}?*`);
  const kept = await apiJSON<{ rows: unknown[] }>(page, 'GET', `/api/v1/imports/${batch}`);
  expect(kept.rows).toHaveLength(2);
  await page.getByRole('button', { name: /^Dividends/ }).click();
  await expect(page.getByRole('button', { name: 'Record reviewed dividend', exact: true }).first()).toBeEnabled();
  // What was recorded for the batch left behind is not shown for the queue.
  await expect(page.getByText('Dividend recorded.', { exact: true })).toHaveCount(0);
});

test('a Dividends save in flight holds the result view\'s Import another file (#567 review)', async ({ page }) => {
  const f = await dividendSource(page, 2);
  const batch = new URL(page.url()).searchParams.get('batch_id')!;
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('2 dividend payments remain in review.')).toBeVisible();
  const another = page.getByRole('button', { name: 'Import another file', exact: true });
  await expect(another).toBeEnabled();
  let release!: () => void;
  let started!: () => void;
  const held = new Promise<void>(resolve => { release = resolve; });
  const requested = new Promise<void>(resolve => { started = resolve; });
  await page.route(`**/api/v1/imports/${batch}/rows/*/review-dividend`, async route => { started(); await held; await route.continue(); });
  await page.getByRole('button', { name: /^Dividends/ }).click();
  await recordFirstDividend(page, f);
  await requested;
  try {
    await page.getByRole('button', { name: 'Files', exact: true }).click();
    await expect(another).toBeVisible();
    await expect(another).toBeDisabled();
    // Even a click that reaches the reset leaves the batch in place.
    await another.evaluate(button => { (button as HTMLButtonElement).disabled = false; (button as HTMLButtonElement).click(); });
    expect(new URL(page.url()).searchParams.get('batch_id')).toBe(batch);
    await expect(page.getByText('Upload a file', { exact: true })).toHaveCount(0);
  } finally { release(); }
  await expect(another).toBeEnabled();
  expect(new URL(page.url()).searchParams.get('batch_id')).toBe(batch);
  expect((await f.activity()).transactions).toHaveLength(1);
});

test('Cancel while fetching waits for a Dividends save, and holds the batch until its discard finishes (#567 review)', async ({ page }) => {
  const f = await dividendSource(page, 2, false);
  // The Files view stays on its fetching step: its status reads never land.
  let releaseStatus!: () => void;
  const statusHeld = new Promise<void>(resolve => { releaseStatus = resolve; });
  // The fetch-status poll reads the bare batch path, with no query string.
  await page.route(url => /^\/api\/v1\/imports\/\d+$/.test(url.pathname), async route => { await statusHeld; await route.continue(); });
  await page.goto('/app/ledger/import?view=connections');
  await page.getByRole('row').filter({ hasText: f.connectionName }).getByRole('button', { name: 'Import', exact: true }).click();
  await expect(page.getByText('Fetching from Trading 212…', { exact: true })).toBeVisible();
  await expect.poll(() => new URL(page.url()).searchParams.get('batch_id')).not.toBeNull();
  const batch = new URL(page.url()).searchParams.get('batch_id')!;
  // The server finishes the fetch and stages the payments while Files still
  // shows it fetching (page.request bypasses the held route).
  await expect.poll(async () => {
    const response = await page.request.get(`/api/v1/imports/${batch}`);
    const body = await response.json() as { batch: { source_meta: string } };
    return (JSON.parse(body.batch.source_meta) as { fetch_status?: string }).fetch_status;
  }, { timeout: 15_000 }).toBe('ready');
  expect((await apiJSON<{ rows: unknown[] }>(page, 'GET', `/api/v1/import-dividends?batch_id=${batch}`)).rows).toHaveLength(2);
  const cancel = page.getByRole('button', { name: 'Cancel', exact: true });

  let releaseSave!: () => void;
  let saveStarted!: () => void;
  const saveHeld = new Promise<void>(resolve => { releaseSave = resolve; });
  const saveRequested = new Promise<void>(resolve => { saveStarted = resolve; });
  await page.route(`**/api/v1/imports/${batch}/rows/*/review-dividend`, async route => { saveStarted(); await saveHeld; await route.continue(); });
  await page.getByRole('button', { name: /^Dividends/ }).click();
  await recordFirstDividend(page, f);
  await saveRequested;
  try {
    await page.getByRole('button', { name: 'Files', exact: true }).click();
    await expect(cancel).toBeVisible();
    await expect(cancel).toBeDisabled();
    // Even a click that reaches the handler leaves the batch alone.
    await cancel.evaluate(button => { (button as HTMLButtonElement).disabled = false; (button as HTMLButtonElement).click(); });
    expect(new URL(page.url()).searchParams.get('batch_id')).toBe(batch);
    await expect(page.getByText('Upload a file', { exact: true })).toHaveCount(0);
  } finally { releaseSave(); }
  await expect(cancel).toBeEnabled();
  expect((await f.activity()).transactions).toHaveLength(1);

  // Cancel is itself a batch write: while its discard is in flight the other
  // payment cannot be saved, and the batch stays selected.
  let releaseDiscard!: () => void;
  let discardStarted!: () => void;
  const discardHeld = new Promise<void>(resolve => { releaseDiscard = resolve; });
  const discardRequested = new Promise<void>(resolve => { discardStarted = resolve; });
  await page.route(`**/api/v1/imports/${batch}/discard`, async route => { discardStarted(); await discardHeld; await route.continue(); });
  await cancel.click();
  await discardRequested;
  try {
    expect(new URL(page.url()).searchParams.get('batch_id')).toBe(batch);
    await page.getByRole('button', { name: /^Dividends/ }).click();
    await expect(page.getByRole('button', { name: 'Record reviewed dividend', exact: true })).toBeDisabled();
    await page.getByRole('button', { name: 'Files', exact: true }).click();
  } finally { releaseDiscard(); }
  await expect(page.getByText('Upload a file', { exact: true })).toBeVisible();
  await expect.poll(() => new URL(page.url()).searchParams.get('batch_id')).toBeNull();
  // A status read that lands after the discard does not bring the batch back.
  releaseStatus();
  await page.waitForLoadState('networkidle');
  await expect(page.getByText('Upload a file', { exact: true })).toBeVisible();
  expect(new URL(page.url()).searchParams.get('batch_id')).toBeNull();
});

test('Cancel after a failed fetch-status read clears its error from Online connections (#567 review)', async ({ page }) => {
  const f = await dividendSource(page, 1, false);
  await page.route(url => /^\/api\/v1\/imports\/\d+$/.test(url.pathname), route => route.fulfill({
    status: 503, contentType: 'application/json', body: JSON.stringify({ error: { code: 'SERVICE_UNAVAILABLE', message: 'Try again' } })
  }));
  await page.goto('/app/ledger/import?view=connections');
  await page.getByRole('row').filter({ hasText: f.connectionName }).getByRole('button', { name: 'Import', exact: true }).click();
  await expect.poll(() => new URL(page.url()).searchParams.get('batch_id')).not.toBeNull();
  const batch = new URL(page.url()).searchParams.get('batch_id')!;
  const fetchError = page.getByText('Could not start the fetch. Check the connection and try again.', { exact: true });
  // The failed read is reported on Online connections while the fetch is open.
  await page.getByRole('button', { name: 'Online connections', exact: true }).click();
  await expect(fetchError).toBeVisible();
  await page.getByRole('button', { name: 'Files', exact: true }).click();
  const discarded = page.waitForResponse(response => new URL(response.url()).pathname === `/api/v1/imports/${batch}/discard`);
  await page.getByRole('button', { name: 'Cancel', exact: true }).click();
  expect((await discarded).ok()).toBe(true);
  await expect(page.getByText('Upload a file', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Online connections', exact: true }).click();
  await expect(page.getByRole('row').filter({ hasText: f.connectionName })).toBeVisible();
  await expect(fetchError).toHaveCount(0);
});

test('Back to import returns to the staged batch in Files (#567 review)', async ({ page }) => {
  await dividendSource(page);
  const batch = new URL(page.url()).searchParams.get('batch_id')!;
  await page.getByRole('button', { name: /^Dividends/ }).click();
  await expect(page.getByRole('heading', { name: 'Pending dividend', exact: true })).toBeVisible();
  await page.getByRole('link', { name: 'Back to import', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Commit to ledger' })).toBeVisible();
  await expect(page.getByText('Upload a file', { exact: true })).toHaveCount(0);
  const url = new URL(page.url());
  expect(url.searchParams.get('batch_id')).toBe(batch);
  expect(url.searchParams.get('view')).toBe('files');
});

test('[acceptance] historical cash dividend repair keeps the original payment and owner note on a phone', async ({ page }) => {
  test.skip(Boolean(process.env.E2E_BASE_URL), 'historical source-link fixture needs the disposable local e2e database');
  const f = await dividendSource(page);
  const reviews = await apiJSON<{ rows: Array<{ batch_id: number; row_id: number; state: string; resolution: { account_id?: number } }> }>(page, 'GET', '/api/v1/import-dividends');
  const row = reviews.rows.find(r => r.state === 'pending' && r.resolution.account_id === f.cash.id)!;
  expect(row).toBeTruthy();
  const legacy = await apiJSON<{ id: number; version_id: number }>(page, 'POST', '/api/v1/transactions', f.csrfToken, {
    transaction_date: todayISO(), description: 'Historical cash dividend', note_markdown: 'Keep the owner note',
    journal_entries: [{ entry_date: todayISO(), entry_kind: 'ordinary', postings: [
      { account_id: f.cash.id, commodity_id: f.currencyID, quantity_value: '8500', quantity_scale: 2 },
      { account_id: f.income.id, commodity_id: f.currencyID, quantity_value: '-8500', quantity_scale: 2 }
    ] }]
  });
  linkLegacyCashDividend(row.row_id, legacy.id, f.cash.id);
  await page.goto(`/app/ledger/import?view=dividends&batch_id=${row.batch_id}`);
  await expect(page.getByRole('heading', { name: 'Repair previously imported cash payment', exact: true })).toBeVisible();
  await expect(page.getByRole('combobox', { name: 'Cash account', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: 'Declare source cash as gross with no withholding', exact: true }).click();
  await expect(page.getByLabel('Dividend amount', { exact: true })).toHaveValue('85');
  await expect(page.getByLabel('Withholding amount', { exact: true })).toHaveValue('0');
  await page.getByRole('combobox', { name: 'Income account', exact: true }).selectOption(String(f.income.id));
  await page.getByLabel('Reason for repair', { exact: true }).fill('Confirm payer from statement');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await expectNoAccessibilityViolations(page, 'Historical dividend repair on a phone');
  await page.getByRole('button', { name: 'Repair dividend attribution', exact: true }).click();
  await expect(page.getByText('Dividend attribution repaired; cash payment preserved.', { exact: true })).toBeVisible();
  expect((await f.activity()).transactions.map(t => t.id)).toEqual([legacy.id]);
  const repaired = await apiJSON<{ id: number; version_id: number; note_markdown: string }>(page, 'GET', `/api/v1/transactions/${legacy.id}`);
  expect(repaired.version_id).not.toBe(legacy.version_id);
  expect(repaired.note_markdown).toBe('Keep the owner note');
});

test('a dividend repair keeps its reviewed version when the list refreshes behind unsaved fields', async ({ page }) => {
  test.skip(Boolean(process.env.E2E_BASE_URL), 'historical source-link fixture needs the disposable local e2e database');
  const f = await dividendSource(page);
  const reviews = await apiJSON<{ rows: Array<{ batch_id: number; row_id: number; state: string; resolution: { account_id?: number } }> }>(page, 'GET', '/api/v1/import-dividends');
  const row = reviews.rows.find(r => r.state === 'pending' && r.resolution.account_id === f.cash.id)!;
  expect(row).toBeTruthy();
  const today = todayISO();
  const original = {
    transaction_date: today, description: 'Dividend repair refetch',
    journal_entries: [{ entry_date: today, entry_kind: 'ordinary', postings: [
      { account_id: f.cash.id, commodity_id: f.currencyID, quantity_value: '8500', quantity_scale: 2 },
      { account_id: f.income.id, commodity_id: f.currencyID, quantity_value: '-8500', quantity_scale: 2 }
    ] }]
  };
  const legacy = await apiJSON<{ id: number; version_id: number }>(page, 'POST', '/api/v1/transactions', f.csrfToken, original);
  linkLegacyCashDividend(row.row_id, legacy.id, f.cash.id);
  await page.clock.install();
  await page.goto(`/app/ledger/import?view=dividends&batch_id=${row.batch_id}`);
  const form = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Repair previously imported cash payment', exact: true }) });
  await form.getByRole('button', { name: 'Declare source cash as gross with no withholding', exact: true }).click();
  await form.getByRole('combobox', { name: 'Income account', exact: true }).selectOption(String(f.income.id));
  const reason = form.getByLabel('Reason for repair', { exact: true });
  await reason.fill('Keep my unsaved repair');

  // Another tab changes the classification while this form still holds the
  // original income account. A refetch must not bless those old fields with
  // the newer version token.
  const otherIncome = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', f.csrfToken, {
    name: `New dividend classification ${legacy.id}`, account_class: 'income', account_kind: 'income',
    default_commodity_id: f.currencyID, allows_postings: true, opened_on: today, effective_from: today
  });
  const newer = await apiJSON<{ version_id: number }>(page, 'PATCH', `/api/v1/transactions/${legacy.id}`, f.csrfToken, {
    ...original, expected_version_id: legacy.version_id,
    journal_entries: [{ ...original.journal_entries[0], postings: [original.journal_entries[0].postings[0],
      { ...original.journal_entries[0].postings[1], account_id: otherIncome.id }]
    }]
  });
  await page.clock.fastForward('00:45');
  const refetched = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/import-dividends' && response.ok());
  await page.context().setOffline(true);
  await page.context().setOffline(false);
  const refreshed = await (await refetched).json() as { rows: Array<{ row_id: number; expected_transaction_version_id: number }> };
  expect(refreshed.rows.find(r => r.row_id === row.row_id)?.expected_transaction_version_id).toBe(newer.version_id);
  await expect(reason).toHaveValue('Keep my unsaved repair');
  await expect(form.getByRole('combobox', { name: 'Income account', exact: true })).toHaveValue(String(f.income.id));

  const previewed = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname.endsWith('/reconciliation-impact'));
  await form.getByRole('button', { name: 'Repair dividend attribution', exact: true }).click();
  const response = await previewed;
  expect(response.request().postDataJSON().expected_transaction_version_id).toBe(legacy.version_id);
  expect(response.status()).toBe(409);
  await expect(form.getByText('This entry changed in another tab or window while you were editing it. Reload it and make your change again.', { exact: true })).toBeVisible();
  const after = await apiJSON<{ version_id: number; transaction_kind: string; journal_entries: Array<{ postings: Array<{ account_id: number }> }> }>(page, 'GET', `/api/v1/transactions/${legacy.id}`);
  expect(after.version_id).toBe(newer.version_id);
  expect(after.transaction_kind).toBe('ordinary');
  expect(after.journal_entries.flatMap(entry => entry.postings.map(posting => posting.account_id))).toContain(otherIncome.id);
});
