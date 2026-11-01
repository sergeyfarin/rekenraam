import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { compile } from '@inlang/paraglide-js';
import { afterAll, expect, test } from 'vitest';

// #493: a partial locale may lack a message. Compile a two-locale project the
// way the app does and check that the missing message renders in English, not
// blank or as its key.
const dir = mkdtempSync(join(tmpdir(), 'rekenraam-partial-'));
afterAll(() => rmSync(dir, { recursive: true, force: true }));

test('a message missing from a partial locale renders in English', async () => {
  const plugin = fileURLToPath(new URL('../../../node_modules/@inlang/plugin-message-format/dist/index.js', import.meta.url));
  mkdirSync(join(dir, 'project.inlang'));
  mkdirSync(join(dir, 'messages'));
  writeFileSync(join(dir, 'project.inlang', 'settings.json'), JSON.stringify({
    baseLocale: 'en',
    locales: ['en', 'es'],
    modules: [plugin],
    'plugin.inlang.messageFormat': { pathPattern: join(dir, 'messages', '{locale}.json') }
  }));
  writeFileSync(join(dir, 'messages', 'en.json'), JSON.stringify({ probe_kept: 'Kept', probe_new: 'New {name}' }));
  writeFileSync(join(dir, 'messages', 'es.json'), JSON.stringify({ probe_kept: 'Conservado' }));
  await compile({ project: join(dir, 'project.inlang'), outdir: join(dir, 'out') });

  const messages = await import(/* @vite-ignore */ pathToFileURL(join(dir, 'out', 'messages.js')).href);
  expect(messages.probe_kept({}, { locale: 'es' })).toBe('Conservado');
  expect(messages.probe_new({ name: 'Ana' }, { locale: 'es' })).toBe('New Ana');
}, 30_000);
