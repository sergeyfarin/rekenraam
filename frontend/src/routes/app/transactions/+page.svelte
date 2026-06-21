<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import Plus from '@lucide/svelte/icons/plus';
  import { m } from '$lib/paraglide/messages.js';
  import Panel from '$lib/components/panel.svelte';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import { authSessionQueryOptions } from '$lib/api/auth';
  import { transactionsQueryKey, type TransactionResponse } from '$lib/api/transactions';
  import TransactionList from '$lib/transactions/transaction-list.svelte';
  import TransactionDetailPanel from '$lib/transactions/transaction-detail-panel.svelte';
  import TransactionEditor from '$lib/transactions/transaction-editor.svelte';

  // ── Panel state ───────────────────────────────────────────────────
  type PanelState =
    | { type: 'none' }
    | { type: 'detail'; transaction: TransactionResponse }
    | { type: 'edit'; transaction: TransactionResponse }
    | { type: 'correct'; transaction: TransactionResponse }
    | { type: 'create' }
    | { type: 'just-deleted'; transaction: TransactionResponse };

  let panel = $state<PanelState>({ type: 'none' });

  // ── Session ───────────────────────────────────────────────────────
  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const csrfToken = $derived(sessionQuery.data?.csrf_token);

  const queryClient = useQueryClient();

  // Invalidate the transactions query so the list refetches after mutations.
  async function invalidateTransactions() {
    await queryClient.invalidateQueries({ queryKey: transactionsQueryKey });
  }

  // ── Row click: open detail panel ──────────────────────────────────
  function handleRowClick(tx: TransactionResponse) {
    panel = { type: 'detail', transaction: tx };
  }

  // ── Editor callbacks ──────────────────────────────────────────────
  async function handleSaved(saved: TransactionResponse) {
    await invalidateTransactions();
    panel = { type: 'detail', transaction: saved };
  }

  function handleCancel() {
    // Return to detail view if we were editing; otherwise close.
    if (panel.type === 'edit' || panel.type === 'correct') {
      // We came from detail — go back to detail with original transaction.
      // If we have the transaction object on the state, use it.
      const tx = panel.transaction;
      panel = { type: 'detail', transaction: tx };
    } else {
      panel = { type: 'none' };
    }
  }

  // ── List actions ──────────────────────────────────────────────────
  function handleEdit(tx: TransactionResponse) {
    panel = { type: 'edit', transaction: tx };
  }

  function handleCreateCorrection(tx: TransactionResponse) {
    panel = { type: 'correct', transaction: tx };
  }

  async function handleApprove(tx: TransactionResponse) {
    await invalidateTransactions();
    // If detail panel is open for this tx, refresh it.
    if (panel.type === 'detail' && panel.transaction.id === tx.id) {
      panel = { type: 'none' };
    }
  }

  async function handleUnvoid(tx: TransactionResponse) {
    await invalidateTransactions();
    panel = { type: 'none' };
  }

  // ── Detail panel callbacks ────────────────────────────────────────
  async function handleRefresh() {
    await invalidateTransactions();
    panel = { type: 'none' };
  }

  function handleSoftDeleted(tx: TransactionResponse) {
    // Show brief undo affordance before closing.
    panel = { type: 'just-deleted', transaction: tx };
    // Auto-dismiss after 5 seconds.
    setTimeout(async () => {
      if (panel.type === 'just-deleted') {
        panel = { type: 'none' };
      }
      await invalidateTransactions();
    }, 5000);
  }

  async function handleUndoDelete(tx: TransactionResponse) {
    if (!csrfToken) return;
    try {
      const { restoreTransaction } = await import('$lib/api/transactions');
      await restoreTransaction(tx.id, csrfToken, {});
      await invalidateTransactions();
    } catch {
      // Ignore — the transaction stays deleted; user can use Settings → Trash.
    }
    panel = { type: 'none' };
  }
</script>

<div class="lg:grid lg:grid-cols-[minmax(0,1fr)_22rem] lg:gap-6 xl:grid-cols-[minmax(0,1fr)_26rem]">
  <!-- Main list -->
  <div class="min-w-0">
    <div class="mb-4 flex items-center justify-between">
      <div><!-- spacer --></div>
      <button
        type="button"
        class="inline-flex items-center gap-2 rounded-[var(--radius-control)] bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90"
        onclick={() => (panel = { type: 'create' })}
      >
        <Plus size={16} aria-hidden="true" />
        {m.transactions_new()}
      </button>
    </div>

    <TransactionList
      {csrfToken}
      onRowClick={handleRowClick}
      onEdit={handleEdit}
      onCreateCorrection={handleCreateCorrection}
      onApprove={handleApprove}
      onUnvoid={handleUnvoid}
    />
  </div>

  <!-- Side panel (hidden on mobile when no panel is active) -->
  {#if panel.type !== 'none'}
    <!--
      Mobile: full-screen overlay below the header.
      Desktop: right column, fixed position within the layout column.
    -->
    <aside
      class="fixed inset-0 z-30 overflow-y-auto bg-surface lg:relative lg:inset-auto lg:z-auto lg:overflow-visible"
      aria-label={m.transactions_detail_actions()}
    >
      <!-- Mobile backdrop -->
      <div
        class="fixed inset-0 bg-background/50 lg:hidden"
        role="presentation"
        onclick={() => (panel = { type: 'none' })}
      ></div>

      <div class="relative lg:sticky lg:top-4">
        <Panel>
          {#if panel.type === 'detail'}
            <TransactionDetailPanel
              transaction={panel.transaction}
              {csrfToken}
              onClose={() => (panel = { type: 'none' })}
              onEdit={handleEdit}
              onCreateCorrection={handleCreateCorrection}
              onSoftDeleted={handleSoftDeleted}
              onRefresh={handleRefresh}
            />
          {:else if panel.type === 'edit'}
            <TransactionEditor
              mode="edit"
              transaction={panel.transaction}
              {csrfToken}
              onSaved={handleSaved}
              onCancel={handleCancel}
            />
          {:else if panel.type === 'correct'}
            <TransactionEditor
              mode="correct"
              transaction={panel.transaction}
              {csrfToken}
              onSaved={handleSaved}
              onCancel={handleCancel}
            />
          {:else if panel.type === 'create'}
            <TransactionEditor
              mode="create"
              {csrfToken}
              onSaved={handleSaved}
              onCancel={() => (panel = { type: 'none' })}
            />
          {:else if panel.type === 'just-deleted'}
            <div class="space-y-3 py-2">
              <p class="text-sm text-muted">{m.transactions_detail_just_deleted()}</p>
              <div class="flex gap-2">
                <button
                  type="button"
                  class="inline-flex items-center gap-2 rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
                  onclick={() => handleUndoDelete(panel.type === 'just-deleted' ? panel.transaction : { id: 0 } as TransactionResponse)}
                >
                  {m.transactions_detail_undo()}
                </button>
                <button
                  type="button"
                  class="inline-flex items-center gap-2 rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-muted transition hover:bg-control-hover"
                  onclick={async () => { panel = { type: 'none' }; await invalidateTransactions(); }}
                >
                  {m.transactions_detail_close()}
                </button>
              </div>
            </div>
          {/if}
        </Panel>
      </div>
    </aside>
  {/if}
</div>
