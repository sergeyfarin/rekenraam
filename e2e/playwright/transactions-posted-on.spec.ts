import { expect, test, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { daysFromTodayISO, todayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';

/**
 * #187: an account posting may post on another day than the transaction date.
 * A delayed card posting moves the whole entry to the posting day; a two-day
 * transfer posts each leg on its own day, bridged through transfer clearing.
 * Phone-sized screen.
 */

test.use({ viewport: { width: 390, height: 844 } });

type RegisterEntry = {
  transaction_date: string;
  entry_date: string;
  description: string;
  posting: { quantity_value: string };
  running_balance: { normal_quantity_value: string };
};

async function register(page: Page, accountID: number): Promise<RegisterEntry[]> {
  const result = await apiJSON<{ entries: RegisterEntry[] }>(page, 'GET', `/api/v1/accounts/${accountID}/register`);
  return result.entries;
}

type Journal = { journal_entries: Array<{ entry_date: string; postings: Array<{ account_id: number; account_system_role?: string | null; quantity_value: string }> }> };

function journalShape(journal: Journal) {
  return journal.journal_entries.map((entry) => ({
    date: entry.entry_date,
    postings: entry.postings.map((posting) => posting.account_system_role ?? posting.account_id).map(String).sort()
  }));
}

test('a card purchase posted the next day lists on its posting day', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const card = await createCashAccount(page, csrfToken, `Posted card ${suffix}`, currencyID);
  const categoryName = `Posted groceries ${suffix}`;
  await apiJSON(page, 'POST', '/api/v1/categories', csrfToken, { name: categoryName, category_type: 'expense' });
  const description = `Late grocery ${suffix}`;
  const today = todayISO();
  const tomorrow = daysFromTodayISO(1);

  await page.goto(`/app/ledger/accounts/${card.id}/register`);
  await page.getByRole('button', { name: 'Add transaction' }).click();
  const form = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Add transaction' }) });
  await form.getByLabel('Date', { exact: true }).fill(today);
  await form.getByRole('button', { name: 'Posted on another day' }).click();
  await form.getByLabel('Posted on').fill(tomorrow);
  await form.getByLabel('Description').fill(description);
  await form.getByLabel('Amount').fill('-25.00');
  await form.getByPlaceholder('Search categories').fill(categoryName);
  await form.locator('ul[role="listbox"] button[role="option"]').filter({ hasText: categoryName }).click();
  await form.getByRole('button', { name: 'Add transaction' }).click();

  await expect.poll(async () => (await register(page, card.id)).map((entry) => [entry.transaction_date, entry.entry_date, entry.posting.quantity_value]))
    .toEqual([[today, tomorrow, '-2500']]);
  const row = page.getByRole('row').filter({ hasText: description });
  await expect(row).toContainText(tomorrow);
  await expect(row).toContainText(`Transaction date ${today}`);
});

test('a two-day transfer posts each leg on its day and survives an edit', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const checkingName = `Posted checking ${suffix}`;
  const savingsName = `Posted savings ${suffix}`;
  const checking = await createCashAccount(page, csrfToken, checkingName, currencyID);
  const savings = await createCashAccount(page, csrfToken, savingsName, currencyID);
  const description = `Slow transfer ${suffix}`;
  const today = todayISO();
  const arrival = daysFromTodayISO(2);

  await page.goto(`/app/ledger/transactions?account_id=${checking.id}`);
  await page.getByRole('button', { name: 'Add transaction' }).click();
  const form = page.getByRole('complementary', { name: 'Actions' });
  await form.getByLabel('Date', { exact: true }).fill(today);
  await form.getByLabel('Description').fill(description);
  await form.getByRole('button', { name: 'Use split entry' }).click();
  const accounts = form.getByRole('combobox', { name: 'Account', exact: true });
  const amounts = form.getByRole('textbox', { name: 'Amount', exact: true });
  await accounts.nth(0).selectOption({ label: checkingName });
  await amounts.nth(0).fill('-100.00');
  await accounts.nth(1).selectOption({ label: savingsName });
  await amounts.nth(1).fill('100.00');
  await form.getByRole('button', { name: 'Posted on another day' }).nth(1).click();
  await form.getByLabel('Posted on').fill(arrival);
  await form.getByRole('button', { name: 'Add transaction' }).click();

  await expect.poll(async () => (await register(page, savings.id)).map((entry) => [entry.entry_date, entry.running_balance.normal_quantity_value]))
    .toEqual([[arrival, '10000']]);
  expect((await register(page, checking.id)).map((entry) => [entry.entry_date, entry.running_balance.normal_quantity_value]))
    .toEqual([[today, '-10000']]);

  const list = await apiJSON<{ transactions: Array<{ id: number; description: string }> }>(page, 'GET', `/api/v1/transactions?account_id=${checking.id}`);
  const transactionID = list.transactions.find((tx) => tx.description === description)!.id;
  const expected = [
    { date: today, postings: [String(checking.id), 'transfer_clearing'].sort() },
    { date: arrival, postings: [String(savings.id), 'transfer_clearing'].sort() }
  ];
  const read = () => apiJSON<Journal & { version_id: number }>(page, 'GET', `/api/v1/transactions/${transactionID}`);
  expect(journalShape(await read())).toEqual(expected);

  // The main list is dated by the transaction and says when the rest posts.
  const row = page.getByRole('row').filter({ hasText: description });
  await expect(row).toContainText(`Posted ${arrival}`);

  // Reopening the transfer keeps each leg's day; saving it again must not
  // flatten the journal onto the transaction date.
  const before = (await read()).version_id;
  // The detail panel names the leg that posts later.
  const detail = page.getByRole('complementary', { name: 'Actions' });
  await expect(detail.getByRole('listitem').filter({ hasText: savingsName })).toContainText(`Posted ${arrival}`);
  await expect(detail.getByRole('listitem').filter({ hasText: checkingName })).not.toContainText('Posted 2');
  // Saving opens the detail panel, which covers the list on a phone.
  await page.getByRole('button', { name: 'Close', exact: true }).click();
  await row.getByRole('button', { name: 'Edit', exact: true }).click();
  const edit = page.getByRole('complementary', { name: 'Actions' });
  await expect(edit.getByRole('heading', { name: 'Edit transaction' })).toBeVisible();
  // A transfer between two accounts reopens in Transfer mode (#481).
  await expect(edit.getByLabel('Arrives on')).toHaveValue(arrival);
  await edit.getByLabel('Description').fill(`${description} edited`);
  await edit.getByRole('button', { name: 'Save transaction' }).click();
  await expect.poll(async () => (await read()).version_id).not.toBe(before);
  expect(journalShape(await read())).toEqual(expected);
});

test('reorder mode moves a posting past the day edge to the next day after confirming', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const checking = await createCashAccount(page, csrfToken, `Edge checking ${suffix}`, currencyID);
  const savings = await createCashAccount(page, csrfToken, `Edge savings ${suffix}`, currencyID);
  const description = `Edge transfer ${suffix}`;
  const today = todayISO();
  const tomorrow = daysFromTodayISO(1);
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_date: today, status: 'posted', description,
    journal_entries: [{ entry_date: today, postings: [
      { account_id: checking.id, commodity_id: currencyID, quantity_value: '-5000', quantity_scale: 2 },
      { account_id: savings.id, commodity_id: currencyID, quantity_value: '5000', quantity_scale: 2 }
    ] }]
  });

  await page.goto(`/app/ledger/accounts/${savings.id}/register`);
  await page.getByRole('button', { name: 'Reorder' }).click();
  const row = page.getByRole('row').filter({ hasText: description });
  // The only entry of its day: the arrows offer the adjacent days instead.
  const nextDay = row.getByRole('button', { name: 'Move to the next day' });
  await expect(nextDay).toBeVisible();
  await expect(row.getByRole('button', { name: 'Move later in the day' })).toHaveCount(0);
  await nextDay.click();
  await expect(page.getByText(`Move this posting to ${tomorrow}?`)).toBeVisible();
  const confirm = page.getByRole('button', { name: `Move to ${tomorrow}` });
  await expect(confirm).toBeFocused();
  await confirm.click();

  await expect.poll(async () => (await register(page, savings.id)).map((entry) => [entry.transaction_date, entry.entry_date]))
    .toEqual([[today, tomorrow]]);
  expect((await register(page, checking.id)).map((entry) => entry.entry_date)).toEqual([today]);
  await expect(row).toContainText(`Transaction date ${today}`);

  // Back again: cancelling leaves it, confirming returns it to one day.
  await row.getByRole('button', { name: 'Move to the previous day' }).click();
  await page.getByRole('button', { name: 'Cancel', exact: true }).click();
  expect((await register(page, savings.id)).map((entry) => entry.entry_date)).toEqual([tomorrow]);
  await row.getByRole('button', { name: 'Move to the previous day' }).click();
  await page.getByRole('button', { name: `Move to ${today}` }).click();
  await expect.poll(async () => (await register(page, savings.id)).map((entry) => entry.entry_date)).toEqual([today]);
});
