import type { SearchCatalogItem } from '#lib/api/search.ts';
import { builtinCategoryLabel } from '#lib/categories/category-labels.ts';
import { routes, routeSearchKeywords, sectionFor } from '#lib/navigation/routes.ts';
import { m } from '#lib/paraglide/messages.js';
import type { SearchResult } from './search-model.ts';

export function searchItems(catalog: SearchCatalogItem[], hidden: boolean): SearchResult[] {
  const pages: SearchResult[] = routes.filter(route => !route.legacy && !route.route.includes('[')).map(route => {
    const section = sectionFor(route.section)?.title() ?? (route.section === 'settings' ? m.settings_title() : '');
    return { id: `page:${route.route}`, group: 'page', title: route.titleKey === 'nav_overview' ? section : route.title(),
      detail: section, href: route.route, keywords: routeSearchKeywords(route).join(' ') };
  });
  const objects: SearchResult[] = catalog.map(item => ({
    id: `${item.kind}:${item.id}`, group: item.kind,
    title: item.name.trim() || (item.builtin_key ? builtinCategoryLabel(item.builtin_key) : item.code.trim()) || `#${item.id}`,
    keywords: `${item.code} ${item.account_kind}`,
    detail: item.kind === 'category' && item.builtin_key ? undefined : item.code.trim(), inactive: item.status !== 'active',
    href: item.kind === 'account' ? `/app/ledger/accounts/${item.id}/register`
      : item.kind === 'category' ? `/app/ledger/categories/${item.id}` : `/app/investments/instruments/${item.id}`
  }));
  const commands: SearchResult[] = [
    { id: 'command:add', group: 'command', title: m.transactions_form_create_title(), command: 'add-transaction' },
    { id: 'command:transfer', group: 'command', title: m.home_shortcut_transfer(), command: 'add-transfer' },
    { id: 'command:amounts', group: 'command', title: hidden ? m.search_show_amounts() : m.nav_hide_amounts(), command: 'hide-amounts' },
    { id: 'command:theme', group: 'command', title: m.search_switch_theme(), command: 'theme' }
  ];
  return [...pages, ...objects, ...commands];
}
