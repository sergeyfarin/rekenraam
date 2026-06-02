import type { CurrencyCatalogEntry } from '$lib/api/currencies';

export type LocalizedCurrencyCatalogEntry = CurrencyCatalogEntry & {
  name: string;
  label: string;
};

const regionCurrencyCodes: Record<string, string> = {
  AT: 'EUR',
  AU: 'AUD',
  BE: 'EUR',
  BR: 'BRL',
  CA: 'CAD',
  CH: 'CHF',
  CN: 'CNY',
  CZ: 'CZK',
  DE: 'EUR',
  DK: 'DKK',
  ES: 'EUR',
  FI: 'EUR',
  FR: 'EUR',
  GB: 'GBP',
  IE: 'EUR',
  IN: 'INR',
  IT: 'EUR',
  JP: 'JPY',
  MX: 'MXN',
  NL: 'EUR',
  NO: 'NOK',
  NZ: 'NZD',
  PL: 'PLN',
  PT: 'EUR',
  SE: 'SEK',
  US: 'USD',
  ZA: 'ZAR'
};

const commonCurrencyCodes = ['USD', 'EUR', 'CAD', 'AUD', 'GBP', 'JPY', 'CHF', 'NZD', 'SEK', 'NOK', 'DKK', 'BTC'];

export function localizedCurrencyCatalog(
  catalog: CurrencyCatalogEntry[],
  displayNames: Intl.DisplayNames | null
): LocalizedCurrencyCatalogEntry[] {
  return catalog.map((currency) => {
    const name = displayNames?.of(currency.code) ?? currency.english_name;

    return {
      ...currency,
      name,
      label: `${currency.code} - ${name}`
    };
  });
}

export function localeCurrencyCode(locales: readonly string[]) {
  for (const locale of locales) {
    const region = regionFromLocale(locale);
    const currencyCode = region ? regionCurrencyCodes[region] : undefined;

    if (currencyCode) {
      return currencyCode;
    }
  }

  return 'USD';
}

export function quickCurrencyCodes(catalog: LocalizedCurrencyCatalogEntry[], localeCode: string) {
  const catalogCodes = new Set(catalog.map((currency) => currency.code));

  return Array.from(new Set([localeCode, ...commonCurrencyCodes])).filter((code) => catalogCodes.has(code));
}

function regionFromLocale(locale: string) {
  try {
    const parsedLocale = new Intl.Locale(locale);

    if (parsedLocale.region) {
      return parsedLocale.region.toUpperCase();
    }
  } catch {
    // Fall through to the simple BCP-47 region match below.
  }

  return locale.match(/[-_]([A-Za-z]{2})\b/)?.[1]?.toUpperCase();
}
