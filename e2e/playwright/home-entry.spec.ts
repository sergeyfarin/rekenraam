import type { Page } from '@playwright/test';
import { expect, test } from './support/fixtures';
import { apiJSON } from './support/api';
import { todayISO } from './support/dates';
import { expectFocusTrapped, expectNoAccessibilityViolations, focusIsInside } from './support/a11y';
import { createCashAccount, readyForLedger } from './support/ledger';

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
    await dialog.getByPlaceholder('Search categories').click();
    await dialog.locator('ul[role="listbox"] button[role="option"]').first().click();
    const created = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/transactions');
    await dialog.getByRole('button', { name: 'Add transaction', exact: true }).click();
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
