import { readFile } from 'node:fs/promises';
import { expect, type Page, test } from '@playwright/test';
import { apiJSON, csrfTokenFor } from './support/api';
import { todayISO } from './support/dates';
import { createCashAccount, createLiabilityAccount, ensureCurrency, readyForLedger } from './support/ledger';

test('spending report ranks categories, nets refunds, and ignores transfers', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const today = todayISO();

  const checking = await createCashAccount(page, csrfToken, `Reports checking ${suffix}`, currencyID);
  const savings = await createCashAccount(page, csrfToken, `Reports savings ${suffix}`, currencyID);

  const categories = await apiJSON<{
    categories: Array<{ id: number; code?: string; allows_postings: boolean }>;
  }>(page, 'GET', '/api/v1/categories');
  const category = (code: string) => {
    const found = categories.categories.find((item) => item.code === code && item.allows_postings);
    if (!found) throw new Error(`seeded category ${code} not found`);
    return found.id;
  };
  const groceries = category('expense_food_groceries');
  const fuel = category('expense_transport_fuel');

  // 80.00 groceries, then a 30.00 refund against the same category: the report
  // must show 50.00, not 110.00.
  await postTransaction(page, today, checking.id, groceries, currencyID, 8000);
  await postTransaction(page, today, checking.id, groceries, currencyID, -3000);
  // 60.00 fuel, so groceries (50.00) must rank below it.
  await postTransaction(page, today, checking.id, fuel, currencyID, 6000);
  // A transfer between the user's own accounts has no category posting and must
  // never appear as spending.
  await postTransfer(page, today, checking.id, savings.id, currencyID, 25000);

  await page.goto(`/app/reports?view=spending&group_by=category&start_date=${today}&end_date=${today}&bucket=month`);

  const table = page.getByRole('table');
  await expect(table).toBeVisible();
  const rows = table.locator('tbody tr');
  await expect(rows).toHaveCount(2);

  // Fuel 60.00 outranks the net 50.00 of groceries.
  await expect(rows.nth(0)).toContainText('Fuel');
  await expect(rows.nth(0)).toContainText('60.00');
  await expect(rows.nth(1)).toContainText('Groceries');
  await expect(rows.nth(1)).toContainText('50.00');

  // Neither the refund nor the transfer produced a row of its own.
  await expect(table).not.toContainText('110.00');
  await expect(table).not.toContainText('250.00');

  // Switching the dimension changes only group_by and keeps the date range.
  await page.getByRole('button', { name: 'Payee', exact: true }).click();
  await expect(page).toHaveURL(/group_by=payee/);
  await expect(page).toHaveURL(new RegExp(`start_date=${today}`));

  // Income is a separate direction, reported as a positive magnitude.
  await page.getByRole('button', { name: 'Category', exact: true }).click();
  await page.getByRole('button', { name: 'Income', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Where money came from' })).toBeVisible();
});

test('report filters narrow the result and travel in the URL', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const today = todayISO();

  const checking = await createCashAccount(page, csrfToken, `Filter checking ${suffix}`, currencyID);
  const savings = await createCashAccount(page, csrfToken, `Filter savings ${suffix}`, currencyID);

  const categories = await apiJSON<{
    categories: Array<{ id: number; code?: string; allows_postings: boolean }>;
  }>(page, 'GET', '/api/v1/categories');
  const category = (code: string) => {
    const found = categories.categories.find((item) => item.code === code && item.allows_postings);
    if (!found) throw new Error(`seeded category ${code} not found`);
    return found.id;
  };

  // Checking spends 110.00 across two categories; savings spends 90.00. Every
  // assertion below can tell a filtered total from an unfiltered one.
  await postTransaction(page, today, checking.id, category('expense_food_groceries'), currencyID, 4000);
  await postTransaction(page, today, checking.id, category('expense_transport_fuel'), currencyID, 7000);
  await postTransaction(page, today, savings.id, category('expense_housing_rent_mortgage'), currencyID, 9000);

  await page.goto(`/app/reports?view=spending&group_by=category&start_date=${today}&end_date=${today}&bucket=month`);

  const table = page.getByRole('table');
  await expect(table.locator('tbody tr')).toHaveCount(3);

  // Selecting one category narrows the report and writes the dimension into the
  // URL, so the link reproduces exactly this result.
  await openFilter(page, 'Categories');
  await page.getByRole('checkbox', { name: 'Groceries', exact: true }).check();

  await expect(page).toHaveURL(new RegExp(`category_id=${category('expense_food_groceries')}`));
  await expect(table.locator('tbody tr')).toHaveCount(1);
  await expect(table).toContainText('Groceries');
  await expect(table).not.toContainText('Fuel');

  // The active selection is named on the page, so a shared link explains its
  // own scope without opening a control.
  await expect(page.getByRole('region', { name: 'Filters' })).toContainText('Groceries');

  // A reload proves the URL alone carries the filter — nothing lives in memory.
  await page.reload();
  await expect(page.getByRole('table').locator('tbody tr')).toHaveCount(1);

  // Net worth cannot express a category, so that control is not offered there;
  // the selection survives in the URL for the trip back.
  await page.getByRole('button', { name: 'Net worth', exact: true }).click();
  await expect(page.getByRole('region', { name: 'Filters' })).not.toContainText('Categories');
  await expect(page).toHaveURL(/category_id=/);

  await page.getByRole('button', { name: 'Spending', exact: true }).click();
  await expect(page.getByRole('table').locator('tbody tr')).toHaveCount(1);

  // Clearing drops the dimension from the URL rather than writing it empty.
  await page.getByRole('button', { name: 'Clear all filters' }).click();
  await expect(page).not.toHaveURL(/category_id=/);
  await expect(page.getByRole('table').locator('tbody tr')).toHaveCount(3);

  // The account dimension applies to both reports, and its descendant toggle
  // only appears once there is a selection to expand.
  await expect(page.getByRole('checkbox', { name: 'Include sub-accounts of the selected accounts' })).toHaveCount(0);
  await openFilter(page, 'Accounts');
  await page.getByRole('checkbox', { name: `Filter checking ${suffix}`, exact: true }).check();
  await expect(page).toHaveURL(new RegExp(`account_id=${checking.id}`));
  await page.getByRole('checkbox', { name: 'Include sub-accounts of the selected accounts' }).check();
  await expect(page).toHaveURL(/include_descendants=true/);

  // The account filter must reach the net-worth calculation, not just the URL:
  // checking alone is -110.00 where both accounts together are -200.00.
  await page.getByRole('button', { name: 'Net worth', exact: true }).click();
  const netWorth = page.getByRole('table');
  await expect(netWorth).toContainText('-110.00');
  await expect(netWorth).not.toContainText('-200.00');
});

test('a spending row drills down to exactly its transactions', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const today = todayISO();

  const checking = await createCashAccount(page, csrfToken, `Drill checking ${suffix}`, currencyID);

  const categories = await apiJSON<{
    categories: Array<{ id: number; code?: string; allows_postings: boolean }>;
  }>(page, 'GET', '/api/v1/categories');
  const category = (code: string) => {
    const found = categories.categories.find((item) => item.code === code && item.allows_postings);
    if (!found) throw new Error(`seeded category ${code} not found`);
    return found.id;
  };

  await postTransaction(page, today, checking.id, category('expense_food_groceries'), currencyID, 4200);
  await postTransaction(page, today, checking.id, category('expense_transport_fuel'), currencyID, 3100);

  await page.goto(`/app/reports?view=spending&group_by=category&start_date=${today}&end_date=${today}&bucket=month`);

  await page.getByRole('link', { name: 'Groceries' }).click();

  // The link carries the report's own semantics, not just its dates.
  await expect(page).toHaveURL(/\/app\/transactions\?/);
  await expect(page).toHaveURL(/date_basis=entry/);
  await expect(page).toHaveURL(/status=posted/);
  await expect(page).toHaveURL(new RegExp(`category_id=${category('expense_food_groceries')}`));

  // The list is narrowed to that row, and says why — the filter bar cannot show
  // a category, so without the notice it would look like a broken list.
  await expect(page.getByText('Showing the transactions behind one report row.')).toBeVisible();

  // Scoped to this test's own account, because the amount cell renders the
  // commodity symbol hard against the figure and the e2e database is shared: a
  // bare `getByText('31.00')` over the whole table also matched the preflight
  // spec's instrument, whose generated code ends in a digit and whose sell
  // quantity is `1.000` — "…31.000" whenever `Date.now()` happened to end in 3.
  // That is a one-in-ten failure of the full suite, and it is the selector's
  // fault, not the app's.
  const ownRows = page.getByRole('table').locator('tbody tr').filter({ hasText: checking.name });
  await expect(ownRows.filter({ hasText: '42.00' })).toHaveCount(1);
  await expect(ownRows.filter({ hasText: '31.00' })).toHaveCount(0);

  // The way out restores the unfiltered list.
  await page.getByRole('button', { name: 'Show all transactions' }).click();
  await expect(
    page.getByRole('table').locator('tbody tr').filter({ hasText: checking.name }).filter({ hasText: '31.00' })
  ).toHaveCount(1);
});

test('cashflow separates spending from transfers and names its cash scope', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const today = todayISO();

  const checking = await createCashAccount(page, csrfToken, `Cashflow checking ${suffix}`, currencyID);
  const savings = await createCashAccount(page, csrfToken, `Cashflow savings ${suffix}`, currencyID);
  const card = await createLiabilityAccount(page, csrfToken, `Cashflow card ${suffix}`, currencyID);

  const categories = await apiJSON<{
    categories: Array<{ id: number; code?: string; allows_postings: boolean }>;
  }>(page, 'GET', '/api/v1/categories');
  const category = (code: string) => {
    const found = categories.categories.find((item) => item.code === code && item.allows_postings);
    if (!found) throw new Error(`seeded category ${code} not found`);
    return found.id;
  };

  // 1000.00 salary in, 60.00 groceries out.
  await postTransaction(page, today, checking.id, category('income_salary_wages'), currencyID, -100000);
  await postTransaction(page, today, checking.id, category('expense_food_groceries'), currencyID, 6000);
  // 200.00 checking to savings: both are liquid cash, so this must not appear
  // anywhere — it changed neither the cash total nor the cashflow.
  await postTransfer(page, today, checking.id, savings.id, currencyID, 20000);
  // 150.00 card payment: the card is outside the cash scope, so this is a
  // transfer, never spending.
  await postTransfer(page, today, checking.id, card.id, currencyID, 15000);

  // Scoped to this test's own accounts: the e2e database is shared within a
  // run, and every spec posts to today, so the default scope would also pick up
  // the other specs' cash accounts.
  const scoped = `&account_id=${checking.id}&account_id=${savings.id}`;
  await page.goto(`/app/reports?view=cashflow&start_date=${today}&end_date=${today}&bucket=day${scoped}`);

  const table = page.getByRole('table');
  await expect(table).toBeVisible();
  const row = table.locator('tbody tr').first();

  await expect(row).toContainText('1,000.00');
  await expect(row).toContainText('60.00');
  // 1000 - 60 - 150; the 200.00 internal transfer cancels.
  await expect(row).toContainText('+790.00');
  // The card payment is financing movement, not an expense.
  await expect(row).toContainText('-150.00');
  await expect(page.getByText(/Measuring the 2 accounts you selected/)).toBeVisible();

  // Narrowing to checking alone turns the internal transfer into financing
  // movement, because savings is now outside the measured set.
  await page.goto(`/app/reports?view=cashflow&start_date=${today}&end_date=${today}&bucket=day&account_id=${checking.id}`);
  // 1000 - 60 - 200 - 150.
  await expect(page.getByRole('table').locator('tbody tr').first()).toContainText('+590.00');

  // Without an account filter the scope is the named liquid-cash default, and
  // the transfer policy is stated rather than assumed.
  await page.goto(`/app/reports?view=cashflow&start_date=${today}&end_date=${today}&bucket=day`);
  await expect(page.getByText(/Measuring your \d+ active cash accounts/)).toBeVisible();
  await expect(page.getByText(/is shown as a transfer, never as spending/)).toBeVisible();
});

test('a report exports CSV a spreadsheet can read, and prints without its chrome', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const today = todayISO();

  const checking = await createCashAccount(page, csrfToken, `Export checking ${suffix}`, currencyID);
  const categories = await apiJSON<{
    categories: Array<{ id: number; code?: string; allows_postings: boolean }>;
  }>(page, 'GET', '/api/v1/categories');
  const groceries = categories.categories.find(
    (item) => item.code === 'expense_food_groceries' && item.allows_postings
  );
  if (!groceries) throw new Error('seeded groceries category not found');

  await postTransaction(page, today, checking.id, groceries.id, currencyID, 123456);

  await page.goto(
    `/app/reports?view=cashflow&start_date=${today}&end_date=${today}&bucket=day&account_id=${checking.id}`
  );
  await expect(page.getByRole('table')).toBeVisible();

  const download = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Download CSV' }).click();
  const file = await download;

  expect(file.suggestedFilename()).toBe(`rekenraam-cashflow-${today}-${today}.csv`);

  const path = await file.path();
  const content = await readFile(path, 'utf8');
  const [header, firstRow] = content.split('\r\n');
  expect(header).toContain('Net movement');
  // Exact and unformatted: a grouped or comma-decimal figure would not parse in
  // a spreadsheet, and 1234.56 is the amount, not "1,234.56".
  expect(firstRow).toContain('1234.56');
  expect(firstRow).not.toContain('1,234.56');

  // The file says what it measured. A column of figures with no scope, basis, or
  // exclusion policy is one nobody can check later — and the header row is still
  // first, so the context costs a naive reader nothing.
  expect(content).toContain('Posted transactions only');
  expect(content).toContain('Measuring the 1 accounts you selected.');
  expect(content).toContain('Excluded: all');

  // Printing drops what cannot be acted on from paper and keeps the report.
  await page.emulateMedia({ media: 'print' });
  await expect(page.getByRole('button', { name: 'Download CSV' })).toBeHidden();
  await expect(page.getByRole('navigation', { name: 'Report' })).toBeHidden();
  await expect(page.getByRole('table')).toBeVisible();
  await page.emulateMedia({ media: 'screen' });
});

test('a cashflow row drills down to the categories behind it', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const today = todayISO();

  const checking = await createCashAccount(page, csrfToken, `Drill cash ${suffix}`, currencyID);
  const card = await createLiabilityAccount(page, csrfToken, `Drill card ${suffix}`, currencyID);

  const categories = await apiJSON<{
    categories: Array<{ id: number; code?: string; allows_postings: boolean }>;
  }>(page, 'GET', '/api/v1/categories');
  const category = (code: string) => {
    const found = categories.categories.find((item) => item.code === code && item.allows_postings);
    if (!found) throw new Error(`seeded category ${code} not found`);
    return found.id;
  };

  await postTransaction(page, today, checking.id, category('expense_food_groceries'), currencyID, 8000);
  await postTransaction(page, today, checking.id, category('expense_transport_fuel'), currencyID, 3000);
  // A card payment is financing movement, so it must not appear in the
  // breakdown of the outflow figure.
  await postTransfer(page, today, checking.id, card.id, currencyID, 25000);

  await page.goto(
    `/app/reports?view=cashflow&start_date=${today}&end_date=${today}&bucket=day&account_id=${checking.id}`
  );

  const cashflowRow = page.getByRole('table').locator('tbody tr').first();
  await expect(cashflowRow).toContainText('110.00');

  // The outflow figure is the link into its own breakdown.
  await cashflowRow.getByRole('link', { name: '110.00' }).click();

  await expect(page).toHaveURL(/view=spending/);
  await expect(page).toHaveURL(/mode=spending/);
  await expect(page).toHaveURL(new RegExp(`account_id=${checking.id}`));

  // The breakdown adds up to the figure it came from, and the card payment is
  // absent because a transfer has no category posting.
  const spendingRows = page.getByRole('table').locator('tbody tr');
  await expect(spendingRows).toHaveCount(2);
  await expect(page.getByRole('table')).toContainText('80.00');
  await expect(page.getByRole('table')).toContainText('30.00');
  await expect(page.getByRole('table')).not.toContainText('250.00');
});

test('one multi-currency journey travels through every report without combining commodities', async ({
  page
}) => {
  const { csrfToken, currencyID: usdID } = await readyForLedger(page);
  const eurID = await ensureCurrency(page, csrfToken, 'EUR', 'Euro');
  const suffix = Date.now();
  const today = todayISO();

  const usdChecking = await createCashAccount(page, csrfToken, `Multi USD ${suffix}`, usdID);
  const eurChecking = await createCashAccount(page, csrfToken, `Multi EUR ${suffix}`, eurID);

  const categories = await apiJSON<{
    categories: Array<{ id: number; code?: string; allows_postings: boolean }>;
  }>(page, 'GET', '/api/v1/categories');
  const category = (code: string) => {
    const found = categories.categories.find((item) => item.code === code && item.allows_postings);
    if (!found) throw new Error(`seeded category ${code} not found`);
    return found.id;
  };
  const groceries = category('expense_food_groceries');

  // The two figures are deliberately different, and neither is the sum: if any
  // report ever combined them, 40.00 or 90.00 would appear where 25.00 and
  // 65.00 belong.
  await postTransaction(page, today, usdChecking.id, groceries, usdID, 6500);
  await postTransaction(page, today, eurChecking.id, groceries, eurID, 2500);

  // Every report is scoped to this test's own accounts: the e2e database is
  // shared within a run and every spec posts to today.
  const scoped = `&account_id=${usdChecking.id}&account_id=${eurChecking.id}`;
  const range = `start_date=${today}&end_date=${today}&bucket=day`;

  // --- Spending: one category, two commodities, ranked separately.
  await page.goto(`/app/reports?view=spending&group_by=category&${range}${scoped}`);
  const spendingTable = page.getByRole('table');
  await expect(spendingTable).toBeVisible();
  await expect(spendingTable).toContainText('65.00');
  await expect(spendingTable).toContainText('25.00');
  await expect(spendingTable).not.toContainText('90.00');
  // Unlike commodities are named as separate, and the chart is suppressed
  // because bars across them would be a comparison the ledger cannot justify.
  await expect(page.getByText(/ranked separately by commodity/)).toBeVisible();
  await expect(page.getByRole('img')).toHaveCount(0);

  // --- Cashflow: the same postings, still per commodity, still separate.
  await page.goto(`/app/reports?view=cashflow&${range}${scoped}`);
  const cashflowTable = page.getByRole('table');
  await expect(cashflowTable).toBeVisible();
  await expect(cashflowTable).toContainText('-65.00');
  await expect(cashflowTable).toContainText('-25.00');
  await expect(page.getByText(/shown separately by commodity/)).toBeVisible();
  await expect(page.getByRole('img')).toHaveCount(0);

  // CSV carries both commodities as their own rows, exact and uncombined.
  const download = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Download CSV' }).click();
  const content = await readFile((await (await download).path()) as string, 'utf8');
  const rows = content.split('\r\n').filter((line) => line.includes('-65.00') || line.includes('-25.00'));
  expect(rows).toHaveLength(2);
  expect(content).not.toContain('-90.00');

  // --- Net worth: two rows for one bucket, one per commodity.
  await page.goto(`/app/reports?view=net-worth&${range}${scoped}`);
  const netWorthRows = page.getByRole('table').locator('tbody tr');
  await expect(netWorthRows).toHaveCount(2);
  await expect(page.getByRole('table')).toContainText('-65.00');
  await expect(page.getByRole('table')).toContainText('-25.00');

  // --- A commodity filter narrows to one commodity, survives in the URL, and
  // the report becomes single-commodity again — chart and all.
  await page.goto(`/app/reports?view=spending&group_by=category&${range}${scoped}&commodity_id=${eurID}`);
  const filtered = page.getByRole('table');
  await expect(filtered).toContainText('25.00');
  await expect(filtered).not.toContainText('65.00');
  await expect(page.getByText(/ranked separately by commodity/)).toBeHidden();
  await expect(page.getByRole('img')).toHaveCount(1);

  // --- Drilling from one commodity's cashflow row carries that commodity, so
  // the breakdown never widens into the other one.
  await page.goto(`/app/reports?view=cashflow&${range}${scoped}`);
  const eurRow = page.getByRole('table').locator('tbody tr').filter({ hasText: 'EUR' }).first();
  await eurRow.getByRole('link', { name: '25.00' }).click();

  await expect(page).toHaveURL(new RegExp(`commodity_id=${eurID}`));
  await expect(page).toHaveURL(/view=spending/);
  const breakdown = page.getByRole('table');
  await expect(breakdown).toContainText('25.00');
  await expect(breakdown).not.toContainText('65.00');
});

test('a report keeps each measure at its own scale, in the table and the export', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const today = todayISO();

  const checking = await createCashAccount(page, csrfToken, `Scale checking ${suffix}`, currencyID);
  const categories = await apiJSON<{
    categories: Array<{ id: number; code?: string; allows_postings: boolean }>;
  }>(page, 'GET', '/api/v1/categories');
  const category = (code: string) => {
    const found = categories.categories.find((item) => item.code === code && item.allows_postings);
    if (!found) throw new Error(`seeded category ${code} not found`);
    return found.id;
  };

  // 50.00 of salary recorded at scale 2, and 100 of groceries recorded at scale
  // 0 — both legal for this commodity. Inflow and outflow therefore accumulate
  // at different scales, and a screen that reads one scale for the whole row
  // renders the 100 as 1.00.
  await postScaledTransaction(page, today, checking.id, category('income_salary_wages'), currencyID, '5000', 2, true);
  await postScaledTransaction(page, today, checking.id, category('expense_food_groceries'), currencyID, '100', 0, false);

  await page.goto(
    `/app/reports?view=cashflow&start_date=${today}&end_date=${today}&bucket=day&account_id=${checking.id}`
  );

  const row = page.getByRole('table').locator('tbody tr').first();
  await expect(row).toContainText('50.00');
  await expect(row).toContainText('100.00');
  // The outflow is one hundred, not one.
  await expect(row).not.toContainText('1.00\n');
  // 50 in, 100 out.
  await expect(row).toContainText('-50.00');

  const download = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Download CSV' }).click();
  const content = await readFile((await (await download).path()) as string, 'utf8');
  const dataRow = content.split('\r\n')[1];
  expect(dataRow).toContain('100.00');
  expect(dataRow).toContain('50.00');
});

async function postTransaction(
  page: Page,
  date: string,
  accountID: number,
  categoryID: number,
  commodityID: number,
  cents: number
) {
  const csrfToken = await csrfTokenFor(page);
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    status: 'posted',
    transaction_date: date,
    description: 'reports fixture',
    journal_entries: [
      {
        entry_date: date,
        postings: [
          { account_id: accountID, commodity_id: commodityID, quantity_value: String(-cents), quantity_scale: 2 },
          { account_id: categoryID, commodity_id: commodityID, quantity_value: String(cents), quantity_scale: 2 }
        ]
      }
    ]
  }, [201]);
}

/** Posts one balanced entry at an explicit quantity scale. */
async function postScaledTransaction(
  page: Page,
  date: string,
  accountID: number,
  categoryID: number,
  commodityID: number,
  coefficient: string,
  scale: number,
  incoming: boolean
) {
  const csrfToken = await csrfTokenFor(page);
  const accountValue = incoming ? coefficient : `-${coefficient}`;
  const categoryValue = incoming ? `-${coefficient}` : coefficient;
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    status: 'posted',
    transaction_date: date,
    description: 'reports scale fixture',
    journal_entries: [
      {
        entry_date: date,
        postings: [
          { account_id: accountID, commodity_id: commodityID, quantity_value: accountValue, quantity_scale: scale },
          { account_id: categoryID, commodity_id: commodityID, quantity_value: categoryValue, quantity_scale: scale }
        ]
      }
    ]
  }, [201]);
}

async function postTransfer(
  page: Page,
  date: string,
  fromAccountID: number,
  toAccountID: number,
  commodityID: number,
  cents: number
) {
  const csrfToken = await csrfTokenFor(page);
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    status: 'posted',
    transaction_date: date,
    description: 'reports transfer fixture',
    journal_entries: [
      {
        entry_date: date,
        postings: [
          { account_id: fromAccountID, commodity_id: commodityID, quantity_value: String(-cents), quantity_scale: 2 },
          { account_id: toAccountID, commodity_id: commodityID, quantity_value: String(cents), quantity_scale: 2 }
        ]
      }
    ]
  }, [201]);
}

/** Opens one filter disclosure by its visible label. */
async function openFilter(page: Page, label: string) {
  const disclosure = page.locator('details').filter({ has: page.getByText(label, { exact: true }) });
  await disclosure.locator('summary').click();
}
