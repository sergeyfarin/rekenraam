import { globSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { parse, type AST } from 'svelte/compiler';
import { expect, it } from 'vitest';

const src = fileURLToPath(new URL('../../', import.meta.url));
const scrollUtility = /\boverflow(?:-[xy])?-(?:auto|scroll)\b/;

// These pickers put their instrument option buttons in the keyboard tab order.
// Identify each exception by its file and listbox label.
const tabbablePickers = [
  'lib/investments/buy-form.svelte',
  'lib/investments/dividend-form.svelte',
  'lib/investments/sell-form.svelte',
  'lib/investments/short-trade-form.svelte'
];

function* elements(node: unknown): Generator<AST.RegularElement> {
  if (node === null || typeof node !== 'object') return;
  if ('type' in node && node.type === 'RegularElement') yield node as AST.RegularElement;
  for (const child of Object.values(node)) yield* elements(child);
}

// The scan took 1.3 s alongside three Chromium workers on 2026-11-22.
it('keeps scrollable combobox listboxes out of the tab order', { timeout: 10_000 }, () => {
  const failures: string[] = [];
  const exceptions = new Set(tabbablePickers);
  for (const file of globSync('**/*.svelte', { cwd: src })) {
    const source = readFileSync(path.join(src, file), 'utf8');
    if (!source.includes('listbox')) continue;
    for (const { attributes, start } of elements(parse(source, { modern: true }).fragment)) {
      const attribute = (name: string) => {
        const match = attributes.find((item) => item.type === 'Attribute' && item.name === name);
        return match ? source.slice(match.start, match.end) : '';
      };
      if (!/^role\s*=\s*(?:["']listbox["']|listbox)$/.test(attribute('role')) || !scrollUtility.test(attribute('class'))) continue;
      if (/^tabindex\s*=\s*(?:["']-1["']|-1|\{\s*-1\s*\})$/.test(attribute('tabindex'))) continue;
      if (attribute('aria-label').includes('m.investments_form_instrument_results()') && exceptions.delete(file)) continue;
      failures.push(`${file}:${source.slice(0, start).split('\n').length}`);
    }
  }
  expect(failures, 'Scrollable combobox listboxes need tabindex="-1"').toEqual([]);
  expect([...exceptions], 'Stale scrollable listbox exceptions').toEqual([]);
});
