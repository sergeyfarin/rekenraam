import { test, expect } from '@playwright/test';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';
import { launchApp, waitForHealthy, stop, repoRoot } from './playwright/support/fixtures';
import { seedReferenceBook } from './playwright/support/reference-book';

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
      const seeded = await seedReferenceBook(page);
      const captures = [
        { route: `/app/ledger/transactions?account_id=${seeded.ids.get('checking')}`, name: 'transactions', heading: 'Transactions' },
        { route: '/app/investments', name: 'investments', heading: 'Investments' }
      ];
      for (const { route, name, heading } of captures) {
        await page.goto(route);
        await expect(page.getByRole('heading', { name: heading, exact: true }).first()).toBeVisible();
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
      const revision = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repoRoot, encoding: 'utf8' }).trim();
      await writeFile(join(repoRoot, 'website/assets/capture.json'), JSON.stringify({
        source_revision: revision,
        fixture: 'fixtures/reference-book.json',
        language: 'en',
        viewport: { width: 1440, height: 1050 },
        files: captures.map(({ name, route }) => ({ file: `${name}.png`, route })),
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
