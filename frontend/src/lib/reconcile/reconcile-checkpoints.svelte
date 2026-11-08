<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import TriangleExclamationPoint from '@lucide/svelte/icons/triangle-exclamation-point';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import Panel from '#lib/components/panel.svelte';
  import StatusBadge from '#lib/components/status-badge.svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import ModalDialog from '#lib/components/modal-dialog.svelte';
  import { formatQuantity } from '#lib/money/format.ts';
  import { coveredByLaterActiveCheckpoint } from '#lib/reconcile/checkpoint-chain.ts';
  import {
    reconciliationCheckpointsQueryOptions,
    reconciliationQueryKey,
    voidReconciliationCheckpoint,
    type ReconciliationCheckpointResponse
  } from '#lib/api/reconciliation.ts';

  let {
    accountID,
    commodityID,
    csrfToken,
    voidable = true
  }: {
    accountID: number;
    commodityID?: number;
    csrfToken?: string;
    // Off during an open session: voiding the checkpoint it started from would
    // only make its finish fail as stale.
    voidable?: boolean;
  } = $props();

  const locale = $derived(getLocale());
  const queryClient = useQueryClient();

  const checkpointsQuery = createQuery(() =>
    reconciliationCheckpointsQueryOptions(accountID, commodityID)
  );

  const checkpoints = $derived(checkpointsQuery.data?.checkpoints ?? []);

  // Only active checkpoints represent a current reconciled-through point.
  const active = $derived(
    checkpoints
      .filter((c: ReconciliationCheckpointResponse) => c.status === 'active')
      .sort((a, b) => b.statement_date.localeCompare(a.statement_date))
  );

  // Invalidated and voided checkpoints stay as audit evidence (#240). Their
  // postings are offered again in the next reconciliation.
  const history = $derived(
    checkpoints
      .filter((c: ReconciliationCheckpointResponse) => c.status !== 'active')
      .sort((a, b) => b.statement_date.localeCompare(a.statement_date) || b.id - a.id)
  );

  function reasonFor(checkpoint: ReconciliationCheckpointResponse): string {
    return (checkpoint.status === 'voided' ? checkpoint.void_reason : checkpoint.invalidation_reason) ?? '';
  }

  function balanceOf(checkpoint: ReconciliationCheckpointResponse): string {
    return formatQuantity(
      checkpoint.statement_balance.quantity_value,
      checkpoint.statement_balance.quantity_scale,
      locale
    );
  }

  // ── Void a checkpoint (#248) ──────────────────────────────────────
  // Voiding is the recovery path for a wrongly finished statement: the
  // checkpoint stays as history and its postings are offered again, unless a
  // later active checkpoint chained from it still covers them.
  let voidTarget = $state.raw<ReconciliationCheckpointResponse | undefined>(undefined);
  let voidReason = $state('');
  let voidPending = $state(false);
  let voidError = $state<unknown>(undefined);
  let voidNeedsReason = $state(false);
  let voidedNotice = $state<string | undefined>(undefined);
  let reasonInput = $state<HTMLTextAreaElement | undefined>(undefined);

  const voidTargetChained = $derived(
    voidTarget !== undefined && coveredByLaterActiveCheckpoint(voidTarget.id, checkpoints)
  );

  function openVoid(checkpoint: ReconciliationCheckpointResponse) {
    voidTarget = checkpoint;
    voidReason = '';
    voidError = undefined;
    voidNeedsReason = false;
    voidedNotice = undefined;
  }

  function cancelVoid() {
    if (voidPending) return;
    voidTarget = undefined;
  }

  async function confirmVoid(event: SubmitEvent) {
    event.preventDefault();
    if (!voidTarget || !csrfToken || voidPending) return;
    voidError = undefined;
    if (voidReason.trim() === '') {
      voidNeedsReason = true;
      reasonInput?.focus();
      return;
    }
    voidNeedsReason = false;
    voidPending = true;
    try {
      const voided = await voidReconciliationCheckpoint(voidTarget.id, { change_reason: voidReason.trim() }, csrfToken);
      voidedNotice = m.reconcile_checkpoint_voided({ date: voided.statement_date });
      voidTarget = undefined;
      await queryClient.invalidateQueries({ queryKey: reconciliationQueryKey });
    } catch (error) {
      voidError = error;
    } finally {
      voidPending = false;
    }
  }
</script>

<Panel>
  <h3 class="text-sm font-semibold text-foreground">{m.reconcile_checkpoints_title()}</h3>

  {#if voidedNotice}
    <p class="mt-2 text-sm text-positive" role="status">{voidedNotice}</p>
  {/if}

  {#if checkpointsQuery.isPending}
    <p class="mt-2 text-sm text-muted" role="status">{m.reconcile_checkpoints_loading()}</p>
  {:else if checkpointsQuery.isError}
    <div class="mt-2 space-y-2">
      <p class="text-sm text-danger" role="alert">{m.reconcile_checkpoints_error()}</p>
      <button
        type="button"
        class="rounded-(--radius-control) border border-border bg-control px-3 py-1.5 text-sm font-semibold text-foreground transition-colors hover:bg-control-hover"
        onclick={() => checkpointsQuery.refetch()}
      >
        {m.transactions_retry()}
      </button>
    </div>
  {:else if active.length === 0}
    <p class="mt-2 text-sm text-muted">{m.reconcile_checkpoints_empty()}</p>
  {:else}
    <table class="mt-3 w-full text-sm">
      <thead>
        <tr class="border-b border-border text-left text-xs font-semibold uppercase tracking-widest text-muted">
          <th class="py-2">{m.reconcile_checkpoints_col_date()}</th>
          <th class="py-2 text-right">{m.reconcile_checkpoints_col_balance()}</th>
          {#if voidable}
            <th class="py-2 text-right"><span class="sr-only">{m.reconcile_checkpoints_col_actions()}</span></th>
          {/if}
        </tr>
      </thead>
      <tbody>
        {#each active as checkpoint (checkpoint.id)}
          <tr class="border-b border-border/50 last:border-0">
            <td class="py-2 whitespace-nowrap text-foreground">{checkpoint.statement_date}</td>
            <td class="py-2 text-right tabular-nums text-foreground">{balanceOf(checkpoint)}</td>
            {#if voidable}
              <td class="py-2 pl-3 text-right">
                <button
                  type="button"
                  class="inline-flex min-h-9 items-center rounded-(--radius-control) border border-border bg-control px-3 py-1.5 text-sm font-semibold text-danger transition-colors hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-60"
                  aria-label={m.reconcile_checkpoint_void_aria({ date: checkpoint.statement_date })}
                  disabled={!csrfToken}
                  onclick={() => openVoid(checkpoint)}
                >
                  {m.reconcile_checkpoint_void()}
                </button>
              </td>
            {/if}
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}

  {#if history.length > 0}
    <h4 class="mt-5 text-sm font-semibold text-foreground">{m.reconcile_checkpoints_history_title()}</h4>
    <p class="mt-1 text-xs leading-5 text-muted">{m.reconcile_checkpoints_history_copy()}</p>
    <table class="mt-2 w-full text-sm">
      <thead>
        <tr class="border-b border-border text-left text-xs font-semibold uppercase tracking-widest text-muted">
          <th class="py-2">{m.reconcile_checkpoints_col_date()}</th>
          <th class="py-2">{m.reconcile_checkpoints_col_status()}</th>
          <th class="py-2 text-right">{m.reconcile_checkpoints_col_balance()}</th>
        </tr>
      </thead>
      <tbody>
        {#each history as checkpoint (checkpoint.id)}
          <tr class="border-b border-border/50 align-top last:border-0">
            <td class="py-2 whitespace-nowrap text-muted">{checkpoint.statement_date}</td>
            <td class="py-2">
              <StatusBadge tone={checkpoint.status === 'voided' ? 'neutral' : 'warning'}>
                {checkpoint.status === 'voided'
                  ? m.reconcile_checkpoints_status_voided()
                  : m.reconcile_checkpoints_status_invalidated()}
              </StatusBadge>
              {#if reasonFor(checkpoint)}
                <p class="mt-1 text-xs text-muted">{reasonFor(checkpoint)}</p>
              {/if}
            </td>
            <td class="py-2 text-right tabular-nums text-muted">{balanceOf(checkpoint)}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</Panel>

{#if voidTarget}
  <ModalDialog role="alertdialog" labelledby="checkpoint-void-title" describedby="checkpoint-void-description"
    pending={voidPending} onCancel={cancelVoid} class="max-w-md">
    <form onsubmit={confirmVoid}>
      <div class="flex items-start gap-3 border-b border-border px-4 py-3">
        <TriangleExclamationPoint size={18} class="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
        <div class="min-w-0">
          <h3 id="checkpoint-void-title" class="text-sm font-semibold text-foreground">
            {m.reconcile_checkpoint_void_title()}
          </h3>
          <div id="checkpoint-void-description" class="mt-1 space-y-2 text-xs leading-5 text-muted">
            <p>
              {m.reconcile_checkpoint_void_copy({
                date: voidTarget.statement_date,
                balance: balanceOf(voidTarget)
              })}
            </p>
            <p>
              {voidTargetChained
                ? m.reconcile_checkpoint_void_chained_copy()
                : m.reconcile_checkpoint_void_offered_copy()}
            </p>
          </div>
        </div>
      </div>

      <div class="space-y-2 px-4 py-3">
        <label
          class="block text-xs font-semibold uppercase tracking-[0.12em] text-muted"
          for="checkpoint-void-reason"
        >
          {m.reconcile_checkpoint_void_reason_label()}
        </label>
        <textarea
          id="checkpoint-void-reason"
          bind:this={reasonInput}
          bind:value={voidReason}
          rows="2"
          aria-required="true"
          aria-invalid={voidNeedsReason}
          aria-describedby={voidNeedsReason ? 'checkpoint-void-reason-error' : undefined}
          placeholder={m.reconcile_checkpoint_void_reason_placeholder()}
          disabled={voidPending}
          class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted focus:border-accent disabled:cursor-not-allowed disabled:opacity-70"
        ></textarea>
        {#if voidNeedsReason}
          <p id="checkpoint-void-reason-error" class="text-xs text-danger">{m.reconcile_checkpoint_void_needs_reason()}</p>
        {/if}
        <APIFormError error={voidError} id="checkpoint-void-error" />
      </div>

      <div class="flex flex-wrap justify-end gap-2 border-t border-border px-4 py-3">
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-60"
          onclick={cancelVoid}
          disabled={voidPending}
        >
          {m.reconcile_checkpoint_void_cancel()}
        </button>
        <button
          type="submit"
          class="inline-flex items-center gap-2 rounded-(--radius-control) bg-danger px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
          disabled={voidPending || !csrfToken}
        >
          {voidPending ? m.reconcile_checkpoint_voiding() : m.reconcile_checkpoint_void_confirm()}
        </button>
      </div>
    </form>
  </ModalDialog>
{/if}
