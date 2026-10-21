// Independent projection checks and scanner sensitivity controls for #284.
// Usage: node <this file> <prepared directory> <dupl binary> <jscpd entrypoint>
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const [prepared, dupl, jscpd] = process.argv.slice(2);
assert(prepared && dupl && jscpd, 'expected prepared directory, dupl binary and jscpd entrypoint');
const require = createRequire(new URL('../../../frontend/package.json', import.meta.url));
const { parse } = require('svelte/compiler');
const manifest = JSON.parse(readFileSync(join(prepared, 'manifest.json'), 'utf8'));
let components = 0;
let scriptBlocks = 0;
let projections = 0;
const hash = (data) => createHash('sha256').update(data).digest('hex');
for (const entry of [...manifest.go, ...manifest.frontend]) {
  const source = readFileSync(join(prepared, 'source', entry.path));
  assert.equal(hash(source), entry.sha256);
  for (const projection of entry.projections) {
    assert.equal(hash(readFileSync(join(prepared, 'frontend', projection.path))), projection.sha256);
    projections++;
  }
  if (!entry.path.endsWith('.svelte')) continue;
  components++;
  const text = source.toString();
  const ast = parse(text, { modern: true });
  const scripts = [ast.instance, ast.module].filter(Boolean).sort((a, b) => a.start - b.start);
  const projectedScripts = entry.projections.filter((p) => p.path.endsWith('.ts'));
  assert.equal(projectedScripts.length, scripts.length);
  scripts.forEach((script, i) => {
    const projection = readFileSync(join(prepared, 'frontend', projectedScripts[i].path), 'utf8');
    // Svelte supplies independent content offsets, including a second module script.
    assert.equal(projection.slice(script.content.start, script.content.end), text.slice(script.content.start, script.content.end));
    assert.equal(projection.slice(0, script.content.start).trim(), '');
    assert.equal(projection.slice(script.content.end).trim(), '');
    assert.equal(projection.split('\n').length, text.split('\n').length);
    scriptBlocks++;
  });
  const markup = readFileSync(join(prepared, 'frontend', entry.projections.find((p) => p.path.endsWith('.html')).path), 'utf8');
  const removed = [...scripts, ast.css].filter(Boolean);
  let offset = 0;
  for (const node of removed.sort((a, b) => a.start - b.start)) {
    assert.equal(markup.slice(offset, node.start), text.slice(offset, node.start));
    assert.equal(markup.slice(node.start, node.end).trim(), '');
    offset = node.end;
  }
  assert.equal(markup.slice(offset), text.slice(offset));
}

const controlRoot = mkdtempSync(join(tmpdir(), 'rekenraam-clone-controls-'));
for (const name of ['positive-go', 'negative-go', 'positive-web', 'negative-web']) mkdirSync(join(controlRoot, name));
const sourceLines = (path, first, last) => readFileSync(join(prepared, 'source', path), 'utf8').split('\n').slice(first - 1, last).join('\n');
const goSnippet = 'package controls\n' + sourceLines('backend/internal/app/investment_share_exchange_correction.go', 113, 158) + '\n';
const tsSnippet = sourceLines('frontend/src/lib/investments/split-form.svelte', 218, 257) + '\n';
const markupSnippet = sourceLines('frontend/src/lib/transactions/transaction-detail-panel.svelte', 257, 279) + '\n';
for (const name of ['a', 'b']) {
  writeFileSync(join(controlRoot, 'positive-go', name + '.go'), goSnippet);
  writeFileSync(join(controlRoot, 'positive-web', name + '.ts'), tsSnippet);
  writeFileSync(join(controlRoot, 'positive-web', name + '.html'), markupSnippet);
}
writeFileSync(join(controlRoot, 'negative-go', 'a.go'), 'package controls\nfunc a() {}\n');
writeFileSync(join(controlRoot, 'negative-web', 'a.ts'), Array.from({ length: 20 },
  (_, i) => `export const uniqueValue${i} = ${i};`).join('\n') + '\n');
const positiveGo = execFileSync(dupl, ['-t', '150', '-plumbing', join(controlRoot, 'positive-go')], { encoding: 'utf8' });
const negativeGo = execFileSync(dupl, ['-t', '150', '-plumbing', join(controlRoot, 'negative-go')], { encoding: 'utf8' });
assert(positiveGo.includes('duplicate of'));
assert.equal(negativeGo, '');
let positiveFormats;
for (const mode of ['positive', 'negative']) {
  const output = join(controlRoot, mode + '-report');
  execFileSync(process.execPath, [jscpd, join(controlRoot, mode + '-web'), '--format', 'typescript,markup',
    '--min-lines', '5', '--min-tokens', '100', '--max-lines', '100000', '--max-size', '2mb',
    '--mode', 'mild', '--reporters', 'json', '--silent', '--output', output]);
  const report = JSON.parse(readFileSync(join(output, 'jscpd-report.json'), 'utf8'));
  if (mode === 'positive') {
    positiveFormats = [...new Set(report.duplicates.map((d) => d.format))].sort();
    assert.deepEqual(positiveFormats, ['markup', 'typescript']);
  } else assert.equal(report.duplicates.length, 0);
}
console.log(JSON.stringify({ ref: manifest.ref, originalFiles: manifest.go.length + manifest.frontend.length,
  components, scriptBlocks, projections, svelteCompiler: require('svelte/package.json').version,
  goPositive: true, goNegative: true, webPositiveFormats: positiveFormats, webNegative: true }, null, 2));
