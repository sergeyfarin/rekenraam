import MiniSearch from 'minisearch';

export const searchGroups = ['page', 'account', 'category', 'instrument', 'command'] as const;
export type SearchGroup = typeof searchGroups[number];
export type SearchScope = SearchGroup | 'all';
export type SearchResult = {
  id: string;
  group: SearchGroup;
  title: string;
  detail?: string;
  keywords?: string;
  href?: string;
  command?: 'add-transaction' | 'add-transfer' | 'hide-amounts' | 'theme';
  inactive?: boolean;
};

export function normalizeSearch(value: string): string {
  return value.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase().trim();
}

/** Small reference sets only; never transaction history. Build once per catalog/locale. */
export function createSearchIndex(items: SearchResult[]) {
  const index = new MiniSearch<SearchResult>({
    fields: ['title', 'detail', 'keywords'],
    processTerm: normalizeSearch,
    searchOptions: { combineWith: 'AND', prefix: true, fuzzy: 0.2, boost: { title: 3 } }
  });
  index.addAll(items);
  return index;
}

/** Stable grouped results, exact names before prefix/fuzzy matches within a group. */
export function searchResults(
  items: SearchResult[], index: MiniSearch<SearchResult>, query: string,
  scope: SearchScope = 'all', includeInactive = false, locale = 'en'
): SearchResult[] {
  const term = normalizeSearch(query);
  const scores = new Map(term ? index.search(term).map(item => [item.id, item.score]) : []);
  const rank = (item: SearchResult) => {
    const title = normalizeSearch(item.title);
    return title === term ? 3 : title.startsWith(term) ? 2 : 1;
  };
  return items.filter(item => {
    if (scope !== 'all' && item.group !== scope) return false;
    if (item.inactive && !includeInactive) return false;
    if (!term) return scope !== 'all' || item.group === 'page' || item.group === 'command';
    // Literal substring supports tickers and names with punctuation too.
    return scores.has(item.id) || normalizeSearch(`${item.title} ${item.detail ?? ''} ${item.keywords ?? ''}`).includes(term);
  }).sort((a, b) => searchGroups.indexOf(a.group) - searchGroups.indexOf(b.group)
    || (term ? rank(b) - rank(a) || (scores.get(b.id) ?? 0) - (scores.get(a.id) ?? 0) : 0)
    || a.title.localeCompare(b.title, locale) || a.id.localeCompare(b.id));
}

export function isEditableTarget(target: EventTarget | null): boolean {
  return target !== null && target instanceof HTMLElement && (target.isContentEditable || !!target.closest('input, textarea, select, [role="textbox"], [role="combobox"]'));
}

export function searchShortcut(event: KeyboardEvent): boolean {
  if (event.defaultPrevented || event.isComposing || event.repeat || event.altKey) return false;
  if ((event.ctrlKey || event.metaKey) && !event.shiftKey && event.key.toLowerCase() === 'k') return true;
  return event.key === '/' && !event.ctrlKey && !event.metaKey && !isEditableTarget(event.target);
}
