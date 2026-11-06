import { readFile, readdir, mkdir, rm, cp, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const directory = dirname(fileURLToPath(import.meta.url));
const repository = resolve(directory, '..');
export const languageNames = { en: 'English', nl: 'Nederlands', de: 'Deutsch', fr: 'Français', es: 'Español', ru: 'Русский' };
const source = 'https://github.com/sergeyfarin/rekenraam';

export function escapeHTML(value) {
  return String(value).replace(/[&<>"']/g, (character) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[character]);
}

function shape(value, path = '') {
  if (typeof value === 'string') {
    if (!value.trim()) throw new Error(`Empty translation: ${path}`);
    return [[path, [...value.matchAll(/\{([a-zA-Z_]+)\}/g)].map(match => match[1]).sort()]];
  }
  if (!value || typeof value !== 'object') throw new Error(`Invalid translation: ${path}`);
  return Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).flatMap(([key, item]) => shape(item, path ? `${path}.${key}` : key));
}

export async function readCatalogs(catalogDirectory = join(directory, 'locales')) {
  const settings = JSON.parse(await readFile(join(repository, 'frontend/project.inlang/settings.json'), 'utf8'));
  const files = (await readdir(catalogDirectory)).filter(file => file.endsWith('.json')).sort();
  const expected = settings.locales.map(locale => `${locale}.json`).sort();
  if (JSON.stringify(files) !== JSON.stringify(expected)) throw new Error('Website catalogs must match the app locales');
  const catalogs = Object.fromEntries(await Promise.all(settings.locales.map(async locale => {
    if (!languageNames[locale]) throw new Error(`Missing language name: ${locale}`);
    return [locale, JSON.parse(await readFile(join(catalogDirectory, `${locale}.json`), 'utf8'))];
  })));
  const baseShape = JSON.stringify(shape(catalogs[settings.baseLocale]));
  for (const [locale, catalog] of Object.entries(catalogs)) {
    if (JSON.stringify(shape(catalog)) !== baseShape) throw new Error(`Translation keys or placeholders differ: ${locale}`);
  }
  return catalogs;
}

export function siteConfig({ basePath = '', origin = 'https://rekenraam.com' } = {}) {
  basePath = basePath.replace(/\/$/, '');
  if (basePath && !/^\/(?:[A-Za-z0-9_-]+\/)*[A-Za-z0-9_-]+$/.test(basePath)) throw new Error('Invalid base path');
  const url = new URL(origin);
  if (url.protocol !== 'https:' || url.pathname !== '/' || url.search || url.hash || url.username || url.password) throw new Error('Origin must be a plain HTTPS origin');
  return { basePath, origin: url.origin };
}

const localePath = (locale, config) => `${config.basePath}/${locale === 'en' ? '' : `${locale}/`}`;

function languageLinks(locale, config) {
  return Object.entries(languageNames).map(([code, name]) => `<a href="${localePath(code, config)}" lang="${code}" hreflang="${code}"${code === locale ? ' aria-current="page"' : ''}>${name}</a>`).join('');
}

function renderPage(locale, t, config, notFound = false) {
  const e = escapeHTML;
  const asset = name => `${config.basePath}/assets/${name}`;
  const canonical = `${config.origin}${localePath(locale, config)}`;
  const links = languageLinks(locale, config);
  const heading = notFound ? t.notFoundTitle : t.metaTitle;
  const screenshot = (file, caption, alt, primary = false) => `<figure class="screenshot" data-gallery-slide data-caption="${e(caption)}">
    <a class="screenshot-link" href="${asset(file)}" aria-label="${e(`${t.openScreenshot}: ${caption}`)}"><img src="${asset(file)}" alt="${e(alt)}" width="1440" height="1050" ${primary ? 'fetchpriority="high"' : 'loading="lazy"'} decoding="async"></a>
    <figcaption><span>${e(caption)}</span><a href="${asset(file)}">${e(t.openScreenshot)} <span aria-hidden="true">↗</span></a></figcaption>
  </figure>`;

  const main = notFound ? `<main id="content" class="not-found container"><p class="eyebrow">Rekenraam</p><h1>${e(t.notFoundTitle)}</h1><p>${e(t.notFoundBody)}</p><a class="button" href="${localePath(locale, config)}">${e(t.notFoundLink)}</a></main>` : `<main id="content">
    <section class="hero container" aria-labelledby="intro-title">
      <div><p class="eyebrow">${e(t.heroLabel)}</p><h1 id="intro-title">${e(t.heroTitle)}</h1></div>
      <div class="actions"><a class="button" href="${source}#try-it">${e(t.heroSource)} <span aria-hidden="true">↗</span></a><a href="#status">${e(t.heroStatus)}</a></div>
    </section>
    <section id="screenshots" class="container screenshot-showcase" data-gallery aria-label="${e(t.navScreenshots)}">
      <div class="gallery-slides">${screenshot('reports-spending.png', t.reportCaption, t.reportAlt, true)}${screenshot('transactions.png', t.transactionsCaption, t.transactionsAlt)}${screenshot('investments.png', t.investmentsCaption, t.investmentsAlt)}</div>
      <div class="gallery-controls" data-gallery-controls role="group" aria-label="${e(t.navScreenshots)}" hidden>
        <button type="button" data-gallery-previous aria-label="${e(t.previousScreenshot)}"><span aria-hidden="true">←</span></button>
        ${[t.reportCaption, t.transactionsCaption, t.investmentsCaption].map((caption, index) => `<button type="button" data-gallery-select="${index}" aria-label="${e(caption)}" aria-pressed="${index === 0}">${index + 1}</button>`).join('')}
        <button type="button" data-gallery-next aria-label="${e(t.nextScreenshot)}"><span aria-hidden="true">→</span></button>
      </div>
      <p class="sr-only" data-gallery-announcement aria-live="polite" aria-atomic="true"></p>
      <p class="caption-note">${e(t.screenshotNote)}</p>
    </section>
    <section id="features" class="container section" aria-label="${e(t.navFeatures)}">
      <div class="features-grid">
        <article><h2>${e(t.storageTitle)}</h2><p>${e(t.storageBody)}</p></article>
        <article><h2>${e(t.ledgerTitle)}</h2><p>${e(t.ledgerBody)}</p></article>
        <article><h2>${e(t.investmentTitle)}</h2><p>${e(t.investmentBody)}</p></article>
        <article><h2>${e(t.budgetTitle)}</h2><p>${e(t.budgetBody)}</p></article>
      </div>
    </section>
    <section id="running" class="container section split-section" aria-labelledby="setup-title">
      <div class="section-heading"><h2 id="setup-title">${e(t.setupTitle)}</h2></div>
      <div class="section-body"><p>${e(t.setupBody)}</p><p>${e(t.setupMaintenance)}</p><div class="text-links"><a href="${source}#try-it">${e(t.setupInstall)} <span aria-hidden="true">↗</span></a><a href="${source}/blob/main/docs/deployment-security.md">${e(t.setupSecurity)} <span aria-hidden="true">↗</span></a></div></div>
    </section>
    <section id="status" class="status-section" aria-labelledby="status-title"><div class="container split-section">
      <div class="section-heading"><h2 id="status-title">${e(t.statusTitle)}</h2></div>
      <div class="section-body"><p>${e(t.statusBody)}</p><p>${e(t.statusLimits)}</p><p>${e(t.translationNote)}</p><div class="text-links"><a href="${source}/blob/main/docs/roadmap.md">${e(t.statusRoadmap)} <span aria-hidden="true">↗</span></a><a href="${source}/blob/main/docs/implemented.md">${e(t.statusFeatures)} <span aria-hidden="true">↗</span></a><a href="${source}/issues">${e(t.statusIssues)} <span aria-hidden="true">↗</span></a></div></div>
    </div></section>
    <aside class="container retirement section" aria-labelledby="retirement-title"><div><h2 id="retirement-title">${e(t.retirementTitle)}</h2><p>${e(t.retirementBody)}</p></div><a href="https://retirement.rekenraam.com">retirement.rekenraam.com <span aria-hidden="true">↗</span></a></aside>
  </main>`;

  return `<!doctype html>
<html lang="${locale}">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <meta name="color-scheme" content="light dark">
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src 'self'; style-src 'self'; script-src 'self'; base-uri 'none'; form-action 'none'; object-src 'none'">
  <title>${e(heading)}</title>
  <meta name="description" content="${e(t.metaDescription)}">
  ${notFound ? '<meta name="robots" content="noindex">' : `<link rel="canonical" href="${canonical}">
  ${Object.keys(languageNames).map(code => `<link rel="alternate" hreflang="${code}" href="${config.origin}${localePath(code, config)}">`).join('\n  ')}
  <link rel="alternate" hreflang="x-default" href="${config.origin}${localePath('en', config)}">`}
  <meta property="og:type" content="website">
  <meta property="og:site_name" content="Rekenraam">
  <meta property="og:title" content="${e(heading)}">
  <meta property="og:description" content="${e(t.metaDescription)}">
  <meta property="og:url" content="${notFound ? `${config.origin}${config.basePath}/404.html` : canonical}">
  <meta property="og:image" content="${config.origin}${asset('reports-spending.png')}">
  <meta property="og:image:alt" content="${e(t.reportAlt)}">
  <link rel="icon" href="${asset('favicon.svg')}" type="image/svg+xml">
  <link rel="stylesheet" href="${asset('site.css')}">
  <script src="${asset('theme.js')}" defer></script>
  ${notFound ? '' : `<script src="${asset('gallery.js')}" defer></script>`}
</head>
<body>
  <a class="skip-link" href="#content">${e(t.skipLink)}</a>
  <header class="header container">
    <a class="brand" href="${localePath(locale, config)}" aria-label="Rekenraam"><span class="abacus" aria-hidden="true"><span></span><span></span><span></span></span>rekenraam</a>
    <nav class="main-nav" aria-label="${e(t.navFeatures)}"><a href="${localePath(locale, config)}#features">${e(t.navFeatures)}</a><a href="${localePath(locale, config)}#screenshots">${e(t.navScreenshots)}</a><a href="${localePath(locale, config)}#running">${e(t.navSetup)}</a><a href="${source}">${e(t.navSource)} <span aria-hidden="true">↗</span></a></nav>
    <div class="header-settings"><button class="theme-toggle" type="button" data-theme-toggle data-label-light="${e(t.themeLight)}" data-label-dark="${e(t.themeDark)}" hidden></button><details class="language-picker"><summary>${languageNames[locale]} <span aria-hidden="true">⌄</span></summary><nav aria-label="${e(t.languageLabel)}">${links}</nav></details></div>
  </header>
  <div class="development"><div class="container"><strong><span class="status-dot" aria-hidden="true"></span>${e(t.developmentLabel)}</strong><span>${e(t.developmentSummary)}</span></div></div>
  ${main}
  <footer class="footer container"><div class="footer-top"><p>${e(t.footerMeaning)}</p><a href="${source}/blob/main/LICENSE">${e(t.footerLicense)}</a></div><nav class="footer-languages" aria-label="${e(t.languageLabel)}">${links}</nav><p class="privacy-note">${e(t.footerPrivacy)}</p></footer>
</body>
</html>
`;
}

export async function build({ outputDirectory = join(directory, 'build'), basePath = '', origin = 'https://rekenraam.com', catalogDirectory } = {}) {
  const config = siteConfig({ basePath, origin });
  const catalogs = await readCatalogs(catalogDirectory);
  // Output is never source; validate first so invalid catalogs leave the last build intact.
  await rm(outputDirectory, { recursive: true, force: true });
  await mkdir(join(outputDirectory, 'assets'), { recursive: true });
  await cp(join(directory, 'assets'), join(outputDirectory, 'assets'), {
    recursive: true,
    filter: path => !['capture.json', 'README.md'].includes(path.split('/').at(-1))
  });
  await cp(join(repository, 'docs/images/reports-spending.png'), join(outputDirectory, 'assets/reports-spending.png'));
  for (const [locale, catalog] of Object.entries(catalogs)) {
    const pageDirectory = locale === 'en' ? outputDirectory : join(outputDirectory, locale);
    await mkdir(pageDirectory, { recursive: true });
    await writeFile(join(pageDirectory, 'index.html'), renderPage(locale, catalog, config));
  }
  await writeFile(join(outputDirectory, '404.html'), renderPage('en', catalogs.en, config, true));
  await writeFile(join(outputDirectory, '.nojekyll'), '');
  await writeFile(join(outputDirectory, 'robots.txt'), `User-agent: *\nAllow: /\nSitemap: ${config.origin}${config.basePath}/sitemap.xml\n`);
  await writeFile(join(outputDirectory, 'sitemap.xml'), `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">${Object.keys(catalogs).map(locale => `<url><loc>${config.origin}${localePath(locale, config)}</loc></url>`).join('')}</urlset>\n`);
  return { outputDirectory, locales: Object.keys(catalogs), ...config };
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const result = await build({ basePath: process.env.SITE_BASE_PATH, origin: process.env.SITE_ORIGIN });
  console.log(`Built ${result.locales.length} languages in ${result.outputDirectory}`);
}
