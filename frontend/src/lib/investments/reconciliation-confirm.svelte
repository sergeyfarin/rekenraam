<script lang="ts">
  import AlertTriangle from '@lucide/svelte/icons/alert-triangle';
  import { m } from '#lib/paraglide/messages.js';
  import type { ReconciliationImpactResponse } from '#lib/api/investments.ts';
  import type { GainImpactRow } from '#lib/investments/gain-impact.ts';
  import GainImpactList from '#lib/investments/gain-impact-list.svelte';

  type CheckpointImpact = ReconciliationImpactResponse['affected_checkpoints'][number];

  let {
    impacts,
    gainRows = [],
    gainRefreshed = false,
    pending = false,
    onCancel,
    onConfirm
  }: {
    impacts: CheckpointImpact[];
    gainRows?: GainImpactRow[];
    gainRefreshed?: boolean;
    pending?: boolean;
    onCancel: () => void;
    onConfirm: () => void;
  } = $props();

  const titleID = 'investment-recon-modal-title';
  const hasCheckpoints = $derived(impacts.length > 0);
  const hasGains = $derived(gainRows.length > 0);
</script>

<!--
  The same warning the transaction editor shows, over the same message catalog:
  a reconciliation invalidated from the investments screen and one invalidated
  from the register are the same event, so they must read identically. Replay
  gain changes (T-114) are listed alongside, and one confirmation accepts both.
-->
<div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-4 py-6 backdrop-blur-sm">
  <div
    class="max-h-full w-full max-w-lg overflow-y-auto rounded-[var(--radius-panel)] border border-border bg-surface shadow-[var(--shadow-panel)]"
    role="alertdialog"
    aria-modal="true"
    aria-labelledby={titleID}
  >
    <div class="flex items-start gap-3 border-b border-border px-4 py-3">
      <AlertTriangle size={18} class="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
      <div class="min-w-0">
        <h3 id={titleID} class="text-sm font-semibold text-foreground">
          {hasCheckpoints ? m.transactions_reconciliation_warning_title() : m.investments_gain_impact_title()}
        </h3>
        {#if hasCheckpoints}
          <p class="mt-1 text-xs leading-5 text-muted">
            {m.transactions_reconciliation_warning_copy()}
          </p>
        {/if}
      </div>
    </div>

    {#if hasCheckpoints}
      <ul class="divide-y divide-border px-4 py-3 text-sm">
        {#each impacts as checkpoint (checkpoint.checkpoint_id)}
          <li class="py-2 text-foreground">
            {m.transactions_reconciliation_checkpoint_label({
              account: checkpoint.account_label,
              commodity: checkpoint.commodity_code,
              date: checkpoint.statement_date
            })}
          </li>
        {/each}
      </ul>
    {/if}

    {#if hasGains}
      <div class:border-t={hasCheckpoints} class="border-border">
        <GainImpactList rows={gainRows} refreshed={gainRefreshed} showHeading={hasCheckpoints} labelledBy={titleID} />
      </div>
    {/if}

    <div class="flex flex-wrap justify-end gap-2 border-t border-border px-4 py-3">
      <button
        type="button"
        class="inline-flex items-center gap-2 rounded-[var(--radius-control)] border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
        onclick={onCancel}
      >
        {m.transactions_reconciliation_cancel()}
      </button>
      <button
        type="button"
        class="inline-flex items-center gap-2 rounded-[var(--radius-control)] bg-warning px-4 py-2.5 text-sm font-semibold text-warning-foreground transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
        onclick={onConfirm}
        disabled={pending}
      >
        <AlertTriangle size={14} aria-hidden="true" />
        {#if hasCheckpoints && hasGains}
          {m.investments_gain_impact_confirm_with_reconciliation()}
        {:else if hasGains}
          {m.investments_gain_impact_confirm()}
        {:else}
          {m.transactions_reconciliation_confirm()}
        {/if}
      </button>
    </div>
  </div>
</div>
