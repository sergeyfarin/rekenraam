<script lang="ts">
  import ChartPie from '@lucide/svelte/icons/chart-pie';
  import Ellipsis from '@lucide/svelte/icons/ellipsis';
  import FileUp from '@lucide/svelte/icons/file-up';
  import List from '@lucide/svelte/icons/list';
  import ArrowLeftRight from '@lucide/svelte/icons/arrow-left-right';
  import Plus from '@lucide/svelte/icons/plus';
  import { dismissible } from '#lib/navigation/disclosure.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { addTransaction, addTransfer } from '#lib/transactions/add-transaction.svelte.ts';
  import { homeShortcuts, phoneShortcutCount, splitShortcuts, type ShortcutID } from './shortcuts.ts';

  // Home's centred row of tiles (#477). Phones show the first three and list
  // the rest behind More, so every shortcut, Add included, stays reachable.
  const shortcuts = $derived(homeShortcuts());
  const more = $derived(splitShortcuts(shortcuts).more);
  const icons = { add: Plus, transfer: ArrowLeftRight, import: FileUp, transactions: List, spending: ChartPie } satisfies Record<ShortcutID, unknown>;
  const open = (id: ShortcutID) => (id === 'transfer' ? addTransfer() : addTransaction());

  let moreOpen = $state(false);
  let moreButton = $state<HTMLButtonElement>();

  function closeMore(restoreFocus: boolean) {
    moreOpen = false;
    if (restoreFocus) moreButton?.focus();
  }

  // The dialog hands focus back to More, which stays on the page.
  function openFromMore(id: ShortcutID) {
    closeMore(true);
    open(id);
  }

  const tileClass = 'flex min-h-25 w-full flex-col items-center gap-2 rounded-(--radius-panel) px-1.5 pt-3 pb-2.5 text-center text-sm text-foreground transition hover:bg-control-hover';
  const iconClass = 'grid size-13 shrink-0 place-items-center rounded-(--radius-panel) border';
  const menuItemClass = 'flex min-h-11 w-full items-center rounded-(--radius-control) px-3 text-left text-sm text-foreground hover:bg-control-hover';
</script>

{#snippet tile(id: ShortcutID, label: string)}
  {@const Icon = icons[id]}
  <span class="{iconClass} {id === 'add' ? 'border-accent bg-accent text-accent-foreground' : 'border-border bg-surface text-accent'}">
    <Icon size={22} aria-hidden="true" />
  </span>
  <span class="line-clamp-2 leading-tight">{label}</span>
{/snippet}

<ul aria-label={m.home_shortcuts_label()} class="mx-auto mt-6 flex max-w-[56rem] justify-center gap-1.5 max-md:gap-0">
  {#each shortcuts as shortcut, index (shortcut.id)}
    <li class="w-24 max-md:w-1/4 {index >= phoneShortcutCount ? 'max-md:hidden' : ''}">
      {#if shortcut.href}
        <a href={shortcut.href} class={tileClass}>{@render tile(shortcut.id, shortcut.label)}</a>
      {:else}
        <button type="button" onclick={() => open(shortcut.id)} class={tileClass}>{@render tile(shortcut.id, shortcut.label)}</button>
      {/if}
    </li>
  {/each}
  {#if more.length}
    <li class="relative w-1/4 md:hidden" {@attach dismissible(() => moreOpen, closeMore)}>
      <button bind:this={moreButton} type="button" aria-expanded={moreOpen} aria-controls="home-shortcuts-more" onclick={() => (moreOpen = !moreOpen)} class={tileClass}>
        <span class="{iconClass} border-border bg-surface text-muted"><Ellipsis size={22} aria-hidden="true" /></span>
        <span class="line-clamp-2 leading-tight">{m.home_shortcut_more()}</span>
      </button>
      {#if moreOpen}
        <div id="home-shortcuts-more" role="group" aria-label={m.home_shortcut_more()} class="absolute right-0 top-full z-30 mt-1 w-56 max-w-[calc(100vw-2rem)] rounded-(--radius-panel) border border-border bg-surface p-1 shadow-(--shadow-panel)">
          {#each more as shortcut (shortcut.id)}
            {#if shortcut.href}
              <a href={shortcut.href} class={menuItemClass}>{shortcut.label}</a>
            {:else}
              <button type="button" onclick={() => openFromMore(shortcut.id)} class={menuItemClass}>{shortcut.label}</button>
            {/if}
          {/each}
        </div>
      {/if}
    </li>
  {/if}
</ul>
