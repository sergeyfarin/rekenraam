<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import Panel from '$lib/components/panel.svelte';
  import StatePanel from '$lib/components/state-panel.svelte';
  import { cashflowQueryOptions } from '$lib/api/ledger';
  import { formatQuantity } from '$lib/money/format';
  import { m } from '$lib/paraglide/messages.js';
  import { cashflowIsEmpty, cashflowIsMultiCommodity, cashflowRows, isNegativeAmount } from './cashflow';
  import type { ReportFilters } from './report-filters';
  import { cashflowCSV, downloadCSV, reportFilename } from './report-csv';

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
    ...cashflowQueryOptions({
      startDate: filters.startDate,
      endDate: filters.endDate,
      bucket: filters.bucket
    })
  }));

  const rows = $derived(query.data ? cashflowRows(query.data) : []);
  const isEmpty = $derived(query.data ? cashflowIsEmpty(query.data) : false);
  const multiCommodity = $derived(query.data ? cashflowIsMultiCommodity(query.data) : false);

  const scopeNames = $derived(
    (query.data?.scope_accounts ?? [])
      .map((account) => account.name || account.code || `#${account.account_id}`)
      .join(', ')
  );

  function exportCSV() {
    if (!query.data) return;
    downloadCSV(
      reportFilename('cashflow', query.data.start_date, query.data.end_date),
      cashflowCSV(query.data, commodityLabel)
    );
  }

  function amount(value: string, scale: number, commodityId: number | null): string {
    if (commodityId === null) return '—';
    return `${commodityLabel(commodityId)} ${formatQuantity(value, scale, locale)}`;
  }

  // A negative net movement gets both a colour and a non-colour cue, since
  // colour alone must never be the only signal on a financial state.
  function signedClass(value: string): string {
    return isNegativeAmount(value) ? 'text-danger' : 'text-foreground';
  }
</script>

{#if query.isPending}
  <StatePanel title={m.reports_cashflow_loading_title()} copy={m.reports_loading_copy()} />
{:else if query.isError}
  <StatePanel title={m.reports_cashflow_error_title()} copy={m.reports_error_copy()}>
    <button
      type="button"
      class="rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      onclick={() => query.refetch()}
    >
      {m.reports_retry()}
    </button>
  </StatePanel>
{:else if isEmpty}
  <StatePanel title={m.reports_cashflow_empty_title()} copy={m.reports_cashflow_empty_copy()} />
{:else}
  <Panel>
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 class="text-lg font-semibold tracking-tight text-foreground">{m.reports_cashflow_title()}</h2>
        <p class="mt-1 text-sm text-muted">{m.reports_cashflow_copy()}</p>
      </div>
      <div class="flex items-center gap-3">
        <p class="text-sm text-muted">{m.reports_posted_only()}</p>
        <button
          type="button"
          class="h-9 rounded-(--radius-control) border border-border bg-control px-3 text-sm font-semibold text-foreground transition hover:bg-control-hover print:hidden"
          onclick={exportCSV}
        >
          {m.reports_export_csv()}
        </button>
      </div>
    </div>

    <!--
      The transfer policy and the cash scope are both stated on the screen. A
      user reading "net movement" needs to know which accounts counted as cash
      and that moving money between two of them was excluded on purpose.
    -->
    <p class="mt-3 text-sm text-muted">{m.reports_cashflow_transfer_policy()}</p>
    {#if scopeNames}
      <p class="mt-1 text-sm text-muted">{m.reports_cashflow_scope({ accounts: scopeNames })}</p>
    {/if}

    {#if multiCommodity}
      <p class="mt-4 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        {m.reports_multi_commodity_notice()}
      </p>
    {/if}

    <div class="mt-5 overflow-x-auto">
      <table class="w-full min-w-[52rem] border-collapse text-left text-sm">
        <caption class="sr-only">{m.reports_cashflow_table_caption()}</caption>
        <thead>
          <tr class="border-b border-border text-xs uppercase tracking-[0.12em] text-muted">
            <th scope="col" class="px-3 py-3 font-semibold">{m.reports_column_period()}</th>
            <th scope="col" class="px-3 py-3 font-semibold">{m.reports_column_commodity()}</th>
            <th scope="col" class="px-3 py-3 text-right font-semibold">{m.reports_column_inflow()}</th>
            <th scope="col" class="px-3 py-3 text-right font-semibold">{m.reports_column_outflow()}</th>
            <th scope="col" class="px-3 py-3 text-right font-semibold">{m.reports_column_operating_net()}</th>
            <th scope="col" class="px-3 py-3 text-right font-semibold">{m.reports_column_transfer_in()}</th>
            <th scope="col" class="px-3 py-3 text-right font-semibold">{m.reports_column_transfer_out()}</th>
            <th scope="col" class="px-3 py-3 text-right font-semibold">{m.reports_column_net_movement()}</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as row (row.key)}
            <tr class="border-b border-border/70 last:border-b-0">
              <td class="px-3 py-3 text-foreground">{formatRange(row.startDate, row.endDate)}</td>
              <td class="px-3 py-3 font-medium text-foreground">
                {row.commodityId === null ? m.reports_cashflow_no_movement() : commodityLabel(row.commodityId)}
              </td>
              <td class="px-3 py-3 text-right tabular-nums text-foreground">
                {amount(row.inflow, row.quantityScale, row.commodityId)}
              </td>
              <td class="px-3 py-3 text-right tabular-nums text-foreground">
                {amount(row.outflow, row.quantityScale, row.commodityId)}
              </td>
              <td class="px-3 py-3 text-right font-semibold tabular-nums {signedClass(row.operatingNet)}">
                {amount(row.operatingNet, row.quantityScale, row.commodityId)}
              </td>
              <td class="px-3 py-3 text-right tabular-nums text-muted">
                {amount(row.transferIn, row.quantityScale, row.commodityId)}
              </td>
              <td class="px-3 py-3 text-right tabular-nums text-muted">
                {amount(row.transferOut, row.quantityScale, row.commodityId)}
              </td>
              <td class="px-3 py-3 text-right font-semibold tabular-nums {signedClass(row.netMovement)}">
                {amount(row.netMovement, row.quantityScale, row.commodityId)}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </Panel>
{/if}
