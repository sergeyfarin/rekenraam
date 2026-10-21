import { readFileSync } from 'node:fs';
import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from './support/fixtures';
import { apiJSON, csrfTokenFor } from './support/api';
import { todayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';

test.use({ viewport: { width: 390, height: 844 } });

async function seed(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `Row actions ${suffix}`, currencyID);
  const category = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: `Row category ${suffix}`, category_type: 'expense'
  });
  const description = `Review coffee ${suffix}`;
  const transaction = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_date: todayISO(), transaction_kind: 'ordinary', status: 'posted', needs_review: true, description,
    journal_entries: [{ entry_date: todayISO(), entry_kind: 'ordinary', postings: [
      { account_id: account.id, commodity_id: currencyID, quantity_value: '-450', quantity_scale: 2 },
      { account_id: category.id, commodity_id: currencyID, quantity_value: '450', quantity_scale: 2 }
    ] }]
  });
  const paths = {
    journal: `/app/transactions?account_id=${account.id}`,
    account: `/app/accounts/${account.id}/register`,
    category: `/app/categories/${category.id}`
  };
  return { account, category, transaction, description, paths, currencyID };
}

const row = (page: Page, description: string) => page.getByRole('row').filter({ hasText: description });
const readTransaction = (page: Page, id: number) => apiJSON<{ status: string; needs_review: boolean }>(page, 'GET', `/api/v1/transactions/${id}`);

for (const screen of ['journal', 'account', 'category'] as const) {
  test(`${screen} row actions approve, edit, correct and unvoid the transaction on mobile`, async ({ page }) => {
    const s = await seed(page);
    await page.goto(s.paths[screen]);
    const entry = row(page, s.description);
    await entry.getByRole('button', { name: 'Approve', exact: true }).click();
    await expect.poll(async () => (await readTransaction(page, s.transaction.id)).needs_review).toBe(false);
    await expect(entry.getByRole('button', { name: 'Approve', exact: true })).toHaveCount(0);
    // Approving must not also open the row's detail panel.
    await expect(page.getByRole('complementary', { name: 'Actions' })).toHaveCount(0);

    await entry.getByRole('button', { name: 'Edit', exact: true }).focus();
    await page.keyboard.press('Enter');
    await expect(page.getByRole('heading', { name: 'Edit transaction', exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Cancel', exact: true }).last().click();
    await page.getByRole('button', { name: 'Close', exact: true }).click();

    await entry.getByRole('button', { name: 'Create correction', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Create correction', exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Cancel', exact: true }).last().click();
    await page.getByRole('button', { name: 'Close', exact: true }).click();

    await apiJSON(page, 'POST', `/api/v1/transactions/${s.transaction.id}/void`, await csrfTokenFor(page), { change_reason: 'Review duplicate' });
    await page.reload();
    // The register API defaults to posted entries; select voided explicitly.
    if (screen === 'account') await page.getByRole('combobox', { name: 'Status', exact: true }).selectOption('voided');
    await entry.getByRole('button', { name: 'Unvoid', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
    expect((await readTransaction(page, s.transaction.id)).status).toBe('voided');
    await page.getByRole('button', { name: 'Close', exact: true }).click();
    await entry.getByRole('button', { name: 'Unvoid', exact: true }).click();
    await dialog.getByRole('textbox').fill('Restore valid entry');
    await dialog.getByRole('button', { name: 'Unvoid', exact: true }).click();
    await expect.poll(async () => (await readTransaction(page, s.transaction.id)).status).toBe('posted');
    if (screen === 'account') await page.getByRole('combobox', { name: 'Status', exact: true }).selectOption('posted');
    await expect(entry.getByRole('button', { name: 'Edit', exact: true })).toBeVisible();
    await expect(entry.getByRole('button', { name: 'Unvoid', exact: true })).toHaveCount(0);
  });
}

test('a stale edit is refused, the editor reloads the newer version and its save keeps it (#296)', async ({ page }) => {
  const s = await seed(page);
  await page.goto(s.paths.journal);
  await row(page, s.description).getByRole('button', { name: 'Edit', exact: true }).click();
  const form = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Edit transaction', exact: true }) });
  await expect(form.getByLabel('Amount', { exact: true })).toHaveValue('-4.50');

  // Another tab changes the amount to 9.00 while this form is open.
  type Stored = { version_id: number; description: string; journal_entries: Array<{ postings: Array<{ line_key: string; account_id: number; quantity_value: string }> }> };
  const opened = await apiJSON<Stored>(page, 'GET', `/api/v1/transactions/${s.transaction.id}`);
  await apiJSON(page, 'PATCH', `/api/v1/transactions/${s.transaction.id}`, await csrfTokenFor(page), {
    expected_version_id: opened.version_id, transaction_date: todayISO(), description: s.description, change_reason: 'other tab',
    journal_entries: [{ entry_date: todayISO(), postings: opened.journal_entries[0].postings.map((p) => ({
      line_key: p.line_key, account_id: p.account_id, commodity_id: s.currencyID,
      quantity_value: p.account_id === s.account.id ? '-900' : '900', quantity_scale: 2
    })) }]
  });

  // This form still carries 4.50: its description-only save must not revert 9.00.
  await form.getByLabel('Description', { exact: true }).fill(`${s.description} edited`);
  const refused = page.waitForResponse((r) => r.request().method() === 'PATCH' && new URL(r.url()).pathname === `/api/v1/transactions/${s.transaction.id}`);
  await form.getByRole('button', { name: 'Save transaction', exact: true }).click();
  expect((await refused).status()).toBe(409);
  await expect(form.getByRole('status').filter({ hasText: 'The form now shows the latest version' })).toBeVisible();
  await expect(form.getByLabel('Amount', { exact: true })).toHaveValue('-9.00');
  const kept = await apiJSON<Stored>(page, 'GET', `/api/v1/transactions/${s.transaction.id}`);
  expect(kept.journal_entries[0].postings.find((p) => p.account_id === s.account.id)?.quantity_value).toBe('-900');

  // Saved again from the reloaded form, the change lands on the newer version.
  await form.getByLabel('Description', { exact: true }).fill(`${s.description} edited`);
  const saved = page.waitForResponse((r) => r.request().method() === 'PATCH' && new URL(r.url()).pathname === `/api/v1/transactions/${s.transaction.id}`);
  await form.getByRole('button', { name: 'Save transaction', exact: true }).click();
  expect((await saved).status()).toBe(200);
  const final = await apiJSON<Stored>(page, 'GET', `/api/v1/transactions/${s.transaction.id}`);
  expect(final.description).toBe(`${s.description} edited`);
  expect(final.journal_entries[0].postings.find((p) => p.account_id === s.account.id)?.quantity_value).toBe('-900');
});

for (const locale of ['en', 'es', 'fr', 'nl', 'de', 'ru']) {
  test(`row actions use ${locale} labels and keyboard controls on all three screens at 390px`, async ({ page }) => {
    const s = await seed(page);
    const m = JSON.parse(readFileSync(new URL(`../../frontend/messages/settings/${locale}.json`, import.meta.url), 'utf8'));
    await page.evaluate(value => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    for (const path of Object.values(s.paths)) {
      await page.goto(path);
      const actions = row(page, s.description).getByRole('group', { name: m.transactions_detail_actions, exact: true });
      await expect(actions.getByRole('button', { name: m.transactions_action_approve, exact: true })).toBeVisible();
      await expect(actions.getByRole('button', { name: m.transactions_action_correct, exact: true })).toBeVisible();
      const edit = actions.getByRole('button', { name: m.transactions_action_edit, exact: true });
      await edit.focus();
      await page.keyboard.press('Enter');
      await expect(page.getByRole('heading', { name: m.transactions_form_edit_title, exact: true })).toBeVisible();
      expect((await new AxeBuilder({ page }).include('main').withRules(['button-name', 'aria-allowed-attr', 'nested-interactive']).analyze()).violations).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    }
  });
}

test('row actions show failures, disable repeated commands while pending and allow retry', async ({ page }) => {
  const s = await seed(page);
  await page.goto(s.paths.account);
  const entry = row(page, s.description);
  let finishRequest!: () => void;
  const pending = new Promise<void>(resolve => { finishRequest = resolve; });
  await page.route(`**/api/v1/transactions/${s.transaction.id}/approve`, async route => {
    await pending;
    await route.fulfill({ status: 500, json: { error: { code: 'INTERNAL_ERROR', message: 'failure' } } });
  });
  await entry.getByRole('button', { name: 'Approve', exact: true }).click();
  await expect(entry.getByRole('button', { name: 'Approve', exact: true })).toBeDisabled();
  await expect(entry.getByRole('button', { name: 'Edit', exact: true })).toBeDisabled();
  finishRequest();
  await expect(page.getByRole('alert')).toBeVisible();
  expect((await readTransaction(page, s.transaction.id)).needs_review).toBe(true);
  await page.unroute(`**/api/v1/transactions/${s.transaction.id}/approve`);
  await entry.getByRole('button', { name: 'Approve', exact: true }).click();
  await expect(entry.getByRole('button', { name: 'Approve', exact: true })).toHaveCount(0);
  await expect(page.getByRole('alert')).toHaveCount(0);

  await page.route(`**/api/v1/transactions/${s.transaction.id}`, route => route.fulfill({ status: 500, json: { error: { code: 'INTERNAL_ERROR', message: 'failure' } } }));
  await entry.getByRole('button', { name: 'Edit', exact: true }).click();
  await expect(page.getByRole('alert')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Edit transaction', exact: true })).toHaveCount(0);
  await page.unroute(`**/api/v1/transactions/${s.transaction.id}`);
  await entry.getByRole('button', { name: 'Edit', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Edit transaction', exact: true })).toBeVisible();
});

test('unvoid asks before retrying a reconciliation refusal with override', async ({ page }) => {
  const s = await seed(page);
  await apiJSON(page, 'POST', `/api/v1/transactions/${s.transaction.id}/void`, await csrfTokenFor(page), { change_reason: 'Review duplicate' });
  const overrides: boolean[] = [];
  const reviewedSets: unknown[] = [];
  await page.route(`**/api/v1/transactions/${s.transaction.id}/unvoid`, async route => {
    const body = route.request().postDataJSON();
    const override = body.reconciliation_override === true;
    overrides.push(override);
    if (!override) {
      await route.fulfill({ status: 409, json: { error: { code: 'CONFLICT', message: 'reconciliation override is required' } } });
      return;
    }
    // The override names the previewed set (#296); the real book has no
    // checkpoint, so the request reaches it acknowledging the real, empty set.
    reviewedSets.push(body.reconciliation_override_checkpoint_ids);
    await route.continue({ postData: JSON.stringify({ ...body, reconciliation_override_checkpoint_ids: [] }) });
  });
  // Isolate the lifecycle dialog's override negotiation; the real unvoid
  // preview is covered in transactions-lifecycle-reconciliation.spec.ts (#220).
  await page.route(`**/api/v1/transactions/${s.transaction.id}/unvoid/reconciliation-impact`, route => route.fulfill({
    json: { affected_checkpoints: [{ checkpoint_id: 1, account_id: s.account.id, account_label: s.account.name,
      commodity_id: 1, commodity_code: 'USD', statement_date: todayISO(), statement_account_sequence: 1, entry_date: todayISO() }] }
  }));
  await page.goto(s.paths.category);
  const unvoid = row(page, s.description).getByRole('button', { name: 'Unvoid', exact: true });
  await unvoid.click();
  await page.getByRole('dialog').getByRole('textbox').fill('Restore valid entry');
  await page.getByRole('dialog').getByRole('button', { name: 'Unvoid', exact: true }).click();
  const warning = page.getByRole('alertdialog');
  await expect(warning).toContainText(s.account.name);
  expect((await readTransaction(page, s.transaction.id)).status).toBe('voided');
  await warning.getByRole('button', { name: 'Go back', exact: true }).click();
  expect(overrides).toEqual([false]);
  await page.getByRole('button', { name: 'Close', exact: true }).click();
  await unvoid.click();
  await page.getByRole('dialog').getByRole('textbox').fill('Restore valid entry');
  await page.getByRole('dialog').getByRole('button', { name: 'Unvoid', exact: true }).click();
  await warning.getByRole('button', { name: 'Proceed and invalidate', exact: true }).click();
  await expect.poll(async () => (await readTransaction(page, s.transaction.id)).status).toBe('posted');
  expect(overrides).toEqual([false, false, true]);
  expect(reviewedSets).toEqual([[1]]);
});

for (const screen of ['journal', 'account', 'category'] as const) {
  test(`${screen} retains loading, error, retry and empty list states`, async ({ page }) => {
    const s = await seed(page);
    let release!: () => void;
    const pending = new Promise<void>(resolve => { release = resolve; });
    let state: 'loading' | 'empty' | 'success' = 'loading';
    const listURL = screen === 'account'
      ? `**/api/v1/accounts/${s.account.id}/register*` : '**/api/v1/transactions?*';
    await page.route(listURL, async route => {
      if (state === 'success') return route.continue();
      if (state === 'loading') {
        await pending;
        return route.fulfill({ status: 400, json: { error: { code: 'VALIDATION_FAILED', message: 'failure' } } });
      }
      return route.fulfill({ json: screen === 'account' ? { entries: [], next_cursor: null } : { transactions: [], next_cursor: null } });
    });
    await page.goto(s.paths[screen]);
    await expect(page.locator('[data-transaction-table] .animate-pulse').first()).toBeVisible();
    release();
    await expect(page.getByRole('button', { name: 'Retry', exact: true })).toBeVisible();
    state = 'empty';
    await page.getByRole('button', { name: 'Retry', exact: true }).click();
    await expect(page.getByText('No transactions', { exact: true })).toBeVisible();
    state = 'success';
    await page.reload();
    await expect(row(page, s.description).getByRole('button', { name: 'Edit', exact: true })).toBeVisible();
  });
}

test('investment rows offer their detail instead of the generic editor', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const cash = await createCashAccount(page, csrfToken, `Action cash ${suffix}`, currencyID);
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: `RA${suffix}`, instrument_type: 'stock', display_name: `Action stock ${suffix}`, symbol: `RA${suffix}`,
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 0, price_scale: 2, effective_from: todayISO()
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Action holding ${suffix}`, opened_on: todayISO(), effective_from: todayISO()
  });
  await apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, {
    transaction_date: todayISO(), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: cash.id, quantity_value: '1', quantity_scale: 0, cash_amount_value: '1000',
    cash_amount_scale: 2, cash_commodity_id: currencyID
  });
  for (const path of [`/app/accounts/${holding.id}/register`, `/app/transactions?account_id=${holding.id}`]) {
    await page.goto(path);
    const entry = page.locator('[data-table-row]').first();
    await expect(entry).toBeVisible();
    await expect(entry.getByRole('button', { name: 'Edit', exact: true })).toHaveCount(0);
    await expect(entry.getByRole('button', { name: 'Create correction', exact: true })).toHaveCount(0);
    await entry.click();
    await expect(page.getByText('Investment postings use named workflows.', { exact: false })).toBeVisible();
  }
});

test('a desktop row can reopen unvoid after cancelling while detail stays open', async ({ page }) => {
  const s = await seed(page);
  await apiJSON(page, 'POST', `/api/v1/transactions/${s.transaction.id}/void`, await csrfTokenFor(page), { change_reason: 'Review duplicate' });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto(s.paths.account);
  await page.getByRole('combobox', { name: 'Status', exact: true }).selectOption('voided');
  const unvoid = row(page, s.description).getByRole('button', { name: 'Unvoid', exact: true });
  await unvoid.click();
  await page.getByRole('dialog').getByRole('button', { name: 'Cancel', exact: true }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await unvoid.click();
  await expect(page.getByRole('dialog', { name: 'Unvoid transaction' })).toBeVisible();
});

test('the account register shows one lifecycle at a time and never offers a posted-only "All" (#223)', async ({ page }) => {
  const s = await seed(page);
  const voidedDescription = `Voided coffee ${Date.now()}`;
  const csrfToken = await csrfTokenFor(page);
  const voided = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_date: todayISO(), transaction_kind: 'ordinary', status: 'posted', description: voidedDescription,
    journal_entries: [{ entry_date: todayISO(), entry_kind: 'ordinary', postings: [
      { account_id: s.account.id, commodity_id: s.currencyID, quantity_value: '-200', quantity_scale: 2 },
      { account_id: s.category.id, commodity_id: s.currencyID, quantity_value: '200', quantity_scale: 2 }
    ] }]
  });
  await apiJSON(page, 'POST', `/api/v1/transactions/${voided.id}/void`, await csrfTokenFor(page), { change_reason: 'Duplicate' });

  await page.goto(s.paths.account);
  const status = page.getByRole('combobox', { name: 'Status', exact: true });
  await expect(status).toHaveValue('posted');
  await expect(status.getByRole('option')).toHaveText(['Posted', 'Voided']);
  await expect(row(page, s.description)).toBeVisible();
  await expect(row(page, voidedDescription)).toHaveCount(0);

  await status.selectOption('voided');
  await expect(row(page, voidedDescription)).toBeVisible();
  await expect(row(page, s.description)).toHaveCount(0);

  await row(page, voidedDescription).getByRole('button', { name: 'Unvoid', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('textbox').fill('Restore valid entry');
  await dialog.getByRole('button', { name: 'Unvoid', exact: true }).click();
  await expect.poll(async () => (await readTransaction(page, voided.id)).status).toBe('posted');
  await page.reload();
  // #326 keeps the chosen lifecycle on reload. The unvoided row has left it.
  await expect(status).toHaveValue('voided');
  await expect(row(page, voidedDescription)).toHaveCount(0);
  await status.selectOption('posted');
  await expect(status).toHaveValue('posted');
  await expect(row(page, voidedDescription)).toBeVisible();
  await expect(row(page, s.description)).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test('a category screen shows only its own leg of a split transaction (#222)', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `Split cash ${suffix}`, currencyID);
  const groceries = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: `Split groceries ${suffix}`, category_type: 'expense'
  });
  const dining = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: `Split dining ${suffix}`, category_type: 'expense'
  });
  const description = `Split shop ${suffix}`;
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_date: todayISO(), transaction_kind: 'ordinary', status: 'posted', description,
    journal_entries: [{ entry_date: todayISO(), entry_kind: 'ordinary', postings: [
      { account_id: account.id, commodity_id: currencyID, quantity_value: '-3000', quantity_scale: 2 },
      { account_id: groceries.id, commodity_id: currencyID, quantity_value: '1000', quantity_scale: 2 },
      { account_id: dining.id, commodity_id: currencyID, quantity_value: '2000', quantity_scale: 2 }
    ] }]
  });
  for (const [category, expected, other] of [[groceries, '10.00', '20.00'], [dining, '20.00', '10.00']] as const) {
    await page.goto(`/app/categories/${category.id}`);
    const entry = row(page, description);
    await expect(entry).toContainText(expected);
    await expect(entry).not.toContainText(other);
    await expect(entry).not.toContainText('30.00');
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  }
});
