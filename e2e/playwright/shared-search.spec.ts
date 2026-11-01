import { expect, test } from './support/fixtures';
import { readyForLedger } from './support/ledger';
import { expectNoAccessibilityViolations, expectFocusTrapped } from './support/a11y';
import { seedReferenceBook } from './support/reference-book';
import { observePageRequests, waitForPageReady } from './support/page-readiness';

const search = (page: import('@playwright/test').Page) => page.getByRole('combobox', { name: 'Search', exact: true });

for (const width of [1280, 390]) {
  test(`search entered during Spending range repair survives completion at ${width}px (#472)`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    observePageRequests(page);
    const { ids, suffix } = await seedReferenceBook(page);
    await page.goto('/app');
    // Home places search as its command bar; on phones it is a field-like trigger.
    if (width === 390) await expect(page.getByRole('button', { name: 'Search pages, accounts, commands…', exact: true })).toBeVisible();
    else await expect(search(page)).toBeVisible();
    await page.keyboard.press('Control+k');
    await expect(search(page)).toBeFocused();
    await search(page).fill('Spending');
    await expect(page.getByRole('option', { name: 'Spending Reports', exact: true })).toBeVisible();
    const needle = `Reference Corp${suffix}`;

    // Spending has rendered before its same-page default range is committed.
    // Enter search at that history checkpoint, before completion callbacks,
    // without sleeps, production test hooks or a copy of the implementation.
    await page.evaluate(needle => {
      const original = history.replaceState;
      history.replaceState = function (state, unused, url) {
        const target = new URL(String(url), location.href);
        const result = original.call(this, state, unused, url);
        if (target.pathname === '/app/reports/spending' &&
          target.searchParams.has('start_date')) {
          history.replaceState = original;
          window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true, bubbles: true }));
          // Let the same rendering microtask mount the phone dialog's input.
          queueMicrotask(() => {
            const input = document.querySelector<HTMLInputElement>('input[role="combobox"][aria-label="Search"]')!;
            input.focus();
            input.value = needle;
            input.dispatchEvent(new Event('input', { bubbles: true }));
            document.documentElement.dataset.searchCheckpoint = input.value;
          });
        }
        return result;
      };
    }, needle);

    await page.keyboard.press('Enter');
    await expect(page.locator('html')).toHaveAttribute('data-search-checkpoint', needle);
    await expect(page).toHaveURL(url => url.pathname === '/app/reports/spending' && url.searchParams.has('start_date'));
    await waitForPageReady(page);
    await expect(search(page)).toHaveValue(needle);
    await expect(search(page)).toHaveAttribute('aria-expanded', 'true');
    await expect(page.getByRole('option').filter({ hasText: needle })).toBeVisible();

    await page.keyboard.press('Enter');
    await expect(page).toHaveURL(`/app/investments/instruments/${ids.get('REF')}`);
    await waitForPageReady(page);
    if (width === 390) await expect(page.getByRole('dialog', { name: 'Search', exact: true })).toHaveCount(0);
    else await expect(search(page)).toHaveAttribute('aria-expanded', 'false');
    await page.keyboard.press('Control+k');
    await expect(search(page)).toHaveValue('');
  });
}

test('shared search navigates pages and reference objects with keyboard and scoped localized matching', async ({ page }) => {
  // Keep the journey meaningful on a populated book even when run alone:
  // another reference book must not defeat this run's unique suffix (#468/#472).
  await seedReferenceBook(page);
  const { ids, suffix } = await seedReferenceBook(page);
  await page.goto('/app');
  const requests: string[] = [];
  page.on('request', request => { if (request.url().includes('/api/v1/search/')) requests.push(request.url()); });
  let readStart = 0;
  const expectCatalogWalk = async () => {
    await expect(page.getByRole('status').filter({ hasText: /Showing/ })).toBeVisible();
    const walk = requests.slice(readStart).map(url => new URL(url));
    expect(walk.length).toBeGreaterThan(0);
    expect(walk.every(url => url.pathname === '/api/v1/search/catalog')).toBe(true);
    const cursors = walk.map(url => url.searchParams.get('cursor'));
    expect(cursors[0]).toBeNull();
    // One composed cursor walk per opening, including on books over 500 items.
    // Repeated cursors or a per-result endpoint still fail this assertion.
    expect(new Set(cursors).size).toBe(cursors.length);
    readStart = requests.length;
  };
  await expect(search(page)).toBeVisible();
  await page.keyboard.press('/');
  await expect(search(page)).toBeFocused();
  await expect(search(page)).toHaveAttribute('aria-expanded', 'true');
  await expectCatalogWalk();
  await expectNoAccessibilityViolations(page, 'desktop search results');
  await search(page).fill('Spending');
  await page.keyboard.press('Enter');
  // The report normalizes a missing range; its bare URL is only transient.
  await expect(page).toHaveURL(url => url.pathname === '/app/reports/spending' && url.searchParams.has('start_date'));
  await page.keyboard.press('Control+k');
  await search(page).fill(`Reference Corp${suffix}`);
  await expect(page.getByRole('option').filter({ hasText: `Reference Corp${suffix}` })).toBeVisible();
  await expectCatalogWalk();
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(`/app/investments/instruments/${ids.get('REF')}`);
  await page.keyboard.press('Meta+k');
  await page.getByRole('button', { name: 'Accounts', exact: true }).click();
  await search(page).fill('Ref');
  await expect(page.getByRole('group', { name: 'Pages', exact: true })).toHaveCount(0);
  await expect(page.getByRole('group', { name: 'Accounts', exact: true })).toBeVisible();
  await expectCatalogWalk();
  await page.keyboard.press('ArrowDown');
  const activeID = await search(page).getAttribute('aria-activedescendant');
  await expect(page.locator(`[id="${activeID}"]`)).toHaveAttribute('aria-selected', 'true');
  await page.keyboard.press('Escape');
  await expect(search(page)).toHaveAttribute('aria-expanded', 'false');

  await page.evaluate(() => localStorage.setItem('PARAGLIDE_LOCALE', 'nl'));
  await page.reload();
  await expect(page.getByRole('combobox', { name: 'Zoeken', exact: true })).toBeVisible();
  await page.keyboard.press('/');
  const dutch = page.getByRole('combobox', { name: 'Zoeken', exact: true });
  await dutch.fill('Boodschappen');
  await expect(page.getByRole('option').filter({ hasText: 'Boodschappen' })).toBeVisible();
});

test('commands open an unsaved editor, shortcuts respect editable and modal fields', async ({ page }) => {
  await readyForLedger(page);
  await page.goto('/app/ledger/transactions');
  // Existing page filters remain independent of the global command field.
  const filter = page.getByRole('searchbox').first();
  await filter.fill('sample');
  await filter.press('/');
  await expect(filter).toHaveValue('sample/');
  await expect(page).toHaveURL(url => url.searchParams.get('q') === 'sample/');
  await expect(search(page)).toHaveAttribute('aria-expanded', 'false');
  await filter.press('Control+k');
  await expect(search(page)).toBeFocused();
  await search(page).fill('Add transaction');
  await page.keyboard.press('Enter');
  const heading = page.getByRole('heading', { name: 'Add transaction', exact: true });
  await expect(heading).toBeVisible();
  const form = page.locator('form').filter({ has: heading });
  await form.getByLabel('Amount', { exact: true }).fill('-12.34');
  await form.getByLabel('Description', { exact: true }).fill('/');
  await expect(search(page)).toHaveAttribute('aria-expanded', 'false');
  await expect(page).toHaveURL(url => url.pathname === '/app/ledger/transactions' && !url.searchParams.has('create') && url.searchParams.get('q') === 'sample/');
  await form.getByRole('button', { name: 'Cancel', exact: true }).last().click();
  await page.reload();
  await expect(heading).toHaveCount(0);
  await expect(search(page)).toBeVisible();
  await page.keyboard.press('/');
  await search(page).fill('Hide amounts');
  await page.keyboard.press('Enter');
  await expect(page.getByRole('button', { name: 'Hide amounts', exact: true })).toHaveAttribute('aria-pressed', 'true');
});

for (const theme of ['light', 'dark'] as const) {
  test(`phone search fills the screen, traps focus and passes axe in ${theme}`, async ({ page }) => {
    await readyForLedger(page);
    await page.evaluate(value => localStorage.setItem('rekenraam-theme', value), theme);
    await page.setViewportSize({ width: 390, height: 844 });
    // The header's compact trigger; Home's command bar is covered in home-entry.
    await page.goto('/app/ledger/accounts');
    const trigger = page.getByRole('button', { name: 'Search', exact: true });
    await trigger.click();
    const dialog = page.getByRole('dialog', { name: 'Search', exact: true });
    await expect(search(page)).toBeFocused();
    await expect(dialog).toBeVisible();
    await expect.poll(() => dialog.evaluate(node => Math.round(node.getBoundingClientRect().width))).toBe(390);
    await expect(page.getByRole('status').filter({ hasText: /Showing/ })).toBeVisible();
    await expectNoAccessibilityViolations(page, `phone search results ${theme}`);
    await search(page).fill('zzzznotfound');
    await expect(page.getByRole('status').filter({ hasText: 'No results' })).toBeVisible();
    await expectNoAccessibilityViolations(page, `phone search ${theme}`);
    await expectFocusTrapped(page, dialog, 'phone search');
    await page.keyboard.press('Escape');
    await expect(dialog).toHaveCount(0);
    await expect(trigger).toBeFocused();
    await page.keyboard.press('Control+k');
    await expect(search(page)).toBeFocused();
    await page.keyboard.press('Escape');
  });
}

test('search coverage failure remains explicit and retries without hiding pages or commands', async ({ page }) => {
  await readyForLedger(page);
  await page.route('**/api/v1/search/catalog*', route => route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: { code: 'INTERNAL_ERROR', message: 'synthetic' } }) }));
  await page.goto('/app');
  await expect(search(page)).toBeVisible();
  await page.keyboard.press('/');
  await expect(page.getByText('Some objects could not be loaded. Page links and commands are still available.')).toBeVisible();
  await expect(page.getByRole('alert')).toBeVisible();
  await expect(page.getByRole('option').filter({ hasText: 'Add transaction' })).toBeVisible();
  await page.unroute('**/api/v1/search/catalog*');
  await page.getByRole('button', { name: 'Retry', exact: true }).click();
  await expect(page.getByRole('status').filter({ hasText: /Showing/ })).toBeVisible();
  await expect(page.getByRole('alert')).toHaveCount(0);
});

test('search follows every catalog cursor before declaring complete results', async ({ page }) => {
  await readyForLedger(page);
  const cursors: Array<string | null> = [];
  await page.route('**/api/v1/search/catalog*', async route => {
    const cursor = new URL(route.request().url()).searchParams.get('cursor');
    cursors.push(cursor);
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(cursor ? {
      items: [{ kind: 'account', id: 900002, name: 'Café final reference', code: '', builtin_key: '', status: 'active', account_kind: 'checking' }]
    } : {
      items: [{ kind: 'account', id: 900001, name: 'Initial reference', code: '', builtin_key: '', status: 'active', account_kind: 'checking' }],
      next_cursor: 'account:900001'
    }) });
  });
  await page.goto('/app');
  await search(page).fill('cafe final');
  await expect(page.getByRole('option', { name: 'Café final reference', exact: true })).toBeVisible();
  await expect(page.getByRole('status').filter({ hasText: 'Showing 1 of 1 results' })).toBeVisible();
  expect(cursors).toEqual([null, 'account:900001']);
});

test('phone Add command hands focus to transaction entry', async ({ page }) => {
  await readyForLedger(page);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/app');
  await page.getByRole('button', { name: 'Search pages, accounts, commands…', exact: true }).click();
  await search(page).fill('Add transaction');
  await page.keyboard.press('Enter');
  // Home has no editor of its own: the command opens the shared entry sheet (#477).
  const panel = page.getByRole('dialog', { name: 'Add transaction', exact: true });
  await expect(page.getByRole('dialog', { name: 'Search', exact: true })).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'Add transaction', exact: true })).toBeVisible();
  await expect.poll(() => panel.evaluate(node => node.contains(document.activeElement))).toBe(true);
  await page.keyboard.press('Control+k');
  await expect(page.getByRole('dialog', { name: 'Search', exact: true })).toHaveCount(0);
  await page.keyboard.press('Escape');
  await expect(page.getByRole('heading', { name: 'Add transaction', exact: true })).toHaveCount(0);
});

test('merged Preferences is still searchable as language and appearance', async ({ page }) => {
  await readyForLedger(page);
  await page.goto('/app');
  const search = page.getByRole('combobox', { name: 'Search', exact: true });
  await expect(search).toBeVisible();
  await search.fill('language');
  const preferences = page.getByRole('option').filter({ hasText: 'Preferences' }).first();
  await expect(preferences).toBeVisible();
  await search.fill('appearance');
  await expect(preferences).toBeVisible();
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL('/app/settings/preferences');
  await expect(page.getByTestId('preferences-settings')).toBeVisible();
});
