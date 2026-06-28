<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import TrendingUp from '@lucide/svelte/icons/trending-up';
  import X from '@lucide/svelte/icons/x';
  import Panel from '$lib/components/panel.svelte';
  import StatePanel from '$lib/components/state-panel.svelte';
  import {
    investmentPositionsQueryOptions,
    investmentLotsQueryOptions,
    investmentInstrumentsQueryOptions,
    type InvestmentPositionResponse
  } from '$lib/api/investments';
  import { parseISO } from 'date-fns';
  import { formatScaledValue } from './investment-labels';
  import { m } from '$lib/paraglide/messages.js';
  import { getLocale } from '$lib/paraglide/runtime.js';

  const locale = $derived(getLocale());
  const dateFormatter = $derived(
    new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'short', day: 'numeric' })
  );

  function formatDate(iso: string): string {
    return dateFormatter.format(parseISO(iso));
  }

  const positionsQuery = createQuery(() => investmentPositionsQueryOptions());
  const instrumentsQuery = createQuery(() => investmentInstrumentsQueryOptions());

  let selectedPosition = $state<InvestmentPositionResponse | null>(null);

  const lotsQuery = createQuery(() => ({
    ...investmentLotsQueryOptions(
      selectedPosition?.account_id,
      selectedPosition?.commodity_id
    ),
    enabled: selectedPosition !== null
  }));

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

  function selectPosition(pos: InvestmentPositionResponse) {
    selectedPosition = pos;
  }

  function closeDetail() {
    selectedPosition = null;
  }

  function lotStatusLabel(status: string): string {
    switch (status) {
      case 'open':
        return m.investments_lot_status_open();
      case 'closed':
        return m.investments_lot_status_closed();
      default:
        return status;
    }
  }

  const isLoading = $derived(positionsQuery.isPending || instrumentsQuery.isPending);
  const isError = $derived(positionsQuery.isError || instrumentsQuery.isError);
  const positions = $derived(positionsQuery.data?.positions ?? []);
  const openPositions = $derived(positions.filter((p) => Number(p.quantity_value) !== 0));
</script>

<div>
  <div class="p-4 sm:p-6 lg:p-8">
    {#if isLoading}
      <Panel>
        <p class="text-sm text-muted">{m.investments_loading()}</p>
      </Panel>
    {:else if isError}
      <StatePanel
        title={m.investments_error_title()}
        copy={m.investments_error_copy()}
      />
    {:else if openPositions.length === 0}
      <StatePanel
        title={m.investments_empty_title()}
        copy={m.investments_empty_copy()}
      />
    {:else}
      <div class="lg:grid lg:grid-cols-[minmax(0,1fr)_24rem] lg:gap-6">
        <!-- Positions table -->
        <div class="min-w-0">
          <Panel padding="none">
            <div class="overflow-x-auto">
              <table class="min-w-full text-sm">
                <thead>
                  <tr class="border-b border-border bg-surface-strong/40">
                    <th class="py-3 pl-5 pr-3 text-left font-semibold text-foreground">{m.investments_col_instrument()}</th>
                    <th class="px-3 py-3 text-right font-semibold text-foreground">{m.investments_col_quantity()}</th>
                    <th class="px-3 py-3 text-right font-semibold text-foreground">{m.investments_col_cost_basis()}</th>
                    <th class="py-3 pl-3 pr-5 text-right font-semibold text-foreground">{m.investments_col_latest_price()}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-border">
                  {#each openPositions as pos (pos.account_id + '_' + pos.commodity_id)}
                    <tr
                      class={`cursor-pointer transition hover:bg-surface-strong/30 ${selectedPosition?.account_id === pos.account_id && selectedPosition?.commodity_id === pos.commodity_id ? 'bg-surface-strong/50' : ''}`}
                      onclick={() => selectPosition(pos)}
                      role="button"
                      tabindex="0"
                      onkeydown={(e) => (e.key === 'Enter' || e.key === ' ') && selectPosition(pos)}
                      aria-label={m.investments_row_aria({ name: instrumentName(pos.commodity_id) })}
                    >
                      <td class="py-3 pl-5 pr-3">
                        <div class="flex items-center gap-2">
                          <TrendingUp size={14} class="shrink-0 text-muted" aria-hidden="true" />
                          <span class="font-medium text-foreground">{instrumentName(pos.commodity_id)}</span>
                        </div>
                      </td>
                      <td class="px-3 py-3 text-right font-mono text-foreground">
                        {formatScaledValue(pos.quantity_value, pos.quantity_scale, locale)}
                      </td>
                      <td class="px-3 py-3 text-right font-mono text-muted">
                        {formatScaledValue(pos.remaining_cost_basis_value, pos.remaining_cost_basis_scale, locale)}
                      </td>
                      <td class="py-3 pl-3 pr-5 text-right">
                        {#if pos.latest_price_value !== undefined && pos.latest_price_scale !== undefined}
                          <span class="font-mono text-foreground">
                            {formatScaledValue(pos.latest_price_value, pos.latest_price_scale, locale)}
                          </span>
                          {#if pos.latest_price_date}
                            <span class="ml-1 text-xs text-muted">{formatDate(pos.latest_price_date)}</span>
                          {/if}
                        {:else}
                          <span class="text-muted">—</span>
                        {/if}
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          </Panel>
        </div>

        <!-- Lot detail panel -->
        {#if selectedPosition}
          <aside
            class="fixed inset-0 z-30 overflow-y-auto bg-surface lg:relative lg:inset-auto lg:z-auto lg:overflow-visible"
            aria-label={m.investments_lots_detail_label()}
          >
            <div
              class="fixed inset-0 bg-background/50 lg:hidden"
              role="presentation"
              onclick={closeDetail}
            ></div>

            <div class="relative lg:sticky lg:top-4">
              <Panel>
                <div class="mb-4 flex items-center justify-between">
                  <h2 class="text-base font-semibold text-foreground">
                    {instrumentName(selectedPosition.commodity_id)} — {m.investments_lots_title()}
                  </h2>
                  <button
                    type="button"
                    onclick={closeDetail}
                    class="rounded p-1 text-muted transition hover:text-foreground"
                    aria-label={m.investments_lots_close()}
                  >
                    <X size={16} aria-hidden="true" />
                  </button>
                </div>

                {#if lotsQuery.isPending}
                  <p class="text-sm text-muted">{m.investments_lots_loading()}</p>
                {:else if lotsQuery.isError}
                  <p class="text-sm text-destructive">{m.investments_lots_error()}</p>
                {:else if (lotsQuery.data?.lots ?? []).length === 0}
                  <p class="text-sm text-muted">{m.investments_lots_empty()}</p>
                {:else}
                  <div class="space-y-3">
                    {#each lotsQuery.data!.lots as lot (lot.id)}
                      <div class="rounded-(--radius-panel) border border-border p-3 text-sm">
                        <div class="flex items-center justify-between gap-2">
                          <span class="font-medium text-foreground">{formatDate(lot.opened_on)}</span>
                          <span
                            class="rounded-full px-2 py-0.5 text-xs font-medium {lot.status === 'open' ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400' : 'bg-muted/30 text-muted'}"
                          >
                            {lotStatusLabel(lot.status)}
                          </span>
                        </div>
                        <div class="mt-2 grid grid-cols-2 gap-x-3 gap-y-1 text-xs">
                          <span class="text-muted">{m.investments_lot_remaining_qty()}</span>
                          <span class="text-right font-mono text-foreground">
                            {formatScaledValue(lot.remaining_quantity_value, lot.remaining_quantity_scale, locale)}
                          </span>
                          <span class="text-muted">{m.investments_lot_cost_basis()}</span>
                          <span class="text-right font-mono text-foreground">
                            {formatScaledValue(lot.remaining_cost_basis_value, lot.remaining_cost_basis_scale, locale)}
                          </span>
                          <span class="text-muted">{m.investments_lot_original_qty()}</span>
                          <span class="text-right font-mono text-muted">
                            {formatScaledValue(lot.quantity_value, lot.quantity_scale, locale)}
                          </span>
                        </div>
                      </div>
                    {/each}
                  </div>
                {/if}
              </Panel>
            </div>
          </aside>
        {/if}
      </div>
    {/if}
  </div>
</div>
