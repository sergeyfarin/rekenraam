<script lang="ts">
  import { joinCommodityAmount } from '$lib/money/format';
  import { createInfiniteQuery, useQueryClient } from '@tanstack/svelte-query';
  import { m } from '$lib/paraglide/messages.js';
  import { getLocale } from '$lib/paraglide/runtime.js';
  import { authSessionQueryOptions } from '$lib/api/auth';
  import { createQuery } from '@tanstack/svelte-query';
  import Panel from '$lib/components/panel.svelte';
  import StatePanel from '$lib/components/state-panel.svelte';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import StatusBadge from '$lib/components/status-badge.svelte';
  import {
    deletedTransactionsInfiniteQueryOptions,
    deletedTransactionsQueryKey,
    transactionsQueryKey,
    restoreTransaction,
    type DeletedTransactionResponse
  } from '$lib/api/transactions';
  import {
    formatSignedAmount,
    commodityDisplay
  } from '$lib/transactions/transaction-labels';
  import TransactionTable from '$lib/transactions/transaction-table.svelte';
  import type { Column } from '$lib/transactions/transaction-table-types';
  import { APIClientError } from '$lib/api/client';

  const queryClient = useQueryClient();
  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const locale = $derived(getLocale());

  const query = createInfiniteQuery(() => deletedTransactionsInfiniteQueryOptions());

  const rows = $derived(
    query.data?.pages.flatMap((p) => p.transactions ?? []) ?? []
  );

  // ── Restore state ────────────────────────────────────────────────────
  let restorePendingID = $state<number | undefined>(undefined);
  let restoreError = $state<unknown>(undefined);

  // Reconciliation override confirmation modal
  let overrideModal = $state<{
    open: boolean;
    row: DeletedTransactionResponse | null;
  }>({ open: false, row: null });

  async function csrfToken(): Promise<string> {
    if (sessionQuery.data?.csrf_token) return sessionQuery.data.csrf_token;
    const session = await sessionQuery.refetch();
    const token = session.data?.csrf_token;
    if (!token) throw new Error('CSRF token is unavailable');
    return token;
  }

  async function handleRestore(row: DeletedTransactionResponse, withOverride: boolean) {
    restorePendingID = row.id;
    restoreError = undefined;

    try {
      const token = await csrfToken();
      await restoreTransaction(row.id, token, {
        change_reason: 'restored from trash',
        reconciliation_override: withOverride || undefined
      });
      // Invalidate both the trash list and the main transactions list.
      await queryClient.invalidateQueries({ queryKey: deletedTransactionsQueryKey });
      await queryClient.invalidateQueries({ queryKey: transactionsQueryKey });
    } catch (error) {
      restoreError = error;
    } finally {
      restorePendingID = undefined;
    }
  }

  function handleRestoreClick(row: DeletedTransactionResponse) {
    if (row.restore_blocked_by_reconciliation) {
      overrideModal = { open: true, row };
    } else {
      handleRestore(row, false);
    }
  }

  async function confirmOverride() {
    if (!overrideModal.row) return;
    const row = overrideModal.row;
    overrideModal = { open: false, row: null };
    await handleRestore(row, true);
  }

  function cancelOverride() {
    overrideModal = { open: false, row: null };
  }

  // ── Display helpers ──────────────────────────────────────────────────
  function primaryPosting(row: DeletedTransactionResponse) {
    const postings = row.journal_entries?.flatMap((e) => e.postings ?? []) ?? [];
    return (
      postings.find((p) => p.account_class === 'asset' || p.account_class === 'liability') ??
      postings[0]
    );
  }

  function formatDeletedAt(deletedAt: string | undefined): string {
    if (!deletedAt) return '—';
    try {
      return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(
        new Date(deletedAt)
      );
    } catch {
      return deletedAt;
    }
  }
</script>

<div class="space-y-4">
  <Panel>
    <h2 class="text-lg font-semibold tracking-tight">{m.trash_title()}</h2>
    <p class="mt-1 text-sm text-muted">{m.trash_description()}</p>
  </Panel>

  {#if restoreError}
    <Panel>
      <APIFormError error={restoreError} />
    </Panel>
  {/if}

  {#snippet deletedCell(row: DeletedTransactionResponse)}
    <span class="tabular-nums text-muted text-xs">{formatDeletedAt(row.deleted_at)}</span>
  {/snippet}

  {#snippet dateCell(row: DeletedTransactionResponse)}
    <span class="tabular-nums text-muted">{row.transaction_date}</span>
  {/snippet}

  {#snippet payeeCell(row: DeletedTransactionResponse)}
    <div class="flex items-center gap-2">
      <span class="font-medium text-foreground">{row.payee_name || row.description || '—'}</span>
      {#if row.status === 'voided'}
        <StatusBadge tone="warning">{row.status}</StatusBadge>
      {/if}
    </div>
  {/snippet}

  {#snippet amountCell(row: DeletedTransactionResponse)}
    {@const p = primaryPosting(row)}
    {#if p}
      <span class={`font-medium tabular-nums ${p.quantity_value.startsWith('-') ? 'text-danger' : 'text-foreground'}`}>
        {joinCommodityAmount(
          commodityDisplay(p),
          formatSignedAmount(p, p.account_class as import('$lib/transactions/transaction-labels').AccountClass, locale)
        )}
      </span>
    {:else}
      <span class="text-muted">—</span>
    {/if}
  {/snippet}

  {#snippet reasonCell(row: DeletedTransactionResponse)}
    <span class="text-sm text-muted">{row.delete_reason || '—'}</span>
  {/snippet}

  {#snippet actionsCell(row: DeletedTransactionResponse)}
    <div class="flex justify-end">
      <button
        type="button"
        onclick={() => handleRestoreClick(row)}
        disabled={restorePendingID === row.id}
        class="inline-flex items-center gap-1.5 rounded-(--radius-control) px-3 py-1.5 text-xs font-semibold transition
               border border-border bg-control text-foreground hover:bg-control-hover
               disabled:cursor-not-allowed disabled:opacity-60"
      >
        {#if restorePendingID === row.id}
          {m.trash_restoring()}
        {:else if row.restore_blocked_by_reconciliation}
          {m.trash_restore_guarded()}
        {:else}
          {m.trash_restore_easy()}
        {/if}
      </button>
    </div>
  {/snippet}

  <TransactionTable
    {rows}
    rowId={(tx) => String(tx.id)}
    columns={[
      { key: 'deleted', header: m.trash_col_deleted(), priority: 2, width: '11rem', cell: deletedCell },
      { key: 'date', header: m.trash_col_date(), priority: 1, width: '8rem', cell: dateCell },
      { key: 'payee', header: m.trash_col_payee(), priority: 1, cell: payeeCell },
      { key: 'amount', header: m.trash_col_amount(), priority: 1, align: 'right', width: '9rem', cell: amountCell },
      { key: 'reason', header: m.trash_col_reason(), priority: 3, cell: reasonCell },
      { key: 'actions', header: '', priority: 1, width: '9rem', align: 'right', cell: actionsCell }
    ] satisfies Column<DeletedTransactionResponse>[]}
    isLoading={query.isPending}
    isFetchingNextPage={query.isFetchingNextPage}
    hasNextPage={query.hasNextPage}
    onLoadMore={() => { if (!query.isFetchingNextPage) query.fetchNextPage(); }}
    error={query.error instanceof Error ? query.error : query.error ? new Error(String(query.error)) : null}
    onRetry={() => query.refetch()}
  />
</div>

<!-- Reconciliation override confirmation modal -->
{#if overrideModal.open && overrideModal.row}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
    role="dialog"
    aria-modal="true"
    aria-labelledby="trash-override-title"
  >
    <div class="w-full max-w-md rounded-(--radius-panel) border border-border bg-surface p-6 shadow-lg">
      <h3 id="trash-override-title" class="text-base font-semibold text-foreground">
        {m.trash_restore_confirm_title()}
      </h3>
      <p class="mt-2 text-sm text-muted">{m.trash_restore_confirm_description()}</p>
      <div class="mt-4 flex justify-end gap-3">
        <button
          type="button"
          onclick={cancelOverride}
          class="rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
        >
          {m.trash_restore_confirm_cancel()}
        </button>
        <button
          type="button"
          onclick={confirmOverride}
          class="rounded-(--radius-control) bg-danger px-4 py-2 text-sm font-semibold text-white transition hover:opacity-90"
        >
          {m.trash_restore_confirm_proceed()}
        </button>
      </div>
    </div>
  </div>
{/if}
