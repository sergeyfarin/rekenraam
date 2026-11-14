<script lang="ts">
  import { createInfiniteQuery } from '@tanstack/svelte-query';
  import { importHistoryQueryKey, listImportBatches } from '#lib/api/imports.ts';
  import Panel from '#lib/components/panel.svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { m } from '#lib/paraglide/messages.js';
  let { onSelect }: { onSelect: (id: number) => void } = $props();
  const query = createInfiniteQuery(() => ({
    queryKey: importHistoryQueryKey, initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) => listImportBatches({ cursor: pageParam }),
    getNextPageParam: last => last.next_cursor ?? undefined
  }));
  const batches = $derived(query.data?.pages.flatMap(page => page.batches) ?? []);
  const status = {
    previewing: m.import_history_status_previewing,
    committed: m.import_history_status_committed,
    partially_committed: m.import_history_status_partially_committed,
    discarded: m.import_history_status_discarded,
    failed: m.import_history_status_failed
  };
</script>
<div class="mt-6 max-w-2xl">
  <Panel>
    <h2 class="text-sm font-semibold">{m.import_history_title()}</h2>
    {#if query.isPending}<p role="status" class="mt-3 text-sm text-muted">{m.import_dividend_loading()}</p>
    {:else if query.isError}<APIFormError error={query.error} /><button type="button" class="min-h-11 underline" onclick={() => query.refetch()}>{m.install_gate_retry()}</button>
    {:else if batches.length === 0}<p class="mt-3 text-sm text-muted">{m.import_history_empty()}</p>
    {:else}
      <ul class="mt-3 divide-y divide-border">
        {#each batches as batch (batch.id)}
          <li><button type="button" class="flex min-h-11 w-full flex-wrap items-center justify-between gap-2 py-2 text-left text-sm" onclick={() => onSelect(batch.id)}>
            <span class="min-w-0 wrap-anywhere font-semibold underline">{batch.original_filename || String(batch.id)}</span>
            <span class="text-muted">{status[batch.status]()}</span>
          </button></li>
        {/each}
      </ul>
    {/if}
    {#if query.hasNextPage}<button type="button" class="min-h-11 text-sm underline" disabled={query.isFetchingNextPage} onclick={() => query.fetchNextPage()}>{m.import_dividend_more()}</button>{/if}
  </Panel>
</div>
