import { test, expect } from './support/owned-book';
import { apiJSON, csrfTokenFor } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';
import { expectNoAccessibilityViolations } from './support/a11y';

// Home's briefing (#476). An owned book, because the headline is a book-wide
// figure and the empty-book card needs a book with no accounts.

test('an empty book starts with an account, not a briefing of zeroes', async ({ page }) => {
  await readyForLedger(page);
  // Browser setup leaves the book with an account, so the empty answer is stubbed.
  await page.route('**/api/v1/home', route => route.fulfill({ json: {
    as_of: '2026-10-30', start_boundary: '2026-09-30', reporting_currency: null, empty_book: true,
    headlines: ['net_worth', 'cash'].map(id => ({ id, account_count: 0, value: null, start_value: null, change: null, unavailable: 'no_accounts', missing: [], stale: [] }))
  } }));
  await page.goto('/app');
  await expect(page.getByRole('heading', { name: 'Start with an account' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Add an account' })).toHaveAttribute('href', '/app/ledger/accounts');
  await expect(page.getByTestId('home-headline')).toHaveCount(0);
  await expect(page.getByTestId('home-status')).toHaveCount(0);
});

for (const theme of ['light', 'dark'] as const) {
  test(`the briefing shows net worth, its month change and one Home read in ${theme}`, async ({ page }) => {
    const { currencyID } = await readyForLedger(page);
    let csrfToken = await csrfTokenFor(page);
    await apiJSON(page, 'POST', `/api/v1/currencies/${currencyID}/default`, csrfToken, undefined, [200]);
    csrfToken = await csrfTokenFor(page);
    const account = await createCashAccount(page, csrfToken, 'Briefing checking', currencyID);
    const salary = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name: 'Briefing salary', category_type: 'income' });
    const { owner_today: today } = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
    await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
      status: 'posted', transaction_date: today, journal_entries: [{ entry_date: today, postings: [
        { account_id: account.id, commodity_id: currencyID, quantity_value: '123456', quantity_scale: 2 },
        { account_id: salary.id, commodity_id: currencyID, quantity_value: '-123456', quantity_scale: 2 }
      ] }]
    });
    await page.evaluate(value => localStorage.setItem('rekenraam-theme', value), theme);

    const homeReads: string[] = [];
    page.on('request', request => { if (new URL(request.url()).pathname === '/api/v1/home') homeReads.push(request.url()); });
    await page.goto('/app');
    const headline = page.getByTestId('home-headline');
    // 1,234.56 is 100 or more, so Home shows it without minor units.
    await expect(headline).toContainText('Net worth');
    await expect(headline).toContainText(/1,235/);
    await expect(headline).not.toContainText('1,234.56');
    await expect(headline).toContainText(/\+.*1,235 in /);
    await expect(headline.getByText(/from the end of .* to /)).toHaveCount(1);
    const status = page.getByTestId('home-status');
    await expect(status.getByRole('link').first()).toHaveAttribute('href', /^\/app\//);
    expect(homeReads).toHaveLength(1);

    const menu = page.getByRole('button', { name: 'Net worth, Choose the headline' });
    await menu.click();
    const choices = page.getByRole('group', { name: 'Choose the headline' });
    await expect(choices.getByRole('button', { name: /^Net worth All \d+ accounts$/ })).toHaveAttribute('aria-pressed', 'true');
    await page.keyboard.press('Escape');
    await expect(menu).toBeFocused();
    await menu.click();
    await choices.getByRole('button', { name: /^Cash/ }).click();
    await expect(page.getByRole('button', { name: 'Cash, Choose the headline' })).toBeFocused();
    await expect(headline).toContainText(/1,235/);

    await expectNoAccessibilityViolations(page, `home briefing ${theme} desktop`);
    await page.setViewportSize({ width: 390, height: 844 });
    await expectNoAccessibilityViolations(page, `home briefing ${theme} phone`);
    await page.setViewportSize({ width: 320, height: 720 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}
