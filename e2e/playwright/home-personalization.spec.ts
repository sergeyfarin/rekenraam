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
