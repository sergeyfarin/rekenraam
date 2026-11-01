import { partial } from './locale-status.json';
import { counts } from './missing.json';

// #493: partial locales may lack new messages, which then render in English.
// `catalogs:missing` keeps the counts current; catalogs:check rejects a stale list.
export function isPartialLocale(locale: string): boolean {
  return partial.includes(locale);
}

export function untranslatedCount(locale: string): number {
  return (counts as Record<string, number>)[locale] ?? 0;
}
