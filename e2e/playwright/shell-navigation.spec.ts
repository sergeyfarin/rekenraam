import { expect, test } from './support/fixtures';
import { ensureBrowserSession, signOut } from './support/session';
import { expectNoAccessibilityViolations } from './support/a11y';

const range = 'start_date=2026-01-01&end_date=2026-03-31&bucket=month';

test('sections open overviews, tabs use real pages and report filters survive history', async ({ page }) => {
  await ensureBrowserSession(page);
  const sections = page.getByRole('navigation', { name: 'Sections', exact: true });
  for (const [label, path] of [['Ledger', 'ledger'], ['Investments', 'investments'], ['Planning', 'planning'], ['Reports', 'reports']]) {
    await sections.getByRole('link', { name: label, exact: true }).click();
    await expect(page).toHaveURL(`/app/${path}`);
    await expect(sections.getByRole('link', { name: label, exact: true })).toHaveAttribute('aria-current', 'page');
    await expect(page.getByRole('navigation', { name: `${label} pages`, exact: true }).getByRole('link', { name: 'Overview', exact: true })).toHaveAttribute('aria-current', 'page');
    await expect(page).toHaveTitle(`${label} · Rekenraam`);
  }
  await page.getByRole('link', { name: 'Gains · in Investments', exact: true }).click();
  await expect(page).toHaveURL('/app/investments/gains');
  await expect(page.getByRole('navigation', { name: 'Investments pages' }).getByRole('link', { name: 'Gains', exact: true })).toHaveAttribute('aria-current', 'page');

  await page.goto(`/app/reports?view=spending&${range}&group_by=payee&account_id=123&account_id=456#detail`);
  await expect(page).toHaveURL(`/app/reports/spending?${range}&group_by=payee&account_id=123&account_id=456#detail`);
  const reports = page.getByRole('navigation', { name: 'Reports pages', exact: true });
  await reports.getByRole('link', { name: 'Cashflow', exact: true }).click();
  await expect(page).toHaveURL(`/app/reports/cashflow?${range}&group_by=payee&account_id=123&account_id=456`);
  await page.goBack();
  await expect(reports.getByRole('link', { name: 'Spending', exact: true })).toHaveAttribute('aria-current', 'page');
  await page.goForward();
  await expect(reports.getByRole('link', { name: 'Cashflow', exact: true })).toHaveAttribute('aria-current', 'page');
});

test('legacy investment and pricing links preserve state and same-page views react to links and history', async ({ page }) => {
  await ensureBrowserSession(page);
  for (const [from, to] of [
    ['/app/pricing?base_commodity_id=1&quote_commodity_id=2', '/app/investments/prices?base_commodity_id=1&quote_commodity_id=2'],
    ['/app/investments?tab=instruments&search=reference', '/app/investments/instruments?search=reference'],
    ['/app/investments?tab=gains&run_id=9', '/app/investments/gains?run_id=9']
  ]) {
    await page.goto(from);
    await expect(page).toHaveURL(to);
    await expect(page.locator('main h1')).toBeVisible();
  }
  await page.goto('/app/investments');
  const views = page.getByRole('navigation', { name: 'Investments', exact: true });
  await views.getByRole('link', { name: 'Suggestions', exact: true }).click();
  await expect(page).toHaveURL('/app/investments?tab=suggestions');
  await expect(views.getByRole('link', { name: 'Suggestions', exact: true })).toHaveAttribute('aria-current', 'page');
  await views.getByRole('link', { name: 'Splits', exact: true }).click();
  await expect(page).toHaveURL('/app/investments?tab=splits');
  await page.goBack();
  await expect(views.getByRole('link', { name: 'Suggestions', exact: true })).toHaveAttribute('aria-current', 'page');
  await page.goForward();
  await expect(views.getByRole('link', { name: 'Splits', exact: true })).toHaveAttribute('aria-current', 'page');
});

for (const theme of ['light', 'dark'] as const) {
  for (const width of [320, 390, 1280]) {
    test(`shell menus pass axe, fit and return focus at ${width}px in ${theme}`, async ({ page }) => {
      await ensureBrowserSession(page);
      await page.evaluate((value) => localStorage.setItem('rekenraam-theme', value), theme);
      await page.setViewportSize({ width, height: 900 });
      await page.goto('/app/investments/prices');
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
      const pages = page.getByRole('navigation', { name: 'Investments pages', exact: true });
      // Attention badges append their screen-reader sentence to the link name.
      const current = pages.locator('a[href="/app/investments/prices"]');
      await expect(current).toHaveAttribute('aria-current', 'page');
      await expect.poll(() => current.evaluate((node) => {
        const bounds = node.getBoundingClientRect();
        return bounds.left >= 0 && bounds.right <= innerWidth;
      })).toBe(true);
      if (width < 768) {
        const phone = page.getByRole('button', { name: 'Menu: Investments', exact: true });
        await phone.click();
        await expect(phone).toHaveAttribute('aria-expanded', 'true');
        await expectNoAccessibilityViolations(page, `phone menu ${theme} ${width}`);
        await page.getByRole('navigation', { name: 'Sections', exact: true }).getByRole('link', { name: 'Ledger', exact: true }).focus();
        await page.keyboard.press('Escape');
        await expect(phone).toBeFocused();
        await expect(phone).toHaveAttribute('aria-expanded', 'false');
      }
      const settings = page.getByRole('button', { name: 'Settings', exact: true });
      await settings.click();
      await expectNoAccessibilityViolations(page, `settings menu ${theme} ${width}`);
      expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBe(0);
      await page.getByRole('group', { name: 'Theme', exact: true }).getByRole('button', { name: theme === 'light' ? 'Light' : 'Dark', exact: true }).focus();
      await page.keyboard.press('Escape');
      await expect(settings).toBeFocused();
      await page.emulateMedia({ media: 'print' });
      await expect(page.locator('.shell-bar')).toBeHidden();
      await expect(pages).toBeHidden();
    });
  }
}

test('settings theme choice follows system changes, persists a fixed choice and signs out', async ({ page }) => {
  await ensureBrowserSession(page);
  await page.emulateMedia({ colorScheme: 'light' });
  const settings = page.getByRole('button', { name: 'Settings', exact: true });
  await settings.click();
  const theme = page.getByRole('group', { name: 'Theme', exact: true });
  await theme.getByRole('button', { name: 'System', exact: true }).click();
  await expect(theme.getByRole('button', { name: 'System', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await page.emulateMedia({ colorScheme: 'dark' });
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await theme.getByRole('button', { name: 'Light', exact: true }).click();
  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await signOut(page);
  await expect(page.getByRole('heading', { name: 'Sign in to continue' })).toBeVisible();
  await expect(page.getByRole('combobox')).toBeVisible();
  await expect(page.getByRole('group', { name: 'Theme', exact: true })).toBeVisible();
  await page.goto('/app/ledger');
  await expect(page).toHaveURL('/');
});


test('header, first tab and heading align and shell heights stay fixed', async ({ page }) => {
  await ensureBrowserSession(page);
  for (const width of [320, 390, 1280]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto('/app/ledger');
    await expect(page.getByRole('heading', { name: 'Ledger', exact: true })).toBeVisible();
    const heading = await page.locator('main h1').boundingBox();
    const tabs = page.getByRole('navigation', { name: 'Ledger pages', exact: true });
    const firstText = await tabs.getByRole('link', { name: 'Overview', exact: true }).evaluate((element) => {
      const range = document.createRange();
      range.selectNodeContents(element);
      return range.getBoundingClientRect().left;
    });
    expect(Math.abs(firstText - heading!.x)).toBeLessThanOrEqual(1);
    const header = page.locator('.shell-bar header');
    expect((await header.boundingBox())!.height).toBe(56);
    expect((await tabs.boundingBox())!.height).toBe(40);
    const home = width >= 768
      ? page.getByRole('navigation', { name: 'Sections', exact: true }).getByRole('link', { name: 'Home', exact: true })
      : page.getByRole('button', { name: 'Menu: Ledger', exact: true });
    // On phones the menu icon starts at the shared edge; on desktop its Home text does.
    const left = width >= 768 ? await home.evaluate((element) => {
      const range = document.createRange();
      range.selectNodeContents(element);
      return range.getBoundingClientRect().left;
    }) : await home.locator('svg').evaluate((element) => element.getBoundingClientRect().left);
    expect(Math.abs(left - heading!.x)).toBeLessThanOrEqual(1);
    await page.goto('/app/planning/recurring');
    await expect(page.getByRole('heading', { name: 'Recurring', exact: true })).toBeVisible();
    const controls = await page.locator('main [data-page-width] > .space-y-5 > div').first().boundingBox();
    expect(controls!.x).toBe(heading!.x);
  }
  const media = await page.context().newCDPSession(page);
  await media.send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-reduced-transparency', value: 'reduce' }] });
  await expect(page.locator('.shell-bar')).toHaveCSS('backdrop-filter', 'none');
  await media.detach();
});

test('the current tab stays visible when a desktop page narrows to phone width', async ({ page }) => {
  await ensureBrowserSession(page);
  for (const path of ['prices', 'gains']) {
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto(`/app/investments/${path}`);
    const current = page.getByRole('navigation', { name: 'Investments pages', exact: true }).locator(`a[href="/app/investments/${path}"]`);
    await expect(current).toHaveAttribute('aria-current', 'page');
    await page.setViewportSize({ width: 320, height: 844 });
    await expect.poll(() => current.evaluate((element) => {
      const bounds = element.getBoundingClientRect();
      const row = element.closest('ul')!;
      const viewport = row.getBoundingClientRect();
      // The two 40px fades must not obscure the current page's label.
      const left = viewport.left + (row.classList.contains('shell-tabs-fade-start') ? 40 : 0);
      const right = viewport.right - (row.classList.contains('shell-tabs-fade-end') ? 40 : 0);
      return bounds.left >= left - 1 && bounds.right <= right + 1;
    })).toBe(true);
  }
});

test('header menus fit a short phone viewport and keep sign out reachable', async ({ page }) => {
  await ensureBrowserSession(page);
  await page.setViewportSize({ width: 320, height: 320 });
  await page.getByRole('button', { name: 'Menu: Home', exact: true }).click();
  for (const id of ['shell-phone-menu', 'shell-settings-menu']) {
    if (id === 'shell-settings-menu') {
      await page.keyboard.press('Escape');
      await page.getByRole('button', { name: 'Settings', exact: true }).click();
    }
    await expect(page.locator(`#${id}`)).toBeVisible();
    expect(await page.locator(`#${id}`).evaluate((element) => element.getBoundingClientRect().bottom)).toBeLessThanOrEqual(320);
  }
  await page.getByRole('button', { name: 'Sign out', exact: true }).click();
  await expect(page).toHaveURL('/');
  await expect(page.getByRole('heading', { name: 'Sign in to continue' })).toBeVisible();
});
