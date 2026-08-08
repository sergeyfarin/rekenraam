<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import Panel from '$lib/components/panel.svelte';
  import StatePanel from '$lib/components/state-panel.svelte';
  import { netWorthSeriesQueryOptions } from '$lib/api/ledger';
  import { formatQuantity } from '$lib/money/format';
  import { m } from '$lib/paraglide/messages.js';
  import { hasMultipleCommodities, netWorthRows } from './net-worth';
  import type { ReportFilters } from './report-filters';

  let {
    filters,
    locale,
    commodityLabel,
    formatRange
  } = $props<{
    filters: ReportFilters;
    locale: string;
    commodityLabel: (commodityID: number) => string;
    formatRange: (start: string, end: string) => string;
  }>();

  const query = createQuery(() => ({
    ...netWorthSeriesQueryOptions({
      startDate: filters.startDate,
      endDate: filters.endDate,
      bucket: filters.bucket
    })
  }));

  const rows = $derived(query.data ? netWorthRows(query.data) : []);
  const multiCommodity = $derived(hasMultipleCommodities(rows));
</script>

{#if query.isPending}
  <StatePanel title={m.reports_loading_title()} copy={m.reports_loading_copy()} />
{:else if query.isError}
  <StatePanel title={m.reports_error_title()} copy={m.reports_error_copy()}>
    <button
      type="button"
      class="rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      onclick={() => query.refetch()}
    >
      {m.reports_retry()}
    </button>
  </StatePanel>
{:else if rows.length === 0}
  <StatePanel title={m.reports_empty_title()} copy={m.reports_empty_copy()} />
{:else}
  <Panel>
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 class="text-lg font-semibold tracking-tight text-foreground">{m.reports_net_worth_title()}</h2>
        <p class="mt-1 text-sm text-muted">{m.reports_net_worth_copy()}</p>
      </div>
      <p class="text-sm text-muted">{m.reports_posted_only()}</p>
    </div>

    {#if multiCommodity}
      <p class="mt-4 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        {m.reports_multi_commodity_notice()}
      </p>
    {/if}

    <div class="mt-5 overflow-x-auto">
      <table class="w-full min-w-[34rem] border-collapse text-left text-sm">
        <caption class="sr-only">{m.reports_net_worth_table_caption()}</caption>
        <thead>
          <tr class="border-b border-border text-xs uppercase tracking-[0.12em] text-muted">
            <th scope="col" class="px-3 py-3 font-semibold">{m.reports_column_period()}</th>
            <th scope="col" class="px-3 py-3 font-semibold">{m.reports_column_commodity()}</th>
            <th scope="col" class="px-3 py-3 text-right font-semibold">{m.reports_column_net_worth()}</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as row (`${row.startDate}-${row.endDate}-${row.commodity_id}`)}
            <tr class="border-b border-border/70 last:border-b-0">
              <td class="px-3 py-3 text-foreground">{formatRange(row.startDate, row.endDate)}</td>
              <td class="px-3 py-3 font-medium text-foreground">{commodityLabel(row.commodity_id)}</td>
              <td class="px-3 py-3 text-right font-semibold tabular-nums text-foreground">
                {commodityLabel(row.commodity_id)}
                {formatQuantity(row.normal_quantity_value, row.quantity_scale, locale)}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </Panel>
{/if}
