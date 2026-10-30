<script lang="ts">
  import type { Attachment } from 'svelte/attachments';
  import { tick } from 'svelte';
  import { beforeNavigate, goto } from '$app/navigation';
  import { createInfiniteQuery } from '@tanstack/svelte-query';
  import Search from '@lucide/svelte/icons/search';
  import X from '@lucide/svelte/icons/x';
  import ModalDialog from '#lib/components/modal-dialog.svelte';
  import { modalOpen } from '#lib/components/modal-focus.ts';
  import { getSearchCatalog, searchCatalogQueryKey } from '#lib/api/search.ts';
  import { getAPIClientErrorMessage } from '#lib/api-error-messages.ts';
  import { dismissible } from '#lib/navigation/disclosure.ts';
  import { amountPrivacy, setAmountsHidden } from '#lib/money/privacy.svelte.ts';
  import { themeState, setTheme } from '#lib/theme.svelte.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { addTransaction } from '#lib/transactions/add-transaction.svelte.ts';
  import { searchItems } from './search-items.ts';
  import { createSearchIndex, searchGroups, searchResults, searchShortcut, type SearchResult, type SearchScope } from './search-model.ts';

  // The owner mounts one global field. Home can use variant="home" in place of
  // the header; an object switcher can supply scope and onSelect without owning
  // a second index, data fetcher, shortcut handler or result UI.
  let { scope = 'all', variant = 'compact', shortcuts = true, onSelect }: {
    scope?: SearchScope;
    variant?: 'compact' | 'home';
    shortcuts?: boolean;
    onSelect?: (item: SearchResult) => void | Promise<void>;
  } = $props();
  const uid = $props.id();
  const listID = `${uid}-results`;
  let open = $state(false);
  let phone = $state(false);
  let query = $state('');
  let currentScope = $state<SearchScope>('all');
  let inactive = $state(false);
  let limit = $state(6);
  let activeID = $state<string>();
  let input: HTMLInputElement | undefined;
  let restoringFocus = false;
  let navigationError = $state(false);

  const catalog = createInfiniteQuery(() => ({
    queryKey: searchCatalogQueryKey,
    queryFn: ({ pageParam, signal }) => getSearchCatalog(pageParam, signal),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
    enabled: open,
    staleTime: 0,
    notifyOnChangeProps: 'all'
  }));
  const objects = $derived(catalog.data?.pages.flatMap(page => page.items) ?? []);
  const items = $derived(searchItems(objects, amountPrivacy.hidden));
  const index = $derived(createSearchIndex(items));
  const matches = $derived(searchResults(items, index, query, currentScope, inactive, getLocale()));
  const groups = $derived(searchGroups.map(group => ({ group, items: matches.filter(item => item.group === group).slice(0, limit) })));
  const visible = $derived(groups.flatMap(group => group.items));
  const active = $derived(visible.find(item => item.id === activeID) ?? visible[0]);
  const loading = $derived(catalog.isPending || catalog.isFetching || catalog.hasNextPage);

  // Follow every catalog cursor. Until complete, never call zero matches 'no results'.
  $effect(() => {
    if (open && catalog.hasNextPage && !catalog.isFetching && !catalog.isError) void catalog.fetchNextPage();
  });
  $effect(() => { currentScope = scope; });

  function label(group: SearchScope): string {
    switch (group) {
      case 'all': return m.search_all();
      case 'page': return m.search_pages();
      case 'account': return m.accounts_title();
      case 'category': return m.categories_title();
      case 'instrument': return m.quotes_instruments();
      case 'command': return m.search_commands();
    }
  }
  function close(restore: boolean) {
    open = false;
    activeID = undefined;
    if (restore && !phone) {
      restoringFocus = true;
      input?.focus();
      restoringFocus = false;
    }
  }
  function resetSelection() { activeID = undefined; limit = 6; navigationError = false; }
  function changeScope(value: SearchScope) { currentScope = value; resetSelection(); input?.focus(); }
  async function focusSearch() {
    // Never steal focus out of another modal or phone editor.
    if (modalOpen() && !open) return;
    phone = window.matchMedia('(max-width: 767px)').matches;
    open = true;
    await tick();
    input?.focus();
  }
  function shortcut(event: KeyboardEvent) {
    if (shortcuts && searchShortcut(event) && (!modalOpen() || open)) {
      event.preventDefault(); void focusSearch();
    }
  }
  const bindInput: Attachment<HTMLInputElement> = node => {
    input = node;
    if (open) node.focus();
    return () => { if (input === node) input = undefined; };
  };
  const viewport: Attachment<HTMLElement> = () => {
    const media = window.matchMedia('(max-width: 767px)');
    phone = media.matches;
    const resize = () => { close(false); phone = media.matches; };
    media.addEventListener('change', resize);
    return () => media.removeEventListener('change', resize);
  };
  // Reset the page we are leaving, never a new search opened on arrival.
  // Query/hash-only navigation (such as a report repairing its default range)
  // stays on the same page and must not erase an independent search (#472).
  beforeNavigate(({ from, to }) => {
    if (from?.url.pathname === to?.url.pathname) return;
    close(false); query = ''; currentScope = scope; inactive = false; resetSelection();
  });

  async function choose(item: SearchResult) {
    navigationError = false;
    try {
      // Release the phone modal before another workflow opens its own focus
      // scope. Completion must not close a later search opened by the user.
      close(false);
      await tick();
      if (onSelect) await onSelect(item);
      else if (item.command === 'hide-amounts') setAmountsHidden(!amountPrivacy.hidden);
      else if (item.command === 'theme') setTheme(themeState.name === 'dark' ? 'light' : 'dark');
      else if (item.command === 'add-transaction') await addTransaction();
      else if (item.href) await goto(item.href);
    } catch { open = true; navigationError = true; await tick(); input?.focus(); }
  }
  async function keydown(event: KeyboardEvent) {
    if (event.isComposing) return;
    if (event.key === 'Escape') {
      if (!phone) { event.preventDefault(); close(true); }
      return;
    }
    if (event.key === 'Enter' && open && active) { event.preventDefault(); await choose(active); return; }
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault(); open = true;
      const position = visible.findIndex(item => item.id === active?.id);
      activeID = visible[(position + (event.key === 'ArrowDown' ? 1 : -1) + visible.length) % visible.length]?.id;
      await tick();
      document.getElementById(`${uid}-${activeID}`)?.scrollIntoView({ block: 'nearest' });
    }
  }
</script>

<svelte:window onkeydown={shortcut} />

{#snippet field()}
  <div class="flex min-w-0 items-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 focus-within:ring-2 focus-within:ring-accent">
    <Search size={16} aria-hidden="true" class="shrink-0 text-muted" />
    <input {@attach bindInput} data-initial-focus type="text" role="combobox" aria-label={m.search_label()}
      aria-autocomplete="list" aria-expanded={open} aria-controls={open ? listID : undefined}
      aria-activedescendant={open && active ? `${uid}-${active.id}` : undefined}
      aria-describedby={open ? `${uid}-status` : undefined} autocomplete="off"
      placeholder={m.search_placeholder()} value={query}
      oninput={event => { query = event.currentTarget.value; open = true; resetSelection(); }}
      onfocus={() => { if (!restoringFocus) open = true; }} onkeydown={keydown}
      class="min-h-11 min-w-0 w-full bg-transparent text-sm text-foreground outline-none placeholder:text-muted" />
  </div>
{/snippet}

{#snippet results()}
  <div class="flex flex-wrap gap-1 border-b border-border p-2" aria-label={m.search_scope_label()}>
    {#each ['all', 'account', 'category', 'instrument'] as choice}
      {@const value = choice as SearchScope}
      <button type="button" aria-pressed={currentScope === value} onclick={() => changeScope(value)}
        class="min-h-11 rounded-(--radius-control) px-2 text-xs text-muted hover:bg-control-hover aria-pressed:bg-surface-strong aria-pressed:text-foreground">{label(value)}</button>
    {/each}
  </div>
  <div id={`${uid}-status`} role="status" class="px-3 py-2 text-xs text-muted">
    {#if catalog.isError}{m.search_partial_error()} <button type="button" onclick={() => { void catalog.refetch(); }} class="min-h-11 underline">{m.search_retry()}</button>
    {:else if loading}{m.search_loading()}
    {:else if matches.length === 0}{m.search_empty()}
    {:else}{m.search_result_count({ shown: visible.length, total: matches.length })}{/if}
  </div>
  {#if catalog.isError}<p role="alert" class="px-3 text-sm text-danger">{getAPIClientErrorMessage(catalog.error)}</p>{/if}
  {#if navigationError}<p role="alert" class="px-3 text-sm text-danger">{m.search_navigation_error()}</p>{/if}
  <div id={listID} role="listbox" aria-label={m.search_results()} class="max-h-[55dvh] overflow-y-auto p-1">
    {#each groups as group (group.group)}
      {#if group.items.length}
        <div role="group" aria-labelledby={`${uid}-group-${group.group}`}>
          <div id={`${uid}-group-${group.group}`} class="px-3 pt-3 pb-1 text-xs font-semibold text-muted">{label(group.group)}</div>
          {#each group.items as item (item.id)}
            <button type="button" role="option" id={`${uid}-${item.id}`} aria-selected={active?.id === item.id}
              tabindex="-1" onpointerdown={event => event.preventDefault()} onclick={() => { void choose(item); }}
              class="flex min-h-11 w-full cursor-pointer flex-col text-left justify-center rounded-(--radius-control) px-3 py-2 text-sm text-foreground hover:bg-control-hover aria-selected:bg-surface-strong">
              <span class="wrap-anywhere">{item.title}</span>
              {#if item.detail}<span class="wrap-anywhere text-xs text-muted">{item.detail}</span>{/if}
            </button>
          {/each}
        </div>
      {/if}
    {/each}
  </div>
  {#if visible.length < matches.length}<button type="button" onclick={() => { limit += 10; }} class="min-h-11 w-full text-sm text-accent hover:underline">{m.search_more()}</button>{/if}
  <label class="flex min-h-11 items-center gap-2 border-t border-border px-3 text-xs text-muted"><input type="checkbox" bind:checked={inactive} onchange={resetSelection} />{m.search_include_inactive()}</label>
{/snippet}

<div class="relative min-w-0 {variant === 'home' ? 'w-full' : ''}" {@attach viewport} {@attach dismissible(() => open && !phone, close)}>
  {#if phone}
    <button type="button" aria-label={variant === 'home' ? m.search_placeholder() : m.search_label()} aria-haspopup="dialog" onclick={() => { void focusSearch(); }}
      class="flex min-h-11 min-w-11 items-center justify-center rounded-full text-muted hover:bg-control-hover hover:text-foreground">
      <Search size={18} aria-hidden="true" />
      {#if variant === 'home'}<span class="px-3 text-sm">{m.search_placeholder()}</span>{/if}
    </button>
    {#if open}
      <ModalDialog label={m.search_label()} onCancel={() => close(false)} class="m-0! h-dvh max-h-dvh! w-full! max-w-none! rounded-none!">
        <div class="flex items-center gap-2 border-b border-border p-3">
          <div class="min-w-0 flex-1">{@render field()}</div>
          <button type="button" aria-label={m.search_close()} onclick={() => close(false)} class="flex min-h-11 min-w-11 items-center justify-center rounded-full text-muted hover:bg-control-hover"><X size={18} aria-hidden="true" /></button>
        </div>
        {@render results()}
      </ModalDialog>
    {/if}
  {:else}
    {@render field()}
    {#if open}
      <div data-search-popup class="absolute right-0 top-full z-50 mt-2 w-[min(32rem,calc(100vw-2rem))] rounded-(--radius-panel) border border-border bg-surface shadow-(--shadow-panel)">
        {@render results()}
      </div>
    {/if}
  {/if}
</div>

