/**
 * Autonyms for the locales the app ships. A locale is always offered in its own
 * language: someone hunting for their language in a list they cannot read needs
 * to see "Nederlands", not whatever the current locale calls Dutch.
 */
export const localeAutonyms: Record<string, string> = {
  en: 'English',
  es: 'Español',
  fr: 'Français',
  nl: 'Nederlands',
  de: 'Deutsch',
  ru: 'Русский'
};

export function localeAutonym(locale: string): string {
  return localeAutonyms[locale] ?? locale;
}
