import { expect, test } from './support/fixtures';
import { apiJSON } from './support/api';
import { expectNoAccessibilityViolations } from './support/a11y';
import { createCashAccount, ensureCurrency, readyForLedger } from './support/ledger';
import { daysFromTodayISO } from './support/dates';

/**
 * #595: a nearly dormant savings account is reconciled against the bank every
 * month with no movement. Each statement shows in the register as its own
 * row — date, balance, "Reconciled" — between the postings at its boundary,
 * and none of them is a transaction. #596: the empty statements finish, the
 * last one through the Reconcile screen.
 */

type Posting = { id: number; account_id: number };
type Transaction = { journal_entries: Array<{ postings: Posting[] }> };

test('a dormant account shows each reconciled statement in its register', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const openedOn = daysFromTodayISO(-130);
  const depositOn = daysFromTodayISO(-120);
  const statementDates = [daysFromTodayISO(-100), daysFromTodayISO(-70), daysFromTodayISO(-40), daysFromTodayISO(-10)];
  const savings = await createCashAccount(page, csrfToken, `Dormant savings ${suffix}`, currencyID, openedOn);
  const income = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: `Dormant gift ${suffix}`, category_type: 'income'
  });
  const deposit = await apiJSON<Transaction>(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_date: depositOn, status: 'posted', description: `Opening deposit ${suffix}`,
    journal_entries: [{ entry_date: depositOn, postings: [
      { account_id: savings.id, commodity_id: currencyID, quantity_value: '123456', quantity_scale: 2 },
      { account_id: income.id, commodity_id: currencyID, quantity_value: '-123456', quantity_scale: 2 }
    ] }]
  });
  const depositPosting = deposit.journal_entries[0].postings.find((p) => p.account_id === savings.id)!;

  // The first statement verifies the deposit; the next two verify nothing new.
  await apiJSON(page, 'POST', `/api/v1/accounts/${savings.id}/postings/reconciliation-status`, csrfToken, {
    posting_version_ids: [depositPosting.id], status: 'cleared', cleared_on: statementDates[0]
  });
  for (const statementDate of statementDates.slice(0, 3)) {
    const session = await apiJSON<{ id: number }>(page, 'POST', `/api/v1/accounts/${savings.id}/reconciliations/start`, csrfToken, {
      commodity_id: currencyID, source_kind: 'statement', statement_date: statementDate,
      statement_balance_value: '123456', statement_balance_scale: 2
    });
    await apiJSON(page, 'POST', `/api/v1/reconciliations/${session.id}/finish`, csrfToken, { change_reason: 'statement verified' });
  }

  // The fourth, empty too, is finished on the Reconcile screen.
  await page.goto(`/app/ledger/reconcile?account=${savings.id}`);
  await page.getByLabel('Statement closing date').fill(statementDates[3]);
  await page.getByLabel('Statement ending balance').fill('1234.56');
  await page.getByRole('button', { name: 'Start reconciling' }).click();
  await expect(page.getByText('Balanced')).toBeVisible();
  await page.getByLabel('Reason', { exact: true }).fill('statement verified');
  await page.getByRole('button', { name: 'Finish reconciliation' }).click();
  await expect(page.getByText('Reconciliation complete')).toBeVisible();
  await expect.poll(async () => (await apiJSON<{ checkpoints: Array<{ status: string }> }>(
    page, 'GET', `/api/v1/accounts/${savings.id}/reconciliation-checkpoints`
  )).checkpoints.filter((c) => c.status === 'active').length).toBe(4);

  for (const colorScheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme });
    await page.goto(`/app/ledger/accounts/${savings.id}/register`);
    const table = page.locator('[data-transaction-table]');
    const statements = table.locator('[data-register-statement]');
    await expect(statements).toHaveCount(4);
    // Statements were finished in date order, so newest first is descending ids.
    const ids = (await statements.evaluateAll((rows) => rows.map((row) => Number(row.getAttribute('data-register-statement')))));
    expect(ids).toEqual([...ids].sort((a, b) => b - a));
    // Newest first, each with its balance and the word, never colour alone.
    for (const index of statementDates.keys()) {
      const row = statements.nth(index);
      await expect(row).toContainText('Statement');
      await expect(row).toContainText('1,234.56');
      await expect(row.getByText('Reconciled', { exact: true })).toBeVisible();
      await expect(row.getByRole('link', { name: 'Open in Reconcile' })).toHaveAttribute(
        'href', `/app/ledger/reconcile?account=${savings.id}&commodity=${currencyID}`
      );
    }
    // One transaction row, below every statement; statements carry no row actions.
    const dataRows = table.locator('[data-table-row]');
    await expect(dataRows).toHaveCount(1);
    await expect(dataRows.first()).toContainText(`Opening deposit ${suffix}`);
    const order = await table.locator('tbody tr').evaluateAll((rows) =>
      rows.map((row) => (row.hasAttribute('data-table-marker') ? 'statement' : 'posting'))
    );
    expect(order).toEqual(['statement', 'statement', 'statement', 'statement', 'posting']);
    await expect(table.locator('[data-table-marker]').getByRole('button')).toHaveCount(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
    await expectNoAccessibilityViolations(page, `${colorScheme} register with statement rows`);
  }

  // The statement rows reflow at 320px without a horizontal page scroll.
  await page.setViewportSize({ width: 320, height: 720 });
  await page.goto(`/app/ledger/accounts/${savings.id}/register`);
  await expect(page.locator('[data-register-statement]')).toHaveCount(4);
  await expect(page.locator('[data-register-statement]').first().getByText('Reconciled', { exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);

  // The link opens Reconcile for the account.
  await page.locator('[data-register-statement]').first().getByRole('link', { name: 'Open in Reconcile' }).click();
  await expect(page).toHaveURL(new RegExp(`/app/ledger/reconcile\\?account=${savings.id}&commodity=${currencyID}$`));
});

/**
 * #595 review. A statement in another currency than the account's default
 * links to Reconcile for that currency, where it can be voided; returning to
 * the register then shows no stale "Reconciled" row.
 */
test('a statement row opens Reconcile in its own currency, and a void there clears it from the register', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const chf = await ensureCurrency(page, csrfToken, 'CHF', 'Swiss franc');
  expect(chf).not.toBe(currencyID);
  const account = await createCashAccount(page, csrfToken, `Franc pocket ${suffix}`, currencyID, daysFromTodayISO(-60));
  const statementDate = daysFromTodayISO(-5);
  const session = await apiJSON<{ id: number }>(page, 'POST', `/api/v1/accounts/${account.id}/reconciliations/start`, csrfToken, {
    commodity_id: chf, source_kind: 'statement', statement_date: statementDate,
    statement_balance_value: '0', statement_balance_scale: 2
  });
  await apiJSON(page, 'POST', `/api/v1/reconciliations/${session.id}/finish`, csrfToken, { change_reason: 'statement verified' });

  await page.goto(`/app/ledger/accounts/${account.id}/register`);
  const row = page.locator('[data-register-statement]');
  await expect(row).toHaveCount(1);
  await expect(row).toContainText('CHF');
  await row.getByRole('link', { name: 'Open in Reconcile' }).click();
  await expect(page).toHaveURL(new RegExp(`/app/ledger/reconcile\\?account=${account.id}&commodity=${chf}$`));

  const voidButton = page.getByRole('button', { name: `Void the statement of ${statementDate}` });
  await expect(voidButton).toBeVisible();
  await voidButton.click();
  const dialog = page.getByRole('alertdialog', { name: 'Void this statement?' });
  await dialog.getByLabel('Reason for voiding').fill(`Wrong currency ${suffix}`);
  await dialog.getByRole('button', { name: 'Void statement' }).click();
  await expect(dialog).toBeHidden();

  // Hold the register's refetch open: going back must not show the voided
  // statement from cache while it is pending.
  let release: () => void = () => {};
  const held = new Promise<void>((resolve) => { release = resolve; });
  let refetches = 0;
  await page.route(`**/api/v1/accounts/${account.id}/register**`, async (route) => {
    refetches++;
    await held;
    await route.continue();
  });
  await page.goBack();
  await expect(page).toHaveURL(new RegExp(`/app/ledger/accounts/${account.id}/register`));
  await expect.poll(() => refetches).toBeGreaterThan(0);
  await expect(page.getByText('No transactions', { exact: true })).toBeVisible();
  await expect(page.locator('[data-register-statement]')).toHaveCount(0);
  release();
  await expect(page.getByText('No transactions', { exact: true })).toBeVisible();
});

/**
 * #594 review. A statement dated after today is left out of the
 * reconciliation age, but its register row still links to Reconcile in its
 * own currency, where it can be seen and voided.
 */
test('a future-dated statement in another currency opens Reconcile in that currency', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const chf = await ensureCurrency(page, csrfToken, 'CHF', 'Swiss franc');
  expect(chf).not.toBe(currencyID);
  const account = await createCashAccount(page, csrfToken, `Future franc ${suffix}`, currencyID, daysFromTodayISO(-60));
  const statementDate = daysFromTodayISO(10);
  const session = await apiJSON<{ id: number }>(page, 'POST', `/api/v1/accounts/${account.id}/reconciliations/start`, csrfToken, {
    commodity_id: chf, source_kind: 'statement', statement_date: statementDate,
    statement_balance_value: '0', statement_balance_scale: 2
  });
  await apiJSON(page, 'POST', `/api/v1/reconciliations/${session.id}/finish`, csrfToken, { change_reason: 'statement verified' });

  await page.goto(`/app/ledger/accounts/${account.id}/register`);
  const row = page.locator('[data-register-statement]');
  await expect(row).toHaveCount(1);
  await row.getByRole('link', { name: 'Open in Reconcile' }).click();
  await expect(page).toHaveURL(new RegExp(`/app/ledger/reconcile\\?account=${account.id}&commodity=${chf}$`));

  await expect(page.getByLabel('Currency')).toHaveValue(String(chf));
  const voidButton = page.getByRole('button', { name: `Void the statement of ${statementDate}` });
  await expect(voidButton).toBeVisible();
  await voidButton.click();
  const dialog = page.getByRole('alertdialog', { name: 'Void this statement?' });
  await dialog.getByLabel('Reason for voiding').fill(`Future statement ${suffix}`);
  await dialog.getByRole('button', { name: 'Void statement' }).click();
  await expect(dialog).toBeHidden();
  await expect(voidButton).toBeHidden();
});

/**
 * #594 review. A holding is reconciled in its instrument, but a statement in
 * another commodity (a cash statement entered through the API) still shows in
 * its register, and its link opens Reconcile in that commodity.
 */
test('a holding statement in a currency opens Reconcile in that currency', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const opened = daysFromTodayISO(-60);
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: `RH${suffix}`, instrument_type: 'stock', display_name: `Register shares ${suffix}`,
    quantity_scale: 0, price_scale: 2, quote_commodity_id: currencyID, effective_from: opened
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Register holding ${suffix}`, opened_on: opened, effective_from: opened
  });
  const statementDate = daysFromTodayISO(-5);
  const session = await apiJSON<{ id: number }>(page, 'POST', `/api/v1/accounts/${holding.id}/reconciliations/start`, csrfToken, {
    commodity_id: currencyID, source_kind: 'statement', statement_date: statementDate,
    statement_balance_value: '0', statement_balance_scale: 2
  });
  await apiJSON(page, 'POST', `/api/v1/reconciliations/${session.id}/finish`, csrfToken, { change_reason: 'statement verified' });

  await page.goto(`/app/ledger/accounts/${holding.id}/register`);
  const row = page.locator('[data-register-statement]');
  await expect(row).toHaveCount(1);
  await row.getByRole('link', { name: 'Open in Reconcile' }).click();
  await expect(page).toHaveURL(new RegExp(`/app/ledger/reconcile\\?account=${holding.id}&commodity=${currencyID}$`));

  const currency = page.getByLabel('Currency');
  await expect(currency).toHaveValue(String(currencyID));
  await expect(currency.locator('option')).toHaveText([`RH${suffix}`, /\S/]);
  const voidButton = page.getByRole('button', { name: `Void the statement of ${statementDate}` });
  await expect(voidButton).toBeVisible();
  await voidButton.click();
  const dialog = page.getByRole('alertdialog', { name: 'Void this statement?' });
  await dialog.getByLabel('Reason for voiding').fill(`Cash statement ${suffix}`);
  await dialog.getByRole('button', { name: 'Void statement' }).click();
  await expect(dialog).toBeHidden();
  await expect(voidButton).toBeHidden();
});
