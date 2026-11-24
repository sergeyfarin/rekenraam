import { readFileSync } from 'node:fs';
import { expect, test, type Page } from './support/fixtures';
import { test as ownedBookTest } from './support/owned-book';
import { apiJSON } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';
import { translationLocales } from './support/locales';

// The owner's day, not the runner's: the server dates the disable on it, and
// the two differ near midnight or across time zones (#664 review).
async function ownerToday(page: Page): Promise<string> {
  return (await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session')).owner_today;
}

function addDays(day: string, days: number): string {
  const [year, month, date] = day.split('-').map(Number);
  return new Date(Date.UTC(year, month - 1, date + days)).toISOString().slice(0, 10);
}

type Stored = {
  version_id: number;
  description: string;
  journal_entries: Array<{
    entry_date: string;
    postings: Array<{ account_id: number; quantity_value: string; quantity_scale: number; commodity_id: number }>;
  }>;
};

// #664: disabling a category on the day of its last posting keeps that day
// open, as closing an account does. The category leaves the pickers at once,
// and an existing entry dated that day still saves with its category.
test('[acceptance] an entry dated the disable day still saves with its disabled category', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const name = `Disable day meals ${suffix}`;
  const description = `Disable day lunch ${suffix}`;
  const account = await createCashAccount(page, csrfToken, `Disable day wallet ${suffix}`, currencyID);
  const category = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name, category_type: 'expense' });
  const today = await ownerToday(page);
  const created = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_date: today, status: 'posted', description,
    journal_entries: [{ entry_date: today, postings: [
      { account_id: account.id, commodity_id: currencyID, quantity_value: '-1250', quantity_scale: 2 },
      { account_id: category.id, commodity_id: currencyID, quantity_value: '1250', quantity_scale: 2 }
    ] }]
  });
  const read = () => apiJSON<Stored>(page, 'GET', `/api/v1/transactions/${created.id}`);

  // Disable it from the category screen; the screen disables from today.
  await page.goto('/app/ledger/categories');
  await page.getByRole('button', { name: `Archive ${name}` }).click();
  await expect.poll(async () => (await apiJSON<{ status: string }>(page, 'GET', `/api/v1/categories/${category.id}`)).status).toBe('archived');
  const disabled = await apiJSON<{ status: string; closed_on: string }>(page, 'GET', `/api/v1/categories/${category.id}`);
  expect(disabled).toMatchObject({ status: 'archived', closed_on: today });

  // Edit the existing entry's description and save.
  await page.goto('/app/ledger/transactions');
  const edit = page.getByRole('complementary', { name: 'Actions' });
  await page.getByRole('row').filter({ hasText: description }).getByRole('button', { name: 'Edit', exact: true }).click();
  await expect(edit.getByRole('heading', { name: 'Edit transaction' })).toBeVisible();
  await edit.getByLabel('Description').fill(`${description} edited`);
  const before = (await read()).version_id;
  await edit.getByRole('button', { name: 'Save transaction' }).click();
  await expect.poll(async () => (await read()).version_id).not.toBe(before);

  await page.reload();
  await expect(page.getByRole('row').filter({ hasText: `${description} edited` })).toBeVisible();
  const stored = await read();
  expect(stored.description).toBe(`${description} edited`);
  expect(stored.journal_entries).toHaveLength(1);
  expect(stored.journal_entries[0].entry_date).toBe(today);
  expect(stored.journal_entries[0].postings).toEqual(expect.arrayContaining([
    expect.objectContaining({ account_id: category.id, quantity_value: '1250', quantity_scale: 2, commodity_id: currencyID }),
    expect.objectContaining({ account_id: account.id, quantity_value: '-1250', quantity_scale: 2, commodity_id: currencyID })
  ]));
});

// A later posting, here a future-dated one, refuses the disable; the screen
// says which day and what to do. Owned book: the future posting would move
// other specs' book-wide figures.
for (const locale of translationLocales) {
  ownedBookTest(`[acceptance] a disable that would strand a later posting names its date in ${locale}`, async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    const settings = JSON.parse(
      readFileSync(new URL(`../../frontend/messages/settings/${locale}.json`, import.meta.url), 'utf8')
    ) as Record<string, string>;
    const { csrfToken, currencyID } = await readyForLedger(page);
    const suffix = Date.now();
    const name = `Later posting ${suffix}`;
    const account = await createCashAccount(page, csrfToken, `Later wallet ${suffix}`, currencyID);
    const category = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name, category_type: 'expense' });
    const tomorrow = addDays(await ownerToday(page), 1);
    await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
      transaction_date: tomorrow, status: 'posted', description: `Prepaid ${suffix}`,
      journal_entries: [{ entry_date: tomorrow, postings: [
        { account_id: account.id, commodity_id: currencyID, quantity_value: '-500', quantity_scale: 2 },
        { account_id: category.id, commodity_id: currencyID, quantity_value: '500', quantity_scale: 2 }
      ] }]
    });

    await page.evaluate((value) => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    await page.goto('/app/ledger/categories');
    const archive = settings.categories_action_archive.replace('{name}', name);
    const answered = page.waitForResponse((response) => response.url().endsWith(`/api/v1/categories/${category.id}/disable`));
    await page.getByRole('button', { name: archive }).click();
    const response = await answered;
    expect(response.status()).toBe(409);
    expect((await response.json()).error).toMatchObject({
      code: 'REFERENCE_DATED_CONFLICT', conflict_rule: 'category_posting_after_disable', conflict_date: tomorrow
    });

    const feedback = page.locator('#categories-action-error');
    const [lead, rest] = settings.api_error_reference_conflict_category_posting_after_disable.split('{date}');
    await expect(feedback).toContainText(lead.trim());
    await expect(feedback).toContainText(rest.trim());
    await expect(feedback).not.toContainText(tomorrow, { useInnerText: true });
    // Beside the tapped button, on screen on a phone.
    await expect(feedback).toBeInViewport();
    expect((await apiJSON<{ status: string }>(page, 'GET', `/api/v1/categories/${category.id}`)).status).toBe('active');
  });
}

// A picker loaded before the archive must not offer the category afterwards:
// the shell refreshes every cached category list after a category write. The
// ledger would accept a posting dated the disable day, so a stale picker is
// the only thing that would let one through (#664 review).
test('[acceptance] a category archived in the same tab leaves the entry picker', async ({ page }) => {
  const { csrfToken } = await readyForLedger(page);
  const suffix = Date.now();
  const create = (name: string) => apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name, category_type: 'expense' });
  const kept = `Picker kept ${suffix}`;
  const retired = `Picker retired ${suffix}`;
  const retiredCategory = await create(retired);
  await create(kept);
  page.on('dialog', (dialog) => void dialog.accept());

  const pickerOptions = async () => {
    await page.getByRole('button', { name: 'New transaction' }).click();
    const form = page.getByRole('region', { name: 'New transaction', exact: true });
    await form.getByRole('combobox', { name: 'Category', exact: true }).fill(String(suffix));
    return { form, options: form.getByRole('listbox', { name: 'Category', exact: true }).getByRole('option') };
  };
  const tabs = page.getByRole('navigation', { name: 'Ledger pages', exact: true });

  await page.goto('/app/ledger/transactions');
  let { form, options } = await pickerOptions();
  await expect(options.filter({ hasText: retired })).toBeVisible();
  await form.getByRole('combobox', { name: 'Category', exact: true }).fill('');
  await form.getByRole('button', { name: 'Cancel', exact: true }).first().click();
  await expect(form).toBeHidden();

  await tabs.getByRole('link', { name: 'Categories', exact: true }).click();
  await page.getByRole('button', { name: `Archive ${retired}` }).click();
  await expect.poll(async () => (await apiJSON<{ status: string }>(page, 'GET', `/api/v1/categories/${retiredCategory.id}`)).status).toBe('archived');

  await tabs.getByRole('link', { name: 'Transactions', exact: true }).click();
  ({ options } = await pickerOptions());
  await expect(options.filter({ hasText: kept })).toBeVisible();
  await expect(options.filter({ hasText: retired })).toHaveCount(0);
});
