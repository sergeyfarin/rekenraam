import type { Page } from '@playwright/test';
import { expect, test } from './support/fixtures';
import { apiJSON } from './support/api';
import { daysFromTodayISO, todayISO } from './support/dates';
import { expectNoAccessibilityViolations, focusIsInside } from './support/a11y';
import { createCashAccount, ensureCurrency, readyForLedger } from './support/ledger';

// Transfer entry (#481): the shared entry dialog in Transfer mode, from Home's
// tile, search and an account register. One currency or two, each side on its
// own day, a fee on Bank fees; a saved transfer reopens in Transfer mode.

const transferDialog = (page: Page) => page.getByRole('dialog', { name: 'Transfer between accounts', exact: true });
const shortcuts = (page: Page) => page.getByRole('list', { name: 'Shortcuts', exact: true });

type Journal = {
  transaction_kind: string;
  journal_entries: Array<{
    entry_date: string;
    entry_kind: string;
    postings: Array<{ account_id: number; account_system_role?: string | null; account_class: string; quantity_value: string; commodity_id: number }>;
  }>;
};

async function setTheme(page: Page, theme: 'light' | 'dark') {
  await page.evaluate(value => localStorage.setItem('rekenraam-theme', value), theme);
}

function accountLegs(journal: Journal) {
  return journal.journal_entries.flatMap(entry =>
    entry.postings.filter(p => !p.account_system_role && (p.account_class === 'asset' || p.account_class === 'liability')).map(p => [entry.entry_date, p.account_id, p.quantity_value] as const)
  );
}

for (const theme of ['light', 'dark'] as const) {
  test(`desktop: a two-day transfer with a fee from Home, reopened in Transfer mode, in ${theme}`, async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const suffix = `${theme} ${Date.now()}`;
    const checking = await createCashAccount(page, csrfToken, `Transfer checking ${suffix}`, currencyID);
    const savings = await createCashAccount(page, csrfToken, `Transfer savings ${suffix}`, currencyID);
    const description = `Transfer ${suffix}`;
    const today = todayISO();
    const arrival = daysFromTodayISO(2);
    await setTheme(page, theme);
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto('/app');

    const tile = shortcuts(page).getByRole('button', { name: 'Transfer', exact: true });
    await tile.focus();
    await page.keyboard.press('Enter');
    const dialog = transferDialog(page);
    await expect(dialog).toBeVisible();
    await expect.poll(() => focusIsInside(dialog)).toBe(true);
    await expectNoAccessibilityViolations(page, `desktop transfer dialog ${theme}`);

    // Refusals are translated and write nothing.
    const posts: number[] = [];
    page.on('response', response => {
      if (response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/transactions') posts.push(response.status());
    });
    await dialog.getByRole('combobox', { name: 'From account', exact: true }).selectOption({ label: checking.name });
    await dialog.getByRole('combobox', { name: 'To account', exact: true }).selectOption({ label: checking.name });
    await dialog.getByLabel('Amount', { exact: true }).fill('50.00');
    await dialog.getByRole('button', { name: 'Add transaction', exact: true }).click();
    await expect(dialog.getByText('From and To must be different accounts.')).toBeVisible();
    expect(posts).toEqual([]);

    await dialog.getByLabel('Date', { exact: true }).fill(today);
    await dialog.getByLabel('Description', { exact: true }).fill(description);
    await dialog.getByRole('combobox', { name: 'To account', exact: true }).selectOption({ label: savings.name });
    await dialog.getByLabel('Fee (optional)', { exact: true }).fill('1.50');
    await dialog.getByRole('button', { name: 'Posts on different days', exact: true }).click();
    await dialog.getByLabel('Arrives on', { exact: true }).fill(arrival);
    await expectNoAccessibilityViolations(page, `desktop transfer dialog filled ${theme}`);
    const created = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/transactions');
    await dialog.getByRole('button', { name: 'Add transaction', exact: true }).click();
    expect((await created).status()).toBe(201);
    const { id } = await (await created).json() as { id: number };
    await expect(dialog).toHaveCount(0);
    await expect(tile).toBeFocused();

    const journal = await apiJSON<Journal>(page, 'GET', `/api/v1/transactions/${id}`);
    expect(journal.transaction_kind).toBe('transfer');
    // 50.00 plus the 1.50 fee leaves checking today; 50.00 reaches savings on
    // its day; the fee is its own expense posting.
    const legs = accountLegs(journal);
    expect(legs).toContainEqual([today, checking.id, '-5150']);
    expect(legs).toContainEqual([arrival, savings.id, '5000']);
    const fee = journal.journal_entries.flatMap(e => e.postings).find(p => p.account_class === 'expense');
    expect(fee?.quantity_value).toBe('150');

    // Reopened, it is a transfer again, with every field.
    await page.goto(`/app/ledger/transactions?transaction_id=${id}`);
    const panel = page.getByRole('complementary', { name: 'Actions' });
    await panel.getByRole('button', { name: 'Edit', exact: true }).click();
    await expect(panel.getByRole('heading', { name: 'Edit transaction' })).toBeVisible();
    await expect(panel.getByRole('combobox', { name: 'From account', exact: true })).toHaveValue(String(checking.id));
    await expect(panel.getByRole('combobox', { name: 'To account', exact: true })).toHaveValue(String(savings.id));
    await expect(panel.getByLabel('Amount', { exact: true })).toHaveValue('50.00');
    await expect(panel.getByLabel('Fee (optional)', { exact: true })).toHaveValue('1.50');
    await expect(panel.getByLabel('Arrives on', { exact: true })).toHaveValue(arrival);
  });
}

test.describe('on a touch phone', () => {
  test.use({ hasTouch: true, viewport: { width: 390, height: 844 } });

  test('a currency exchange from an account register, both amounts exact, explained in the detail', async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const eurID = await ensureCurrency(page, csrfToken, 'EUR', 'Euro');
    const suffix = Date.now();
    const euro = await createCashAccount(page, csrfToken, `Exchange euro ${suffix}`, eurID);
    const dollars = await createCashAccount(page, csrfToken, `Exchange dollars ${suffix}`, currencyID);
    const description = `Exchange ${suffix}`;

    await page.goto(`/app/ledger/accounts/${euro.id}/register`);
    await page.getByRole('button', { name: 'Transfer', exact: true }).tap();
    const dialog = transferDialog(page);
    await expect(dialog).toBeVisible();
    await expect.poll(() => dialog.evaluate(node => Math.round(node.getBoundingClientRect().width))).toBe(390);
    // The register's account is the transfer's From.
    await expect(dialog.getByRole('combobox', { name: 'From account', exact: true })).toHaveValue(String(euro.id));
    await dialog.getByLabel('Description', { exact: true }).fill(description);
    await dialog.getByRole('combobox', { name: 'To account', exact: true }).selectOption({ label: dollars.name });
    await dialog.getByLabel('Sent (EUR)', { exact: true }).fill('100.00');
    await dialog.getByLabel('Received (USD)', { exact: true }).fill('108.00');
    await expect(dialog.getByText('Rate: 1 EUR = 1.08 USD')).toBeVisible();
    await expectNoAccessibilityViolations(page, 'phone exchange dialog');

    const created = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/transactions');
    await dialog.getByRole('button', { name: 'Add transaction', exact: true }).tap();
    expect((await created).status()).toBe(201);
    const { id } = await (await created).json() as { id: number };
    await expect(dialog).toHaveCount(0);

    const journal = await apiJSON<Journal>(page, 'GET', `/api/v1/transactions/${id}`);
    expect(journal.journal_entries.map(entry => entry.entry_kind)).toEqual(['exchange']);
    expect(accountLegs(journal).map(([, accountID, value]) => [accountID, value])).toEqual([[euro.id, '-10000'], [dollars.id, '10800']]);
    // The server added one trading leg per currency.
    expect(journal.journal_entries[0].postings.filter(p => p.account_system_role === 'commodity_trading')).toHaveLength(2);

    await page.goto(`/app/ledger/transactions?transaction_id=${id}`);
    const panel = page.getByRole('complementary', { name: 'Actions' });
    await expect(panel.getByText('Currency exchange', { exact: true })).toBeVisible();
    await expect(panel.getByText('Rate: 1 EUR = 1.08 USD')).toBeVisible();
    await expect(panel.getByText("An estimate against a daily reference rate, not the bank's actual spread. Nothing is recorded as a gain or loss.")).toBeVisible();
    await expectNoAccessibilityViolations(page, 'phone exchange detail');
  });
});

type Created = { id: number; version_id: number; journal_entries: Array<{ entry_date: string; postings: Array<{ id: number; account_id: number; memo: string; quantity_value: string; account_system_role?: string | null; account_class: string; line_key: string }> }> };

/** Clears the account's posting and finishes a statement at `balance` cents. */
async function reconcileAccount(page: Page, csrfToken: string, accountID: number, commodityID: number, postingID: number, day: string, balance: number) {
  await apiJSON(page, 'POST', `/api/v1/accounts/${accountID}/postings/reconciliation-status`, csrfToken, {
    posting_version_ids: [postingID], status: 'cleared', cleared_on: day
  });
  const session = await apiJSON<{ id: number }>(page, 'POST', `/api/v1/accounts/${accountID}/reconciliations/start`, csrfToken, {
    commodity_id: commodityID, source_kind: 'statement', statement_date: day,
    statement_balance_value: String(balance), statement_balance_scale: 2
  });
  await apiJSON(page, 'POST', `/api/v1/reconciliations/${session.id}/finish`, csrfToken, { change_reason: 'matched statement' });
}

async function activeCheckpoints(page: Page, accountID: number) {
  const { checkpoints } = await apiJSON<{ checkpoints: Array<{ status: string }> }>(page, 'GET', `/api/v1/accounts/${accountID}/reconciliation-checkpoints`);
  return checkpoints.filter(c => c.status === 'active').length;
}

test.describe('editing on a touch phone', () => {
  test.use({ hasTouch: true, viewport: { width: 390, height: 844 } });

  test('an edited exchange with a receiving fee saves after reviewing both accounts\' checkpoints', async ({ page }) => {
    const { csrfToken, currencyID } = await readyForLedger(page);
    const eurID = await ensureCurrency(page, csrfToken, 'EUR', 'Euro');
    const suffix = Date.now();
    const euro = await createCashAccount(page, csrfToken, `Edit euro ${suffix}`, eurID);
    const dollars = await createCashAccount(page, csrfToken, `Edit dollars ${suffix}`, currencyID);
    const { categories } = await apiJSON<{ categories: Array<{ id: number; builtin_key?: string }> }>(page, 'GET', '/api/v1/categories');
    const bankFees = categories.find(c => c.builtin_key === 'expense.financial.bank_fees')!;
    const day = todayISO();
    // €100.00 left; $107.00 arrived after the receiving bank's $1.00 fee.
    const created = await apiJSON<Created>(page, 'POST', '/api/v1/transactions', csrfToken, {
      transaction_kind: 'transfer', transaction_date: day, status: 'posted', description: `Edit exchange ${suffix}`,
      journal_entries: [{ entry_date: day, entry_kind: 'exchange', postings: [
        { account_id: euro.id, commodity_id: eurID, quantity_value: '-10000', quantity_scale: 2 },
        { account_id: dollars.id, commodity_id: currencyID, quantity_value: '10700', quantity_scale: 2 },
        { account_id: bankFees.id, commodity_id: currencyID, quantity_value: '100', quantity_scale: 2 }
      ] }]
    });
    // Each status change writes a new version, so read the current posting first.
    const posting = async (accountID: number) =>
      (await apiJSON<Created>(page, 'GET', `/api/v1/transactions/${created.id}`)).journal_entries.flatMap(e => e.postings).find(p => p.account_id === accountID)!;
    await reconcileAccount(page, csrfToken, euro.id, eurID, (await posting(euro.id)).id, day, -10000);
    await reconcileAccount(page, csrfToken, dollars.id, currencyID, (await posting(dollars.id)).id, day, 10700);

    await page.goto(`/app/ledger/transactions?transaction_id=${created.id}`);
    const panel = page.getByRole('complementary', { name: 'Actions' });
    await panel.getByRole('button', { name: 'Edit', exact: true }).tap();
    await expect(panel.getByLabel('Sent (EUR)', { exact: true })).toHaveValue('100.00');
    await expect(panel.getByLabel('Received (USD)', { exact: true })).toHaveValue('107.00');
    await expect(panel.getByLabel('Fee (optional)', { exact: true })).toHaveValue('1.00');
    await expect(panel.getByRole('combobox', { name: 'Fee charged by', exact: true })).toHaveValue('receiving');
    // What converted: the fee taken back out of the receiving side.
    await expect(panel.getByText('1 EUR = 1.08 USD')).toBeVisible();

    await panel.getByLabel('Sent (EUR)', { exact: true }).fill('99.00');
    await panel.getByLabel('Received (USD)', { exact: true }).fill('106.50');
    await expectNoAccessibilityViolations(page, 'phone exchange edit');
    await panel.getByRole('button', { name: 'Save transaction', exact: true }).tap();
    const review = page.getByRole('alertdialog', { name: 'This change affects a reconciled period' });
    await expect(review).toBeVisible();
    await expect(review.getByRole('listitem')).toHaveCount(2);
    await expect(review.getByRole('listitem').filter({ hasText: euro.name })).toHaveCount(1);
    await expect(review.getByRole('listitem').filter({ hasText: dollars.name })).toHaveCount(1);
    await review.getByRole('button', { name: 'Proceed and invalidate', exact: true }).tap();
    await expect(review).toHaveCount(0);

    await expect.poll(async () => {
      const journal = await apiJSON<Journal>(page, 'GET', `/api/v1/transactions/${created.id}`);
      return accountLegs(journal).map(([, accountID, value]) => [accountID, value]);
    }).toEqual([[euro.id, '-9900'], [dollars.id, '10650']]);
    const saved = await apiJSON<Journal>(page, 'GET', `/api/v1/transactions/${created.id}`);
    expect(saved.journal_entries[0].postings.find(p => p.account_class === 'expense')?.quantity_value).toBe('100');
    expect(saved.journal_entries[0].postings.filter(p => p.account_system_role === 'commodity_trading').map(p => p.quantity_value).sort())
      .toEqual(['-10750', '9900']);
    expect(await activeCheckpoints(page, euro.id)).toBe(0);
    expect(await activeCheckpoints(page, dollars.id)).toBe(0);
  });
});

test('an unchanged transfer save keeps its memos, fee and days', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const checking = await createCashAccount(page, csrfToken, `Memo checking ${suffix}`, currencyID);
  const savings = await createCashAccount(page, csrfToken, `Memo savings ${suffix}`, currencyID);
  const { categories } = await apiJSON<{ categories: Array<{ id: number; builtin_key?: string }> }>(page, 'GET', '/api/v1/categories');
  const bankFees = categories.find(c => c.builtin_key === 'expense.financial.bank_fees')!;
  const day = todayISO();
  const arrival = daysFromTodayISO(2);
  const created = await apiJSON<Created>(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_date: day, status: 'posted', description: `Memo transfer ${suffix}`,
    journal_entries: [{ entry_date: day, postings: [
      { account_id: checking.id, commodity_id: currencyID, quantity_value: '-5150', quantity_scale: 2, memo: 'sender ref 7' },
      { account_id: savings.id, commodity_id: currencyID, quantity_value: '5000', quantity_scale: 2, memo: 'recipient ref 9', posted_on: arrival },
      { account_id: bankFees.id, commodity_id: currencyID, quantity_value: '150', quantity_scale: 2 }
    ] }]
  });
  // The owner's postings keep their lines and memos; the bridge is the
  // server's and is rebuilt (same amounts and days, new lines) on every save.
  const lines = (journal: Created) => journal.journal_entries.flatMap(e => e.postings.map(p =>
    [e.entry_date, p.account_id, p.quantity_value, p.memo, p.account_system_role ? 'bridge' : p.line_key].join('|'))).sort();
  const before = lines(created);

  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto(`/app/ledger/transactions?transaction_id=${created.id}`);
  const panel = page.getByRole('complementary', { name: 'Actions' });
  await panel.getByRole('button', { name: 'Edit', exact: true }).click();
  await expect(panel.getByLabel('Amount', { exact: true })).toHaveValue('50.00');
  await expect(panel.getByLabel('Arrives on', { exact: true })).toHaveValue(arrival);
  await panel.getByLabel('Description', { exact: true }).fill(`Memo transfer ${suffix} edited`);
  await panel.getByRole('button', { name: 'Save transaction', exact: true }).click();
  await expect.poll(async () => (await apiJSON<Created>(page, 'GET', `/api/v1/transactions/${created.id}`)).version_id).not.toBe(created.version_id);
  const after = await apiJSON<Created>(page, 'GET', `/api/v1/transactions/${created.id}`);
  expect(lines(after)).toEqual(before);
});
