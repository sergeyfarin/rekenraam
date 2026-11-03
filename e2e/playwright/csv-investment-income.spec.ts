import { expect, test } from './support/fixtures';
import { apiJSON } from './support/api';
import { todayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';
import { expectNoAccessibilityViolations } from './support/a11y';

test.use({ viewport: { width: 390, height: 844 } });

test('[acceptance] CSV dividends and separate tax use saved mappings and reviewed instruments on a phone', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const code = `CSV${suffix}`;
  const instrument = await apiJSON<{ commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: code, instrument_type: 'stock', display_name: `CSV payer ${suffix}`, symbol: code,
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 0, price_scale: 2, effective_from: todayISO()
  });
  const cash = await createCashAccount(page, csrfToken, `CSV cash ${suffix}`, currencyID);
  const category = async (kind: 'income' | 'expense') => apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `CSV ${kind} ${suffix}`, account_class: kind, account_kind: kind, default_commodity_id: currencyID,
    allows_postings: true, opened_on: todayISO(), effective_from: todayISO()
  });
  const income = await category('income');
  const tax = await category('expense');
  const profileName = `CSV income ${suffix}`;
  const contents = `Date;Type;Code;Cash;Gross;Tax;Ref;Memo\n${todayISO()};DIV;${code};85,00;100,00;15,00;div-${suffix};First dividend\n${todayISO()};TAX;${code};-5,00;;;tax-${suffix};Separate withholding\n${todayISO()};DIV;UNKNOWN;42,50;50,00;7,50;review-${suffix};Reviewed dividend\n`;
  const upload = async () => page.locator('input[type="file"]').setInputFiles({ name: 'income.csv', mimeType: 'text/csv', buffer: Buffer.from(contents) });
  const activity = () => apiJSON<{ transactions: Array<{ id: number }> }>(page, 'GET', `/api/v1/transactions?commodity_id=${instrument.commodity_id}`);

  await page.goto('/app/ledger/import');
  await upload();
  await page.getByLabel('Mapping name').fill(profileName);
  await page.getByRole('combobox', { name: 'Date column', exact: true }).selectOption('Date');
  await page.getByRole('combobox', { name: 'Amount column', exact: true }).selectOption('Cash');
  await page.getByRole('combobox', { name: 'Payee or description column', exact: true }).selectOption('');
  await page.getByRole('combobox', { name: 'Memo column', exact: true }).selectOption('Memo');
  await page.getByRole('combobox', { name: 'Bank transaction ID column', exact: true }).selectOption('Ref');
  await page.getByRole('combobox', { name: 'Date layout', exact: true }).selectOption('YMD');
  await page.getByRole('combobox', { name: 'Number format', exact: true }).selectOption(',');
  const mapping = page.getByRole('group', { name: 'Dividend and tax mapping', exact: true });
  await mapping.getByLabel('Import instrument dividends and withholding').check();
  await mapping.getByRole('combobox', { name: 'Payment type column', exact: true }).selectOption('Type');
  await mapping.getByLabel('Source value for dividends').fill('DIV');
  await mapping.getByLabel('Source value for tax').fill('TAX');
  await mapping.getByRole('combobox', { name: /^Instrument commodity code column/ }).selectOption('Code');
  await mapping.getByRole('combobox', { name: 'Gross dividend column', exact: true }).selectOption('Gross');
  await mapping.getByRole('combobox', { name: 'Withholding amount column', exact: true }).selectOption('Tax');
  await mapping.getByRole('combobox', { name: 'Cash account', exact: true }).selectOption(String(cash.id));
  await mapping.getByRole('combobox', { name: 'Income account', exact: true }).selectOption(String(income.id));
  await mapping.getByRole('combobox', { name: 'Withholding account', exact: true }).selectOption(String(tax.id));
  await page.getByRole('button', { name: 'Upload & preview' }).click();
  const review = page.getByRole('group', { name: 'Investment row 3', exact: true });
  await expect(review).toBeVisible();
  await expect(review.getByRole('combobox', { name: 'Instrument', exact: true })).toHaveValue('');
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByRole('button', { name: 'Commit to ledger' })).toBeEnabled();
  expect((await activity()).transactions).toHaveLength(0);
  await review.getByRole('combobox', { name: 'Instrument', exact: true }).selectOption(String(instrument.commodity_id));
  await review.getByLabel('Dividend amount').fill('55.00');
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByRole('button', { name: 'Commit to ledger' })).toBeEnabled();
  expect((await activity()).transactions).toHaveLength(0);
  await review.getByLabel('Dividend amount').fill('50.00');
  await expectNoAccessibilityViolations(page, 'CSV investment mapping review on a phone');
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('3 transactions committed')).toBeVisible();
  expect((await activity()).transactions).toHaveLength(3);

  await page.getByRole('button', { name: 'Import another file' }).click();
  await upload();
  const saved = page.getByLabel('Saved mapping');
  await expect.poll(() => saved.evaluate((select: HTMLSelectElement) => select.selectedOptions[0]?.textContent?.trim())).toBe(`${profileName} — suggested`);
  await page.getByRole('button', { name: 'Edit mapping' }).click();
  await expect(mapping.getByRole('combobox', { name: 'Cash account', exact: true })).toHaveValue(String(cash.id));
  await expect(mapping.getByRole('combobox', { name: 'Gross dividend column', exact: true })).toHaveValue('Gross');
  await page.getByRole('button', { name: 'Save changes' }).click();
  await page.getByRole('button', { name: 'Upload & preview' }).click();
  await expect(page.getByRole('group', { name: 'Investment row 3', exact: true }).getByRole('combobox', { name: 'Instrument', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('0 transactions committed')).toBeVisible();
  expect((await activity()).transactions).toHaveLength(3);
});

test('CSV dividends and withholding without a payer column keep exact amounts and dedupe', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const date = todayISO();
  const cash = await createCashAccount(page, csrfToken, `Unassigned CSV ${suffix}`, currencyID);
  const category = async (kind: 'income' | 'expense') => apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Unassigned ${kind} ${suffix}`, account_class: kind, account_kind: kind, default_commodity_id: currencyID,
    allows_postings: true, opened_on: date, effective_from: date
  });
  const income = await category('income');
  const tax = await category('expense');
  const profile = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/import-profiles', csrfToken, {
    name: `Unassigned mapping ${suffix}`, adapter_kind: 'csv', config: JSON.stringify({ delimiter: 'semicolon', date_column: 'Date', amount_column: 'Cash', date_layout: 'YMD', decimal_separator: ',', external_ref_column: 'Ref',
      investment: { kind: 'column', kind_column: 'Type', dividend_value: 'DIV', withholding_value: 'TAX', gross_column: 'Gross', withholding_column: 'Tax', cash_account_id: cash.id, income_account_id: income.id, withholding_account_id: tax.id } })
  });
  const contents = `Date;Type;Cash;Gross;Tax;Ref\n${date};DIV;85,00;100,00;15,00;div-${suffix}\n${date};TAX;-2,00;;;tax-${suffix}\n`;
  const upload = async () => {
    await page.locator('input[type="file"]').setInputFiles({ name: 'unassigned.csv', mimeType: 'text/csv', buffer: Buffer.from(contents) });
    await page.getByLabel('Saved mapping').selectOption(String(profile.id));
    await page.getByRole('button', { name: 'Upload & preview' }).click();
  };
  await page.goto('/app/ledger/import');
  await upload();
  for (const index of [1, 2]) {
    const row = page.getByRole('group', { name: `Investment row ${index}`, exact: true });
    await expect(row.getByRole('combobox', { name: /^Instrument/ })).toHaveValue('');
    await expect(row.getByRole('option', { name: 'Unassigned', exact: true })).toHaveCount(1);
  }
  await expectNoAccessibilityViolations(page, 'CSV income without payer attribution');
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('2 transactions committed')).toBeVisible();
  const coverage = await apiJSON<{ unassigned: Array<{ transaction_id: number; cash: { value: string; scale: number }; gross_state: string; withholding_state: string }> }>(page, 'GET', '/api/v1/investments/income-coverage');
  const ledger = await apiJSON<{ transactions: Array<{ id: number }> }>(page, 'GET', `/api/v1/transactions?account_id=${cash.id}`);
  expect(ledger.transactions).toHaveLength(2);
  const items = coverage.unassigned.filter(row => ledger.transactions.some(transaction => transaction.id === row.transaction_id));
  expect(items).toHaveLength(2);
  expect(items.reduce((sum, item) => sum + BigInt(item.cash.value) * 100n / 10n ** BigInt(item.cash.scale), 0n)).toBe(8300n);
  expect(items.find(item => item.gross_state === 'none')?.withholding_state).toBe('known');
  await page.getByRole('button', { name: 'Import another file' }).click();
  await upload();
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('0 transactions committed')).toBeVisible();
});
