import { expect, test, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { expectNoAccessibilityViolations } from './support/a11y';
import { createCashAccount, readyForLedger } from './support/ledger';

/**
 * #459: the page header carries the page's main action. Transactions, an
 * account register and a category put *Add transaction* there, prefilled for
 * the page, so entry works by touch; N and `?` are keyboard extras that never
 * act while typing, and page widths keep the shell's left edge.
 */

const pageHeader = (page: Page) => page.locator('main header').first();
const addForm = (page: Page) => page.locator('form').filter({ has: page.getByRole('heading', { name: 'Add transaction', exact: true }) });

async function seed(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `Action checking ${suffix}`, currencyID);
  const category = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: `Action hobbies ${suffix}`, category_type: 'expense'
  });
  return { account, category: { id: category.id, name: `Action hobbies ${suffix}` } };
}

test.describe('on a phone', () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true });

  test('Add transaction is tapped from each page header and starts from that page', async ({ page }) => {
    const { account, category } = await seed(page);
    const noSidewaysScroll = async () => expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);

    await page.goto('/app/ledger/transactions');
    await expect(pageHeader(page).getByRole('heading', { level: 1, name: 'Transactions' })).toBeVisible();
    await pageHeader(page).getByRole('button', { name: 'Add transaction', exact: true }).tap();
    await expect(addForm(page)).toBeVisible();
    await addForm(page).getByRole('button', { name: 'Cancel', exact: true }).first().tap();

    await page.goto(`/app/ledger/accounts/${account.id}/register`);
    await expect(pageHeader(page).getByRole('heading', { level: 1, name: account.name })).toBeVisible();
    await expect(pageHeader(page).getByRole('navigation', { name: 'Breadcrumb' }).getByRole('link', { name: 'Accounts', exact: true }))
      .toHaveAttribute('href', '/app/ledger/accounts');
    await noSidewaysScroll();
    await pageHeader(page).getByRole('button', { name: 'Add transaction', exact: true }).tap();
    await expect(addForm(page).getByLabel('Account').locator('option:checked')).toHaveText(account.name);
    await noSidewaysScroll();
    await addForm(page).getByRole('button', { name: 'Cancel', exact: true }).first().tap();

    await page.goto(`/app/ledger/categories/${category.id}`);
    await expect(pageHeader(page).getByRole('heading', { level: 1, name: category.name })).toBeVisible();
    await pageHeader(page).getByRole('button', { name: 'Add transaction', exact: true }).tap();
    await expect(addForm(page).getByText(category.name)).toBeVisible();
    await addForm(page).getByRole('button', { name: 'Cancel', exact: true }).first().tap();

    // The breadcrumb is the way back to the list.
    await pageHeader(page).getByRole('link', { name: 'Categories', exact: true }).tap();
    await expect(page).toHaveURL('/app/ledger/categories');
  });

  for (const theme of ['light', 'dark'] as const) {
    test(`keyboard help passes axe and fits in ${theme}`, async ({ page }) => {
      await readyForLedger(page);
      await page.evaluate((value) => localStorage.setItem('rekenraam-theme', value), theme);
      await page.goto('/app/ledger/transactions');
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
      await page.locator('#main-content').focus();
      await page.keyboard.press('?');
      const help = page.getByRole('dialog', { name: 'Keyboard shortcuts' });
      await expect(help).toBeVisible();
      await expectNoAccessibilityViolations(page, `keyboard help ${theme}`);
      expect(await help.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
      await help.getByRole('button', { name: 'Close', exact: true }).tap();
      await expect(help).toHaveCount(0);
    });
  }
});

test('N and ? work from the keyboard and never while typing', async ({ page }) => {
  const { account } = await seed(page);
  await page.setViewportSize({ width: 1280, height: 900 });

  // N opens the page's own editor, prefilled for the register.
  await page.goto(`/app/ledger/accounts/${account.id}/register`);
  await expect(pageHeader(page).getByRole('heading', { level: 1, name: account.name })).toBeVisible();
  await page.locator('#main-content').focus();
  await page.keyboard.press('n');
  const form = addForm(page);
  await expect(form.getByLabel('Account').locator('option:checked')).toHaveText(account.name);
  await expect(page).toHaveURL(`/app/ledger/accounts/${account.id}/register`);

  // Typing in a field types; neither key opens anything.
  const description = form.getByLabel('Description');
  await description.fill('');
  await description.press('n');
  await description.press('?');
  await expect(description).toHaveValue('n?');
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(addForm(page)).toHaveCount(1);

  // N never replaces an open editor and its unsaved input.
  await page.locator('#main-content').focus();
  await page.keyboard.press('n');
  await expect(description).toHaveValue('n?');
  await form.getByRole('button', { name: 'Cancel', exact: true }).first().click();

  // ? lists the shortcuts; Escape closes and returns focus.
  await page.locator('#main-content').focus();
  await page.keyboard.press('?');
  const help = page.getByRole('dialog', { name: 'Keyboard shortcuts' });
  await expect(help).toBeVisible();
  for (const label of ['Search pages, accounts and commands', 'Add transaction', 'Show this list']) {
    await expect(help.getByText(label, { exact: true })).toBeVisible();
  }
  await expect(help.locator('kbd', { hasText: /^N$/ })).toBeVisible();
  // A shortcut pressed inside the help does not act behind it.
  await page.keyboard.press('n');
  await expect(addForm(page)).toHaveCount(0);
  await page.keyboard.press('Escape');
  await expect(help).toHaveCount(0);
  await expect(page.locator('#main-content')).toBeFocused();

  // Elsewhere N opens the shared entry dialog in place (#477).
  await page.goto('/app');
  await page.locator('#main-content').focus();
  await page.keyboard.press('n');
  await expect(page.getByRole('dialog', { name: 'Add transaction', exact: true })).toBeVisible();
  await expect(addForm(page)).toBeVisible();
  await expect(page).toHaveURL((url) => url.pathname === '/app' && !url.searchParams.has('create'));
});

test('page widths narrow from the shared left edge', async ({ page }) => {
  await readyForLedger(page);
  await page.setViewportSize({ width: 1700, height: 900 });
  const column = page.locator('main [data-page-width]');

  await page.goto('/app/ledger/transactions');
  await expect(column).toHaveAttribute('data-page-width', 'fluid');
  const fluid = (await column.boundingBox())!;
  await page.goto('/app/reports/spending');
  await expect(column).toHaveAttribute('data-page-width', 'standard');
  const standard = (await column.boundingBox())!;

  expect(standard.width).toBeLessThanOrEqual(1200);
  expect(fluid.width).toBeGreaterThan(1400);
  expect(standard.x).toBe(fluid.x);
  const home = await page.getByRole('navigation', { name: 'Sections', exact: true }).getByRole('link', { name: 'Home', exact: true })
    .evaluate((element) => {
      const range = document.createRange();
      range.selectNodeContents(element);
      return range.getBoundingClientRect().left;
    });
  expect(Math.abs(home - standard.x)).toBeLessThanOrEqual(1);
});
