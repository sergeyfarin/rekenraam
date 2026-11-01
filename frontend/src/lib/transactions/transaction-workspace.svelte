<script lang="ts" module>
  import type { TransactionResponse } from '#lib/api/transactions.ts';
  import type { TransactionRowActionHandler } from './row-action.ts';
  import type { InvestmentPrefill } from '#lib/investments/investment-prefill.ts';

  /** What a screen's list may ask the shared side panel to do. */
  export type TransactionWorkspaceActions = {
    /** Open a loaded transaction's detail. */
    showDetail: (transaction: TransactionResponse) => void;
    /** Load a transaction by ID (a register row carries only its posting), then open its detail. */
    showTransaction: (transactionID: number) => Promise<void>;
    edit: (transaction: TransactionResponse) => void;
    correct: (transaction: TransactionResponse) => void;
    rowAction: TransactionRowActionHandler;
    readonly pending: boolean;
  };

  /**
   * How a screen offers a new transaction. `editor` opens the generic editor
   * with the screen's defaults; `investment` offers the investment record
   * forms pre-filled with the screen's holding or instrument (#186), since the
   * generic editor never writes an investment-linked posting; `none` offers
   * nothing.
   */
  export type TransactionWorkspaceCreate =
    | { kind: 'editor'; defaults?: { accountID?: number; categoryID?: number } }
    | { kind: 'investment'; prefill: InvestmentPrefill }
    | { kind: 'none' };
</script>

<script lang="ts">
  import { page } from '$app/state';
  import { replaceState } from '$app/navigation';
  import type { Snippet } from 'svelte';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import Plus from '@lucide/svelte/icons/plus';
  import { m } from '#lib/paraglide/messages.js';
  import Panel from '#lib/components/panel.svelte';
  import PageHeader from '#lib/components/page-header.svelte';
  import type { Crumb } from '#lib/navigation/routes.ts';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { phoneModal } from '#lib/components/modal-focus.ts';
  import { authSessionQueryOptions } from '#lib/api/auth.ts';
  import { approveTransaction, getTransaction, restoreTransaction } from '#lib/api/transactions.ts';
  import { invalidateTransactionReads } from './invalidate.ts';
  import type { TransactionRowAction } from './row-action.ts';
  import TransactionDetailPanel from './transaction-detail-panel.svelte';
  import TransactionEditor from './transaction-editor.svelte';
  import InvestmentEntry from '#lib/investments/investment-entry.svelte';
  import { providePageAdd } from './add-transaction.svelte.ts';

  // The transaction workspace (#185): one list screen plus the shared side
  // panel for detail, edit, correction, creation and undo of a deletion. The
  // main transactions screen, an account register and a category's
  // transactions all use it, so every screen offers the same actions. The
  // screen supplies its list and how it offers a new transaction. A screen
  // that names a `title` gets the page header with that offer in its action
  // slot (#459); otherwise the offer ends the toolbar row after `header`.
  let {
    title,
    breadcrumbs = [],
    header,
    list,
    create = { kind: 'editor' },
    linkedTransactionID = 0
  }: {
    title?: string;
    breadcrumbs?: Crumb[];
    /** Leading content of the toolbar row, such as a back link and title. */
    header?: Snippet;
    list: Snippet<[TransactionWorkspaceActions, string | undefined]>;
    create?: TransactionWorkspaceCreate;
    /** A transaction to open on arrival (a `transaction_id` link). */
    linkedTransactionID?: number;
  } = $props();

  type PanelState =
    | { type: 'none' }
    | { type: 'detail'; transaction: TransactionResponse; initialAction?: 'unvoid' }
    | { type: 'edit'; transaction: TransactionResponse }
    | { type: 'correct'; transaction: TransactionResponse }
    | { type: 'create' }
    | { type: 'just-deleted'; transaction: TransactionResponse };

  let panel = $state<PanelState>({ type: 'none' });
  let loadingTransactionID = $state<number | undefined>(undefined);
  let actionPending = $state(false);
  let loadError = $state<unknown>();

  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const csrfToken = $derived(sessionQuery.data?.csrf_token);
  const queryClient = useQueryClient();

  // A `transaction_id` link can arrive without remounting this screen: after a
  // save, View transaction points at the same route. Consume each new ID, not
  // just the first one seen on mount.
  let openedLinkedTransactionID = 0;
  $effect(() => {
    const transactionID = linkedTransactionID;
    if (!Number.isSafeInteger(transactionID) || transactionID <= 0) {
      openedLinkedTransactionID = 0;
      return;
    }
    if (transactionID === openedLinkedTransactionID) return;
    openedLinkedTransactionID = transactionID;
    void showTransaction(transactionID);
  });

  // A write may change any list, every account's running balance and the
  // reports and budgets read from them.
  async function invalidate() {
    await invalidateTransactionReads(queryClient);
  }

  async function showTransaction(transactionID: number) {
    loadingTransactionID = transactionID;
    loadError = undefined;
    try {
      const transaction = await getTransaction(transactionID);
      if (loadingTransactionID === transactionID) panel = { type: 'detail', transaction };
    } catch (error) {
      if (loadingTransactionID === transactionID) loadError = error;
    } finally {
      if (loadingTransactionID === transactionID) loadingTransactionID = undefined;
    }
  }

  // Search's Add command opens an unsaved editor, including on this route.
  // Consume the trigger so refresh/Back does not reopen a dismissed working copy.
  $effect(() => {
    const currentURL = page.shallow?.url ?? page.url;
    if (currentURL.searchParams.get('create') === 'transaction' && create.kind === 'editor') {
      panel = { type: 'create' };
      const url = new URL(currentURL.href);
      url.searchParams.delete('create');
      void replaceState(url, page.state);
    }
  });

  // N and search's Add command open this editor in place. They never replace
  // an editor that is already open, which may hold unsaved input.
  $effect(() => {
    if (create.kind !== 'editor') return;
    return providePageAdd(() => {
      if (panel.type !== 'create' && panel.type !== 'edit' && panel.type !== 'correct') panel = { type: 'create' };
    });
  });

  const actions: TransactionWorkspaceActions = {
    showDetail: (transaction) => { panel = { type: 'detail', transaction }; },
    showTransaction,
    edit: (transaction) => { panel = { type: 'edit', transaction }; },
    correct: (transaction) => { panel = { type: 'correct', transaction }; },
    rowAction: (action, transactionID) => { void handleRowAction(action, transactionID); },
    get pending() { return actionPending; }
  };

  // List summaries are enough for buttons. Fetch the complete transaction only
  // after a user asks to edit/correct/view it; never fan out per register row.
  async function handleRowAction(action: TransactionRowAction, transactionID: number) {
    if (actionPending || !csrfToken) return;
    actionPending = true;
    loadingTransactionID = undefined;
    loadError = undefined;
    try {
      if (action === 'approve') {
        await approveTransaction(transactionID, csrfToken);
        await invalidate();
        if (panel.type === 'detail' && panel.transaction.id === transactionID) panel = { type: 'none' };
      } else {
        const transaction = await getTransaction(transactionID);
        if ((action === 'edit' || action === 'correct' || action === 'unvoid') && transaction.transaction_kind === 'investment') {
          panel = { type: 'detail', transaction };
        } else if (action === 'edit' || action === 'correct') {
          panel = { type: action, transaction };
        } else {
          panel = { type: 'detail', transaction, initialAction: action === 'unvoid' ? 'unvoid' : undefined };
        }
      }
    } catch (error) {
      loadError = error;
    } finally {
      actionPending = false;
    }
  }

  async function handleSaved(saved: TransactionResponse) {
    await invalidate();
    panel = { type: 'detail', transaction: saved };
  }

  // Cancelling an edit or correction returns to the transaction's detail.
  function handleCancel() {
    panel = panel.type === 'edit' || panel.type === 'correct'
      ? { type: 'detail', transaction: panel.transaction }
      : { type: 'none' };
  }

  async function handleRefresh() {
    await invalidate();
    panel = { type: 'none' };
  }

  function handleSoftDeleted(transaction: TransactionResponse) {
    // A brief undo affordance before the panel closes.
    panel = { type: 'just-deleted', transaction };
    setTimeout(async () => {
      if (panel.type === 'just-deleted') panel = { type: 'none' };
      await invalidate();
    }, 5000);
  }

  async function handleUndoDelete(transaction: TransactionResponse) {
    if (!csrfToken) return;
    try {
      // Undo restores exactly what was just deleted, not a later version (#296).
      await restoreTransaction(transaction.id, csrfToken, { expected_version_id: transaction.version_id });
      await invalidate();
    } catch {
      // It stays deleted; Transactions → Deleted can still restore it.
    }
    panel = { type: 'none' };
  }

  async function closeDeleted() {
    panel = { type: 'none' };
    await invalidate();
  }
</script>

{#snippet createAction()}
  {#if create.kind === 'editor'}
    <button
      type="button"
      class="inline-flex min-h-11 items-center gap-2 rounded-[var(--radius-control)] bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90"
      onclick={() => (panel = { type: 'create' })}
    >
      <Plus size={16} aria-hidden="true" />
      {m.transactions_form_create_title()}
    </button>
  {:else if create.kind === 'investment'}
    <div class="flex max-w-full flex-wrap items-center justify-end gap-2">
      <InvestmentEntry layout="menu" prefill={create.prefill} />
    </div>
  {/if}
{/snippet}

{#if title}
  <PageHeader {title} {breadcrumbs} actions={create.kind === 'none' ? undefined : createAction} />
{/if}

<div class="lg:grid lg:grid-cols-[minmax(0,1fr)_22rem] lg:gap-6 xl:grid-cols-[minmax(0,1fr)_26rem]">
  <div class="min-w-0">
    {#if (loadingTransactionID !== undefined || actionPending) && panel.type === 'none'}
      <p role="status" class="mb-4 text-sm text-muted">{m.transactions_linked_loading()}</p>
    {:else if loadError}
      <div class="mb-4"><APIFormError error={loadError} /></div>
    {/if}
    {#if header || (!title && create.kind !== 'none')}
      <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div class="flex min-w-0 flex-wrap items-center gap-3">
          {@render header?.()}
        </div>
        {#if !title}{@render createAction()}{/if}
      </div>
    {/if}

    {@render list(actions, csrfToken)}
  </div>

  {#if panel.type !== 'none'}
    <!-- Mobile: a full-screen modal overlay (#342). Desktop: the right-hand column. -->
    <aside
      class="fixed inset-0 z-30 overflow-y-auto bg-surface outline-none lg:relative lg:inset-auto lg:z-auto lg:overflow-visible"
      aria-label={m.transactions_detail_actions()}
      {@attach phoneModal({ onCancel: () => (panel = { type: 'none' }), pending: () => actionPending })}
    >
      <div class="fixed inset-0 bg-background/50 lg:hidden" role="presentation" onclick={() => (panel = { type: 'none' })}></div>

      <div class="relative lg:sticky lg:top-4">
        <Panel>
          {#if panel.type === 'detail'}
            {#key panel}
              <TransactionDetailPanel
                initialAction={panel.initialAction}
                transaction={panel.transaction}
                {csrfToken}
                onClose={() => (panel = { type: 'none' })}
                onEdit={actions.edit}
                onCreateCorrection={actions.correct}
                onSoftDeleted={handleSoftDeleted}
                onRefresh={handleRefresh}
              />
            {/key}
          {:else if panel.type === 'edit'}
            <TransactionEditor mode="edit" transaction={panel.transaction} {csrfToken} onSaved={handleSaved} onCancel={handleCancel} />
          {:else if panel.type === 'correct'}
            <TransactionEditor mode="correct" transaction={panel.transaction} {csrfToken} onSaved={handleSaved} onCancel={handleCancel} />
          {:else if panel.type === 'create'}
            <TransactionEditor
              mode="create"
              {csrfToken}
              createDefaults={create.kind === 'editor' ? create.defaults : undefined}
              onSaved={handleSaved}
              onCancel={() => (panel = { type: 'none' })}
            />
          {:else if panel.type === 'just-deleted'}
            {@const deleted = panel.transaction}
            <div class="space-y-3 py-2">
              <p class="text-sm text-muted">{m.transactions_detail_just_deleted()}</p>
              <div class="flex gap-2">
                <button
                  type="button"
                  class="inline-flex items-center gap-2 rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
                  onclick={() => handleUndoDelete(deleted)}
                >
                  {m.transactions_detail_undo()}
                </button>
                <button
                  type="button"
                  class="inline-flex items-center gap-2 rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-muted transition hover:bg-control-hover"
                  onclick={closeDeleted}
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
