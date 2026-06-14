<script lang="ts">
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import type { Snippet } from 'svelte';

  let {
    depth = 0,
    expandable = false,
    expanded = false,
    expandLabel,
    collapseLabel,
    onToggle,
    children
  } = $props<{
    depth?: number;
    expandable?: boolean;
    expanded?: boolean;
    expandLabel: string;
    collapseLabel: string;
    onToggle: () => void;
    children?: Snippet;
  }>();

  const boundedDepth = $derived(Math.max(0, Math.min(depth, 6)));
  const toggleLabel = $derived(expanded ? collapseLabel : expandLabel);
</script>

<div class="min-w-0" style:padding-left={`${boundedDepth * 1.25}rem`}>
  <div class="flex min-w-0 items-start gap-2">
    {#if expandable}
      <button
        type="button"
        class="mt-0.5 inline-flex size-6 shrink-0 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-muted transition hover:bg-control-hover hover:text-foreground"
        aria-expanded={expanded}
        aria-label={toggleLabel}
        title={toggleLabel}
        onclick={onToggle}
      >
        {#if expanded}
          <ChevronDown size={14} aria-hidden="true" />
        {:else}
          <ChevronRight size={14} aria-hidden="true" />
        {/if}
      </button>
    {:else}
      <span class="size-6 shrink-0" aria-hidden="true"></span>
    {/if}

    <div class="min-w-0">
      {#if children}
        {@render children()}
      {/if}
    </div>
  </div>
</div>
