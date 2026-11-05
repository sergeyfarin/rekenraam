import { readFileSync } from 'node:fs';

// #493: which locales a browser journey repeats in. Translation journeys run in
// English and Russian: Russian covers long text, Cyrillic and four plural forms,
// and the other locales share its catalog contract. Formatting and parsing tests
// (numbers, dates, money) keep every configured locale, since those differ by
// locale whether or not a message is translated.
const project = JSON.parse(readFileSync(new URL('../../../frontend/project.inlang/settings.json', import.meta.url), 'utf8')) as { locales: string[] };

export const allLocales: readonly string[] = project.locales;
export const translationLocales = ['en', 'ru'] as const;
export const nonEnglishTranslationLocales = ['ru'] as const;
