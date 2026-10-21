<script lang="ts">
  import type { Snippet } from 'svelte';
  import Panel from './panel.svelte';

  let {
    title,
    copy,
    alert = false,
    children
  }: {
    title: string;
    copy: string;
    /** An error state: announce the title and copy (not the retry controls). */
    alert?: boolean;
    children?: Snippet;
  } = $props();
</script>

<Panel>
  <div role={alert ? 'alert' : undefined}>
    <p class="text-sm font-semibold text-foreground">{title}</p>
    <p class="mt-2 max-w-3xl text-sm leading-6 text-muted">{copy}</p>
  </div>
  {#if children}
    <div class="mt-5 space-y-4">
      {@render children()}
    </div>
  {/if}
</Panel>
