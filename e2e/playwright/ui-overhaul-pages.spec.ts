import { test, expect } from './support/owned-book';
import { createCashAccount, readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';
import { apiJSON } from './support/api';
import { seedBulkAccounts } from './support/bulk-accounts';
import { expectNoAccessibilityViolations } from './support/a11y';
import { requireTrading212Stub, setTrading212Fills } from './support/trading212';

for (const width of [320, 1280]) {
  test(`Import views keep a staged batch through navigation, reload and old dividend links at ${width}px (#447)`, async ({ page }) => {
    await readyForLedger(page);
    await page.setViewportSize({ width, height: 900 });
    await page.goto('/app/ledger/import');
    const [year, month, day] = todayISO().split('-');
    await page.locator('input[type="file"]').setInputFiles({ name: 'views.csv', mimeType: 'text/csv', buffer: Buffer.from(`Date;Payee;Amount\n${day}/${month}/${year};View merchant;-12,34\n`) });
    await page.getByLabel('Mapping name').fill(`Views ${width}`);
    await page.getByRole('button', { name: 'Upload & preview' }).click();
    await expect(page.getByRole('button', { name: 'Commit to ledger' })).toBeVisible();
    await expect.poll(() => new URL(page.url()).searchParams.get('batch_id')).not.toBeNull();
    const batch = new URL(page.url()).searchParams.get('batch_id')!;
    await page.getByRole('button', { name: 'Rules and profiles', exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`batch_id=${batch}.*view=rules|view=rules.*batch_id=${batch}`));
    await page.goBack();
    await expect(page.getByRole('button', { name: 'Commit to ledger' })).toBeVisible();
    await page.reload();
    await expect(page.getByRole('button', { name: 'Commit to ledger' })).toBeVisible();
    await expect(page.getByText('View merchant', { exact: true }).first()).toBeVisible();
    await page.goto(`/app/import/dividends?batch_id=${batch}#review`);
    await expect(page).toHaveURL(`/app/ledger/import?batch_id=${batch}&view=dividends#review`);
    await expect(page.getByRole('button', { name: /^Dividends/, pressed: true })).toBeVisible();
    await page.getByRole('button', { name: 'Files', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Commit to ledger' })).toBeVisible();
    await expectNoAccessibilityViolations(page);
    // A late batch response cannot undo a newer history selection.
    const importTab = page.getByRole('navigation', { name: 'Ledger pages', exact: true }).locator('a[href="/app/ledger/import"]');
    await importTab.click();
    let release!: () => void;
    let started!: () => void;
    const held = new Promise<void>(resolve => { release = resolve; });
    const requested = new Promise<void>(resolve => { started = resolve; });
    await page.route(`**/api/v1/imports/${batch}?*`, async route => { started(); await held; await route.continue(); });
    await page.getByRole('button', { name: /^views.csv/ }).click();
    await requested;
    await importTab.click();
    const finished = page.waitForResponse(response => new URL(response.url()).pathname === `/api/v1/imports/${batch}`);
    release();
    await finished;
    await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))));
    await expect(page.locator('input[type="file"]')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Commit to ledger' })).toHaveCount(0);

  });
}

test('Ledger groups a large catalog, hides holdings and manages institutions (#448)', async ({ page, ownedBookDatabasePath }) => {
  test.setTimeout(120_000);
  const { csrfToken } = await readyForLedger(page);
  seedBulkAccounts(ownedBookDatabasePath, 'Overview account', 120);
  await page.goto('/app/ledger');
  await expect(page.getByRole('link', { name: /Overview account 00119/ })).toBeVisible();
  await page.getByRole('button', { name: 'Institutions', exact: true }).click();
  await page.getByRole('button', { name: 'Add institution', exact: true }).first().click();
  await page.getByLabel('Name', { exact: true }).fill('Overview bank');
  await page.getByRole('button', { name: 'Create institution', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Overview bank', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Archive institution', exact: true }).click();
  await page.getByLabel('Show archived', { exact: true }).check();
  await page.getByRole('button', { name: 'Restore institution', exact: true }).click();
  const institutions = await apiJSON<{ institutions: Array<{ name: string; status: string }> }>(page, 'GET', '/api/v1/institutions');
  expect(institutions.institutions.find(i => i.name === 'Overview bank')?.status).toBe('active');
  await page.setViewportSize({ width: 320, height: 900 });
  await expectNoAccessibilityViolations(page);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test('brokerage → instrument → brokerage on a phone keeps holdings inside one account (#448)', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const today = todayISO();
  const broker = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: 'Overview brokerage', account_class: 'asset', account_kind: 'brokerage', allows_postings: false, opened_on: today
  });
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: 'Broker cash', account_class: 'asset', account_kind: 'brokerage_cash', allows_postings: true,
    parent_account_id: broker.id, default_commodity_id: currencyID, opened_on: today
  });
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: 'UIBROKER', display_name: 'Overview stock', instrument_type: 'stock', quantity_scale: 0, price_scale: 2, quote_commodity_id: currencyID, identifiers: {}, metadata: {}
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: 'Hidden overview holding', parent_account_id: broker.id, opened_on: today
  });
  await apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, {
    transaction_date: today, commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: cash.id, cash_commodity_id: currencyID, quantity_value: '2', quantity_scale: 0, cash_amount_value: '10000', cash_amount_scale: 2
  });
  await page.setViewportSize({ width: 320, height: 900 });
  await page.goto('/app/ledger');
  await expect(page.getByRole('link', { name: /Hidden overview holding/ })).toHaveCount(0);
  await expect(page.getByRole('link', { name: /Broker cash/ })).toHaveCount(0);
  await page.getByRole('link', { name: /Overview brokerage/ }).click();
  await expect(page.getByRole('heading', { name: 'Overview brokerage', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Positions', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Broker cash', exact: true })).toBeVisible();
  await page.getByRole('link', { name: 'UIBROKER', exact: true }).click();
  await expect(page.getByRole('heading', { name: /Overview stock/ })).toBeVisible();
  await page.getByRole('link', { name: 'Overview brokerage', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Positions', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  await expect(page.getByRole('tab', { name: 'Investment', exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test('holding registers stay reachable and hidden children still guard archive and delete (#567 review)', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const today = todayISO();
  const suffix = Date.now();
  const broker = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Guarded brokerage ${suffix}`, account_class: 'asset', account_kind: 'brokerage', allows_postings: false, opened_on: today
  });
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Guarded cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash', allows_postings: true,
    parent_account_id: broker.id, default_commodity_id: currencyID, opened_on: today
  });
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: `GUARD${suffix}`, display_name: `Guarded stock ${suffix}`, instrument_type: 'stock', quantity_scale: 0, price_scale: 2, quote_commodity_id: currencyID, identifiers: {}, metadata: {}
  });
  const holdingName = `Guarded holding ${suffix}`;
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: holdingName, parent_account_id: broker.id, opened_on: today
  });
  // A holding with no parent at all is still listed with its value.
  const orphanName = `Unparented holding ${suffix}`;
  const orphan = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: orphanName, opened_on: today
  });
  for (const target of [holding.id, orphan.id]) {
    await apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, {
      transaction_date: today, commodity_id: instrument.commodity_id, holding_account_id: target,
      cash_account_id: cash.id, cash_commodity_id: currencyID, quantity_value: '1', quantity_scale: 0, cash_amount_value: '1000', cash_amount_scale: 2
    });
  }
  await page.setViewportSize({ width: 390, height: 900 });
  await page.goto('/app/ledger');
  // Its cash child and holding are folded into the brokerage row, but they
  // are still children: the server would refuse to delete or close it.
  await expect(page.getByRole('button', { name: `Delete Guarded brokerage ${suffix}`, exact: true })).toBeDisabled();
  await expect(page.getByRole('button', { name: `Close Guarded brokerage ${suffix}`, exact: true })).toBeDisabled();
  await expect(page.getByRole('heading', { name: 'Holdings outside a brokerage', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: orphanName, exact: true })).toHaveAttribute('href', `/app/ledger/accounts/${orphan.id}/register`);

  // Brokerage → holding register → instrument → holding register.
  await page.getByRole('link', { name: new RegExp(`Guarded brokerage ${suffix}`) }).click();
  await page.getByRole('link', { name: holdingName, exact: true }).click();
  await expect(page).toHaveURL(`/app/ledger/accounts/${holding.id}/register`);
  await expect(page.getByRole('heading', { level: 1, name: holdingName })).toBeVisible();
  await expect(page.getByRole('navigation', { name: 'Breadcrumb' }).getByRole('link', { name: `Guarded brokerage ${suffix}`, exact: true }))
    .toHaveAttribute('href', `/app/ledger/accounts/${broker.id}/register`);
  await page.getByRole('link', { name: 'Instrument page', exact: true }).click();
  await expect(page.getByRole('heading', { name: new RegExp(`Guarded stock ${suffix}`) })).toBeVisible();
  const holdings = page.getByRole('list', { name: 'Holdings', exact: true });
  await expect(holdings.getByRole('link', { name: orphanName, exact: true })).toBeVisible();
  await holdings.getByRole('link', { name: holdingName, exact: true }).click();
  await expect(page).toHaveURL(`/app/ledger/accounts/${holding.id}/register`);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test('an ordinary register never asks for the book-wide Ledger read (#567 review)', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const account = await createCashAccount(page, csrfToken, `Light register ${Date.now()}`, currencyID);
  const ledgerReads: string[] = [];
  page.on('request', request => {
    const path = new URL(request.url()).pathname;
    if (path.startsWith('/api/v1/ledger/accounts/') || path === '/api/v1/ledger/overview') ledgerReads.push(path);
  });
  await page.goto(`/app/ledger/accounts/${account.id}/register`);
  await expect(page.getByRole('heading', { level: 1, name: /Light register/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Positions', exact: true })).toHaveCount(0);
  expect(ledgerReads).toEqual([]);
});


test('an existing non-money account register still names its account (#448)', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const income = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: 'Legacy register income', account_class: 'income', account_kind: 'income',
    default_commodity_id: currencyID, allows_postings: true, opened_on: todayISO()
  });
  await page.goto(`/app/ledger/accounts/${income.id}/register`);
  await expect(page.getByRole('heading', { name: 'Legacy register income', exact: true })).toBeVisible();
  await expect(page.getByRole('alert')).toHaveCount(0);
});


test('a commit retains its batch while views change and another import is blocked (#447)', async ({ page }) => {
  requireTrading212Stub();
  const { csrfToken, currencyID } = await readyForLedger(page);
  const account = await createCashAccount(page, csrfToken, 'Busy batch cash', currencyID);
  await setTrading212Fills('busy-batch-key', { ticker: 'BUSY', isin: '', currency: 'EUR' }, []);
  await apiJSON(page, 'POST', '/api/v1/import-connections', csrfToken, {
    source: 'trading212', display_name: 'Busy batch connection', api_key: 'busy-batch-key', cash_account_id: account.id
  });
  await page.goto('/app/ledger/import');
  const [year, month, day] = todayISO().split('-');
  await page.locator('input[type="file"]').setInputFiles({ name: 'busy.csv', mimeType: 'text/csv', buffer: Buffer.from(`Date;Payee;Amount\n${day}/${month}/${year};;-1,00\n`) });
  await page.getByLabel('Mapping name').fill('Busy batch mapping');
  await page.getByRole('button', { name: 'Upload & preview' }).click();
  await expect(page.getByRole('button', { name: 'Commit to ledger' })).toBeVisible();
  await page.getByLabel('Target account', { exact: true }).selectOption(String(account.id));
  await page.getByLabel('Currency', { exact: true }).first().selectOption(String(currencyID));
  await page.getByLabel('Default category', { exact: true }).selectOption({ index: 1 });
  await page.getByRole('button', { name: 'Apply to all rows' }).click();
  const batch = new URL(page.url()).searchParams.get('batch_id')!;
  let release!: () => void;
  let started!: () => void;
  const held = new Promise<void>(resolve => { release = resolve; });
  const requested = new Promise<void>(resolve => { started = resolve; });
  await page.route(`**/api/v1/imports/${batch}/preview-commit`, async route => { started(); await held; await route.continue(); });
  const committed: string[] = [];
  page.on('request', request => {
    const match = new URL(request.url()).pathname.match(/^\/api\/v1\/imports\/(\d+)\/commit$/);
    if (request.method() === 'POST' && match) committed.push(match[1]);
  });
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await requested;
  try {
    await page.getByRole('navigation', { name: 'Ledger pages', exact: true }).locator('a[href="/app/ledger/import"]').click();
    expect(new URL(page.url()).searchParams.get('batch_id')).toBe(batch);
    await page.getByRole('button', { name: 'Online connections', exact: true }).click();
    await expect(page.getByRole('row').filter({ hasText: 'Busy batch connection' }).getByRole('button', { name: 'Import', exact: true })).toBeDisabled();
    expect(new URL(page.url()).searchParams.get('batch_id')).toBe(batch);
  } finally { release(); }
  await page.getByRole('button', { name: 'Files', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Import another file' })).toBeVisible();
  expect(committed).toEqual([batch]);
});
