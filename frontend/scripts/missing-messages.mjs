// Lists the messages each partial locale still lacks (#493) and writes them to
// src/lib/i18n/missing.json, which the language picker reads for its counts and
// catalogs:check compares against a fresh run. Partial locales render a missing
// message in English, so this list is the catch-up work, not an error.
import { readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const root = new URL('../', import.meta.url);
const read = path => JSON.parse(readFileSync(new URL(path, root), 'utf8'));

export const missingPath = new URL('src/lib/i18n/missing.json', root);

export function computeMissing() {
  const project = read('project.inlang/settings.json');
  const status = read('src/lib/i18n/locale-status.json');
  const patterns = project['plugin.inlang.messageFormat'].pathPattern;
  const catalog = (pattern, locale) => {
    const data = read(pattern.replace(/^\.\//, '').replace('{locale}', locale));
    delete data.$schema;
    return data;
  };
  const keys = {};
  for (const locale of status.partial) {
    keys[locale] = patterns.flatMap(pattern => {
      const translated = catalog(pattern, locale);
      return Object.keys(catalog(pattern, project.baseLocale)).filter(key => !(key in translated));
    }).sort();
  }
  const counts = Object.fromEntries(Object.entries(keys).map(([locale, list]) => [locale, list.length]));
  return { counts, keys };
}

export function render(missing) {
  return `${JSON.stringify(missing, null, 2)}\n`;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const missing = computeMissing();
  writeFileSync(missingPath, render(missing));
  console.log(Object.entries(missing.counts).map(([locale, count]) => `${locale}: ${count} missing`).join(', '));
}
