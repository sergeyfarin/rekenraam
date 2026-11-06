<script lang="ts">
  import { tick } from 'svelte';
  import ArrowUpRight from '@lucide/svelte/icons/arrow-up-right';
  import ArrowDownLeft from '@lucide/svelte/icons/arrow-down-left';
  import ArrowLeftRight from '@lucide/svelte/icons/arrow-left-right';
  import ChartLine from '@lucide/svelte/icons/chart-line';
  import type { EntryTab } from './entry-mode.ts';

  // New transaction's four stable tabs (#506). Selection follows focus
  // (arrows, Home, End), which is safe because switching never discards
  // entered work. The mode accent shows only on the selected tab, as its
  // underline, soft fill and icon; the label, the icon's shape and the
  // underline carry the mode without colour. A muted tab is quieter only
  // while unselected and is never disabled.
  let {
    tabs,
    active,
    onSelect,
    panelID,
    idPrefix,
    label,
    disabled = false
  }: {
    /** The four tabs, in entryTabs order. */
    tabs: { id: EntryTab; label: string; muted?: boolean }[];
    active: EntryTab;
    onSelect: (tab: EntryTab) => void;
    panelID: string;
    /** Tab IDs are `${idPrefix}-tab-${tab}`, so the panel can name its tab. */
    idPrefix: string;
    label: string;
    disabled?: boolean;
  } = $props();

  const icons = { out: ArrowUpRight, in: ArrowDownLeft, transfer: ArrowLeftRight, investment: ChartLine };
  // Whole class names, so Tailwind sees each one.
  const selectedClass: Record<EntryTab, string> = {
    out: 'border-entry-out bg-entry-out-soft',
    in: 'border-entry-in bg-entry-in-soft',
    transfer: 'border-entry-transfer bg-entry-transfer-soft',
    investment: 'border-entry-investment bg-entry-investment-soft'
  };
  const iconClass: Record<EntryTab, string> = {
    out: 'text-entry-out',
    in: 'text-entry-in',
    transfer: 'text-entry-transfer',
    investment: 'text-entry-investment'
  };

  function tabID(tab: EntryTab) {
    return `${idPrefix}-tab-${tab}`;
  }

  async function keydown(event: KeyboardEvent) {
    const index = tabs.findIndex((tab) => tab.id === active);
    const last = tabs.length - 1;
    const next =
      event.key === 'ArrowRight' ? (index === last ? 0 : index + 1)
      : event.key === 'ArrowLeft' ? (index === 0 ? last : index - 1)
      : event.key === 'Home' ? 0
      : event.key === 'End' ? last
      : -1;
    if (next < 0) return;
    event.preventDefault();
    onSelect(tabs[next].id);
    await tick();
    document.getElementById(tabID(tabs[next].id))?.focus();
  }
</script>

<div role="tablist" aria-label={label} class="entry-tabs grid grid-cols-4 gap-1 border-b border-border">
  {#each tabs as tab (tab.id)}
    {@const selected = tab.id === active}
    {@const Icon = icons[tab.id]}
    <button
      type="button"
      role="tab"
      id={tabID(tab.id)}
      aria-selected={selected}
      aria-controls={panelID}
      tabindex={selected ? 0 : -1}
      {disabled}
      data-muted={tab.muted && !selected ? '' : undefined}
      onclick={() => onSelect(tab.id)}
      onkeydown={keydown}
      class="-mb-px flex min-h-11 min-w-0 flex-col items-center justify-center gap-1 rounded-t-(--radius-control) border-b-2 px-1 py-1.5 text-xs leading-tight transition hover:bg-control-hover sm:flex-row sm:gap-1.5 sm:px-2 sm:text-sm disabled:cursor-not-allowed
        {selected ? `font-semibold text-foreground ${selectedClass[tab.id]}` : tab.muted ? 'border-transparent font-medium text-muted' : 'border-transparent font-medium text-foreground'}"
    >
      <Icon size={16} aria-hidden="true" class="shrink-0 {selected ? iconClass[tab.id] : ''}" />
      <span class="min-w-0 text-center break-words">{tab.label}</span>
    </button>
  {/each}
</div>

<style>
  /* Forced colours drop fills and tints; the selected tab keeps a system
     highlight underline, and a muted tab a dashed one, so neither depends on
     colour (#506). */
  @media (forced-colors: active) {
    .entry-tabs [aria-selected='true'] {
      border-bottom-color: Highlight;
    }
    .entry-tabs [data-muted] {
      border-bottom: 2px dashed GrayText;
    }
  }
</style>
