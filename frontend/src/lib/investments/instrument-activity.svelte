<script lang="ts">
  import { replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import { parseTransactionFilters, writeTransactionFilters } from '#lib/transactions/transaction-url-filters.ts';
  import { createInfiniteQuery } from '@tanstack/svelte-query';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import Panel from '#lib/components/panel.svelte';
  import StatusBadge from '#lib/components/status-badge.svelte';
  import { transactionsInfiniteQueryOptions, type TransactionResponse } from '#lib/api/transactions.ts';
  import { formatQuantity } from '#lib/money/format.ts';
  import { transactionTitle } from '#lib/transactions/transaction-title.ts';
  import { systemLabelText } from '#lib/transactions/system-label.ts';
  import { resolveAccountLabel, statusLabel, statusTone } from '#lib/transactions/transaction-labels.ts';
  import TransactionTable from '#lib/transactions/transaction-table.svelte';
  import TransactionFilterBar, { type TransactionFilters } from '#lib/transactions/transaction-filter-bar.svelte';
  import TransactionRowActions from '#lib/transactions/transaction-row-actions.svelte';
  import type { TransactionWorkspaceActions } from '#lib/transactions/transaction-workspace.svelte';
  import { instrumentAccountIDs, instrumentUnits } from './instrument-activity.ts';

  // An instrument's activity (#229): postings and cash-dividend operation
  // facts identify its transactions. One request per page
  // through the commodity filter, never a fetch per holding.
  // SvelteKit 3 keeps shallow URL changes separate from page.url.
  const filterURL = $derived(page.shallow?.url ?? page.url);

  let {
    commodityID,
    csrfToken,
    actions
  }: {
    commodityID: number;
    csrfToken?: string;
    actions: TransactionWorkspaceActions;
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
      commodityID,
      status: filters.status,
      q: filters.q,
      afterDate: filters.afterDate,
      beforeDate: filters.beforeDate
    })
  );

  const rows = $derived(query.data?.pages.flatMap((p) => p.transactions ?? []) ?? []);

  function accountsLabel(tx: TransactionResponse): string {
    const postings = tx.journal_entries?.flatMap((entry) => entry.postings ?? []) ?? [];
    const labels = instrumentAccountIDs(tx, commodityID)
      .map((id) => postings.find((posting) => posting.account_id === id))
      .filter((posting) => posting !== undefined)
      .map(resolveAccountLabel);
    return labels.length === 0 ? '—' : labels.join(', ');
  }
</script>

<div class="space-y-4">
  <Panel variant="toolbar">
    <TransactionFilterBar bind:filters={() => filters, setFilters} show={{ status: true, dateRange: true, search: true }} />
  </Panel>

  {#snippet dateCell(tx: TransactionResponse)}
    <span class="tabular-nums text-muted">{tx.transaction_date}</span>
  {/snippet}

  {#snippet titleCell(tx: TransactionResponse)}
    <span class="block truncate font-medium text-foreground">{transactionTitle(tx, systemLabelText, '—')}</span>
  {/snippet}

  {#snippet unitsCell(tx: TransactionResponse)}
    {@const units = instrumentUnits(tx, commodityID)}
    {#if units}
      <span class={`font-medium tabular-nums ${units.value.startsWith('-') ? 'text-danger' : 'text-foreground'}`}>
        {#if !units.value.startsWith('-') && units.value !== '0'}+{/if}{formatQuantity(units.value, units.scale, locale)}
      </span>
    {:else}
      <span class="text-muted">—</span>
    {/if}
  {/snippet}

  {#snippet accountsCell(tx: TransactionResponse)}
    <span class="truncate text-sm text-muted">{accountsLabel(tx)}</span>
  {/snippet}

  {#snippet statusCell(tx: TransactionResponse)}
    <StatusBadge tone={statusTone(tx.status)}>{statusLabel(tx.status)}</StatusBadge>
  {/snippet}

  {#snippet actionsCell(tx: TransactionResponse)}
    <div class="flex items-center justify-end">
      <TransactionRowActions transaction={tx} onAction={actions.rowAction} disabled={!csrfToken || actions.pending} />
    </div>
  {/snippet}

  <TransactionTable
    {rows}
    rowId={(tx) => String(tx.id)}
    columns={[
      { key: 'date', header: m.category_tx_col_date(), priority: 1, width: '9rem', cell: dateCell },
      { key: 'title', header: m.category_tx_col_payee(), priority: 1, cell: titleCell },
      { key: 'units', header: m.investments_instrument_view_col_units(), priority: 1, align: 'right', width: '9rem', cell: unitsCell },
      { key: 'accounts', header: m.investments_instrument_view_col_accounts(), priority: 2, cell: accountsCell },
      { key: 'status', header: m.category_tx_col_status(), priority: 3, width: '7rem', cell: statusCell },
      { key: 'actions', header: m.transactions_detail_actions(), priority: 1, width: '10rem', align: 'right', cell: actionsCell }
    ]}
    isLoading={query.isPending}
    isFetchingNextPage={query.isFetchingNextPage}
    hasNextPage={query.hasNextPage}
    onLoadMore={() => { if (!query.isFetchingNextPage) query.fetchNextPage(); }}
    error={query.error instanceof Error ? query.error : query.error ? new Error(String(query.error)) : null}
    onRetry={() => query.refetch()}
    onRowClick={actions.showDetail}
  />

</div>
