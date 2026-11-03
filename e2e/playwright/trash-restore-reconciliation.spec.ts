import { expect, test, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

// Handlers that read a fetched response can still be running when the page
// closes (a refetch after the last assertion); without this the test fails
// with "Response has been disposed" (#236).
test.afterEach(async ({ page }) => {
  await page.unrouteAll({ behavior: 'ignoreErrors' });
});

/**
 * #225: restoring a deleted transaction into a reconciled period asked for an
 * override without naming a single checkpoint. The Deleted view dialog now lists
 * every checkpoint from the restore's own preview (later ones included) and
 * only then lets the owner confirm. Phone-sized screen.
 */

test.use({ viewport: { width: 390, height: 844 } });

type Checkpoint = { id: number; status: string; session_id?: number };

async function seed(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const checking = await createCashAccount(page, csrfToken, `Trash checking ${suffix}`, currencyID);
  const category = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: `Trash groceries ${suffix}`, category_type: 'expense'
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
  const checkpoints = async () =>
    (await apiJSON<{ checkpoints: Checkpoint[] }>(page, 'GET', `/api/v1/accounts/${checking.id}/reconciliation-checkpoints`)).checkpoints;
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
  const active = async () => (await checkpoints()).filter((c) => c.status === 'active').map((c) => c.id).sort();
  // A soft-deleted transaction is not served by the ordinary read.
  const deleted = async (id: number) => (await page.request.get(`/api/v1/transactions/${id}`)).status() === 404;

  // The subject is deleted before two statements cover its day.
  const subject = await spend(`Trash subject ${suffix}`, 1000);
  await apiJSON(page, 'POST', `/api/v1/transactions/${subject.id}/soft-delete`, csrfToken, { change_reason: 'duplicate' });
  const first = await reconcile(await spend(`Trash kept ${suffix}`, 500), -500);
  const later = await reconcile(await spend(`Trash later ${suffix}`, 300), -800);
  return { suffix, checking, subject, first, later, active, deleted };
}

const restoreButton = (page: Page, description: string) =>
  page.getByRole('row').filter({ hasText: description }).getByRole('button', { name: /^Restore/ });

test('a guarded Deleted restore lists every checkpoint before override; cancel writes nothing', async ({ page }) => {
  const s = await seed(page);
  const description = `Trash subject ${s.suffix}`;
  let release!: () => void;
  const held = new Promise<void>((resolve) => { release = resolve; });
  let failPreview = true;
  await page.route(`**/api/v1/transactions/${s.subject.id}/restore/reconciliation-impact`, async (route) => {
    await held;
    if (failPreview) return route.fulfill({ status: 500, json: { error: { code: 'INTERNAL_ERROR', message: 'failure' } } });
    return route.continue();
  });

  await page.goto('/app/ledger/transactions?view=deleted');
  await restoreButton(page, description).click();
  const warning = page.getByRole('alertdialog', { name: 'Restore and invalidate reconciliation?' });
  const proceed = warning.getByRole('button', { name: 'Restore anyway' });
  // Loading: nothing is listed yet, so nothing can be confirmed.
  await expect(warning.getByRole('status')).toHaveText('Loading affected checkpoints…');
  await expect(proceed).toBeDisabled();
  // Error, then retry.
  release();
  await expect(warning.getByRole('alert')).toBeVisible();
  await expect(proceed).toBeDisabled();
  failPreview = false;
  await warning.getByRole('button', { name: 'Retry', exact: true }).click();
  await expect(warning.getByRole('listitem')).toHaveCount(2);
  await expect(warning.getByText(`Trash checking ${s.suffix} (USD) reconciled through`).first()).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);

  await warning.getByRole('button', { name: 'Cancel', exact: true }).click();
  await expect(warning).toHaveCount(0);
  expect(await s.deleted(s.subject.id)).toBe(true);
  expect(await s.active()).toEqual([s.first, s.later].sort());

  await restoreButton(page, description).click();
  await expect(warning.getByRole('listitem')).toHaveCount(2);
  await proceed.click();
  await expect(warning).toHaveCount(0);
  await expect.poll(() => s.deleted(s.subject.id)).toBe(false);
  expect(await s.active()).toEqual([]);
  await expect(page.getByRole('row').filter({ hasText: description })).toHaveCount(0);
});

test('a plain Deleted restore refused by a newer statement opens the checkpoint list', async ({ page }) => {
  const s = await seed(page);
  const description = `Trash subject ${s.suffix}`;
  // A stale list: it still says the restore needs no override.
  await page.route('**/api/v1/transactions/deleted*', async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    for (const row of body.transactions ?? []) row.restore_blocked_by_reconciliation = false;
    await route.fulfill({ response, json: body });
  });
  await page.goto('/app/ledger/transactions?view=deleted');
  await restoreButton(page, description).click();
  const warning = page.getByRole('alertdialog', { name: 'Restore and invalidate reconciliation?' });
  await expect(warning.getByRole('listitem')).toHaveCount(2);
  expect(await s.deleted(s.subject.id)).toBe(true);
  await warning.getByRole('button', { name: 'Restore anyway' }).click();
  await expect.poll(() => s.deleted(s.subject.id)).toBe(false);
  expect(await s.active()).toEqual([]);
});

// #325: a deleted voided transaction is badged with its translated status, not
// the raw `voided` code.
test('a deleted voided transaction shows its translated status', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const checking = await createCashAccount(page, csrfToken, `Trash voided ${suffix}`, currencyID);
  const category = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: `Trash voided groceries ${suffix}`, category_type: 'expense'
  });
  const day = todayISO();
  const description = `Trash voided ${suffix}`;
  const tx = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_date: day, status: 'posted', description,
    journal_entries: [{ entry_date: day, postings: [
      { account_id: checking.id, commodity_id: currencyID, quantity_value: '-700', quantity_scale: 2 },
      { account_id: category.id, commodity_id: currencyID, quantity_value: '700', quantity_scale: 2 }
    ] }]
  });
  await apiJSON(page, 'POST', `/api/v1/transactions/${tx.id}/void`, csrfToken, { change_reason: 'duplicate' });
  await apiJSON(page, 'POST', `/api/v1/transactions/${tx.id}/soft-delete`, csrfToken, { change_reason: 'duplicate' });

  await page.goto('/app/ledger/transactions?view=deleted');
  const row = page.getByRole('row').filter({ hasText: description });
  await expect(row.getByText('Voided', { exact: true })).toBeVisible();
  await expect(row.getByText('voided', { exact: true })).toHaveCount(0);
});
