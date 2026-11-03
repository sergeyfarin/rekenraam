import { expect, test } from './support/fixtures';
import { seedReferenceBook } from './support/reference-book';
import { goHomeInApp } from './support/session';

for (const width of [1280, 390]) {
  test(`section paths preserve old links and filtered register history at ${width}px (#460)`, async ({ page }) => {
    test.setTimeout(120_000);
    await page.setViewportSize({ width, height: 900 });
    const { ids } = await seedReferenceBook(page);
    const state = '?highlight=first&highlight=second&empty=&future=a%2Bb+%26+c#detail';
    const paths = [
      ['/app/accounts', '/app/ledger/accounts', 'Accounts'],
      [`/app/accounts/${ids.get('checking')}/register`, `/app/ledger/accounts/${ids.get('checking')}/register`, 'Accounts'],
      ['/app/transactions', '/app/ledger/transactions', 'Transactions'],
      ['/app/categories', '/app/ledger/categories', 'Categories'],
      [`/app/categories/${ids.get('groceries')}`, `/app/ledger/categories/${ids.get('groceries')}`, 'Categories'],
      ['/app/import', '/app/ledger/import', 'Import'],
      ['/app/import/dividends', '/app/ledger/import/dividends', 'Import'],
      ['/app/reconcile', '/app/ledger/reconcile', 'Reconcile'],
      ['/app/budgets', '/app/planning/budgets', 'Budgets'],
      ['/app/recurring', '/app/planning/recurring', 'Recurring'],
      ['/app/forecast', '/app/planning/forecast', 'Forecast']
    ];

    for (const [from, to, tab] of paths) {
      await test.step(from, async () => {
        await page.goto(`${from}${state}`);
        await expect(page).toHaveURL(`${to}${state}`);
        await expect(page.locator('main h1')).toBeVisible();
        const section = to.startsWith('/app/ledger') ? 'Ledger' : 'Planning';
        const tabPath = to.includes('/register') ? '/app/ledger/accounts'
          : to.includes('/categories/') ? '/app/ledger/categories'
          : to.endsWith('/dividends') ? '/app/ledger/import' : to;
        const current = page.getByRole('navigation', { name: `${section} pages`, exact: true })
          .locator(`a[href="${tabPath}"]`);
        await expect(current).toHaveAttribute('aria-current', 'page');
        await expect(current).toContainText(tab);
      });
    }

    // The redirect must happen before the real screen consumes its filters.
    const register = `/app/ledger/accounts/${ids.get('checking')}/register?after_date=2026-01-01&before_date=2026-01-31&status=posted`;
    await page.goto(register.replace('/ledger', ''));
    await expect(page).toHaveURL(register);
    const assertRegister = async () => {
      await expect(page.getByLabel('From', { exact: true })).toHaveValue('2026-01-01');
      await expect(page.getByLabel('To', { exact: true })).toHaveValue('2026-01-31');
      await expect(page.getByRole('combobox', { name: 'Status', exact: true })).toHaveValue('posted');
      await expect(page.getByRole('row').filter({ hasText: 'January salary' })).toHaveCount(1);
    };
    await assertRegister();
    await page.reload();
    await assertRegister();
    const shared = await page.context().newPage();
    try {
      await shared.goto(register);
      await expect(shared.getByRole('row').filter({ hasText: 'January salary' })).toHaveCount(1);
    } finally {
      await shared.close();
    }
    await goHomeInApp(page);
    await expect(page).toHaveURL('/app');
    await page.goBack();
    await expect(page).toHaveURL(register);
    await assertRegister();
    await page.goForward();
    await expect(page).toHaveURL('/app');
  });
}
