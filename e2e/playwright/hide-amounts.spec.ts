import { expect, test } from './support/fixtures';
import { readyForLedger } from './support/ledger';
import { expectNoAccessibilityViolations } from './support/a11y';

test('hide amounts persists across reloads and tabs while editable inputs stay exact', async ({ page, context }) => {
  await readyForLedger(page);
  await page.goto('/app/ledger/transactions');
  const toggle = page.getByRole('button', { name: 'Hide amounts', exact: true });
  await toggle.click();
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');
  await page.reload();
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');
  await page.getByRole('button', { name: 'New transaction', exact: true }).click();
  const form = page.getByRole('region', { name: 'New transaction', exact: true });
  await form.getByLabel('Amount', { exact: true }).fill('1234.56');
  const other = await context.newPage();
  await other.goto(page.url());
  await other.getByRole('button', { name: 'Hide amounts', exact: true }).click();
  await expect(toggle).toHaveAttribute('aria-pressed', 'false');
  await expect(form.getByLabel('Amount', { exact: true })).toHaveValue('1234.56');
  await other.close();
  await form.getByRole('button', { name: 'Cancel', exact: true }).last().click();
  await page.getByRole('alertdialog', { name: 'Discard this transaction?' }).getByRole('button', { name: 'Discard', exact: true }).click();
  await toggle.click();
  await expectNoAccessibilityViolations(page, 'hide amounts');
});
