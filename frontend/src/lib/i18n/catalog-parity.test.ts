import { readFileSync } from 'node:fs';
import { describe, expect, test } from 'vitest';
import { computeMissing, missingPath, render } from '../../../scripts/missing-messages.mjs';

const project = JSON.parse(readFileSync(new URL('../../../project.inlang/settings.json', import.meta.url), 'utf8'));
const patterns: string[] = project['plugin.inlang.messageFormat'].pathPattern;
const locales: string[] = project.locales;
const baseLocale: string = project.baseLocale;
// #493: maintained locales carry every key; partial ones may lack new keys,
// which render in English and are listed in missing.json beside this file.
const status: { maintained: string[]; partial: string[] } = JSON.parse(readFileSync(new URL('./locale-status.json', import.meta.url), 'utf8'));

// A message is a string, or one plural variant (`declarations`, `selectors`,
// `match`) whose cases the locale chooses for its own plural rules.
type Variant = { declarations: string[]; selectors: string[]; match: Record<string, string> };
type Message = string | Variant[];

function catalog(pattern: string, locale: string): Record<string, Message> {
  const data = JSON.parse(readFileSync(new URL(`../../../${pattern.replace('{locale}', locale)}`, import.meta.url), 'utf8'));
  delete data.$schema;
  return data;
}

const keys = (data: Record<string, Message>) => Object.keys(data).sort();
const placeholders = (text: string) => [...text.matchAll(/\{([A-Za-z_]\w*)\}/g)].map(match => match[1]);
const inputs = (variant: Variant) => variant.declarations.filter(item => item.startsWith('input ')).map(item => item.slice(6).trim());
const parameters = (message: Message) => [...new Set(typeof message === 'string'
  ? placeholders(message)
  : message.flatMap(variant => [...inputs(variant), ...Object.values(variant.match).flatMap(placeholders)]))].sort();
const texts = (message: Message) => typeof message === 'string' ? [message] : message.flatMap(variant => Object.values(variant.match));

// Read the compiler's configuration rather than maintaining a second list:
// newly enabled domains and locales must satisfy the same contract.
for (const pattern of patterns) {
  const base = catalog(pattern, baseLocale);
  describe(pattern, () => {
    for (const locale of locales) {
      const translated = catalog(pattern, locale);
      const partial = status.partial.includes(locale);
      test(partial ? `${locale} (partial) has only base-locale message keys` : `${locale} has exactly the base-locale message keys`, () => {
        if (partial) expect(keys(translated).filter(key => !(key in base))).toEqual([]);
        else expect(keys(translated)).toEqual(keys(base));
      });
      test(`${locale} preserves nonempty messages and parameter names`, () => {
        for (const key of keys(base)) {
          if (partial && !(key in translated)) continue;
          const message = translated[key];
          expect(typeof message === 'string', key).toBe(typeof base[key] === 'string');
          for (const text of texts(message ?? '')) expect(text.trim(), key).not.toBe('');
          expect(parameters(message), key).toEqual(parameters(base[key]));
          if (typeof message !== 'string') {
            // A plural category this locale's rules produce but the message
            // lacks would render the key name; the catch-all prevents that.
            for (const variant of message) {
              expect(Object.keys(variant.match).some(key => key.endsWith('=*')), key).toBe(true);
            }
          }
        }
      });
    }
  });
}

for (const locale of locales) {
  test(`${locale} has no colliding keys in the shared message namespace`, () => {
    const owners = new Map<string, string>();
    for (const pattern of patterns) {
      for (const key of keys(catalog(pattern, locale))) {
        expect(owners.get(key), `${key} in ${pattern}`).toBeUndefined();
        owners.set(key, pattern);
      }
    }
  });
}

test('every configured locale is either maintained or partial, and English is maintained', () => {
  expect([...status.maintained, ...status.partial].sort()).toEqual([...locales].sort());
  expect(status.maintained).toContain(baseLocale);
  expect(status.partial.filter(locale => status.maintained.includes(locale))).toEqual([]);
});

test('missing.json lists what each partial locale lacks (run `pnpm run catalogs:missing`)', () => {
  expect(readFileSync(missingPath, 'utf8')).toBe(render(computeMissing()));
});
