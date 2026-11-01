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
    entry.postings.filter(p => !p.account_system_role).map(p => [entry.entry_date, p.account_id, p.quantity_value] as const)
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
    await expect(panel.getByText('For comparison only: nothing is recorded as a gain or loss.')).toBeVisible();
    await expectNoAccessibilityViolations(page, 'phone exchange detail');
  });
});
