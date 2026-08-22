<script lang="ts">
  import { createInfiniteQuery, useQueryClient } from '@tanstack/svelte-query';
  import AlertTriangle from '@lucide/svelte/icons/triangle-alert';
  import CheckCircle from '@lucide/svelte/icons/check-circle';
  import Circle from '@lucide/svelte/icons/circle';
  import { m } from '$lib/paraglide/messages.js';
  import { getLocale } from '$lib/paraglide/runtime.js';
  import Panel from '$lib/components/panel.svelte';
  import StatusBadge from '$lib/components/status-badge.svelte';
  import {
    accountRegisterInfiniteQueryOptions,
    accountRegisterQueryKey,
    movePosting,
    type AccountRegisterEntryResponse
  } from '$lib/api/transactions';
  import { formatQuantity } from '$lib/money/format';
  import { formatSignedAmount, statusTone } from './transaction-labels';
  import type { AccountClass } from './transaction-labels';
  import TransactionTable from './transaction-table.svelte';
  import TransactionFilterBar from './transaction-filter-bar.svelte';
  import type { TransactionFilters } from './transaction-filter-bar.svelte';

  let {
    accountID,
    csrfToken,
    onRowClick
  }: {
    accountID: number;
    csrfToken?: string;
    onRowClick?: (entry: AccountRegisterEntryResponse) => void;
  } = $props();

  let filters = $state<TransactionFilters>({});
  let movePendingKey = $state<string | undefined>(undefined);
  let moveError = $state<unknown>(undefined);

  const locale = $derived(getLocale());
  const queryClient = useQueryClient();

  const query = createInfiniteQuery(() =>
    accountRegisterInfiniteQueryOptions(accountID, {
      status: filters.status,
      afterDate: filters.afterDate,
      beforeDate: filters.beforeDate
    })
  );

  const rows = $derived(
    query.data?.pages.flatMap((p) => p.entries ?? []) ?? []
  );

  // Format the running balance amount for a register entry.
  // BalanceQuantity uses normal_quantity_value (class-normalized sign) for display.
  function formatBalance(entry: AccountRegisterEntryResponse): string {
    const bal = entry.running_balance;
    // normal_quantity_value is already sign-flipped for liability/income/equity.
    return formatQuantity(bal.normal_quantity_value, bal.quantity_scale, locale);
  }

  // Format the posting amount using the inflow/outflow sign convention for this
  // account's class. The register always shows amounts from this account's perspective.
  function formatAmount(entry: AccountRegisterEntryResponse): string {
    const p = entry.posting;
    return formatSignedAmount(p, p.account_class as AccountClass, locale);
  }

  // Commodity prefix for the posting amount (symbol or code).
  function commodityPrefix(entry: AccountRegisterEntryResponse): string {
    const p = entry.posting;
    return p.commodity_symbol ?? p.commodity_code;
  }

  // Determine the counterpart label: non-system postings from the other legs.
  // For a register row, entry.posting is the leg for this account. The counterpart
  // is shown in place of a separate "account" column.
  // We don't have the full transaction's other postings here, so we use the
  // payee_name or description as context — the register purposefully doesn't fan-out
  // to show counterpart names from this response shape.
  // The resolved account label for entry.posting is the *this* account (already known
  // from the route context); counterpart info comes via the transaction description/payee.

  async function handleMove(entry: AccountRegisterEntryResponse, direction: 'earlier' | 'later') {
    if (!csrfToken) return;
    const key = `${entry.posting.posting_line_id}-${direction}`;
    movePendingKey = key;
    moveError = undefined;
    try {
      await movePosting(accountID, entry.posting.posting_line_id, direction, csrfToken);
      // Invalidate the register query so the updated order is fetched fresh.
      await queryClient.invalidateQueries({ queryKey: [...accountRegisterQueryKey, accountID] });
    } catch (e) {
      moveError = e;
    } finally {
      movePendingKey = undefined;
    }
  }

</script>

<div class="space-y-4">
  <Panel variant="toolbar">
    <TransactionFilterBar
      bind:filters
      show={{ status: true, dateRange: true }}
    />
  </Panel>

  {#snippet dateCell(entry: AccountRegisterEntryResponse)}
    <span class="tabular-nums text-muted">{entry.entry_date}</span>
  {/snippet}

  {#snippet payeeCell(entry: AccountRegisterEntryResponse)}
    <div class="min-w-0">
      <span class="block truncate font-medium text-foreground">
        {entry.payee_name || entry.description || '—'}
      </span>
      {#if entry.status === 'voided'}
        <span class="mt-0.5 inline-block">
          <StatusBadge tone={statusTone(entry.status)}>{m.transaction_status_voided()}</StatusBadge>
        </span>
      {/if}
    </div>
  {/snippet}

  {#snippet amountCell(entry: AccountRegisterEntryResponse)}
    {@const amt = formatAmount(entry)}
    {@const isNeg = entry.posting.quantity_value.startsWith('-')}
    {@const assetOrExpense = entry.posting.account_class === 'asset' || entry.posting.account_class === 'expense'}
    {@const displayNeg = assetOrExpense ? isNeg : !isNeg}
    <span class={`font-medium tabular-nums ${displayNeg ? 'text-danger' : 'text-foreground'}`}>
      {commodityPrefix(entry)}{amt}
    </span>
  {/snippet}

  {#snippet balanceCell(entry: AccountRegisterEntryResponse)}
    {@const bal = formatBalance(entry)}
    {@const balNeg = entry.running_balance.normal_quantity_value.startsWith('-')}
    <span class={`tabular-nums ${balNeg ? 'text-danger' : 'text-foreground'}`}>
      {bal}
    </span>
  {/snippet}

  {#snippet reconciliationCell(entry: AccountRegisterEntryResponse)}
    {@const status = entry.posting.reconciliation_status}
    <span class="inline-flex items-center gap-1 text-xs" title={
      status === 'reconciled' ? m.register_recon_reconciled()
      : status === 'cleared' ? m.register_recon_cleared()
      : m.register_recon_uncleared()
    }>
      {#if status === 'reconciled'}
        <CheckCircle size={13} class="text-positive" aria-hidden="true" />
        <span class="sr-only">{m.register_recon_reconciled()}</span>
      {:else if status === 'cleared'}
        <Circle size={13} class="text-accent" aria-hidden="true" />
        <span class="sr-only">{m.register_recon_cleared()}</span>
      {:else}
        <Circle size={13} class="text-muted" aria-hidden="true" />
        <span class="sr-only">{m.register_recon_uncleared()}</span>
      {/if}
    </span>
  {/snippet}

  {#snippet memoCell(entry: AccountRegisterEntryResponse)}
    <span class="truncate text-xs text-muted">{entry.posting.memo || ''}</span>
  {/snippet}

  {#snippet actionsCell(entry: AccountRegisterEntryResponse)}
    {@const postingKey = String(entry.posting.posting_line_id)}
    <div class="flex items-center justify-end gap-1">
      <button
        type="button"
        class="inline-flex h-7 w-7 items-center justify-center rounded text-muted hover:bg-surface-strong hover:text-foreground disabled:opacity-30"
        title={m.register_move_earlier()}
        aria-label={m.register_move_earlier()}
        disabled={movePendingKey === `${postingKey}-earlier`}
        onclick={(e) => { e.stopPropagation(); handleMove(entry, 'earlier'); }}
      >
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M18 15l-6-6-6 6"/>
        </svg>
      </button>
      <button
        type="button"
        class="inline-flex h-7 w-7 items-center justify-center rounded text-muted hover:bg-surface-strong hover:text-foreground disabled:opacity-30"
        title={m.register_move_later()}
        aria-label={m.register_move_later()}
        disabled={movePendingKey === `${postingKey}-later`}
        onclick={(e) => { e.stopPropagation(); handleMove(entry, 'later'); }}
      >
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M6 9l6 6 6-6"/>
        </svg>
      </button>
    </div>
  {/snippet}

  {#if moveError}
    <div class="flex items-center gap-2 rounded-(--radius-control) border border-warning/40 bg-warning-soft px-3 py-2 text-xs text-warning">
      <AlertTriangle size={13} aria-hidden="true" />
      {m.register_error_copy()}
    </div>
  {/if}

  <TransactionTable
    {rows}
    rowId={(entry) => `${entry.transaction_id}:${entry.version_id}`}
    columns={[
      { key: 'date', header: m.register_col_date(), priority: 1, width: '9rem', cell: dateCell },
      { key: 'payee', header: m.register_col_payee(), priority: 1, cell: payeeCell },
      { key: 'amount', header: m.register_col_amount(), priority: 1, align: 'right', width: '10rem', cell: amountCell },
      { key: 'balance', header: m.register_col_balance(), priority: 1, align: 'right', width: '10rem', cell: balanceCell },
      { key: 'reconciliation', header: m.register_col_reconciliation(), priority: 2, width: '5rem', align: 'right', cell: reconciliationCell },
      { key: 'memo', header: m.register_col_memo(), priority: 3, cell: memoCell },
      { key: 'actions', header: '', priority: 1, width: '6rem', align: 'right', cell: actionsCell }
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
