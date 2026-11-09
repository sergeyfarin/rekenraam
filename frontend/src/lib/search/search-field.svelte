<script lang="ts">
  import type { Attachment } from 'svelte/attachments';
  import { tick } from 'svelte';
  import { beforeNavigate, goto } from '$app/navigation';
  import { createInfiniteQuery } from '@tanstack/svelte-query';
  import Search from '@lucide/svelte/icons/search';
  import X from '@lucide/svelte/icons/x';
  import ModalDialog from '#lib/components/modal-dialog.svelte';
  import { modalOpen } from '#lib/components/modal-focus.ts';
  import { editorOpen } from '#lib/components/editor-hold.svelte.ts';
  import { getSearchCatalog, searchCatalogQueryKey } from '#lib/api/search.ts';
  import { getAPIClientErrorMessage } from '#lib/api-error-messages.ts';
  import { dismissible } from '#lib/navigation/disclosure.ts';
  import { amountPrivacy, setAmountsHidden } from '#lib/money/privacy.svelte.ts';
  import { themeState, setTheme } from '#lib/theme.svelte.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { addTransaction, addTransfer } from '#lib/transactions/add-transaction.svelte.ts';
  import { searchItems } from './search-items.ts';
  import { registerSearchOwner, type SearchRequest } from './search-launcher.ts';
  import { SearchRequestState } from './search-request.svelte.ts';
  import { applePlatform, createSearchIndex, searchGroups, searchResults, searchShortcut, searchShortcutHint, type SearchResult, type SearchScope } from './search-model.ts';

  // The owner mounts one global search. The header renders it as a *Search
  // Ctrl K* button opening a centred dialog (#490); Home uses variant="home"
  // as its command bar instead. An object switcher opens the owner's search
  // through openSearch (search-launcher.ts) with a scope and onSelect, without
  // a second index, data fetcher, shortcut handler or result UI.
  let { scope = 'all', variant = 'header', shortcuts = true, onSelect }: {
    scope?: SearchScope;
    variant?: 'header' | 'home';
    shortcuts?: boolean;
    onSelect?: (item: SearchResult) => void | Promise<void>;
  } = $props();
  const uid = $props.id();
  const listID = `${uid}-results`;
  let open = $state(false);
  let phone = $state(false);
  const home = $derived(variant === 'home');
  // Everything but Home's desktop bar is a dialog: full screen on phones,
  // centred over the page from the header button.
  const modal = $derived(phone || !home);
  // Phones open the same field at its usual size.
  const large = $derived(!phone);
  let trigger = $state<HTMLButtonElement>();
  let apple = $state(false);
  const hint = $derived(searchShortcutHint(apple, m.keyboard_key_ctrl()));
  // The shown scope and an openSearch request's scope chip and selection.
  const session = new SearchRequestState(() => scope, () => onSelect);
  let query = $state('');
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
  const matches = $derived(searchResults(items, index, query, session.scope, inactive, getLocale()));
  const groups = $derived(searchGroups.map(group => ({ group, items: matches.filter(item => item.group === group).slice(0, limit) })));
  const visible = $derived(groups.flatMap(group => group.items));
  const active = $derived(visible.find(item => item.id === activeID) ?? visible[0]);
  const loading = $derived(catalog.isPending || catalog.isFetching || catalog.hasNextPage);

  // Follow every catalog cursor. Until complete, never call zero matches 'no results'.
  $effect(() => {
    if (open && catalog.hasNextPage && !catalog.isFetching && !catalog.isError) void catalog.fetchNextPage();
  });
  $effect(() => { session.reset(); });

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
  // Closes without ending a request: choosing releases focus this way.
  function release() {
    open = false;
    activeID = undefined;
  }
  function close(restore: boolean) {
    release();
    session.dispose();
    // A dialog hands focus back to its opener itself.
    if (restore && !modal) {
      restoringFocus = true;
      input?.focus();
      restoringFocus = false;
    }
  }
  function resetSelection() { activeID = undefined; limit = 6; navigationError = false; }
  function changeScope(value: SearchScope) { session.setScope(value); resetSelection(); input?.focus(); }
  // A modal, or an editor open beside the page, owns the keyboard: search
  // never opens over it, since choosing a result would leave its work.
  const blocked = () => modalOpen() || editorOpen();
  async function focusSearch() {
    if (blocked() && !open) return;
    phone = window.matchMedia('(max-width: 767px)').matches;
    // A shortcut pressed with nothing focused returns focus to the button.
    if (!open && modal && (!document.activeElement || document.activeElement === document.body)) trigger?.focus();
    open = true;
    await tick();
    input?.focus();
  }
  function shortcut(event: KeyboardEvent) {
    if (shortcuts && searchShortcut(event) && (!blocked() || open)) {
      event.preventDefault(); openPlain();
    }
  }
  // The button and shortcuts start a new ordinary interaction, ending any
  // earlier one (a request included) still waiting on its selection.
  function openPlain() {
    if (!open && !blocked()) session.dispose();
    void focusSearch();
  }
  function request(next: SearchRequest): boolean {
    if (blocked() && !open) return false;
    session.start(next);
    resetSelection();
    void focusSearch();
    return true;
  }
  const owner: Attachment<HTMLElement> = () => {
    apple = applePlatform((navigator as Navigator & { userAgentData?: { platform?: string } }).userAgentData?.platform || navigator.platform);
    return shortcuts ? registerSearchOwner(request) : undefined;
  };
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
    close(false); query = ''; session.reset(); inactive = false; resetSelection();
  });

  async function choose(item: SearchResult) {
    navigationError = false;
    await session.choose(item, {
      // Release the modal before another workflow opens its own focus scope.
      // Completion must not close a later search opened by the user.
      release: () => { release(); },
      act: async (select, chosen) => {
        await tick();
        if (select) await select(chosen);
        else if (chosen.command === 'hide-amounts') setAmountsHidden(!amountPrivacy.hidden);
        else if (chosen.command === 'theme') setTheme(themeState.name === 'dark' ? 'light' : 'dark');
        else if (chosen.command === 'add-transaction') await addTransaction();
        else if (chosen.command === 'add-transfer') addTransfer();
        else if (chosen.href) await goto(chosen.href);
      },
      failed: async () => { open = true; navigationError = true; await tick(); input?.focus(); }
    });
  }
  async function keydown(event: KeyboardEvent) {
    if (event.isComposing) return;
    if (event.key === 'Escape') {
      // In a dialog, Escape is the dialog's cancel.
      if (!modal) { event.preventDefault(); close(true); }
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
  <div class="flex min-w-0 items-center gap-2 rounded-full border border-border bg-control focus-within:ring-2 focus-within:ring-accent {large ? 'px-5' : 'px-3'} {large && !modal ? 'shadow-(--shadow-panel)' : ''}">
    <Search size={large ? 20 : 16} aria-hidden="true" class="shrink-0 text-muted" />
    <input {@attach bindInput} data-initial-focus type="text" role="combobox" aria-label={m.search_label()}
      aria-autocomplete="list" aria-expanded={open} aria-controls={open ? listID : undefined}
      aria-activedescendant={open && active ? `${uid}-${active.id}` : undefined}
      aria-describedby={open ? `${uid}-status` : undefined} autocomplete="off"
      placeholder={m.search_placeholder()} value={query}
      oninput={event => { query = event.currentTarget.value; open = true; resetSelection(); }}
      onfocus={() => { if (!restoringFocus) open = true; }} onkeydown={keydown}
      class="min-w-0 w-full bg-transparent text-foreground outline-none placeholder:text-muted {large ? 'min-h-14 text-base' : 'min-h-11 text-sm'}" />
  </div>
{/snippet}

{#snippet results()}
  <div class="flex flex-wrap gap-1 border-b border-border p-2" aria-label={m.search_scope_label()}>
    {#each ['all', 'account', 'category', 'instrument'] as choice}
      {@const value = choice as SearchScope}
      <button type="button" aria-pressed={session.scope === value} onclick={() => changeScope(value)}
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

<div class="relative min-w-0 {home ? 'w-full' : ''}" {@attach viewport} {@attach owner} {@attach dismissible(() => open && !modal, close)}>
  {#if modal}
    <!-- Home's phone trigger looks like the field it opens; the header's is
         an icon on phones and a Search Ctrl K button from md up. -->
    <button bind:this={trigger} type="button" aria-label={phone ? (home ? m.search_placeholder() : m.search_label()) : undefined}
      aria-haspopup="dialog" aria-keyshortcuts={phone ? undefined : hint.keys}
      aria-disabled={editorOpen() && !open ? 'true' : undefined} title={editorOpen() ? m.search_editor_open() : undefined}
      onclick={openPlain}
      class={home
        ? 'flex min-h-13 w-full items-center gap-3 rounded-full border border-border bg-control px-5 text-left text-muted shadow-(--shadow-panel) hover:bg-control-hover'
        : phone
          ? 'flex min-h-11 min-w-11 items-center justify-center rounded-full text-muted hover:bg-control-hover hover:text-foreground'
          : 'group flex min-h-11 shrink-0 items-center rounded-full'}>
      {#if home}
        <Search size={20} aria-hidden="true" class="shrink-0" />
        <span class="truncate text-base">{m.search_placeholder()}</span>
      {:else if phone}
        <Search size={18} aria-hidden="true" class="shrink-0" />
      {:else}
        <span class="flex h-9 items-center gap-2 rounded-full border border-border bg-control pr-2 pl-3 text-sm text-muted transition group-hover:bg-control-hover group-hover:text-foreground">
          <Search size={16} aria-hidden="true" class="shrink-0" />
          <span>{m.search_label()}</span>
          <kbd class="rounded-sm border border-border px-1.5 font-mono text-xs whitespace-nowrap">{hint.label}</kbd>
        </span>
      {/if}
    </button>
    {#if open}
      {#if phone}
        <ModalDialog label={m.search_label()} onCancel={() => close(false)} class="m-0! h-dvh max-h-dvh! w-full! max-w-none! rounded-none!">
          <div class="flex items-center gap-2 border-b border-border p-3">
            <div class="min-w-0 flex-1">{@render field()}</div>
            <button type="button" aria-label={m.search_close()} onclick={() => close(false)} class="flex min-h-11 min-w-11 items-center justify-center rounded-full text-muted hover:bg-control-hover"><X size={18} aria-hidden="true" /></button>
          </div>
          {@render results()}
        </ModalDialog>
      {:else}
        <!-- Centred across, at a fixed height from the top so the field does
             not move while results load (#490). -->
        <ModalDialog label={m.search_label()} onCancel={() => close(false)} closeOnBackdrop
          class="mt-[12dvh]! mb-auto! max-h-[calc(88dvh-1rem)]! max-w-[45rem]">
          <div class="border-b border-border p-3">{@render field()}</div>
          {@render results()}
        </ModalDialog>
      {/if}
    {/if}
  {:else}
    {@render field()}
    {#if open}
      <div data-search-popup class="absolute inset-x-0 top-full z-50 mt-2 rounded-(--radius-panel) border border-border bg-surface text-left shadow-(--shadow-panel)">
        {@render results()}
      </div>
    {/if}
  {/if}
</div>
