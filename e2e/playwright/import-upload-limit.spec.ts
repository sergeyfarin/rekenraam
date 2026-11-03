import { expect, test } from './support/fixtures';
import { readFileSync } from 'node:fs';
import { mkdtemp, open, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { readyForLedger } from './support/ledger';
import { apiJSON } from './support/api';
import { translationLocales } from './support/locales';

test.use({ viewport: { width: 390, height: 844 } });

for (const locale of translationLocales) {
  for (const format of ['qif', 'csv']) {
    test(`[acceptance] oversized ${format} shows the size error and recovers in ${locale} at 390px`, async ({ page }) => {
      await readyForLedger(page);
      const app = JSON.parse(readFileSync(new URL(`../../frontend/messages/app/${locale}.json`, import.meta.url), 'utf8'));
      const settings = JSON.parse(readFileSync(new URL(`../../frontend/messages/settings/${locale}.json`, import.meta.url), 'utf8'));
      await page.evaluate(value => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
      await page.goto('/app/ledger/import');
      const before = await apiJSON(page, 'GET', '/api/v1/imports');
      const directory = await mkdtemp(join(tmpdir(), 'rekenraam-upload-'));
      try {
        // Pass a path: Playwright's in-memory file payload has its own 50 MB cap.
        const filename = join(directory, `oversized.${format}`);
        const file = await open(filename, 'w');
        try {
          await file.truncate((50 << 20) + 1);
        } finally {
          await file.close();
        }
        const endpoint = format === 'csv' ? '/api/v1/imports/analyze' : '/api/v1/imports';
        const response = page.waitForResponse(res => new URL(res.url()).pathname === endpoint && res.request().method() === 'POST');
        await page.locator('input[type="file"]').setInputFiles(filename);
        if (format === 'qif') {
          await page.getByRole('button', { name: app.import_upload_submit, exact: true }).click();
        }
        const rejected = await response;
        expect(rejected.status()).toBe(413);
        expect(await rejected.json()).toMatchObject({ error: { code: 'IMPORT_UPLOAD_TOO_LARGE' } });
        await expect(page.getByRole('alert').filter({ hasText: settings.api_error_import_upload_too_large })).toBeVisible();
        expect(await apiJSON(page, 'GET', '/api/v1/imports')).toEqual(before);
        await expect(page.getByRole('button', { name: app.import_commit_button, exact: true })).toBeHidden();

        await page.locator('input[type="file"]').setInputFiles({
          name: `small.${format}`, mimeType: 'text/plain',
          buffer: Buffer.from(format === 'csv' ? 'date,memo,amount\n2026-10-14,Test shop,-1.00\n' : '!Type:Bank\nD10/14/2026\nT-1.00\nPTest shop\n^\n')
        });
        await expect(page.getByText(settings.api_error_import_upload_too_large, { exact: true })).toBeHidden();
        if (format === 'csv') {
          await expect(page.getByRole('combobox', { name: app.import_csv_date_column, exact: true })).toHaveValue('date');
        } else {
          await page.getByRole('button', { name: app.import_upload_submit, exact: true }).click();
          await expect(page.getByRole('button', { name: app.import_commit_button, exact: true })).toBeVisible();
        }
      } finally {
        await rm(directory, { recursive: true, force: true });
      }
    });
  }
}
