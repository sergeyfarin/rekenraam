<script lang="ts">
  import { parseISO } from 'date-fns';
  import type { CapitalReturnRead } from '#lib/api/investments.ts';
  import { formatExactMoney, formatQuantity } from '#lib/money/format.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';

  let { entry }: { entry: CapitalReturnRead } = $props();
  const locale = $derived(getLocale());
  const dates = $derived(new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'short', day: 'numeric' }));
  function amount(value: string | null, scale: number | null): string {
    return value === null || scale === null
      ? m.investments_roc_unresolved()
      : `${formatExactMoney(value, scale, entry.currency_scale, locale)} ${entry.currency_code}`;
  }
</script>

<article class="space-y-3 rounded-(--radius-control) border border-border bg-surface p-3 text-sm" aria-label={m.investments_roc_receipt({ instrument: entry.instrument_name })}>
  <div class="flex flex-wrap justify-between gap-2">
    <span class="font-semibold text-foreground">{entry.instrument_name} · {entry.holding_name}</span>
    <span class="text-xs text-muted">{entry.effective ? m.investments_roc_current() : m.investments_roc_historical()}</span>
  </div>
  <dl class="space-y-1">
    <div class="flex flex-wrap justify-between gap-x-3"><dt class="text-muted">{m.investments_capital_return_effective_date()}</dt><dd>{dates.format(parseISO(entry.effective_on))}</dd></div>
    <div class="flex flex-wrap justify-between gap-x-3"><dt class="text-muted">{m.investments_capital_return_payment_date()}</dt><dd>{dates.format(parseISO(entry.payment_on))}</dd></div>
    <div class="flex flex-wrap justify-between gap-x-3"><dt class="text-muted">{m.investments_capital_return_amount()}</dt><dd class="font-mono">{amount(entry.amount_value, entry.amount_scale)}</dd></div>
    <div class="flex flex-wrap justify-between gap-x-3"><dt class="text-muted">{m.investments_roc_reduction()}</dt><dd class="font-mono">{amount(entry.reduction_value, entry.reduction_scale)}</dd></div>
    <div class="flex flex-wrap justify-between gap-x-3"><dt class="text-muted">{m.investments_roc_excess()}</dt><dd class="font-mono">{amount(entry.excess_value, entry.excess_scale)}</dd></div>
  </dl>
  {#if entry.has_excess || entry.unknown_lots > 0}
    <p class="text-xs text-muted" role="note">{m.investments_roc_decision_note()}</p>
  {:else}
    <p class="text-xs text-muted">{m.investments_roc_no_excess()}</p>
  {/if}
  <details>
    <summary class="cursor-pointer font-medium text-accent">{m.investments_roc_allocations_source()}</summary>
    <ul class="mt-2 space-y-2">
      {#each entry.effects as effect (effect.lot_id)}
        <li class="space-y-1 border-t border-border pt-2">
          <p>{m.investments_roc_lot_allocation({ lot: String(effect.lot_id), quantity: formatQuantity(effect.entitled_quantity_value, effect.entitled_quantity_scale, locale), allocated: amount(effect.allocated_value, effect.allocated_scale) })}</p>
          <p class="text-xs text-muted">{m.investments_roc_lot_result({ reduction: amount(effect.reduction_value, effect.reduction_scale), excess: amount(effect.excess_value, effect.excess_scale) })}</p>
        </li>
      {/each}
    </ul>
    <p class="mt-2 text-xs text-muted">{m.investments_roc_revision({ version: String(entry.revision_seq), operation: String(entry.operation_id), audit: String(entry.audit_event_id) })}</p>
    {#if entry.caused_by_operation_id !== null}
      <p class="text-xs text-muted">{m.investments_roc_revision_cause({ operation: String(entry.caused_by_operation_id) })}</p>
    {/if}
    <p class="text-xs text-muted">{new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(parseISO(entry.created_at))}</p>
    <pre class="mt-2 whitespace-pre-wrap break-all text-xs text-muted">{entry.source_evidence_json}</pre>
  </details>
  <a class="font-medium text-accent" href={`/app/ledger/transactions?transaction_id=${entry.transaction_id}`}>{m.investments_roc_open_receipt()}</a>
</article>
