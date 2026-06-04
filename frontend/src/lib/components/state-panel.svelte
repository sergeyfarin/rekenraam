<script lang="ts">
  import type { Snippet } from 'svelte';
  import Panel from './panel.svelte';

  let {
    title,
    copy,
    children
  } = $props<{
    title: string;
    copy: string;
    children?: Snippet;
  }>();
</script>

<Panel>
  <p class="text-sm font-semibold text-foreground">{title}</p>
  <p class="mt-2 max-w-3xl text-sm leading-6 text-muted">{copy}</p>
  {#if children}
    <div class="mt-5 space-y-4">
      {@render children()}
    </div>
  {/if}
</Panel>
