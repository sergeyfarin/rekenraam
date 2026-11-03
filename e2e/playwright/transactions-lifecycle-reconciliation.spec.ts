import { expect, test, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

/**
 * #220: a void or unvoid refused by a finished reconciliation once previewed
 * its impact through the edit endpoint with an empty body, which failed
 * validation. The dialog now lists every checkpoint the lifecycle change
 * would invalidate — later ones included — before an explicit override.
 * Phone-sized screen.
 */

test.use({ viewport: { width: 390, height: 844 } });

type Checkpoint = { id: number; status: string; session_id?: number };

async function seed(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const checking = await createCashAccount(page, csrfToken, `Lifecycle checking ${suffix}`, currencyID);
  const category = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: `Lifecycle groceries ${suffix}`, category_type: 'expense'
  });
  const day = todayISO();
  const spend = (description: string, cents: number) =>
    apiJSON<{ id: number; journal_entries: Array<{ postings: Array<{ id: number; account_id: number }> }> }>(
      page, 'POST', '/api/v1/transactions', csrfToken, {
        transaction_date: day, status: 'posted', description,
        journal_entries: [{ entry_date: day, postings: [
          { account_id: checking.id, commodity_id: currencyID, quantity_value: String(-cents), quantity_scale: 2 },
          { account_id: category.id, commodity_id: currencyID, quantity_value: String(cents), quantity_scale: 2 }
        ] }]
      });
  /** Clears the transaction's checking posting and finishes a statement at `balance`. */
  const reconcile = async (tx: Awaited<ReturnType<typeof spend>>, balance: number) => {
    const posting = tx.journal_entries[0].postings.find((p) => p.account_id === checking.id)!;
    await apiJSON(page, 'POST', `/api/v1/accounts/${checking.id}/postings/reconciliation-status`, csrfToken, {
      posting_version_ids: [posting.id], status: 'cleared', cleared_on: day
    });
    const session = await apiJSON<{ id: number }>(page, 'POST', `/api/v1/accounts/${checking.id}/reconciliations/start`, csrfToken, {
      commodity_id: currencyID, source_kind: 'statement', statement_date: day,
      statement_balance_value: String(balance), statement_balance_scale: 2
    });
    await apiJSON(page, 'POST', `/api/v1/reconciliations/${session.id}/finish`, csrfToken, { change_reason: 'matched statement' });
    const checkpoint = (await checkpoints()).find((c) => c.session_id === session.id);
    if (!checkpoint) throw new Error('expected a checkpoint for the finished statement');
    return checkpoint.id;
  };
  const checkpoints = async () =>
    (await apiJSON<{ checkpoints: Checkpoint[] }>(page, 'GET', `/api/v1/accounts/${checking.id}/reconciliation-checkpoints`)).checkpoints;
  const active = async () => (await checkpoints()).filter((c) => c.status === 'active').map((c) => c.id).sort();
  const status = async (id: number) =>
    (await apiJSON<{ status: string }>(page, 'GET', `/api/v1/transactions/${id}`)).status;
  return { suffix, checking, spend, reconcile, active, status };
}

test('a refused void and unvoid show every affected checkpoint before override', async ({ page }) => {
  const s = await seed(page);
  const subject = await s.spend(`Statement fix ${s.suffix}`, 1000);
  const first = await s.reconcile(subject, -1000);
  // A second statement the same day, after the subject, is invalidated too.
  const later = await s.reconcile(await s.spend(`Later spend ${s.suffix}`, 500), -1500);

  await page.goto(`/app/ledger/accounts/${s.checking.id}/register`);
  await page.getByRole('row').filter({ hasText: `Statement fix ${s.suffix}` }).click();
  await page.getByRole('button', { name: 'Void…' }).click();
  await page.getByLabel('Reason').fill('duplicate charge');
  await page.getByRole('dialog', { name: 'Void transaction' }).getByRole('button', { name: 'Void', exact: true }).click();

  const warning = page.getByRole('alertdialog', { name: 'This change affects a reconciled period' });
  await expect(warning).toBeVisible();
  await expect(warning.getByRole('listitem')).toHaveCount(2);
  await expect(warning.getByText(`Lifecycle checking ${s.suffix} (USD) reconciled through`).first()).toBeVisible();

  // Cancelling writes nothing.
  await warning.getByRole('button', { name: 'Go back' }).click();
  await expect(warning).toHaveCount(0);
  expect(await s.status(subject.id)).toBe('posted');
  expect(await s.active()).toEqual([first, later].sort());

  // Confirming invalidates exactly the listed checkpoints.
  await page.getByRole('button', { name: 'Void…' }).click();
  await page.getByLabel('Reason').fill('duplicate charge');
  await page.getByRole('dialog', { name: 'Void transaction' }).getByRole('button', { name: 'Void', exact: true }).click();
  await warning.getByRole('button', { name: 'Proceed and invalidate' }).click();
  await expect.poll(() => s.status(subject.id)).toBe('voided');
  expect(await s.active()).toEqual([]);

  // A new statement covers the day again; unvoid shows it and only it.
  // The later spend's checkpoint was invalidated too, so it is re-verified
  // on this statement (#240): -5.00 later plus -2.00 next.
  const again = await s.reconcile(await s.spend(`Next spend ${s.suffix}`, 200), -700);
  // The register hides voided entries; a transaction link opens its panel.
  await page.goto(`/app/ledger/transactions?transaction_id=${subject.id}`);
  await page.getByRole('button', { name: 'Unvoid…' }).click();
  await page.getByLabel('Reason').fill('charge was real');
  await page.getByRole('dialog', { name: 'Unvoid transaction' }).getByRole('button', { name: 'Unvoid', exact: true }).click();
  await expect(warning.getByRole('listitem')).toHaveCount(1);
  await warning.getByRole('button', { name: 'Proceed and invalidate' }).click();
  await expect.poll(() => s.status(subject.id)).toBe('posted');
  expect(await s.active()).not.toContain(again);
});
