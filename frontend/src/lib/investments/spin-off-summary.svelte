<script lang="ts">
  import { parseISO } from 'date-fns';
  import type { CurrencyResponse } from '#lib/api/currencies.ts';
  import type { SpinOffPlan } from '#lib/api/investments.ts';
  import { formatExactMoney, formatQuantity } from '#lib/money/format.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { basisFractionPercent } from './split-ratio';

  // One spin-off's parent lots and per-currency totals (#180), shared by the
  // entry preview and the transaction detail. Every amount is the exact
  // coefficient the server returned; unknown basis is a word, never a zero.
  // The remaining parent basis is shown only where the server reports it.
  let { plan, parentName, newName, currenciesByID }: {
    plan: SpinOffPlan;
    parentName: string;
    newName: string;
    currenciesByID: Map<number, CurrencyResponse>;
  } = $props();

  const locale = $derived(getLocale());
  const dateFormatter = $derived(new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'short', day: 'numeric' }));
  const percent = $derived.by(() => {
    const exact = basisFractionPercent(plan.basis_fraction_value, plan.basis_fraction_scale);
    return formatQuantity(exact.value, exact.scale, locale);
  });

  function basisText(value: string | null, scale: number | null, currencyID: number): string {
    if (value === null || scale === null) return m.investments_basis_unknown();
    const currency = currenciesByID.get(currencyID);
    return `${formatExactMoney(value, scale, currency?.standard_scale ?? 2, locale)} ${currency?.code ?? ''}`.trim();
  }
</script>

<div class="space-y-3">
  <p class="text-sm font-medium text-foreground">{m.investments_spin_off_total({
    new: formatQuantity(plan.destination_quantity_value, plan.destination_quantity_scale, locale), newName, parentName,
    ratioNew: String(plan.ratio_numerator), ratioOld: String(plan.ratio_denominator), percent
  })}</p>
  <ul class="space-y-2 text-sm text-foreground" aria-label={m.investments_spin_off_lots_label()}>
    {#each plan.links as link (link.source_lot_id)}
      <li class="rounded-(--radius-control) border border-border px-3 py-2">
        <span class="block break-words font-mono">{m.investments_spin_off_lot_units({
          parent: formatQuantity(link.source_quantity_value, link.source_quantity_scale, locale), parentName,
          new: formatQuantity(link.destination_quantity_value, link.destination_quantity_scale, locale), newName
        })}</span>
        <span class="block text-xs text-muted">
          {link.original_acquired_on
            ? m.investments_exchange_lot_acquired({ date: dateFormatter.format(parseISO(link.original_acquired_on)) })
            : m.investments_exchange_lot_acquired_unknown()}
          · {m.investments_spin_off_lot_allocated({ basis: basisText(link.allocated_basis_value, link.allocated_basis_scale, link.cost_commodity_id) })}
          {#if link.remaining_basis_value !== null || link.basis_knowledge === 'unknown'}
            · {m.investments_spin_off_lot_remaining({ basis: basisText(link.remaining_basis_value, link.remaining_basis_scale, link.cost_commodity_id) })}
          {/if}
        </span>
      </li>
    {/each}
  </ul>
  <dl class="space-y-1 text-sm">
    {#each plan.basis_totals as total (total.cost_commodity_id)}
      {@const code = currenciesByID.get(total.cost_commodity_id)?.code ?? ''}
      <div class="flex flex-wrap justify-between gap-x-3">
        <dt class="text-muted">{m.investments_spin_off_total_allocated({ currency: code })}</dt>
        <dd class="font-mono text-foreground">
          {basisText(total.allocated_basis_value, total.allocated_basis_scale, total.cost_commodity_id)}
          {#if total.unknown_lots > 0}
            <span class="block text-xs font-sans text-muted">{m.investments_exchange_unknown_lots({ count: String(total.unknown_lots) })}</span>
          {/if}
        </dd>
      </div>
      {#if total.remaining_basis_value !== null}
        <div class="flex flex-wrap justify-between gap-x-3">
          <dt class="text-muted">{m.investments_spin_off_total_remaining({ currency: code })}</dt>
          <dd class="font-mono text-foreground">{basisText(total.remaining_basis_value, total.remaining_basis_scale, total.cost_commodity_id)}</dd>
        </div>
      {/if}
    {/each}
  </dl>
  <p class="text-xs text-muted">{m.investments_spin_off_nothing_realized()}</p>
</div>
