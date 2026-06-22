<script lang="ts">
  import { page } from '$app/stores';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import ArrowLeft from '@lucide/svelte/icons/arrow-left';
  import { m } from '$lib/paraglide/messages.js';
  import Panel from '$lib/components/panel.svelte';
  import { authSessionQueryOptions } from '$lib/api/auth';
  import { accountsQueryOptions } from '$lib/api/accounts';
  import {
    transactionsQueryKey,
    accountRegisterQueryKey,
    type AccountRegisterEntryResponse
  } from '$lib/api/transactions';
  import { getTransaction, type TransactionResponse } from '$lib/api/transactions';
  import AccountRegister from '$lib/transactions/account-register.svelte';
  import TransactionDetailPanel from '$lib/transactions/transaction-detail-panel.svelte';
  import TransactionEditor from '$lib/transactions/transaction-editor.svelte';

  // ── Route param ───────────────────────────────────────────────────
  const accountID = $derived(Number($page.params.id));

  // ── Session + CSRF ────────────────────────────────────────────────
  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const csrfToken = $derived(sessionQuery.data?.csrf_token);

  // ── Account name for breadcrumb ───────────────────────────────────
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const account = $derived(
    accountsQuery.data?.accounts.find((a) => a.id === accountID)
  );
  const accountName = $derived(account?.name ?? account?.code ?? String(accountID));

  // ── Panel state ───────────────────────────────────────────────────
  type PanelState =
    | { type: 'none' }
    | { type: 'detail'; transaction: TransactionResponse }
    | { type: 'edit'; transaction: TransactionResponse }
    | { type: 'correct'; transaction: TransactionResponse }
    | { type: 'just-deleted'; transaction: TransactionResponse };

  let panel = $state<PanelState>({ type: 'none' });

  const queryClient = useQueryClient();

  // Invalidate both the global transactions query and this account's register.
  async function invalidate() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: transactionsQueryKey }),
      queryClient.invalidateQueries({ queryKey: [...accountRegisterQueryKey, accountID] })
    ]);
  }

  // ── Register row click: load the full transaction then open detail ─
  let loadingEntryID = $state<number | undefined>(undefined);

  async function handleRowClick(entry: AccountRegisterEntryResponse) {
    loadingEntryID = entry.transaction_id;
    try {
      const tx = await getTransaction(entry.transaction_id);
      panel = { type: 'detail', transaction: tx };
    } catch {
      // If the fetch fails just ignore — the user can retry by clicking again.
    } finally {
      loadingEntryID = undefined;
    }
  }

  // ── Editor callbacks ──────────────────────────────────────────────
  async function handleSaved(saved: TransactionResponse) {
    await invalidate();
    panel = { type: 'detail', transaction: saved };
  }

  function handleCancel() {
    if (panel.type === 'edit' || panel.type === 'correct') {
      const tx = panel.transaction;
      panel = { type: 'detail', transaction: tx };
    } else {
      panel = { type: 'none' };
    }
  }

  function handleEdit(tx: TransactionResponse) {
    panel = { type: 'edit', transaction: tx };
  }

  function handleCreateCorrection(tx: TransactionResponse) {
    panel = { type: 'correct', transaction: tx };
  }

  function handleSoftDeleted(tx: TransactionResponse) {
    panel = { type: 'just-deleted', transaction: tx };
    setTimeout(async () => {
      if (panel.type === 'just-deleted') {
        panel = { type: 'none' };
      }
      await invalidate();
    }, 5000);
  }

  async function handleRefresh() {
    await invalidate();
    panel = { type: 'none' };
  }

  async function handleUndoDelete(tx: TransactionResponse) {
    if (!csrfToken) return;
    try {
      const { restoreTransaction } = await import('$lib/api/transactions');
      await restoreTransaction(tx.id, csrfToken, {});
      await invalidate();
    } catch {
      // Stays deleted; recoverable from Settings → Trash.
    }
    panel = { type: 'none' };
  }
</script>

<!-- Breadcrumb / back nav -->
<div class="mb-4 flex items-center gap-3">
  <a
    href="/app/accounts"
    class="inline-flex items-center gap-1.5 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
    aria-label={m.register_back_to_accounts()}
  >
    <ArrowLeft size={14} aria-hidden="true" />
    {m.register_back_to_accounts()}
  </a>
  {#if accountName}
    <span class="text-sm font-semibold text-foreground">{accountName}</span>
  {/if}
</div>

<div class="lg:grid lg:grid-cols-[minmax(0,1fr)_22rem] lg:gap-6 xl:grid-cols-[minmax(0,1fr)_26rem]">
  <!-- Register -->
  <div class="min-w-0">
    <AccountRegister
      {accountID}
      {csrfToken}
      onRowClick={handleRowClick}
    />
  </div>

  <!-- Loading indicator while fetching a clicked transaction -->
  {#if loadingEntryID !== undefined && panel.type === 'none'}
    <aside class="hidden lg:block" aria-live="polite" aria-busy="true">
      <div class="lg:sticky lg:top-4">
        <Panel>
          <div class="flex items-center gap-3 py-4">
            <span class="h-5 w-5 animate-spin rounded-full border-2 border-border border-t-accent" aria-hidden="true"></span>
            <span class="text-sm text-muted">{m.register_loading_title()}</span>
          </div>
        </Panel>
      </div>
    </aside>
  {/if}

  <!-- Side panel (hidden on mobile when no panel is active) -->
  {#if panel.type !== 'none'}
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
                  onclick={async () => { panel = { type: 'none' }; await invalidate(); }}
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
