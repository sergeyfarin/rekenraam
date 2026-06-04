<script lang="ts">
  import type { Snippet } from 'svelte';

  type Padding = 'none' | 'sm' | 'md';
  type Variant = 'default' | 'toolbar' | 'subtle';

  let {
    padding = 'md',
    variant = 'default',
    children
  } = $props<{
    padding?: Padding;
    variant?: Variant;
    children?: Snippet;
  }>();

  const paddingClass = $derived.by(() => {
    switch (padding) {
      case 'none':
        return '';
      case 'sm':
        return 'p-4 sm:p-5';
      default:
        return 'p-5 sm:p-6';
    }
  });

  const variantClass = $derived.by(() => {
    switch (variant) {
      case 'toolbar':
        return 'bg-toolbar';
      case 'subtle':
        return 'bg-surface-strong/55';
      default:
        return 'bg-surface';
    }
  });
</script>

<section class={`rounded-[var(--radius-panel)] border border-border shadow-[var(--shadow-panel)] ${variantClass} ${paddingClass}`}>
  {#if children}
    {@render children()}
  {/if}
</section>
