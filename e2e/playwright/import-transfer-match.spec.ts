import type { Page } from '@playwright/test';
import { expect, test } from './support/fixtures';
import { apiJSON } from './support/api';
import { daysFromTodayISO, todayISO } from './support/dates';
import { expectNoAccessibilityViolations } from './support/a11y';
import { createCashAccount, readyForLedger } from './support/ledger';

// #485: a transfer entered by hand, then both bank statements imported. Each
// statement row is offered as matching the transfer's side on its account;
// linking writes no journal, so the book keeps one transaction, and importing
// either statement again changes nothing.

function dmy(iso: string): string {
  const [year, month, day] = iso.split('-');
  return `${day}/${month}/${year}`;
}

function statement(date: string, payee: string, amount: string): Buffer {
  return Buffer.from(`Datum;Omschrijving;Bedrag\n${dmy(date)};${payee};${amount}\n`);
}

async function upload(page: Page, name: string, buffer: Buffer, mappingName?: string) {
  await page.goto('/app/ledger/import');
  await page.locator('input[type="file"]').setInputFiles({ name, mimeType: 'text/csv', buffer });
  // A new file shows the mapping-name field; a recognised one preselects its saved mapping.
  const nameField = page.getByLabel('Mapping name');
  await expect(nameField.or(page.getByLabel('Saved mapping')).first()).toBeVisible();
  if (mappingName && await nameField.isVisible()) await nameField.fill(mappingName);
  await page.getByRole('button', { name: 'Upload & preview' }).click();
}

async function resolveTo(page: Page, accountID: number, currencyID: number) {
  await page.getByLabel('Target account', { exact: true }).selectOption(String(accountID));
  await page.getByLabel('Currency', { exact: true }).selectOption(String(currencyID));
  await page.getByLabel('Default category').selectOption({ index: 1 });
  await page.getByRole('button', { name: 'Apply to all rows' }).click();
}

async function transactionIDs(page: Page, accountID: number): Promise<number[]> {
  const ledger = await apiJSON<{ transactions: Array<{ id: number }> }>(page, 'GET', `/api/v1/transactions?account_id=${accountID}`);
  return ledger.transactions.map((transaction) => transaction.id);
}

test('[acceptance] a hand-entered transfer and both imported statements stay one transaction', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const checking = await createCashAccount(page, csrfToken, `Match checking ${suffix}`, currencyID);
  const savings = await createCashAccount(page, csrfToken, `Match savings ${suffix}`, currencyID);
  const description = `Savings top-up ${suffix}`;
  const today = todayISO();
  const arrival = daysFromTodayISO(2);

  // Enter the transfer by hand: it leaves today and arrives two days later.
  await page.goto('/app');
  await page.getByRole('list', { name: 'Shortcuts', exact: true }).getByRole('button', { name: 'Transfer', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'New transaction', exact: true });
  await dialog.getByLabel('Date', { exact: true }).fill(today);
  await dialog.getByLabel('Description', { exact: true }).fill(description);
  await dialog.getByRole('combobox', { name: 'From account', exact: true }).selectOption({ label: checking.name });
  await dialog.getByRole('combobox', { name: 'To account', exact: true }).selectOption({ label: savings.name });
  await dialog.getByLabel('Amount', { exact: true }).fill('75.00');
  await dialog.getByRole('button', { name: 'Posts on different days', exact: true }).click();
  await dialog.getByLabel('Arrives on', { exact: true }).fill(arrival);
  const created = page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/transactions');
  await dialog.getByRole('button', { name: 'Save transaction', exact: true }).click();
  expect((await created).status()).toBe(201);
  const { id: transferID } = await (await created).json() as { id: number };
  await expect(dialog).toHaveCount(0);

  // Checking statement: committing straight away stops at the match, which is
  // shown with why it matched. Nothing is preselected, not even the only
  // candidate: the owner links the row explicitly (#485 review).
  const mapping = `Match bank ${suffix}`;
  const checkingFile = statement(today, 'Naar spaarrekening', '-75,00');
  await upload(page, 'checking.csv', checkingFile, mapping);
  await resolveTo(page, checking.id, currencyID);
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByRole('alert').filter({ hasText: 'Decide the matches below, then import again.' })).toBeVisible();
  const checkingMatch = page.getByTestId('import-match-row');
  await expect(checkingMatch.getByText('Same account and currency, exactly', { exact: false })).toContainText('posted on the statement date');
  await expect(checkingMatch.getByRole('radio', { name: new RegExp(`Link to ${description}`) })).not.toBeChecked();
  await expect(checkingMatch.getByRole('radio', { name: 'Keep as a new transaction' })).not.toBeChecked();
  await expectNoAccessibilityViolations(page, 'import match review');
  // A second commit without a choice still links nothing.
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByRole('alert').filter({ hasText: 'Decide the matches below, then import again.' })).toBeVisible();
  await checkingMatch.getByRole('radio', { name: new RegExp(`Link to ${description}`) }).check();
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('1 row linked to a transaction already in your ledger')).toBeVisible();

  // Savings statement on a phone: the row is dated a day before the money
  // arrived; checking first shows the match, then commit links it.
  await page.setViewportSize({ width: 390, height: 844 });
  const savingsFile = statement(daysFromTodayISO(1), 'Van betaalrekening', '75,00');
  await upload(page, 'savings.csv', savingsFile);
  await resolveTo(page, savings.id, currencyID);
  await page.getByRole('button', { name: 'Check for transactions already entered' }).click();
  const savingsMatch = page.getByTestId('import-match-row');
  await expect(savingsMatch.getByText('posted 1 day after the statement date', { exact: false })).toBeVisible();
  await savingsMatch.getByRole('radio', { name: new RegExp(`Link to ${description}`) }).check();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await expectNoAccessibilityViolations(page, 'import match review at 390 px');
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('1 row linked to a transaction already in your ledger')).toBeVisible();

  expect(await transactionIDs(page, checking.id)).toEqual([transferID]);
  expect(await transactionIDs(page, savings.id)).toEqual([transferID]);

  // Importing either statement again is a no-op: the rows are duplicates and
  // nothing matches any more.
  await page.setViewportSize({ width: 1280, height: 900 });
  for (const [name, file, account] of [['checking-again.csv', checkingFile, checking], ['savings-again.csv', savingsFile, savings]] as const) {
    await upload(page, name, file);
    await resolveTo(page, account.id, currencyID);
    await expect(page.getByText('Already imported', { exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Check for transactions already entered' }).click();
    await expect(page.getByText('No statement rows match transactions already in your ledger.')).toBeVisible();
    await page.getByRole('button', { name: 'Commit to ledger' }).click();
    await expect(page.getByText('0 transactions committed')).toBeVisible();
  }
  expect(await transactionIDs(page, checking.id)).toEqual([transferID]);
  expect(await transactionIDs(page, savings.id)).toEqual([transferID]);
});

// A row the owner keeps as new commits as it does without a match.
test('a matching statement row kept as new commits as its own transaction', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const checking = await createCashAccount(page, csrfToken, `Keep new checking ${suffix}`, currencyID);
  const savings = await createCashAccount(page, csrfToken, `Keep new savings ${suffix}`, currencyID);
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_kind: 'transfer', transaction_date: todayISO(), description: `Entered ${suffix}`,
    journal_entries: [{ entry_date: todayISO(), entry_kind: 'ordinary', postings: [
      { account_id: checking.id, commodity_id: currencyID, quantity_value: '-2000', quantity_scale: 2 },
      { account_id: savings.id, commodity_id: currencyID, quantity_value: '2000', quantity_scale: 2 }
    ] }]
  }, [201]);

  await upload(page, 'keep-new.csv', statement(todayISO(), 'Lunch', '-20,00'), `Keep new ${suffix}`);
  await resolveTo(page, checking.id, currencyID);
  await page.getByRole('button', { name: 'Check for transactions already entered' }).click();
  const match = page.getByTestId('import-match-row');
  await match.getByText('Keep as a new transaction').click();
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('1 transactions committed')).toBeVisible();
  await expect(page.getByText(/linked to a transaction/)).toBeHidden();
  expect(await transactionIDs(page, checking.id)).toHaveLength(2);
});

// #485 review: a row linked to the wrong transfer is repaired from the
// transaction's detail panel. The link moves with a reason, the old one is
// kept as history, and re-importing the statement stays a no-op.
test('a statement row linked to the wrong transfer is moved to the right one', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const checking = await createCashAccount(page, csrfToken, `Repair checking ${suffix}`, currencyID);
  const savings = await createCashAccount(page, csrfToken, `Repair savings ${suffix}`, currencyID);
  const transfer = (description: string) => apiJSON<{ id: number }>(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_kind: 'transfer', transaction_date: todayISO(), description,
    journal_entries: [{ entry_date: todayISO(), entry_kind: 'ordinary', postings: [
      { account_id: checking.id, commodity_id: currencyID, quantity_value: '-10000', quantity_scale: 2 },
      { account_id: savings.id, commodity_id: currencyID, quantity_value: '10000', quantity_scale: 2 }
    ] }]
  }, [201]);
  const wrong = await transfer(`Rent share ${suffix}`);
  const right = await transfer(`Savings top-up ${suffix}`);

  const file = statement(todayISO(), 'Van betaalrekening', '100,00');
  await upload(page, 'repair.csv', file, `Repair ${suffix}`);
  await resolveTo(page, savings.id, currencyID);
  await page.getByRole('button', { name: 'Check for transactions already entered' }).click();
  const match = page.getByTestId('import-match-row');
  await expect(match.getByRole('radio')).toHaveCount(3);
  await match.getByRole('radio', { name: new RegExp(`Link to Rent share ${suffix}`) }).check();
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('1 row linked to a transaction already in your ledger')).toBeVisible();

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`/app/ledger/transactions?transaction_id=${wrong.id}`);
  const links = page.getByTestId('statement-links');
  await expect(links.getByText('Linked', { exact: true })).toBeVisible();
  await links.getByRole('button', { name: 'Link to another transaction' }).click();
  const dialog = page.getByRole('dialog', { name: 'Link this statement row to another transaction' });
  await expect(dialog).toBeVisible();
  const target = dialog.getByRole('radio', { name: new RegExp(`Link to Savings top-up ${suffix}`) });
  await expect(target).toBeVisible();
  await expect(dialog.getByRole('button', { name: 'Move link' })).toBeDisabled();
  await expectNoAccessibilityViolations(page, 'statement link reassignment at 390 px');
  await target.check();
  await dialog.getByLabel('Reason').fill('linked to the wrong transfer');
  await dialog.getByRole('button', { name: 'Move link' }).click();
  await expect(dialog).toHaveCount(0);
  await expect(links).toHaveCount(0);

  await page.goto(`/app/ledger/transactions?transaction_id=${right.id}`);
  await expect(page.getByTestId('statement-links').getByText('Linked', { exact: true })).toBeVisible();

  // Unlink keeps the row listed where it was linked, with the reason.
  await page.getByTestId('statement-links').getByRole('button', { name: 'Unlink' }).click();
  const unlink = page.getByRole('dialog', { name: 'Unlink this statement row' });
  await unlink.getByLabel('Reason').fill('checking the statement first');
  await expectNoAccessibilityViolations(page, 'statement link unlink at 390 px');
  await unlink.getByRole('button', { name: 'Unlink row' }).click();
  await expect(unlink).toHaveCount(0);
  await expect(page.getByTestId('statement-links').getByText('Unlinked', { exact: true })).toBeVisible();
  await expect(page.getByTestId('statement-links')).toContainText('checking the statement first');

  // Re-importing the statement is still a no-op.
  await page.setViewportSize({ width: 1280, height: 900 });
  await upload(page, 'repair-again.csv', file);
  await resolveTo(page, savings.id, currencyID);
  await expect(page.getByText('Already imported', { exact: true })).toBeVisible();
  expect(await transactionIDs(page, savings.id)).toHaveLength(2);
});

// Second review: another tab moves the link while the reassignment dialog is
// open. The stale request is refused, the dialog reloads the row by its
// identity (it is no longer listed on this transaction) and asks for a new
// choice, and that choice succeeds.
test('a reassignment refused because another tab moved the link is chosen again and succeeds', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const checking = await createCashAccount(page, csrfToken, `Two tabs checking ${suffix}`, currencyID);
  const savings = await createCashAccount(page, csrfToken, `Two tabs savings ${suffix}`, currencyID);
  const transfer = (description: string) => apiJSON<{ id: number }>(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_kind: 'transfer', transaction_date: todayISO(), description,
    journal_entries: [{ entry_date: todayISO(), entry_kind: 'ordinary', postings: [
      { account_id: checking.id, commodity_id: currencyID, quantity_value: '-10000', quantity_scale: 2 },
      { account_id: savings.id, commodity_id: currencyID, quantity_value: '10000', quantity_scale: 2 }
    ] }]
  }, [201]);
  const wrong = await transfer(`Rent share ${suffix}`);
  const other = await transfer(`Gym fee ${suffix}`);
  const right = await transfer(`Savings top-up ${suffix}`);

  await upload(page, 'two-tabs.csv', statement(todayISO(), 'Van betaalrekening', '100,00'), `Two tabs ${suffix}`);
  await resolveTo(page, savings.id, currencyID);
  await page.getByRole('button', { name: 'Check for transactions already entered' }).click();
  await page.getByTestId('import-match-row').getByRole('radio', { name: new RegExp(`Link to Rent share ${suffix}`) }).check();
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('1 row linked to a transaction already in your ledger')).toBeVisible();

  await page.goto(`/app/ledger/transactions?transaction_id=${wrong.id}`);
  await page.getByTestId('statement-links').getByRole('button', { name: 'Link to another transaction' }).click();
  const dialog = page.getByRole('dialog', { name: 'Link this statement row to another transaction' });
  const toRight = dialog.getByRole('radio', { name: new RegExp(`Link to Savings top-up ${suffix}`) });
  await expect(toRight).toBeVisible();

  // The other tab moves the row to the gym fee.
  const { links } = await apiJSON<{ links: Array<{ identity_id: number; link_id: number; active: boolean }> }>(page, 'GET', `/api/v1/transactions/${wrong.id}/statement-links`);
  const candidates = await apiJSON<{ candidates: Array<{ transaction_id: number; line_key: string; posted_on: string }> }>(page, 'GET', `/api/v1/import-identities/${links[0].identity_id}/link-candidates`);
  const gym = candidates.candidates.find((candidate) => candidate.transaction_id === other.id);
  if (!gym) throw new Error('the gym fee is not a candidate');
  await apiJSON(page, 'POST', `/api/v1/import-identities/${links[0].identity_id}/reassign`, csrfToken, {
    expected_link_id: links[0].link_id, expected_link_active: true, reason: 'other tab',
    transaction_id: gym.transaction_id, line_key: gym.line_key, posted_on: gym.posted_on
  }, [204]);

  await toRight.check();
  await dialog.getByLabel('Reason').fill('linked to the wrong transfer');
  await dialog.getByRole('button', { name: 'Move link' }).click();
  await expect(dialog.getByText('changed after you reviewed it', { exact: false })).toBeVisible();
  await expect(toRight).not.toBeChecked();
  await expect(dialog.getByRole('button', { name: 'Move link' })).toBeDisabled();

  await toRight.check();
  await dialog.getByRole('button', { name: 'Move link' }).click();
  await expect(dialog).toHaveCount(0);
  const moved = await apiJSON<{ links: Array<{ active: boolean }> }>(page, 'GET', `/api/v1/transactions/${right.id}/statement-links`);
  expect(moved.links.map((link) => link.active)).toEqual([true]);
  const left = await apiJSON<{ links: unknown[] }>(page, 'GET', `/api/v1/transactions/${other.id}/statement-links`);
  expect(left.links).toEqual([]);
});
