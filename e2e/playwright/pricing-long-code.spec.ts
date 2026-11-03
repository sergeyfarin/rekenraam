import { readFileSync } from 'node:fs';
import type { Locator } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { test, expect } from './support/owned-book';
import { apiJSON } from './support/api';
import { todayISO } from './support/dates';
import { readyForLedger } from './support/ledger';
import { translationLocales } from './support/locales';

test.use({ viewport: { width: 390, height: 844 } });

// Check the rendered text as well as its container: hiding overflow or
// truncating the identifier must not turn this regression green.
async function expectWrappedText(locator: Locator, text: string) {
  await expect(locator).toHaveText(text);
  expect(await locator.evaluate(element => {
    const bounds = element.getBoundingClientRect();
    const range = document.createRange();
    range.selectNodeContents(element);
    return bounds.left >= 0 && bounds.right <= window.innerWidth &&
      element.scrollWidth <= element.clientWidth &&
      [...range.getClientRects()].every(rect => rect.left >= bounds.left && rect.right <= bounds.right);
  })).toBe(true);
}

for (const locale of translationLocales) {
  for (const theme of ['light', 'dark']) {
    test(`[acceptance] long instrument codes and exact pricing fit 390px in ${locale} ${theme}`, async ({ page }) => {
      const { csrfToken, currencyID } = await readyForLedger(page);
      const m = JSON.parse(readFileSync(new URL(`../../frontend/messages/pricing/${locale}.json`, import.meta.url), 'utf8'));
      const currencies = await apiJSON<{ currencies: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/currencies');
      const quote = currencies.currencies.find(currency => currency.id === currencyID)!.code;
      const fixtures: Array<{ code: string; commodityID: number; priceID: number }> = [];
      // #387's reported width, the symbol's 64-character maximum, and a
      // longer commodity code (which has no separate declared length cap).
      for (const length of [41, 64, 256]) {
        const code = `LONG${'W'.repeat(length - 4)}`;
        const instrument = await apiJSON<{ commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
          commodity_code: code, instrument_type: 'stock', display_name: `Synthetic long code ${length}`,
          symbol: code.slice(0, 64), quote_commodity_id: currencyID, trading_commodity_id: currencyID,
          quantity_scale: 12, price_scale: 12
        });
        const price = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/pricing/manual', csrfToken, {
          base_commodity_id: instrument.commodity_id, quote_commodity_id: currencyID, valuation_date: todayISO(),
          price_value: '9007199254740993123', price_scale: 18,
          base_quantity_value: '9007199254740993123', base_quantity_scale: 18, reason: 'Synthetic long code observation'
        });
        fixtures.push({ code, commodityID: instrument.commodity_id, priceID: price.id });
      }
      await page.evaluate(({ locale, theme }) => {
        localStorage.setItem('PARAGLIDE_LOCALE', locale);
        localStorage.setItem('rekenraam-theme', theme);
      }, { locale, theme });
      await page.goto('/app/investments/prices');
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
      await expect(page.locator('#pricing-history-base')).toBeVisible();
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);

      const amount = `9${locale === 'en' ? '.' : ','}007199254740993123`;
      const fill = (template: string, values: Record<string, string>) =>
        template.replace(/\{(\w+)\}/g, (_, key: string) => values[key]);
      for (const fixture of fixtures) {
        const pair = fill(m.pricing_pair, { base: fixture.code, quote });
        const priceText = fill(m.pricing_amount_pair, { price: amount, quote, base: amount, instrument: fixture.code });
        const button = page.getByRole('button', { name: pair, exact: true });
        const card = page.getByRole('listitem').filter({ has: button });
        await expectWrappedText(button, pair);
        await expectWrappedText(card.getByText(priceText, { exact: true }), priceText);
        await button.focus();
        await page.keyboard.press('Enter');
        await expect(page.locator('#pricing-history-base')).toHaveValue(String(fixture.commodityID));
        const row = page.locator(`[data-price-id="${fixture.priceID}"]`);
        await expectWrappedText(row.getByText(priceText, { exact: true }), priceText);
        await expect(page.locator('#pricing-history-base option:checked')).toHaveText(fixture.code);
        for (const id of ['pricing-history-base', 'pricing-history-quote']) {
          const bounds = await page.locator(`#${id}`).boundingBox();
          expect(bounds!.x).toBeGreaterThanOrEqual(0);
          expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(390);
        }

        await row.getByRole('button', { name: m.pricing_correct, exact: true }).click();
        const dialog = page.getByRole('dialog', { name: m.pricing_correct, exact: true });
        await expect(dialog.locator('#price-base option:checked')).toHaveText(fixture.code);
        await expect(dialog.locator('#price-amount')).toHaveValue(amount);
        await dialog.locator('#price-reason').fill('Inspect exact long code correction');
        await dialog.getByRole('button', { name: m.pricing_review, exact: true }).click();
        const review = dialog.getByRole('region', { name: m.pricing_review_title });
        await expectWrappedText(review.getByText(priceText, { exact: true }).first(), priceText);
        expect(await dialog.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true);
        await page.keyboard.press('Escape');
        await page.keyboard.press('Escape');
        await expect(dialog).toBeHidden();
        expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
      }
      expect((await new AxeBuilder({ page }).include('main').analyze()).violations).toEqual([]);
    });
  }
}
