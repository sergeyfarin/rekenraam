import { test, expect } from '@playwright/test';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';
import { launchApp, waitForHealthy, stop, repoRoot } from './playwright/support/fixtures';
import { seedReferenceBook } from './playwright/support/reference-book';

// The book's euro spending: one commodity, so the report draws its chart.
const spendingPath = '/app/reports/spending?start_date=2026-01-01&end_date=2026-03-31';
const spendingViewport = { width: 1440, height: 1112 };

test('capture website screens from a disposable synthetic book', async ({ browser }) => {
  // Never point this asset generator at an existing installation.
  if (process.env.E2E_BASE_URL) throw new Error('Website screenshots require a disposable local app');
  const directory = await mkdtemp(join(tmpdir(), 'rekenraam-website-'));
  const backend = launchApp(directory, Number(process.env.WEBSITE_CAPTURE_PORT ?? '16980'));
  try {
    await waitForHealthy(backend.app, backend.url, backend.logPath);
    const context = await browser.newContext({
      baseURL: backend.url,
      viewport: { width: 1440, height: 1050 },
      deviceScaleFactor: 1,
      colorScheme: 'light',
      locale: 'en-US'
    });
    try {
      const page = await context.newPage();
      // A disposable database: no run suffix, so names read as the fixture spells them.
      const seeded = await seedReferenceBook(page, { suffix: '' });
      const spendingRoute = `${spendingPath}&commodity_id=${seeded.ids.get('EUR')}`;
      const captures = [
        { route: `/app/ledger/transactions?account_id=${seeded.ids.get('checking')}`, name: 'transactions' },
        { route: '/app/investments', name: 'investments' }
      ];
      for (const { route, name } of captures) {
        await page.goto(route);
        // The shell draws each page's h1 (the overview's is its tab name).
        await expect(page.locator('main h1')).toBeVisible();
        await expect(page.getByRole('table').first().locator('tbody tr').first()).toBeVisible();
        if (name === 'investments') {
          await page.getByRole('table').first().locator('tbody tr').first().click();
          await expect(page.getByRole('complementary', { name: 'Lot detail' })).toContainText('Cost basis');
        }
        await page.evaluate(() => document.fonts.ready);
        await page.screenshot({
          path: join(repoRoot, 'website/assets', `${name}.png`),
          animations: 'disabled'
        });
      }
      // The README and the website's lead image: the spending report over the
      // reference book's quarter, at 2x for the README (#489). The left half
      // stays light and the right half follows the dark theme, so one image
      // shows both without a second scroll position. The website's build
      // copies it and states these pixel sizes.
      const spending = await browser.newContext({
        baseURL: backend.url,
        viewport: spendingViewport,
        deviceScaleFactor: 2,
        colorScheme: 'light',
        locale: 'en-US',
        storageState: await context.storageState()
      });
      try {
        const report = await spending.newPage();
        await report.goto(spendingRoute);
        await expect(report.locator('main h1')).toHaveText('Spending');
        await expect(report.getByRole('table').first().locator('tbody tr').first()).toBeVisible();
        await expect(report.locator('main figure svg').first()).toBeVisible();
        await report.evaluate(() => document.fonts.ready);
        const light = await report.screenshot({ animations: 'disabled' });

        await report.emulateMedia({ colorScheme: 'dark' });
        await expect
          .poll(() => report.evaluate(() => document.documentElement.dataset.theme))
          .toBe('dark');
        await report.evaluate(() => document.fonts.ready);
        const dark = await report.screenshot({ animations: 'disabled' });

        // Compose in the browser: no image dependency is needed, and the two
        // screenshots keep their 2x pixels. Clip at 50% so each half is the
        // same report region, not two differently laid-out crops.
        const composite = await browser.newPage({
          viewport: spendingViewport,
          deviceScaleFactor: 2
        });
        try {
          await composite.setContent(`
            <style>
              html, body { margin: 0; background: #000; }
              #theme-split { position: relative; width: ${spendingViewport.width}px; height: ${spendingViewport.height}px; overflow: hidden; }
              #theme-split img { position: absolute; inset: 0; display: block; width: 100%; height: 100%; }
              #theme-split .light { clip-path: inset(0 50% 0 0); }
              #theme-split .dark { clip-path: inset(0 0 0 50%); }
            </style>
            <div id="theme-split">
              <img class="light" alt="">
              <img class="dark" alt="">
            </div>
          `);
          await composite.locator('#theme-split').evaluate(async (element, images) => {
            const [lightImage, darkImage] = Array.from(element.querySelectorAll('img')) as HTMLImageElement[];
            lightImage.src = `data:image/png;base64,${images.light}`;
            darkImage.src = `data:image/png;base64,${images.dark}`;
            await Promise.all([lightImage.decode(), darkImage.decode()]);
          }, { light: light.toString('base64'), dark: dark.toString('base64') });
          await composite.locator('#theme-split').screenshot({
            path: join(repoRoot, 'docs/images/reports-spending.png'),
            animations: 'disabled'
          });
        } finally {
          await composite.close();
        }
      } finally {
        await spending.close();
      }
      // The pictured code is HEAD only when nothing but these assets differs
      // from it: capture after committing the change, then commit the images.
      const revision = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repoRoot, encoding: 'utf8' }).trim();
      const uncommitted = execFileSync('git', ['status', '--porcelain', '--', '.', ':!website/assets', ':!docs/images'], { cwd: repoRoot, encoding: 'utf8' }).trim();
      await writeFile(join(repoRoot, 'website/assets/capture.json'), JSON.stringify({
        source_revision: revision,
        source_clean: uncommitted === '',
        fixture: 'fixtures/reference-book.json',
        language: 'en',
        viewport: { width: 1440, height: 1050 },
        files: [
          ...captures.map(({ name, route }) => ({ file: `${name}.png`, route })),
          {
            file: '../../docs/images/reports-spending.png',
            route: spendingRoute,
            viewport: spendingViewport,
            device_scale_factor: 2,
            themes: { left: 'light', right: 'dark' }
          }
        ],
        note: 'Disposable database; synthetic reference book; no live providers or owner records.'
      }, null, 2) + '\n');
    } finally {
      await context.close();
    }
  } finally {
    await stop(backend.app);
    backend.log.end();
    await rm(directory, { recursive: true, force: true });
  }
});
