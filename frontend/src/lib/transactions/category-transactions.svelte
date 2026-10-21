<script lang="ts">
  import { replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import { parseTransactionFilters, writeTransactionFilters } from './transaction-url-filters.ts';
  import { otherPostingDays } from './posted-on';
  import { transactionTitle } from '#lib/transactions/transaction-title.ts';
  import { systemLabelText } from '#lib/transactions/system-label.ts';
  import { createInfiniteQuery, useQueryClient } from '@tanstack/svelte-query';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import Panel from '#lib/components/panel.svelte';
  import StatusBadge from '#lib/components/status-badge.svelte';
  import {
    placeTransaction,
    transactionsInfiniteQueryOptions,
    type TransactionResponse
  } from '#lib/api/transactions.ts';
  import { formatQuantity } from '#lib/money/format.ts';
  import { categoryAmounts as categoryScopeAmounts } from './category-amount';
  import {
    resolveAccountLabel,
    statusLabel,
    statusTone
  } from './transaction-labels';
  import TransactionTable from './transaction-table.svelte';
  import TransactionFilterBar from './transaction-filter-bar.svelte';
  import ReorderBar from './reorder-bar.svelte';
  import ReorderArrows from './reorder-arrows.svelte';
  import { ReorderMode } from './reorder-mode.svelte.ts';
  import { placePosition, sameDayNeighbor, type ReorderDirection } from './reorder';
  import type { TransactionFilters } from './transaction-filter-bar.svelte';

  import type { TransactionWorkspaceActions } from './transaction-workspace.svelte';
  import TransactionRowActions from './transaction-row-actions.svelte';

  // SvelteKit 3 keeps shallow URL changes separate from page.url.
  const filterURL = $derived(page.shallow?.url ?? page.url);

  let {
    categoryID,
    csrfToken,
    actions,
    onRowClick
  }: {
    categoryID: number;
    csrfToken?: string;
    actions: TransactionWorkspaceActions;
    onRowClick?: (tx: TransactionResponse) => void;
  } = $props();

  const filters = $derived(parseTransactionFilters(filterURL.searchParams));
  function setFilters(value: TransactionFilters) {
    const url = new URL(filterURL.href);
    url.search = writeTransactionFilters(filterURL.searchParams, value).toString();
    void replaceState(url, page.state);
  }
  const locale = $derived(getLocale());

  const query = createInfiniteQuery(() =>
    transactionsInfiniteQueryOptions({
      categoryID,
      status: filters.status,
      q: filters.q,
      afterDate: filters.afterDate,
      beforeDate: filters.beforeDate
    })
  );

  const rows = $derived(
    query.data?.pages.flatMap((p) => p.transactions ?? []) ?? []
  );

  // Per-commodity amount this category contributes to a transaction (#222).
  type CommodityAmount = {
    commodityID: number;
    commodityCode: string;
    commoditySymbol: string | null | undefined;
    displayValue: string;
    isPositive: boolean;
  };

  function categoryAmounts(tx: TransactionResponse): CommodityAmount[] {
    return categoryScopeAmounts(tx, categoryID).map((total) => {
      const isPositive = !total.value.startsWith('-');
      const absValue = isPositive ? total.value : total.value.slice(1);
      return {
        commodityID: total.commodityID,
        commodityCode: total.commodityCode,
        commoditySymbol: total.commoditySymbol,
        displayValue: formatQuantity(absValue, total.scale, locale),
        isPositive
      };
    });
  }

  // Same-day reorder (#185). A category has no statement of its own, so a
  // move always applies everywhere both transactions appear.
  const reorder = new ReorderMode(useQueryClient());
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

  // Counterpart labels: non-category (non-income/expense) postings, excluding system accounts.
  function counterpartLabel(tx: TransactionResponse): string {
    const allPostings = tx.journal_entries?.flatMap((e) => e.postings ?? []) ?? [];
    const counterparts = allPostings.filter(
      (p) => (p.account_class === 'asset' || p.account_class === 'liability')
        && !p.account_system_role
    );
    const labels = [...new Set(counterparts.map(resolveAccountLabel))];
    if (labels.length === 0) return '—';
    if (labels.length <= 2) return labels.join(', ');
    return m.category_tx_n_accounts({ n: labels.length });
  }
</script>

<div class="space-y-4">
  <Panel variant="toolbar">
    <TransactionFilterBar
      bind:filters={() => filters, setFilters}
      show={{ status: true, dateRange: true, search: true }}
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
    <div class="min-w-0">
      <span class="block truncate font-medium text-foreground">
        {transactionTitle(tx, systemLabelText, '—')}
      </span>
      {#if tx.needs_review}
        <span class="mt-0.5 inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider ring-1 ring-accent/40 text-accent">
          {m.transactions_needs_review()}
        </span>
      {/if}
    </div>
  {/snippet}

  {#snippet amountCell(tx: TransactionResponse)}
    {@const amounts = categoryAmounts(tx)}
    {#if amounts.length === 0}
      <span class="text-muted">—</span>
    {:else}
      <div class="flex flex-col items-end gap-0.5">
        {#each amounts as amt (amt.commodityID)}
          <span class={`font-medium tabular-nums leading-tight ${amt.isPositive ? 'text-foreground' : 'text-danger'}`}>
            {#if !amt.isPositive}−{/if}{amt.commoditySymbol ?? amt.commodityCode}{amt.displayValue}
          </span>
        {/each}
      </div>
    {/if}
  {/snippet}

  {#snippet counterpartCell(tx: TransactionResponse)}
    <span class="truncate text-sm text-muted">{counterpartLabel(tx)}</span>
  {/snippet}

  {#snippet statusCell(tx: TransactionResponse)}
    <StatusBadge tone={statusTone(tx.status)}>{statusLabel(tx.status)}</StatusBadge>
  {/snippet}

  {#snippet flagsCell(tx: TransactionResponse)}
    {#if tx.needs_review}
      <span class="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider ring-1 ring-accent/40 text-accent">
        {m.transactions_needs_review()}
      </span>
    {:else}
      <span></span>
    {/if}
  {/snippet}

  {#snippet reorderCell(tx: TransactionResponse)}
    <div class="flex items-center justify-end gap-2">
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
      { key: 'date', header: m.category_tx_col_date(), priority: 1, width: '9rem', cell: dateCell },
      { key: 'payee', header: m.category_tx_col_payee(), priority: 1, cell: payeeCell },
      { key: 'amount', header: m.category_tx_col_amount(), priority: 1, align: 'right', width: '10rem', cell: amountCell },
      { key: 'counterpart', header: m.category_tx_col_counterpart(), priority: 2, cell: counterpartCell },
      { key: 'status', header: m.category_tx_col_status(), priority: 3, width: '7rem', cell: statusCell },
      { key: 'flags', header: m.category_tx_col_flags(), priority: 3, width: '7rem', cell: flagsCell },
      { key: 'actions', header: m.transactions_detail_actions(), priority: 1, width: '12rem', align: 'right', cell: reorderCell }
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
