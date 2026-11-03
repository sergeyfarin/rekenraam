import { readFileSync } from 'node:fs';
import { expect, test } from './support/fixtures';
import { ensureBrowserSession } from './support/session';

/**
 * The locale is resolved from `localStorage` and baked into the rendered
 * markup, so switching it has to survive a document reload. This walks the
 * whole path a user takes — pick a language in settings, then look at an
 * unrelated screen — rather than asserting on the settings page alone.
 */
test('switches the interface language and keeps it across navigation', async ({ page }) => {
  await ensureBrowserSession(page);

  await page.goto('/app/settings/preferences');
  await expect(page.getByRole('heading', { name: 'Language' })).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('lang', 'en');

  await page.getByRole('button', { name: 'Русский' }).click();

  await expect(page.getByRole('heading', { name: 'Язык' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Русский' })).toHaveAttribute('aria-pressed', 'true');

  // The two catalogs compile into one namespace but are separate files, so a
  // string from each has to be checked: 'Язык' comes from messages/app and the
  // settings-section nav from messages/settings.
  await expect(page.getByRole('link', { name: 'Предпочтения' })).toBeVisible();

  await page.goto('/app/reports');
  await expect(page.getByRole('heading', { name: 'Отчёты', level: 1 })).toBeVisible();
  // Assistive technology reads the page in the language it declares (#325).
  await expect(page.locator('html')).toHaveAttribute('lang', 'ru');
  await page.locator('main').getByRole('link', { name: 'Чистые активы', exact: true }).click();
  await expect(page).toHaveURL(/\/app\/reports\/net-worth\?/);
  await expect(page.getByRole('heading', { name: 'Чистые активы', level: 1 })).toBeVisible();
  await expect(page.getByRole('combobox', { name: 'Группировка' })).toBeVisible();

  await page.goto('/app/settings/preferences');
  await page.getByRole('button', { name: 'English' }).click();
  await expect(page.getByRole('heading', { name: 'Language' })).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('lang', 'en');
});

// #493: es, fr and de are partial; their buttons say so with the count of texts
// still shown in English, while the button's name stays the language alone.
test('marks partial languages with their untranslated count', async ({ page }) => {
  const status = JSON.parse(readFileSync(new URL('../../frontend/src/lib/i18n/locale-status.json', import.meta.url), 'utf8')) as { maintained: string[]; partial: string[] };
  const missing = JSON.parse(readFileSync(new URL('../../frontend/src/lib/i18n/missing.json', import.meta.url), 'utf8')) as { counts: Record<string, number> };
  const names: Record<string, string> = { en: 'English', es: 'Español', fr: 'Français', nl: 'Nederlands', de: 'Deutsch', ru: 'Русский' };
  await ensureBrowserSession(page);
  await page.goto('/app/settings/preferences');

  for (const locale of status.partial) {
    const button = page.getByRole('button', { name: names[locale], exact: true });
    const note = page.locator(`#language-partial-${locale}`);
    await expect(note).toContainText('Partial');
    if (missing.counts[locale] > 0) await expect(note).toContainText(String(missing.counts[locale]));
    await expect(button).toHaveAccessibleDescription(/Partial/);
  }
  for (const locale of status.maintained) {
    await expect(page.locator(`#language-partial-${locale}`)).toHaveCount(0);
  }
});
