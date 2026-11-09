import type { SearchResult, SearchScope } from './search-model.ts';

// The one mounted search that owns the shortcuts (the header's, or Home's
// command bar) registers here, so other UI such as #442's object switcher can
// open that same search with a scope chip and its own selection handler
// instead of mounting a second index, fetcher or result list.

export type SearchRequest = {
  scope?: SearchScope;
  onSelect?: (item: SearchResult) => void | Promise<void>;
};

let owner: ((request: SearchRequest) => boolean) | undefined;

/** Registers the shortcut owner; the returned undo only clears this owner. */
export function registerSearchOwner(open: (request: SearchRequest) => boolean): () => void {
  owner = open;
  return () => { if (owner === open) owner = undefined; };
}

/** Opens the shared search; false when none is mounted or a modal owns the keyboard. */
export function openSearch(request: SearchRequest = {}): boolean {
  return owner?.(request) ?? false;
}
