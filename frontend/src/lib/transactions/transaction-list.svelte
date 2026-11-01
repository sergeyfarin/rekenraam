<script lang="ts">
  import { otherPostingDays } from './posted-on';
  import { transactionTitle } from '#lib/transactions/transaction-title.ts';
  import { systemLabelText } from '#lib/transactions/system-label.ts';
  import { joinCommodityAmount } from '#lib/money/format.ts';
  import { untrack } from 'svelte';
  import { createInfiniteQuery, useQueryClient } from '@tanstack/svelte-query';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import Panel from '#lib/components/panel.svelte';
  import StatusBadge from '#lib/components/status-badge.svelte';
  import {
    transactionsInfiniteQueryOptions,
    placeTransaction,
    type TransactionResponse
  } from '#lib/api/transactions.ts';
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
  import ReorderBar from './reorder-bar.svelte';
  import ReorderArrows from './reorder-arrows.svelte';
  import { ReorderMode } from './reorder-mode.svelte.ts';
  import { placePosition, sameDayNeighbor, type ReorderDirection } from './reorder';
  import type { TransactionFilters } from './transaction-filter-bar.svelte';
  import type { TransactionQueryFilters } from './transaction-url-filters';
  import type { Column } from './transaction-table-types';

  import type { TransactionWorkspaceActions } from './transaction-workspace.svelte';

  let {
    csrfToken,
    actions,
    initialFilters,
    onFiltersChange,
    onRowClick,
    lockStatus,
    emptyTitle,
    emptyCopy
  }: {
    csrfToken?: string;
    actions: TransactionWorkspaceActions;
    /** Seeds the bar from the URL, so a drill-down link lands pre-filtered. */
    initialFilters?: TransactionQueryFilters;
    /** Reports each change back so the route can keep the URL in step. */
    onFiltersChange?: (filters: TransactionQueryFilters) => void;
    onRowClick?: (tx: TransactionResponse) => void;
    /** A view that owns lifecycle, such as Drafts, hides the status filter. */
    lockStatus?: TransactionFilters['status'];
    emptyTitle?: string;
    emptyCopy?: string;
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
    status: untrack(() => lockStatus ?? seed?.status),
    kind: seed?.kind,
    accountID: seed?.accountID,
    payeeID: seed?.payeeID,
    q: seed?.q,
    needsReview: seed?.needsReview,
    afterDate: seed?.afterDate,
    beforeDate: seed?.beforeDate
  });
  const activeFilters = $derived<TransactionQueryFilters>({
    ...linkFilters,
    ...filters,
    ...(lockStatus ? { status: lockStatus } : {})
  });

  $effect(() => {
    onFiltersChange?.(activeFilters);
  });
  const locale = $derived(getLocale());
  const queryClient = useQueryClient();

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

  // Same-day reorder (#185): a move places the row beside its visible
  // same-day neighbour, everywhere both appear.
  const reorder = new ReorderMode(queryClient);
  const transactionDay = (tx: TransactionResponse) => tx.transaction_date;

  function neighbor(tx: TransactionResponse, direction: ReorderDirection) {
    // The table hands back a proxy of the row, so find it by identity, not reference.
    return sameDayNeighbor(rows, rows.findIndex((row) => row.id === tx.id), direction, transactionDay);
  }

  function move(tx: TransactionResponse, direction: ReorderDirection) {
    const anchor = neighbor(tx, direction);
    if (!anchor || !csrfToken) return;
    const token = csrfToken;
    void reorder.move((reviewed) => placeTransaction(tx.id, anchor.id, placePosition(direction), token, reviewed));
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
      show={{ status: !lockStatus, kind: true, account: true, payee: true, search: true, dateRange: true, needsReview: true }}
    />
  </Panel>

  <div class="flex justify-end">
    <ReorderBar mode={reorder} disabled={!csrfToken} />
  </div>

  {#snippet dateCell(tx: TransactionResponse)}
    <span class="tabular-nums text-muted">{tx.transaction_date}</span>
    {@const postedDays = otherPostingDays(tx)}
    {#if postedDays.length}
      <span class="block text-xs tabular-nums text-muted">{m.transactions_posted_on_note({ dates: postedDays.join(', ') })}</span>
    {/if}
  {/snippet}

  {#snippet payeeCell(tx: TransactionResponse)}
    <span class="font-medium text-foreground">{transactionTitle(tx, systemLabelText, '—')}</span>
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
      {#if reorder.active}
        <ReorderArrows
          canMoveLater={!!neighbor(tx, 'later')}
          canMoveEarlier={!!neighbor(tx, 'earlier')}
          pending={reorder.pending}
          onLater={() => move(tx, 'later')}
          onEarlier={() => move(tx, 'earlier')}
        />
      {/if}
      <TransactionRowActions
        transaction={tx}
        onAction={actions.rowAction}
        disabled={!csrfToken || actions.pending}
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
      { key: 'actions', header: m.transactions_detail_actions(), priority: 1, width: '12rem', align: 'right', cell: actionsCell }
    ]}
    isLoading={query.isPending}
    isFetchingNextPage={query.isFetchingNextPage}
    hasNextPage={query.hasNextPage}
    onLoadMore={() => { if (!query.isFetchingNextPage) query.fetchNextPage(); }}
    error={query.error instanceof Error ? query.error : query.error ? new Error(String(query.error)) : null}
    onRetry={() => query.refetch()}
    onRowClick={onRowClick}
    emptyTitle={emptyTitle}
    emptyCopy={emptyCopy}
  />
</div>
