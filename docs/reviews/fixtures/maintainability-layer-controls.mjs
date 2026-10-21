// Deliberate audit controls, not an application/CI test or defect fix.
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const [prepared, scanner, output] = process.argv.slice(2);
assert(prepared && scanner && output, 'expected prepared source, scanner, output JSON');
const require = createRequire(new URL('../../../frontend/package.json', import.meta.url));
const ts = require('typescript');
const root = mkdtempSync(join(tmpdir(), 'rekenraam-layer-controls-'));
mkdirSync(join(root,'source'),{recursive:true});
const samples = {
  'control.ts': `import {createQuery as query} from '@tanstack/svelte-query';
import {getRows} from '#lib/api/rows.ts';
import {apiClient} from '#lib/api/client.ts';
query(() => ({queryFn: () => getRows()}));
for (const row of rows) getRows(row.id);
apiClient.GET('/probe');
fetch('/probe');
const geometry = Number(12n);
const arithmetic = coefficient * 100;
const text = 'fetch() Number(1)'; // fetch('ignored')
`,
  'control.svelte': `<script lang="ts">
import {createQuery as query} from '@tanstack/svelte-query';
import {getRows} from '#lib/api/rows.ts';
import Detail from './detail.svelte';
query(() => ({queryFn: () => getRows()}));
const text = 'fetch() Number(1)'; // fetch('ignored')
</script>
{#each rows as row}<Detail id={row.id} /><button onclick={() => getRows(row.id)}>{Number(row.id)}</button>{/each}
`
};
for (const [path,source] of Object.entries(samples)) writeFileSync(join(root,'source',path),source);
writeFileSync(join(root,'manifest.json'),JSON.stringify({ref:'synthetic',frontend:Object.keys(samples).map(path=>({path}))}));
const originalArgs=process.argv;
process.argv=[process.execPath,scanner,root,join(root,'sites.csv')];
try { await import(pathToFileURL(resolve(scanner)).href); }
finally { process.argv=originalArgs; }
// Count CSV rows by their unambiguous quoted kind fields. Fixtures contain no
// multiline source cells; source text can itself mention these field values.
const rows=readFileSync(join(root,'sites.csv'),'utf8').trim().split('\n').slice(1);
const count=kind=>rows.filter(row=>row.split(',')[2]===`"${kind}"`).length;
assert.equal(count('query'),2); // Aliased imports in both TS and Svelte.
assert.equal(count('api-import-call'),4);
assert.equal(count('numeric-date'),2); // Includes template expression.
assert.equal(count('component'),1);
assert.equal(count('arithmetic'),1);
assert.equal(count('transport'),2); // Only actual fetch/client calls; no comment/string sites.
assert(rows.some(row=>row.includes('"ForOfStatement"')));
assert(rows.some(row=>row.includes('"EachBlock"')));

const manifest=JSON.parse(readFileSync(join(prepared,'manifest.json'),'utf8'));
async function load(path) {
  const source=readFileSync(join(prepared,'source',path),'utf8');
  const result=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}});
  return import('data:text/javascript;base64,'+Buffer.from(result.outputText).toString('base64'));
}
const {parseForecastFilters}=await load('frontend/src/lib/forecast/forecast-model.ts');
const {isCalendarDate}=await load('frontend/src/lib/dates/calendar-date.ts');
const dates=['2026-02-28','2026-02-29','2026-02-31','2026-04-31','2024-02-29','2026-13-01'].map(date=>({date,
  forecastAccepts:parseForecastFilters(new URLSearchParams({spending_model:'adaptive_v1',history_complete_from:date})).valid,
  calendarValid:isCalendarDate(date)}));
assert.deepEqual(dates.map(d=>d.forecastAccepts),[true,true,true,true,true,false]);
assert.deepEqual(dates.map(d=>d.calendarValid),[true,false,false,false,true,false]);
writeFileSync(output,JSON.stringify({ref:manifest.ref,typescript:require('typescript/package.json').version,
  svelte:require('svelte/package.json').version,synthetic:{tsAndSvelteAliasedQueries:2,apiCalls:4,numericIncludingTemplate:2,
  arithmeticOperators:1,repeatedComponents:1,realTransportCalls:2,commentStringTransportCalls:0,loopContexts:true},forecastCalendarProbe:dates},null,2)+'\n');
console.log('Frontend AST sensitivity and forecast calendar mismatch: confirmed');
