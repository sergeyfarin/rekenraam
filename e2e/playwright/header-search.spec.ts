import { readFileSync } from 'node:fs';
import path from 'node:path';
import { expect, repoRoot, test, type Page } from './support/fixtures';
import { expectFocusTrapped, expectNoAccessibilityViolations } from './support/a11y';
import { apiJSON } from './support/api';
import { todayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';

// #490: outside Home the header shows a rounded Search Ctrl K button, never a
// field. It opens the shared search centred over the page (full screen on
// phones); Home keeps its command bar.

const searchDialog = (page: Page, name = 'Search') => page.getByRole('dialog', { name, exact: true });
const combobox = (page: Page, name = 'Search') => page.getByRole('combobox', { name, exact: true });
const headerButton = (page: Page) => page.locator('.shell-bar').getByRole('button', { name: /^Search Ctrl K$/ });
const noSideScroll = (page: Page) => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);

for (const theme of ['light', 'dark'] as const) {
  test(`header button opens the centred search, traps focus and returns it in ${theme}`, async ({ page }) => {
    await readyForLedger(page);
    await page.evaluate(value => localStorage.setItem('rekenraam-theme', value), theme);
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto('/app/ledger');
    const button = headerButton(page);
    await expect(button).toBeVisible();
    await expect(button).toHaveAttribute('aria-keyshortcuts', 'Control+K');
    await expect(button).toHaveAttribute('aria-haspopup', 'dialog');
    // A button, never a second field.
    await expect(page.locator('.shell-bar').getByRole('combobox')).toHaveCount(0);
    await expect(combobox(page)).toHaveCount(0);

    await button.click();
    const dialog = searchDialog(page);
    await expect(dialog).toBeVisible();
    await expect(combobox(page)).toBeFocused();
    // Centred across the page, at the width of Home's command bar.
    const box = (await dialog.boundingBox())!;
    expect(Math.abs(box.x - (1280 - box.x - box.width))).toBeLessThanOrEqual(2);
    expect(box.width).toBeGreaterThan(600);
    await expect(page.getByRole('status').filter({ hasText: /Showing/ })).toBeVisible();
    await expect(dialog.getByRole('group', { name: 'Pages', exact: true })).toBeVisible();
    await expectNoAccessibilityViolations(page, `centred search ${theme}`);
    await expectFocusTrapped(page, dialog, 'centred search');

    // Narrow to a scope chip, then Escape returns focus to the button.
    await combobox(page).focus();
    await dialog.getByRole('button', { name: 'Accounts', exact: true }).click();
    await expect(combobox(page)).toBeFocused();
    await expect(dialog.getByRole('group', { name: 'Pages', exact: true })).toHaveCount(0);
    await page.keyboard.press('Escape');
    await expect(dialog).toHaveCount(0);
    await expect(button).toBeFocused();

    // A backdrop click closes it too.
    await page.keyboard.press('Enter');
    await expect(dialog).toBeVisible();
    await page.mouse.click(20, 880);
    await expect(dialog).toHaveCount(0);
    await expect(button).toBeFocused();
  });
}

test('slash and Ctrl/⌘ K open the centred search from any page, select and close it', async ({ page }) => {
  await readyForLedger(page);
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto('/app/ledger');
  await expect(headerButton(page)).toBeVisible();

  // With nothing focused, closing returns focus to the header button.
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
  await page.keyboard.press('/');
  await expect(combobox(page)).toBeFocused();
  await expect(combobox(page)).toHaveValue('');
  await page.keyboard.press('Escape');
  await expect(searchDialog(page)).toHaveCount(0);
  await expect(headerButton(page)).toBeFocused();

  await page.keyboard.press('Control+k');
  await combobox(page).fill('Spending');
  await expect(page.getByRole('option', { name: 'Spending Reports', exact: true })).toBeVisible();
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(url => url.pathname === '/app/reports/spending');
  await expect(searchDialog(page)).toHaveCount(0);

  await page.keyboard.press('Meta+k');
  await expect(combobox(page)).toBeFocused();
  // A new page starts a new search.
  await expect(combobox(page)).toHaveValue('');
  await page.keyboard.press('Escape');

  // Never over another dialog; works again once that dialog has closed.
  await page.locator('#main-content').focus();
  await page.keyboard.press('?');
  const help = page.getByRole('dialog', { name: 'Keyboard shortcuts', exact: true });
  await expect(help).toBeVisible();
  await page.keyboard.press('Control+k');
  await page.keyboard.press('/');
  await expect(searchDialog(page)).toHaveCount(0);
  await page.keyboard.press('Escape');
  await expect(help).toHaveCount(0);
  await page.keyboard.press('Control+k');
  await expect(combobox(page)).toBeFocused();
  await page.keyboard.press('Escape');
});

test('search never opens over an open transaction editor, so an unsaved edit is not left behind', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const description = `Search guard lunch ${suffix}`;
  const account = await createCashAccount(page, csrfToken, `Search guard wallet ${suffix}`, currencyID);
  const category = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name: `Search guard meals ${suffix}`, category_type: 'expense' });
  const today = todayISO();
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_date: today, status: 'posted', description,
    journal_entries: [{ entry_date: today, postings: [
      { account_id: account.id, commodity_id: currencyID, quantity_value: '-1250', quantity_scale: 2 },
      { account_id: category.id, commodity_id: currencyID, quantity_value: '1250', quantity_scale: 2 }
    ] }]
  });
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto('/app/ledger/transactions');
  await page.getByRole('row').filter({ hasText: description }).getByRole('button', { name: 'Edit', exact: true }).click();
  const panel = page.getByRole('complementary', { name: 'Actions' });
  await expect(panel.getByRole('heading', { name: 'Edit transaction' })).toBeVisible();
  const field = panel.getByLabel('Description');
  await field.fill(`${description} edited`);

  // Not from the field, not from the page, not from the button.
  await field.press('Control+k');
  await page.locator('#main-content').focus();
  await page.keyboard.press('/');
  await page.keyboard.press('Meta+k');
  const button = headerButton(page);
  await expect(button).toHaveAttribute('aria-disabled', 'true');
  await expect(button).toHaveAttribute('title', 'Close the open editor to search');
  // aria-disabled stops Playwright's own click; a forced one must do nothing.
  await button.click({ force: true });
  await expect(searchDialog(page)).toHaveCount(0);
  await expect(field).toHaveValue(`${description} edited`);
  await expect(page).toHaveURL(url => url.pathname === '/app/ledger/transactions');

  // Closing the editor gives search back.
  await panel.getByRole('button', { name: 'Cancel', exact: true }).last().click();
  await expect(panel.getByRole('heading', { name: 'Edit transaction' })).toHaveCount(0);
  await expect(button).not.toHaveAttribute('aria-disabled', 'true');
  await page.keyboard.press('Control+k');
  await expect(combobox(page)).toBeFocused();
  await page.keyboard.press('Escape');
});

test('Home keeps its central bar: no header button, Ctrl K focuses the bar', async ({ page }) => {
  await readyForLedger(page);
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto('/app');
  await expect(page.locator('main').getByRole('combobox', { name: 'Search', exact: true })).toBeVisible();
  await expect(headerButton(page)).toHaveCount(0);
  await page.keyboard.press('Control+k');
  await expect(combobox(page)).toBeFocused();
  await expect(page.getByRole('dialog')).toHaveCount(0);
});

test('the shortcut hint says ⌘ K on Apple platforms', async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(Navigator.prototype, 'platform', { get: () => 'MacIntel' });
    Object.defineProperty(Navigator.prototype, 'userAgentData', { get: () => undefined });
  });
  await readyForLedger(page);
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto('/app/ledger');
  const button = page.locator('.shell-bar').getByRole('button', { name: 'Search ⌘ K', exact: true });
  await expect(button).toBeVisible();
  await expect(button).toHaveAttribute('aria-keyshortcuts', 'Meta+K');
});

test('the button label and Ctrl key are localized in every locale', async ({ page }) => {
  await readyForLedger(page);
  await page.setViewportSize({ width: 1280, height: 900 });
  const messages = (locale: string) => ({
    ...JSON.parse(readFileSync(path.join(repoRoot, 'frontend/messages/app/en.json'), 'utf8')),
    ...JSON.parse(readFileSync(path.join(repoRoot, 'frontend/messages/app', `${locale}.json`), 'utf8'))
  }) as Record<string, string>;
  await page.goto('/app/ledger');
  for (const locale of ['en', 'nl', 'ru', 'es', 'fr', 'de']) {
    const text = messages(locale);
    await page.evaluate(value => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    await page.reload();
    const button = page.locator('.shell-bar').getByRole('button', { name: `${text.search_label} ${text.keyboard_key_ctrl} K`, exact: true });
    await expect(button).toBeVisible();
    await button.click();
    await expect(combobox(page, text.search_label)).toBeFocused();
    await page.keyboard.press('Escape');
    await expect(button).toBeFocused();
  }
});

for (const width of [320, 800]) {
  test(`header search reflows without sideways scroll at ${width}px`, async ({ page }) => {
    await readyForLedger(page);
    await page.setViewportSize({ width, height: 800 });
    await page.goto('/app/ledger');
    const trigger = width < 768
      ? page.locator('.shell-bar').getByRole('button', { name: 'Search', exact: true })
      : headerButton(page);
    await expect(trigger).toBeVisible();
    expect(await noSideScroll(page)).toBe(0);
    await trigger.click();
    const dialog = searchDialog(page);
    await expect(combobox(page)).toBeFocused();
    await combobox(page).fill('Spending');
    await expect(page.getByRole('option').first()).toBeVisible();
    const box = (await dialog.boundingBox())!;
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(width);
    expect(await dialog.evaluate(node => node.scrollWidth - node.clientWidth)).toBe(0);
    await expectNoAccessibilityViolations(page, `search dialog ${width}`);
    await page.keyboard.press('Escape');
    await expect(dialog).toHaveCount(0);
    await expect(trigger).toBeFocused();
  });
}
