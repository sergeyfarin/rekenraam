import { test, expect } from './support/owned-book';
import { apiJSON, csrfTokenFor } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';
import { expectNoAccessibilityViolations } from './support/a11y';
import type { HomeLayoutState } from '../../frontend/src/lib/api/home';

test('keyboard reorders Home, restores focus, and adds or removes widgets with Undo', async ({ page }) => {
  await readyForLedger(page);
  await page.goto('/app');
  const widgets = page.getByTestId('home-widgets');
  const budgets = page.getByTestId('home-widget-budgets');
  const budgetActions = budgets.getByRole('button', { name: 'Actions for Budgets' });
  await budgetActions.press('Enter');
  await budgets.getByRole('button', { name: 'Move up' }).press('Enter');
  await expect(widgets.locator('article').first()).toHaveAttribute('id', 'home-widget-budgets');
  await expect(budgetActions).toBeFocused();
  await expect(page.getByText(/^Budgets.* moved to position 1 of 5\.$/)).toHaveAttribute('aria-live', 'polite');

  await page.reload();
  await expect(widgets.locator('article').first()).toHaveAttribute('id', 'home-widget-budgets');
  await budgetActions.press('Enter');
  await budgets.getByRole('button', { name: 'Remove' }).press('Enter');
  const preview = page.getByTestId('explore-widget-budgets');
  await expect(preview).toBeVisible();
  await expect(budgets).toHaveCount(0);
  await page.getByRole('button', { name: 'Undo', exact: true }).click();
  await expect(widgets.locator('article').first()).toHaveAttribute('id', 'home-widget-budgets');

  const cashflow = page.getByTestId('explore-widget-cashflow');
  await cashflow.getByRole('button', { name: 'Add to My Home' }).press('Enter');
  await expect(page.getByTestId('home-widget-cashflow').getByRole('button', { name: /^Actions for Cashflow/ })).toBeFocused();
  await expect(cashflow).toHaveCount(0);

  const activeCashflow = page.getByTestId('home-widget-cashflow');
  const initialHref = await activeCashflow.getByRole('link', { name: /Open cashflow report/ }).getAttribute('href');
  await activeCashflow.getByRole('button', { name: /^Actions for Cashflow/ }).press('Enter');
  await activeCashflow.getByRole('button', { name: 'Settings' }).press('Enter');
  const cashflowSettings = page.getByRole('dialog', { name: 'Cashflow', exact: true });
  await cashflowSettings.getByLabel('Size').selectOption('wide');
  await cashflowSettings.getByLabel('Period').selectOption('last_month');
  await cashflowSettings.getByRole('button', { name: 'Save layout' }).click();
  await expect(cashflowSettings).toHaveCount(0);
  await expect(activeCashflow).toHaveClass(/md:col-span-2/);
  await expect(activeCashflow.getByRole('link', { name: /Open cashflow report/ })).not.toHaveAttribute('href', initialHref!);
});

test('Home and phone More open Customize, which saves a named headline and the selected default register', async ({ page }) => {
  const { currencyID } = await readyForLedger(page);
  const checking = await createCashAccount(page, await csrfTokenFor(page), 'Layout Checking', currencyID);
  await createCashAccount(page, await csrfTokenFor(page), 'Layout Savings', currencyID);
  await page.setViewportSize({ width: 320, height: 760 });
  await page.goto('/app');
  const headline = page.getByTestId('home-headline');
  await headline.getByRole('button').click();
  await headline.locator('..').getByRole('button', { name: 'Customize Home' }).click();
  const panel = page.getByRole('dialog', { name: 'Customize Home' });
  await expect(panel.getByLabel('Headline')).toBeVisible();
  await panel.getByRole('button', { name: 'Close' }).click();

  const tiles = page.getByRole('list', { name: 'Shortcuts' });
  const importActions = tiles.getByRole('button', { name: 'Actions for Import file' });
  await importActions.press('Enter');
  await tiles.getByRole('button', { name: 'Move right' }).press('Enter');
  const more = tiles.getByRole('group', { name: 'More', exact: true });
  const movedImport = more.getByRole('button', { name: 'Actions for Import file' });
  await expect(movedImport).toBeFocused();
  await movedImport.press('Enter');
  await movedImport.press('Escape');
  await expect(more).toBeVisible();
  await expect(movedImport).toBeFocused();
  await movedImport.press('Escape');
  await expect(tiles.getByRole('button', { name: 'More', exact: true })).toBeFocused();

  await tiles.getByRole('button', { name: 'More', exact: true }).click();
  await tiles.getByRole('button', { name: 'Customize Home' }).click();
  await panel.getByLabel('Headline').selectOption('account_set');
  await panel.getByLabel('Group name').fill('Everyday accounts');
  await panel.getByRole('checkbox', { name: 'Layout Checking' }).check();
  await panel.getByRole('checkbox', { name: 'Layout Savings' }).check();
  await panel.getByRole('combobox', { name: 'Add shortcut' }).selectOption('account');
  await panel.getByRole('combobox', { name: 'Choose an account' }).selectOption(String(checking.id));
  await panel.getByRole('button', { name: 'Add shortcut', exact: true }).click();
  await expectNoAccessibilityViolations(page, 'customize home at 320px');
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  await panel.getByRole('button', { name: 'Save layout' }).click();
  await expect(headline.getByRole('button')).toContainText('Everyday accounts');

  await page.goto('/app/settings/preferences');
  const section = page.getByRole('heading', { name: 'Home page', exact: true });
  await expect(section).toBeVisible();
  await page.getByRole('button', { name: 'Reset Home', exact: true }).click();
  await page.getByRole('dialog', { name: 'Reset Home' }).getByRole('button', { name: 'Reset Home' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await page.goto('/app');
  await expect(headline.getByRole('button')).toContainText('Net worth');
  await tiles.getByRole('button', { name: 'More', exact: true }).click();
  await expect(tiles.getByRole('link', { name: 'Layout Checking' })).toHaveAttribute('href', `/app/ledger/accounts/${checking.id}/register`);
});

test('Customize refuses a stale whole layout and reloads the other tab’s revision', async ({ page, context }) => {
  await readyForLedger(page);
  await page.goto('/app/settings/preferences');
  await page.getByRole('button', { name: 'Customize Home' }).click();
  const panel = page.getByRole('dialog', { name: 'Customize Home' });
  const headline = panel.getByLabel('Headline');
  await headline.selectOption('cash');

  const other = await context.newPage();
  await other.goto('/app');
  const latest = await apiJSON<HomeLayoutState>(other, 'GET', '/api/v1/home/layout');
  await apiJSON(other, 'PUT', '/api/v1/home/layout', await csrfTokenFor(other), {
    expected_revision: latest.revision,
    layout: { ...latest.layout, show_review: false, widgets: ['budgets'] }
  });

  await panel.getByRole('button', { name: 'Save layout' }).click();
  await expect(panel.getByRole('alert')).toContainText('Home changed in another tab');
  await expect(headline).toHaveValue('auto');
  await expect(panel.getByRole('checkbox', { name: 'Show review notes' })).not.toBeChecked();
  await expect(panel.getByRole('region', { name: 'My Home' }).getByRole('listitem')).toHaveCount(1);

  await headline.selectOption('cash');
  await panel.getByRole('button', { name: 'Save layout' }).click();
  await expect(panel).toHaveCount(0);
  const saved = await apiJSON<HomeLayoutState>(page, 'GET', '/api/v1/home/layout');
  expect(saved.layout).toMatchObject({ headline: 'cash', show_review: false, widgets: ['budgets'] });
  expect(saved.revision).toBe(latest.revision + 2);
  await other.close();
});

test('Customize can remove both saved headline scopes without resetting the layout', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const account = await createCashAccount(page, csrfToken, 'Headline Checking', currencyID);
  const initial = await apiJSON<HomeLayoutState>(page, 'GET', '/api/v1/home/layout');
  await apiJSON(page, 'PUT', '/api/v1/home/layout', csrfToken, {
    expected_revision: initial.revision,
    layout: {
      ...initial.layout,
      headline: 'account_set',
      account_id: account.id,
      account_set: { name: 'Saved group', account_ids: [account.id] },
      explore_open: false
    }
  });

  await page.goto('/app');
  await page.getByRole('button', { name: 'Customize Home' }).click();
  const panel = page.getByRole('dialog', { name: 'Customize Home' });
  const headline = panel.getByRole('combobox', { name: 'Headline', exact: true });
  await panel.getByLabel('Group name').clear();
  await expect(panel.getByRole('button', { name: 'Save layout' })).toBeDisabled();
  await headline.selectOption('cash');
  await panel.getByRole('button', { name: 'Remove account headline' }).click();
  await expect(headline).toBeFocused();
  await panel.getByRole('button', { name: 'Save layout' }).click();
  await expect(panel).toHaveCount(0);

  const saved = await apiJSON<HomeLayoutState>(page, 'GET', '/api/v1/home/layout');
  expect(saved.layout.account_id).toBeUndefined();
  expect(saved.layout.account_set).toBeUndefined();
  expect(saved.layout).toMatchObject({ headline: 'cash', explore_open: false });
  const currentHeadline = page.getByTestId('home-headline');
  await currentHeadline.getByRole('button').click();
  const choices = currentHeadline.locator('..');
  await expect(choices.getByRole('button', { name: /Saved group|Headline Checking/ })).toHaveCount(0);
});

test('Customize keeps focus on the move control through repeated keyboard moves', async ({ page }) => {
  await readyForLedger(page);
  await page.goto('/app');
  await page.getByRole('button', { name: 'Customize Home' }).click();
  const panel = page.getByRole('dialog', { name: 'Customize Home' });
  await expect(panel.getByRole('textbox', { name: 'Label for Transfer' })).toBeVisible();
  const widgets = panel.getByRole('region', { name: 'My Home' });
  const accounts = widgets.getByRole('listitem').filter({ hasText: 'Accounts' });
  const up = accounts.getByRole('button', { name: 'Move up' });
  const down = accounts.getByRole('button', { name: 'Move down' });
  await up.press('Enter');
  await expect(up).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(widgets.getByRole('listitem').first()).toContainText('Accounts');
  await expect(down).toBeFocused();
  await expect(panel.getByText('Accounts moved to position 1 of 5.')).toHaveAttribute('aria-live', 'polite');
  await panel.getByRole('button', { name: 'Save layout' }).click();
  await expect(page.getByTestId('home-widgets').locator('article').first()).toHaveAttribute('id', 'home-widget-accounts');
});

test('changing the Spending account scope reloads figures and the report link together', async ({ page }) => {
  const { currencyID } = await readyForLedger(page);
  await apiJSON(page, 'POST', `/api/v1/currencies/${currencyID}/default`, await csrfTokenFor(page), undefined, [200]);
  const csrfToken = await csrfTokenFor(page);
  const checking = await createCashAccount(page, csrfToken, 'Spending Checking', currencyID);
  const savings = await createCashAccount(page, csrfToken, 'Spending Savings', currencyID);
  const category = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: 'Scoped groceries', category_type: 'expense'
  });
  const { owner_today: date } = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
  for (const [account, cents] of [[checking.id, 12300], [savings.id, 45600]]) {
    await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
      status: 'posted', transaction_date: date, journal_entries: [{ entry_date: date, postings: [
        { account_id: account, commodity_id: currencyID, quantity_value: String(-cents), quantity_scale: 2 },
        { account_id: category.id, commodity_id: currencyID, quantity_value: String(cents), quantity_scale: 2 }
      ] }]
    });
  }

  await page.goto('/app');
  const spending = page.getByTestId('home-widget-spending');
  await expect(spending).toContainText('$579');
  await spending.getByRole('button', { name: /^Actions for Spending/ }).click();
  await spending.getByRole('button', { name: 'Settings' }).click();
  const panel = page.getByRole('dialog', { name: 'Spending this month', exact: true });
  await panel.getByRole('checkbox', { name: checking.name }).check();
  const reread = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/home' && response.ok());
  await panel.getByRole('button', { name: 'Save layout' }).click();
  await reread;
  await expect(panel).toHaveCount(0);
  await expect(spending).toContainText('$123');
  await expect(spending).toContainText('Accounts selected: 1');
  await expect(spending).not.toContainText('$579');
  const url = new URL((await spending.getByRole('link', { name: /Open spending report/ }).getAttribute('href'))!, page.url());
  expect(url.searchParams.get('account_id')).toBe(String(checking.id));
  expect(url.searchParams.get('include_descendants')).toBe('true');
});
