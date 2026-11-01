import type { Locator, Page } from '@playwright/test';
import { expect, test } from './support/fixtures';
import { apiExactText, apiJSON } from './support/api';
import { todayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';

// The third #481 review: preservation must not depend on the route through
// the editor. Switching Simple ↔ Split keeps each posting's line, so its
// cleared state and checkpoint stay its own, and an edit never rewrites the
// metadata it does not show (an integer above 2^53 included; #311 pattern).

type Posting = { id: number; account_id: number; line_key: string; reconciliation_status: string; cleared_on?: string | null };
type Saved = { id: number; journal_entries: Array<{ postings: Posting[] }> };

async function setup(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const account = await createCashAccount(page, csrfToken, `Preserve account ${Date.now()}`, currencyID);
  const { categories } = await apiJSON<{ categories: Array<{ id: number; builtin_key?: string }> }>(page, 'GET', '/api/v1/categories');
  const category = categories.find(c => c.builtin_key === 'expense.financial.bank_fees')!;
  await page.setViewportSize({ width: 1280, height: 900 });
  return { csrfToken, currencyID, account, category };
}

async function clearedSpend(page: Page, f: Awaited<ReturnType<typeof setup>>, description: string) {
  const day = todayISO();
  const p = (id: number, value: string) => ({ account_id: id, commodity_id: f.currencyID, quantity_value: value, quantity_scale: 2 });
  const created = await apiJSON<Saved>(page, 'POST', '/api/v1/transactions', f.csrfToken, {
    transaction_date: day, description, journal_entries: [{ postings: [p(f.account.id, '-10000'), p(f.category.id, '10000')] }]
  });
  const own = created.journal_entries[0].postings.find(p => p.account_id === f.account.id)!;
  await apiJSON(page, 'POST', `/api/v1/accounts/${f.account.id}/postings/reconciliation-status`, f.csrfToken, {
    posting_version_ids: [own.id], status: 'cleared', cleared_on: day
  });
  return { created, own, day };
}

async function openEditor(page: Page, id: number) {
  await page.goto(`/app/ledger/transactions?transaction_id=${id}`);
  const panel = page.getByRole('complementary', { name: 'Actions' });
  await panel.getByRole('button', { name: 'Edit', exact: true }).click();
  return panel;
}

async function save(page: Page, panel: Locator, id: number) {
  const patch = page.waitForResponse(r => r.request().method() === 'PATCH' && new URL(r.url()).pathname === `/api/v1/transactions/${id}`);
  await panel.getByRole('button', { name: 'Save transaction', exact: true }).click();
  return patch;
}

test('Simple to Split keeps the posting line and its cleared state', async ({ page }) => {
  const f = await setup(page);
  const { created, own, day } = await clearedSpend(page, f, 'Simple identity');
  const panel = await openEditor(page, created.id);
  await panel.getByRole('button', { name: 'Use split entry', exact: true }).click();
  expect((await save(page, panel, created.id)).status()).toBe(200);
  const after = await apiJSON<Saved>(page, 'GET', `/api/v1/transactions/${created.id}`);
  const kept = after.journal_entries[0].postings.find(p => p.account_id === f.account.id)!;
  expect(kept.line_key).toBe(own.line_key);
  expect(kept.reconciliation_status).toBe('cleared');
  expect(kept.cleared_on).toBe(day);
});

test('Simple to Split and back keeps a reconciled posting without a checkpoint override', async ({ page }) => {
  const f = await setup(page);
  const { created, own, day } = await clearedSpend(page, f, 'Simple checkpoint');
  const session = await apiJSON<{ id: number }>(page, 'POST', `/api/v1/accounts/${f.account.id}/reconciliations/start`, f.csrfToken, {
    commodity_id: f.currencyID, source_kind: 'statement', statement_date: day, statement_balance_value: '-10000', statement_balance_scale: 2
  });
  await apiJSON(page, 'POST', `/api/v1/reconciliations/${session.id}/finish`, f.csrfToken, { change_reason: 'matched statement' });

  // Split, then Simple again: both transitions carry the same lines.
  const panel = await openEditor(page, created.id);
  await panel.getByRole('button', { name: 'Use split entry', exact: true }).click();
  await panel.getByRole('button', { name: 'Use simple entry', exact: true }).click();
  await expect(panel.getByLabel('Amount', { exact: true })).toHaveValue('-100.00');
  await panel.getByLabel('Description', { exact: true }).fill('Simple checkpoint edited');
  expect((await save(page, panel, created.id)).status()).toBe(200);
  const after = await apiJSON<Saved>(page, 'GET', `/api/v1/transactions/${created.id}`);
  const kept = after.journal_entries[0].postings.find(p => p.account_id === f.account.id)!;
  expect(kept.line_key).toBe(own.line_key);
  expect(kept.reconciliation_status).toBe('reconciled');
});

test('a split the simple form would change offers no switch to it', async ({ page }) => {
  const f = await setup(page);
  const p = (id: number, value: string, memo = '') => ({ account_id: id, commodity_id: f.currencyID, quantity_value: value, quantity_scale: 2, memo });
  const created = await apiJSON<Saved>(page, 'POST', '/api/v1/transactions', f.csrfToken, {
    transaction_date: todayISO(), description: 'Memo split', journal_entries: [{ postings: [p(f.account.id, '-10000', 'card ref 4'), p(f.category.id, '10000')] }]
  });
  const panel = await openEditor(page, created.id);
  await expect(panel.getByText('Split postings', { exact: false }).first()).toBeVisible();
  await expect(panel.getByRole('button', { name: 'Use simple entry', exact: true })).toHaveCount(0);
});

test('a description edit keeps journal and posting metadata exactly', async ({ page }) => {
  const f = await setup(page);
  const body = `{"transaction_date":"${todayISO()}","description":"Exact metadata","metadata":{"import_ref":9007199254740993},"journal_entries":[{"metadata":{"large_ref":9007199254740993},"postings":[{"account_id":${f.account.id},"commodity_id":${f.currencyID},"quantity_value":"-10000","quantity_scale":2,"metadata":{"source_ref":9007199254740993}},{"account_id":${f.category.id},"commodity_id":${f.currencyID},"quantity_value":"10000","quantity_scale":2}]}]}`;
  const created = JSON.parse(await apiExactText(page, 'POST', '/api/v1/transactions', f.csrfToken, body)) as { id: number };
  const panel = await openEditor(page, created.id);
  await panel.getByLabel('Description', { exact: true }).fill('Exact metadata edited');
  expect((await save(page, panel, created.id)).status()).toBe(200);
  const after = await apiExactText(page, 'GET', `/api/v1/transactions/${created.id}`);
  expect(after).toContain('"description":"Exact metadata edited"');
  expect(after).toContain('"import_ref":9007199254740993');
  expect(after).toContain('"large_ref":9007199254740993');
  expect(after).toContain('"source_ref":9007199254740993');
});
