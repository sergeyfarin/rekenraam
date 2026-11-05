'use strict';

// Design fixtures only. This artifact has no API client or financial calculator.
const sections = {
  // Each section opens on its Overview. Ledger's Overview is the accounts list with ledger actions.
  Ledger: [['Overview', '/app/ledger'], ['Transactions', '/app/transactions'], ['Categories', '/app/categories'], ['Payees', '/app/payees'], ['Import', '/app/import'], ['Reconcile', '/app/reconcile']],
  Investments: [['Overview', '/app/investments'], ['Instruments', '/app/investments/instruments'], ['Returns', '/app/investments/returns'], ['Income', '/app/investments/income'], ['Gains', '/app/investments/gains'], ['Prices', '/app/pricing']],
  Planning: [['Overview', '/app/planning'], ['Budgets', '/app/budgets'], ['Recurring', '/app/recurring'], ['Forecast', '/app/forecast']],
  // Gains belongs to Investments; Reports lists it in its menu only, so a tab row never holds another section's page.
  Reports: [['Overview', '/app/reports'], ['Spending', '/app/reports/spending'], ['Cashflow', '/app/reports/cashflow'], ['Net worth', '/app/reports/net-worth'], ['Gains', '/app/investments/gains'], ['Export', '/app/reports/export']],
  // One list for the settings side list and the header's settings menu. Deleted transactions live in Ledger, not here.
  Settings: [['General', '/app/settings'], ['Preferences', '/app/settings/preferences'], ['Currencies and FX rates', '/app/settings/currencies'], ['Security', '/app/settings/security'], ['Data and backups', '/app/settings/data']]
};
// A brokerage is one account to the owner: its cash plus positions. Per-instrument holding
// accounts stay in the ledger model but are never listed as accounts (instrument pages show them).
const accounts = [
  { id: '1', name: 'Checking', institution: 'ING', kind: 'Checking', balance: '€4,210.55', pinned: true },
  { id: '6', name: 'Credit card', institution: 'ING', kind: 'Credit card', balance: '−€486.90' },
  { id: '5', name: 'Old checking', institution: 'ING', kind: 'Checking', balance: '€0.00', closed: true },
  { id: '2', name: 'Savings', institution: 'ASN', kind: 'Savings', balance: '€31,500.00', pinned: true },
  { id: '7', name: 'Revolut', institution: 'Revolut', kind: 'Checking', balance: '€1,126.40' },
  { id: '3', name: 'Brokerage', institution: 'DEGIRO', kind: 'Brokerage', balance: '€20,400.00', pinned: true, brokerage: true, note: 'Cash €12,000.00 · 1 position €8,400.00' }
];
for (const a of accounts) a.detail = `${a.kind} · ${a.institution} · EUR${a.closed ? ' · closed' : ''}`;
const institutions = [
  { name: 'ING', country: 'Netherlands', connection: 'File import (CSV profile)' },
  { name: 'ASN', country: 'Netherlands', connection: 'File import' },
  { name: 'Revolut', country: 'Lithuania', connection: 'File import' },
  { name: 'DEGIRO', country: 'Netherlands', connection: 'File import' }
];
const categories = [{ id: '10', name: 'Groceries', detail: 'Everyday › Groceries' }, { id: '11', name: 'Energy', detail: 'Housing › Energy' }];
const instruments = [{ id: '20', name: 'VWRL', detail: 'Vanguard FTSE All-World · held at DEGIRO' }];
const payees = [{ id: '30', name: 'Albert Heijn', count: 214, category: 'Groceries' }, { id: '31', name: 'Jumbo', count: 61, category: 'Groceries' }];
// Review is reached from Home's review line and the header's review button, not from a section.
const registry = [{ name: 'Home', path: '/app', section: 'Home' }, { name: 'Review', path: '/app/review', section: 'Home' }];
for (const [section, pages] of Object.entries(sections)) {
  for (const [name, path] of pages) {
    if (!registry.some(page => page.path === path)) registry.push({ name, path, section, title: name === 'Overview' ? `${section} overview` : name });
  }
}
const $ = selector => document.querySelector(selector);
const esc = value => String(value).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]);
const href = path => `#${path}`;
const link = (label, path, cls = '') => `<a class="${cls}" href="${esc(href(path))}">${label}</a>`;
const paths = {
  plus: '<path d="M12 5v14M5 12h14"/>',
  transfer: '<path d="M4 8h15l-4-4M20 16H5l4 4"/>',
  upload: '<path d="M12 15V4m-5 5 5-5 5 5M5 20h14"/>',
  list: '<path d="M9 6h11M9 12h11M9 18h11M4 6h.01M4 12h.01M4 18h.01"/>',
  chart: '<path d="M4 4v16h16M9 16v-5m5 5V7m5 9v-3"/>',
  bank: '<path d="M3 21h18M5 10h14M12 3 4 8h16zM6 10v8m4-8v8m4-8v8m4-8v8"/>',
  scale: '<path d="M12 3v18M7 21h10M4 7h16M7 7l-3 7a3 3 0 0 0 6 0zm10 0-3 7a3 3 0 0 0 6 0z"/>',
  trend: '<path d="m22 7-8.5 8.5-5-5L2 17M16 7h6v6"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
  eye: '<path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/>',
  eyeOff: '<path d="m3 3 18 18M10.6 5.1A10 10 0 0 1 12 5c6.4 0 10 7 10 7a17 17 0 0 1-3.2 4.2M6.6 6.6C3.7 8.4 2 12 2 12s3.6 7 10 7a9.7 9.7 0 0 0 5.4-1.6M9.9 9.9a3 3 0 0 0 4.2 4.2"/>',
  gear: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-2.8 1.2V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-2.9-1.2l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1A1.7 1.7 0 0 0 3.1 14H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.2-2.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1A1.7 1.7 0 0 0 10 3.1V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 2.9 1.2l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0 1.2 2.9h.1a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/>',
  sliders: '<path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6"/>',
  more: '<circle cx="5" cy="12" r="1.2"/><circle cx="12" cy="12" r="1.2"/><circle cx="19" cy="12" r="1.2"/>',
  chevron: '<path d="m6 9 6 6 6-6"/>',
  menu: '<path d="M4 6h16M4 12h16M4 18h16"/>',
  inbox: '<path d="M22 12h-6l-2 3h-4l-2-3H2"/><path d="M5.5 5h13L22 12v6a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2v-6z"/>',
  arrow: '<path d="M5 12h14m-6-6 6 6-6 6"/>'
};
const icon = (name, cls = '') => `<svg class="ic ${cls}" viewBox="0 0 24 24" aria-hidden="true" focusable="false">${paths[name]}</svg>`;
let hidden = false;
let theme = 'system';
try { hidden = localStorage.getItem('navigation-prototype-hidden') === 'true'; theme = localStorage.getItem('navigation-prototype-theme') || 'system'; } catch { /* File previews may deny storage. */ }
// Open design question the owner compares in the prototype bar: section menus or plain links.
const options = { sections: 'links' };
try { options.sections = JSON.parse(localStorage.getItem('navigation-prototype-options') || '{}').sections === 'menus' ? 'menus' : 'links'; } catch { /* Optional browser persistence. */ }
let scenario = 'everyday';
let route;
let searchScope = '';
let homeScope = '';
let searchResults = [];
let selected = 0;
let returnFocus;
let entryDirty = false;
let reconciliationPreview = false;
let reconciliationFinished = false;
let homeScroll = 0;
let noticeTimer;
// Home layout lives in memory here; production stores it per owner and book on the server (#438).
const defaultLayout = () => ({ headline: 'networth', home: ['spending', 'budgets', 'accounts', 'recurring', 'networth'], shortcuts: ['add', 'transfer', 'import', 'transactions', 'spending', 'checking'], exploreOpen: true });
let layout = defaultLayout();
let undoLayout = null;
const money = value => `<span class="money">${hidden ? '••••••' : esc(value)}</span>`;
// Home summarises: amounts of 100 or more drop their cents there (rounded half up for display only).
// Registers, reports, menus and editors always show the exact amount.
function rounded(value) {
  const parts = value.match(/^([+−]?)€([\d,]+)\.(\d{2})$/);
  if (!parts) return value;
  const amount = Number(`${parts[2].replace(/,/g, '')}.${parts[3]}`);
  return amount < 100 ? value : `${parts[1]}€${Math.round(amount).toLocaleString('en-US')}`;
}
const sm = value => money(rounded(value));
const accountPath = id => `/app/accounts/${id}/register`;
const currentAccount = () => accounts.find(account => account.id === (route.path.match(/^\/app\/accounts\/(\d+)\/register$/)?.[1] || route.params.get('account_id'))) || accounts[0];
const phone = () => matchMedia('(max-width: 600px)').matches;
function notify(message, undo = false) {
  clearTimeout(noticeTimer);
  $('#notice').innerHTML = `${esc(message)}${undo ? ' <button data-undo>Undo</button>' : ''}`;
  noticeTimer = setTimeout(() => { $('#notice').textContent = ''; }, undo ? 8000 : 6000);
}
function applyTheme() {
  document.documentElement.dataset.theme = theme === 'system' ? (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light') : theme;
}
applyTheme();
matchMedia('(prefers-color-scheme: dark)').addEventListener('change', applyTheme);

function readRoute() {
  const url = new URL(location.hash.slice(1) || '/app', 'https://prototype.invalid');
  // Same destinations as the proposed route migration; keep unrelated filters.
  if (url.pathname === '/app/accounts') url.pathname = '/app/ledger';
  if (url.pathname === '/app/reports' && url.searchParams.has('view')) {
    const view = url.searchParams.get('view') || 'spending';
    url.pathname = `/app/reports/${['spending', 'cashflow', 'net-worth'].includes(view) ? view : 'spending'}`;
    url.searchParams.delete('view');
  }
  if (url.pathname === '/app/investments' && url.searchParams.has('tab')) {
    const tab = url.searchParams.get('tab');
    if (['instruments', 'gains'].includes(tab)) url.pathname += `/${tab}`;
    if (tab === 'splits' || tab === 'suggestions') url.searchParams.set('view', tab);
    url.searchParams.delete('tab');
  }
  if (url.pathname === '/app/import/dividends') { url.pathname = '/app/import'; url.searchParams.set('view', 'dividends'); }
  if (['/app/settings/language', '/app/settings/appearance'].includes(url.pathname)) url.pathname = '/app/settings/preferences';
  if (url.pathname === '/app/settings/trash') { url.pathname = '/app/transactions'; url.searchParams.set('view', 'deleted'); }
  const canonical = url.pathname + url.search;
  if (location.hash !== href(canonical)) history.replaceState(null, '', href(canonical));
  return { path: url.pathname, params: url.searchParams, canonical };
}
function pageInfo() {
  const exact = registry.find(page => page.path === route.path);
  if (exact) return exact;
  if (/^\/app\/accounts\/\d+\/register$/.test(route.path)) return { name: currentAccount().name, section: 'Ledger', object: 'Accounts' };
  if (/^\/app\/categories\/\d+$/.test(route.path)) return { name: categories.find(c => c.id === route.path.split('/').at(-1))?.name || 'Category', section: 'Ledger', object: 'Categories' };
  if (/^\/app\/investments\/instruments\/\d+$/.test(route.path)) return { name: 'VWRL', section: 'Investments', object: 'Instruments' };
  return { name: 'Page not in this prototype', section: 'Home' };
}
function attentionHint(path) {
  if (scenario === 'failed' && ['/app/import', '/app/recurring'].includes(path)) return 'Check unavailable';
  if (scenario === 'empty') return '';
  return path === '/app/import' ? '1 batch' : path === '/app/recurring' ? '2 drafts' : path === '/app/investments/income' ? '3 to assign' : '';
}
// Tabs show only the number; the sentence stays for screen readers and in menus.
function countBadge(hint) {
  if (!hint) return '';
  const failed = hint === 'Check unavailable';
  return `<span class="count${failed ? ' failed' : ''}" aria-hidden="true">${failed ? '!' : hint.split(' ')[0]}</span><span class="sr-only">, ${hint}</span>`;
}
function pageLinks(pages, section = '', compact = false) {
  return pages.map(([name, path]) => {
    const isParent = (path === '/app/ledger' && route.path.startsWith('/app/accounts/')) || (['/app/categories', '/app/investments/instruments'].includes(path) && route.path.startsWith(`${path}/`));
    const home = registry.find(page => page.path === path)?.section;
    const hint = section && home !== section ? `in ${home}` : attentionHint(path);
    return `<a href="${href(path)}" ${route.path === path ? 'aria-current="page"' : isParent ? 'aria-current="true"' : ''}>${name}${compact ? countBadge(hint) : `<small class="muted">${hint}</small>`}</a>`;
  }).join('');
}
function pins() {
  return scenario === 'empty' ? '<p class="muted">No pinned accounts yet.</p>' : accounts.filter(a => a.pinned).map(account => link(`${account.name} ${money(account.balance)}`, accountPath(account.id))).join('');
}
function renderShell(info) {
  const menus = Object.entries(sections).filter(([name]) => name !== 'Settings').map(([name, pages]) => options.sections === 'links'
    // Links: a section opens its Overview and the tab row does the rest.
    ? `<div class="nav-item"><a class="nav-trigger" href="${href(pages[0][1])}" ${info.section === name ? 'aria-current="true"' : ''}>${name}</a></div>`
    : `<div class="nav-item"><button class="nav-trigger" data-disclosure="nav-${name}" aria-expanded="false" aria-controls="nav-${name}" ${info.section === name ? 'aria-current="true"' : ''}>${name}</button>
    <div id="nav-${name}" class="disclosure ${name === 'Ledger' ? 'ledger' : ''}" hidden><div>${pageLinks(pages, name)}</div>${name === 'Ledger' ? `<div><p class="menu-caption">Pinned accounts</p>${pins()}</div>` : ''}</div></div>`).join('');
  const isHome = route.path === '/app';
  // Only the section's own pages form its tab row; Settings keeps its side list instead.
  const tabs = info.section !== 'Settings' && sections[info.section] ? sections[info.section].filter(([, path]) => registry.find(page => page.path === path)?.section === info.section) : null;
  $('#shell').innerHTML = `<div class="shell-inner">
    <a class="brand" href="#/app" ${isHome ? 'aria-current="page"' : ''}>Home</a>
    <nav class="desktop-nav" aria-label="Sections">${menus}</nav>
    <div class="shell-actions">
      ${isHome ? '' : `<button data-search class="compact-search" aria-label="Search pages, accounts or commands">${icon('search')}<span class="search-copy">Search…</span><kbd class="search-copy">/</kbd></button>`}
      ${(() => { const n = reviewCount(); const failed = scenario === 'failed'; return `<a class="icon-button review-link" href="${href('/app/review')}" ${route.path === '/app/review' ? 'aria-current="page"' : ''} aria-label="Review${failed ? ', status unavailable' : n ? `, ${n} items need action` : ''}" title="Review">${icon('inbox')}${failed ? '<span class="count failed" aria-hidden="true">!</span>' : n ? `<span class="count" aria-hidden="true">${n}</span>` : ''}</a>`; })()}
      <button id="hide-amounts" class="icon-button" aria-label="Hide amounts" aria-pressed="${hidden}" title="Hide amounts">${icon(hidden ? 'eyeOff' : 'eye')}</button>
      ${isHome ? `<button class="icon-button" data-customize aria-label="Customize Home" title="Customize Home">${icon('sliders')}</button>` : ''}
      <div class="nav-item"><button class="nav-trigger icon-button" data-disclosure="settings-menu" aria-label="Settings and appearance" aria-expanded="false" aria-controls="settings-menu">${icon('gear')}</button>
        <div id="settings-menu" class="disclosure settings" hidden>${pageLinks(sections.Settings)}
          <label>Theme <select id="theme"><option value="light">Light</option><option value="dark">Dark</option><option value="system">System</option></select></label>
          <button data-sign-out>Sign out (sample)</button>
        </div></div>
      <button class="mobile-toggle icon-button" data-disclosure="mobile-pages" aria-label="All pages" aria-expanded="false" aria-controls="mobile-pages">${icon('menu')}</button>
    </div></div>
    <nav id="mobile-pages" class="mobile-pages" aria-label="All pages" hidden>${link('Home', '/app')}${Object.entries(sections).filter(([name]) => name !== 'Settings').map(([name, pages]) => `<details><summary>${name}</summary>${pageLinks(pages, name)}${name === 'Ledger' ? `<p class="menu-caption">Pinned accounts</p>${pins()}` : ''}</details>`).join('')}<details><summary>Settings</summary>${pageLinks(sections.Settings)}</details></nav>
    ${tabs ? `<nav class="section-tabs" aria-label="${info.section} pages"><div class="tabs-inner">${pageLinks(tabs, '', true)}</div></nav>` : ''}`;
  $('#theme').value = theme;
}
function heading(info, scope = '', add = false, extra = '') {
  const breadcrumb = info.object ? `<nav class="breadcrumbs" aria-label="Breadcrumb">${link(info.object === 'Accounts' ? 'Ledger' : info.object, info.object === 'Accounts' ? '/app/ledger' : info.object === 'Categories' ? '/app/categories' : '/app/investments/instruments')} › ${esc(info.name)}</nav>` : '';
  return `${breadcrumb}<div class="page-heading"><div><h1>${info.object ? `<button class="object-trigger" data-scope="${info.object}">${esc(info.name)}${icon('chevron', 'nav-chev')}</button>` : esc(info.title || info.name)}</h1>${scope ? `<p class="scope">${scope}</p>` : ''}</div><div class="page-actions">${extra}${add ? `<button class="primary" data-add>${icon('plus')}Add transaction</button>` : ''}</div></div>`;
}

// ---- Home -----------------------------------------------------------------

// The label is the headline's scope; the menu says what it covers as a count, never an account list.
const headlines = {
  networth: { label: 'Net worth', value: '€56,750.05', change: '+€2,140.00', scope: 'All 5 accounts' },
  cash: { label: 'Cash', value: '€36,836.95', change: '+€2,140.00', scope: '3 cash accounts' },
  checking: { label: 'Checking', value: '€4,210.55', change: '−€1,320.10', scope: '1 account' },
  liquid: { label: 'Everyday accounts', value: '€6,912.40', change: '−€1,020.10', scope: '2 accounts you chose' }
};
function spark(values, label) {
  const lo = Math.min(...values), hi = Math.max(...values);
  const pts = values.map((v, i) => `${Math.round(i * 600 / (values.length - 1))} ${Math.round(135 - (v - lo) / (hi - lo || 1) * 115)}`);
  const line = `M${pts.join(' L')}`;
  return `<svg class="spark" viewBox="0 0 600 150" preserveAspectRatio="none" role="img" aria-label="${esc(label)}"><path class="spark-area" d="${line} L600 150 L0 150 Z"/><path class="spark-line" d="${line}"/></svg>`;
}
// The missing-valuation sample has several unpriced instruments: sentences name one, count several.
const unpriced = ['VWRL', 'IWDA', 'ASML'];
const unpricedText = () => unpriced.length === 1 ? `${unpriced[0]} has no price` : `${unpriced.length} instruments have no price`;
const missingPricesPath = '/app/pricing?coverage=missing';
// Out-of-date prices still value a position (at its last price) and never block a total;
// they are disclosed as one count with the oldest date and one grouped review item.
const stale = { count: 180, of: 240, oldest: '3 Sep', path: '/app/pricing?coverage=stale' };
const staleText = () => `${stale.count} of ${stale.of} prices older than 7 days, oldest ${stale.oldest}`;
// Any value net worth depends on can be missing (blocks the total) or out of date (last value used,
// counted): market prices, FX rates, and values the owner enters, such as a house or a pension.
// The manual sample book has a house with no value yet and a pension last updated 30 Jun (expected quarterly).
const blocked = () => scenario === 'missing' || scenario === 'manual';
function widgetDefs() {
  const missing = scenario === 'missing';
  const old = scenario === 'stale';
  const empty = scenario === 'empty';
  const bars = rows => `<div class="w-bars">${rows.map(([name, width, value, path]) => `<div class="bar-row">${link(name, path)}<span class="bar" aria-hidden="true"><span style="width:${width}%"></span></span>${sm(value)}</div>`).join('')}</div>`;
  const list = rows => `<ul class="rows">${rows.map(([name, sub, right]) => `<li><span>${name}<br><small class="muted">${sub}</small></span><span>${right}</span></li>`).join('')}</ul>`;
  const defs = [
    { id: 'spending', cat: 'Everyday', title: 'Spending this month', open: ['Open spending report', '/app/reports/spending?from=2026-10-01&to=2026-10-29'], fact: sm('€3,720.40'), sub: `of ${sm('€5,300.00')} budgeted · 1–29 Oct · expense categories · EUR`,
      body: bars([['Housing', 100, '€1,450.00', '/app/categories/11'], ['Groceries', 42, '€612.30', '/app/categories/10'], ['Transport', 20, '€284.10', '/app/transactions?category_id=12'], ['Other', 95, '€1,373.00', '/app/reports/spending']]) },
    { id: 'budgets', cat: 'Everyday', title: 'Budgets · October', open: ['Open budgets', '/app/budgets?month=2026-10'], fact: `${sm('€1,579.60')} left`, sub: 'On pace: 70% spent, 87% of the month gone',
      body: `<ul class="rows">${[['Groceries', '€612.30', '€700.00', ''], ['Dining out', '€231.00', '€200.00', hidden ? 'over' : '€31 over'], ['Transport', '€284.10', '€350.00', '']].map(([name, used, of, note]) => `<li><span>${name}${note ? ` <span class="warning">· ${note}</span>` : ''}</span><span>${sm(used)} of ${sm(of)}</span></li>`).join('')}</ul>` },
    { id: 'cashflow', cat: 'Everyday', title: 'Cashflow · October', open: ['Open cashflow report', '/app/reports/cashflow'], fact: sm('+€2,140.00'), sub: 'Liquid accounts · posted actuals · EUR',
      body: list([['Money in', 'salary, refunds', sm('€8,960.00')], ['Money out', 'spending, transfers out', sm('€6,820.00')]]) },
    // Shows the owner's pins, the same list as the Ledger menu, rather than a second "largest balances" ranking.
    { id: 'accounts', cat: 'Everyday', title: 'Accounts', open: ['All accounts', '/app/ledger'], fact: '5 open accounts', sub: 'Pinned accounts · balances at 29 Oct',
      body: `<ul class="rows">${accounts.filter(a => a.pinned).map(a => `<li>${link(a.name, accountPath(a.id))}<span>${sm(a.balance)}</span></li>`).join('')}</ul><p class="scope">+ 2 more open accounts · 1 closed</p>` },
    { id: 'networth', cat: 'Wealth & investments', title: 'Net worth · 12 months', wide: true, open: ['Open net worth report', '/app/reports/net-worth'], fact: blocked() ? 'Unavailable' : sm('+€7,850.05'), sub: blocked() ? `${scenario === 'manual' ? 'House has no value yet' : `${unpricedText()} for 29 Oct`}; the total is not shown partially` : 'Nov 2025 – Oct 2026 · month-end values · EUR',
      body: blocked() ? '' : spark([48900, 49600, 49100, 50400, 51300, 50900, 52200, 53100, 52800, 53970, 54610, 56750], 'Net worth rose from about 48.9 to 56.8 thousand euro over 12 months') },
    { id: 'holdings', cat: 'Wealth & investments', title: 'Holdings', open: ['Open investments', '/app/investments'], fact: missing ? '3 positions' : old ? `${stale.of} positions` : '1 position', sub: missing ? `Known value only · ${unpriced.length} positions unpriced` : old ? `Largest by value · ${stale.count} at prices older than 7 days` : 'Largest by value · 29 Oct',
      body: list(missing ? unpriced.map(code => [link(code, '/app/investments/instruments/20'), 'DEGIRO brokerage', 'No price']) : [[link('VWRL', '/app/investments/instruments/20'), 'Vanguard FTSE All-World · DEGIRO brokerage', sm('€8,400.00')]]) },
    { id: 'returns', cat: 'Wealth & investments', title: 'Returns · year to date', open: ['Open returns', '/app/investments/returns'], fact: missing ? 'Unavailable' : '+7.4%', sub: 'Time-weighted · 1 Jan – 29 Oct · all investment accounts', body: '' },
    { id: 'allocation', cat: 'Wealth & investments', title: 'Allocation', open: ['Open returns', '/app/investments/returns'], fact: missing ? 'Unavailable' : 'Equity 41%', sub: missing ? `${unpriced.length} positions unpriced` : 'Investment accounts · market value 29 Oct',
      body: missing ? '' : '<div class="stack" role="img" aria-label="Equity 41 percent, cash 59 percent"><span style="width:41%"></span><span style="width:59%"></span></div><p class="scope">Equity 41% · Cash 59%</p>' },
    { id: 'income', cat: 'Wealth & investments', title: 'Dividend income', badge: ['Review', 'warn'], open: ['Open income', '/app/investments/income'], fact: '3 to assign', sub: `${sm('€412.80')} confirmed this year`,
      body: list([['VWRL', '25 Sep · unassigned cash', sm('€96.40')]]) },
    { id: 'recurring', cat: 'Planning', title: 'Recurring', badge: ['2 drafts', 'warn'], open: ['Review drafts', '/app/recurring?tab=due'], fact: 'Next: Rent on 1 Nov', sub: '9 schedules active · 2 drafts awaiting review',
      body: list([['Rent', '1 Nov · Checking', sm('−€1,450.00')], ['Energy', '3 Nov · Checking', sm('−€142.00')], ['Salary', '25 Nov · Checking', sm('+€3,200.00')]]) },
    { id: 'forecast', cat: 'Planning', title: 'Forecast · liquid cash', badge: ['Estimate', 'info'], open: ['Open forecast', '/app/forecast'], fact: `${sm('€33,900')} on 31 Dec`, sub: 'Includes recurring schedules and learned spending',
      body: spark([36840, 35900, 36400, 35600, 35100, 36200, 35800, 34600, 33900], 'Projected liquid cash eases to about 33.9 thousand euro by 31 December') },
    { id: 'imports', cat: 'Ledger & data', title: 'Imports', badge: ['Review', 'warn'], open: ['Open import', '/app/import'], fact: '1 batch in review', sub: 'Last import 2 days ago',
      body: list([[link('ING CSV · October', '/app/import?batch_id=october'), '42 rows · 3 need a category', '']]) },
    { id: 'reconcile', cat: 'Ledger & data', title: 'Reconciliation', open: ['Open reconcile', '/app/reconcile'], fact: '2 of 5 accounts', sub: 'reconciled in the last 60 days',
      body: list([['Checking', 'last reconciled', '12 Sep'], ['Savings', 'last reconciled', '30 Sep']]) },
    { id: 'prices', cat: 'Ledger & data', title: 'Prices', badge: missing ? ['Missing', 'warn'] : old ? ['Out of date', 'warn'] : ['Up to date', 'info'], open: ['Open prices', '/app/pricing'], fact: missing ? `${unpriced.length} missing` : old ? `${stale.count} out of date` : '1 price from 29 Oct', sub: 'FX rates are in Settings › Currencies and FX rates',
      body: missing ? `<p>${unpriced.join(', ')} · ${link('Add missing prices', missingPricesPath)}</p>` : old ? `<p>Oldest ${stale.oldest} · ${link('Review out-of-date prices', stale.path)}</p>` : '' },
    { id: 'backups', cat: 'Ledger & data', title: 'Backups', badge: ['Set up', 'setup'], open: ['Open backups and data', '/app/settings/data'], fact: 'Not set up', sub: 'Protect your books with automatic backups',
      body: `<p>${link('Set up backups', '/app/settings/data', 'button')}</p>` }
  ];
  if (empty) for (const d of defs) if (d.id !== 'backups') Object.assign(d, { fact: 'No data yet', sub: 'Appears once you add an account and transactions', body: '', badge: null });
  return defs;
}
const shortcutPool = {
  add: { label: 'Add transaction', icon: 'plus', action: 'add' },
  transfer: { label: 'Transfer', icon: 'transfer', action: 'transfer' },
  import: { label: 'Import file', icon: 'upload', path: '/app/import' },
  transactions: { label: 'Transactions', icon: 'list', path: '/app/transactions' },
  spending: { label: 'Spending by category', icon: 'chart', path: '/app/reports/spending' },
  checking: { label: 'Checking', icon: 'bank', path: accountPath('1') },
  reconcile: { label: 'Reconcile Checking', icon: 'scale', path: '/app/reconcile?account_id=1' },
  savings: { label: 'Savings', icon: 'bank', path: accountPath('2') },
  returns: { label: 'Returns year to date', icon: 'trend', path: '/app/investments/returns' }
};
function reviewEntries() {
  if (scenario === 'empty' || scenario === 'failed') return [];
  return [
    ['warn', link('2 recurring drafts awaiting review', '/app/recurring?tab=due')],
    ['warn', link('1 import batch to review · 3 rows need a category', '/app/import?batch_id=october')],
    ['info', link('Note: 3 dividends not yet assigned', '/app/investments/income')],
    // Many missing prices are one review item, not one per instrument; the Prices page lists them.
    ...(scenario === 'stale' ? [['warn', `${link(`${stale.count} prices out of date`, stale.path)}<small class="muted"> · oldest ${stale.oldest} · positions use their last price</small>`]] : []),
    ...(scenario === 'manual' ? [['warn', 'House has no value yet<small class="muted"> · net worth waits for it · add one on the account</small>'],
      ['warn', 'Pension last updated 30 Jun<small class="muted"> · you update it every 3 months · its last value is used</small>']] : []),
    ...(scenario === 'missing' ? [['warn', `${link(`${unpricedText()} for 29 Oct`, missingPricesPath)}<small class="muted"> · ${unpriced.join(', ')}</small>`]] : [])
  ];
}
// One page for everything that needs attention and the state of background work.
function renderReview(info) {
  const entries = reviewEntries();
  const rows = tone => entries.filter(([t]) => t === tone).map(([t, text]) => `<li><span class="dot ${t}"></span><span class="t">${text}</span></li>`).join('');
  const action = rows('warn'), notes = rows('info');
  const status = [
    ['Backups', 'Last backup 29 Oct 03:00 · kept 14', '/app/settings/data'],
    ['Ledger self-check', 'Passed 28 Oct', '/app/settings/data'],
    ['Prices', scenario === 'stale' ? `<span class="warning">${staleText()}</span>` : scenario === 'missing' ? `<span class="warning">${unpricedText()}</span>` : 'Refreshed 29 Oct 06:00', '/app/pricing'],
    ['FX rates', 'Refreshed 29 Oct 06:00', '/app/settings/currencies'],
    ...(scenario === 'manual' ? [['Values you enter', '<span class="warning">House has no value · Pension out of date</span>', '/app/ledger']] : []),
    ['Trading 212', 'Last download 29 Oct 07:10 · 4 rows', '/app/import?view=connections'],
    ['File imports', '1 batch staged, not committed', '/app/import']
  ];
  const failed = scenario === 'failed';
  return `${heading({ ...info, title: 'Review' }, 'Everything that needs your attention, and the state of background work · checked 29 Oct 08:00')}
    ${failed ? '<section class="card review-card"><h2>Needs action</h2><p class="warning">Review status could not be checked.</p><button data-retry>Retry check</button></section>'
      : `<section class="card review-card"><h2>Needs action</h2>${action ? `<ul class="rows review-rows">${action}</ul>` : '<p class="ok">All clear, nothing to review.</p>'}</section>`}
    ${notes ? `<section class="card review-card"><h2>Notes</h2><ul class="rows review-rows">${notes}</ul></section>` : ''}
    <section class="card review-card"><h2>Status</h2>${scenario === 'empty' ? '<p class="muted">Nothing runs yet. Set up backups and prices when you add accounts.</p>' : `<div class="table-region" role="region" aria-label="Status of background work" tabindex="0"><table><tbody>${status.map(([name, value, path]) => `<tr><th scope="row">${link(name, path)}</th><td>${value}</td></tr>`).join('')}</tbody></table></div>`}</section>`;
}
function reviewCount() { return reviewEntries().filter(([tone]) => tone === 'warn').length; }
function shortcutTile(id, index) {
  const s = shortcutPool[id];
  const inner = `<span class="tile-ic ${id === 'add' ? 'primary' : ''}">${icon(s.icon)}</span><span class="tile-l">${esc(s.label)}</span>`;
  const main = s.path ? link(inner, s.path, 'tile-a') : `<button class="tile-a" ${s.action === 'add' ? 'data-add' : 'data-transfer'}>${inner}</button>`;
  const last = index === layout.shortcuts.length - 1;
  return `<li class="tile ${index >= 3 ? 'extra' : ''}">${main}
    <button class="tile-more" data-disclosure="sc-${id}" aria-expanded="false" aria-controls="sc-${id}" aria-label="Options for shortcut ${esc(s.label)}">${icon('more')}</button>
    <div id="sc-${id}" class="disclosure tile-menu" hidden>
      <button data-sc="left" data-id="${id}" ${index === 0 ? 'disabled' : ''}>Move left</button>
      <button data-sc="right" data-id="${id}" ${last ? 'disabled' : ''}>Move right</button>
      <button data-sc="remove" data-id="${id}" ${id === 'add' ? 'disabled' : ''}>${id === 'add' ? 'Always available' : 'Remove shortcut'}</button>
    </div></li>`;
}
function renderShortcuts() {
  const left = Object.keys(shortcutPool).filter(id => !layout.shortcuts.includes(id));
  const extra = layout.shortcuts.slice(3);
  return `<ul class="shortcuts" aria-label="Shortcuts">${layout.shortcuts.map(shortcutTile).join('')}
    <li class="tile ${left.length ? '' : 'none-left'} add-tile"><button class="tile-a" data-disclosure="sc-new" aria-expanded="false" aria-controls="sc-new"><span class="tile-ic add">${icon('plus')}</span><span class="tile-l">Add shortcut</span></button>
      <div id="sc-new" class="disclosure tile-menu" hidden><p class="menu-caption">Actions, pages and saved views</p>${left.map(id => `<button data-sc-add="${id}">${esc(shortcutPool[id].label)}</button>`).join('') || '<p class="muted">Every shortcut is on Home.</p>'}</div></li>
    <li class="tile more-tile"><button class="tile-a" data-disclosure="sc-more" aria-expanded="false" aria-controls="sc-more"><span class="tile-ic">${icon('more')}</span><span class="tile-l">More</span></button>
      <div id="sc-more" class="disclosure tile-menu" hidden>${extra.map(id => shortcutPool[id].path ? link(esc(shortcutPool[id].label), shortcutPool[id].path) : `<button ${shortcutPool[id].action === 'add' ? 'data-add' : 'data-transfer'}>${esc(shortcutPool[id].label)}</button>`).join('')}<button data-customize>Customize shortcuts…</button></div></li>
  </ul>`;
}
function widgetCard(d, zone) {
  const onHome = zone === 'home';
  const i = layout.home.indexOf(d.id);
  const menu = onHome ? `<button data-w="up" data-id="${d.id}" ${i === 0 ? 'disabled' : ''}>Move up</button><button data-w="down" data-id="${d.id}" ${i === layout.home.length - 1 ? 'disabled' : ''}>Move down</button><button data-w="settings" data-id="${d.id}">Settings…</button><button data-w="remove" data-id="${d.id}">Remove from My Home</button>`
    : `<button data-w="add" data-id="${d.id}">Add to My Home</button><button data-w="top" data-id="${d.id}">Add to top of My Home</button>`;
  return `<article class="card widget ${d.wide && onHome ? 'wide' : ''} ${onHome ? '' : 'explore'}" id="${zone}-${d.id}" aria-labelledby="${zone}-${d.id}-title">
    <div class="w-head"><h3 id="${zone}-${d.id}-title">${esc(d.title)}</h3>${d.badge ? `<span class="badge ${d.badge[1]}">${esc(d.badge[0])}</span>` : ''}
      <button class="w-more" data-disclosure="${zone}-${d.id}-menu" aria-expanded="false" aria-controls="${zone}-${d.id}-menu" aria-label="Options for ${esc(d.title)}">${icon('more')}</button>
      <div id="${zone}-${d.id}-menu" class="disclosure w-menu" hidden>${menu}${link(`${esc(d.open[0])}`, d.open[1])}</div></div>
    <p class="fact">${d.fact}</p><p class="scope">${d.sub}</p>${d.body}
    <div class="w-foot">${onHome ? '' : `<button class="add-home" data-w="add" data-id="${d.id}" aria-label="Add ${esc(d.title)} to My Home">${icon('plus')}Add to My Home</button>`}${link(`${esc(d.open[0])} →`, d.open[1], 'w-open')}</div>
  </article>`;
}
function renderHome() {
  const empty = scenario === 'empty';
  // Net worth with a missing price is not shown at all, not even partially: Home shows cash, which needs no price.
  const fallback = blocked() && layout.headline === 'networth';
  const h = fallback ? headlines.cash : headlines[layout.headline] || headlines.networth;
  const line1 = empty ? 'Your book is ready for its first account'
    : `${h.label} <strong>${sm(h.value)}</strong> <span class="muted">·</span> <span class="${hidden ? '' : h.change.startsWith('−') ? 'loss' : 'gain'}">${sm(h.change)}</span> <span class="muted">in Oct</span>`;
  const switcher = empty ? '' : `<div class="nav-item"><button class="bl-switch" data-disclosure="headline-menu" aria-expanded="false" aria-controls="headline-menu" aria-label="Change headline">${icon('chevron')}</button>
    <div id="headline-menu" class="disclosure center" hidden>${Object.entries(headlines).map(([id, v]) => `<button data-headline="${id}" aria-pressed="${layout.headline === id}">${v.label}<small class="muted">${v.scope}</small></button>`).join('')}<button data-customize>Customize Home…</button></div></div>`;
  // One status line. A valuation problem leads (it explains the headline) and the rest of the review is a count;
  // otherwise the review is named. The header's Review button carries the total. One item is named, several of
  // one kind are counted by kind, mixed kinds are counted as values. Missing outranks out of date.
  const lead = scenario === 'missing' ? { text: fallback ? `Net worth needs ${unpriced.length} prices` : `${unpriced.length} prices missing`, path: missingPricesPath }
    : scenario === 'stale' ? { text: `${stale.count} prices out of date`, path: stale.path }
    : scenario === 'manual' ? { text: fallback ? 'Net worth needs a value for House' : 'House has no value yet', path: '/app/ledger' } : null;
  const more = reviewCount() - 1;
  const line2 = empty ? '<p class="bl muted">No accounts or transactions yet</p>'
    : `<p class="bl status"><span class="dot warn" aria-hidden="true"></span>${lead ? `${link(lead.text, lead.path, 'warning')}<span class="muted" aria-hidden="true">·</span>` : ''}<a id="home-review" class="bl-review" href="${href('/app/review')}"${lead && scenario !== 'failed' ? ` aria-label="${more} more to review"` : ''}>${scenario === 'failed' ? '<span class="warning">Review status unavailable</span>' : lead ? `<span>${more} more</span>` : '<span><strong>2 drafts</strong> and <strong>1 import</strong> to review</span>'}${icon('arrow')}</a></p>`;
  const defs = widgetDefs();
  const byId = Object.fromEntries(defs.map(d => [d.id, d]));
  const rest = defs.filter(d => !layout.home.includes(d.id));
  const groups = ['Everyday', 'Wealth & investments', 'Planning', 'Ledger & data'].map(cat => [cat, rest.filter(d => d.cat === cat)]);
  return `<section class="hero" aria-label="Briefing">
      <div class="brief"><div class="bl headline"><h1>${line1}</h1>${switcher}</div>${line2}
</div>
      <div class="cmd" role="search">
        <div class="cmd-box">${icon('search')}${homeScope ? `<button id="home-scope-chip" class="chip" aria-label="Remove ${homeScope} scope">${homeScope} ×</button>` : ''}
          <label class="sr-only" for="home-cmd">Search pages, accounts, payees and commands</label>
          <input id="home-cmd" type="search" role="combobox" aria-expanded="false" aria-controls="home-cmd-list" aria-autocomplete="list" autocomplete="off" placeholder="Search or type a payee">
          <kbd>/</kbd></div>
        <div id="home-cmd-panel" class="cmd-drop" hidden><div id="home-cmd-chips" class="chips"></div><div id="home-cmd-list" role="listbox" aria-label="Search results"></div></div>
      </div>
      ${renderShortcuts()}
    </section>
    <section class="home-section my-home" aria-label="My Home">
      ${empty ? `<div class="card onboarding"><h3>Start with an account</h3><p>Add an account before entering or importing transactions. The widgets below fill in as data arrives.</p>${link('Set up accounts', '/app/ledger', 'button primary')}</div>` : ''}
      ${layout.home.length ? `<div class="grid">${layout.home.map(id => widgetCard(byId[id], 'home')).join('')}</div>` : '<p class="muted">My Home is empty. Add widgets from More to explore below.</p>'}
    </section>
    <section class="home-section" id="explore" aria-labelledby="explore-title"><div class="section-head"><h2 id="explore-title">More to explore</h2><span class="muted">Add any of the widgets below to the home page</span>
      <button id="explore-toggle" aria-expanded="${layout.exploreOpen}" aria-controls="explore-body">${layout.exploreOpen ? 'Collapse' : 'Show all'}</button></div>
      <div id="explore-body" ${layout.exploreOpen ? '' : 'hidden'}>${groups.map(([cat, items]) => `<h3 class="cat">${cat} <span class="muted">${items.length}</span></h3>${items.length ? `<div class="grid">${items.map(d => widgetCard(d, 'explore')).join('')}</div>` : '<p class="muted">Everything in this group is already on your Home.</p>'}`).join('')}</div>
    </section>`;
}
function changeLayout(patch, message, undoable = true) {
  undoLayout = undoable ? structuredClone(layout) : null;
  layout = { ...layout, ...patch };
  render();
  if (message) notify(message, undoable);
  if ($('#customize-dialog').open) renderCustomize();
}
const swap = (list, i, j) => { const copy = list.slice(); [copy[i], copy[j]] = [copy[j], copy[i]]; return copy; };
function renderCustomize() {
  const defs = Object.fromEntries(widgetDefs().map(d => [d.id, d]));
  const row = (list, id, label, removable) => {
    const i = layout[list].indexOf(id);
    return `<li><span>${esc(label)}</span><span class="actions-inline"><button data-cz="up" data-list="${list}" data-id="${id}" aria-label="Move ${esc(label)} ${list === 'home' ? 'up' : 'earlier'}" ${i === 0 ? 'disabled' : ''}>↑</button><button data-cz="down" data-list="${list}" data-id="${id}" aria-label="Move ${esc(label)} ${list === 'home' ? 'down' : 'later'}" ${i === layout[list].length - 1 ? 'disabled' : ''}>↓</button><button data-cz="remove" data-list="${list}" data-id="${id}" aria-label="Remove ${esc(label)}" ${removable ? '' : 'disabled'}>×</button></span></li>`;
  };
  $('#customize-body').innerHTML = `
    <fieldset><legend>Headline</legend>${Object.entries(headlines).map(([id, v]) => `<label class="radio"><input type="radio" name="cz-headline" value="${id}" ${layout.headline === id ? 'checked' : ''}> ${v.label}</label>`).join('')}</fieldset>
    <h3>Shortcuts</h3><p class="scope">Add transaction can be moved but not removed.</p><ul class="cz-list">${layout.shortcuts.map(id => row('shortcuts', id, shortcutPool[id].label, id !== 'add')).join('')}</ul>
    <h3>My Home order</h3><p class="scope">Top to bottom. Anything removed goes back to More to explore.</p><ul class="cz-list">${layout.home.map(id => row('home', id, defs[id].title, true)).join('')}</ul>
    <label class="radio"><input type="checkbox" id="cz-explore" ${layout.exploreOpen ? 'checked' : ''}> Show More to explore expanded</label>
    <div class="actions"><button data-cz="reset">Reset Home</button><button class="primary" data-close="customize-dialog">Done</button></div>`;
}

// ---- Other pages ----------------------------------------------------------

function filters() {
  return `<form id="filters" class="filters"><label>Month<input name="month" type="month" value="${esc(route.params.get('month') || '2026-03')}"></label><label>Find in entries<input name="q" value="${esc(route.params.get('q') || '')}"></label><button>Apply filters</button></form>`;
}
function registerRows(account) {
  if (account.id !== '1') return `<tr><td>01 Mar</td><td>${account.id === '2' ? 'Opening savings' : 'Sample balance'}</td><td>Opening balance</td><td class="numeric">${money(account.balance)}</td></tr>`;
  return `<tr><td>27 Mar</td><td>${link('Albert Heijn', '/app/transactions?payee_id=30')}</td><td>${link('Groceries', '/app/categories/10')}</td><td class="numeric">${money('−€42.15')}</td></tr><tr><td>25 Mar</td><td>Salary</td><td>Income</td><td class="numeric">${money('+€3,200.00')}</td></tr>`;
}
const segmented = (label, views, current) => `<nav class="segmented" aria-label="${label}">${views.map(([name, path, key]) => link(name, path, key === current ? 'on" aria-current="true' : '')).join('')}</nav>`;
function renderRegister(info) {
  const account = currentAccount();
  const extra = account.brokerage ? '<button data-invest>New transaction</button>' : '';
  const positions = account.brokerage ? `<section class="card"><h2>Positions</h2><ul class="rows"><li><span>${link('VWRL', '/app/investments/instruments/20')}<br><small class="muted">Vanguard FTSE All-World · 96 units</small></span>${scenario === 'missing' ? 'No price' : money('€8,400.00')}</li></ul><p class="scope">Each position is an instrument held in this account, not a separate account.</p></section>` : '';
  return `${heading(info, `${esc(account.detail)} · ${money(account.balance)} at 29 Oct${account.note ? ` · ${hidden ? 'cash and positions' : esc(account.note)}` : ''}`, true, extra)}${positions}${filters()}
    ${route.params.has('dropped') ? `<p role="status" class="scope">Removed incompatible filter: ${esc(route.params.get('dropped'))}.</p>` : ''}
    <section class="card"><h2>${account.brokerage ? 'Cash entries' : 'Entries'}</h2><div class="table-region" role="region" aria-label="Account entries" tabindex="0"><table><thead><tr><th>Date</th><th>Payee</th><th>Category</th><th class="numeric">Amount</th></tr></thead><tbody>${registerRows(account)}</tbody></table></div>
    <p class="scope">Illustrative rows; the prototype does not recalculate account balances.</p></section>
    <div class="actions">${link('Reconcile this account', `/app/reconcile?account_id=${account.id}`, 'button')}</div>`;
}
function renderSpending(info) {
  const rows = [['Housing', '€1,450.00', '100%', '/app/categories/11'], ['Groceries', '€612.30', '42%', '/app/categories/10'], ['Transport', '€284.10', '20%', '/app/transactions?category_id=12']];
  return `${heading(info, '1–29 Oct 2026 · all expense categories · EUR · posted actuals')}
    <div class="split-layout"><section class="card"><h2>Spending by category</h2><p class="fact">${money('€3,720.40')}</p>${rows.map(([name, value, width, path]) => `<div class="bar-row">${link(name, path)}<span class="bar" aria-hidden="true"><span style="width:${width}"></span></span>${money(value)}</div>`).join('')}<p class="scope">Top three categories shown. Other categories total ${money('€1,373.00')}.</p></section>
    <aside class="card"><h2>Explore this report</h2><p>Category links open their detail page. Your browser Back button returns here.</p>${link('Transactions in this period', '/app/transactions?from=2026-10-01&to=2026-10-29', 'button')}</aside></div>`;
}
function renderImport(info) {
  const view = route.params.get('view') || 'files';
  const batch = route.params.has('batch_id');
  const views = segmented('Import views', [['Files', '/app/import', 'files'], ['Connections', '/app/import?view=connections', 'connections'], ['Rules and profiles', '/app/import?view=rules', 'rules'], [`Dividends${scenario === 'empty' ? '' : ' · 3'}`, '/app/import?view=dividends', 'dividends']], view);
  const body = view === 'dividends' ? `<h2>Dividend review</h2><p>3 sample payments need optional attribution.</p>${link('Open income coverage', '/app/investments/income', 'button')}`
    : view === 'connections' ? '<h2>Online connections</h2><ul class="rows"><li><span>Trading 212<br><small class="muted">Not connected · fetches cash movements and trades with an API key</small></span><button data-sample>Connect…</button></li></ul><p class="scope">A connection belongs to an institution; Accounts › Institutions shows which accounts it feeds.</p>'
    : view === 'rules' ? '<h2>Rules and profiles</h2><ul class="rows"><li><span>Rule: payee contains “ALBERT HEIJN” → Groceries<br><small class="muted">Applied while reviewing imports</small></span></li><li><span>CSV profile: ING current account<br><small class="muted">Semicolon, decimal comma, DD-MM-YYYY</small></span></li></ul>'
    : batch ? `<h2>ING CSV · October</h2><p><span class="badge">In review</span> 42 source rows · 3 need a category</p><p>Known cash remains visible while incomplete rows stay in review.</p><label>Category for sample row <select id="import-category"><option value="">Choose category…</option><option>Groceries</option><option>Housing</option></select></label><div class="actions"><button data-review-import>Review sample changes</button>${link('Back to batches', '/app/import')}</div>`
    : `<h2>File imports</h2><p><button data-sample>${icon('upload')}Upload a file</button></p><ul class="rows"><li><span>ING CSV · October<br><small class="muted">42 rows · 3 need a category</small></span>${link('Continue review', '/app/import?batch_id=october', 'button')}</li></ul>`;
  return `${heading(info, 'Files, connections and the rules that apply to them. Nothing is committed until you review it.')}${views}<section class="card">${body}</section>`;
}
function renderReconcile(info) {
  const account = currentAccount();
  return `${heading(info, `${esc(account.detail)} · sample statement at 29 Oct`)}
    <section class="card"><h2>Statement for <button class="object-trigger" data-scope="Accounts">${esc(account.name)}${icon('chevron', 'nav-chev')}</button></h2>
    ${reconciliationFinished ? '<p role="status">Sample reconciliation finished. No checkpoint was written.</p>' : `<p>Statement balance ${money(account.balance)} · difference ${money('€0.00')}</p><p>Review the selected entries and exact statement before finishing.</p>${reconciliationPreview ? `<div><h3>Review checkpoint</h3><p>Account: ${esc(account.name)} · 29 Oct 2026 · difference ${money('€0.00')}</p><button class="primary" data-finish-reconcile>Finish sample reconciliation</button></div>` : '<button class="primary" data-preview-reconcile>Preview reconciliation</button>'}`}
    <div class="actions">${link('Open account register', accountPath(account.id))}</div></section>`;
}
function renderAccounts(info) {
  const view = route.params.get('view') || 'accounts';
  const showClosed = route.params.get('closed') === '1';
  const views = segmented('Account views', [['Accounts', '/app/ledger', 'accounts'], ['Institutions', '/app/ledger?view=institutions', 'institutions']], view);
  if (view === 'institutions') {
    return `${heading(info, 'Banks, brokers and other providers your accounts belong to', false, '<button data-sample>Add institution</button>')}${views}
      <section class="card"><div class="table-region" role="region" aria-label="Institutions" tabindex="0"><table><thead><tr><th>Institution</th><th>Country</th><th>Accounts</th><th>How data arrives</th></tr></thead><tbody>${institutions.map(i => {
        const open = accounts.filter(a => a.institution === i.name && !a.closed);
        return `<tr><td><strong>${esc(i.name)}</strong></td><td>${esc(i.country)}</td><td>${open.map(a => link(esc(a.name), accountPath(a.id))).join(', ')}</td><td>${esc(i.connection)}</td></tr>`;
      }).join('')}</tbody></table></div><p class="scope">Online connections are set up under ${link('Import › Connections', '/app/import?view=connections')}.</p></section>`;
  }
  const groups = institutions.map(i => [i, accounts.filter(a => a.institution === i.name && (showClosed || !a.closed))]).filter(([, list]) => list.length);
  const closed = accounts.filter(a => a.closed).length;
  // Ledger's Overview: the ledger's actions, pinned accounts first, then every account by institution.
  const pinned = accounts.filter(a => a.pinned && !a.closed);
  const actions = `<button data-sample>Transfer</button>${link('Import file', '/app/import', 'button')}${link('Reconcile', '/app/reconcile', 'button')}<button data-sample>Add account</button>`;
  return `${heading(info, 'Accounts by institution · balances at 29 Oct · EUR', true, actions)}${views}
    ${pinned.length && scenario !== 'empty' ? `<section class="card account-list pinned-list" aria-labelledby="pinned-title"><div class="account-group"><div class="group-head"><h2 id="pinned-title">Pinned</h2><span class="muted">Shown first here and on Home</span></div><ul class="rows">${pinned.map(a => `<li><span>${link(esc(a.name), accountPath(a.id))} <small class="muted">· ${esc(a.institution)}</small></span>${money(a.balance)}</li>`).join('')}</ul></div></section>` : ''}
    <section class="card account-list">${groups.map(([i, list]) => `<div class="account-group"><div class="group-head"><h2>${esc(i.name)}</h2><span class="muted">${esc(i.country)}</span></div>
      <ul class="rows">${list.map(a => `<li><span>${a.pinned ? '<span class="pin" aria-hidden="true">★</span><span class="sr-only">Pinned: </span>' : ''}${link(esc(a.name), accountPath(a.id))} <small class="muted">· ${esc(a.kind)}${a.closed ? ' · closed' : ''}${a.note ? ` · ${hidden ? 'cash and positions' : esc(a.note)}` : ''}</small></span>${money(a.balance)}</li>`).join('')}</ul></div>`).join('')}</section>
    <p>${showClosed ? link('Hide closed accounts', '/app/ledger') : link(`Show closed accounts (${closed})`, '/app/ledger?closed=1')}</p>`;
}
function renderIndex(info, objects, makePath) {
  return `${heading(info, 'Sample index · detail pages also support scoped search', info.name === 'Categories')}
    <section class="card"><h2>${esc(info.name)}</h2><ul class="rows">${objects.map(object => `<li><span>${link(esc(object.name), makePath(object.id))}<br><small class="muted">${esc(object.detail)}</small></span>${object.balance ? money(object.balance) : ''}</li>`).join('')}</ul></section>`;
}
function renderCategories(info) {
  const view = route.params.get('view') || 'categories';
  const views = segmented('Classification views', [['Categories', '/app/categories', 'categories'], ['Tags', '/app/categories?view=tags', 'tags']], view);
  const body = view === 'tags' ? '<h2>Tags</h2><ul class="rows"><li><span>holiday-2026<br><small class="muted">18 transactions</small></span></li><li><span>reimbursable<br><small class="muted">4 transactions</small></span></li></ul><p class="scope">Tags cross categories; archive a tag to stop suggesting it.</p>'
    : `<h2>Categories</h2><ul class="rows">${categories.map(c => `<li><span>${link(esc(c.name), `/app/categories/${c.id}`)}<br><small class="muted">${esc(c.detail)}</small></span></li>`).join('')}</ul>`;
  return `${heading(info, 'How spending and income are classified', view === 'categories')}${views}<section class="card">${body}</section>`;
}
function renderTransactions(info) {
  const view = route.params.get('view') || 'all';
  const views = segmented('Transaction views', [['All', '/app/transactions', 'all'], [`Drafts${scenario === 'empty' ? '' : ' · 2'}`, '/app/transactions?view=drafts', 'drafts'], ['Deleted', '/app/transactions?view=deleted', 'deleted']], view);
  const body = view === 'deleted' ? '<h2>Deleted transactions</h2><ul class="rows"><li><span>12 Oct · Duplicate coffee entry<br><small class="muted">Deleted 13 Oct · Checking</small></span><button data-sample>Restore</button></li></ul><p class="scope">Deleted transactions are out of the ledger until restored. Voided transactions stay in the ledger with their history and appear under All.</p>'
    : view === 'drafts' ? `<h2>Drafts</h2><p>2 recurring drafts awaiting review.</p>${link('Review in Recurring', '/app/recurring?tab=due', 'button')}`
    : `<h2>Transactions</h2>${link('Open Checking register', accountPath('1'), 'button')}<p class="scope">Global search and report links retain their URL filters.</p>`;
  return `${heading(info, `Sample entries${route.params.has('payee_id') ? ' · payee: Albert Heijn' : ''}${route.params.has('category_id') ? ' · selected category' : ''}${route.params.has('q') ? ` · matching “${esc(route.params.get('q'))}”` : ''}`, true)}${views}${view === 'all' ? filters() : ''}<section class="card">${body}</section>`;
}
function renderSettings(info) {
  const pages = {
    General: '<h2>General</h2><ul class="rows"><li><span>Default currency</span><span>EUR</span></li><li><span>Time zone</span><span>Europe/Amsterdam</span></li><li><span>Version</span><span>0.1.0-dev</span></li></ul>',
    Preferences: `<h2>Language and region</h2><p><label>Language <select><option>English</option><option>Nederlands</option></select></label></p><p class="scope">Numbers and dates follow the language; amounts never change.</p>
      <h2>Appearance</h2><p><label>Theme <select data-theme-select><option value="system">System</option><option value="light">Light</option><option value="dark">Dark</option></select></label></p>
      <h2>Home page</h2><p class="scope">Headline, shortcuts and widgets. The same panel opens from the sliders button on Home.</p><div class="actions"><button data-customize>Customize Home…</button><button data-cz="reset">Reset Home</button></div>`,
    'Currencies and FX rates': '<h2>Currencies and FX rates</h2><p>EUR · USD · GBP enabled. Rate history, refresh schedule and failed refreshes live here.</p>',
    Security: '<h2>Security</h2><p>Two-step sign-in, sessions and personal access tokens.</p>',
    'Data and backups': `<h2>Data and backups</h2><ul class="rows"><li><span>Backups<br><small class="muted">Not set up</small></span><button data-sample>Set up backups</button></li><li><span>Ledger self-check<br><small class="muted">Passed 28 Oct, after the nightly backup</small></span><button data-sample>Run now</button></li><li><span>Export<br><small class="muted">Ledger CSV, QIF and full archive</small></span>${link('Open Reports › Export', '/app/reports/export')}</li></ul>`
  };
  return `${heading(info)}<div class="settings-layout"><nav class="side-list" aria-label="Settings pages">${pageLinks(sections.Settings)}</nav>
    <section class="card">${pages[info.name] || ''}</section></div>`;
}
function renderOther(info) {
  if (route.path === '/app/ledger') return renderAccounts(info);
  if (route.path === '/app/planning') return `${heading(info, 'October · what needs attention this month')}<div class="grid">
    <section class="card"><h2>Budgets</h2><p class="fact">${money('€1,580')} left</p><p class="scope">70% spent, 87% of the month gone</p>${link('Open budgets →', '/app/budgets')}</section>
    <section class="card"><h2>Recurring</h2><p class="fact">2 due</p><p class="scope">Drafts waiting for review</p>${link('Review due →', '/app/recurring?tab=due')}</section>
    <section class="card"><h2>Forecast</h2><p class="fact">${money('€2,940')}</p><p class="scope">Lowest liquid balance in the next 90 days, 24 Nov · estimate</p>${link('Open forecast →', '/app/forecast')}</section></div>`;
  if (route.path === '/app/reports') return `${heading(info, 'Every report, with its last figure; saved reports below')}<div class="grid">
    ${[['Spending', 'Oct · €3,720 across 14 categories', '/app/reports/spending'], ['Cashflow', 'Oct · +€1,140 net', '/app/reports/cashflow'], ['Net worth', '29 Oct · €56,750', '/app/reports/net-worth'], ['Gains (in Investments)', 'Year to date · realized', '/app/investments/gains'], ['Export', 'Ledger CSV, QIF and archive', '/app/reports/export']].map(([name, fact, path]) => `<section class="card"><h2>${link(name, path)}</h2><p class="scope">${hidden ? 'Amounts hidden' : fact}</p></section>`).join('')}</div>
    <section class="card"><h2>Saved reports</h2><p class="muted">Saved filters and periods appear here.</p></section>`;
  if (info.name === 'Categories') return renderCategories(info);
  if (route.path === '/app/investments/instruments') return renderIndex(info, instruments, id => `/app/investments/instruments/${id}`);
  if (info.object === 'Categories') return `${heading(info, 'October · expense category · posted actuals', true)}<section class="card"><h2>Category activity</h2><p>${link('View matching transactions', `/app/transactions?category_id=${route.path.split('/').at(-1)}`)}</p>${link('Open spending report', '/app/reports/spending')}</section>`;
  if (info.object === 'Instruments') return `${heading(info, 'Vanguard FTSE All-World · held in DEGIRO brokerage · EUR', false, '<button data-invest>New transaction</button>')}<section class="card"><h2>Position</h2><ul class="rows"><li><span>${link('DEGIRO brokerage', accountPath('3'))}<br><small class="muted">96 units · average cost €78.10</small></span>${scenario === 'missing' ? 'No price' : money('€8,400.00')}</li></ul><h3>Splits and corporate actions</h3><p class="scope">None recorded. Whole-book split history is a view on Overview.</p></section>`;
  if (info.name === 'Transactions') return renderTransactions(info);
  if (info.name === 'Payees') return `${heading(info, 'Payee management belongs to #443')}<section class="card"><h2>Albert Heijn</h2><p>214 transactions · last used 27 Oct</p>${link('Transactions with Albert Heijn', '/app/transactions?payee_id=30', 'button')}</section>`;
  if (info.name === 'Recurring') return `${heading(info, 'Drafts and templates are separate views')}${segmented('Recurring views', [['Due', '/app/recurring?tab=due', 'due'], ['Templates', '/app/recurring?tab=templates', 'templates']], route.params.get('tab') === 'templates' ? 'templates' : 'due')}<section class="card"><h2>${route.params.get('tab') === 'templates' ? 'Templates' : '2 drafts awaiting review'}</h2><p>Sample review destination; posting drafts is outside this navigation prototype.</p></section>`;
  if (info.name === 'Net worth') return `${heading(info, '29 Oct 2026 · same 5 open sample accounts in EUR')}<section class="card"><h2>Valuation</h2><p class="fact">${scenario === 'manual' ? 'Unavailable · House has no value yet' : scenario === 'missing' ? `Unavailable · ${unpricedText()}` : money('€56,750.05')}</p>${link('Inspect accounts', '/app/ledger')}</section>`;
  if (info.name === 'Export') return `${heading(info, 'Whole-ledger files; each report also downloads its own table as CSV')}<section class="card"><h2>Ledger CSV, QIF and archive</h2><p>Navigation destination only. No sample files are generated.</p>${link('Backups live in Settings › Data and backups', '/app/settings/data')}</section>`;
  if (info.section === 'Settings') return renderSettings(info);
  if (route.path === '/app/investments') {
    const view = route.params.get('view') || 'positions';
    return `${heading(info, 'Positions across all brokerage accounts · 29 Oct', false, '<button data-invest>New transaction</button>')}${segmented('Overview views', [['Positions', '/app/investments', 'positions'], ['Splits', '/app/investments?view=splits', 'splits'], ['Suggestions · 1', '/app/investments?view=suggestions', 'suggestions']], view)}<section class="card"><h2>${view === 'splits' ? 'Split history' : view === 'suggestions' ? 'Suggested dividend events' : 'Positions'}</h2>${view === 'positions' ? `<ul class="rows"><li><span>${link('VWRL', '/app/investments/instruments/20')}<br><small class="muted">DEGIRO brokerage · 96 units</small></span>${scenario === 'missing' ? 'No price' : money('€8,400.00')}</li></ul>` : '<p>Sample view.</p>'}${route.params.has('view') ? `<p role="status">Selected view: ${esc(view)}</p>` : ''}</section>`;
  }
  return `${heading(info)}<section class="card"><h2>${esc(info.name)} destination</h2><p>This screen establishes navigation and page placement. Its detailed workflow is outside this prototype.</p>${link('Back to Home', '/app', 'button')}</section>`;
}
function render(navigated = false) {
  const old = route;
  if (old?.path === '/app' && navigated) homeScroll = lastHomeScroll;
  route = readRoute();
  const info = pageInfo();
  if (old?.path === '/app/reconcile' && old.canonical !== route.canonical) { reconciliationPreview = false; reconciliationFinished = false; }
  renderShell(info);
  $('#main').className = info.object === 'Accounts' || route.path === '/app/ledger' || ['Transactions', 'Import'].includes(info.name) ? 'fluid' : route.path === '/app' ? 'home' : '';
  $('#main').innerHTML = route.path === '/app' ? renderHome() : info.object === 'Accounts' ? renderRegister(info) : route.path === '/app/reports/spending' ? renderSpending(info) : info.name === 'Import' ? renderImport(info) : info.name === 'Reconcile' ? renderReconcile(info) : route.path === '/app/review' ? renderReview(info) : renderOther(info);
  if ($('[data-theme-select]')) $('[data-theme-select]').value = theme;
  document.title = `${info.title || info.name} · Rekenraam design prototype`;
  if (navigated) { $('#main').focus({ preventScroll: true }); requestAnimationFrame(() => scrollTo(0, route.path === '/app' ? homeScroll : 0)); }
}
// The hash changes before render runs, so remember Home's scroll as it happens.
let lastHomeScroll = 0;
addEventListener('scroll', () => { if (route?.path === '/app') lastHomeScroll = scrollY; }, { passive: true });
for (const [key, value] of Object.entries(options)) $(`#opt-${key}`).value = value;
render();
window.addEventListener('hashchange', () => render(true));

function closeDisclosures() {
  for (const trigger of document.querySelectorAll('[data-disclosure]')) {
    trigger.setAttribute('aria-expanded', 'false');
    const panel = document.getElementById(trigger.dataset.disclosure);
    if (panel) panel.hidden = true;
  }
  closeHomeList();
}

// ---- Search: one engine for Home's bar, the header field and object switchers ----

function scopeDestination(kind, id, scoped) {
  const path = kind === 'Accounts' ? (route.path === '/app/reconcile' ? '/app/reconcile' : accountPath(id)) : kind === 'Categories' ? `/app/categories/${id}` : `/app/investments/instruments/${id}`;
  const params = scoped ? new URLSearchParams(route.params) : new URLSearchParams();
  params.delete('dropped');
  if (kind === 'Accounts' && scoped) {
    if (params.has('holding_id')) { params.delete('holding_id'); params.set('dropped', 'holding selection'); }
    if (route.path === '/app/reconcile') params.set('account_id', id);
  }
  return path + (params.size ? `?${params}` : '');
}
function buildResults(raw, scope, showClosed = false) {
  const query = raw.toLocaleLowerCase().trim();
  const matches = text => text.toLocaleLowerCase().includes(query);
  const amount = a => hidden ? '••••••' : a.balance;
  if (scope) {
    // Scoped from an object page keeps that page's context; scoped from Home opens the default page.
    const keep = route.path !== '/app';
    const sources = scope === 'Accounts' ? accounts : scope === 'Categories' ? categories : instruments;
    const items = sources.filter(item => matches(`${item.name} ${item.detail}`) && (!item.closed || showClosed));
    const row = item => ({ label: item.name, detail: `${item.detail}${item.balance ? ` · ${amount(item)}` : ''}`, path: scopeDestination(scope, item.id, keep) });
    if (scope !== 'Accounts' || query) return items.map(row);
    return [...items.filter(a => a.pinned).map(a => ({ ...row(a), group: 'Pinned' })), ...items.filter(a => !a.pinned).map(a => ({ ...row(a), group: 'Other accounts' }))];
  }
  if (!query) {
    return [
      { label: 'Add transaction', detail: 'Command · opens the editor', action: 'add', group: 'Suggested' },
      ...['/app/transactions', '/app/reports/spending', '/app/investments/returns'].map(path => registry.find(p => p.path === path)).map(p => ({ label: p.name, detail: `Page · ${p.section}`, path: p.path, group: 'Suggested' })),
      { label: 'Checking', detail: `Pinned account · ${amount(accounts[0])}`, path: accountPath('1'), group: 'Suggested' }
    ];
  }
  const out = [];
  const payee = payees.find(p => matches(p.name));
  if (payee) {
    const group = `Payee · ${payee.name}`;
    out.push({ label: `Add expense at ${payee.name}`, detail: `Opens the editor prefilled · ${payee.category} (last used)`, action: 'add', payee, group });
    out.push({ label: `Transactions with ${payee.name}`, detail: `${payee.count} transactions · filtered by payee`, path: `/app/transactions?payee_id=${payee.id}`, group });
  }
  const group = 'Pages, accounts and categories';
  const commands = [
    { label: 'Add transaction', detail: 'Command', action: 'add' },
    { label: 'Transfer between accounts', detail: 'Command', action: 'transfer' },
    { label: 'Import a file', detail: 'Command', path: '/app/import' },
    { label: 'Reconcile an account', detail: 'Command', path: '/app/reconcile' }
  ];
  out.push(...[
    ...commands,
    ...registry.map(page => ({ label: page.name, detail: `Page · ${page.section}`, path: page.path })),
    ...accounts.filter(a => !a.closed).map(a => ({ label: a.name, detail: `Account · ${a.detail} · ${amount(a)}`, path: accountPath(a.id) })),
    ...categories.map(c => ({ label: c.name, detail: `Category · ${c.detail}`, path: `/app/categories/${c.id}` })),
    ...instruments.map(i => ({ label: i.name, detail: `Instrument · ${i.detail}`, path: `/app/investments/instruments/${i.id}` }))
  ].filter(item => matches(`${item.label} ${item.detail}`)).slice(0, 8).map(item => ({ ...item, group })));
  out.push({ label: `Search all transactions for “${raw.trim()}”`, detail: 'Payees, descriptions, references and notes', path: `/app/transactions?q=${encodeURIComponent(raw.trim())}`, group: 'Everything' });
  return out;
}
function paintResults(list, input, items, prefix, empty = 'No matching objects') {
  searchResults = items;
  selected = 0;
  let html = '';
  let current;
  items.forEach((item, index) => {
    if (item.group && item.group !== current) {
      if (current !== undefined) html += '</div>';
      current = item.group;
      html += `<div role="group" aria-labelledby="${prefix}-g${index}"><div class="dh" id="${prefix}-g${index}" role="presentation">${esc(item.group)}</div>`;
    }
    html += `<div class="result" id="${prefix}-${index}" role="option" aria-selected="${index === 0}" data-result="${index}">${esc(item.label)}<small>${esc(item.detail)}</small></div>`;
  });
  if (current !== undefined) html += '</div>';
  list.innerHTML = items.length ? html : `<div role="option" aria-disabled="true" aria-selected="false" class="result">${empty}</div>`;
  updateSelection(input, prefix);
}
function updateSelection(input, prefix) {
  input.removeAttribute('aria-activedescendant');
  for (const item of document.querySelectorAll(`[id^="${prefix}-"][data-result]`)) item.setAttribute('aria-selected', String(Number(item.dataset.result) === selected));
  if (searchResults.length) input.setAttribute('aria-activedescendant', `${prefix}-${selected}`);
}
function chooseResult(index) {
  const result = searchResults[index];
  if (!result) return;
  if ($('#search-dialog').open) $('#search-dialog').close();
  closeHomeList();
  if (result.action === 'add') openEntry(result.payee);
  else if (result.action === 'transfer') notify('Transfer editor is outside this navigation prototype. Use Add to test the shared entry interaction.');
  else location.hash = href(result.path);
}
function searchKeys(event, input, prefix, onEscape) {
  if (['ArrowDown', 'ArrowUp'].includes(event.key) && searchResults.length) {
    event.preventDefault();
    selected = (selected + (event.key === 'ArrowDown' ? 1 : -1) + searchResults.length) % searchResults.length;
    updateSelection(input, prefix);
    document.getElementById(`${prefix}-${selected}`).scrollIntoView({ block: 'nearest' });
  } else if (event.key === 'Enter') { event.preventDefault(); chooseResult(selected); }
  else if (event.key === 'Backspace' && !input.value && onEscape.scope) { onEscape.scope(); }
  else if (event.key === 'Escape' && onEscape.close) { event.preventDefault(); onEscape.close(); }
}
function openSearch(scope = '') {
  returnFocus = document.activeElement;
  closeDisclosures();
  searchScope = scope;
  $('#search-input').value = '';
  $('#show-closed').checked = false;
  updateSearch();
  $('#search-dialog').showModal();
  $('#search-input').focus();
}
function updateSearch() {
  $('#scope-chip').hidden = !searchScope;
  $('#scope-chip').textContent = `${searchScope} ×`;
  $('#scope-chip').setAttribute('aria-label', `Remove ${searchScope} scope`);
  $('#closed-control').hidden = searchScope !== 'Accounts';
  $('#scope-options').innerHTML = searchScope ? '' : ['Accounts', 'Categories', 'Instruments'].map(scope => `<button class="chip" data-search-scope="${scope}">${scope}</button>`).join('');
  paintResults($('#search-results'), $('#search-input'), buildResults($('#search-input').value, searchScope, $('#show-closed').checked), 'result');
}
$('#search-input').addEventListener('input', updateSearch);
$('#search-input').addEventListener('keydown', event => searchKeys(event, $('#search-input'), 'result', { scope: () => { searchScope = ''; updateSearch(); } }));
$('#show-closed').addEventListener('change', updateSearch);
$('#search-dialog').addEventListener('close', () => { if (returnFocus?.isConnected) returnFocus.focus(); });

// Home's bar is the same search inline, with its results under it. Phones open it full screen.
function openHomeList() {
  const input = $('#home-cmd');
  if (!input) return;
  input.setAttribute('aria-expanded', 'true');
  $('#home-cmd-panel').hidden = false;
  paintResults($('#home-cmd-list'), input, buildResults(input.value, homeScope), 'hres', 'No matches');
  $('#home-cmd-chips').innerHTML = !input.value && !homeScope ? `<span class="muted">Narrow to</span> ${['Accounts', 'Categories', 'Instruments'].map(scope => `<button class="chip" data-home-scope="${scope}">${scope}</button>`).join('')}` : '';
}
function closeHomeList() {
  const input = $('#home-cmd');
  if (!input || $('#home-cmd-panel').hidden) return;
  input.setAttribute('aria-expanded', 'false');
  input.removeAttribute('aria-activedescendant');
  $('#home-cmd-panel').hidden = true;
}
function setHomeScope(scope) {
  const value = $('#home-cmd')?.value || '';
  homeScope = scope;
  render();
  const input = $('#home-cmd');
  input.value = value;
  input.focus();
  openHomeList();
}
document.addEventListener('focusin', event => {
  if (event.target.id !== 'home-cmd') return;
  if (phone()) { event.target.blur(); openSearch(homeScope); return; }
  openHomeList();
});
document.addEventListener('input', event => { if (event.target.id === 'home-cmd') openHomeList(); });
document.addEventListener('keydown', event => {
  if (event.target.id !== 'home-cmd') return;
  if ($('#home-cmd-panel').hidden && ['ArrowDown', 'Enter'].includes(event.key)) openHomeList();
  searchKeys(event, event.target, 'hres', { scope: homeScope ? () => setHomeScope('') : null, close: () => { if (!$('#home-cmd-panel').hidden) closeHomeList(); else event.target.blur(); } });
});
document.addEventListener('mousedown', event => { if (event.target.closest('#home-cmd-panel')) event.preventDefault(); });
document.addEventListener('focusout', event => {
  if (event.target.id === 'home-cmd' && !event.relatedTarget?.closest?.('.cmd')) closeHomeList();
});

// ---- Entry ----------------------------------------------------------------

function openEntry(payee) {
  returnFocus = document.activeElement;
  closeDisclosures();
  $('#entry-account').innerHTML = accounts.filter(a => !a.closed).map(a => `<option value="${a.id}">${a.name} · ${a.detail}</option>`).join('');
  $('#entry-form').reset();
  $('#entry-account').value = currentAccount().id;
  if (pageInfo().object === 'Categories') $('#entry-form').elements.category.value = pageInfo().name === 'Groceries' ? 'Groceries' : 'Housing';
  if (payee) { $('#entry-form').elements.payee.value = payee.name; $('#entry-form').elements.category.value = payee.category; }
  entryDirty = false;
  $('#discard-warning').hidden = true;
  $('#keep-entry').hidden = true;
  $('#discard-entry').hidden = true;
  $('#entry-dialog').showModal();
  if (payee) $('#entry-form').elements.amount.focus();
}
function cancelEntry() {
  if (entryDirty) { $('#discard-warning').hidden = false; $('#keep-entry').hidden = false; $('#discard-entry').hidden = false; $('#keep-entry').focus(); }
  else $('#entry-dialog').close();
}
$('#entry-form').addEventListener('input', () => { entryDirty = true; });
$('#entry-dialog').addEventListener('cancel', event => { event.preventDefault(); cancelEntry(); });
$('#entry-dialog').addEventListener('close', () => { if (returnFocus?.isConnected) returnFocus.focus(); });
$('#entry-form').addEventListener('submit', event => {
  event.preventDefault();
  entryDirty = false;
  $('#entry-dialog').close();
  notify('Sample entry confirmed. No financial data was written.');
});

// ---- Home layout actions ----------------------------------------------------

function focusAfter(selector, fallback) { requestAnimationFrame(() => (document.querySelector(selector) || document.querySelector(fallback))?.focus()); }
function widgetAction(action, id) {
  const defs = Object.fromEntries(widgetDefs().map(d => [d.id, d]));
  const i = layout.home.indexOf(id);
  const title = defs[id].title;
  if (action === 'up' || action === 'down') {
    changeLayout({ home: swap(layout.home, i, action === 'up' ? i - 1 : i + 1) }, `${title} moved ${action}`, false);
    focusAfter(`[aria-controls="home-${id}-menu"]`);
  } else if (action === 'remove') {
    const next = layout.home[i + 1] || layout.home[i - 1];
    changeLayout({ home: layout.home.filter(x => x !== id) }, `${title} moved to More to explore`);
    focusAfter(next ? `[aria-controls="home-${next}-menu"]` : '#my-home-title', '#main');
  } else if (action === 'add' || action === 'top') {
    changeLayout({ home: action === 'top' ? [id, ...layout.home] : [...layout.home, id] }, `${title} added to ${action === 'top' ? 'the top of ' : ''}My Home`);
    focusAfter(`[aria-controls="home-${id}-menu"]`);
  } else if (action === 'settings') {
    notify('Widget settings (period, accounts) are part of personalization, #438.');
  }
}
function shortcutAction(action, id) {
  const i = layout.shortcuts.indexOf(id);
  const label = shortcutPool[id].label;
  if (action === 'remove') { changeLayout({ shortcuts: layout.shortcuts.filter(x => x !== id) }, `Shortcut ${label} removed`); focusAfter('[aria-controls="sc-new"]'); }
  else { changeLayout({ shortcuts: swap(layout.shortcuts, i, action === 'left' ? i - 1 : i + 1) }, `${label} moved ${action}`, false); focusAfter(`[aria-controls="sc-${id}"]`); }
}
function customizeAction(target) {
  const { cz: action, list, id } = target.dataset;
  if (action === 'reset') { changeLayout(defaultLayout(), 'Home reset to defaults'); focusAfter('[data-cz="reset"]'); return; }
  const i = layout[list].indexOf(id);
  if (action === 'remove') changeLayout({ [list]: layout[list].filter(x => x !== id) }, `${list === 'home' ? 'Widget moved to More to explore' : 'Shortcut removed'}`);
  else changeLayout({ [list]: swap(layout[list], i, action === 'up' ? i - 1 : i + 1) }, '', false);
  focusAfter(`[data-cz="${action}"][data-id="${id}"]:not([disabled])`, '#customize-body h3');
}

document.addEventListener('click', event => {
  const target = event.target.closest('button, a, [data-result]');
  if (!target) { if (!event.target.closest('.disclosure, #mobile-pages, .cmd')) closeDisclosures(); return; }
  if (target.classList.contains('skip')) { event.preventDefault(); $('#main').focus(); return; }
  if (target.dataset.disclosure) {
    const wasOpen = target.getAttribute('aria-expanded') === 'true';
    closeDisclosures();
    target.setAttribute('aria-expanded', String(!wasOpen));
    document.getElementById(target.dataset.disclosure).hidden = wasOpen;
  } else if (target.hasAttribute('data-search')) openSearch();
  else if (target.dataset.scope) openSearch(target.dataset.scope);
  else if (target.dataset.searchScope) { searchScope = target.dataset.searchScope; $('#search-input').value = ''; updateSearch(); $('#search-input').focus(); }
  else if (target.id === 'scope-chip') { searchScope = ''; updateSearch(); $('#search-input').focus(); }
  else if (target.dataset.homeScope) setHomeScope(target.dataset.homeScope);
  else if (target.id === 'home-scope-chip') setHomeScope('');
  else if (target.hasAttribute('data-result')) chooseResult(Number(target.dataset.result));
  else if (target.hasAttribute('data-add')) openEntry();
  else if (target.hasAttribute('data-cancel-entry')) cancelEntry();
  else if (target.id === 'keep-entry') { $('#discard-warning').hidden = true; $('#keep-entry').hidden = true; $('#discard-entry').hidden = true; $('#entry-form').elements.payee.focus(); }
  else if (target.id === 'discard-entry') { entryDirty = false; $('#entry-dialog').close(); }
  else if (target.dataset.close) document.getElementById(target.dataset.close).close();
  else if (target.id === 'hide-amounts') {
    hidden = !hidden;
    try { localStorage.setItem('navigation-prototype-hidden', String(hidden)); } catch { /* Optional browser persistence. */ }
    render(); $('#hide-amounts').focus();
  }
  else if (target.id === 'explore-toggle') { layout.exploreOpen = !layout.exploreOpen; render(); $('#explore-toggle').focus(); }
  else if (target.dataset.headline) { changeLayout({ headline: target.dataset.headline }, '', false); focusAfter('.bl-switch'); }
  else if (target.dataset.w) widgetAction(target.dataset.w, target.dataset.id);
  else if (target.dataset.sc) shortcutAction(target.dataset.sc, target.dataset.id);
  else if (target.dataset.scAdd) { changeLayout({ shortcuts: [...layout.shortcuts, target.dataset.scAdd] }, `Shortcut ${shortcutPool[target.dataset.scAdd].label} added`); focusAfter(`[aria-controls="sc-${target.dataset.scAdd}"]`); }
  else if (target.dataset.cz) customizeAction(target);
  else if (target.hasAttribute('data-customize')) { returnFocus = document.activeElement; closeDisclosures(); renderCustomize(); $('#customize-dialog').showModal(); }
  else if (target.hasAttribute('data-undo')) { if (undoLayout) { layout = undoLayout; undoLayout = null; render(); if ($('#customize-dialog').open) renderCustomize(); } $('#notice').textContent = 'Change undone'; }
  else if (target.hasAttribute('data-retry')) { scenario = 'everyday'; $('#scenario').value = scenario; render(); notify('Sample review check completed.'); }
  else if (target.hasAttribute('data-preview-reconcile')) { reconciliationPreview = true; render(); $('[data-finish-reconcile]').focus(); }
  else if (target.hasAttribute('data-finish-reconcile')) { reconciliationFinished = true; render(); $('#main').focus(); }
  else if (target.hasAttribute('data-review-import')) notify($('#import-category').value ? 'Sample changes reviewed. The batch remains uncommitted.' : 'Choose a category for the sample row first.');
  else if (target.hasAttribute('data-invest')) notify('Buy, sell, dividend and corporate-action forms open here in the app; they are outside this navigation prototype.');
  else if (target.hasAttribute('data-sample')) notify('Sample control only; nothing is changed.');
  else if (target.hasAttribute('data-transfer')) notify('Transfer editor is outside this navigation prototype. Use Add to test the shared entry interaction.');
  else if (target.hasAttribute('data-sign-out')) { closeDisclosures(); notify('Sample session only. There is no authenticated connection to sign out.'); }
  else if (target.tagName === 'A') closeDisclosures();
});
$('#customize-dialog').addEventListener('close', () => { if (returnFocus?.isConnected) returnFocus.focus(); });
document.addEventListener('submit', event => {
  if (event.target.id !== 'filters') return;
  event.preventDefault();
  const params = new URLSearchParams(route.params);
  for (const [key, value] of new FormData(event.target)) { if (value) params.set(key, value); else params.delete(key); }
  location.hash = href(`${route.path}?${params}`);
});
document.addEventListener('change', event => {
  if (event.target.matches('[data-theme-select]')) { theme = event.target.value; applyTheme(); $('#theme').value = theme; }
  if (event.target.id === 'theme') { theme = event.target.value; applyTheme(); try { localStorage.setItem('navigation-prototype-theme', theme); } catch { /* Optional browser persistence. */ } }
  const option = event.target.id?.match(/^opt-(sections)$/)?.[1];
  if (option) { options[option] = event.target.value; try { localStorage.setItem('navigation-prototype-options', JSON.stringify(options)); } catch { /* Optional browser persistence. */ } render(); }
  if (event.target.id === 'scenario') { scenario = event.target.value; reconciliationPreview = false; reconciliationFinished = false; render(); }
  if (event.target.name === 'cz-headline') changeLayout({ headline: event.target.value }, '', false);
  if (event.target.id === 'cz-explore') { layout.exploreOpen = event.target.checked; render(); }
});
document.addEventListener('keydown', event => {
  if (event.key === 'Escape' && !document.querySelector('dialog[open]')) {
    const trigger = document.querySelector('[data-disclosure][aria-expanded="true"]');
    closeDisclosures(); trigger?.focus(); return;
  }
  if (document.querySelector('dialog[open]') || event.target.closest('input, select, textarea, [contenteditable="true"]')) return;
  const focusSearch = () => { if ($('#home-cmd') && !phone()) { event.preventDefault(); $('#home-cmd').focus(); } else { event.preventDefault(); openSearch(); } };
  if (((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') || (!event.ctrlKey && !event.metaKey && !event.altKey && event.key === '/')) focusSearch();
  else if (!event.ctrlKey && !event.metaKey && !event.altKey && event.key.toLowerCase() === 'n') { event.preventDefault(); openEntry(); }
  else if (event.key === '?') $('#help-dialog').showModal();
});
