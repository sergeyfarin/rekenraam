<script lang="ts">
  import { replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import { parseTransactionFilters, writeTransactionFilters } from './transaction-url-filters.ts';
  import { transactionTitle } from '#lib/transactions/transaction-title.ts';
  import { systemLabelText } from '#lib/transactions/system-label.ts';
  import { createInfiniteQuery, useQueryClient } from '@tanstack/svelte-query';
  import CheckCircle from '@lucide/svelte/icons/check-circle';
  import Circle from '@lucide/svelte/icons/circle';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import Panel from '#lib/components/panel.svelte';
  import StatusBadge from '#lib/components/status-badge.svelte';
  import {
    accountRegisterInfiniteQueryOptions,
    movePosting,
    movePostingToDay,
    placeTransaction,
    type AccountRegisterEntryResponse
  } from '#lib/api/transactions.ts';
  import { formatQuantity } from '#lib/money/format.ts';
  import { formatSignedAmount, statusTone } from './transaction-labels';
  import type { AccountClass } from './transaction-labels';
  import TransactionTable from './transaction-table.svelte';
  import CorrectionChainNote from './correction-chain-note.svelte';
  import TransactionFilterBar from './transaction-filter-bar.svelte';
  import type { TransactionFilters } from './transaction-filter-bar.svelte';
  import ReorderBar from './reorder-bar.svelte';
  import ReorderArrows from './reorder-arrows.svelte';
  import { ReorderMode } from './reorder-mode.svelte.ts';
  import { placePosition, sameDayNeighbor, type ReorderDirection } from './reorder';
  import { hasPostingDay } from './posted-on';
  import { adjacentCalendarDate } from '#lib/dates/calendar-date.ts';

  import type { TransactionWorkspaceActions } from './transaction-workspace.svelte';
  import TransactionRowActions from './transaction-row-actions.svelte';

  // SvelteKit 3 keeps shallow URL changes separate from page.url.
  const filterURL = $derived(page.shallow?.url ?? page.url);

  let {
    accountID,
    csrfToken,
    actions,
    onRowClick,
    offerOnlyThisAccount = true
  }: {
    accountID: number;
    csrfToken?: string;
    actions: TransactionWorkspaceActions;
    onRowClick?: (entry: AccountRegisterEntryResponse) => void;
    /** A bank or card statement can list a day in its own order (#185). */
    offerOnlyThisAccount?: boolean;
  } = $props();

  // Explicit, never blank: the register shows one lifecycle at a time (#223).
  const filters = $derived.by<TransactionFilters>(() => {
    const parsed = parseTransactionFilters(filterURL.searchParams);
    return {
      status: parsed.status === 'voided' ? 'voided' : 'posted',
      afterDate: parsed.afterDate,
      beforeDate: parsed.beforeDate
    };
  });

  function setFilters(value: TransactionFilters) {
    const url = new URL(filterURL.href);
    url.search = writeTransactionFilters(filterURL.searchParams, value).toString();
    void replaceState(url, page.state);
  }

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

  // Same-day reorder (#185). By default a move places the transaction beside
  // its visible same-day neighbour everywhere both appear; "Only in this
  // account" moves this register's posting alone. Two postings of one
  // transaction in this account can only be reordered here.
  const reorder = new ReorderMode(queryClient);
  const entryDay = (entry: AccountRegisterEntryResponse) => entry.entry_date;

  // A register row is one posting, and one transaction can post to this
  // account more than once (#221), so the transaction is not the row. The
  // posting line outlives transaction versions, so a row keeps its DOM and
  // focus when an edit or reorder refreshes the register.
  const rowKey = (entry: AccountRegisterEntryResponse) => String(entry.posting.posting_line_id);

  function neighbor(entry: AccountRegisterEntryResponse, direction: ReorderDirection) {
    // The table hands back a proxy of the row, so find it by identity, not reference.
    return sameDayNeighbor(rows, rows.findIndex((row) => rowKey(row) === rowKey(entry)), direction, entryDay);
  }

  // At a day's edge the arrow offers to move this account's posting to the
  // adjacent calendar day (#187): the bank posted it a day later or earlier.
  // Other accounts and the transaction date stay; investment postings have
  // their own workflows.
  function canMoveDay(entry: AccountRegisterEntryResponse): boolean {
    return entry.status === 'posted' && entry.transaction_kind !== 'investment' && hasPostingDay(entry.posting);
  }

  function arrowLabel(entry: AccountRegisterEntryResponse, direction: ReorderDirection): string | undefined {
    if (neighbor(entry, direction) || !canMoveDay(entry)) return undefined;
    return direction === 'later' ? m.transactions_reorder_next_day() : m.transactions_reorder_previous_day();
  }

  function move(entry: AccountRegisterEntryResponse, direction: ReorderDirection) {
    const anchor = neighbor(entry, direction);
    if (!csrfToken) return;
    if (!anchor) {
      if (!canMoveDay(entry)) return;
      const token = csrfToken;
      reorder.proposeDayMove(
        adjacentCalendarDate(entry.entry_date, direction === 'later' ? 1 : -1),
        (reviewed) => movePostingToDay(accountID, entry.posting.posting_line_id, direction, token, reviewed)
      );
      return;
    }
    const token = csrfToken;
    const local = reorder.onlyThisAccount || anchor.transaction_id === entry.transaction_id;
    void reorder.move((reviewed) => local
      ? movePosting(accountID, entry.posting.posting_line_id, direction, token, reviewed)
      : placeTransaction(entry.transaction_id, anchor.transaction_id, placePosition(direction), token, reviewed));
  }

</script>

<div class="space-y-4">
  <Panel variant="toolbar">
    <TransactionFilterBar
      bind:filters={() => filters, setFilters}
      show={{ status: true, dateRange: true }}
      singleStatus
    />
  </Panel>

  <div class="flex justify-end">
    <ReorderBar mode={reorder} {offerOnlyThisAccount} disabled={!csrfToken} />
  </div>

  {#snippet dateCell(entry: AccountRegisterEntryResponse)}
    <span class="tabular-nums text-muted">{entry.entry_date}</span>
    {#if entry.transaction_date !== entry.entry_date}
      <span class="block text-xs tabular-nums text-muted">{m.transactions_transaction_date_note({ date: entry.transaction_date })}</span>
    {/if}
  {/snippet}

  {#snippet payeeCell(entry: AccountRegisterEntryResponse)}
    <div class="min-w-0">
      <span class="block truncate font-medium text-foreground">
        {transactionTitle(entry, systemLabelText, '—')}
      </span>
      {#if entry.status === 'voided'}
        <span class="mt-0.5 inline-block">
          <StatusBadge tone={statusTone(entry.status)}>{m.transaction_status_voided()}</StatusBadge>
        </span>
      {/if}
      {#if entry.correction_chain}
        <CorrectionChainNote {entry} chain={entry.correction_chain} {locale} />
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
    <div class="flex items-center justify-end gap-2">
      {#if reorder.active}
        <ReorderArrows
          canMoveLater={!!neighbor(entry, 'later') || canMoveDay(entry)}
          canMoveEarlier={!!neighbor(entry, 'earlier') || canMoveDay(entry)}
          laterLabel={arrowLabel(entry, 'later')}
          earlierLabel={arrowLabel(entry, 'earlier')}
          pending={reorder.pending}
          onLater={() => move(entry, 'later')}
          onEarlier={() => move(entry, 'earlier')}
        />
      {/if}
      <TransactionRowActions
        transaction={{ id: entry.transaction_id, status: entry.status, needs_review: entry.needs_review, transaction_kind: entry.transaction_kind }}
        onAction={actions.rowAction}
        disabled={!csrfToken || actions.pending}
      />
    </div>
  {/snippet}

  <TransactionTable
    {rows}
    rowId={rowKey}
    columns={[
      { key: 'date', header: m.register_col_date(), priority: 1, width: '9rem', cell: dateCell },
      { key: 'payee', header: m.register_col_payee(), priority: 1, cell: payeeCell },
      { key: 'amount', header: m.register_col_amount(), priority: 1, align: 'right', width: '10rem', cell: amountCell },
      { key: 'balance', header: m.register_col_balance(), priority: 1, align: 'right', width: '10rem', cell: balanceCell },
      { key: 'reconciliation', header: m.register_col_reconciliation(), priority: 2, width: '5rem', align: 'right', cell: reconciliationCell },
      { key: 'memo', header: m.register_col_memo(), priority: 3, cell: memoCell },
      { key: 'actions', header: m.transactions_detail_actions(), priority: 1, width: '12rem', align: 'right', cell: actionsCell }
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
