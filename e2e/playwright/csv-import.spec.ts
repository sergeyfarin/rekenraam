import { expect, test } from './support/fixtures';
import { todayISO } from './support/dates';
import { apiJSON } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';
import { expectNoAccessibilityViolations } from './support/a11y';

function todayDMY(): string {
  const [year, month, day] = todayISO().split('-');
  return `${day}/${month}/${year}`;
}

test('[acceptance] a bank CSV mapping is reused with payee resolution and preview rules', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `CSV bank ${suffix}`, currencyID);
  const profileName = `Semicolon bank ${suffix}`;

  await page.goto('/app/ledger/import');
  await page.locator('input[type="file"]').setInputFiles({
    name: 'statement.csv',
    mimeType: 'text/csv',
    buffer: Buffer.from(`Datum;Omschrijving;Bedrag\n${todayDMY()};Bakker;-12,34\n${todayDMY()};Bakker;-4,56\n`)
  });
  await page.getByLabel('Mapping name').fill(profileName);
  await page.getByRole('button', { name: 'Upload & preview' }).click();
  await expect(page.getByText('-12.34')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Resolve payees' })).toBeVisible();
  await expect(page.getByText('Matching rows: 2')).toBeVisible();
  await page.getByRole('button', { name: 'Create “Bakker”' }).click();
  await expect(page.getByRole('heading', { name: 'Resolve payees' })).toBeHidden();
  // #330: every per-row picker in the staged preview has its own name.
  await expect(page.getByRole('combobox', { name: 'Category, row 2', exact: true })).toBeVisible();
  await expectNoAccessibilityViolations(page, 'staged CSV preview');

  await page.getByLabel('Target account', { exact: true }).selectOption(String(account.id));
  await page.getByLabel('Currency', { exact: true }).selectOption(String(currencyID));
  await page.getByLabel('Default category').selectOption({ index: 1 });
  await page.getByRole('button', { name: 'Apply to all rows' }).click();
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('2 transactions committed')).toBeVisible();

  await page.getByRole('button', { name: 'Import another file' }).click();
  const ruleName = `Bakker typo ${suffix}`;
  await page.getByRole('button', { name: 'Add rule' }).click();
  await page.getByLabel('Name').fill(ruleName);
  await page.getByLabel('Contains').fill('Bakkr');
  await page.getByLabel('Set category').selectOption({ index: 1 });
  await page.getByLabel('Set payee').selectOption({ label: 'Bakker' });
  await page.getByRole('button', { name: 'Save rule' }).click();
  await expect(page.getByText('Import rule saved.')).toBeVisible();

  await page.locator('input[type="file"]').setInputFiles({
    name: 'next-statement.csv',
    mimeType: 'text/csv',
    buffer: Buffer.from(`Datum;Omschrijving;Bedrag\n${todayDMY()};Bakkr;-8,90\n`)
  });
  const profileSelect = page.getByLabel('Saved mapping');
  await expect.poll(() => profileSelect.evaluate((select: HTMLSelectElement) => select.selectedOptions[0]?.textContent?.trim())).toBe(`${profileName} — suggested`);
  await page.getByRole('button', { name: 'Edit mapping' }).click();
  await page.getByLabel('Mapping name').fill(`${profileName} updated`);
  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect.poll(() => profileSelect.evaluate((select: HTMLSelectElement) => select.selectedOptions[0]?.textContent?.trim())).toBe(`${profileName} updated — suggested`);
  await page.getByRole('button', { name: 'Upload & preview' }).click();
  await expect(page.getByText('-8.90')).toBeVisible();
  await expect(page.getByText(`Rule: ${ruleName}`)).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Resolve payees' })).toBeHidden();
  await expect(page.getByRole('button', { name: 'Commit to ledger' })).toBeVisible();
});

// #300: a value the declared convention rejects is flagged with a translated
// warning, and commit refuses the row instead of reading it another way.
test('a CSV row rejected under the declared decimal comma is flagged and not committed', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `CSV rejected ${suffix}`, currencyID);

  await page.goto('/app/ledger/import');
  await page.locator('input[type="file"]').setInputFiles({
    name: 'rejected.csv',
    mimeType: 'text/csv',
    buffer: Buffer.from(`Boekdatum;Tegenpartij;Bedrag\n${todayDMY()};Bakker;-12,34\n${todayDMY()};Slager;-12.50\n`)
  });
  await page.getByLabel('Mapping name').fill(`Rejected comma ${suffix}`);
  await page.getByRole('button', { name: 'Upload & preview' }).click();
  await expect(page.getByText('Row 2: Amount “-12.50” does not match the file\'s decimal separator; this row will not be imported')).toBeVisible();
  await expect(page.getByText('Not readable under this file\'s format', { exact: false })).toBeVisible();

  await page.getByLabel('Target account', { exact: true }).selectOption(String(account.id));
  await page.getByLabel('Currency', { exact: true }).selectOption(String(currencyID));
  await page.getByLabel('Default category').selectOption({ index: 1 });
  await page.getByRole('button', { name: 'Apply to all rows' }).click();
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('1 transactions committed')).toBeVisible();
  await expect(page.getByText('1 failed — review the remaining rows to fix them')).toBeVisible();
});

// #301: a failed row is repaired in its own batch. The result step goes back
// to the preview, where only the uncommitted row can be re-resolved.
test('a failed import row is fixed and committed from review remaining rows', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `CSV resume ${suffix}`, currencyID);

  await page.goto('/app/ledger/import');
  await page.locator('input[type="file"]').setInputFiles({
    name: 'resume.csv',
    mimeType: 'text/csv',
    buffer: Buffer.from(`Valutadatum;Begunstigde;Bedrag EUR\n${todayDMY()};Bakker;-12,34\n${todayDMY()};Slager;-5,00\n`)
  });
  await page.getByLabel('Mapping name').fill(`Resume ${suffix}`);
  await page.getByRole('button', { name: 'Upload & preview' }).click();
  await page.getByLabel('Target account', { exact: true }).selectOption(String(account.id));
  await page.getByLabel('Currency', { exact: true }).selectOption(String(currencyID));
  await page.getByLabel('Default category').selectOption({ index: 1 });
  await page.getByRole('button', { name: 'Apply to all rows' }).click();
  // Leave the Slager row without an account: it fails, Bakker commits.
  const slager = page.getByRole('row', { name: /Slager/ });
  await slager.locator('select').first().selectOption('');
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('1 transactions committed')).toBeVisible();
  await expect(page.getByText('1 failed — review the remaining rows to fix them')).toBeVisible();

  await page.getByRole('button', { name: 'Review remaining rows' }).click();
  await expect(slager.getByText('Not imported — fix the row and commit again')).toBeVisible();
  await expect(page.getByRole('row', { name: /Bakker/ }).locator('select')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Discard' })).toBeHidden();
  await slager.locator('select').first().selectOption(String(account.id));
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('2 transactions committed')).toBeVisible();
  await expect(page.getByText(/\d+ failed/)).toBeHidden();
});

// #305: the batch override names the checkpoints shown beside it. Checking it
// lists the reconciliation the pending rows reach; the commit sends exactly
// that set, and the row lands with the checkpoint invalidated.
test('the import override lists the checkpoints it will invalidate and commits against them', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await apiJSON<{ id: number; name: string }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `CSV reconciled ${suffix}`,
    account_class: 'asset',
    account_kind: 'checking',
    default_commodity_id: currencyID,
    allows_postings: true,
    opened_on: '2021-01-01'
  });
  const categories = await apiJSON<{ categories: Array<{ id: number; code?: string; allows_postings: boolean }> }>(
    page, 'GET', '/api/v1/categories');
  const groceries = categories.categories.find((item) => item.code === 'expense_food_groceries' && item.allows_postings);
  if (!groceries) throw new Error('seeded groceries category not found');
  // Past-dated: the reports specs assert on today's totals in this shared book.
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_date: '2021-05-06',
    journal_entries: [{
      entry_date: '2021-05-06',
      postings: [
        { account_id: account.id, quantity_value: '-1500', quantity_scale: 2, commodity_id: currencyID },
        { account_id: groceries.id, quantity_value: '1500', quantity_scale: 2, commodity_id: currencyID }
      ]
    }]
  });
  await page.goto('/app/ledger/reconcile');
  await page.getByLabel('Account').selectOption({ label: account.name });
  await page.getByLabel('Statement closing date').fill('2021-05-31');
  await page.getByLabel('Statement ending balance').fill('-15.00');
  await page.getByRole('button', { name: 'Start reconciling' }).click();
  await page.getByRole('checkbox').first().check();
  await expect(page.getByText('Balanced')).toBeVisible();
  await page.getByLabel('Reason').fill(`Import recon ${suffix}`);
  await page.getByRole('button', { name: 'Finish reconciliation' }).click();
  await expect(page.getByText('Reconciliation complete')).toBeVisible();
  const checkpointsPath = `/api/v1/accounts/${account.id}/reconciliation-checkpoints`;
  const before = await apiJSON<{ checkpoints: Array<{ id: number; status: string }> }>(page, 'GET', checkpointsPath);
  const checkpointID = before.checkpoints.find((checkpoint) => checkpoint.status === 'active')!.id;

  await page.goto('/app/ledger/import');
  await page.locator('input[type="file"]').setInputFiles({
    name: 'reconciled.csv',
    mimeType: 'text/csv',
    buffer: Buffer.from('Transactiedatum;Notitie;Mutatie\n20/05/2021;Late receipt;-2,00\n')
  });
  await page.getByLabel('Mapping name').fill(`Reconciled ${suffix}`);
  await page.getByRole('button', { name: 'Upload & preview' }).click();
  await page.getByLabel('Target account', { exact: true }).selectOption(String(account.id));
  await page.getByLabel('Currency', { exact: true }).selectOption(String(currencyID));
  await page.getByLabel('Default category').selectOption({ index: 1 });
  await page.getByRole('button', { name: 'Apply to all rows' }).click();

  // Checking the override saves the resolutions and lists the set they reach.
  await page.getByLabel('Override reconciliation guards').check();
  const list = page.locator('#commit-reconciliation-checkpoints');
  await expect(list).toContainText('This commit will invalidate these reconciliation checkpoints');
  await expect(list).toContainText(account.name);
  await expect(list).toContainText('2021-05-31');

  const commitRequest = page.waitForRequest((request) => request.url().endsWith('/commit') && request.method() === 'POST');
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  expect((await commitRequest).postDataJSON().reconciliation_override_checkpoint_ids).toEqual([checkpointID]);
  await expect(page.getByText('1 transactions committed')).toBeVisible();
  const after = await apiJSON<{ checkpoints: Array<{ id: number; status: string }> }>(page, 'GET', checkpointsPath);
  expect(after.checkpoints.find((checkpoint) => checkpoint.id === checkpointID)?.status).toBe('invalidated');
});
