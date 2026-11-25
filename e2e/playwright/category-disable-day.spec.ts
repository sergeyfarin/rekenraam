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

async function dividendFixture(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const today = await ownerToday(page);
  const opened = addDays(today, -10);
  const name = `Prior-day shares ${suffix}`;
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: `BD${suffix}`, instrument_type: 'stock', display_name: name,
    quantity_scale: 0, price_scale: 2, quote_commodity_id: currencyID, effective_from: opened
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Prior-day holding ${suffix}`, opened_on: opened, effective_from: opened
  });
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Prior-day cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash', default_commodity_id: currencyID,
    allows_postings: true, opened_on: opened, effective_from: opened
  });
  const category = (label: string) => apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name: `${label} ${suffix}`, category_type: 'income' });
  const retired = await category('Retired dividends');
  const fallback = await category('Fallback dividends');
  const other = await category('Other dividends');
  return { csrfToken, currencyID, today, name, instrument, holding, cash, retired, fallback, other };
}

// A default is applied only while its account is not archived today, even on
// a backdated entry whose dated picker still offers it; choosing it by hand
// stays possible there (#664 review).
test('[acceptance] a backdated cash dividend leaves an archived default for the owner to choose', async ({ page }) => {
  const f = await dividendFixture(page);
  await apiJSON(page, 'POST', '/api/v1/investments/dividend-defaults', f.csrfToken, {
    commodity_id: f.instrument.commodity_id, income_account_id: f.retired.id, status: 'active', effective_from: addDays(f.today, -10)
  });
  await apiJSON(page, 'POST', `/api/v1/categories/${f.retired.id}/disable`, f.csrfToken, {});

  await page.goto('/app/ledger/transactions');
  await openInvestmentActivity(page, 'dividend');
  const form = page.getByRole('dialog', { name: 'Record dividend' });
  await form.locator('#div-date').fill(addDays(f.today, -1));
  await form.locator('#div-instrument').fill(f.name);
  await page.getByRole('option').filter({ hasText: f.name }).click();
  await form.locator('#div-cash-account').selectOption(String(f.cash.id));
  await form.locator('#div-amount').fill('3.25');
  const income = form.locator('#div-income-account');
  await expect(income.locator(`option[value="${f.retired.id}"]`)).toHaveCount(1);
  await expect(income).toHaveValue('');
  const record = form.getByRole('button', { name: 'Record dividend', exact: true });
  await expect(record).toBeDisabled();
  await income.selectOption(String(f.retired.id));
  await expect(record).toBeEnabled();
});

// A chosen account the picker stops offering after a date change stays
// visible as unavailable and blocks Record; it is never dropped for the
// server to replace with the stored default (#664 review).
test('[acceptance] a reinvested dividend keeps an unavailable choice visible and blocks Record', async ({ page }) => {
  const f = await dividendFixture(page);
  await apiJSON(page, 'POST', '/api/v1/investments/dividend-defaults', f.csrfToken, {
    commodity_id: f.instrument.commodity_id, income_account_id: f.fallback.id, status: 'active', effective_from: addDays(f.today, -10)
  });
  await apiJSON(page, 'POST', `/api/v1/categories/${f.retired.id}/disable`, f.csrfToken, {});

  await page.goto('/app/ledger/transactions');
  await openInvestmentActivity(page, 'reinvested');
  const form = page.getByRole('dialog');
  await form.locator('#div-date').fill(addDays(f.today, -1));
  await form.locator('#div-instrument').fill(f.name);
  await page.getByRole('option').filter({ hasText: f.name }).click();
  await form.locator('#div-holding-account').selectOption(String(f.holding.id));
  await form.locator('#div-cost-currency').selectOption(String(f.currencyID));
  await form.locator('#div-quantity').fill('1');
  await form.locator('#div-amount').fill('25.00');
  const income = form.locator('#div-income-account');
  await income.selectOption(String(f.retired.id));
  const record = form.locator('button[type="submit"]');
  await expect(record).toBeEnabled();

  await form.locator('#div-date').fill(f.today);
  await expect(income).toHaveValue(String(f.retired.id));
  await expect(income.locator('option:checked')).toContainText('not available on this date');
  await expect(form.locator('#div-income-unavailable')).toBeVisible();
  await expect(record).toBeDisabled();

  await income.selectOption(String(f.other.id));
  await expect(form.locator('#div-income-unavailable')).toHaveCount(0);
  await expect(record).toBeEnabled();
});

// A default that filled the income field is an automatic choice: once its
// account is archived today, the form takes it back instead of sending it,
// while a backdated entry can still choose it by hand (#664 review).
test('[acceptance] a filled-in dividend default archived afterwards is taken back', async ({ page }) => {
  await page.clock.install();
  const f = await dividendFixture(page);
  await apiJSON(page, 'POST', '/api/v1/investments/dividend-defaults', f.csrfToken, {
    commodity_id: f.instrument.commodity_id, income_account_id: f.retired.id, status: 'active', effective_from: addDays(f.today, -10)
  });

  await page.goto('/app/ledger/transactions');
  await openInvestmentActivity(page, 'dividend');
  const form = page.getByRole('dialog', { name: 'Record dividend' });
  const yesterday = addDays(f.today, -1);
  await form.locator('#div-date').fill(yesterday);
  await form.locator('#div-instrument').fill(f.name);
  await page.getByRole('option').filter({ hasText: f.name }).click();
  await form.locator('#div-cash-account').selectOption(String(f.cash.id));
  await form.locator('#div-amount').fill('3.25');
  const income = form.locator('#div-income-account');
  await expect(income).toHaveValue(String(f.retired.id));
  const record = form.getByRole('button', { name: 'Record dividend', exact: true });
  await expect(record).toBeEnabled();

  // Archived elsewhere; the owner-day list refreshes once it is stale.
  await apiJSON(page, 'POST', `/api/v1/categories/${f.retired.id}/disable`, f.csrfToken, {});
  await page.clock.fastForward('00:15');
  await form.locator('#div-date').fill(f.today);
  await expect(income).toHaveValue('');
  await form.locator('#div-date').fill(yesterday);
  await expect(income).toHaveValue('');
  await expect(record).toBeDisabled();
  await expect(income.locator(`option[value="${f.retired.id}"]`)).toHaveCount(1);
  await income.selectOption(String(f.retired.id));
  await expect(record).toBeEnabled();
});

// A default still loading when the instrument is cleared never applies, and
// clearing the instrument takes back a default already filled in (#664 review).
test('[acceptance] a dividend default for a cleared instrument does not apply', async ({ page }) => {
  const f = await dividendFixture(page);
  await apiJSON(page, 'POST', '/api/v1/investments/dividend-defaults', f.csrfToken, {
    commodity_id: f.instrument.commodity_id, income_account_id: f.fallback.id, status: 'active', effective_from: addDays(f.today, -10)
  });
  let releaseDefaults: () => void = () => {};
  const defaultsHeld = new Promise<void>((resolve) => {
    void page.route('**/api/v1/investments/dividend-defaults', async (route) => {
      if (route.request().method() !== 'GET') return route.continue();
      const response = await route.fetch();
      resolve();
      await new Promise<void>((release) => { releaseDefaults = release; });
      await route.fulfill({ response });
    });
  });

  await page.goto('/app/ledger/transactions');
  await openInvestmentActivity(page, 'dividend');
  const form = page.getByRole('dialog', { name: 'Record dividend' });
  await form.locator('#div-instrument').fill(f.name);
  await page.getByRole('option').filter({ hasText: f.name }).click();
  await defaultsHeld;
  await form.locator('#div-instrument').fill('');
  releaseDefaults();
  await page.unrouteAll({ behavior: 'wait' });

  const income = form.locator('#div-income-account');
  await form.locator('#div-cash-account').selectOption(String(f.cash.id));
  await form.locator('#div-amount').fill('3.25');
  await expect(form.locator(`#div-income-account option[value="${f.fallback.id}"]`)).toHaveCount(1);
  await expect(income).toHaveValue('');
  await expect(form.getByRole('button', { name: 'Record dividend', exact: true })).toBeDisabled();

  // Selected again, the default fills in; clearing the instrument takes it back.
  await form.locator('#div-instrument').fill(f.name);
  await page.getByRole('option').filter({ hasText: f.name }).click();
  await expect(income).toHaveValue(String(f.fallback.id));
  await form.locator('#div-instrument').fill('');
  await expect(income).toHaveValue('');
});

// Switching instruments replaces the withholding amount a default filled in
// with the next instrument's default, and keeps an amount the owner typed
// (#664 review: the first instrument's tax was posted against the second).
test('[acceptance] a dividend default withholding amount follows the selected instrument', async ({ page }) => {
  const f = await dividendFixture(page);
  const opened = addDays(f.today, -10);
  const secondName = `Second shares ${Date.now()}`;
  const second = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', f.csrfToken, {
    commodity_code: `SS${Date.now()}`, instrument_type: 'stock', display_name: secondName,
    quantity_scale: 0, price_scale: 2, quote_commodity_id: f.currencyID, effective_from: opened
  });
  const tax = (label: string) => apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', f.csrfToken, { name: `${label} ${Date.now()}`, category_type: 'expense' });
  const firstTax = await tax('First tax');
  const secondTax = await tax('Second tax');
  for (const [commodity, account, value] of [[f.instrument.commodity_id, firstTax.id, '125'], [second.commodity_id, secondTax.id, '275']] as const) {
    await apiJSON(page, 'POST', '/api/v1/investments/dividend-defaults', f.csrfToken, {
      commodity_id: commodity, income_account_id: f.fallback.id, withholding_account_id: account,
      default_withholding_value: value, default_withholding_scale: 2, status: 'active', effective_from: opened
    });
  }

  await page.goto('/app/ledger/transactions');
  await openInvestmentActivity(page, 'dividend');
  const form = page.getByRole('dialog', { name: 'Record dividend' });
  const choose = async (name: string) => {
    await form.locator('#div-instrument').fill(name);
    await page.getByRole('option').filter({ hasText: name }).click();
  };
  const amount = form.locator('#div-withholding');
  const account = form.locator('#div-withholding-account');

  await choose(f.name);
  await expect(amount).toHaveValue('1.25');
  await expect(account).toHaveValue(String(firstTax.id));
  await choose(secondName);
  await expect(amount).toHaveValue('2.75');
  await expect(account).toHaveValue(String(secondTax.id));

  // Toggling withholding off and on keeps it shown but leaves the amount
  // automatic, so the next instrument's default still replaces it.
  const toggle = form.getByRole('checkbox', { name: 'Add withholding tax' });
  await choose(f.name);
  await expect(amount).toHaveValue('1.25');
  await toggle.uncheck();
  await toggle.check();
  await choose(secondName);
  await expect(amount).toHaveValue('2.75');
  await expect(toggle).toBeChecked();

  // An amount the owner typed is theirs and survives the next switch.
  await amount.fill('5.00');
  await choose(f.name);
  await expect(amount).toHaveValue('5.00');
  await expect(account).toHaveValue(String(firstTax.id));
});

// The server checks an automatic account when it writes: an archive landing
// while Save is in flight refuses the dividend, and the form takes the
// default back for the owner to review (#664 review).
test('[acceptance] a dividend saved with a default archived in flight is refused', async ({ page }) => {
  const f = await dividendFixture(page);
  await apiJSON(page, 'POST', '/api/v1/investments/dividend-defaults', f.csrfToken, {
    commodity_id: f.instrument.commodity_id, income_account_id: f.retired.id, status: 'active', effective_from: addDays(f.today, -10)
  });

  await page.goto('/app/ledger/transactions');
  await openInvestmentActivity(page, 'dividend');
  const form = page.getByRole('dialog', { name: 'Record dividend' });
  await form.locator('#div-date').fill(addDays(f.today, -1));
  await form.locator('#div-instrument').fill(f.name);
  await page.getByRole('option').filter({ hasText: f.name }).click();
  await form.locator('#div-cash-account').selectOption(String(f.cash.id));
  await form.locator('#div-amount').fill('3.25');
  const income = form.locator('#div-income-account');
  await expect(income).toHaveValue(String(f.retired.id));

  let release: () => void = () => {};
  const held = new Promise<void>((resolve) => {
    void page.route('**/api/v1/investments/dividend', async (route) => {
      expect(route.request().postDataJSON()).toMatchObject({ income_account_id: f.retired.id, income_account_from_default: true });
      resolve();
      await new Promise<void>((go) => { release = go; });
      await route.continue();
    });
  });
  const answered = page.waitForResponse((response) => response.url().endsWith('/api/v1/investments/dividend') && response.request().method() === 'POST');
  await form.getByRole('button', { name: 'Record dividend', exact: true }).click();
  await held;
  await apiJSON(page, 'POST', `/api/v1/categories/${f.retired.id}/disable`, f.csrfToken, {});
  release();
  const response = await answered;
  expect(response.status()).toBe(409);
  expect((await response.json()).error.code).toBe('DIVIDEND_DEFAULT_CHANGED');
  await page.unrouteAll({ behavior: 'wait' });

  await expect(form.locator('#div-form-error')).toContainText('The dividend default changed');
  await expect(income).toHaveValue('');
  await expect(form.getByRole('button', { name: 'Record dividend', exact: true })).toBeDisabled();
  await expect(income.locator(`option[value="${f.retired.id}"]`)).toHaveCount(1);
});

// With two defaults for one instrument, a backdated entry shows the one in
// effect on its date, as the server resolves it, and a date change resolves
// again (#664 review: the form picked the newer default and the server
// refused it on every retry).
test('[acceptance] a backdated dividend shows the default in effect on its date', async ({ page }) => {
  const f = await dividendFixture(page);
  for (const [account, from] of [[f.fallback.id, addDays(f.today, -10)], [f.other.id, addDays(f.today, -2)]] as const) {
    await apiJSON(page, 'POST', '/api/v1/investments/dividend-defaults', f.csrfToken, {
      commodity_id: f.instrument.commodity_id, income_account_id: account, status: 'active', effective_from: from
    });
  }

  await page.goto('/app/ledger/transactions');
  await openInvestmentActivity(page, 'dividend');
  const form = page.getByRole('dialog', { name: 'Record dividend' });
  const income = form.locator('#div-income-account');
  await form.locator('#div-date').fill(addDays(f.today, -5));
  await form.locator('#div-instrument').fill(f.name);
  await page.getByRole('option').filter({ hasText: f.name }).click();
  await expect(income).toHaveValue(String(f.fallback.id));

  await form.locator('#div-date').fill(f.today);
  await expect(income).toHaveValue(String(f.other.id));
  await form.locator('#div-date').fill(addDays(f.today, -5));
  await expect(income).toHaveValue(String(f.fallback.id));

  await form.locator('#div-cash-account').selectOption(String(f.cash.id));
  await form.locator('#div-amount').fill('3.25');
  const answered = page.waitForResponse((response) => response.url().endsWith('/api/v1/investments/dividend') && response.request().method() === 'POST');
  await form.getByRole('button', { name: 'Record dividend', exact: true }).click();
  const response = await answered;
  expect(response.status()).toBe(201);
  const postings = ((await response.json()).journal_entries as Array<{ postings: Array<{ account_id: number }> }>).flatMap((entry) => entry.postings);
  expect(postings.map((posting) => posting.account_id)).toContain(f.fallback.id);
});
