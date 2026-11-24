import { readFileSync } from 'node:fs';
import { expect, test, type Page, type Route } from './support/fixtures';
import { test as ownedBookTest } from './support/owned-book';
import { apiJSON } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';
import { openInvestmentActivity } from './support/investment-entry';
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

    const feedback = page.locator(`#categories-action-error-${category.id}`);
    const [lead, rest] = settings.api_error_reference_conflict_category_posting_after_disable.split('{date}');
    await expect(feedback).toContainText(lead.trim());
    await expect(feedback).toContainText(rest.trim());
    await expect(feedback).not.toContainText(tomorrow, { useInnerText: true });
    // Beside the tapped button, on screen on a phone.
    await expect(feedback).toBeInViewport();
    expect((await apiJSON<{ status: string }>(page, 'GET', `/api/v1/categories/${category.id}`)).status).toBe('active');
  });
}

// A picker loaded before the archive must not offer the category afterwards,
// even while its refetch is still pending: the ledger accepts a posting dated
// the disable day, so the cached list is the only thing that would let one
// through (#664 review). Covers the entry editor's category picker and the
// Cash dividend income picker, which reads its own account cache.
test('[acceptance] a category archived in the same tab leaves every picker while the refresh is pending', async ({ page }) => {
  const { csrfToken } = await readyForLedger(page);
  const suffix = Date.now();
  const create = (name: string, type: 'income' | 'expense') =>
    apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name, category_type: type });
  const kept = `Picker kept ${suffix}`;
  const retired = `Picker retired ${suffix}`;
  const retiredCategory = await create(retired, 'expense');
  await create(kept, 'expense');
  const keptIncome = await create(`Dividends kept ${suffix}`, 'income');
  const retiredIncome = await create(`Dividends retired ${suffix}`, 'income');
  page.on('dialog', (dialog) => void dialog.accept());

  const entry = page.getByRole('region', { name: 'New transaction', exact: true });
  const cancelEntry = async () => {
    await entry.getByRole('button', { name: 'Cancel', exact: true }).first().click();
    await expect(entry).toBeHidden();
  };
  const categoryOptions = async () => {
    await page.getByRole('button', { name: 'New transaction' }).click();
    await entry.getByRole('combobox', { name: 'Category', exact: true }).fill(String(suffix));
    return entry.getByRole('listbox', { name: 'Category', exact: true }).getByRole('option');
  };
  const incomeOption = (id: number) => page.locator(`#div-income-account option[value="${id}"]`);
  const dividend = page.getByRole('dialog', { name: 'Record dividend' });
  const closeDividend = async () => {
    await page.keyboard.press('Escape');
    await expect(dividend).toBeHidden();
    if (await entry.isVisible()) await cancelEntry();
  };
  const tabs = page.getByRole('navigation', { name: 'Ledger pages', exact: true });

  // Warm both pickers.
  await page.goto('/app/ledger/transactions');
  let options = await categoryOptions();
  await expect(options.filter({ hasText: retired })).toBeVisible();
  await entry.getByRole('combobox', { name: 'Category', exact: true }).fill('');
  await cancelEntry();
  await openInvestmentActivity(page, 'dividend');
  await expect(incomeOption(retiredIncome.id)).toHaveCount(1);
  await closeDividend();

  await tabs.getByRole('link', { name: 'Categories', exact: true }).click();
  await expect(page.getByRole('button', { name: `Archive ${retired}` })).toBeVisible();

  // From here, after the archives succeed, every category and account list read stays pending.
  const held: Route[] = [];
  const listReads = new Set(['/api/v1/categories', '/api/v1/accounts', '/api/v1/investments/entry-accounts']);
  await page.route((url) => listReads.has(url.pathname), (route) => {
    if (route.request().method() === 'GET') held.push(route);
    else void route.continue();
  });

  for (const [name, id] of [[retired, retiredCategory.id], [`Dividends retired ${suffix}`, retiredIncome.id]] as const) {
    await page.getByRole('button', { name: `Archive ${name}` }).click();
    await expect.poll(async () => (await apiJSON<{ status: string }>(page, 'GET', `/api/v1/categories/${id}`)).status).toBe('archived');
  }

  await tabs.getByRole('link', { name: 'Transactions', exact: true }).click();
  const heldBefore = held.length;
  options = await categoryOptions();
  await expect.poll(() => held.length, { message: 'the picker refetch is pending' }).toBeGreaterThan(heldBefore);
  await expect(options.filter({ hasText: kept })).toBeVisible();
  await expect(options.filter({ hasText: retired })).toHaveCount(0);
  await entry.getByRole('combobox', { name: 'Category', exact: true }).fill('');
  await cancelEntry();

  await openInvestmentActivity(page, 'dividend');
  await expect(incomeOption(keptIncome.id)).toHaveCount(1);
  await expect(incomeOption(retiredIncome.id)).toHaveCount(0);
  expect(held.length, 'the dividend picker refetch is pending too').toBeGreaterThan(0);

  await page.unrouteAll({ behavior: 'ignoreErrors' });
  for (const route of held) await route.continue().catch(() => undefined);
});

// Two overlapping refusals each stay on their own row: A's late answer must
// not replace B's date or leave A without its message (#664 review).
ownedBookTest('[acceptance] overlapping archive refusals stay on their own rows', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `Overlap wallet ${suffix}`, currencyID);
  const today = await ownerToday(page);
  const refused = async (name: string, days: number) => {
    const category = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name, category_type: 'expense' });
    const date = addDays(today, days);
    await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
      transaction_date: date, status: 'posted', description: `${name} prepaid`,
      journal_entries: [{ entry_date: date, postings: [
        { account_id: account.id, commodity_id: currencyID, quantity_value: '-500', quantity_scale: 2 },
        { account_id: category.id, commodity_id: currencyID, quantity_value: '500', quantity_scale: 2 }
      ] }]
    });
    return { id: category.id, name, date };
  };
  const a = await refused(`Overlap A ${suffix}`, 1);
  const b = await refused(`Overlap B ${suffix}`, 2);
  const formatted = (date: string) => page.evaluate((value) =>
    new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeZone: 'UTC' }).format(Date.parse(`${value}T00:00:00Z`)), date);

  await page.goto('/app/ledger/categories');
  let releaseA: () => void = () => {};
  const aHeld = new Promise<void>((resolve) => {
    void page.route(`**/api/v1/categories/${a.id}/disable`, async (route) => {
      const response = await route.fetch();
      await new Promise<void>((release) => { releaseA = release; resolve(); });
      await route.fulfill({ response });
    });
  });
  await page.getByRole('button', { name: `Archive ${a.name}` }).click();
  await aHeld;
  await page.getByRole('button', { name: `Archive ${b.name}` }).click();
  const errorOf = (id: number) => page.locator(`#categories-action-error-${id}`);
  await expect(errorOf(b.id)).toContainText(await formatted(b.date));
  releaseA();
  await expect(errorOf(a.id)).toContainText(await formatted(a.date));
  await expect(errorOf(b.id)).toContainText(await formatted(b.date));
  await expect(errorOf(b.id)).not.toContainText(await formatted(a.date));
});

// A Cash dividend never carries an archived default: the field stays blank
// and Record dividend waits for an offered account. Otherwise the hidden ID
// was sent and the ledger accepted it on the disable day (#664 review).
test('[acceptance] a cash dividend does not apply an archived default income category', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const today = await ownerToday(page);
  const opened = addDays(today, -10);
  const name = `Default shares ${suffix}`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: `DD${suffix}`, instrument_type: 'stock', display_name: name,
    quantity_scale: 0, price_scale: 2, quote_commodity_id: currencyID, effective_from: opened
  });
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Default cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash', default_commodity_id: currencyID,
    allows_postings: true, opened_on: opened, effective_from: opened
  });
  const retired = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name: `Default dividends ${suffix}`, category_type: 'income' });
  const active = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name: `Other dividends ${suffix}`, category_type: 'income' });
  await apiJSON(page, 'POST', '/api/v1/investments/dividend-defaults', csrfToken, {
    commodity_id: instrument.commodity_id, income_account_id: retired.id, status: 'active', effective_from: opened
  });
  await apiJSON(page, 'POST', `/api/v1/categories/${retired.id}/disable`, csrfToken, {});

  await page.goto('/app/ledger/transactions');
  await openInvestmentActivity(page, 'dividend');
  const form = page.getByRole('dialog', { name: 'Record dividend' });
  await form.locator('#div-instrument').fill(name);
  await page.getByRole('option').filter({ hasText: name }).click();
  await form.locator('#div-cash-account').selectOption(String(cash.id));
  await form.locator('#div-amount').fill('3.25');
  const income = form.locator('#div-income-account');
  await expect(income.locator(`option[value="${retired.id}"]`)).toHaveCount(0);
  await expect(income).toHaveValue('');
  const record = form.getByRole('button', { name: 'Record dividend', exact: true });
  await expect(record).toBeDisabled();
  await income.selectOption(String(active.id));
  await expect(record).toBeEnabled();
});
