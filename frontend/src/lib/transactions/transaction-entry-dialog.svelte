<script lang="ts">
  import { onDestroy } from 'svelte';
  import { afterNavigate } from '$app/navigation';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import X from '@lucide/svelte/icons/x';
  import ModalDialog from '#lib/components/modal-dialog.svelte';
  import { authSessionQueryOptions } from '#lib/api/auth.ts';
  import type { TransactionResponse } from '#lib/api/transactions.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { closeTransactionEntry, transactionEntry } from './add-transaction.svelte.ts';
  import { invalidateTransactionReads } from './invalidate.ts';
  import TransactionEditor from './transaction-editor.svelte';

  // The shared Add transaction dialog (#477): the existing editor, its locale
  // parsing, guards and error recovery, in a dialog on desktop and a
  // full-screen sheet on phones. The shell mounts it once, outside every page,
  // and outside the shell's error state, so a background refresh of Home, any
  // page or the session leaves the working copy alone; the editor itself reads
  // its prefill once (#433). Cancel and Escape discard
  // it; the dialog returns focus to the control that opened it.
  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const queryClient = useQueryClient();

  async function saved(transaction: TransactionResponse) {
    // The page beneath (a report, a budget month, balances) refetches too.
    await invalidateTransactionReads(queryClient);
    closeTransactionEntry(transaction);
  }

  // The opener can leave while the dialog is open: a shell error and recovery
  // re-render the page behind it. Focus then returns to the page's main
  // content, or to the shell error's own controls while it shows.
  function fallbackFocus() {
    return document.getElementById('main-content') ?? document.querySelector<HTMLElement>('main button, main a[href]');
  }

  afterNavigate(() => {
    transactionEntry.saved = undefined;
  });

  // The open flag never outlives the dialog: after signing out, the next
  // session must not reopen an empty entry.
  onDestroy(() => closeTransactionEntry());
</script>

{#if transactionEntry.open}
  <ModalDialog
    label={m.transactions_form_create_title()}
    onCancel={() => closeTransactionEntry()}
    {fallbackFocus}
    class="max-w-2xl p-4 sm:p-6 max-md:m-0! max-md:h-dvh max-md:max-h-dvh! max-md:w-full! max-md:max-w-none! max-md:rounded-none! max-md:border-0"
  >
    <TransactionEditor
      mode="create"
      csrfToken={sessionQuery.data?.csrf_token}
      createDefaults={transactionEntry.defaults}
      onSaved={saved}
      onCancel={() => closeTransactionEntry()}
    />
  </ModalDialog>
{/if}

<!-- Mounted before it has text, so the confirmation is announced. -->
<p role="status" class="sr-only">{transactionEntry.saved ? m.transactions_entry_saved() : ''}</p>
{#if transactionEntry.saved}
  {@const added = transactionEntry.saved}
  <div class="fixed inset-x-4 bottom-4 z-40 mx-auto flex max-w-md items-center gap-3 rounded-(--radius-panel) border border-border bg-surface py-2 pr-2 pl-4 text-sm shadow-(--shadow-panel)">
    <span aria-hidden="true" class="min-w-0 flex-1">{m.transactions_entry_saved()}</span>
    <a href={`/app/ledger/transactions?transaction_id=${added.id}`} class="inline-flex min-h-11 items-center px-2 font-semibold text-accent underline-offset-4 hover:underline">{m.transactions_entry_view()}</a>
    <button type="button" aria-label={m.transactions_detail_close()} onclick={() => (transactionEntry.saved = undefined)} class="inline-flex min-h-11 min-w-11 items-center justify-center rounded-full text-muted hover:bg-control-hover hover:text-foreground">
      <X size={16} aria-hidden="true" />
    </button>
  </div>
{/if}
