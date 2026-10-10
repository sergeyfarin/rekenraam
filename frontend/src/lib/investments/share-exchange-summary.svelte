<script lang="ts">
  import { parseISO } from 'date-fns';
  import type { CurrencyResponse } from '#lib/api/currencies.ts';
  import type { ShareExchangePlan } from '#lib/api/investments.ts';
  import { formatExactMoney, formatQuantity } from '#lib/money/format.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';

  // One share exchange's lots and per-currency totals (#178), shared by the
  // entry preview and the transaction detail. Every amount is the exact
  // coefficient the server returned; unknown basis is a word, never a zero.
  let { plan, oldName, newName, currenciesByID }: {
    plan: ShareExchangePlan;
    oldName: string;
    newName: string;
    currenciesByID: Map<number, CurrencyResponse>;
  } = $props();

  const locale = $derived(getLocale());
  const dateFormatter = $derived(new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'short', day: 'numeric' }));

  function basisText(value: string | null, scale: number | null, currencyID: number): string {
    if (value === null || scale === null) return m.investments_basis_unknown();
    const currency = currenciesByID.get(currencyID);
    return `${formatExactMoney(value, scale, currency?.standard_scale ?? 2, locale)} ${currency?.code ?? ''}`.trim();
  }
</script>

<div class="space-y-3">
  <p class="text-sm font-medium text-foreground">{m.investments_exchange_total({
    old: formatQuantity(plan.source_quantity_value, plan.source_quantity_scale, locale), oldName,
    new: formatQuantity(plan.destination_quantity_value, plan.destination_quantity_scale, locale), newName,
    ratioNew: String(plan.ratio_numerator), ratioOld: String(plan.ratio_denominator)
  })}</p>
  <ul class="space-y-2 text-sm text-foreground" aria-label={m.investments_exchange_lots_label()}>
    {#each plan.links as link (link.source_lot_id)}
      {@const quantities = {
        old: formatQuantity(link.source_quantity_value, link.source_quantity_scale, locale), oldName,
        new: formatQuantity(link.destination_quantity_value, link.destination_quantity_scale, locale), newName
      }}
      <li class="rounded-(--radius-control) border border-border px-3 py-2">
        <span class="block break-words font-mono">{m.investments_exchange_lot_units(quantities)}</span>
        <span class="block text-xs text-muted">
          {link.original_acquired_on
            ? m.investments_exchange_lot_acquired({ date: dateFormatter.format(parseISO(link.original_acquired_on)) })
            : m.investments_exchange_lot_acquired_unknown()}
          · {m.investments_exchange_lot_basis({ basis: basisText(link.carried_basis_value, link.carried_basis_scale, link.cost_commodity_id) })}
        </span>
      </li>
    {/each}
  </ul>
  <dl class="space-y-1 text-sm">
    {#each plan.basis_totals as total (total.cost_commodity_id)}
      <div class="flex flex-wrap justify-between gap-x-3">
        <dt class="text-muted">{m.investments_exchange_total_basis({ currency: currenciesByID.get(total.cost_commodity_id)?.code ?? '' })}</dt>
        <dd class="font-mono text-foreground">
          {basisText(total.carried_basis_value, total.carried_basis_scale, total.cost_commodity_id)}
          {#if total.unknown_lots > 0}
            <span class="block text-xs font-sans text-muted">{m.investments_exchange_unknown_lots({ count: String(total.unknown_lots) })}</span>
          {/if}
        </dd>
      </div>
    {/each}
  </dl>
  <p class="text-xs text-muted">{m.investments_exchange_nothing_realized()}</p>
</div>
