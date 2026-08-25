<script lang="ts">
  import { joinCommodityAmount } from '$lib/money/format';
  import { untrack } from 'svelte';
  import { createInfiniteQuery } from '@tanstack/svelte-query';
  import { m } from '$lib/paraglide/messages.js';
  import { getLocale } from '$lib/paraglide/runtime.js';
  import Panel from '$lib/components/panel.svelte';
  import StatusBadge from '$lib/components/status-badge.svelte';
  import {
    transactionsInfiniteQueryOptions,
    moveTransaction,
    type TransactionResponse
  } from '$lib/api/transactions';
  import {
    formatSignedAmount,
    resolveAccountLabel,
    statusLabel,
    statusTone,
    commodityDisplay
  } from './transaction-labels';
  import TransactionTable from './transaction-table.svelte';
  import TransactionFilterBar from './transaction-filter-bar.svelte';
  import TransactionRowActions from './transaction-row-actions.svelte';
  import type { TransactionFilters } from './transaction-filter-bar.svelte';
  import type { TransactionQueryFilters } from './transaction-url-filters';
  import type { Column } from './transaction-table-types';

  let {
    csrfToken,
    initialFilters,
    onFiltersChange,
    onRowClick,
    onEdit,
    onCreateCorrection,
    onApprove,
    onUnvoid
  }: {
    csrfToken?: string;
    /** Seeds the bar from the URL, so a drill-down link lands pre-filtered. */
    initialFilters?: TransactionQueryFilters;
    /** Reports each change back so the route can keep the URL in step. */
    onFiltersChange?: (filters: TransactionQueryFilters) => void;
    onRowClick?: (tx: TransactionResponse) => void;
    onEdit?: (tx: TransactionResponse) => void;
    onCreateCorrection?: (tx: TransactionResponse) => void;
    onApprove?: (tx: TransactionResponse) => void;
    onUnvoid?: (tx: TransactionResponse) => void;
  } = $props();

  // Filters the bar cannot set — a drill-down's category and date basis — are
  // held separately so the bar's own edits never silently drop them.
  // `untrack` states the intent the warning asks about: the URL seeds this list
  // once. After that the bar owns the filters and the route follows it.
  const seed = untrack(() => initialFilters);
  let linkFilters = $state<TransactionQueryFilters>({
    categoryID: seed?.categoryID,
    categoryType: seed?.categoryType,
    dateBasis: seed?.dateBasis
  });
  const fromReport = $derived(linkFilters.categoryID !== undefined || linkFilters.categoryType !== undefined);
  let filters = $state<TransactionFilters>({
    status: seed?.status,
    kind: seed?.kind,
    accountID: seed?.accountID,
    payeeID: seed?.payeeID,
    q: seed?.q,
    needsReview: seed?.needsReview,
    afterDate: seed?.afterDate,
    beforeDate: seed?.beforeDate
  });
  const activeFilters = $derived<TransactionQueryFilters>({ ...linkFilters, ...filters });

  $effect(() => {
    onFiltersChange?.(activeFilters);
  });
  let movePendingID = $state<number | undefined>(undefined);
  let moveError = $state<unknown>(undefined);

  const locale = $derived(getLocale());

  const query = createInfiniteQuery(() =>
    transactionsInfiniteQueryOptions(activeFilters)
  );

  const rows = $derived(
    query.data?.pages.flatMap((p) => p.transactions ?? []) ?? []
  );

  // Pick the primary posting: first asset/liability leg, or fall back to first.
  function primaryPosting(tx: TransactionResponse) {
    const postings = tx.journal_entries?.flatMap((e) => e.postings ?? []) ?? [];
    return (
      postings.find((p) => p.account_class === 'asset' || p.account_class === 'liability') ??
      postings[0]
    );
  }

  // Unique visible account labels for the Account column.
  function accountsLabel(tx: TransactionResponse): string {
    const postings = tx.journal_entries?.flatMap((e) => e.postings ?? []) ?? [];
    const visible = postings.filter((p) => !p.account_system_role);
    const labels = [...new Set(visible.map(resolveAccountLabel))];
    if (labels.length === 0) return '—';
    if (labels.length <= 2) return labels.join(', ');
    return m.transactions_detail_n_accounts({ n: labels.length });
  }

  async function handleMove(tx: TransactionResponse, direction: 'earlier' | 'later') {
    if (!csrfToken) return;
    movePendingID = tx.id;
    moveError = undefined;
    try {
      await moveTransaction(tx.id, direction, csrfToken);
      await query.refetch();
    } catch (e) {
      moveError = e;
    } finally {
      movePendingID = undefined;
    }
  }
</script>

<div class="space-y-4">
  {#if fromReport}
    <!-- A drill-down narrows the list by a category and date basis the filter
         bar cannot show. Saying so, and offering the way out, is the difference
         between a filtered list and a list that looks broken. -->
    <Panel variant="toolbar">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <p class="text-sm text-foreground">{m.transactions_from_report()}</p>
        <button
          type="button"
          class="text-sm font-semibold text-accent underline underline-offset-2 transition hover:opacity-80"
          onclick={() => (linkFilters = {})}
        >
          {m.transactions_from_report_clear()}
        </button>
      </div>
    </Panel>
  {/if}

  <Panel variant="toolbar">
    <TransactionFilterBar
      bind:filters
      show={{ status: true, kind: true, account: true, payee: true, search: true, dateRange: true, needsReview: true }}
    />
  </Panel>

  {#snippet dateCell(tx: TransactionResponse)}
    <span class="tabular-nums text-muted">{tx.transaction_date}</span>
  {/snippet}

  {#snippet payeeCell(tx: TransactionResponse)}
    <span class="font-medium text-foreground">{tx.payee_name || tx.description || '—'}</span>
    {#if tx.needs_review}
      <span class="ml-2 inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider ring-1 ring-accent/40 text-accent">
        {m.transactions_needs_review()}
      </span>
    {/if}
  {/snippet}

  {#snippet amountCell(tx: TransactionResponse)}
    {@const p = primaryPosting(tx)}
    {#if p}
      <span class={`font-medium tabular-nums ${p.quantity_value.startsWith('-') ? 'text-danger' : 'text-foreground'}`}>
        {joinCommodityAmount(
          commodityDisplay(p),
          formatSignedAmount(p, p.account_class as import('./transaction-labels').AccountClass, locale)
        )}
      </span>
    {:else}
      <span class="text-muted">—</span>
    {/if}
  {/snippet}

  {#snippet accountCell(tx: TransactionResponse)}
    <span class="text-sm text-muted">{accountsLabel(tx)}</span>
  {/snippet}

  {#snippet statusCell(tx: TransactionResponse)}
    <StatusBadge tone={statusTone(tx.status)}>{statusLabel(tx.status)}</StatusBadge>
  {/snippet}

  {#snippet actionsCell(tx: TransactionResponse)}
    <div class="flex items-center justify-end gap-1">
      <!-- Same-day Move up/down -->
      <button
        type="button"
        class="inline-flex h-7 w-7 items-center justify-center rounded text-muted hover:bg-surface-strong hover:text-foreground disabled:opacity-30"
        title={m.transactions_action_view()}
        disabled={movePendingID === tx.id}
        onclick={(e) => { e.stopPropagation(); handleMove(tx, 'earlier'); }}
        aria-label="Move earlier"
      >
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M18 15l-6-6-6 6"/>
        </svg>
      </button>
      <button
        type="button"
        class="inline-flex h-7 w-7 items-center justify-center rounded text-muted hover:bg-surface-strong hover:text-foreground disabled:opacity-30"
        disabled={movePendingID === tx.id}
        onclick={(e) => { e.stopPropagation(); handleMove(tx, 'later'); }}
        aria-label="Move later"
      >
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M6 9l6 6 6-6"/>
        </svg>
      </button>
      <TransactionRowActions
        transaction={tx}
        onEdit={onEdit}
        onCreateCorrection={onCreateCorrection}
        onApprove={onApprove}
        onUnvoid={onUnvoid}
        onView={onRowClick}
      />
    </div>
  {/snippet}

  <TransactionTable
    {rows}
    rowId={(tx) => String(tx.id)}
    columns={[
      { key: 'date', header: m.transactions_detail_date(), priority: 1, width: '9rem', cell: dateCell },
      { key: 'payee', header: m.transactions_detail_payee(), priority: 1, cell: payeeCell },
      { key: 'amount', header: m.transactions_detail_amount(), priority: 1, align: 'right', width: '10rem', cell: amountCell },
      { key: 'account', header: m.transactions_detail_account(), priority: 2, cell: accountCell },
      { key: 'status', header: m.transactions_filter_status(), priority: 3, width: '7rem', cell: statusCell },
      { key: 'actions', header: '', priority: 1, width: '12rem', align: 'right', cell: actionsCell }
    ]}
    isLoading={query.isPending}
    isFetchingNextPage={query.isFetchingNextPage}
    hasNextPage={query.hasNextPage}
    onLoadMore={() => { if (!query.isFetchingNextPage) query.fetchNextPage(); }}
    error={query.error instanceof Error ? query.error : query.error ? new Error(String(query.error)) : null}
    onRetry={() => query.refetch()}
    onRowClick={onRowClick}
  />
</div>
