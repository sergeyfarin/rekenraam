import { expect, test } from './support/fixtures';
import { readFileSync } from 'node:fs';
import AxeBuilder from '@axe-core/playwright';
import { ensureCurrency, readyForLedger } from './support/ledger';
import { translationLocales } from './support/locales';

test.use({ viewport: { width: 390, height: 844 } });

for (const locale of translationLocales) {
  test(`[acceptance] currency provider selects have distinct names in every direction on mobile in ${locale}`, async ({ page }) => {
    const m = JSON.parse(readFileSync(new URL(`../../frontend/messages/settings/${locale}.json`, import.meta.url), 'utf8'));
    const name = (pair: string) => {
      const [base, quote] = pair.split('/');
      return m.currencies_provider_assignment_label.replace('{base}', base).replace('{quote}', quote);
    };
    const { csrfToken } = await readyForLedger(page);
    await ensureCurrency(page, csrfToken, 'EUR', 'Euro');
    await page.evaluate(value => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    await page.goto('/app/settings/currencies');
    const row = page.locator('tbody tr').filter({ has: page.getByText('EUR', { exact: true }) });
    await expect(row.locator('select')).toHaveCount(1);
  
    for (const [direction, pairs] of [
      [m.currencies_rate_direction_currency_default, ['EUR/USD']],
      [m.currencies_rate_direction_default_currency, ['USD/EUR']],
      [m.currencies_rate_direction_both, ['EUR/USD', 'USD/EUR']]
    ] as const) {
      await page.getByRole('button', { name: direction, exact: true }).click();
      await expect(row.locator('select')).toHaveCount(pairs.length);
      expect((await new AxeBuilder({ page }).include('main').withRules(['select-name']).analyze()).violations).toEqual([]);
      for (const pair of pairs) {
        const provider = row.getByRole('combobox', { name: name(pair), exact: true });
        await expect(provider).toBeVisible();
        await provider.focus();
        await expect(provider).toBeFocused();
      }
      if (pairs.length === 2) {
        await row.getByRole('combobox', { name: name('EUR/USD'), exact: true }).focus();
        await page.keyboard.press('Tab');
        await expect(row.getByRole('combobox', { name: name('USD/EUR'), exact: true })).toBeFocused();
      }
    }
  });
}
