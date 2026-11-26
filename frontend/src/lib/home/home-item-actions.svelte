<script lang="ts">
  import Ellipsis from '@lucide/svelte/icons/ellipsis';
  import type { Snippet } from 'svelte';
  import { dismissible } from '#lib/navigation/disclosure.ts';
  import { m } from '#lib/paraglide/messages.js';

  let { id, label, children }: { id: string; label: string; children: Snippet<[() => void]> } = $props();
  let open = $state(false);
  let button: HTMLButtonElement;

  function close(restore: boolean) {
    open = false;
    if (restore) button?.focus();
  }
</script>

<div class="relative shrink-0" {@attach dismissible(() => open, close)}>
  <button bind:this={button} {id} type="button" aria-label={m.home_layout_actions({ item: label })}
    aria-expanded={open} aria-controls={open ? `${id}-menu` : undefined} onclick={() => (open = !open)}
    class="inline-grid size-11 place-items-center rounded-full text-muted hover:bg-control-hover">
    <Ellipsis size={18} aria-hidden="true" />
  </button>
  {#if open}
    <div id="{id}-menu" role="group" aria-label={m.home_layout_actions({ item: label })}
      class="absolute right-0 top-full z-20 w-44 max-w-[calc(100vw-2rem)] rounded-(--radius-panel) border border-border bg-surface p-1 text-left text-sm shadow-(--shadow-panel)">
      {@render children(() => close(true))}
    </div>
  {/if}
</div>
