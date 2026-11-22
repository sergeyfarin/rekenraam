import { expect, test } from './support/fixtures';
import { apiJSON } from './support/api';
import { todayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';

type Stored = {
  version_id: number;
  tag_ids: number[];
  journal_entries: Array<{ postings: Array<{ account_id: number; tag_ids: number[] }> }>;
};

// #650: archiving a tag stops new assignments but keeps the ones it has. The
// editor shows an archived assignment, labelled, so an unrelated edit keeps it
// and the owner can clear it; it never offers the archived tag anywhere else.
test('an edit keeps an archived tag the transaction carries until the owner clears it', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const description = `Archived tag lunch ${suffix}`;
  const account = await createCashAccount(page, csrfToken, `Archived tag wallet ${suffix}`, currencyID);
  const category = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: `Archived tag meals ${suffix}`, category_type: 'expense'
  });
  const tag = (name: string) => apiJSON<{ id: number; name: string }>(page, 'POST', '/api/v1/tags', csrfToken, { name, kind: 'flag' });
  const retired = await tag(`Retired ${suffix}`);
  const first = await tag(`First ${suffix}`);
  const second = await tag(`Second ${suffix}`);
  const today = todayISO();
  const created = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_date: today, status: 'posted', description, tag_ids: [retired.id, first.id],
    journal_entries: [{ entry_date: today, postings: [
      { account_id: account.id, commodity_id: currencyID, quantity_value: '-1250', quantity_scale: 2 },
      { account_id: category.id, commodity_id: currencyID, quantity_value: '1250', quantity_scale: 2, tag_ids: [retired.id] }
    ] }]
  });
  await apiJSON(page, 'POST', `/api/v1/tags/${retired.id}/archive`, csrfToken);

  const read = () => apiJSON<Stored>(page, 'GET', `/api/v1/transactions/${created.id}`);
  const categoryTags = (stored: Stored) =>
    stored.journal_entries.flatMap((entry) => entry.postings).find((posting) => posting.account_id === category.id)?.tag_ids;
  const archivedLabel = `${retired.name} (archived)`;

  await page.goto('/app/ledger/transactions');
  const row = page.getByRole('row').filter({ hasText: description });
  const edit = page.getByRole('complementary', { name: 'Actions' });
  const save = async () => {
    const before = (await read()).version_id;
    await edit.getByRole('button', { name: 'Save transaction' }).click();
    await expect.poll(async () => (await read()).version_id).not.toBe(before);
    // Saving opens the detail panel, which covers the list on a phone.
    await page.getByRole('button', { name: 'Close', exact: true }).click();
  };
  const open = async () => {
    await row.getByRole('button', { name: 'Edit', exact: true }).click();
    await expect(edit.getByRole('heading', { name: 'Edit transaction' })).toBeVisible();
  };

  // A description-only edit keeps the archived tag at both levels.
  await open();
  await expect(edit.getByRole('checkbox', { name: archivedLabel })).toBeChecked();
  await expect(edit.getByText('Archived tags stay on this transaction until you clear them.')).toBeVisible();
  await edit.getByLabel('Description').fill(`${description} edited`);
  await save();
  let stored = await read();
  expect(stored.tag_ids.sort()).toEqual([retired.id, first.id].sort());
  expect(categoryTags(stored)).toEqual([retired.id]);

  // Changing an active tag keeps the archived one.
  await open();
  await edit.getByRole('checkbox', { name: first.name, exact: true }).uncheck();
  await edit.getByRole('checkbox', { name: second.name, exact: true }).check();
  await save();
  stored = await read();
  expect(stored.tag_ids.sort()).toEqual([retired.id, second.id].sort());
  expect(categoryTags(stored)).toEqual([retired.id]);

  // Clearing it removes the assignment, and it is not offered again.
  await open();
  await edit.getByRole('checkbox', { name: archivedLabel }).uncheck();
  await save();
  stored = await read();
  expect(stored.tag_ids).toEqual([second.id]);
  expect(categoryTags(stored)).toEqual([retired.id]);
  await open();
  await expect(edit.getByRole('checkbox', { name: second.name, exact: true })).toBeChecked();
  await expect(edit.getByRole('checkbox', { name: archivedLabel })).toHaveCount(0);
});
