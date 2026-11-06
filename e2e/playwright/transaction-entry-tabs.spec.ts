import type { Locator, Page } from '@playwright/test';
import { expect, test } from './support/fixtures';
import { apiJSON } from './support/api';
import { todayISO } from './support/dates';
import { expectNoAccessibilityViolations, focusIsInside } from './support/a11y';
import { createCashAccount, createLiabilityAccount, readyForLedger } from './support/ledger';

// #506: one New transaction with four stable tabs (#491 journeys 1, 3, 6 and
// 17). Labels follow the account, switching keeps entered work, an account
// that does not fit is explained and kept, and an approval shown before a
// change never authorizes the entry after it.

const entryDialog = (page: Page) => page.getByRole('dialog', { name: 'New transaction', exact: true });
const entryRegion = (page: Page) => page.getByRole('region', { name: 'New transaction', exact: true });
const tabNames = (entry: Locator) => entry.getByRole('tab').allInnerTexts();
const selectedTab = (entry: Locator) => entry.locator('[role="tab"][aria-selected="true"]');

type Saved = { id: number; transaction_kind: string; journal_entries: Array<{ postings: Array<{ account_id: number; quantity_value: string }> }> };

async function setup(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const checking = await createCashAccount(page, csrfToken, `Tabs checking ${suffix}`, currencyID);
  const savings = await createCashAccount(page, csrfToken, `Tabs savings ${suffix}`, currencyID);
  const card = await createLiabilityAccount(page, csrfToken, `Tabs card ${suffix}`, currencyID);
  const category = await apiJSON<{ id: number; name: string }>(page, 'POST', '/api/v1/categories', csrfToken, { name: `Tabs groceries ${suffix}`, category_type: 'expense' });
  const payee = await apiJSON<{ id: number; name: string }>(page, 'POST', '/api/v1/payees', csrfToken, { name: `Tabs market ${suffix}` }, [201]);
  return { csrfToken, currencyID, suffix, checking, savings, card, category, payee };
}

async function chooseCategory(entry: Locator, name: string) {
  await entry.getByPlaceholder('Search categories').fill(name);
  await entry.locator('ul[role="listbox"] button[role="option"]').filter({ hasText: name }).first().click();
}

async function chooseExistingPayee(entry: Locator, name: string) {
  await entry.getByRole('combobox', { name: 'Payee', exact: true }).fill(name);
  await entry.getByRole('option', { name, exact: true }).dispatchEvent('mousedown');
}

for (const theme of ['light', 'dark'] as const) {
  test(`journey 1 and 17: four stable tabs labelled by account, by keyboard, in ${theme}`, async ({ page }) => {
    const s = await setup(page);
    await page.evaluate(value => localStorage.setItem('rekenraam-theme', value), theme);
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto('/app');
    await page.locator('#main-content').focus();
    await page.keyboard.press('n');
    const entry = entryDialog(page);
    await expect(entry).toBeVisible();
    await expect.poll(() => focusIsInside(entry)).toBe(true);

    // No account yet: the ordinary labels, Withdrawal selected.
    await expect.poll(() => tabNames(entry)).toEqual(['Withdrawal', 'Deposit', 'Transfer', 'Investment']);
    await expect(selectedTab(entry)).toHaveText('Withdrawal');

    // An ordinary account mutes Investment; a credit card says Charge and Credit.
    const account = entry.getByRole('combobox', { name: 'Account', exact: true });
    await account.selectOption({ label: s.checking.name });
    await expect(entry.getByRole('tab', { name: 'Investment', exact: true })).toHaveAttribute('data-muted', '');
    await account.selectOption({ label: s.card.name });
    await expect.poll(() => tabNames(entry)).toEqual(['Charge', 'Credit', 'Transfer', 'Investment']);
    await expect(selectedTab(entry)).toHaveText('Charge');
    await expectNoAccessibilityViolations(page, `entry tabs ${theme}`);

    // Arrow keys move the selection and focus; nothing is disabled.
    const charge = entry.getByRole('tab', { name: 'Charge', exact: true });
    await charge.focus();
    await page.keyboard.press('ArrowRight');
    const credit = entry.getByRole('tab', { name: 'Credit', exact: true });
    await expect(credit).toBeFocused();
    await expect(credit).toHaveAttribute('aria-selected', 'true');
    // Paying the card from another account is a Transfer, said under Credit.
    await expect(entry.getByText('Paying this card from another of your accounts? Use Transfer', { exact: false })).toBeVisible();
    await page.keyboard.press('End');
    const investment = entry.getByRole('tab', { name: 'Investment', exact: true });
    await expect(investment).toBeFocused();
    // Selected, the muted tab looks like any selected tab, never disabled.
    await expect(investment).toHaveAttribute('aria-selected', 'true');
    await expect(investment).toBeEnabled();
    await expect(investment).not.toHaveAttribute('data-muted');
    await expect(entry.getByText('Choose a holding account to record an investment.', { exact: true })).toBeVisible();
    await expect(entry.getByRole('button', { name: 'Record buy', exact: true })).toBeVisible();
    await expectNoAccessibilityViolations(page, `entry investment tab ${theme}`);
    await page.keyboard.press('Home');
    await expect(charge).toBeFocused();
    await expect(entry.getByRole('combobox', { name: 'Account', exact: true })).toHaveValue(String(s.card.id));

    // Escape still discards from the tabs and returns focus.
    await page.keyboard.press('Escape');
    await expect(entry).toHaveCount(0);
    await expect(page.locator('#main-content')).toBeFocused();
  });
}

test('switching Withdrawal → Deposit → Transfer → Withdrawal keeps the work, and the sign follows the tab', async ({ page }) => {
  const s = await setup(page);
  const description = `Tabs round trip ${s.suffix}`;
  const day = todayISO();
  await page.goto('/app');
  await page.getByRole('list', { name: 'Shortcuts', exact: true }).getByRole('button', { name: 'New transaction', exact: true }).click();
  const entry = entryDialog(page);
  await entry.getByLabel('Date', { exact: true }).fill(day);
  await chooseExistingPayee(entry, s.payee.name);
  await entry.getByLabel('Description', { exact: true }).fill(description);
  await entry.getByRole('combobox', { name: 'Account', exact: true }).selectOption({ label: s.checking.name });
  await entry.getByLabel('Amount', { exact: true }).fill('12.40');
  await chooseCategory(entry, s.category.name);

  await entry.getByRole('tab', { name: 'Deposit', exact: true }).click();
  await expect(entry.getByLabel('Amount', { exact: true })).toHaveValue('12.40');
  await expect(entry.getByText(s.category.name, { exact: true })).toBeVisible();

  // Transfer keeps the date, payee and memo, and the account as From; its
  // own fields start empty and stay its own.
  await entry.getByRole('tab', { name: 'Transfer', exact: true }).click();
  await expect(entry.getByLabel('Date', { exact: true })).toHaveValue(day);
  await expect(entry.getByLabel('Description', { exact: true })).toHaveValue(description);
  await expect(entry.getByRole('combobox', { name: 'Payee', exact: true })).toHaveValue(s.payee.name);
  await expect(entry.getByRole('combobox', { name: 'From account', exact: true })).toHaveValue(String(s.checking.id));
  await entry.getByRole('combobox', { name: 'To account', exact: true }).selectOption({ label: s.savings.name });
  await entry.getByLabel('Fee (optional)', { exact: true }).fill('0.50');

  await entry.getByRole('tab', { name: 'Withdrawal', exact: true }).click();
  await expect(entry.getByRole('combobox', { name: 'Account', exact: true })).toHaveValue(String(s.checking.id));
  await expect(entry.getByLabel('Amount', { exact: true })).toHaveValue('12.40');
  await expect(entry.getByText(s.category.name, { exact: true })).toBeVisible();
  await entry.getByRole('tab', { name: 'Transfer', exact: true }).click();
  await expect(entry.getByRole('combobox', { name: 'To account', exact: true })).toHaveValue(String(s.savings.id));
  await expect(entry.getByLabel('Fee (optional)', { exact: true })).toHaveValue('0.50');

  // Saved from Deposit: money into checking, the category balancing it.
  await entry.getByRole('tab', { name: 'Deposit', exact: true }).click();
  const created = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/transactions');
  await entry.getByRole('button', { name: 'Save transaction', exact: true }).click();
  expect((await created).status()).toBe(201);
  const saved = await (await created).json() as Saved;
  expect(saved.transaction_kind).toBe('ordinary');
  const postings = saved.journal_entries.flatMap(entry => entry.postings).map(p => [p.account_id, p.quantity_value]);
  expect(postings).toEqual(expect.arrayContaining([[s.checking.id, '1240'], [s.category.id, '-1240']]));
  await expect(entry).toHaveCount(0);
});

test('journey 3: an unsaved payment moved from Checking to the credit card keeps its fields and saves there', async ({ page }) => {
  const s = await setup(page);
  const description = `Tabs moved payment ${s.suffix}`;
  const day = todayISO();
  await page.goto(`/app/ledger/accounts/${s.checking.id}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  const entry = entryRegion(page);
  const account = entry.getByRole('combobox', { name: 'Account', exact: true });
  await expect(account).toHaveValue(String(s.checking.id));
  await entry.getByLabel('Date', { exact: true }).fill(day);
  await chooseExistingPayee(entry, s.payee.name);
  await entry.getByLabel('Description', { exact: true }).fill(description);
  await entry.getByLabel('Amount', { exact: true }).fill('25.00');
  await chooseCategory(entry, s.category.name);

  // The same first tab, now named for the card; every field stays.
  await account.selectOption({ label: s.card.name });
  await expect(selectedTab(entry)).toHaveText('Charge');
  await expect(entry.getByLabel('Date', { exact: true })).toHaveValue(day);
  await expect(entry.getByRole('combobox', { name: 'Payee', exact: true })).toHaveValue(s.payee.name);
  await expect(entry.getByLabel('Description', { exact: true })).toHaveValue(description);
  await expect(entry.getByLabel('Amount', { exact: true })).toHaveValue('25.00');
  await expect(entry.getByText(s.category.name, { exact: true })).toBeVisible();

  const created = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/transactions');
  await entry.getByRole('button', { name: 'Save transaction', exact: true }).click();
  expect((await created).status()).toBe(201);
  const saved = await (await created).json() as Saved;
  // A charge raises what the card owes: the same credit the signed amount gave.
  const postings = saved.journal_entries.flatMap(entry => entry.postings).map(p => [p.account_id, p.quantity_value]);
  expect(postings).toEqual(expect.arrayContaining([[s.card.id, '-2500'], [s.category.id, '2500']]));
  expect(postings.map(([id]) => id)).not.toContain(s.checking.id);
});

test('journey 6: a holding cannot take a withdrawal; the entry says so, keeps the work and saves once corrected', async ({ page }) => {
  const s = await setup(page);
  const instrument = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/instruments', s.csrfToken, {
    commodity_code: `TB${s.suffix}`, instrument_type: 'stock', display_name: `Tabs Co ${s.suffix}`, symbol: `TB${s.suffix}`,
    quote_commodity_id: s.currencyID, trading_commodity_id: s.currencyID, quantity_scale: 0, price_scale: 2, effective_from: todayISO()
  });
  const holding = await apiJSON<{ id: number; name: string }>(page, 'POST', '/api/v1/investments/holding-accounts', s.csrfToken, {
    instrument_id: instrument.id, name: `Tabs holding ${s.suffix}`, opened_on: todayISO(), effective_from: todayISO()
  });
  const description = `Tabs holding fee ${s.suffix}`;
  await page.goto(`/app/ledger/accounts/${holding.id}/register`);
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  const entry = entryRegion(page);
  await expect(selectedTab(entry)).toHaveText('Investment');

  await entry.getByRole('tab', { name: 'Withdrawal', exact: true }).click();
  const account = entry.getByRole('combobox', { name: 'Account', exact: true });
  // The holding stays identified, explained beside the picker.
  await expect(account.locator('option:checked')).toHaveText(`Tabs holding ${s.suffix}`);
  const explanation = entry.getByText(`Tabs holding ${s.suffix} is a holding account and records investment activity only.`, { exact: false });
  await expect(explanation).toBeVisible();
  await entry.getByLabel('Date', { exact: true }).fill(todayISO());
  await entry.getByLabel('Description', { exact: true }).fill(description);
  await entry.getByLabel('Amount', { exact: true }).fill('3.00');
  await chooseCategory(entry, s.category.name);

  const posts: number[] = [];
  page.on('response', response => {
    if (response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/transactions') posts.push(response.status());
  });
  await entry.getByRole('button', { name: 'Save transaction', exact: true }).click();
  await expect(entry.getByRole('alert').filter({ hasText: 'records investment activity only' })).toBeVisible();
  expect(posts).toEqual([]);

  await account.selectOption({ label: s.checking.name });
  await expect(explanation).toHaveCount(0);
  await expect(entry.getByLabel('Description', { exact: true })).toHaveValue(description);
  await expect(entry.getByLabel('Amount', { exact: true })).toHaveValue('3.00');
  await entry.getByRole('button', { name: 'Save transaction', exact: true }).click();
  await expect.poll(() => posts).toEqual([201]);
  const listed = await apiJSON<{ transactions: Array<{ description: string }> }>(page, 'GET', `/api/v1/transactions?account_id=${s.checking.id}`);
  expect(listed.transactions.map(item => item.description)).toContain(description);
});

// The split editor's amounts are signed; under a cash tab they follow the
// tab, and the tab never names the other direction (#510 review).
async function startSplitWithdrawal(page: Page, s: Awaited<ReturnType<typeof setup>>, description: string) {
  await page.goto('/app');
  await page.getByRole('list', { name: 'Shortcuts', exact: true }).getByRole('button', { name: 'New transaction', exact: true }).click();
  const entry = entryDialog(page);
  await entry.getByLabel('Date', { exact: true }).fill(todayISO());
  await entry.getByLabel('Description', { exact: true }).fill(description);
  await entry.getByRole('combobox', { name: 'Account', exact: true }).selectOption({ label: s.checking.name });
  await entry.getByLabel('Amount', { exact: true }).fill('12.40');
  await chooseCategory(entry, s.category.name);
  await entry.getByRole('button', { name: 'Use split entry', exact: true }).click();
  const legAccounts = entry.getByRole('combobox', { name: 'Account', exact: true });
  const legAmounts = entry.getByRole('textbox', { name: 'Amount', exact: true });
  await expect(legAccounts.nth(0)).toHaveValue(String(s.checking.id));
  await expect(legAmounts.nth(0)).toHaveValue('-12.40');
  await expect(legAmounts.nth(1)).toHaveValue('12.40');
  return { entry, legAccounts, legAmounts };
}

function savedPostings(page: Page) {
  const created = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/transactions');
  return async () => {
    const response = await created;
    expect(response.status()).toBe(201);
    const saved = await response.json() as Saved;
    return saved.journal_entries.flatMap(entry => entry.postings).map(p => [p.account_id, p.quantity_value]);
  };
}

test('a split kept under Deposit is the same entry the other way, and a contradicting split is refused', async ({ page }) => {
  const s = await setup(page);
  const { entry, legAmounts } = await startSplitWithdrawal(page, s, `Tabs split deposit ${s.suffix}`);

  // Deposit mirrors the split: money into checking, the category balancing it.
  await entry.getByRole('tab', { name: 'Deposit', exact: true }).click();
  await expect(legAmounts.nth(0)).toHaveValue('12.40');
  await expect(legAmounts.nth(1)).toHaveValue('-12.40');

  // Amounts typed the other way under Deposit are refused, naming the tab
  // that fits; nothing is sent.
  await legAmounts.nth(0).fill('-12.40');
  await legAmounts.nth(1).fill('12.40');
  let posted = 0;
  page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/transactions') posted += 1; });
  await entry.getByRole('button', { name: 'Save transaction', exact: true }).click();
  await expect(entry.getByRole('alert')).toContainText(`These amounts move money the other way for ${s.checking.name}: choose “Withdrawal”`);
  expect(posted).toBe(0);

  // Withdrawal agrees with those amounts and leaves them; Deposit flips them again.
  await entry.getByRole('tab', { name: 'Withdrawal', exact: true }).click();
  await expect(legAmounts.nth(0)).toHaveValue('-12.40');
  await entry.getByRole('tab', { name: 'Deposit', exact: true }).click();
  await expect(legAmounts.nth(0)).toHaveValue('12.40');
  await expect(entry.getByRole('alert')).toHaveCount(0);

  const postings = savedPostings(page);
  await entry.getByRole('button', { name: 'Save transaction', exact: true }).click();
  expect(await postings()).toEqual(expect.arrayContaining([[s.checking.id, '1240'], [s.category.id, '-1240']]));
});

test('a split follows the account chosen on Transfer back to the cash tabs', async ({ page }) => {
  const s = await setup(page);
  const { entry, legAccounts, legAmounts } = await startSplitWithdrawal(page, s, `Tabs split card ${s.suffix}`);

  await entry.getByRole('tab', { name: 'Transfer', exact: true }).click();
  await entry.getByRole('combobox', { name: 'From account', exact: true }).selectOption({ label: s.card.name });
  await expect.poll(() => tabNames(entry)).toEqual(['Charge', 'Credit', 'Transfer', 'Investment']);

  // Back on Charge the split posts to the card, as the label says.
  await entry.getByRole('tab', { name: 'Charge', exact: true }).click();
  await expect(legAccounts.nth(0)).toHaveValue(String(s.card.id));
  await expect(legAmounts.nth(0)).toHaveValue('-12.40');

  // Inside the split the entry's leg leads: moving it to checking relabels the tabs.
  await legAccounts.nth(0).selectOption({ label: s.checking.name });
  await expect.poll(() => tabNames(entry)).toEqual(['Withdrawal', 'Deposit', 'Transfer', 'Investment']);
  await legAccounts.nth(0).selectOption({ label: s.card.name });
  await expect(selectedTab(entry)).toHaveText('Charge');

  const postings = savedPostings(page);
  await entry.getByRole('button', { name: 'Save transaction', exact: true }).click();
  const saved = await postings();
  expect(saved).toEqual(expect.arrayContaining([[s.card.id, '-1240'], [s.category.id, '1240']]));
  expect(saved.some(([account]) => account === s.checking.id)).toBe(false);
});

// A split started before any account is chosen: its first account leg
// becomes the entry's account, so both rules above still hold (#510 review).
// The second leg is savings: a split leg on a category needs a currency the
// split editor cannot yet choose, a separate gap older than #506.
async function startSplitFirst(page: Page, s: Awaited<ReturnType<typeof setup>>, description: string) {
  await page.goto('/app');
  await page.getByRole('list', { name: 'Shortcuts', exact: true }).getByRole('button', { name: 'New transaction', exact: true }).click();
  const entry = entryDialog(page);
  await entry.getByLabel('Date', { exact: true }).fill(todayISO());
  await entry.getByLabel('Description', { exact: true }).fill(description);
  await entry.getByRole('button', { name: 'Use split entry', exact: true }).click();
  const legAccounts = entry.getByRole('combobox', { name: 'Account', exact: true });
  const legAmounts = entry.getByRole('textbox', { name: 'Amount', exact: true });
  await legAccounts.nth(0).selectOption(String(s.checking.id));
  await legAmounts.nth(0).fill('-12.40');
  await legAccounts.nth(1).selectOption(String(s.savings.id));
  await legAmounts.nth(1).fill('12.40');
  return { entry, legAccounts, legAmounts };
}

test('a split started before any account follows Deposit', async ({ page }) => {
  const s = await setup(page);
  const { entry, legAmounts } = await startSplitFirst(page, s, `Tabs split first deposit ${s.suffix}`);
  await entry.getByRole('tab', { name: 'Deposit', exact: true }).click();
  await expect(legAmounts.nth(0)).toHaveValue('12.40');
  const postings = savedPostings(page);
  await entry.getByRole('button', { name: 'Save transaction', exact: true }).click();
  expect(await postings()).toEqual(expect.arrayContaining([[s.checking.id, '1240'], [s.savings.id, '-1240']]));
});

test('a split started before any account follows a new From account back to Charge', async ({ page }) => {
  const s = await setup(page);
  const { entry, legAccounts } = await startSplitFirst(page, s, `Tabs split first card ${s.suffix}`);
  await entry.getByRole('tab', { name: 'Transfer', exact: true }).click();
  const from = entry.getByRole('combobox', { name: 'From account', exact: true });
  await expect(from).toHaveValue(String(s.checking.id));
  await from.selectOption(String(s.card.id));
  await entry.getByRole('tab', { name: 'Charge', exact: true }).click();
  await expect(legAccounts.nth(0)).toHaveValue(String(s.card.id));
  const postings = savedPostings(page);
  await entry.getByRole('button', { name: 'Save transaction', exact: true }).click();
  const saved = await postings();
  expect(saved).toEqual(expect.arrayContaining([[s.card.id, '-1240'], [s.savings.id, '1240']]));
  expect(saved.some(([account]) => account === s.checking.id)).toBe(false);
});

test('a confirmation shown before a tab change is gone after it; saving asks again', async ({ page }) => {
  const s = await setup(page);
  const newPayee = `Tabs unknown shop ${s.suffix}`;
  await page.goto('/app');
  await page.locator('#main-content').focus();
  await page.keyboard.press('n');
  const entry = entryDialog(page);
  await entry.getByLabel('Date', { exact: true }).fill(todayISO());
  await entry.getByLabel('Description', { exact: true }).fill(`Tabs approval ${s.suffix}`);
  await entry.getByRole('combobox', { name: 'Account', exact: true }).selectOption({ label: s.checking.name });
  await entry.getByLabel('Amount', { exact: true }).fill('9.00');
  await chooseCategory(entry, s.category.name);
  await entry.getByRole('combobox', { name: 'Payee', exact: true }).fill(newPayee);
  await entry.getByRole('button', { name: 'Save transaction', exact: true }).click();
  const confirmation = entry.getByRole('group', { name: `No payee is named “${newPayee}” yet.`, exact: false });
  await expect(confirmation).toBeVisible();

  const posts: number[] = [];
  page.on('request', request => {
    if (request.method() === 'POST' && ['/api/v1/transactions', '/api/v1/payees'].includes(new URL(request.url()).pathname)) posts.push(1);
  });
  await entry.getByRole('tab', { name: 'Deposit', exact: true }).click();
  await expect(confirmation).toHaveCount(0);
  await entry.getByRole('tab', { name: 'Withdrawal', exact: true }).click();
  await expect(confirmation).toHaveCount(0);
  expect(posts).toEqual([]);
  await entry.getByRole('button', { name: 'Save transaction', exact: true }).click();
  await expect(confirmation).toBeVisible();
  expect(posts).toEqual([]);
});

test.describe('on a touch phone', () => {
  test.use({ hasTouch: true, viewport: { width: 390, height: 844 } });

  test('journey 17: the full-screen sheet fits four tabs in Russian, in the dark theme', async ({ page }) => {
    await readyForLedger(page);
    await page.evaluate(() => {
      localStorage.setItem('rekenraam-theme', 'dark');
      localStorage.setItem('PARAGLIDE_LOCALE', 'ru');
    });
    await page.goto('/app');
    await page.getByRole('list', { name: 'Ярлыки', exact: true }).getByRole('button', { name: 'Новая транзакция', exact: true }).tap();
    const entry = page.getByRole('dialog', { name: 'Новая транзакция', exact: true });
    await expect(entry).toBeVisible();
    await expect.poll(() => entry.evaluate(node => Math.round(node.getBoundingClientRect().width))).toBe(390);
    await expect.poll(() => tabNames(entry)).toEqual(['Списание', 'Пополнение', 'Перевод', 'Инвестиции']);
    for (const tab of await entry.getByRole('tab').all()) {
      const box = (await tab.boundingBox())!;
      expect(box.x).toBeGreaterThanOrEqual(0);
      expect(box.x + box.width).toBeLessThanOrEqual(390);
      expect(box.height).toBeGreaterThanOrEqual(44);
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
    await entry.getByRole('tab', { name: 'Перевод', exact: true }).tap();
    await expect(entry.getByRole('tab', { name: 'Перевод', exact: true })).toHaveAttribute('aria-selected', 'true');
    await expectNoAccessibilityViolations(page, 'phone entry tabs ru dark');
  });
});
