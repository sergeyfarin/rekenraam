<script lang="ts" generics="R">
  import StatePanel from '$lib/components/state-panel.svelte';
  import { m } from '$lib/paraglide/messages.js';
  import type { Column } from './transaction-table-types';

  let {
    rows,
    columns,
    isLoading = false,
    isFetchingNextPage = false,
    hasNextPage = false,
    onLoadMore,
    error = null,
    onRetry,
    onRowClick
  }: {
    rows: R[];
    columns: Column<R>[];
    isLoading?: boolean;
    isFetchingNextPage?: boolean;
    hasNextPage?: boolean;
    onLoadMore: () => void;
    error?: Error | null;
    onRetry?: () => void;
    onRowClick?: (row: R) => void;
  } = $props();

  let sentinel: HTMLDivElement | undefined = $state();
  let focusedRowIndex = $state(-1);

  // IntersectionObserver for infinite scroll — never fires while a fetch is in flight.
  $effect(() => {
    if (!sentinel) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting && hasNextPage && !isFetchingNextPage) {
          onLoadMore();
        }
      },
      { rootMargin: '200px' }
    );
    observer.observe(sentinel);
    return () => observer.disconnect();
  });

  function handleRowKeydown(e: KeyboardEvent, index: number, row: R) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      focusedRowIndex = Math.min(index + 1, rows.length - 1);
      focusRow(focusedRowIndex);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      focusedRowIndex = Math.max(index - 1, 0);
      focusRow(focusedRowIndex);
    } else if (e.key === 'Enter' && onRowClick) {
      e.preventDefault();
      onRowClick(row);
    }
  }

  function focusRow(index: number) {
    const tableEl = document.querySelector('[data-transaction-table]');
    if (!tableEl) return;
    const rows = tableEl.querySelectorAll<HTMLElement>('[data-table-row]');
    rows[index]?.focus();
  }

  const PRIORITY_CLASSES: Record<number, string> = {
    1: '',
    2: 'hidden min-[600px]:table-cell',
    3: 'hidden min-[900px]:table-cell'
  };
</script>

<div class="flex flex-col overflow-hidden" data-transaction-table>
  {#if isLoading && rows.length === 0}
    <!-- Initial loading skeleton -->
    <div class="overflow-x-auto">
      <table class="w-full border-collapse text-sm">
        <thead>
          <tr class="sticky top-0 z-10 border-b border-border bg-surface">
            {#each columns as col (col.key)}
              <th
                class={`px-3 py-2.5 text-left text-xs font-semibold uppercase tracking-widest text-muted ${PRIORITY_CLASSES[col.priority ?? 1]}`}
                style={col.width ? `width: ${col.width}` : undefined}
              >
                {col.header}
              </th>
            {/each}
          </tr>
        </thead>
        <tbody>
          {#each { length: 6 } as _, i (i)}
            <tr class="border-b border-border/50">
              {#each columns as col (col.key)}
                <td class={`px-3 py-3 ${PRIORITY_CLASSES[col.priority ?? 1]}`}>
                  <div class="h-4 animate-pulse rounded bg-surface-strong"></div>
                </td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {:else if error}
    <div class="p-4">
      <StatePanel title={m.transactions_error_title()} copy={m.transactions_error_copy()}>
        {#if onRetry}
          <button
            type="button"
            onclick={onRetry}
            class="rounded-(--radius-control) bg-accent px-4 py-2 text-sm font-semibold text-accent-foreground hover:opacity-90"
          >
            {m.transactions_retry()}
          </button>
        {/if}
      </StatePanel>
    </div>
  {:else if rows.length === 0}
    <div class="p-4">
      <StatePanel title={m.transactions_empty_title()} copy={m.transactions_empty_copy()} />
    </div>
  {:else}
    <div class="overflow-x-auto">
      <table class="w-full border-collapse text-sm">
        <thead>
          <tr class="sticky top-0 z-10 border-b border-border bg-surface">
            {#each columns as col (col.key)}
              <th
                class={`px-3 py-2.5 text-xs font-semibold uppercase tracking-widest text-muted ${col.align === 'right' ? 'text-right' : 'text-left'} ${PRIORITY_CLASSES[col.priority ?? 1]}`}
                style={col.width ? `width: ${col.width}` : undefined}
              >
                {col.header}
              </th>
            {/each}
          </tr>
        </thead>
        <tbody>
          {#each rows as row, i (i)}
            {@const isClickable = !!onRowClick}
            <tr
              data-table-row
              tabindex={isClickable ? 0 : undefined}
              role={isClickable ? 'button' : undefined}
              aria-selected={focusedRowIndex === i}
              class={`border-b border-border/50 outline-none transition-colors ${isClickable ? 'cursor-pointer hover:bg-row-hover focus:bg-row-hover' : ''}`}
              onclick={() => onRowClick?.(row)}
              onkeydown={(e) => handleRowKeydown(e, i, row)}
            >
              {#each columns as col (col.key)}
                <td
                  class={`px-3 py-3 ${col.align === 'right' ? 'text-right' : 'text-left'} ${PRIORITY_CLASSES[col.priority ?? 1]}`}
                >
                  {@render col.cell(row)}
                </td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <!-- Infinite scroll sentinel + fallback button -->
    <div bind:this={sentinel} class="h-px w-full"></div>

    {#if hasNextPage}
      <div class="flex justify-center py-4">
        <button
          type="button"
          onclick={() => { if (!isFetchingNextPage) onLoadMore(); }}
          disabled={isFetchingNextPage}
          class="rounded-(--radius-control) border border-border px-4 py-2 text-sm font-medium text-foreground hover:bg-surface-strong disabled:opacity-50"
        >
          {#if isFetchingNextPage}
            <span class="flex items-center gap-2">
              <span class="h-4 w-4 animate-spin rounded-full border-2 border-border border-t-accent"></span>
              {m.transactions_loading_title()}
            </span>
          {:else}
            {m.transactions_load_more()}
          {/if}
        </button>
      </div>
    {/if}
  {/if}
</div>
