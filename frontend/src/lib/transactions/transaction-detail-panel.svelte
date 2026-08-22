<script lang="ts">
  import X from '@lucide/svelte/icons/x';
  import AlertTriangle from '@lucide/svelte/icons/triangle-alert';
  import { m } from '$lib/paraglide/messages.js';
  import { getLocale } from '$lib/paraglide/runtime.js';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import StatusBadge from '$lib/components/status-badge.svelte';
  import {
    voidTransaction,
    unvoidTransaction,
    softDeleteTransaction,
    restoreTransaction,
    approveTransaction,
    reconciliationImpactForUpdate,
    type TransactionResponse,
    type ReconciliationImpactResponse,
    type TransactionLifecycleRequest
  } from '$lib/api/transactions';
  import { APIClientError } from '$lib/api/client';
  import { formatQuantity } from '$lib/money/format';
  import {
    formatSignedAmount,
    resolveAccountLabel,
    statusLabel,
    statusTone,
    commodityDisplay
  } from './transaction-labels';
  import type { AccountClass } from './transaction-labels';

  // ── Props ─────────────────────────────────────────────────────────
  let {
    transaction,
    csrfToken,
    onClose,
    onEdit,
    onCreateCorrection,
    onSoftDeleted,
    onRefresh
  }: {
    transaction: TransactionResponse;
    csrfToken?: string;
    onClose: () => void;
    onEdit: (tx: TransactionResponse) => void;
    onCreateCorrection: (tx: TransactionResponse) => void;
    onSoftDeleted?: (tx: TransactionResponse) => void;
    onRefresh?: () => void;
  } = $props();

  // ── Lifecycle action modal state ──────────────────────────────────
  type ModalKind = 'void' | 'unvoid' | 'soft-delete' | 'restore' | null;

  let activeModal = $state<ModalKind>(null);
  let reasonInput = $state('');
  let actionPending = $state(false);
  let actionError = $state<unknown>(undefined);

  // Reconciliation override modal (for void/unvoid/restore that hit the period guard)
  let reconciliationModal = $state<{
    open: boolean;
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    pendingKind: 'void' | 'unvoid';
    pendingReason: string;
  }>({ open: false, impacts: [], pendingKind: 'void', pendingReason: '' });

  const locale = $derived(getLocale());
  const isPosted = $derived(transaction.status === 'posted');
  const isVoided = $derived(transaction.status === 'voided');

  // ── Display helpers ───────────────────────────────────────────────
  const allPostings = $derived(
    transaction.journal_entries?.flatMap((e) => e.postings ?? []) ?? []
  );

  const visiblePostings = $derived(
    allPostings.filter((p) => !p.account_system_role)
  );

  const kindLabel = $derived.by(() => {
    switch (transaction.transaction_kind) {
      case 'ordinary': return m.transaction_kind_ordinary();
      case 'transfer': return m.transaction_kind_transfer();
      case 'investment': return m.transaction_kind_investment();
      case 'adjustment': return m.transaction_kind_adjustment();
      case 'opening_balance': return m.transaction_kind_opening_balance();
      default: return transaction.transaction_kind ?? '—';
    }
  });

  // ── Action helpers ────────────────────────────────────────────────
  function openModal(kind: Exclude<ModalKind, null>) {
    activeModal = kind;
    reasonInput = '';
    actionError = undefined;
  }

  function closeModal() {
    activeModal = null;
    reasonInput = '';
    actionError = undefined;
  }

  async function confirmAction() {
    if (!csrfToken || !activeModal) return;

    actionPending = true;
    actionError = undefined;

    const input: TransactionLifecycleRequest = { change_reason: reasonInput.trim() || undefined };

    try {
      switch (activeModal) {
        case 'void':
          await runWithReconciliationGuard('void', reasonInput.trim(), async (withOverride) => {
            return voidTransaction(transaction.id, csrfToken!, {
              ...input,
              reconciliation_override: withOverride || undefined
            });
          });
          break;
        case 'unvoid':
          await runWithReconciliationGuard('unvoid', reasonInput.trim(), async (withOverride) => {
            return unvoidTransaction(transaction.id, csrfToken!, {
              ...input,
              reconciliation_override: withOverride || undefined
            });
          });
          break;
        case 'soft-delete': {
          const result = await softDeleteTransaction(transaction.id, csrfToken!, input);
          closeModal();
          onSoftDeleted?.(result);
          break;
        }
        case 'restore': {
          await restoreTransaction(transaction.id, csrfToken!, input);
          closeModal();
          onRefresh?.();
          break;
        }
      }
    } catch (error) {
      actionError = error;
    } finally {
      actionPending = false;
    }
  }

  async function runWithReconciliationGuard(
    kind: 'void' | 'unvoid',
    reason: string,
    fn: (withOverride: boolean) => Promise<TransactionResponse>
  ) {
    try {
      await fn(false);
      closeModal();
      onRefresh?.();
    } catch (error) {
      if (isReconciliationOverrideRequired(error)) {
        // Fetch impact and show override modal.
        const impact = await reconciliationImpactForUpdate(transaction.id, {});
        reconciliationModal = {
          open: true,
          impacts: impact.affected_checkpoints,
          pendingKind: kind,
          pendingReason: reason
        };
        closeModal();
      } else {
        throw error;
      }
    }
  }

  async function confirmReconciliationOverride() {
    if (!csrfToken) return;
    reconciliationModal = { ...reconciliationModal, open: false };
    actionPending = true;
    actionError = undefined;

    const input: TransactionLifecycleRequest = {
      change_reason: reconciliationModal.pendingReason || undefined,
      reconciliation_override: true
    };

    try {
      if (reconciliationModal.pendingKind === 'void') {
        await voidTransaction(transaction.id, csrfToken, input);
      } else {
        await unvoidTransaction(transaction.id, csrfToken, input);
      }
      onRefresh?.();
    } catch (error) {
      actionError = error;
    } finally {
      actionPending = false;
    }
  }

  async function handleApprove() {
    if (!csrfToken) return;
    actionPending = true;
    actionError = undefined;
    try {
      await approveTransaction(transaction.id, csrfToken);
      onRefresh?.();
    } catch (error) {
      actionError = error;
    } finally {
      actionPending = false;
    }
  }

  function isReconciliationOverrideRequired(error: unknown): boolean {
    return (
      error instanceof APIClientError &&
      error.code === 'CONFLICT' &&
      (error.detail?.includes('reconciliation override') ?? false)
    );
  }

  // ── CSS helpers ───────────────────────────────────────────────────
  const labelClass = 'text-xs font-semibold uppercase tracking-[0.12em] text-muted';
  const valueClass = 'mt-0.5 text-sm text-foreground';
  const reasonInputClass =
    'mt-1.5 h-10 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted hover:bg-control-hover focus:border-accent';
  const btnPrimary =
    'inline-flex items-center gap-2 rounded-[var(--radius-control)] bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60';
  const btnSecondary =
    'inline-flex items-center gap-2 rounded-[var(--radius-control)] border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover';
  const btnDanger =
    'inline-flex items-center gap-2 rounded-[var(--radius-control)] bg-danger px-4 py-2.5 text-sm font-semibold text-danger-foreground transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60';
  const btnWarning =
    'inline-flex items-center gap-2 rounded-[var(--radius-control)] bg-warning px-4 py-2.5 text-sm font-semibold text-warning-foreground transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60';
  const btnAction =
    'inline-flex items-center gap-2 rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-60';
</script>

<!-- Reconciliation override modal -->
{#if reconciliationModal.open}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-4 py-6 backdrop-blur-sm">
    <div
      class="w-full max-w-lg rounded-[var(--radius-panel)] border border-border bg-surface shadow-[var(--shadow-panel)]"
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="recon-modal-title"
    >
      <div class="flex items-start gap-3 border-b border-border px-4 py-3">
        <AlertTriangle size={18} class="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
        <div class="min-w-0">
          <h3 id="recon-modal-title" class="text-sm font-semibold text-foreground">
            {m.transactions_reconciliation_warning_title()}
          </h3>
          <p class="mt-1 text-xs leading-5 text-muted">{m.transactions_reconciliation_warning_copy()}</p>
        </div>
      </div>

      <ul class="divide-y divide-border px-4 py-3 text-sm">
        {#each reconciliationModal.impacts as cp (cp.checkpoint_id)}
          <li class="py-2 text-foreground">
            {m.transactions_reconciliation_checkpoint_label({
              account: cp.account_label,
              commodity: cp.commodity_code,
              date: cp.statement_date
            })}
          </li>
        {/each}
      </ul>

      <div class="flex flex-wrap justify-end gap-2 border-t border-border px-4 py-3">
        <button
          type="button"
          class={btnSecondary}
          onclick={() => (reconciliationModal = { ...reconciliationModal, open: false })}
        >
          {m.transactions_reconciliation_cancel()}
        </button>
        <button
          type="button"
          class={btnWarning}
          onclick={confirmReconciliationOverride}
          disabled={actionPending}
        >
          <AlertTriangle size={14} aria-hidden="true" />
          {m.transactions_reconciliation_confirm()}
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Reason modal for void / unvoid / soft-delete / restore -->
{#if activeModal !== null}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-4 py-6 backdrop-blur-sm">
    <div
      class="w-full max-w-md rounded-[var(--radius-panel)] border border-border bg-surface shadow-[var(--shadow-panel)]"
      role="dialog"
      aria-modal="true"
      aria-labelledby="action-modal-title"
    >
      <div class="border-b border-border px-4 py-3">
        <h3 id="action-modal-title" class="text-sm font-semibold text-foreground">
          {#if activeModal === 'void'}
            {m.transactions_void_title()}
          {:else if activeModal === 'unvoid'}
            {m.transactions_unvoid_title()}
          {:else if activeModal === 'soft-delete'}
            {m.transactions_soft_delete_title()}
          {:else}
            {m.transactions_restore_title()}
          {/if}
        </h3>
        {#if activeModal === 'soft-delete'}
          <p class="mt-1 text-xs leading-5 text-muted">{m.transactions_soft_delete_copy()}</p>
        {/if}
      </div>

      <div class="px-4 py-3">
        <label>
          <span class={labelClass}>
            {#if activeModal === 'void'}
              {m.transactions_void_reason_label()}
            {:else if activeModal === 'unvoid'}
              {m.transactions_unvoid_reason_label()}
            {:else if activeModal === 'soft-delete'}
              {m.transactions_soft_delete_reason_label()}
            {:else}
              {m.transactions_restore_reason_label()}
            {/if}
          </span>
          <input
            type="text"
            bind:value={reasonInput}
            class={reasonInputClass}
            placeholder={activeModal === 'void'
              ? m.transactions_void_reason_placeholder()
              : activeModal === 'unvoid'
                ? m.transactions_unvoid_reason_placeholder()
                : activeModal === 'soft-delete'
                  ? m.transactions_soft_delete_reason_placeholder()
                  : m.transactions_restore_reason_placeholder()}
            maxlength="500"
          />
        </label>

        <APIFormError error={actionError} id="action-modal-error" />
      </div>

      <div class="flex flex-wrap justify-end gap-2 border-t border-border px-4 py-3">
        <button type="button" class={btnSecondary} onclick={closeModal} disabled={actionPending}>
          {#if activeModal === 'void'}
            {m.transactions_void_cancel()}
          {:else if activeModal === 'unvoid'}
            {m.transactions_unvoid_cancel()}
          {:else if activeModal === 'soft-delete'}
            {m.transactions_soft_delete_cancel()}
          {:else}
            {m.transactions_restore_cancel()}
          {/if}
        </button>
        <button
          type="button"
          class={activeModal === 'soft-delete' ? btnDanger : activeModal === 'void' ? btnWarning : btnPrimary}
          onclick={confirmAction}
          disabled={actionPending}
        >
          {#if activeModal === 'void'}
            {m.transactions_void_confirm()}
          {:else if activeModal === 'unvoid'}
            {m.transactions_unvoid_confirm()}
          {:else if activeModal === 'soft-delete'}
            {m.transactions_soft_delete_confirm()}
          {:else}
            {m.transactions_restore_confirm()}
          {/if}
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Detail panel body -->
<div class="space-y-4">
  <!-- Header -->
  <div class="flex items-start justify-between gap-3">
    <div class="min-w-0">
      <h2 class="text-base font-semibold text-foreground">
        {transaction.payee_name || transaction.description || String(transaction.id)}
      </h2>
      <div class="mt-1 flex items-center gap-2">
        <StatusBadge tone={statusTone(transaction.status)}>
          {statusLabel(transaction.status)}
        </StatusBadge>
        {#if transaction.needs_review}
          <StatusBadge tone="warning">{m.transactions_needs_review()}</StatusBadge>
        {/if}
      </div>
    </div>
    <button
      type="button"
      class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-foreground transition hover:bg-control-hover"
      onclick={onClose}
      aria-label={m.transactions_detail_close()}
    >
      <X size={16} aria-hidden="true" />
    </button>
  </div>

  <APIFormError error={actionError} id="detail-panel-error" />

  <!-- Core fields -->
  <dl class="grid grid-cols-2 gap-3">
    <div>
      <dt class={labelClass}>{m.transactions_detail_date()}</dt>
      <dd class={valueClass}>{transaction.transaction_date}</dd>
    </div>

    {#if transaction.payee_name}
      <div>
        <dt class={labelClass}>{m.transactions_detail_payee()}</dt>
        <dd class={valueClass}>{transaction.payee_name}</dd>
      </div>
    {/if}

    {#if transaction.description}
      <div class="col-span-2">
        <dt class={labelClass}>{m.transactions_detail_description()}</dt>
        <dd class={valueClass}>{transaction.description}</dd>
      </div>
    {/if}

    <div>
      <dt class={labelClass}>{m.transactions_detail_kind()}</dt>
      <dd class={valueClass}>{kindLabel}</dd>
    </div>
  </dl>

  <!-- Postings breakdown -->
  {#if visiblePostings.length > 0}
    <div>
      <p class={labelClass + ' mb-2'}>{m.transactions_detail_postings()}</p>
      <ul class="space-y-1">
        {#each visiblePostings as p (p.posting_line_id)}
          <li class="flex items-center justify-between rounded-[var(--radius-control)] border border-border bg-surface px-3 py-2 text-sm">
            <span class="min-w-0 truncate text-foreground">{resolveAccountLabel(p)}</span>
            <span class={`ml-3 shrink-0 font-medium tabular-nums ${p.quantity_value.startsWith('-') ? 'text-danger' : 'text-foreground'}`}>
              {commodityDisplay(p)}{formatSignedAmount(p, p.account_class as AccountClass, locale)}
            </span>
          </li>
        {/each}
      </ul>
    </div>
  {/if}

  <!-- Note and reference -->
  {#if transaction.note_markdown}
    <div>
      <p class={labelClass}>{m.transactions_detail_note()}</p>
      <p class="mt-1 text-sm text-muted whitespace-pre-wrap">{transaction.note_markdown}</p>
    </div>
  {/if}

  {#if transaction.external_ref_hint}
    <div>
      <p class={labelClass}>{m.transactions_detail_ref()}</p>
      <p class="mt-1 text-sm text-muted font-mono">{transaction.external_ref_hint}</p>
    </div>
  {/if}

  <!-- Action buttons -->
  <div class="border-t border-border pt-4">
    <p class={labelClass + ' mb-3'}>{m.transactions_detail_actions()}</p>
    <div class="flex flex-wrap gap-2">
      {#if isPosted}
        <button
          type="button"
          class={btnAction}
          onclick={() => onEdit(transaction)}
          disabled={actionPending}
        >
          {m.transactions_panel_edit()}
        </button>
        <button
          type="button"
          class={btnAction}
          onclick={() => onCreateCorrection(transaction)}
          disabled={actionPending}
        >
          {m.transactions_panel_correct()}
        </button>
        <button
          type="button"
          class={btnAction + ' text-warning hover:border-warning/50'}
          onclick={() => openModal('void')}
          disabled={actionPending}
        >
          {m.transactions_panel_void()}
        </button>
      {/if}

      {#if isVoided}
        <button
          type="button"
          class={btnAction}
          onclick={() => openModal('unvoid')}
          disabled={actionPending}
        >
          {m.transactions_panel_unvoid()}
        </button>
      {/if}

      <!-- Soft-delete is available on both posted and voided rows. -->
      <button
        type="button"
        class={btnAction + ' text-danger hover:border-danger/50'}
        onclick={() => openModal('soft-delete')}
        disabled={actionPending}
      >
        {m.transactions_panel_soft_delete()}
      </button>

      {#if transaction.needs_review}
        <button
          type="button"
          class={btnAction + ' text-accent hover:border-accent/50'}
          onclick={handleApprove}
          disabled={actionPending}
        >
          {actionPending ? m.transactions_approve_pending() : m.transactions_panel_approve()}
        </button>
      {/if}
    </div>
  </div>
</div>
