<script lang="ts">
  import { joinCommodityAmount } from '#lib/money/format.ts';
  import { createInfiniteQuery, createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { authSessionQueryOptions } from '#lib/api/auth.ts';
  import Panel from '#lib/components/panel.svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import ModalDialog from '#lib/components/modal-dialog.svelte';
  import StatusBadge from '#lib/components/status-badge.svelte';
  import {
    deletedTransactionsInfiniteQueryOptions,
    deletedTransactionsQueryKey,
    transactionsQueryKey,
    restoreTransaction,
    getLifecycleReconciliationImpact,
    isReconciliationOverrideRequired,
    isReconciliationImpactStale,
    reviewedOverride,
    type DeletedTransactionResponse,
    type ReconciliationImpactResponse
  } from '#lib/api/transactions.ts';
  import {
    formatSignedAmount,
    commodityDisplay,
    statusLabel
  } from '#lib/transactions/transaction-labels.ts';
  import TransactionTable from '#lib/transactions/transaction-table.svelte';
  import type { Column } from '#lib/transactions/transaction-table-types.ts';
  import { forecastQueryKey } from '#lib/api/forecast.ts';

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

  // Reconciliation override confirmation (#225). The list's
  // restore_blocked_by_reconciliation flag, or a refused plain restore, opens
  // it; it then names every checkpoint from the restore's own preview before
  // the owner can confirm. The preview is never authority: confirming sends
  // the real restore, which rechecks the guard.
  let overrideModal = $state<{
    row: DeletedTransactionResponse | null;
    impacts: ReconciliationImpactResponse['affected_checkpoints'] | null;
    loadError: unknown;
    writeError: unknown;
    pending: boolean;
  }>({ row: null, impacts: null, loadError: undefined, writeError: undefined, pending: false });

  async function csrfToken(): Promise<string> {
    if (sessionQuery.data?.csrf_token) return sessionQuery.data.csrf_token;
    const session = await sessionQuery.refetch();
    const token = session.data?.csrf_token;
    if (!token) throw new Error('CSRF token is unavailable');
    return token;
  }

  /** reviewed is the previewed set an override acknowledges; absent, no override (#296). */
  async function restore(row: DeletedTransactionResponse, reviewed?: ReconciliationImpactResponse['affected_checkpoints']) {
    const token = await csrfToken();
    await restoreTransaction(row.id, token, {
      change_reason: 'restored from deleted transactions',
      expected_version_id: row.version_id,
      ...(reviewed ? reviewedOverride({ affected_checkpoints: reviewed }) : {})
    });
    // Invalidate both the deleted list and the ordinary transactions list.
    await queryClient.invalidateQueries({ queryKey: deletedTransactionsQueryKey });
    await queryClient.invalidateQueries({ queryKey: transactionsQueryKey });
    await queryClient.invalidateQueries({ queryKey: forecastQueryKey });
  }

  async function handleRestoreClick(row: DeletedTransactionResponse) {
    if (row.transaction_kind === 'investment') return;
    restoreError = undefined;
    if (row.restore_blocked_by_reconciliation) {
      void openOverride(row);
      return;
    }
    restorePendingID = row.id;
    try {
      await restore(row);
    } catch (error) {
      // The list flag can be stale: a statement finished since it loaded.
      if (isReconciliationOverrideRequired(error)) void openOverride(row);
      else restoreError = error;
    } finally {
      restorePendingID = undefined;
    }
  }

  async function openOverride(row: DeletedTransactionResponse) {
    overrideModal = { row, impacts: null, loadError: undefined, writeError: undefined, pending: false };
    try {
      const impact = await getLifecycleReconciliationImpact(row.id, 'restore');
      if (overrideModal.row?.id === row.id) overrideModal.impacts = impact.affected_checkpoints;
    } catch (error) {
      if (overrideModal.row?.id === row.id) overrideModal.loadError = error;
    }
  }

  async function confirmOverride() {
    const row = overrideModal.row;
    if (!row || overrideModal.impacts === null) return;
    overrideModal.pending = true;
    overrideModal.writeError = undefined;
    try {
      await restore(row, overrideModal.impacts);
      cancelOverride();
    } catch (error) {
      overrideModal.writeError = error;
      // A statement finished or was voided since the preview: show the set
      // as it is now and let the owner confirm again (#296).
      if (isReconciliationImpactStale(error)) {
        try {
          const impact = await getLifecycleReconciliationImpact(row.id, 'restore');
          if (overrideModal.row?.id === row.id) overrideModal.impacts = impact.affected_checkpoints;
        } catch (previewError) {
          overrideModal.writeError = previewError;
        }
      }
    } finally {
      overrideModal.pending = false;
    }
  }

  function cancelOverride() {
    overrideModal = { row: null, impacts: null, loadError: undefined, writeError: undefined, pending: false };
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

<div class="space-y-4" data-testid="deleted-transactions">
  <Panel>
    <h2 class="text-lg font-semibold tracking-tight">{m.transactions_deleted_title()}</h2>
    <p class="mt-1 text-sm text-muted">{m.transactions_deleted_description()}</p>
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
        <StatusBadge tone="warning">{statusLabel(row.status)}</StatusBadge>
      {/if}
    </div>
  {/snippet}

  {#snippet amountCell(row: DeletedTransactionResponse)}
    {@const p = primaryPosting(row)}
    {#if p}
      <span class={`font-medium tabular-nums ${p.quantity_value.startsWith('-') ? 'text-danger' : 'text-foreground'}`}>
        {joinCommodityAmount(
          commodityDisplay(p),
          formatSignedAmount(p, p.account_class as import('#lib/transactions/transaction-labels.ts').AccountClass, locale)
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
      {#if row.transaction_kind === 'investment'}
        <span class="max-w-48 text-right text-xs text-muted">{m.transactions_deleted_investment_restore_locked()}</span>
      {:else}
        <button
          type="button"
          onclick={() => handleRestoreClick(row)}
          disabled={restorePendingID === row.id}
          class="inline-flex items-center gap-1.5 rounded-(--radius-control) px-3 py-1.5 text-xs font-semibold transition
                 border border-border bg-control text-foreground hover:bg-control-hover
                 disabled:cursor-not-allowed disabled:opacity-60"
        >
          {#if restorePendingID === row.id}
            {m.transactions_deleted_restoring()}
          {:else if row.restore_blocked_by_reconciliation}
            {m.transactions_deleted_restore_guarded()}
          {:else}
            {m.transactions_deleted_restore_easy()}
          {/if}
        </button>
      {/if}
    </div>
  {/snippet}

  <TransactionTable
    {rows}
    rowId={(tx) => String(tx.id)}
    columns={[
      { key: 'deleted', header: m.transactions_deleted_col_deleted(), priority: 2, width: '11rem', cell: deletedCell },
      { key: 'date', header: m.transactions_deleted_col_date(), priority: 1, width: '8rem', cell: dateCell },
      { key: 'payee', header: m.transactions_deleted_col_payee(), priority: 1, cell: payeeCell },
      { key: 'amount', header: m.transactions_deleted_col_amount(), priority: 1, align: 'right', width: '9rem', cell: amountCell },
      { key: 'reason', header: m.transactions_deleted_col_reason(), priority: 3, cell: reasonCell },
      { key: 'actions', header: '', priority: 1, width: '9rem', align: 'right', cell: actionsCell }
    ] satisfies Column<DeletedTransactionResponse>[]}
    isLoading={query.isPending}
    isFetchingNextPage={query.isFetchingNextPage}
    hasNextPage={query.hasNextPage}
    onLoadMore={() => { if (!query.isFetchingNextPage) query.fetchNextPage(); }}
    error={query.error instanceof Error ? query.error : query.error ? new Error(String(query.error)) : null}
    onRetry={() => query.refetch()}
    emptyTitle={m.transactions_deleted_empty_title()}
    emptyCopy={m.transactions_deleted_empty_copy()}
  />
</div>

<!-- Reconciliation override confirmation modal (#225) -->
{#if overrideModal.row}
  <ModalDialog role="alertdialog" labelledby="deleted-override-title" describedby="deleted-override-description"
    pending={overrideModal.pending} onCancel={cancelOverride} class="max-w-md p-6">
    <h3 id="deleted-override-title" class="text-base font-semibold text-foreground">
      {m.transactions_deleted_restore_confirm_title()}
    </h3>
    <p id="deleted-override-description" class="mt-2 text-sm text-muted">{m.transactions_deleted_restore_confirm_description()}</p>

    {#if overrideModal.loadError}
      <div class="mt-3 space-y-2">
        <APIFormError error={overrideModal.loadError} />
        <button
          type="button"
          onclick={() => overrideModal.row && openOverride(overrideModal.row)}
          class="rounded-(--radius-control) border border-border bg-control px-3 py-1.5 text-sm font-semibold text-foreground transition-colors hover:bg-control-hover"
        >
          {m.transactions_retry()}
        </button>
      </div>
    {:else if overrideModal.impacts === null}
      <p class="mt-3 text-sm text-muted" role="status">{m.transactions_deleted_restore_confirm_loading()}</p>
    {:else}
      <ul class="mt-3 divide-y divide-border text-sm">
        {#each overrideModal.impacts as cp (cp.checkpoint_id)}
          <li class="py-2 text-foreground">
            {m.transactions_reconciliation_checkpoint_label({
              account: cp.account_label,
              commodity: cp.commodity_code,
              date: cp.statement_date
            })}
          </li>
        {/each}
      </ul>
    {/if}

    {#if overrideModal.writeError}
      <div class="mt-3"><APIFormError error={overrideModal.writeError} /></div>
    {/if}

    <div class="mt-4 flex flex-wrap justify-end gap-3">
      <button
        type="button"
        onclick={cancelOverride}
        disabled={overrideModal.pending}
        class="rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground transition-colors hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-60"
      >
        {m.transactions_deleted_restore_confirm_cancel()}
      </button>
      <button
        type="button"
        onclick={confirmOverride}
        disabled={overrideModal.impacts === null || overrideModal.pending}
        class="rounded-(--radius-control) bg-danger px-4 py-2 text-sm font-semibold text-background transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
      >
        {overrideModal.pending ? m.transactions_deleted_restoring() : m.transactions_deleted_restore_confirm_proceed()}
      </button>
    </div>
  </ModalDialog>
{/if}
