import { expect, test } from '@playwright/test';
import { ensureBrowserSession } from './support/session';

/**
 * The locale is resolved from `localStorage` and baked into the rendered
 * markup, so switching it has to survive a document reload. This walks the
 * whole path a user takes — pick a language in settings, then look at an
 * unrelated screen — rather than asserting on the settings page alone.
 */
test('switches the interface language and keeps it across navigation', async ({ page }) => {
  await ensureBrowserSession(page);

  await page.goto('/app/settings/language');
  await expect(page.getByRole('heading', { name: 'Language' })).toBeVisible();

  await page.getByRole('button', { name: 'Русский' }).click();

  await expect(page.getByRole('heading', { name: 'Язык' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Русский' })).toHaveAttribute('aria-pressed', 'true');

  // The two catalogs compile into one namespace but are separate files, so a
  // string from each has to be checked: 'Язык' comes from messages/app and the
  // settings-section nav from messages/settings.
  await expect(page.getByRole('link', { name: 'Оформление' })).toBeVisible();

  await page.goto('/app/reports');
  await expect(page.getByRole('heading', { name: 'Отчёты', level: 1 })).toBeVisible();
  await expect(page.getByRole('combobox', { name: 'Группировка' })).toBeVisible();

  await page.goto('/app/settings/language');
  await page.getByRole('button', { name: 'English' }).click();
  await expect(page.getByRole('heading', { name: 'Language' })).toBeVisible();
});
