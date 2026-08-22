<script lang="ts">
  import AlertTriangle from '@lucide/svelte/icons/alert-triangle';
  import { m } from '$lib/paraglide/messages.js';
  import type { ReconciliationImpactResponse } from '$lib/api/investments';

  type CheckpointImpact = ReconciliationImpactResponse['affected_checkpoints'][number];

  let {
    impacts,
    pending = false,
    onCancel,
    onConfirm
  }: {
    impacts: CheckpointImpact[];
    pending?: boolean;
    onCancel: () => void;
    onConfirm: () => void;
  } = $props();

  const titleID = 'investment-recon-modal-title';
</script>

<!--
  The same warning the transaction editor shows, over the same message catalog:
  a reconciliation invalidated from the investments screen and one invalidated
  from the register are the same event, so they must read identically.
-->
<div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-4 py-6 backdrop-blur-sm">
  <div
    class="w-full max-w-lg rounded-[var(--radius-panel)] border border-border bg-surface shadow-[var(--shadow-panel)]"
    role="alertdialog"
    aria-modal="true"
    aria-labelledby={titleID}
  >
    <div class="flex items-start gap-3 border-b border-border px-4 py-3">
      <AlertTriangle size={18} class="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
      <div class="min-w-0">
        <h3 id={titleID} class="text-sm font-semibold text-foreground">
          {m.transactions_reconciliation_warning_title()}
        </h3>
        <p class="mt-1 text-xs leading-5 text-muted">
          {m.transactions_reconciliation_warning_copy()}
        </p>
      </div>
    </div>

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
        {m.transactions_reconciliation_confirm()}
      </button>
    </div>
  </div>
</div>
