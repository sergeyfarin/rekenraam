import { expect, test, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * #185 slice 2: same-day reordering is a mode, off by default. A move places
 * an entry beside its visible same-day neighbour everywhere both appear; an
 * account's "Only in this account" switch reorders that register alone, for a
 * statement that lists the day differently. Phone-sized screen.
 */

test.use({ viewport: { width: 390, height: 844 } });

type Seeded = Awaited<ReturnType<typeof seed>>;

async function seed(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const checking = await createCashAccount(page, csrfToken, `Reorder checking ${suffix}`, currencyID);
  const savings = await createCashAccount(page, csrfToken, `Reorder savings ${suffix}`, currencyID);
  const category = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: `Reorder groceries ${suffix}`, category_type: 'expense'
  });
  // Every account and the category are new, so their registers hold only
  // this test's entries; the main list is filtered to one of them.
  const day = todayISO();
  const posting = (accountID: number, value: string) => ({
    account_id: accountID, commodity_id: currencyID, quantity_value: value, quantity_scale: 2
  });
  const create = (description: string, postings: ReturnType<typeof posting>[], kind = 'ordinary') =>
    apiJSON<{ id: number }>(page, 'POST', '/api/v1/transactions', csrfToken, {
      transaction_date: day, transaction_kind: kind, status: 'posted', description,
      journal_entries: [{ entry_date: day, entry_kind: kind === 'transfer' ? 'transfer_leg' : 'ordinary', postings }]
    });
  return { suffix, day, checking, savings, category, create, posting };
}

/** An account's transactions on the seeded day, oldest first. */
async function registerOrder(page: Page, accountID: number): Promise<number[]> {
  const register = await apiJSON<{ entries: Array<{ transaction_id: number }> }>(
    page, 'GET', `/api/v1/accounts/${accountID}/register`);
  return register.entries.map((entry) => entry.transaction_id).reverse();
}

function row(page: Page, text: string) {
  return page.getByRole('row').filter({ hasText: text });
}

test('reordering on the main list applies in every account both entries share', async ({ page }) => {
  const s: Seeded = await seed(page);
  const coffee = await s.create(`Coffee ${s.suffix}`, [s.posting(s.checking.id, '-300'), s.posting(s.category.id, '300')]);
  const salary = await s.create(`Salary ${s.suffix}`, [s.posting(s.savings.id, '-5000'), s.posting(s.checking.id, '5000')], 'transfer');

  await page.goto(`/app/ledger/transactions?account_id=${s.checking.id}`);
  await expect(row(page, `Salary ${s.suffix}`)).toBeVisible();
  // Off by default: no arrows until the mode starts.
  await expect(page.getByRole('button', { name: 'Move earlier in the day' })).toHaveCount(0);
  await page.getByRole('button', { name: 'Reorder' }).click();
  await expect(page.getByText('The order changes on every screen that shows both entries.')).toBeVisible();
  // The newest is on top; the coffee cannot move earlier than the start of the day.
  await expect(row(page, `Coffee ${s.suffix}`).getByRole('button', { name: 'Move earlier in the day' })).toBeDisabled();
  await row(page, `Salary ${s.suffix}`).getByRole('button', { name: 'Move earlier in the day' }).click();

  await expect.poll(() => registerOrder(page, s.checking.id)).toEqual([salary.id, coffee.id]);
  await page.getByRole('button', { name: 'Done' }).click();
  await expect(page.getByRole('button', { name: 'Move earlier in the day' })).toHaveCount(0);
});

test('only in this account reorders one register and leaves the other', async ({ page }) => {
  const s: Seeded = await seed(page);
  const first = await s.create(`Transfer one ${s.suffix}`, [s.posting(s.checking.id, '-1000'), s.posting(s.savings.id, '1000')], 'transfer');
  const second = await s.create(`Transfer two ${s.suffix}`, [s.posting(s.checking.id, '-2000'), s.posting(s.savings.id, '2000')], 'transfer');

  await page.goto(`/app/ledger/accounts/${s.savings.id}/register`);
  await page.getByRole('button', { name: 'Reorder' }).click();
  await page.getByLabel('Only in this account').check();
  await expect(page.getByText('Other screens keep their order.')).toBeVisible();
  await row(page, `Transfer two ${s.suffix}`).getByRole('button', { name: 'Move earlier in the day' }).click();

  await expect.poll(() => registerOrder(page, s.savings.id)).toEqual([second.id, first.id]);
  expect(await registerOrder(page, s.checking.id)).toEqual([first.id, second.id]);
});

test('reordering in a category applies in the account both entries share', async ({ page }) => {
  const s: Seeded = await seed(page);
  const bread = await s.create(`Bread ${s.suffix}`, [s.posting(s.checking.id, '-250'), s.posting(s.category.id, '250')]);
  const milk = await s.create(`Milk ${s.suffix}`, [s.posting(s.checking.id, '-120'), s.posting(s.category.id, '120')]);

  await page.goto(`/app/ledger/categories/${s.category.id}`);
  await page.getByRole('button', { name: 'Reorder' }).click();
  const later = row(page, `Bread ${s.suffix}`).getByRole('button', { name: 'Move later in the day' });
  await expect(later).toBeVisible();
  await expect(page.getByLabel('Only in this account')).toHaveCount(0);
  await later.click();

  await expect.poll(() => registerOrder(page, s.category.id)).toEqual([milk.id, bread.id]);
  expect(await registerOrder(page, s.checking.id)).toEqual([milk.id, bread.id]);
});

test('each posting of a split is its own register row and moves alone', async ({ page }) => {
  const s: Seeded = await seed(page);
  // Three checking postings in one transaction. Prefix sums of these amounts
  // never equal another posting's amount, so each row's text is unique.
  const split = await s.create(`Split ${s.suffix}`, [
    s.posting(s.checking.id, '-101'), s.posting(s.checking.id, '-202'), s.posting(s.checking.id, '-404'),
    s.posting(s.category.id, '707')
  ]);
  const lineOrder = async () => {
    const register = await apiJSON<{ entries: Array<{ transaction_id: number; posting: { posting_line_id: number; quantity_value: string } }> }>(
      page, 'GET', `/api/v1/accounts/${s.checking.id}/register`);
    return register.entries.map((entry) => entry.posting.quantity_value).reverse();
  };
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  page.on('console', (message) => { if (message.type() === 'error') errors.push(message.text()); });

  await page.goto(`/app/ledger/accounts/${s.checking.id}/register`);
  const rows = page.getByRole('row').filter({ hasText: `Split ${s.suffix}` });
  await expect(rows).toHaveCount(3);
  for (const amount of ['1.01', '2.02', '4.04']) await expect(rows.filter({ hasText: amount })).toHaveCount(1);
  expect(await lineOrder()).toEqual(['-101', '-202', '-404']);

  await page.getByRole('button', { name: 'Reorder' }).click();
  // The oldest posting is the bottom row; moving it later swaps only it.
  const earliest = rows.filter({ hasText: '1.01' }).getByRole('button', { name: 'Move later in the day' });
  await earliest.click();
  await expect.poll(lineOrder).toEqual(['-202', '-101', '-404']);
  await expect(rows).toHaveCount(3);
  // The refreshed register kept the row, so focus stayed on its control.
  await expect(earliest).toBeFocused();
  await expect(rows.nth(1)).toContainText('1.01');
  expect(split.id).toBeGreaterThan(0);
  expect(errors).toEqual([]);
});
