// Browser specs look copy up in the message catalogs (`messages.some_key`).
// A key the app has since removed reads as undefined, and
// `getByRole('button', { name: undefined })` matches every button: the spec
// fails far from its cause, after a full browser run. #459 removed
// `transactions_new` this way and every locale-amounts case failed for three
// commits. Checking the references here catches it in milliseconds.
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { test } from 'node:test';

const here = new URL('./', import.meta.url);
const root = new URL('../', import.meta.url);
const settings = JSON.parse(readFileSync(new URL('frontend/project.inlang/settings.json', root), 'utf8'));
const keys = new Set();
for (const pattern of settings['plugin.inlang.messageFormat'].pathPattern) {
  const catalog = JSON.parse(readFileSync(new URL(`frontend/${pattern.replace(/^\.\//, '').replace('{locale}', settings.baseLocale)}`, root), 'utf8'));
  for (const key of Object.keys(catalog)) keys.add(key);
}

test('every catalog key a browser spec references exists', () => {
  const missing = [];
  for (const directory of ['playwright', 'playwright/support']) {
    for (const file of readdirSync(new URL(`${directory}/`, here), { withFileTypes: true })) {
      if (!file.isFile() || !file.name.endsWith('.ts')) continue;
      const source = readFileSync(new URL(`${directory}/${file.name}`, here), 'utf8');
      for (const [, key] of source.matchAll(/\bmessages\.([a-z][a-z0-9_]*)/g)) {
        if (!keys.has(key)) missing.push(`${directory}/${file.name}: ${key}`);
      }
    }
  }
  assert.deepEqual(missing, []);
});
