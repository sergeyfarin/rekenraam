import test from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdtemp, readFile, cp, writeFile, rm, access, mkdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { build, readCatalogs, siteConfig } from './build.mjs';

const locales = ['en', 'es', 'fr', 'nl', 'de', 'ru'];
const fontHosts = ['fonts.googleapis.com', 'fonts.gstatic.com'];

test('screenshot manifest hashes describe the shipped images', async () => {
  const manifest = JSON.parse(await readFile(new URL('./assets/capture.json', import.meta.url), 'utf8'));
  assert.match(manifest.source_revision, /^[0-9a-f]{40}$/);
  assert.equal(manifest.source_clean, true);
  // Walking manifest.files alone cannot notice a removed entry. Name the
  // complete shipped set first; the exact sorted list also rejects duplicates
  // and unrecognised extra entries.
  const expectedFiles = ['transactions.png', 'investments.png', '../../docs/images/reports-spending.png'];
  assert.deepEqual(manifest.files.map((entry) => entry.file).sort(), [...expectedFiles].sort());
  for (const entry of manifest.files) {
    assert.match(entry.sha256, /^[0-9a-f]{64}$/, entry.file);
    const image = await readFile(new URL(entry.file, new URL('./assets/', import.meta.url)));
    assert.equal(createHash('sha256').update(image).digest('hex'), entry.sha256, entry.file);
  }
});

// Hostnames of every absolute (`https://…`) or protocol-relative (`//…`) URL in
// the text, in any quoting or CSS url(), resolved against the site origin.
function linkedHosts(text, origin = 'https://rekenraam.com') {
  return new Set([...text.matchAll(/(?:https?:)?\/\/[^\s"'<>()]+/g)]
    .filter(match => URL.canParse(match[0], origin))
    .map(match => new URL(match[0], origin).hostname));
}

test('all app locales have complete website catalogs and fully rendered pages', async t => {
  const directory = await mkdtemp(join(tmpdir(), 'rekenraam-site-test-'));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const result = await build({ outputDirectory: join(directory, 'site') });
  const catalogs = await readCatalogs();
  assert.deepEqual(result.locales, locales);
  for (const locale of locales) {
    const html = await readFile(join(result.outputDirectory, locale === 'en' ? '' : locale, 'index.html'), 'utf8');
    assert.ok(html.includes(`<html lang="${locale}">`));
    // Content must be in HTML, including when scripts are disabled or crawlers visit.
    for (const key of ['heroTitle', 'storageTitle', 'ledgerTitle', 'investmentTitle', 'budgetTitle', 'developmentLabel', 'retirementTitle']) {
      const escaped = catalogs[locale][key].replaceAll('&', '&amp;');
      assert.ok(html.includes(escaped), `${locale}: missing prerendered ${key}`);
    }
    assert.equal((html.match(/hreflang="x-default"/g) ?? []).length, 1);
    assert.equal((html.match(/rel="alternate" hreflang=/g) ?? []).length, locales.length + 1);
    assert.equal((html.match(/<h1 /g) ?? []).length, 1);
    // Resolve every internal page, stylesheet, script and screenshot to its published file.
    for (const match of html.matchAll(/(?:href|src)="(\/(?!\/)[^"#]*)(?:#[^"]*)?"/g)) {
      const relative = match[1].replace(/^\//, '');
      await access(join(result.outputDirectory, relative, relative.endsWith('/') || !relative ? 'index.html' : ''));
    }
    // No web-font CDNs: compare parsed hostnames exactly, not substrings of the HTML.
    const hosts = linkedHosts(html);
    for (const fontHost of fontHosts) assert.ok(!hosts.has(fontHost), `${locale}: loads ${fontHost}`);
    assert.ok(html.includes("script-src 'self'"));
  }
  assert.ok((await readFile(join(result.outputDirectory, '404.html'), 'utf8')).includes('content="noindex"'));
  // Compare parsed <loc> entries exactly rather than substring-matching the XML.
  const sitemap = await readFile(join(result.outputDirectory, 'sitemap.xml'), 'utf8');
  const sitemapLocs = [...sitemap.matchAll(/<loc>([^<]*)<\/loc>/g)].map(match => new URL(match[1]));
  assert.ok(sitemapLocs.some(url => url.origin === 'https://rekenraam.com' && url.pathname === '/ru/'));
});

test('a project Pages base path keeps language links and assets under that path', async t => {
  const directory = await mkdtemp(join(tmpdir(), 'rekenraam-site-base-'));
  t.after(() => rm(directory, { recursive: true, force: true }));
  await build({ outputDirectory: directory, basePath: '/rekenraam', origin: 'https://sergeyfarin.github.io' });
  const html = await readFile(join(directory, 'de/index.html'), 'utf8');
  assert.ok(html.includes('href="https://sergeyfarin.github.io/rekenraam/de/"'));
  for (const match of html.matchAll(/(?:href|src)="(\/[^"]*)"/g)) {
    assert.ok(match[1].startsWith('/rekenraam/'), `Unprefixed link: ${match[1]}`);
  }
});

test('catalog drift, empty translations and placeholders fail before replacing a build', async t => {
  const directory = await mkdtemp(join(tmpdir(), 'rekenraam-site-catalogs-'));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const catalogDirectory = join(directory, 'locales');
  await cp(new URL('./locales/', import.meta.url), catalogDirectory, { recursive: true });
  const file = join(catalogDirectory, 'nl.json');
  const catalog = JSON.parse(await readFile(file, 'utf8'));
  const outputDirectory = join(directory, 'site');
  await mkdir(outputDirectory);
  await writeFile(join(outputDirectory, 'last-good-build'), 'preserved');
  for (const invalid of [
    { ...catalog, heroTitle: '' },
    { ...catalog, heroTitle: '{unexpected}' },
    Object.fromEntries(Object.entries(catalog).filter(([key]) => key !== 'ledgerBody'))
  ]) {
    await writeFile(file, JSON.stringify(invalid));
    await assert.rejects(build({ outputDirectory, catalogDirectory }));
    assert.equal(await readFile(join(outputDirectory, 'last-good-build'), 'utf8'), 'preserved');
  }
  await writeFile(file, JSON.stringify(catalog));
  await rm(file);
  await assert.rejects(readCatalogs(catalogDirectory), /match the app locales/);
});

test('translated text is escaped rather than interpreted as markup', async t => {
  const directory = await mkdtemp(join(tmpdir(), 'rekenraam-site-escaping-'));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const catalogDirectory = join(directory, 'locales');
  await cp(new URL('./locales/', import.meta.url), catalogDirectory, { recursive: true });
  const file = join(catalogDirectory, 'en.json');
  const catalog = JSON.parse(await readFile(file, 'utf8'));
  catalog.heroTitle = '<script>alert("copy")</script>';
  await writeFile(file, JSON.stringify(catalog));
  const outputDirectory = join(directory, 'site');
  await build({ outputDirectory, catalogDirectory });
  const html = await readFile(join(outputDirectory, 'index.html'), 'utf8');
  assert.ok(html.includes('&lt;script&gt;alert(&quot;copy&quot;)&lt;/script&gt;'));
  assert.ok(!html.includes(catalog.heroTitle));
});

test('publishing paths reject malformed paths and origins', () => {
  for (const basePath of ['/../private', '//example.com', '/a?b', '/a\\b']) assert.throws(() => siteConfig({ basePath }));
  for (const origin of ['http://rekenraam.com', 'https://rekenraam.com/path', 'https://rekenraam.com/?q=x', 'https://user:pass@example.com']) assert.throws(() => siteConfig({ origin }));
});

test('font-host detection covers absolute, protocol-relative and CSS URLs', () => {
  for (const html of [
    '<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Inter">',
    "<link rel='stylesheet' href='//fonts.googleapis.com/css2?family=Inter'>",
    '<link rel=stylesheet href=//fonts.googleapis.com/css2>',
    '<style>@font-face { src: url(//fonts.gstatic.com/s/inter.woff2) }</style>',
    "<style>@import url('http://fonts.googleapis.com/css');</style>",
  ]) assert.ok(fontHosts.some(host => linkedHosts(html).has(host)), html);
  // Lookalike hosts and same-site paths are not font CDNs.
  for (const html of ['<a href="https://fonts.googleapis.com.example.net/">', '<a href="/fonts.googleapis.com/">', '<a href="https://example.com/?u=fonts.googleapis.com">'])
    assert.ok(!fontHosts.some(host => linkedHosts(html).has(host)), html);
});
