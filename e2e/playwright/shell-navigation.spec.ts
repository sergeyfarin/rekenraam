import { expect, test, type Locator, type Page } from './support/fixtures';
import { ensureBrowserSession, signOut } from './support/session';
import { expectNoAccessibilityViolations } from './support/a11y';
import { allLocales } from './support/locales';

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
      // Search opens over the tinted rows: its field, scopes and results (#489).
      await page.keyboard.press('Control+k');
      const search = page.getByRole('combobox', { name: 'Search', exact: true });
      await expect(search).toBeFocused();
      await search.fill('Prices');
      await expect(page.getByRole('option').first()).toBeVisible();
      await expectNoAccessibilityViolations(page, `search ${theme} ${width}`);
      await page.keyboard.press('Escape');
      await expect(page.getByRole('dialog', { name: 'Search', exact: true })).toBeHidden();
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

// #436: the test above names controls in English. Longer translations are what
// overflow a phone menu or break a label, so open the same three surfaces in
// every other locale, located by structure rather than by English name.
//
// One test per theme and width, not one test per locale. Six viewport passes
// with sixteen axe runs take about 33 s under CI load, which is already past
// the 30 s test timeout; failure then lands on whichever surface happens to be
// next. A fresh test per pass keeps the same coverage with enough budget and
// names the exact locale/theme/width that failed.
for (const locale of allLocales.filter((name) => name !== 'en')) {
  for (const theme of ['light', 'dark'] as const) {
    for (const width of [320, 390, 1280]) {
      test(`shell menus and search pass axe and fit in ${locale} at ${width}px in ${theme}`, async ({ page }) => {
        await ensureBrowserSession(page);
        const fits = (locator: Locator) => locator.evaluate((node) => {
          const bounds = node.getBoundingClientRect();
          return bounds.left >= 0 && bounds.right <= innerWidth;
        });

        await page.evaluate(([name, value]) => {
          localStorage.setItem('PARAGLIDE_LOCALE', name);
          localStorage.setItem('rekenraam-theme', value);
        }, [locale, theme]);
        await page.setViewportSize({ width, height: 900 });
        await page.goto('/app/investments/prices');
        await expect(page.locator('html')).toHaveAttribute('lang', locale);
        await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
        const heading = (await page.locator('main h1').innerText()).trim();
        expect(heading).not.toBe('');
        const label = `${locale} ${theme} ${width}`;

        if (width < 768) {
          const phone = page.locator('button[aria-controls="shell-phone-menu"]');
          await phone.click();
          const menu = page.locator('#shell-phone-menu');
          await expect(menu.locator('a[aria-current]')).toHaveCount(1);
          expect(await fits(menu)).toBe(true);
          await expectNoAccessibilityViolations(page, `phone menu ${label}`);
          await page.keyboard.press('Escape');
          await expect(phone).toBeFocused();
          await expect(menu).toBeHidden();
        }

        await page.keyboard.press('Control+k');
        const search = page.getByRole('dialog').getByRole('combobox');
        await expect(search).toBeFocused();
        await search.fill(heading);
        await expect(page.getByRole('option').first()).toBeVisible();
        await expectNoAccessibilityViolations(page, `search ${label}`);
        await page.keyboard.press('Escape');
        await expect(search).toBeHidden();

        const settingsButton = page.locator('button[aria-controls="shell-settings-menu"]');
        await settingsButton.click();
        const settings = page.locator('#shell-settings-menu');
        await expect(settings).toBeVisible();
        expect(await fits(settings)).toBe(true);
        await expectNoAccessibilityViolations(page, `settings menu ${label}`);
        expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBe(0);
        await page.keyboard.press('Escape');
        await expect(settingsButton).toBeFocused();
      });
    }
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
  for (const row of ['.shell-sections-row', '.shell-pages-row']) {
    await expect(page.locator(row)).toHaveCSS('backdrop-filter', 'none');
    expect(await page.locator(row).evaluate((element) => getComputedStyle(element).backgroundColor), row).not.toMatch(/\/ 0\.|rgba/);
  }
  await media.detach();
});

// Reads screenshot pixels (CSS px, top-left origin) by decoding the PNG on a
// blank page, which carries no app CSP and needs no image library.
async function pixels(page: Page, points: Array<{ x: number; y: number }>): Promise<number[][]> {
  const png = (await page.screenshot()).toString('base64');
  const scratch = await page.context().newPage();
  try {
    return await scratch.evaluate(async ({ png, points }) => {
      const image = new Image();
      image.src = `data:image/png;base64,${png}`;
      await image.decode();
      const canvas = document.createElement('canvas');
      canvas.width = image.width;
      canvas.height = image.height;
      const context = canvas.getContext('2d')!;
      context.drawImage(image, 0, 0);
      return points.map(({ x, y }) => [...context.getImageData(Math.round(x), Math.round(y), 1, 1).data.slice(0, 3)]);
    }, { png, points });
  } finally {
    await scratch.close();
  }
}

// Dark content under the whole sticky bar, as when a dark control or chart
// scrolls beneath it. CSSOM styles are allowed under the app's CSP.
async function darkenUnderBar(page: Page) {
  await page.evaluate(() => {
    const layer = document.createElement('div');
    layer.setAttribute('data-test-backdrop', '');
    Object.assign(layer.style, { position: 'fixed', inset: '0 0 auto 0', height: '200px', background: '#000', zIndex: '20', pointerEvents: 'none' });
    document.body.append(layer);
  });
}

test('the current location is a tab joined to what it opens into, also in forced colours (#489)', async ({ page }) => {
  await ensureBrowserSession(page);
  const sections = page.getByRole('navigation', { name: 'Sections', exact: true });
  const fill = (locator: Locator) => locator.evaluate((element) => getComputedStyle(element).backgroundColor);
  for (const width of [390, 1280]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto('/app/investments/prices');
    const tab = width >= 1024
      ? sections.getByRole('link', { name: 'Investments', exact: true })
      : page.getByRole('button', { name: 'Menu: Investments', exact: true });
    const pagesRow = page.locator('.shell-pages-row');
    expect(await fill(tab), `section tab at ${width}px`).toBe(await fill(pagesRow));
    const tabBox = (await tab.boundingBox())!;
    const rowBox = (await pagesRow.boundingBox())!;
    expect(Math.abs(tabBox.y + tabBox.height - rowBox.y), 'the tab meets its pages row').toBeLessThanOrEqual(0.5);
    const underline = await page.getByRole('navigation', { name: 'Investments pages', exact: true })
      .locator('a[aria-current="page"]').evaluate((element) => getComputedStyle(element, '::after').backgroundColor);
    expect(underline).not.toBe('rgba(0, 0, 0, 0)');

    // Normal transparency, dark content beneath: the tab and the row it
    // joins still render as one colour across the edge (#525 review).
    await darkenUnderBar(page);
    const x = tabBox.x + 6;
    const [inTab, inRow] = await pixels(page, [{ x, y: tabBox.y + tabBox.height - 2 }, { x, y: rowBox.y + 2 }]);
    for (const channel of [0, 1, 2]) expect(Math.abs(inTab[channel] - inRow[channel]), `seam at ${width}px: ${inTab} vs ${inRow}`).toBeLessThanOrEqual(1);

    // Home has no pages row: its tab opens into the page, over the bar's border.
    await page.goto('/app');
    const home = width >= 1024
      ? sections.getByRole('link', { name: 'Home', exact: true })
      : page.getByRole('button', { name: 'Menu: Home', exact: true });
    const pageFill = await page.locator('html').evaluate((element) => getComputedStyle(element).backgroundColor);
    expect(await fill(home), `Home tab at ${width}px`).toBe(pageFill);
    const bar = (await page.locator('.shell-bar').boundingBox())!;
    const homeBox = (await home.boundingBox())!;
    expect(homeBox.y + homeBox.height).toBeGreaterThanOrEqual(bar.y + bar.height - 0.5);
  }

  // Review and Settings are the current tab on their own desktop pages; on
  // phones the menu button already is.
  await page.setViewportSize({ width: 1280, height: 900 });
  const pageFill = await page.locator('html').evaluate((element) => getComputedStyle(element).backgroundColor);
  for (const [route, control] of [
    ['/app/review', page.getByRole('banner').getByRole('link', { name: /^Review/ })],
    ['/app/settings', page.getByRole('banner').getByRole('button', { name: 'Settings', exact: true })]
  ] as const) {
    await page.goto(route);
    await expect(control).toHaveAttribute('data-current', '');
    expect(await fill(control), route).toBe(pageFill);
    const bar = (await page.locator('.shell-bar').boundingBox())!;
    const box = (await control.boundingBox())!;
    expect(box.y + box.height, `${route} tab reaches the page`).toBeGreaterThanOrEqual(bar.y + bar.height - 0.5);
    expect(await control.evaluate((element) => getComputedStyle(element).borderBottomLeftRadius), route).toBe('0px');
    await expectNoAccessibilityViolations(page, `${route} current tab`);
  }
  await page.setViewportSize({ width: 390, height: 900 });
  await page.goto('/app/review');
  expect(await page.getByRole('banner').getByRole('link', { name: /^Review/ }).evaluate((element) => getComputedStyle(element).borderBottomLeftRadius)).not.toBe('0px');
  await expect(page.getByRole('button', { name: 'Menu: Review', exact: true })).toBeVisible();

  // Forced colours drop fills: the tabs keep an outline and the page its underline.
  await page.emulateMedia({ forcedColors: 'active' });
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto('/app/investments/prices');
  const current = sections.getByRole('link', { name: 'Investments', exact: true });
  await expect(current).toHaveCSS('border-top-width', '1px');
  await expect(sections.getByRole('link', { name: 'Ledger', exact: true })).toHaveCSS('border-top-width', '0px');
  const forcedUnderline = await page.getByRole('navigation', { name: 'Investments pages', exact: true })
    .locator('a[aria-current="page"]').evaluate((element) => {
      const after = getComputedStyle(element, '::after');
      return { fill: after.backgroundColor, height: after.height };
    });
  expect(forcedUnderline.height).toBe('3px');
  expect(forcedUnderline.fill).not.toBe('rgba(0, 0, 0, 0)');
  await page.goto('/app/settings');
  await expect(page.getByRole('button', { name: 'Settings', exact: true })).toHaveCSS('border-top-width', '1px');
  await page.emulateMedia({ forcedColors: 'none' });
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
