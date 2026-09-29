<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { parseISO } from 'date-fns';
  import AlertTriangle from '@lucide/svelte/icons/triangle-alert';
  import { m } from '$lib/paraglide/messages.js';
  import { getLocale } from '$lib/paraglide/runtime.js';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import BuyForm from '$lib/investments/buy-form.svelte';
  import SellForm from '$lib/investments/sell-form.svelte';
  import {
    getInvestmentCorrectionChain,
    getInvestmentTradeCorrectionContext,
    investmentCorrectionChainQueryKey,
    previewBuyReversalReconciliation,
    previewSaleReversalReconciliation,
    reverseManualBuy,
    reverseManualSale,
    type ReconciliationImpactResponse
  } from '$lib/api/investments';

  let {
    transactionID,
    csrfToken,
    onRefresh
  }: {
    transactionID: number;
    csrfToken?: string;
    onRefresh?: () => void;
  } = $props();

  const chainQuery = createQuery(() => ({
    queryKey: [...investmentCorrectionChainQueryKey, transactionID],
    queryFn: () => getInvestmentCorrectionChain(transactionID),
    enabled: transactionID > 0
  }));

  let replacementKind = $state<'buy' | 'sell' | null>(null);
  const replacementQuery = createQuery(() => ({
    queryKey: [...investmentCorrectionChainQueryKey, 'source', transactionID],
    queryFn: () => getInvestmentTradeCorrectionContext(transactionID),
    enabled: replacementKind !== null && transactionID > 0
  }));

  let modal = $state<'closed' | 'reason' | 'reconciliation'>('closed');
  let reversalKind = $state<'buy' | 'sale'>('sale');
  let reason = $state('');
  let pending = $state(false);
  let actionError = $state<unknown>(undefined);
  let impacts = $state<ReconciliationImpactResponse['affected_checkpoints']>([]);
  let reasonInputElement: HTMLInputElement | undefined = $state();
  let confirmButtonElement: HTMLButtonElement | undefined = $state();

  $effect(() => {
    if (modal === 'reason') reasonInputElement?.focus();
    if (modal === 'reconciliation') confirmButtonElement?.focus();
  });

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && modal !== 'closed' && !pending) closeModal();
    else if (event.key === 'Escape' && replacementKind !== null) replacementKind = null;
  }

  const locale = $derived(getLocale());
  const dateFormatter = $derived(new Intl.DateTimeFormat(locale, {
    year: 'numeric', month: 'short', day: 'numeric'
  }));

  function formatDate(value: string): string {
    return dateFormatter.format(parseISO(value));
  }

  function closeModal() {
    modal = 'closed';
    actionError = undefined;
    impacts = [];
  }

  async function submitReason() {
    if (!csrfToken || !reason.trim()) return;
    pending = true;
    actionError = undefined;
    try {
      const preview = reversalKind === 'buy'
        ? await previewBuyReversalReconciliation(transactionID, { reason: reason.trim() })
        : await previewSaleReversalReconciliation(transactionID, { reason: reason.trim() });
      if (preview.affected_checkpoints.length > 0) {
        impacts = preview.affected_checkpoints;
        modal = 'reconciliation';
      } else {
        if (reversalKind === 'buy') {
          await reverseManualBuy(transactionID, { reason: reason.trim() }, csrfToken);
        } else {
          await reverseManualSale(transactionID, { reason: reason.trim() }, csrfToken);
        }
        closeModal();
        onRefresh?.();
      }
    } catch (error) {
      actionError = error;
      modal = 'reason';
    } finally {
      pending = false;
    }
  }

  async function confirmReconciliation() {
    if (!csrfToken || !reason.trim()) return;
    pending = true;
    actionError = undefined;
    try {
      if (reversalKind === 'buy') {
        await reverseManualBuy(transactionID, {
          reason: reason.trim(), reconciliation_override: true
        }, csrfToken);
      } else {
        await reverseManualSale(transactionID, {
          reason: reason.trim(), reconciliation_override: true
        }, csrfToken);
      }
      closeModal();
      onRefresh?.();
    } catch (error) {
      actionError = error;
    } finally {
      pending = false;
    }
  }

  function replacementSaved() {
    replacementKind = null;
    void chainQuery.refetch();
    onRefresh?.();
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<section class="space-y-3 border-t border-border pt-4" aria-labelledby="investment-correction-heading">
  <h3 id="investment-correction-heading" class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">
    {m.transactions_investment_history_title()}
  </h3>

  {#if chainQuery.isPending}
    <p class="text-sm text-muted" role="status">{m.transactions_investment_history_loading()}</p>
  {:else if chainQuery.isError}
    <div class="space-y-2">
      <APIFormError error={chainQuery.error} />
      <button type="button" class="text-sm font-semibold text-accent" onclick={() => chainQuery.refetch()}>
        {m.transactions_retry()}
      </button>
    </div>
  {:else if chainQuery.data && chainQuery.data.operations.length > 0}
    <ol class="space-y-2">
      {#each chainQuery.data.operations as node, index (node.operation_id)}
        <li class="rounded-[var(--radius-control)] border border-border bg-surface px-3 py-2 text-sm">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <span class="font-medium text-foreground">
              {#if index === 0}
                {m.transactions_investment_history_original()}
              {:else if node.correction_mode === 'reverse'}
                {m.transactions_investment_history_reversal()}
              {:else}
                {m.transactions_investment_history_replacement()}
              {/if}
              {#if node.transaction_id} · #{node.transaction_id}{/if}
            </span>
            <span class="text-xs text-muted">
              {#if node.effective}
                {m.transactions_investment_history_current()}
              {:else if node.correction_mode === 'reverse' || chainQuery.data.operations.at(-1)?.correction_mode === 'reverse'}
                {m.transactions_investment_history_reversed()}
              {:else}
                {m.transactions_investment_history_superseded()}
              {/if}
            </span>
          </div>
          <p class="mt-1 text-xs text-muted">{formatDate(node.event_date)}</p>
          {#if node.correction_reason}
            <p class="mt-1 text-xs text-foreground">{node.correction_reason}</p>
          {/if}
        </li>
      {/each}
    </ol>
    {#if chainQuery.data.operations.some((node) => node.imported)}
      <p class="text-xs text-muted">{m.transactions_investment_history_imported()}</p>
    {/if}
    {#if chainQuery.data.can_reverse_manual_sale && chainQuery.data.effective_transaction_id === transactionID}
      <button
        type="button"
        class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-warning/50 bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
        disabled={!csrfToken || pending}
        onclick={() => { reversalKind = 'sale'; reason = ''; actionError = undefined; modal = 'reason'; }}
      >
        {m.transactions_investment_reverse_action()}
      </button>
    {/if}
    {#if chainQuery.data.can_reverse_manual_buy && chainQuery.data.effective_transaction_id === transactionID}
      <button
        type="button"
        class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-warning/50 bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
        disabled={!csrfToken || pending}
        onclick={() => { reversalKind = 'buy'; reason = ''; actionError = undefined; modal = 'reason'; }}
      >
        {m.transactions_investment_reverse_buy_action()}
      </button>
    {/if}
    {#if chainQuery.data.effective_transaction_id === transactionID &&
      chainQuery.data.operations.some((node) => node.transaction_id === transactionID && node.operation_kind === 'buy' && node.effective)}
      <button type="button"
        class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
        disabled={!csrfToken}
        onclick={() => { replacementKind = 'buy'; }}>
        {m.transactions_investment_replace_buy_action()}
      </button>
    {/if}
    {#if chainQuery.data.effective_transaction_id === transactionID &&
      chainQuery.data.operations.some((node) => node.transaction_id === transactionID && node.operation_kind === 'sell' && node.effective)}
      <button type="button"
        class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
        disabled={!csrfToken}
        onclick={() => { replacementKind = 'sell'; }}>
        {m.transactions_investment_replace_sale_action()}
      </button>
    {/if}
  {:else}
    <p class="text-sm text-muted">{m.transactions_investment_history_empty()}</p>
  {/if}
</section>

{#if replacementKind !== null}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-3 py-4 backdrop-blur-sm"
    role="presentation">
    <div class="max-h-full w-full max-w-2xl overflow-y-auto rounded-[var(--radius-panel)] border border-border bg-surface p-4 shadow-[var(--shadow-panel)] sm:p-6"
      role="dialog" aria-modal="true" aria-label={replacementKind === 'buy'
        ? m.transactions_investment_replace_buy_title() : m.transactions_investment_replace_sale_title()}>
      {#if replacementQuery.isPending}
        <p class="text-sm text-muted" role="status">{m.transactions_investment_replace_loading()}</p>
      {:else if replacementQuery.isError}
        <APIFormError error={replacementQuery.error} />
        <button type="button" class="mt-2 text-sm font-semibold text-accent" onclick={() => replacementQuery.refetch()}>{m.transactions_retry()}</button>
      {:else if replacementQuery.data?.operation_kind === 'buy' && replacementKind === 'buy' && (!replacementQuery.data.imported || replacementQuery.data.source_identity_id > 0) && !replacementQuery.data.already_corrected && csrfToken}
        <BuyForm {csrfToken} correction={replacementQuery.data} onSaved={replacementSaved} onCancel={() => (replacementKind = null)} />
      {:else if replacementQuery.data?.operation_kind === 'sell' && replacementKind === 'sell' && replacementQuery.data.can_replace_sale && !replacementQuery.data.already_corrected && csrfToken}
        <SellForm {csrfToken} correction={replacementQuery.data} onSaved={replacementSaved} onCancel={() => (replacementKind = null)} />
      {:else}
        <p class="text-sm text-muted">{m.transactions_investment_replace_unavailable()}</p>
      {/if}
      {#if replacementQuery.isPending || replacementQuery.isError || replacementQuery.data?.operation_kind !== replacementKind || (replacementQuery.data.imported && replacementQuery.data.source_identity_id === 0) || replacementQuery.data.already_corrected || (replacementKind === 'sell' && !replacementQuery.data.can_replace_sale) || !csrfToken}
        <button type="button" class="mt-4 min-h-10 rounded-[var(--radius-control)] border border-border bg-control px-4 text-sm font-semibold text-foreground"
          onclick={() => (replacementKind = null)}>{m.investments_form_cancel()}</button>
      {/if}
    </div>
  </div>
{/if}

{#if modal !== 'closed'}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-4 py-6 backdrop-blur-sm">
    <div class="w-full max-w-lg rounded-[var(--radius-panel)] border border-border bg-surface shadow-[var(--shadow-panel)]"
      role="alertdialog" aria-modal="true" aria-labelledby="investment-reversal-title">
      <div class="border-b border-border px-4 py-3">
        <h3 id="investment-reversal-title" class="text-sm font-semibold text-foreground">
          {modal === 'reason'
            ? reversalKind === 'buy' ? m.transactions_investment_reverse_buy_title() : m.transactions_investment_reverse_title()
            : m.transactions_reconciliation_warning_title()}
        </h3>
        <p class="mt-1 text-xs leading-5 text-muted">
          {modal === 'reason'
            ? reversalKind === 'buy' ? m.transactions_investment_reverse_buy_copy() : m.transactions_investment_reverse_copy()
            : m.transactions_reconciliation_warning_copy()}
        </p>
      </div>

      {#if modal === 'reason'}
        <div class="px-4 py-3">
          <label for="investment-reversal-reason" class="block text-xs font-semibold uppercase tracking-[0.12em] text-muted">
            {m.transactions_investment_reverse_reason()}
          </label>
          <input id="investment-reversal-reason" type="text" bind:this={reasonInputElement} bind:value={reason} maxlength="500" required
            class="mt-1.5 h-10 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 text-sm text-foreground outline-none focus:border-accent" />
        </div>
      {:else}
        <ul class="divide-y divide-border px-4 py-3 text-sm">
          {#each impacts as checkpoint (checkpoint.checkpoint_id)}
            <li class="flex gap-2 py-2 text-foreground">
              <AlertTriangle size={16} class="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
              {m.transactions_reconciliation_checkpoint_label({
                account: checkpoint.account_label,
                commodity: checkpoint.commodity_code,
                date: checkpoint.statement_date
              })}
            </li>
          {/each}
        </ul>
      {/if}

      <div class="px-4 pb-2"><APIFormError error={actionError} /></div>
      <div class="flex flex-wrap justify-end gap-2 border-t border-border px-4 py-3">
        <button type="button" class="min-h-10 rounded-[var(--radius-control)] border border-border bg-control px-4 text-sm font-semibold text-foreground"
          disabled={pending} onclick={closeModal}>{m.transactions_reconciliation_cancel()}</button>
        <button type="button" bind:this={confirmButtonElement} class="min-h-10 rounded-[var(--radius-control)] bg-warning px-4 text-sm font-semibold text-warning-foreground disabled:opacity-60"
          disabled={pending || (modal === 'reason' && !reason.trim())}
          onclick={modal === 'reason' ? submitReason : confirmReconciliation}>
          {modal === 'reason' ? m.transactions_investment_reverse_confirm() : m.transactions_reconciliation_confirm()}
        </button>
      </div>
    </div>
  </div>
{/if}
