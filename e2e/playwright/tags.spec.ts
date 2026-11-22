import { apiJSON } from './support/api';
import { expectNoAccessibilityViolations } from './support/a11y';
import { createCashAccount, readyForLedger } from './support/ledger';
import { expect, test } from './support/owned-book';

test('find, rename, archive and restore a tag from Categories on a phone', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 768 });
  const { csrfToken, currencyID } = await readyForLedger(page);
  const { owner_today: day } = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
  const cash = await createCashAccount(page, csrfToken, 'Tags checking', currencyID);
  const expense = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: 'Tags expense', category_type: 'expense'
  });
  const trip = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/tags', csrfToken, { name: 'Harbour trip', kind: 'project' });
  await apiJSON(page, 'POST', '/api/v1/tags', csrfToken, { name: 'Station trip', kind: 'project' });
  for (const [description, tagID] of [['Harbour ferry fare', trip.id], ['Station tickets', undefined]] as const) {
    await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
      description, transaction_date: day, tag_ids: tagID ? [tagID] : [],
      journal_entries: [{ entry_date: day, postings: [
        { account_id: cash.id, quantity_value: '-1000', quantity_scale: 2, commodity_id: currencyID },
        { account_id: expense.id, quantity_value: '1000', quantity_scale: 2, commodity_id: currencyID }
      ] }]
    });
  }

  await page.goto('/app/ledger');
  await page.getByRole('navigation', { name: 'Ledger pages' }).getByRole('link', { name: 'Categories' }).click();
  const views = page.getByRole('navigation', { name: 'Category views' });
  await expect(views.getByRole('link', { name: 'Categories' })).toHaveAttribute('aria-current', 'page');
  await views.getByRole('link', { name: 'Tags' }).click();
  await expect(views.getByRole('link', { name: 'Tags' })).toHaveAttribute('aria-current', 'page');

  const main = page.getByRole('main');
  const tagLink = (name: string) => main.getByRole('link', { name, exact: true });
  const status = main.getByRole('combobox', { name: 'Status' });
  const search = main.getByRole('searchbox', { name: 'Search tags' });
  await expect(tagLink('Station trip')).toBeVisible();
  await search.fill('harbour');
  await expect(tagLink('Station trip')).toHaveCount(0);
  await views.getByRole('link', { name: 'Tags' }).click();
  await expect(search).toHaveValue('');
  await expect(tagLink('Station trip')).toBeVisible();
  await search.fill('harbour');
  await expect(tagLink('Station trip')).toHaveCount(0);
  const tagRow = main.getByRole('listitem').filter({ has: page.getByRole('link', { name: 'Harbour trip', exact: true }) });
  await expect(tagRow.getByRole('definition')).toHaveText('1');
  await expectNoAccessibilityViolations(page, 'Tags view on a phone');

  const dialog = page.getByRole('dialog');
  const name = dialog.getByRole('textbox', { name: 'Tag name' });
  await tagRow.getByRole('button', { name: 'Rename Harbour trip' }).click();
  await expect(name).toBeFocused();
  await name.fill('Station trip');
  await dialog.getByRole('button', { name: 'Save name' }).click();
  await expect(dialog.getByRole('alert')).toBeVisible();
  await expect(name).toHaveValue('Station trip');
  await name.fill('Harbour holiday');
  await dialog.getByRole('button', { name: 'Save name' }).click();
  await expect(tagLink('Harbour holiday')).toBeVisible();

  await main.getByRole('button', { name: 'Archive Harbour holiday' }).click();
  await expect(dialog.getByRole('button', { name: 'Cancel' })).toBeFocused();
  await dialog.getByRole('button', { name: 'Archive', exact: true }).click();
  await expect(main.getByText('No tags match these filters.')).toBeVisible();
  await expect(search).toBeFocused();
  const suggestions = await apiJSON<{ tags: { id: number }[] }>(page, 'GET', '/api/v1/tags?q=Harbour');
  expect(suggestions.tags).toEqual([]);

  await status.selectOption('archived');
  await tagLink('Harbour holiday').click();
  await expect(page).toHaveURL(`/app/ledger/transactions?tag_id=${trip.id}`);
  await expect(main.getByText('Tag: Harbour holiday')).toBeVisible();
  const taggedTransaction = main.getByRole('row').filter({ hasText: 'Harbour ferry fare' });
  const untaggedTransaction = main.getByRole('row').filter({ hasText: 'Station tickets' });
  await expect(taggedTransaction).toBeVisible();
  await expect(untaggedTransaction).toHaveCount(0);
  await page.goBack();
  await expect(status).toHaveValue('archived');
  await expect(search).toHaveValue('harbour');
  await expect(tagLink('Harbour holiday')).toBeVisible();

  await status.selectOption('all');
  await main.getByRole('button', { name: 'Restore Harbour holiday' }).click();
  await dialog.getByRole('button', { name: 'Restore', exact: true }).click();
  await expect(main.getByRole('button', { name: 'Archive Harbour holiday' })).toBeVisible();
  await tagLink('Harbour holiday').click();
  await expect(taggedTransaction).toBeVisible();
  await main.getByRole('button', { name: 'Clear tag filter' }).click();
  await expect(page).toHaveURL('/app/ledger/transactions');
  await expect(untaggedTransaction).toBeVisible();
});

test('load the remaining tags and keep long names inside a dark phone layout', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 768 });
  const { csrfToken } = await readyForLedger(page);
  for (let index = 0; index < 50; index++) {
    await apiJSON(page, 'POST', '/api/v1/tags', csrfToken, { name: `Tag ${String(index).padStart(2, '0')}` });
  }
  const longName = 'Z'.repeat(120);
  await apiJSON(page, 'POST', '/api/v1/tags', csrfToken, { name: longName });
  await page.evaluate(() => {
    localStorage.setItem('rekenraam-theme', 'dark');
    localStorage.setItem('PARAGLIDE_LOCALE', 'nl');
  });
  await page.goto('/app/ledger/categories?view=tags');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  const main = page.getByRole('main');
  await expect(main.getByRole('searchbox', { name: 'Zoek labels' })).toBeVisible();
  const lastTag = main.getByRole('link', { name: longName });
  await expect(lastTag).toHaveCount(0);
  await main.getByRole('button', { name: 'Meer laden' }).click();
  await expect(lastTag).toBeVisible();
  await expect(main.getByRole('listitem')).toHaveCount(51);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(320);
  await expectNoAccessibilityViolations(page, 'Tags with long names in dark mode on a phone');
});

test('the Tags view uses the selected language in every shipped locale', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 768 });
  const { csrfToken } = await readyForLedger(page);
  await apiJSON(page, 'POST', '/api/v1/tags', csrfToken, { name: 'Travel' });

  const localeSearchLabels = {
    en: 'Search tags',
    nl: 'Zoek labels',
    ru: 'Поиск меток'
  };
  for (const [locale, label] of Object.entries(localeSearchLabels)) {
    await test.step(locale, async () => {
      await page.evaluate((value) => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
      await page.goto('/app/ledger/categories?view=tags');
      await expect(page.locator('html')).toHaveAttribute('lang', locale);
      await expect(page.getByRole('searchbox', { name: label })).toBeVisible();
      await expect(page.getByRole('main').getByRole('link', { name: 'Travel', exact: true })).toBeVisible();
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(320);
    });
  }
});
