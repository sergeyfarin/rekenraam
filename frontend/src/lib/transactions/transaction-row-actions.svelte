<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import type { TransactionResponse } from '$lib/api/transactions';

  let {
    transaction,
    onEdit,
    onCreateCorrection,
    onApprove,
    onUnvoid,
    onView
  }: {
    transaction: TransactionResponse;
    onEdit?: (tx: TransactionResponse) => void;
    onCreateCorrection?: (tx: TransactionResponse) => void;
    onApprove?: (tx: TransactionResponse) => void;
    onUnvoid?: (tx: TransactionResponse) => void;
    onView?: (tx: TransactionResponse) => void;
  } = $props();

  const isPosted = $derived(transaction.status === 'posted');
  const isVoided = $derived(transaction.status === 'voided');
  const needsReview = $derived(transaction.needs_review);

  const btnClass =
    'rounded-(--radius-control) px-2.5 py-1 text-xs font-medium text-muted ring-1 ring-border transition hover:bg-surface-strong hover:text-foreground focus:outline-none focus:ring-accent';
</script>

<!--
  Row actions are light, inline controls only. Tab moves focus into these buttons
  when the row has focus (no nested interactives; these are siblings to the row's
  stretched-link in context wrappers). Click events use stopPropagation so they
  do not bubble to the row's onRowClick handler.
-->
<div
  class="flex items-center gap-1"
  role="group"
  aria-label="Row actions"
>
  {#if isPosted}
    {#if onEdit}
      <button
        type="button"
        class={btnClass}
        onclick={(e) => { e.stopPropagation(); onEdit?.(transaction); }}
        aria-label={m.transactions_action_edit()}
      >
        {m.transactions_action_edit()}
      </button>
    {/if}

    {#if onCreateCorrection}
      <button
        type="button"
        class={btnClass}
        onclick={(e) => { e.stopPropagation(); onCreateCorrection?.(transaction); }}
        aria-label={m.transactions_action_correct()}
      >
        {m.transactions_action_correct()}
      </button>
    {/if}
  {/if}

  {#if isVoided}
    {#if onView}
      <button
        type="button"
        class={btnClass}
        onclick={(e) => { e.stopPropagation(); onView?.(transaction); }}
        aria-label={m.transactions_action_view()}
      >
        {m.transactions_action_view()}
      </button>
    {/if}

    {#if onUnvoid}
      <button
        type="button"
        class={btnClass}
        onclick={(e) => { e.stopPropagation(); onUnvoid?.(transaction); }}
        aria-label={m.transactions_action_unvoid()}
      >
        {m.transactions_action_unvoid()}
      </button>
    {/if}
  {/if}

  <!-- Approve is shown regardless of lock status when needs_review is set. -->
  {#if needsReview && onApprove}
    <button
      type="button"
      class="rounded-(--radius-control) px-2.5 py-1 text-xs font-medium text-accent ring-1 ring-accent/30 transition hover:bg-accent hover:text-accent-foreground focus:outline-none focus:ring-accent"
      onclick={(e) => { e.stopPropagation(); onApprove?.(transaction); }}
      aria-label={m.transactions_action_approve()}
    >
      {m.transactions_action_approve()}
    </button>
  {/if}
</div>
