<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { parseISO } from 'date-fns';
  import Panel from '$lib/components/panel.svelte';
  import StatePanel from '$lib/components/state-panel.svelte';
  import { m } from '$lib/paraglide/messages.js';
  import { getLocale } from '$lib/paraglide/runtime.js';
  import { formatQuantity } from '$lib/transactions/transaction-labels';
  import { cashflowQueryOptions, type CashflowOptions } from '$lib/api/reports';
  import { cashflowRows, hasMultipleCommodities, isEmptyCashflow, transferDetail, type CashflowRow } from './cashflow';

  let {
    options,
    commodityLabel
  }: {
    options: CashflowOptions;
    commodityLabel: (commodityID: number) => string;
  } = $props();

  const locale = $derived(getLocale());
  const query = createQuery(() => cashflowQueryOptions(options));

  const rows = $derived(query.data ? cashflowRows(query.data) : []);
  const multiCommodity = $derived(hasMultipleCommodities(rows));
  const scope = $derived(query.data?.query.cash_scope ?? 'default_liquid_cash');
  const resolvedAccountCount = $derived(query.data?.query.filters.resolved_account_ids.length ?? 0);

  const dateFormatter = $derived(
    new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'short', day: 'numeric' })
  );

  // date-fns rather than the Date constructor: calendar dates are financial
  // facts here, and the constructor's parsing is not a contract worth relying on.
  function formatDate(date: string): string {
    return dateFormatter.format(parseISO(date));
  }

  function formatRange(start: string, end: string): string {
    if (start === end) return formatDate(start);
    return m.reports_date_range({ start: formatDate(start), end: formatDate(end) });
  }

  function amount(value: string, row: CashflowRow): string {
    return formatQuantity(value, row.quantityScale, locale);
  }

  /**
   * A signed figure gets an explicit sign as well as its colour, because colour
   * alone is not a cue every reader has. `formatQuantity` already renders a
   * leading minus, so only the positive case needs marking.
   */
  function signedAmount(value: string, row: CashflowRow): string {
    const formatted = amount(value, row);
    return value.startsWith('-') || value === '0' ? formatted : `+${formatted}`;
  }

  function toneFor(value: string): string {
    if (value === '0') return 'text-foreground';
    return value.startsWith('-') ? 'text-danger' : 'text-positive';
  }

  // Responsive column priority. The period and the net movement are the two
  // things that must survive at any width; the split that explains the net
  // comes next, and the derived subtotals go first.
  const derivedCellClass = 'hidden md:table-cell';
  const commodityCellClass = $derived(multiCommodity ? '' : 'hidden min-[600px]:table-cell');
</script>

<Panel>
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div>
      <h2 class="text-lg font-semibold tracking-tight text-foreground">{m.reports_cashflow_title()}</h2>
      <p class="mt-1 text-sm text-muted">{m.reports_cashflow_copy()}</p>
    </div>
    <p class="text-sm text-muted">{m.reports_posted_only()}</p>
  </div>

  <!-- The scope is never an invisible default: the report says which accounts
       it measured, so "my cash" is never silently a different set. -->
  <p class="mt-4 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
    {#if scope === 'selected_accounts'}
      {m.reports_cashflow_scope_selected({ count: resolvedAccountCount })}
    {:else}
      {m.reports_cashflow_scope_default({ count: resolvedAccountCount })}
    {/if}
  </p>

  <!-- Rule 3: movement to an account outside the scope is financing movement,
       never spending. Saying so is part of the report, not a footnote. -->
  <p class="mt-2 text-xs text-muted">{m.reports_cashflow_transfer_policy()}</p>

  {#if query.isPending}
    <div class="mt-5">
      <StatePanel title={m.reports_cashflow_loading_title()} copy={m.reports_cashflow_loading_copy()} />
    </div>
  {:else if query.isError}
    <div class="mt-5">
      <StatePanel title={m.reports_cashflow_error_title()} copy={m.reports_cashflow_error_copy()}>
        <button
          type="button"
          class="rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
          onclick={() => query.refetch()}
        >
          {m.reports_retry()}
        </button>
      </StatePanel>
    </div>
  {:else if isEmptyCashflow(rows)}
    <div class="mt-5">
      <StatePanel title={m.reports_cashflow_empty_title()} copy={m.reports_cashflow_empty_copy()} />
    </div>
  {:else}
    {#if multiCommodity}
      <p class="mt-4 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        {m.reports_multi_commodity_notice()}
      </p>
    {/if}

    <div class="mt-5 overflow-x-auto">
      <table class="w-full border-collapse text-left text-sm">
        <caption class="sr-only">{m.reports_cashflow_title()}</caption>
        <thead>
          <tr class="border-b border-border text-xs uppercase tracking-[0.12em] text-muted">
            <th scope="col" class="px-3 py-3 font-semibold">{m.reports_column_period()}</th>
            <th scope="col" class={`px-3 py-3 font-semibold ${commodityCellClass}`}>
              {m.reports_column_commodity()}
            </th>
            <th scope="col" class="px-3 py-3 text-right font-semibold">{m.reports_cashflow_column_inflow()}</th>
            <th scope="col" class="px-3 py-3 text-right font-semibold">{m.reports_cashflow_column_outflow()}</th>
            <th scope="col" class={`px-3 py-3 text-right font-semibold ${derivedCellClass}`}>
              {m.reports_cashflow_column_operating_net()}
            </th>
            <th scope="col" class={`px-3 py-3 text-right font-semibold ${derivedCellClass}`}>
              {m.reports_cashflow_column_transfers()}
            </th>
            <th scope="col" class="px-3 py-3 text-right font-semibold">
              {m.reports_cashflow_column_net_movement()}
            </th>
          </tr>
        </thead>
        <tbody>
          {#each rows as row (row.key)}
            <tr class="border-b border-border/70 last:border-b-0">
              <td class="px-3 py-3 text-foreground">{formatRange(row.startDate, row.endDate)}</td>
              <td class={`px-3 py-3 font-medium text-foreground ${commodityCellClass}`}>
                {commodityLabel(row.commodityID)}
              </td>
              <td class="px-3 py-3 text-right tabular-nums text-foreground">{amount(row.inflow, row)}</td>
              <td class="px-3 py-3 text-right tabular-nums text-foreground">{amount(row.outflow, row)}</td>
              <td class={`px-3 py-3 text-right tabular-nums ${derivedCellClass} ${toneFor(row.operatingNet)}`}>
                {signedAmount(row.operatingNet, row)}
              </td>
              <td class={`px-3 py-3 text-right tabular-nums ${derivedCellClass} ${toneFor(row.transferNet)}`}>
                {signedAmount(row.transferNet, row)}
                <!-- The incoming/outgoing pair behind the net, so a financing
                     figure can be read without a second query. Only the sides
                     that actually moved are named. -->
                {#if transferDetail(row) === 'both'}
                  <span class="block text-xs font-normal text-muted">
                    {m.reports_cashflow_transfer_detail_both({
                      incoming: amount(row.transferIn, row),
                      outgoing: amount(row.transferOut, row)
                    })}
                  </span>
                {:else if transferDetail(row) === 'in'}
                  <span class="block text-xs font-normal text-muted">
                    {m.reports_cashflow_transfer_detail_in({ incoming: amount(row.transferIn, row) })}
                  </span>
                {:else if transferDetail(row) === 'out'}
                  <span class="block text-xs font-normal text-muted">
                    {m.reports_cashflow_transfer_detail_out({ outgoing: amount(row.transferOut, row) })}
                  </span>
                {/if}
              </td>
              <td class={`px-3 py-3 text-right font-semibold tabular-nums ${toneFor(row.netMovement)}`}>
                {signedAmount(row.netMovement, row)}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <p class="mt-4 text-xs text-muted">{m.reports_cashflow_identity_note()}</p>
  {/if}
</Panel>
