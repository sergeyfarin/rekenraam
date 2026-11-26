<script lang="ts">
  import ChartPie from '@lucide/svelte/icons/chart-pie';
  import Ellipsis from '@lucide/svelte/icons/ellipsis';
  import FileUp from '@lucide/svelte/icons/file-up';
  import List from '@lucide/svelte/icons/list';
  import ListChecks from '@lucide/svelte/icons/list-checks';
  import Landmark from '@lucide/svelte/icons/landmark';
  import ArrowLeftRight from '@lucide/svelte/icons/arrow-left-right';
  import Plus from '@lucide/svelte/icons/plus';
  import { tick } from 'svelte';
  import type { HomeLayoutState } from '#lib/api/home.ts';
  import { dismissible } from '#lib/navigation/disclosure.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { addTransaction, addTransfer } from '#lib/transactions/add-transaction.svelte.ts';
  import HomeItemActions from './home-item-actions.svelte';
  import { focusHomeControl, moveHomeItem } from './layout-model.ts';
  import { useHomePersonalization } from './personalization.svelte.ts';
  import { homeShortcuts, phoneShortcutCount, splitShortcuts, type Shortcut, type ShortcutID } from './shortcuts.ts';

  let { state: layoutState }: { state?: HomeLayoutState } = $props();
  const personalization = useHomePersonalization();
  const shortcuts = $derived(homeShortcuts(layoutState?.layout.shortcuts, layoutState?.accounts));
  const more = $derived(splitShortcuts(shortcuts).more);
  const icons = { add: Plus, transfer: ArrowLeftRight, import: FileUp, transactions: List, spending: ChartPie, account: Landmark, reconcile: ListChecks };
  const open = (id: ShortcutID) => (id === 'transfer' ? addTransfer() : addTransaction());

  let moreOpen = $state(false);
  let moreButton: HTMLButtonElement;

  function closeMore(restoreFocus: boolean) {
    moreOpen = false;
    if (restoreFocus) moreButton?.focus();
  }

  function openFromMore(id: ShortcutID) {
    closeMore(true);
    open(id);
  }

  async function edit(shortcut: Shortcut, offset?: number) {
    if (!layoutState) return;
    const items = layoutState.layout.shortcuts;
    const from = items.findIndex(item => item.id === shortcut.id);
    const to = from + (offset ?? 0);
    const next = offset === undefined ? items.filter(item => item.id !== shortcut.id) : moveHomeItem(items, from, to);
    const result = await personalization.save(layoutState, { ...layoutState.layout, shortcuts: next });
    if (result.status !== 'saved') return;
    await tick();
    if (offset === undefined) {
      if (moreOpen) moreButton?.focus();
      else if (next.length) focusHomeControl(`home-shortcut-actions-${next[Math.min(from, next.length - 1)].id}`);
      return;
    }
    if (to >= phoneShortcutCount && moreButton?.offsetParent) {
      moreOpen = true;
      await tick();
      focusHomeControl(`more-shortcut-actions-${shortcut.id}`);
    } else {
      focusHomeControl(`home-shortcut-actions-${shortcut.id}`);
    }
    const format = new Intl.NumberFormat(getLocale());
    personalization.announce(m.home_layout_moved({ item: shortcut.label, position: format.format(to + 1), total: format.format(next.length) }));
  }

  const tileClass = 'flex min-h-25 w-full flex-col items-center gap-2 rounded-(--radius-panel) px-1.5 pt-3 pb-2.5 text-center text-sm text-foreground transition hover:bg-control-hover';
  const iconClass = 'grid size-13 shrink-0 place-items-center rounded-(--radius-panel) border';
  const menuItemClass = 'flex min-h-11 min-w-0 flex-1 items-center rounded-(--radius-control) px-3 text-left text-sm text-foreground hover:bg-control-hover';
  const actionClass = 'flex min-h-11 w-full items-center rounded-(--radius-control) px-2 text-left hover:bg-control-hover disabled:opacity-50';
</script>

{#snippet tile(Icon: typeof Plus, label: string, primary = false)}
  <span class="{iconClass} {primary ? 'border-accent bg-accent text-accent-foreground' : 'border-border bg-surface text-accent'}">
    <Icon size={22} aria-hidden="true" />
  </span>
  <span class="line-clamp-2 leading-tight wrap-anywhere">{label}</span>
{/snippet}

{#snippet actions(shortcut: Shortcut, index: number, inMore = false)}
  <HomeItemActions id="{inMore ? 'more' : 'home'}-shortcut-actions-{shortcut.id}" label={shortcut.label}>
    {#snippet children(close)}
      <button type="button" class={actionClass} disabled={personalization.pending || index === 0} onclick={() => { close(); void edit(shortcut, -1); }}>{m.home_layout_move_left()}</button>
      <button type="button" class={actionClass} disabled={personalization.pending || index === shortcuts.length - 1} onclick={() => { close(); void edit(shortcut, 1); }}>{m.home_layout_move_right()}</button>
      <button type="button" class={actionClass} onclick={() => { close(); if (inMore) closeMore(true); personalization.open('shortcuts'); }}>{m.home_layout_rename()}</button>
      {#if shortcut.id !== 'add'}
        <button type="button" class={actionClass} disabled={personalization.pending} onclick={() => { close(); void edit(shortcut); }}>{m.home_layout_remove()}</button>
      {/if}
    {/snippet}
  </HomeItemActions>
{/snippet}

<ul aria-label={m.home_shortcuts_label()} class="mx-auto mt-6 flex max-w-[56rem] justify-center gap-1.5 max-md:gap-0">
  {#each shortcuts as shortcut, index (shortcut.id)}
    <li class="flex min-w-0 w-24 flex-col items-center max-md:w-1/4 {index >= phoneShortcutCount ? 'max-md:hidden' : ''}">
      {#if shortcut.href}
        <a href={shortcut.href} class={tileClass}>{@render tile(icons[shortcut.id], shortcut.label)}</a>
      {:else}
        <button type="button" onclick={() => open(shortcut.id)} class={tileClass}>{@render tile(icons[shortcut.id], shortcut.label, shortcut.id === 'add')}</button>
      {/if}
      {#if layoutState}{@render actions(shortcut, index)}{/if}
    </li>
  {/each}
  <li class="w-24 max-md:hidden">
    <button type="button" onclick={() => personalization.open('shortcuts')} class={tileClass}>
      {@render tile(Plus, m.home_layout_add_shortcut())}
    </button>
  </li>
  <li class="relative w-1/4 md:hidden" {@attach dismissible(() => moreOpen, closeMore)}>
    <button bind:this={moreButton} type="button" aria-expanded={moreOpen} aria-controls={moreOpen ? 'home-shortcuts-more' : undefined}
      onclick={() => (moreOpen = !moreOpen)} class={tileClass}>
      {@render tile(Ellipsis, m.home_shortcut_more())}
    </button>
    {#if moreOpen}
      <div id="home-shortcuts-more" role="group" aria-label={m.home_shortcut_more()}
        class="absolute right-0 top-full z-30 mt-1 w-56 max-w-[calc(100vw-2rem)] rounded-(--radius-panel) border border-border bg-surface p-1 shadow-(--shadow-panel)">
        {#each more as shortcut, index (shortcut.id)}
          <div class="flex min-w-0 items-center">
            {#if shortcut.href}
              <a href={shortcut.href} class="{menuItemClass} wrap-anywhere">{shortcut.label}</a>
            {:else}
              <button type="button" onclick={() => openFromMore(shortcut.id)} class="{menuItemClass} wrap-anywhere">{shortcut.label}</button>
            {/if}
            {#if layoutState}{@render actions(shortcut, index + phoneShortcutCount, true)}{/if}
          </div>
        {/each}
        <button type="button" class={actionClass} onclick={() => { closeMore(true); personalization.open(); }}>{m.home_customize()}</button>
      </div>
    {/if}
  </li>
</ul>
