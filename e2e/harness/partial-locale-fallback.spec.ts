import { execFileSync } from 'node:child_process';
import { cpSync, existsSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { createServer, type Server } from 'node:http';
import type { AddressInfo } from 'node:net';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { expect, test } from '@playwright/test';

// #493: Spanish, French and German are partial locales, so a message they lack
// must render in English on a real screen. Every catalog is complete today, so
// this builds an isolated copy of the frontend with one Spanish message removed,
// serves it without a backend, and checks the screen that shows it. The copy is
// discarded afterwards; the repository's catalogs are never touched.
const frontend = fileURLToPath(new URL('../../frontend/', import.meta.url));
const removed = { domain: 'settings', key: 'install_gate_retry' };

let workdir = '';
let server: Server;
let origin = '';

test.beforeAll(async () => {
  test.setTimeout(240_000);
  workdir = mkdtempSync(path.join(tmpdir(), 'rekenraam-partial-locale-'));
  const copy = path.join(workdir, 'frontend');
  const skip = new Set(['node_modules', 'build', '.svelte-kit', 'paraglide']);
  cpSync(frontend, copy, { recursive: true, filter: (source) => !skip.has(path.basename(source)) });
  symlinkSync(path.join(frontend, 'node_modules'), path.join(copy, 'node_modules'), 'dir');

  const catalogPath = path.join(copy, 'messages', removed.domain, 'es.json');
  const catalog = JSON.parse(readFileSync(catalogPath, 'utf8')) as Record<string, unknown>;
  expect(catalog[removed.key], 'the probe message must exist before it is removed').toBeTruthy();
  delete catalog[removed.key];
  writeFileSync(catalogPath, `${JSON.stringify(catalog, null, 2)}\n`);

  execFileSync(path.join(copy, 'node_modules', '.bin', 'vite'), ['build'], { cwd: copy, stdio: 'pipe' });
  const build = path.join(copy, 'build');

  // Static files with the SPA fallback; every API call fails, as with no backend.
  const types: Record<string, string> = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json', '.svg': 'image/svg+xml', '.woff2': 'font/woff2' };
  server = createServer((request, response) => {
    const url = new URL(request.url ?? '/', 'http://localhost');
    if (url.pathname.startsWith('/api/')) {
      response.writeHead(503, { 'content-type': 'application/json' }).end('{"error":{"code":"UNAVAILABLE","message":"unavailable"}}');
      return;
    }
    const file = path.join(build, path.normalize(url.pathname));
    const target = file.startsWith(build) && existsSync(file) && !file.endsWith(path.sep) && path.extname(file) ? file : path.join(build, 'index.html');
    response.writeHead(200, { 'content-type': types[path.extname(target)] ?? 'application/octet-stream' }).end(readFileSync(target));
  });
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve));
  origin = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
});

test.afterAll(async () => {
  await new Promise<void>((resolve) => (server ? server.close(() => resolve()) : resolve()));
  if (workdir) rmSync(workdir, { recursive: true, force: true });
});

test('a message missing from a partial locale renders in English on the sign-in screen', async ({ page }) => {
  const en = JSON.parse(readFileSync(path.join(frontend, 'messages', removed.domain, 'en.json'), 'utf8')) as Record<string, string>;
  const es = JSON.parse(readFileSync(path.join(frontend, 'messages', removed.domain, 'es.json'), 'utf8')) as Record<string, string>;
  const app = JSON.parse(readFileSync(path.join(frontend, 'messages', 'app', 'es.json'), 'utf8')) as Record<string, string>;
  await page.addInitScript(() => localStorage.setItem('PARAGLIDE_LOCALE', 'es'));
  await page.goto(`${origin}/`);

  await expect(page.locator('html')).toHaveAttribute('lang', 'es');
  // The removed message falls back to English, never blank or its key...
  await expect(page.getByRole('button', { name: en[removed.key], exact: true })).toBeVisible();
  await expect(page.getByText(es[removed.key], { exact: true })).toHaveCount(0);
  await expect(page.getByText(removed.key)).toHaveCount(0);
  // ...while the rest of the screen stays Spanish.
  await expect(page.getByRole('combobox', { name: app.language_title, exact: true })).toBeVisible();
});
