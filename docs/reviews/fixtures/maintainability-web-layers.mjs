// #284 fixed-source frontend signals, including Svelte template expressions.
// Import bindings are lexical; no runtime dispatch or network-count proof.
import assert from 'node:assert/strict';
import { readFileSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { join, posix } from 'node:path';

const [prepared, output] = process.argv.slice(2);
assert(prepared && output, 'expected prepared input directory and output JSON');
const require = createRequire(new URL('../../../frontend/package.json', import.meta.url));
const ts = require('typescript');
const { parse } = require('svelte/compiler');
const manifest = JSON.parse(readFileSync(join(prepared, 'manifest.json'), 'utf8'));
const sites = [], components = [];
const numeric = /^(Number|parseFloat|parseInt|BigInt|Date|Date\.(parse|now|UTC)|Intl\.(NumberFormat|DateTimeFormat))$/;
const query = /^(createQuery|createInfiniteQuery|createQueries)$/;
const iteration = /^(map|flatMap|forEach|reduce|filter|some|every)$/;

function modulePath(file, module) {
  if (module.startsWith('#lib/')) return 'frontend/src/lib/' + module.slice(5);
  if (module.startsWith('.')) return posix.normalize(posix.join(posix.dirname(file), module));
  return module;
}
function record(file, source, node, callee, imports, loops, scope, start) {
  const owner = imports.get(callee.split('.')[0]);
  let kind;
  if (callee.startsWith('operator:')) kind = 'arithmetic';
  else if (numeric.test(callee) || /\.(toFixed|toISOString)$/.test(callee)) kind = 'numeric-date';
  else if (query.test(owner?.imported ?? callee) && owner?.module === '@tanstack/svelte-query') kind = 'query';
  else if (callee === 'fetch' || callee.endsWith('.fetch') || /\.(GET|POST|PUT|PATCH|DELETE)$/.test(callee)) kind = 'transport';
  else if (['date-fns','@internationalized/date'].includes(owner?.module)) kind = 'calendar-helper-call';
  else if (owner?.resolved.startsWith('frontend/src/lib/api/')) kind = 'api-import-call';
  if (!kind) return;
  sites.push({file, line: source.slice(0, start).split('\n').length, kind, callee,
    imported: owner?.module ?? '', loop: loops.join(' > '), scope,
    source: source.split('\n')[source.slice(0, start).split('\n').length - 1].trim()});
}
for (const entry of manifest.frontend) {
  const file = entry.path, source = readFileSync(join(prepared, 'source', file), 'utf8');
  const imports = new Map();
  if (file.endsWith('.svelte')) {
    const ast = parse(source, {modern: true});
    for (const script of [ast.instance, ast.module].filter(Boolean)) {
      for (const item of script.content.body.filter(n => n.type === 'ImportDeclaration')) {
        for (const spec of item.specifiers) imports.set(spec.local.name,
          {module: item.source.value, resolved: modulePath(file, item.source.value), imported: spec.imported?.name ?? spec.local.name});
      }
    }
    const name = n => n?.type === 'Identifier' ? n.name : n?.type === 'MemberExpression' && !n.computed ? name(n.object)+'.'+name(n.property) : '';
    function visit(n, loops = [], scope = '') {
      if (!n || typeof n !== 'object') return;
      if (Array.isArray(n)) { for (const child of n) visit(child, loops, scope); return; }
      if (!n.type) return;
      if (n.type === 'FunctionDeclaration') scope = n.id?.name ?? scope;
      if (n.type === 'VariableDeclarator' && ['ArrowFunctionExpression', 'FunctionExpression'].includes(n.init?.type)) scope = n.id?.name ?? scope;
      if (['EachBlock','ForStatement','ForOfStatement','ForInStatement','WhileStatement'].includes(n.type)) loops = [...loops, n.type];
      if (['BinaryExpression','UnaryExpression'].includes(n.type) && ['+','-','*','/','%','**'].includes(n.operator))
        record(file,source,n,'operator:'+n.operator,imports,loops,scope,n.start);
      if (n.type === 'CallExpression' || n.type === 'NewExpression') {
        const callee = name(n.callee);
        record(file, source, n, callee, imports, loops, scope, n.start);
        if (iteration.test(callee.split('.').at(-1))) loops = [...loops, callee];
      }
      if (n.type === 'Component') components.push({file, line: source.slice(0,n.start).split('\n').length,
        component: n.name, target: imports.get(n.name)?.resolved ?? '', loop: loops.join(' > ')});
      for (const [key, child] of Object.entries(n)) if (!['metadata','parent','loc'].includes(key)) visit(child, loops, scope);
    }
    // Script content and template have disjoint source spans.
    for (const script of [ast.instance, ast.module].filter(Boolean)) visit(script.content);
    visit(ast.fragment);
  } else {
    const ast = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
    assert.equal(ast.parseDiagnostics.length, 0, file);
    for (const stmt of ast.statements.filter(ts.isImportDeclaration)) {
      const module = stmt.moduleSpecifier.text, clause = stmt.importClause;
      if (clause?.name) imports.set(clause.name.text, {module, resolved: modulePath(file,module)});
      const bindings = clause?.namedBindings;
      if (bindings && ts.isNamedImports(bindings)) for (const e of bindings.elements) imports.set(e.name.text,{module,resolved:modulePath(file,module),imported:e.propertyName?.text ?? e.name.text});
      if (bindings && ts.isNamespaceImport(bindings)) imports.set(bindings.name.text,{module,resolved:modulePath(file,module)});
    }
    function visit(n, loops = [], scope = '') {
      if (ts.isFunctionDeclaration(n)) scope = n.name?.text ?? scope;
      if (ts.isVariableDeclaration(n) && n.initializer && (ts.isArrowFunction(n.initializer)||ts.isFunctionExpression(n.initializer))) scope=n.name.getText(ast);
      if (ts.isForStatement(n)||ts.isForOfStatement(n)||ts.isForInStatement(n)||ts.isWhileStatement(n)) loops=[...loops,ts.SyntaxKind[n.kind]];
      const op = ts.isBinaryExpression(n) ? ts.tokenToString(n.operatorToken.kind) : ts.isPrefixUnaryExpression(n) ? ts.tokenToString(n.operator) : '';
      if (['+','-','*','/','%','**'].includes(op)) record(file,source,n,'operator:'+op,imports,loops,scope,n.getStart(ast));
      if (ts.isCallExpression(n)||ts.isNewExpression(n)) {
        const callee=n.expression.getText(ast);
        record(file,source,n,callee,imports,loops,scope,n.getStart(ast));
        if (iteration.test(callee.split('.').at(-1))) loops=[...loops,callee];
      }
      ts.forEachChild(n, child=>visit(child,loops,scope));
    }
    visit(ast);
  }
}
sites.sort((a,b)=>a.file.localeCompare(b.file,'en')||a.line-b.line||a.callee.localeCompare(b.callee,'en'));
components.sort((a,b)=>a.file.localeCompare(b.file,'en')||a.line-b.line);
const rows = [...sites, ...components.map(c=>({file:c.file,line:c.line,kind:'component',
  callee:c.component, imported:c.target, loop:c.loop, scope:'',
  source:readFileSync(join(prepared,'source',c.file),'utf8').split('\n')[c.line-1].trim()}))];
rows.sort((a,b)=>a.file.localeCompare(b.file,'en')||a.line-b.line||a.kind.localeCompare(b.kind,'en')||a.callee.localeCompare(b.callee,'en'));
const columns=['file','line','kind','callee','imported','loop','scope','source'];
const cell = value => '"'+String(value ?? '').replaceAll('"','""')+'"';
writeFileSync(output,[columns,...rows.map(r=>columns.map(c=>r[c]))].map(row=>row.map(cell).join(',')).join('\n')+'\n');
console.log(JSON.stringify({files:manifest.frontend.length,sites:sites.length,components:components.length}));
