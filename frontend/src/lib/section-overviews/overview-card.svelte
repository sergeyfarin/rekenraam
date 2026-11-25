<script lang="ts">
  import type { Snippet } from 'svelte';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import Panel from '#lib/components/panel.svelte';
  import { m } from '#lib/paraglide/messages.js';

  let { title, href, failed = false, retry, children }: {
    title: string;
    href: string;
    failed?: boolean;
    retry?: () => void;
    children?: Snippet;
  } = $props();
</script>

<Panel padding="sm">
  <h2 class="text-lg font-semibold text-foreground">
    <a {href} class="flex min-h-11 items-center justify-between gap-2 rounded-(--radius-control) underline-offset-4 hover:underline">
      {title}
      <ChevronRight size={18} aria-hidden="true" class="shrink-0 text-muted" />
    </a>
  </h2>
  {#if failed}
    <p role="alert" class="text-sm text-muted">{m.overview_source_failed()}</p>
    <button type="button" class="min-h-11 text-sm font-medium underline" onclick={retry}>{m.reports_retry()}</button>
  {:else if children}
    <div class="mt-2 space-y-2 text-sm text-muted">
      {@render children()}
    </div>
  {/if}
</Panel>
