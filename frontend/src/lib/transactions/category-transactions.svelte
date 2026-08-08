<script lang="ts">
  import { createInfiniteQuery } from '@tanstack/svelte-query';
  import { m } from '$lib/paraglide/messages.js';
  import { getLocale } from '$lib/paraglide/runtime.js';
  import Panel from '$lib/components/panel.svelte';
  import StatusBadge from '$lib/components/status-badge.svelte';
  import {
    transactionsInfiniteQueryOptions,
    type TransactionResponse
  } from '$lib/api/transactions';
  import { formatQuantity } from '$lib/money/format';
  import {
    resolveAccountLabel,
    statusLabel,
    statusTone,
    type PostingResponse
  } from './transaction-labels';
  import TransactionTable from './transaction-table.svelte';
  import TransactionFilterBar from './transaction-filter-bar.svelte';
  import type { TransactionFilters } from './transaction-filter-bar.svelte';

  let {
    categoryID,
    onRowClick
  }: {
    categoryID: number;
    onRowClick?: (tx: TransactionResponse) => void;
  } = $props();

  let filters = $state<TransactionFilters>({});
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

  // Rescale an integer coefficient string to a greater scale (right-pad with zeros).
  function rescaleUp(coeff: string, fromScale: number, toScale: number): bigint {
    const factor = BigInt(10) ** BigInt(toScale - fromScale);
    return BigInt(coeff) * factor;
  }

  // Per-commodity category amount rows for a transaction.
  // Returns one entry per commodity that has category postings.
  type CommodityAmount = {
    commodityID: number;
    commodityCode: string;
    commoditySymbol: string | null | undefined;
    displayValue: string;
    scale: number;
    isPositive: boolean;
  };

  function categoryAmounts(tx: TransactionResponse): CommodityAmount[] {
    const allPostings = tx.journal_entries?.flatMap((e) => e.postings ?? []) ?? [];

    // Category postings: income or expense class, no system role.
    const catPostings = allPostings.filter(
      (p) => (p.account_class === 'income' || p.account_class === 'expense')
        && !p.account_system_role
    );
    if (catPostings.length === 0) return [];

    // Group by commodity_id.
    const byCommodity = new Map<number, PostingResponse[]>();
    for (const p of catPostings) {
      const group = byCommodity.get(p.commodity_id) ?? [];
      group.push(p);
      byCommodity.set(p.commodity_id, group);
    }

    const result: CommodityAmount[] = [];
    for (const [commodityID, postings] of byCommodity) {
      const maxScale = postings.reduce((s, p) => Math.max(s, p.quantity_scale), 0);

      // Activity-positive sum: expense debit (positive raw) = +; income credit (negative raw) = +.
      // For income/liability/equity, negate the raw coefficient so credits become positive.
      let sum = BigInt(0);
      for (const p of postings) {
        const rescaled = rescaleUp(p.quantity_value, p.quantity_scale, maxScale);
        if (p.account_class === 'income' || p.account_class === 'liability' || p.account_class === 'equity') {
          sum += -rescaled;
        } else {
          sum += rescaled;
        }
      }

      const isPositive = sum >= BigInt(0);
      const absSum = isPositive ? sum : -sum;
      const displayValue = formatQuantity(absSum.toString(), maxScale, locale);

      const rep = postings[0];
      result.push({
        commodityID,
        commodityCode: rep.commodity_code,
        commoditySymbol: rep.commodity_symbol,
        displayValue,
        scale: maxScale,
        isPositive
      });
    }

    return result;
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
      bind:filters
      show={{ status: true, dateRange: true, search: true }}
    />
  </Panel>

  {#snippet dateCell(tx: TransactionResponse)}
    <span class="tabular-nums text-muted">{tx.transaction_date}</span>
  {/snippet}

  {#snippet payeeCell(tx: TransactionResponse)}
    <div class="min-w-0">
      <span class="block truncate font-medium text-foreground">
        {tx.payee_name || tx.description || '—'}
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

  <TransactionTable
    {rows}
    columns={[
      { key: 'date', header: m.category_tx_col_date(), priority: 1, width: '9rem', cell: dateCell },
      { key: 'payee', header: m.category_tx_col_payee(), priority: 1, cell: payeeCell },
      { key: 'amount', header: m.category_tx_col_amount(), priority: 1, align: 'right', width: '10rem', cell: amountCell },
      { key: 'counterpart', header: m.category_tx_col_counterpart(), priority: 2, cell: counterpartCell },
      { key: 'status', header: m.category_tx_col_status(), priority: 3, width: '7rem', cell: statusCell },
      { key: 'flags', header: m.category_tx_col_flags(), priority: 3, width: '7rem', cell: flagsCell }
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
