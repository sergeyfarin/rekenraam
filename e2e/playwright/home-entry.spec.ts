import type { Page } from '@playwright/test';
import { expect, test } from './support/fixtures';
import { apiJSON } from './support/api';
import { todayISO } from './support/dates';
import { expectFocusTrapped, expectNoAccessibilityViolations, focusIsInside } from './support/a11y';
import { createCashAccount, readyForLedger } from './support/ledger';
import { observePageRequests, waitForPageReady } from './support/page-readiness';

// Home's command bar, shortcut tiles and the shared Add transaction dialog
// (#477): the existing editor in a dialog on desktop and a full-screen sheet on
// phones, discarded by Cancel or Escape, returning focus to its opener, and
// holding unsaved input across a background refresh of Home (#433).

const entryDialog = (page: Page) => page.getByRole('dialog', { name: 'Add transaction', exact: true });
const shortcuts = (page: Page) => page.getByRole('list', { name: 'Shortcuts', exact: true });
const addTile = (page: Page) => shortcuts(page).getByRole('button', { name: 'Add transaction', exact: true });

async function setTheme(page: Page, theme: 'light' | 'dark') {
  await page.evaluate(value => localStorage.setItem('rekenraam-theme', value), theme);
}

for (const theme of ['light', 'dark'] as const) {
  test(`desktop Home: command bar, tiles and the entry dialog by keyboard in ${theme}`, async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const account = await createCashAccount(page, csrfToken, `Home entry ${theme} ${Date.now()}`, currencyID);
    const category = await apiJSON<{ name: string }>(page, 'POST', '/api/v1/categories', csrfToken, { name: `Home entry spend ${theme} ${Date.now()}`, category_type: 'expense' });
    const description = `Home entry ${theme} ${Date.now()}`;
    await setTheme(page, theme);
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.clock.install();
    await page.goto('/app');

    // One search, in Home's placement: the header instance is omitted here.
    const search = page.getByRole('combobox', { name: 'Search', exact: true });
    await expect(search).toHaveCount(1);
    await expect(page.locator('main').getByRole('combobox', { name: 'Search', exact: true })).toBeVisible();
    await page.keyboard.press('/');
    await expect(search).toBeFocused();
    await expect(page.getByRole('listbox', { name: 'Search results', exact: true })).toBeVisible();
    await page.keyboard.press('Escape');

    // Add transaction leads, in the accent colour; every tile shows on desktop.
    const tiles = shortcuts(page).getByRole('listitem');
    await expect(tiles).toHaveCount(4);
    await expect(tiles.first()).toHaveText('Add transaction');
    for (const name of ['Import file', 'Transactions', 'Spending by category']) {
      await expect(shortcuts(page).getByRole('link', { name, exact: true })).toBeVisible();
    }
    await expect(shortcuts(page).getByRole('button', { name: 'More', exact: true })).toBeHidden();
    await expectNoAccessibilityViolations(page, `desktop Home ${theme}`);

    await addTile(page).focus();
    await page.keyboard.press('Enter');
    const dialog = entryDialog(page);
    await expect(dialog).toBeVisible();
    await expect.poll(() => focusIsInside(dialog)).toBe(true);
    expect((await dialog.boundingBox())!.width).toBeLessThan(1000);
    await expect(page).toHaveURL(url => url.pathname === '/app');
    await expectNoAccessibilityViolations(page, `desktop entry dialog ${theme}`);
    // Walk from the category field with its suggestions open: options are not
    // Tab stops, so Tab never lands on one that closes under it (#477).
    const categoryField = dialog.getByRole('combobox', { name: 'Category', exact: true });
    await categoryField.focus();
    await expect(categoryField).toHaveAttribute('aria-expanded', 'true');
    await expectNoAccessibilityViolations(page, `desktop entry dialog with suggestions ${theme}`);
    await expectFocusTrapped(page, dialog, `desktop entry dialog ${theme}`);

    // Unsaved input survives Home refetching behind the dialog.
    await dialog.getByLabel('Description', { exact: true }).fill(description);
    await dialog.getByLabel('Amount', { exact: true }).fill('-12.34');
    await page.clock.fastForward('00:45');
    const refetched = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/home' && response.ok());
    await page.context().setOffline(true);
    await page.context().setOffline(false);
    await refetched;
    await expect(dialog.getByLabel('Description', { exact: true })).toHaveValue(description);
    await expect(dialog.getByLabel('Amount', { exact: true })).toHaveValue('-12.34');

    // Escape discards the working copy, posts nothing and returns focus.
    const posts: string[] = [];
    page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/transactions') posts.push(request.url()); });
    await page.keyboard.press('Escape');
    await expect(dialog).toHaveCount(0);
    await expect(addTile(page)).toBeFocused();
    expect(posts).toEqual([]);

    // N reopens it empty; saving closes it, confirms and returns focus.
    await page.keyboard.press('n');
    await expect(dialog).toBeVisible();
    await expect(dialog.getByLabel('Description', { exact: true })).toHaveValue('');
    await expect(dialog.getByLabel('Amount', { exact: true })).toHaveValue('');
    await dialog.getByLabel('Date', { exact: true }).fill(todayISO());
    await dialog.getByLabel('Description', { exact: true }).fill(description);
    await dialog.getByRole('combobox', { name: 'Account', exact: true }).selectOption({ label: account.name });
    await dialog.getByLabel('Amount', { exact: true }).fill('-12.34');
    // The category by keyboard: type, arrow to the match, Enter.
    await categoryField.focus();
    await page.keyboard.type(category.name);
    await page.keyboard.press('ArrowDown');
    await expect(categoryField).toHaveAttribute('aria-activedescendant', /category-0$/);
    await page.keyboard.press('Enter');
    await expect(dialog.getByText(category.name, { exact: true })).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Clear selection', exact: true })).toBeFocused();
    const created = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/transactions');
    await dialog.getByRole('button', { name: 'Add transaction', exact: true }).press('Enter');
    expect((await created).status()).toBe(201);
    const transaction = await (await created).json() as { id: number };
    await expect(dialog).toHaveCount(0);
    await expect(page.getByRole('status').filter({ hasText: 'Transaction added.' })).toBeAttached();
    await expect(addTile(page)).toBeFocused();
    await page.getByRole('link', { name: 'View transaction', exact: true }).click();
    await expect(page).toHaveURL(url => url.pathname === '/app/ledger/transactions' && url.searchParams.get('transaction_id') === String(transaction.id));
    await expect(page.getByRole('definition').filter({ hasText: description })).toBeVisible();
    const saved = await apiJSON<{ description: string; status: string }>(page, 'GET', `/api/v1/transactions/${transaction.id}`);
    expect(saved).toMatchObject({ description, status: 'posted' });
  });
}

test.describe('on a touch phone', () => {
  test.use({ hasTouch: true, viewport: { width: 390, height: 844 } });

  for (const theme of ['light', 'dark'] as const) {
    test(`phone Home: full-screen search, three tiles plus More and a full-screen entry sheet in ${theme}`, async ({ page }) => {
      await readyForLedger(page);
      await setTheme(page, theme);
      await page.goto('/app');

      // The command bar opens the same search full screen.
      const trigger = page.getByRole('button', { name: 'Search pages, accounts, commands…', exact: true });
      await trigger.tap();
      const searchDialog = page.getByRole('dialog', { name: 'Search', exact: true });
      await expect.poll(() => searchDialog.evaluate(node => Math.round(node.getBoundingClientRect().width))).toBe(390);
      await page.getByRole('button', { name: 'Close search', exact: true }).tap();
      await expect(searchDialog).toHaveCount(0);

      // Three tiles plus More; More lists the rest.
      await expect(addTile(page)).toBeVisible();
      await expect(shortcuts(page).getByRole('link', { name: 'Import file', exact: true })).toBeVisible();
      await expect(shortcuts(page).getByRole('link', { name: 'Transactions', exact: true })).toBeVisible();
      await expect(shortcuts(page).getByRole('link', { name: 'Spending by category', exact: true })).toBeHidden();
      const more = shortcuts(page).getByRole('button', { name: 'More', exact: true });
      await more.tap();
      await expect(more).toHaveAttribute('aria-expanded', 'true');
      await expect(page.getByRole('group', { name: 'More', exact: true }).getByRole('link', { name: 'Spending by category', exact: true })).toBeVisible();
      await expectNoAccessibilityViolations(page, `phone Home ${theme}`);
      await page.keyboard.press('Escape');
      await expect(more).toHaveAttribute('aria-expanded', 'false');

      await addTile(page).tap();
      const dialog = entryDialog(page);
      await expect(dialog).toBeVisible();
      await expect.poll(() => dialog.evaluate(node => Math.round(node.getBoundingClientRect().width))).toBe(390);
      await expect.poll(() => focusIsInside(dialog)).toBe(true);
      await expectNoAccessibilityViolations(page, `phone entry sheet ${theme}`);
      await dialog.getByLabel('Description', { exact: true }).fill('Not kept');
      await dialog.getByRole('button', { name: 'Cancel', exact: true }).last().tap();
      await expect(dialog).toHaveCount(0);
      await expect(addTile(page)).toBeFocused();
      await addTile(page).tap();
      await expect(dialog.getByLabel('Description', { exact: true })).toHaveValue('');
    });
  }
});

async function fillExpense(page: Page, accountName: string, categoryName: string, description: string, amount = '-23.45') {
  const dialog = entryDialog(page);
  await dialog.getByLabel('Date', { exact: true }).fill(todayISO());
  await dialog.getByLabel('Description', { exact: true }).fill(description);
  await dialog.getByRole('combobox', { name: 'Account', exact: true }).selectOption({ label: accountName });
  await dialog.getByLabel('Amount', { exact: true }).fill(amount);
  await dialog.getByPlaceholder('Search categories').fill(categoryName);
  await dialog.locator('ul[role="listbox"] button[role="option"]').filter({ hasText: categoryName }).first().click();
}

test('Escape cannot discard an entry while the payee it confirmed is still being created', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `Payee chain ${suffix}`, currencyID);
  const category = await apiJSON<{ name: string }>(page, 'POST', '/api/v1/categories', csrfToken, { name: `Payee chain spend ${suffix}`, category_type: 'expense' });
  const payee = `Payee chain shop ${suffix}`;
  await page.goto('/app');
  await addTile(page).click();
  const dialog = entryDialog(page);
  await fillExpense(page, account.name, category.name, `Payee chain ${suffix}`);
  await dialog.getByLabel('Payee', { exact: true }).fill(payee);
  await dialog.getByRole('button', { name: 'Add transaction', exact: true }).click();

  // Hold the payee creation, the first write of the save chain.
  let release!: () => void;
  const held = new Promise<void>(resolve => { release = resolve; });
  await page.route('**/api/v1/payees', async route => {
    if (route.request().method() === 'POST') await held;
    await route.continue();
  });
  const posts: number[] = [];
  page.on('response', response => {
    if (response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/transactions') posts.push(response.status());
  });
  await dialog.getByRole('button', { name: `Create “${payee}”`, exact: true }).click();
  await expect(dialog.locator('form[aria-busy="true"]')).toBeAttached();
  await page.keyboard.press('Escape');
  await expect(dialog).toBeVisible();
  await expect(dialog.getByLabel('Description', { exact: true })).toHaveValue(`Payee chain ${suffix}`);

  // The owner's confirmed save completes once, and only then closes the dialog.
  release();
  await expect(dialog).toHaveCount(0);
  await expect.poll(() => posts).toEqual([201]);
});

test('saving over a report refreshes it in place', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `Report entry ${suffix}`, currencyID);
  const category = await apiJSON<{ name: string }>(page, 'POST', '/api/v1/categories', csrfToken, { name: `Report entry spend ${suffix}`, category_type: 'expense' });
  observePageRequests(page);
  await page.goto('/app/reports/spending');
  await expect(page).toHaveURL(url => url.searchParams.has('start_date'));
  await waitForPageReady(page);
  // This test's own category has no spending row until the entry is saved.
  const spent = page.locator('main').getByRole('row').filter({ hasText: category.name }).filter({ hasText: '31.17' });
  await expect(page.locator('main').getByRole('table').or(page.getByText('No spending in this period')).first()).toBeVisible();
  await expect(spent).toHaveCount(0);

  await page.locator('#main-content').focus();
  await page.keyboard.press('n');
  await fillExpense(page, account.name, category.name, `Report entry ${suffix}`, '-31.17');
  const refreshed = page.waitForResponse(response => response.request().method() === 'GET' && new URL(response.url()).pathname === '/api/v1/reports/spending' && response.ok());
  await entryDialog(page).getByRole('button', { name: 'Add transaction', exact: true }).click();
  await refreshed;
  await expect(entryDialog(page)).toHaveCount(0);
  await expect(spent.first()).toBeVisible();
});

test('a failed session refresh keeps the unsaved entry, and focus has somewhere to return', async ({ page }) => {
  await readyForLedger(page);
  const description = `Shell failure ${Date.now()}`;
  const sessionFails = () => page.route('**/api/v1/auth/session', route => route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: { code: 'INTERNAL_ERROR', message: 'synthetic' } }) }));
  // The session query refetches whenever the window regains focus.
  const refocus = (status: number) => Promise.all([
    page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/auth/session' && response.status() === status),
    page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')))
  ]);
  await page.goto('/app');
  await addTile(page).click();
  const dialog = entryDialog(page);
  await dialog.getByLabel('Description', { exact: true }).fill(description);
  await dialog.getByLabel('Amount', { exact: true }).fill('-7.50');

  await sessionFails();
  await refocus(500);
  await expect(page.getByText('The app shell could not load')).toBeAttached();
  await expect(dialog).toBeVisible();
  await expect(dialog.getByLabel('Description', { exact: true })).toHaveValue(description);
  await expect(dialog.getByLabel('Amount', { exact: true })).toHaveValue('-7.50');

  // Cancelled over the shell error, focus goes to its Retry, not <body>.
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).last().click();
  await expect(dialog).toHaveCount(0);
  const retry = page.getByRole('button', { name: 'Retry shell', exact: true });
  await expect(retry).toBeFocused();
  await page.unroute('**/api/v1/auth/session');
  await retry.click();
  await expect(addTile(page)).toBeVisible();

  // Recovered while the entry stays open: the opener was re-rendered away,
  // so Cancel returns focus to the page's main content.
  await addTile(page).click();
  await dialog.getByLabel('Description', { exact: true }).fill(description);
  await sessionFails();
  await refocus(500);
  await expect(page.getByText('The app shell could not load')).toBeAttached();
  await page.unroute('**/api/v1/auth/session');
  await refocus(200);
  await expect(page.getByText('The app shell could not load')).toHaveCount(0);
  await expect(dialog.getByLabel('Description', { exact: true })).toHaveValue(description);
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).last().click();
  await expect(dialog).toHaveCount(0);
  await expect(page.locator('#main-content')).toBeFocused();
});
