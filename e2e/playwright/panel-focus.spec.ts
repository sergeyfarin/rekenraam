import { expect, test, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { expectFocusTrapped } from './support/a11y';
import { createCashAccount, readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * #342: below `lg` the transaction workspace panel and the investments lot
 * panel cover the page, so they follow the #327 modal contract: focus moves
 * inside, Tab and Shift+Tab stay inside, Escape closes unless a write is in
 * flight, and focus returns to the row that opened them. From `lg` up they
 * are side columns and the list beside them stays operable.
 */

test.afterEach(async ({ page }) => {
  await page.unrouteAll({ behavior: 'ignoreErrors' });
});

function ago(days: number) { const date = new Date(); date.setDate(date.getDate() - days); return todayISO(date); }

/** A cash account with two spends; returns its register path and the descriptions. */
async function seedRegister(page: Page, label: string) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `${label} checking ${suffix}`, currencyID);
  const category = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: `${label} groceries ${suffix}`, category_type: 'expense'
  });
  const descriptions = [`${label} first ${suffix}`, `${label} second ${suffix}`];
  for (const description of descriptions) {
    await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
      transaction_date: todayISO(), status: 'posted', description,
      journal_entries: [{ entry_date: todayISO(), postings: [
        { account_id: account.id, commodity_id: currencyID, quantity_value: '-1000', quantity_scale: 2 },
        { account_id: category.id, commodity_id: currencyID, quantity_value: '1000', quantity_scale: 2 }
      ] }]
    });
  }
  return { path: `/app/ledger/accounts/${account.id}/register`, descriptions };
}

/** A holding with one bought lot; returns the instrument's display name. */
async function seedPosition(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = `pf${Date.now()}`;
  const name = `Panel focus ${suffix}`;
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, { name: `${name} cash`, account_class: 'asset', account_kind: 'brokerage_cash', default_commodity_id: currencyID, allows_postings: true, opened_on: ago(30), effective_from: ago(30) });
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, { commodity_code: suffix.toUpperCase(), instrument_type: 'stock', display_name: name, symbol: suffix.toUpperCase(), quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 3, price_scale: 2, effective_from: ago(30) });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, { instrument_id: instrument.id, name: `${name} holding`, opened_on: ago(30), effective_from: ago(30) });
  await apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, { transaction_date: ago(10), commodity_id: instrument.commodity_id, holding_account_id: holding.id, cash_account_id: cash.id, quantity_value: '1', quantity_scale: 0, cash_amount_value: '1000', cash_amount_scale: 2, cash_commodity_id: currencyID, cost_basis_method: 'fifo' });
  return name;
}

test.describe('at phone width', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('the transaction panel takes, keeps and returns focus', async ({ page }) => {
    const s = await seedRegister(page, 'Phone panel');
    await page.goto(s.path);
    const row = page.getByRole('row').filter({ hasText: s.descriptions[0] });
    await row.focus();
    await page.keyboard.press('Enter');

    const panel = page.getByRole('complementary', { name: 'Actions' });
    await expect(panel.getByRole('button', { name: 'Edit', exact: true })).toBeVisible();
    await expectFocusTrapped(page, panel, 'transaction panel', 25);
    expect(await page.getByRole('button', { name: 'New transaction' }).evaluate((el) => el.closest('[inert]') !== null),
      'the covered page is inert').toBe(true);
    await page.keyboard.press('Escape');
    await expect(panel).toHaveCount(0);
    await expect(row).toBeFocused();

    // A save in flight keeps the panel open on Escape.
    await page.keyboard.press('Enter');
    await panel.getByRole('button', { name: 'Edit', exact: true }).click();
    let release!: () => void;
    const held = new Promise<void>((resolve) => { release = resolve; });
    await page.route('**/api/v1/transactions/*', async (route) => {
      if (route.request().method() === 'PATCH') await held;
      await route.continue();
    });
    await panel.getByLabel('Description').fill(`${s.descriptions[0]} edited`);
    await panel.getByRole('button', { name: 'Save transaction' }).click();
    await expect(panel.getByRole('button', { name: 'Saving...' })).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(panel).toBeVisible();
    release();
    await expect(panel.getByRole('button', { name: 'Edit', exact: true })).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(panel).toHaveCount(0);
  });

  test('the lot panel takes, keeps and returns focus', async ({ page }) => {
    const name = await seedPosition(page);
    await page.goto('/app/investments');
    const row = page.getByRole('button').filter({ hasText: name });
    await row.focus();
    await page.keyboard.press('Enter');

    const panel = page.getByRole('complementary', { name: 'Lot detail' });
    await expect(panel.getByRole('button', { name: 'Close lot detail' })).toBeVisible();
    await expectFocusTrapped(page, panel, 'lot panel');
    await page.keyboard.press('Escape');
    await expect(panel).toHaveCount(0);
    await expect(row).toBeFocused();

    // The close button returns focus too.
    await page.keyboard.press('Enter');
    await expect.poll(() => panel.evaluate((el) => el.contains(document.activeElement))).toBe(true);
    await panel.getByRole('button', { name: 'Close lot detail' }).click();
    await expect(panel).toHaveCount(0);
    await expect(row).toBeFocused();
  });
});

test.describe('at desktop width', () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test('the list beside an open panel stays operable', async ({ page }) => {
    const s = await seedRegister(page, 'Desk panel');
    await page.goto(s.path);
    const first = page.getByRole('row').filter({ hasText: s.descriptions[0] });
    const second = page.getByRole('row').filter({ hasText: s.descriptions[1] });
    await first.focus();
    await page.keyboard.press('Enter');
    const panel = page.getByRole('complementary', { name: 'Actions' });
    await expect(panel.getByRole('heading', { name: s.descriptions[0] })).toBeVisible();

    // Nothing is inert, focus stays on the list, and Escape leaves the column alone.
    expect(await page.locator('[inert]').count(), 'no inert page').toBe(0);
    await expect(first).toBeFocused();
    await page.keyboard.press('Escape');
    await expect(panel).toBeVisible();

    // The list keeps working: another row opens in the same column.
    await second.focus();
    await page.keyboard.press('Enter');
    await expect(panel.getByRole('heading', { name: s.descriptions[1] })).toBeVisible();

    // Tab leaves the column for the page instead of wrapping.
    await panel.getByRole('button', { name: 'Close' }).first().focus();
    let left = false;
    for (let press = 0; press < 40 && !left; press++) {
      await page.keyboard.press('Tab');
      left = !(await panel.evaluate((el) => el.contains(document.activeElement)));
    }
    expect(left, 'Tab can leave the side column').toBe(true);

    // The lot column is a plain column too.
    const name = await seedPosition(page);
    await page.goto('/app/investments');
    await page.getByRole('button').filter({ hasText: name }).click();
    await expect(page.getByRole('complementary', { name: 'Lot detail' })).toBeVisible();
    expect(await page.locator('[inert]').count(), 'no inert page beside the lot column').toBe(0);
    await page.getByRole('navigation', { name: 'Sections' }).getByRole('link', { name: 'Ledger', exact: true }).click();
    await expect(page).toHaveURL(/\/app\/ledger$/);
  });
});
