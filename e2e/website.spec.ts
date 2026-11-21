import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { allLocales } from './playwright/support/locales';

for (const locale of allLocales) {
  test(`${locale}: localized content, responsive layouts and accessible themes`, async ({ page }) => {
    const failures: string[] = [];
    const origins = new Set<string>();
    page.on('pageerror', error => failures.push(error.message));
    page.on('request', request => origins.add(new URL(request.url()).origin));
    await page.goto(locale === 'en' ? '/' : `/${locale}/`);
    await expect(page.locator('html')).toHaveAttribute('lang', locale);
    await expect(page.locator('h1')).toHaveCount(1);
    await expect(page.locator('.development strong')).not.toBeEmpty();
    await expect(page.locator('#features article')).toHaveCount(4);
    await expect(page.locator('a[href="https://retirement.rekenraam.com"]')).toBeVisible();
    const images = page.locator('.screenshot img');
    await expect(images).toHaveCount(3);
    for (const [index, image] of (await images.all()).entries()) {
      await page.locator(`[data-gallery-select="${index}"]`).click();
      await expect(page.locator('[data-gallery-slide]:visible')).toHaveCount(1);
      await expect(image).toBeVisible();
      await expect.poll(() => image.evaluate(element => (element as HTMLImageElement).naturalWidth)).toBeGreaterThan(0);
    }
    for (const width of [320, 390, 768, 1280]) {
      await page.setViewportSize({ width, height: 900 });
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), `Overflow at ${width}px`).toBeTruthy();
    }
    for (const colorScheme of ['light', 'dark'] as const) {
      await page.emulateMedia({ colorScheme });
      const audit = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze();
      expect(audit.violations, `${locale} ${colorScheme}`).toEqual([]);
    }
    await page.locator('[data-theme-toggle]').click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
    await page.reload();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
    expect(failures).toEqual([]);
    expect([...origins]).toEqual(['http://127.0.0.1:1889']);
  });
}

test('language switching and content work without JavaScript', async ({ browser }) => {
  const context = await browser.newContext({ javaScriptEnabled: false, baseURL: 'http://127.0.0.1:1889' });
  try {
    const page = await context.newPage();
    await page.goto('/');
    await expect(page.locator('.language-picker a')).toHaveCount(allLocales.length);
    for (const locale of [...allLocales].reverse()) {
      await page.locator('.language-picker summary').click();
      await page.locator(`.language-picker a[hreflang="${locale}"]`).click();
      await expect(page.locator('html')).toHaveAttribute('lang', locale);
      await expect(page.locator('#status')).toBeVisible();
      await expect(page.locator('[data-theme-toggle]')).toBeHidden();
      await expect(page.locator('[data-gallery-controls]')).toBeHidden();
      await expect(page.locator('[data-gallery-slide]:visible')).toHaveCount(3);
    }
    const response = await page.goto('/missing-page/');
    expect(response?.status()).toBe(404);
    await expect(page.locator('h1')).toHaveText('Page not found');
    await page.locator('.not-found .button').click();
    await expect(page).toHaveURL('http://127.0.0.1:1889/');
  } finally {
    await context.close();
  }
});

test('screenshot controls wrap, announce selection and work from the keyboard', async ({ page }) => {
  await page.goto('/');
  const slides = page.locator('[data-gallery-slide]');
  const choices = page.locator('[data-gallery-select]');
  await expect(slides.nth(0)).toBeVisible();
  await page.locator('[data-gallery-previous]').click();
  await expect(slides.nth(2)).toBeVisible();
  await expect(choices.nth(2)).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('[data-gallery-announcement]')).toHaveText('Investment positions');
  await page.locator('[data-gallery-next]').click();
  await expect(slides.nth(0)).toBeVisible();
  await page.locator('[data-gallery-next]').focus();
  await page.keyboard.press('Enter');
  await expect(slides.nth(1)).toBeVisible();
  await expect(page.locator('[data-gallery-next]')).toBeFocused();
  await choices.nth(0).focus();
  await page.keyboard.press('Space');
  await expect(slides.nth(0)).toBeVisible();
  await expect(choices.nth(0)).toHaveAttribute('aria-pressed', 'true');
  await expect(choices.nth(1)).toHaveAttribute('aria-pressed', 'false');
  await page.setViewportSize({ width: 320, height: 900 });
  await choices.nth(2).click();
  await expect(slides.nth(2)).toBeVisible();
  const fullImage = slides.nth(2).locator('.screenshot-link');
  const response = await page.request.get((await fullImage.getAttribute('href'))!);
  expect(response.ok()).toBeTruthy();
  expect(response.headers()['content-type']).toBe('image/png');
});
