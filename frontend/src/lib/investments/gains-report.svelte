<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import Panel from '$lib/components/panel.svelte';
  import StatePanel from '$lib/components/state-panel.svelte';
  import {
    investmentGainsQueryOptions,
    investmentInstrumentsQueryOptions,
    type RealizedGainEntry,
    type UnrealizedGainEntry
  } from '$lib/api/investments';
  import { formatScaledValue } from './investment-labels';
  import { m } from '$lib/paraglide/messages.js';
  import { getLocale } from '$lib/paraglide/runtime.js';

  const locale = $derived(getLocale());

  // Date range filter (default: current calendar year)
  const currentYear = new Date().getFullYear();
  let fromDate = $state(`${currentYear}-01-01`);
  let toDate = $state(`${currentYear}-12-31`);
  let appliedFrom = $state(`${currentYear}-01-01`);
  let appliedTo = $state(`${currentYear}-12-31`);

  function applyFilter() {
    appliedFrom = fromDate;
    appliedTo = toDate;
  }

  const gainsQuery = createQuery(() =>
    investmentGainsQueryOptions(appliedFrom || undefined, appliedTo || undefined)
  );
  const instrumentsQuery = createQuery(() => investmentInstrumentsQueryOptions());

  const instrumentsByID = $derived.by(() => {
    const map = new Map<number, string>();
    for (const inst of instrumentsQuery.data?.instruments ?? []) {
      map.set(inst.commodity_id, inst.display_name);
    }
    return map;
  });

  function instrumentName(commodityID: number): string {
    return instrumentsByID.get(commodityID) ?? `#${commodityID}`;
  }

  const realized = $derived(gainsQuery.data?.realized ?? []);
  const unrealized = $derived(gainsQuery.data?.unrealized ?? []);

  const realizedTotal = $derived(
    realized.reduce((sum: bigint, e: RealizedGainEntry) => sum + BigInt(e.realized_gain_value), 0n)
  );

  // Use first entry's scale for total display (they should be same cost commodity)
  const realizedTotalScale = $derived(realized[0]?.proceeds_scale ?? 2);

  function formatGain(value: number | bigint, scale: number): string {
    return formatScaledValue(String(value), scale, locale);
  }

  function gainClass(value: number | bigint): string {
    const n = typeof value === 'bigint' ? value : BigInt(Math.trunc(Number(value)));
    if (n > 0n) return 'text-green-700 dark:text-green-400';
    if (n < 0n) return 'text-destructive';
    return 'text-muted';
  }
</script>

<div class="space-y-6">
  <!-- Unrealized gains -->
  <section>
    <h2 class="mb-3 text-sm font-semibold text-foreground">{m.investments_gains_unrealized_title()}</h2>
    {#if gainsQuery.isPending}
      <Panel>
        <p class="text-sm text-muted">{m.investments_gains_loading()}</p>
      </Panel>
    {:else if gainsQuery.isError}
      <Panel>
        <p class="text-sm text-destructive">{m.investments_gains_error()}</p>
      </Panel>
    {:else if unrealized.length === 0}
      <StatePanel title={m.investments_gains_unrealized_empty()} copy="" />
    {:else}
      <Panel padding="none">
        <div class="overflow-x-auto">
          <table class="min-w-full text-sm">
            <thead>
              <tr class="border-b border-border bg-surface-strong/40">
                <th class="py-3 pl-5 pr-3 text-left font-semibold text-foreground">{m.investments_col_instrument()}</th>
                <th class="px-3 py-3 text-right font-semibold text-foreground">{m.investments_col_quantity()}</th>
                <th class="px-3 py-3 text-right font-semibold text-foreground">{m.investments_col_cost_basis()}</th>
                <th class="px-3 py-3 text-right font-semibold text-foreground">{m.investments_gains_col_market_value()}</th>
                <th class="py-3 pl-3 pr-5 text-right font-semibold text-foreground">{m.investments_gains_col_gain()}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border">
              {#each unrealized as pos (pos.account_id + '_' + pos.commodity_id)}
                <tr class="hover:bg-surface-strong/20">
                  <td class="py-3 pl-5 pr-3 font-medium text-foreground">{instrumentName(pos.commodity_id)}</td>
                  <td class="px-3 py-3 text-right font-mono text-foreground">
                    {formatScaledValue(pos.quantity_value, pos.quantity_scale, locale)}
                  </td>
                  <td class="px-3 py-3 text-right font-mono text-muted">
                    {formatScaledValue(String(pos.remaining_cost_basis_value), pos.remaining_cost_basis_scale, locale)}
                  </td>
                  <td class="px-3 py-3 text-right font-mono text-foreground">
                    {#if pos.market_value_value !== undefined && pos.market_value_scale !== undefined}
                      {formatGain(pos.market_value_value, pos.market_value_scale)}
                    {:else}
                      <span class="text-muted">{m.investments_gains_no_price()}</span>
                    {/if}
                  </td>
                  <td class="py-3 pl-3 pr-5 text-right font-mono">
                    {#if pos.unrealized_gain_value !== undefined && pos.unrealized_gain_scale !== undefined}
                      <span class={gainClass(pos.unrealized_gain_value)}>
                        {formatGain(pos.unrealized_gain_value, pos.unrealized_gain_scale)}
                      </span>
                    {:else}
                      <span class="text-muted">{m.investments_gains_no_price()}</span>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </Panel>
    {/if}
  </section>

  <!-- Realized gains -->
  <section>
    <div class="mb-3 flex flex-wrap items-center gap-3">
      <h2 class="text-sm font-semibold text-foreground">{m.investments_gains_realized_title()}</h2>
      <div class="flex flex-wrap items-center gap-2 text-sm">
        <label class="flex items-center gap-1 text-muted">
          {m.investments_gains_from()}
          <input
            type="date"
            bind:value={fromDate}
            class="rounded border border-border bg-control px-2 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-foreground/30"
          />
        </label>
        <label class="flex items-center gap-1 text-muted">
          {m.investments_gains_to()}
          <input
            type="date"
            bind:value={toDate}
            class="rounded border border-border bg-control px-2 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-foreground/30"
          />
        </label>
        <button
          type="button"
          onclick={applyFilter}
          class="rounded-(--radius-control) border border-border bg-control px-3 py-1 text-xs font-medium text-foreground hover:bg-control-hover"
        >
          {m.investments_gains_filter_apply()}
        </button>
      </div>
    </div>

    {#if gainsQuery.isPending}
      <Panel>
        <p class="text-sm text-muted">{m.investments_gains_loading()}</p>
      </Panel>
    {:else if gainsQuery.isError}
      <Panel>
        <p class="text-sm text-destructive">{m.investments_gains_error()}</p>
      </Panel>
    {:else if realized.length === 0}
      <StatePanel title={m.investments_gains_realized_empty()} copy="" />
    {:else}
      <Panel padding="none">
        <div class="overflow-x-auto">
          <table class="min-w-full text-sm">
            <thead>
              <tr class="border-b border-border bg-surface-strong/40">
                <th class="py-3 pl-5 pr-3 text-left font-semibold text-foreground">{m.investments_gains_col_date()}</th>
                <th class="px-3 py-3 text-left font-semibold text-foreground">{m.investments_col_instrument()}</th>
                <th class="px-3 py-3 text-right font-semibold text-foreground">{m.investments_col_quantity()}</th>
                <th class="px-3 py-3 text-right font-semibold text-foreground">{m.investments_gains_col_proceeds()}</th>
                <th class="px-3 py-3 text-right font-semibold text-foreground">{m.investments_gains_col_disposed_basis()}</th>
                <th class="py-3 pl-3 pr-5 text-right font-semibold text-foreground">{m.investments_gains_col_gain()}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border">
              {#each realized as entry (entry.disposal_date + '_' + entry.commodity_id + '_' + entry.account_id)}
                <tr class="hover:bg-surface-strong/20">
                  <td class="py-3 pl-5 pr-3 font-mono text-xs text-muted">{entry.disposal_date}</td>
                  <td class="px-3 py-3 text-foreground">{instrumentName(entry.commodity_id)}</td>
                  <td class="px-3 py-3 text-right font-mono text-foreground">
                    {formatScaledValue(entry.quantity_value, entry.quantity_scale, locale)}
                  </td>
                  <td class="px-3 py-3 text-right font-mono text-foreground">
                    {formatGain(entry.proceeds_value, entry.proceeds_scale)}
                  </td>
                  <td class="px-3 py-3 text-right font-mono text-muted">
                    {formatGain(-entry.disposed_basis_value, entry.disposed_basis_scale)}
                  </td>
                  <td class="py-3 pl-3 pr-5 text-right font-mono">
                    <span class={gainClass(entry.realized_gain_value)}>
                      {formatGain(entry.realized_gain_value, entry.proceeds_scale)}
                    </span>
                  </td>
                </tr>
              {/each}
            </tbody>
            <tfoot>
              <tr class="border-t-2 border-border bg-surface-strong/30">
                <td colspan="5" class="py-3 pl-5 pr-3 text-sm font-semibold text-foreground">
                  {m.investments_gains_realized_total()}
                </td>
                <td class="py-3 pl-3 pr-5 text-right font-mono font-semibold">
                  <span class={gainClass(realizedTotal)}>
                    {formatGain(realizedTotal, realizedTotalScale)}
                  </span>
                </td>
              </tr>
            </tfoot>
          </table>
        </div>
      </Panel>
    {/if}
  </section>
</div>
