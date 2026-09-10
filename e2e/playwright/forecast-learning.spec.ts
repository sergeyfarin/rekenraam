import { expect, type Page, test } from '@playwright/test';
import { apiJSON, csrfTokenFor } from './support/api';
import { daysFromTodayISO, todayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';
import { expectNoAccessibilityViolations } from './support/a11y';

/** Posts one ordinary purchase funded entirely from the cash account. */
async function postPurchase(
  page: Page,
  date: string,
  accountID: number,
  categoryID: number,
  commodityID: number,
  amount: string
) {
  await apiJSON(page, 'POST', '/api/v1/transactions', await csrfTokenFor(page), {
    status: 'posted',
    transaction_date: date,
    description: 'learned spending fixture',
    journal_entries: [{
      entry_date: date,
      postings: [
        { account_id: accountID, commodity_id: commodityID, quantity_value: `-${amount}`, quantity_scale: 2 },
        { account_id: categoryID, commodity_id: commodityID, quantity_value: amount, quantity_scale: 2 }
      ]
    }]
  }, [201]);
}

/** The Monday `weeks` complete weeks before this week's Monday, as YYYY-MM-DD. */
function mondayWeeksAgo(weeks: number): string {
  const today = new Date(`${todayISO()}T00:00:00Z`);
  const monday = new Date(today);
  monday.setUTCDate(monday.getUTCDate() - ((monday.getUTCDay() + 6) % 7));
  monday.setUTCDate(monday.getUTCDate() - 7 * weeks);
  return monday.toISOString().slice(0, 10);
}

function weekdayWeeksAgo(weeks: number, offsetDays: number): string {
  const start = new Date(`${mondayWeeksAgo(weeks)}T00:00:00Z`);
  start.setUTCDate(start.getUTCDate() + offsetDays);
  return start.toISOString().slice(0, 10);
}

test('[acceptance] estimated spending is opt-in, explained and reversible', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `Learned spending ${suffix}`, currencyID, mondayWeeksAgo(60));
  const categories = await apiJSON<{ categories: Array<{ id: number; code?: string; category_type: string; allows_postings: boolean }> }>(page, 'GET', '/api/v1/categories');
  const groceries = categories.categories.find((item) => item.code === 'expense_food_groceries' && item.allows_postings);
  const income = categories.categories.find((item) => item.code === 'income_salary_wages' && item.allows_postings);
  if (!groceries || !income) throw new Error('expected seeded groceries and income categories');

  // Fund the account, then record twenty complete weeks of grocery spending.
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    status: 'posted',
    transaction_date: mondayWeeksAgo(21),
    description: 'learned spending opening',
    journal_entries: [{
      entry_date: mondayWeeksAgo(21),
      postings: [
        { account_id: account.id, commodity_id: currencyID, quantity_value: '500000', quantity_scale: 2 },
        { account_id: income.id, commodity_id: currencyID, quantity_value: '-500000', quantity_scale: 2 }
      ]
    }]
  }, [201]);
  const historyWeeks = 20;
  for (let week = historyWeeks; week >= 1; week -= 1) {
    await postPurchase(page, weekdayWeeksAgo(week, 2), account.id, groceries.id, currencyID, '7000');
  }

  // Other specs share this database, so the invariant is that viewing
  // estimates changes nothing, not an absolute transaction count.
  const ledgerBefore = await apiJSON<{ transactions: unknown[] }>(page, 'GET', '/api/v1/transactions?limit=200');

  const base = `/app/forecast?horizon_days=30&account_id=${account.id}&include_descendants=false`;
  await page.goto(base);
  await expect(page.getByRole('heading', { name: 'Projected balances' })).toBeVisible();

  // Off by default: no estimate anywhere on the screen.
  const toggle = page.getByTestId('forecast-learning-toggle');
  await expect(toggle).not.toBeChecked();
  await expect(page.getByTestId('forecast-learning-status')).toHaveCount(0);
  const table = page.getByRole('table').first();
  await expect(table.getByRole('columnheader', { name: 'With estimated spending' })).toHaveCount(0);

  // Opting in requires the owner to confirm when recording became complete.
  await toggle.check();
  await expect(page.getByText('These figures are estimates, not saved bills.', { exact: false })).toBeVisible();
  await page.getByTestId('forecast-learning-history').fill(mondayWeeksAgo(historyWeeks));
  await page.getByRole('button', { name: 'Apply', exact: true }).click();

  await expect(page).toHaveURL(/spending_model=adaptive_v1/);
  await expect(page).toHaveURL(/history_complete_from=\d{4}-\d{2}-\d{2}/);
  const status = page.getByTestId('forecast-learning-status');
  await expect(status).toBeVisible();
  await expect(status).toContainText('This is not a claim about future accuracy.');

  // The third curve and column appear, and the model is explained rather than
  // presented as a saved bill.
  await expect(table.getByRole('columnheader', { name: 'With estimated spending' })).toBeVisible();
  const groupCard = page.getByRole('article').filter({ hasText: 'Weekly' }).first();
  await expect(groupCard).toContainText('Model:');
  await expect(groupCard).toContainText('Trained on');
  await expect(groupCard).toContainText('historical variation, not a range the future is expected to fall in');
  await expect(groupCard).toContainText('Longer projections continue the same assumptions and were not tested.');
  // No probability language is ever shown.
  await expect(page.getByText(/confidence|probability|95%/i)).toHaveCount(0);

  await expectNoAccessibilityViolations(page);

  // Mobile stays within the viewport and keyboard focus reaches the toggle.
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.getByTestId('forecast-learning-toggle').focus();
  await expect(page.getByTestId('forecast-learning-toggle')).toBeFocused();
  await page.setViewportSize({ width: 1280, height: 900 });

  // Turning it off restores exactly the core forecast, with no ledger change.
  await page.getByTestId('forecast-learning-toggle').uncheck();
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  await expect(page).not.toHaveURL(/spending_model/);
  await expect(page.getByTestId('forecast-learning-status')).toHaveCount(0);
  await expect(page.getByRole('table').first().getByRole('columnheader', { name: 'With estimated spending' })).toHaveCount(0);

  // Viewing estimates never writes: the ledger is exactly as it was.
  const ledgerAfter = await apiJSON<{ transactions: unknown[] }>(page, 'GET', '/api/v1/transactions?limit=200');
  expect(ledgerAfter.transactions).toEqual(ledgerBefore.transactions);
});

test('[acceptance] a category without enough history says why', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `Sparse learning ${suffix}`, currencyID, mondayWeeksAgo(60));
  const categories = await apiJSON<{ categories: Array<{ id: number; code?: string; allows_postings: boolean }> }>(page, 'GET', '/api/v1/categories');
  const groceries = categories.categories.find((item) => item.code === 'expense_food_groceries' && item.allows_postings);
  if (!groceries) throw new Error('expected seeded groceries category');

  await postPurchase(page, daysFromTodayISO(-10), account.id, groceries.id, currencyID, '4000');

  const url = `/app/forecast?horizon_days=30&account_id=${account.id}&include_descendants=false`
    + `&spending_model=adaptive_v1&history_complete_from=${mondayWeeksAgo(3)}`;
  await page.goto(url);

  const status = page.getByTestId('forecast-learning-status');
  await expect(status).toBeVisible();
  await expect(status).toContainText('No category could be estimated');
  await expect(status).toContainText('16 complete weeks or months are required.');
  // An unavailable overlay shows no estimated column at all, rather than a
  // reassuring zero-spending curve.
  await expect(page.getByRole('table').first().getByRole('columnheader', { name: 'With estimated spending' })).toHaveCount(0);
});

test('[acceptance] a seasonal request explains the 36-month requirement', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `Seasonal learning ${suffix}`, currencyID, mondayWeeksAgo(60));
  const categories = await apiJSON<{ categories: Array<{ id: number; code?: string; allows_postings: boolean }> }>(page, 'GET', '/api/v1/categories');
  const groceries = categories.categories.find((item) => item.code === 'expense_food_groceries' && item.allows_postings);
  if (!groceries) throw new Error('expected seeded groceries category');

  for (let week = 20; week >= 1; week -= 1) {
    await postPurchase(page, weekdayWeeksAgo(week, 2), account.id, groceries.id, currencyID, '6000');
  }

  const url = `/app/forecast?horizon_days=30&account_id=${account.id}&include_descendants=false`
    + `&spending_model=adaptive_v1&history_complete_from=${mondayWeeksAgo(20)}`
    + `&expense_category_id=${groceries.id}&expense_pattern=${groceries.id}:annual_seasonal`;
  await page.goto(url);

  await expect(page.getByTestId('forecast-learning-status')).toContainText('No category could be estimated');
  // The reason appears both as the overall status and against the category.
  await expect(page.getByTestId('forecast-learning-status')).toContainText('36 consecutive complete months');
  await expect(page.getByText('36 consecutive complete months', { exact: false }).first()).toBeVisible();
});

test('[acceptance] a malformed learned-spending URL is rejected, not ignored', async ({ page }) => {
  await readyForLedger(page);
  // A model parameter without the model would silently do nothing if accepted.
  await page.goto('/app/forecast?horizon_days=30&history_complete_from=2024-01-01');
  await expect(page.getByText('These forecast filters are invalid', { exact: false })).toBeVisible();
});
