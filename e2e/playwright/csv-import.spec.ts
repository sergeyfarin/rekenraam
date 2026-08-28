import { expect, test } from '@playwright/test';
import { todayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';

function todayDMY(): string {
  const [year, month, day] = todayISO().split('-');
  return `${day}/${month}/${year}`;
}

test('[acceptance] a bank CSV mapping is reused with grouped payee resolution', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `CSV bank ${suffix}`, currencyID);
  const profileName = `Semicolon bank ${suffix}`;

  await page.goto('/app/import');
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

  await page.getByLabel('Target account').selectOption(String(account.id));
  await page.getByLabel('Currency').selectOption(String(currencyID));
  await page.getByLabel('Default category').selectOption({ index: 1 });
  await page.getByRole('button', { name: 'Apply to all rows' }).click();
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('2 transactions committed')).toBeVisible();

  await page.getByRole('button', { name: 'Import another file' }).click();
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
  await page.getByRole('button', { name: 'Bakker' }).click();
  await expect(page.getByRole('heading', { name: 'Resolve payees' })).toBeHidden();
  await expect(page.getByRole('button', { name: 'Commit to ledger' })).toBeVisible();
});
