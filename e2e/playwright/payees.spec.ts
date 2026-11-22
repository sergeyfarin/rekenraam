import { apiJSON } from './support/api';
import { expectNoAccessibilityViolations } from './support/a11y';
import { createCashAccount, readyForLedger } from './support/ledger';
import { expect, test } from './support/owned-book';

test('manage payees on a phone and open their transactions from the directory and shared search', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const { csrfToken, currencyID } = await readyForLedger(page);
  const { owner_today: day } = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
  const cash = await createCashAccount(page, csrfToken, 'Payees checking', currencyID);
  const expense = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: 'Payees expense', category_type: 'expense'
  });
  const cafe = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/payees', csrfToken, { name: 'Harbour Cafe' });
  const market = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/payees', csrfToken, { name: 'Station Market' });
  for (const payee of [cafe, market]) {
    await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
      payee_id: payee.id, transaction_date: day,
      journal_entries: [{ entry_date: day, postings: [
        { account_id: cash.id, quantity_value: '-1000', quantity_scale: 2, commodity_id: currencyID },
        { account_id: expense.id, quantity_value: '1000', quantity_scale: 2, commodity_id: currencyID }
      ] }]
    });
  }

  await page.goto('/app/ledger');
  await page.getByRole('navigation', { name: 'Ledger pages' }).getByRole('link', { name: 'Payees' }).click();
  const main = page.getByRole('main');
  const filter = main.getByRole('searchbox', { name: 'Search payees' });
  const status = main.getByRole('combobox', { name: 'Status' });
  await expect(main.getByRole('heading', { name: 'Payees', exact: true })).toBeVisible();
  await expect(main.getByRole('link', { name: 'Station Market' })).toBeVisible();
  await filter.fill('harbour');
  await expect(main.getByRole('link', { name: 'Station Market' })).toHaveCount(0);
  const cafeRow = main.getByRole('listitem').filter({ has: page.getByRole('link', { name: 'Harbour Cafe' }) });
  await expect(cafeRow.getByRole('definition').first()).toHaveText('1');
  await expectNoAccessibilityViolations(page, 'Payees directory on a phone');
  await cafeRow.getByRole('button', { name: 'Rename Harbour Cafe' }).click();
  const dialog = page.getByRole('dialog');
  const name = dialog.getByRole('textbox', { name: 'Payee name' });
  await expect(name).toBeFocused();
  await name.fill('Station Market');
  await dialog.getByRole('button', { name: 'Save name' }).click();
  await expect(dialog.getByRole('alert')).toBeVisible();
  await expect(name).toHaveValue('Station Market');
  await name.fill('Harbour Coffee');
  await dialog.getByRole('button', { name: 'Save name' }).click();
  await expect(main.getByRole('link', { name: 'Harbour Coffee' })).toBeVisible();

  await main.getByRole('button', { name: 'Archive Harbour Coffee' }).click();
  await dialog.getByRole('button', { name: 'Archive', exact: true }).click();
  await expect(main.getByText('No payees match these filters.')).toBeVisible();
  await expect(filter).toBeFocused();
  const activeSuggestions = await apiJSON<{ payees: { id: number }[] }>(page, 'GET', '/api/v1/payees?q=Harbour');
  expect(activeSuggestions.payees).toEqual([]);
  await status.selectOption('archived');
  await main.getByRole('link', { name: 'Harbour Coffee' }).click();
  await expect(page).toHaveURL(`/app/ledger/transactions?payee_id=${cafe.id}`);
  await expect(main.getByRole('row').filter({ hasText: 'Harbour Cafe' })).toBeVisible();
  await expect(main.getByRole('row').filter({ hasText: 'Station Market' })).toHaveCount(0);

  // The global result changes the URL and list in a page that is already mounted.
  await page.getByRole('button', { name: 'Search', exact: true }).click();
  await dialog.getByRole('button', { name: 'Payees', exact: true }).click();
  await dialog.getByRole('combobox').fill('Station Market');
  await dialog.getByRole('option', { name: 'Station Market' }).click();
  await expect(page).toHaveURL(`/app/ledger/transactions?payee_id=${market.id}`);
  await expect(main.getByRole('row').filter({ hasText: 'Station Market' })).toBeVisible();
  await expect(main.getByRole('row').filter({ hasText: 'Harbour Cafe' })).toHaveCount(0);
  await main.getByRole('button', { name: 'Clear payee filter' }).click();
  await expect(page).toHaveURL('/app/ledger/transactions');
  await expect(main.getByRole('row').filter({ hasText: 'Harbour Cafe' })).toBeVisible();
  await page.goBack();
  await expect(page).toHaveURL(`/app/ledger/transactions?payee_id=${cafe.id}`);
  await page.goBack();
  await expect(page).toHaveURL((url) => url.pathname === '/app/ledger/payees'
    && url.searchParams.get('status') === 'archived' && url.searchParams.get('q') === 'harbour');
  await expect(status).toHaveValue('archived');
  await expect(filter).toHaveValue('harbour');

  await status.selectOption('all');
  await main.getByRole('button', { name: 'Restore Harbour Coffee' }).click();
  await dialog.getByRole('button', { name: 'Restore', exact: true }).click();
  await expect(main.getByRole('button', { name: 'Archive Harbour Coffee' })).toBeVisible();
  await status.selectOption('active');
  await expect(main.getByRole('link', { name: 'Harbour Coffee' })).toBeVisible();
  const restoredSuggestions = await apiJSON<{ payees: { id: number }[] }>(page, 'GET', '/api/v1/payees?q=Harbour');
  expect(restoredSuggestions.payees.map((payee) => payee.id)).toEqual([cafe.id]);
});

test('the payee directory restores filters and pagination on a small phone in dark mode', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 768 });
  const { csrfToken } = await readyForLedger(page);
  const longName = 'V'.repeat(180);
  for (let i = 0; i < 100; i++) {
    await apiJSON(page, 'POST', '/api/v1/payees', csrfToken, { name: `Vendor ${String(i).padStart(2, '0')}` });
  }
  await apiJSON(page, 'POST', '/api/v1/payees', csrfToken, { name: longName });
  await page.evaluate(() => {
    localStorage.setItem('rekenraam-theme', 'dark');
    localStorage.setItem('PARAGLIDE_LOCALE', 'nl');
  });
  await page.goto('/app/ledger/payees?highlight=kept');
  const main = page.getByRole('main');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await expect(main.getByRole('heading', { name: 'Begunstigden' })).toBeVisible();
  await expect(main.getByRole('link')).toHaveCount(50);
  const status = main.getByRole('combobox', { name: 'Status' });
  const filter = main.getByRole('searchbox', { name: 'Zoek begunstigden' });
  const historyLength = await page.evaluate(() => history.length);
  await status.selectOption('all');
  await filter.fill('v');
  await expect(page).toHaveURL((url) => url.searchParams.get('status') === 'all' && url.searchParams.get('q') === 'v');
  const next = main.getByRole('button', { name: 'Volgende pagina' });
  const last = main.getByRole('link', { name: longName });
  const middle = main.getByRole('link', { name: 'Vendor 50', exact: true });
  const first = main.getByRole('link', { name: 'Vendor 00', exact: true });
  async function openLastPage() {
    await next.click();
    await expect(middle).toBeVisible();
    await next.click();
    await expect(last).toBeVisible();
    await expect(main.getByRole('link')).toHaveCount(1);
  }
  await openLastPage();
  await expectNoAccessibilityViolations(page, 'Long payee on a small phone in dark mode');
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(320);
  const selectedPage = page.url();
  const params = new URL(selectedPage).searchParams;
  expect(params.get('cursor')).toBeTruthy();
  expect(params.get('highlight')).toBe('kept');
  expect(await page.evaluate(() => history.length)).toBe(historyLength);
  await last.click();
  await expect(page).toHaveURL(/\/app\/ledger\/transactions\?payee_id=/);
  await page.goBack();
  await expect(page).toHaveURL(selectedPage);
  await expect(status).toHaveValue('all');
  await expect(filter).toHaveValue('v');
  await expect(last).toBeVisible();
  await main.getByRole('button', { name: 'Vorige pagina' }).click();
  await expect(middle).toBeVisible();

  await status.selectOption('active');
  await expect(page).toHaveURL((url) => !url.searchParams.has('cursor') && !url.searchParams.has('status'));
  await expect(first).toBeVisible();
  await openLastPage();
  await filter.fill('vendor');
  await expect(page).toHaveURL((url) => url.searchParams.get('q') === 'vendor' && !url.searchParams.has('cursor'));
  await expect(first).toBeVisible();

  await filter.fill('v');
  await expect(page).toHaveURL((url) => url.searchParams.get('q') === 'v' && !url.searchParams.has('cursor'));
  await openLastPage();
  const emptyPageCursor = new URL(page.url()).searchParams.get('cursor');
  await main.getByRole('button', { name: `Archiveren · ${longName}` }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Archiveren', exact: true }).click();
  await expect(middle).toBeVisible();
  await expect(page).toHaveURL((url) => url.searchParams.has('cursor')
    && url.searchParams.get('cursor') !== emptyPageCursor && url.searchParams.get('q') === 'v');
});
