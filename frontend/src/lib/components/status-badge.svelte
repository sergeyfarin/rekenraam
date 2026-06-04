<script lang="ts">
  import type { Snippet } from 'svelte';

  type Tone = 'neutral' | 'accent' | 'danger' | 'positive' | 'warning';

  let {
    tone = 'neutral',
    children
  } = $props<{
    tone?: Tone;
    children?: Snippet;
  }>();
</script>

<span
  class:bg-surface-strong={tone === 'neutral'}
  class:text-muted={tone === 'neutral'}
  class:status-accent-soft={tone === 'accent'}
  class:text-accent={tone === 'accent'}
  class:bg-danger-soft={tone === 'danger'}
  class:text-danger={tone === 'danger'}
  class:bg-positive-soft={tone === 'positive'}
  class:text-positive={tone === 'positive'}
  class:bg-warning-soft={tone === 'warning'}
  class:text-warning={tone === 'warning'}
  class="inline-flex min-h-6 items-center rounded-[var(--radius-control)] px-2.5 py-1 text-xs font-semibold"
>
  {#if children}
    {@render children()}
  {/if}
</span>
