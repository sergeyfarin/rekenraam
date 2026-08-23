<script lang="ts">
  import { browser } from '$app/environment';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { createQuery } from '@tanstack/svelte-query';
  import { parseISO } from 'date-fns';
  import Panel from '$lib/components/panel.svelte';
  import StatePanel from '$lib/components/state-panel.svelte';
  import { currenciesQueryOptions } from '$lib/api/currencies';
  import {
    netWorthSeriesQueryOptions,
    type CashflowOptions,
    type NetWorthSeriesOptions,
    type SpendingGroupBy,
    type SpendingMode,
    type SpendingOptions
  } from '$lib/api/reports';
  import { formatQuantity } from '$lib/money/format';
  import { getLocale } from '$lib/paraglide/runtime.js';
  import { m } from '$lib/paraglide/messages.js';
  import { hasMultipleCommodities, netWorthRows } from './net-worth';
  import BucketColumnChart from './bucket-column-chart.svelte';
  import CashflowView from './cashflow-view.svelte';
  import { csvFilename, downloadCSV, exactDecimal, toCSV } from './report-csv';
  import { seriesColumns } from './report-series';
  import ReportFilterControls from './report-filter-controls.svelte';
  import {
    parseReportFilters,
    writeReportFilters,
    type ReportFilterDimension,
    type ReportFilterState
  } from './report-filters';
  import { defaultReportRange, parseReportRange, repairReportRange } from './report-range';
  import { reportErrorState } from './report-error';
  import SpendingView from './spending-view.svelte';

  type ReportView = 'net-worth' | 'spending' | 'cashflow';

  const locale = $derived(getLocale());
  const dateFormatter = $derived(
    new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'short', day: 'numeric' })
  );

  function todayISO(): string {
    const parts = new Intl.DateTimeFormat('en-CA', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit'
    }).formatToParts();
    const value = (type: Intl.DateTimeFormatPartTypes) => parts.find((part) => part.type === type)?.value ?? '';
    return `${value('year')}-${value('month')}-${value('day')}`;
  }

  function defaultFilters(): NetWorthSeriesOptions {
    return defaultReportRange(todayISO());
  }

  function parseFilters(): NetWorthSeriesOptions | null {
    return parseReportRange($page.url.searchParams);
  }

  function parseView(): ReportView {
    const view = $page.url.searchParams.get('view');
    if (view === 'spending' || view === 'cashflow') {
      return view;
    }
    return 'net-worth';
  }

  function parseGroupBy(): SpendingGroupBy {
    return $page.url.searchParams.get('group_by') === 'payee' ? 'payee' : 'category';
  }

  function parseMode(): SpendingMode {
    return $page.url.searchParams.get('mode') === 'income' ? 'income' : 'spending';
  }

  const activeFilters = $derived.by(parseFilters);
  const view = $derived.by(parseView);
  const reportFilters = $derived(parseReportFilters($page.url.searchParams));
  // Net worth has no category or payee dimension: those exist only where a
  // category posting does. Offering them here would be a control that changes
  // nothing.
  const filterDimensions = $derived<ReportFilterDimension[]>(
    view === 'spending' ? ['account', 'commodity', 'category', 'payee'] : ['account', 'commodity']
  );
  // Cashflow is a series like net worth, so it takes the bucket control. On
  // cashflow the account filter is not a narrowing of a fixed basis — it *is*
  // the cash scope, which the view names in its own summary.
  const bucketedView = $derived(view !== 'spending');
  const groupBy = $derived.by(parseGroupBy);
  const mode = $derived.by(parseMode);
  const initialFilters = defaultFilters();
  let startDate = $state(initialFilters.startDate);
  let endDate = $state(initialFilters.endDate);
  let bucket = $state<NetWorthSeriesOptions['bucket']>(initialFilters.bucket);

  $effect(() => {
    if (activeFilters) {
      startDate = activeFilters.startDate;
      endDate = activeFilters.endDate;
      bucket = activeFilters.bucket;
    }
  });

  // A link with a missing or malformed range is repaired in place, never
  // rebuilt: `/app/reports?view=spending&group_by=payee&account_id=12` asks a
  // specific question, and answering it with the default net-worth report would
  // silently discard every selection the sender made.
  $effect(() => {
    if (!browser || activeFilters !== null) {
      return;
    }
    const repaired = repairReportRange($page.url.searchParams, todayISO());
    if (repaired) {
      void goto(`/app/reports?${repaired.toString()}`, {
        replaceState: true,
        keepFocus: true,
        noScroll: true
      });
    }
  });

  // The spending report shares the shell's date range. Bucketing is a net-worth
  // concern only: spending ranks one range, it is not a series.
  const spendingOptions = $derived<SpendingOptions>({
    startDate: (activeFilters ?? initialFilters).startDate,
    endDate: (activeFilters ?? initialFilters).endDate,
    groupBy,
    mode,
    accountIDs: reportFilters.accountIDs,
    includeDescendants: reportFilters.includeDescendants,
    commodityIDs: reportFilters.commodityIDs,
    categoryIDs: reportFilters.categoryIDs,
    payeeIDs: reportFilters.payeeIDs
  });

  const netWorthOptions = $derived<NetWorthSeriesOptions>({
    ...(activeFilters ?? initialFilters),
    accountIDs: reportFilters.accountIDs,
    includeDescendants: reportFilters.includeDescendants,
    commodityIDs: reportFilters.commodityIDs
  });

  const cashflowOptions = $derived<CashflowOptions>({
    ...(activeFilters ?? initialFilters),
    accountIDs: reportFilters.accountIDs,
    includeDescendants: reportFilters.includeDescendants,
    commodityIDs: reportFilters.commodityIDs
  });

  const netWorthQuery = createQuery(() => ({
    ...netWorthSeriesQueryOptions(netWorthOptions),
    enabled: activeFilters !== null && view === 'net-worth'
  }));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  // An overflow or a rejected query is not something retrying can fix, so the
  // stable code speaks for itself and the retry button stands down.
  const netWorthError = $derived(reportErrorState(netWorthQuery.error, m.reports_error_copy()));
  const rows = $derived(netWorthQuery.data ? netWorthRows(netWorthQuery.data) : []);
  const multiCommodity = $derived(hasMultipleCommodities(rows));
  const currencyCodeByID = $derived.by(() => {
    const codes = new Map<number, string>();
    for (const currency of currenciesQuery.data?.currencies ?? []) {
      codes.set(currency.id, currency.code);
    }
    return codes;
  });

  function formatDate(date: string): string {
    return dateFormatter.format(parseISO(date));
  }

  // Axis labels get their own compact format: a full date is wider than a
  // column, and overlapping labels are worse than terse ones.
  const shortDateFormatter = $derived(new Intl.DateTimeFormat(locale, { month: 'short', day: 'numeric' }));

  function shortDate(date: string): string {
    return shortDateFormatter.format(parseISO(date));
  }

  // One commodity only: columns across unlike commodities would compare what
  // the ledger cannot.
  const netWorthColumns = $derived(
    multiCommodity
      ? []
      : seriesColumns(
          rows.map((row) => ({
            key: `${row.startDate}-${row.commodity_id}`,
            label: shortDate(row.endDate),
            quantityValue: row.normal_quantity_value,
            quantityScale: row.quantity_scale
          }))
        )
  );

  function exportNetWorthCSV() {
    const header = [
      m.reports_column_period_start(),
      m.reports_column_period_end(),
      m.reports_column_commodity(),
      m.reports_column_net_worth()
    ];
    const body = rows.map((row) => [
      row.startDate,
      row.endDate,
      commodityLabel(row.commodity_id),
      exactDecimal(row.normal_quantity_value, row.quantity_scale)
    ]);
    const range = activeFilters ?? initialFilters;
    downloadCSV(csvFilename('net-worth', range.startDate, range.endDate), toCSV([header, ...body]));
  }

  function formatRange(start: string, end: string): string {
    return start === end ? formatDate(start) : m.reports_date_range({ start: formatDate(start), end: formatDate(end) });
  }

  function commodityLabel(commodityID: number): string {
    return currencyCodeByID.get(commodityID) ?? m.reports_commodity_unknown({ commodityId: commodityID });
  }

  function formattedTotal(value: string, scale: number, commodityID: number): string {
    return `${commodityLabel(commodityID)} ${formatQuantity(value, scale, locale)}`;
  }

  function applyFilters() {
    const params = new URLSearchParams($page.url.searchParams);
    params.set('start_date', startDate);
    params.set('end_date', endDate);
    params.set('bucket', bucket);
    void goto(`/app/reports?${params.toString()}`, { keepFocus: true, noScroll: true });
  }

  // Every control writes the URL, then the typed query follows. Switching one
  // dimension preserves the rest so a shared link keeps its full state.
  function setParam(name: string, value: string) {
    const params = new URLSearchParams($page.url.searchParams);
    params.set(name, value);
    void goto(`/app/reports?${params.toString()}`, { keepFocus: true, noScroll: true });
  }

  // Filter selections apply immediately rather than waiting for Apply: the date
  // inputs need a submit because a half-typed date is not a query, but a
  // checkbox is never half-set. A selection a net-worth request cannot express
  // stays in the URL, so switching back to spending restores it.
  function applyReportFilters(next: ReportFilterState) {
    const params = writeReportFilters($page.url.searchParams, next);
    void goto(`/app/reports?${params.toString()}`, { keepFocus: true, noScroll: true });
  }
</script>

<div class="space-y-5">
  <!--
    The filter form and view switcher are interactive and mean nothing on
    paper, but the range they hold does — so the print sheet restates it as
    text rather than dropping that context along with the form.
  -->
  <p class="hidden text-sm text-muted print:block">
    {m.reports_print_filters({
      range: formatRange((activeFilters ?? initialFilters).startDate, (activeFilters ?? initialFilters).endDate),
      basis: m.reports_posted_only()
    })}
  </p>

  <div data-print-hide>
  <Panel>
    <form class="flex flex-wrap items-end gap-3" onsubmit={(event) => { event.preventDefault(); applyFilters(); }}>
      <label class="block min-w-40 flex-1 sm:flex-none">
        <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.reports_start_date()}</span>
        <input
          type="date"
          bind:value={startDate}
          max={endDate}
          class="mt-1.5 h-10 w-full rounded-(--radius-control) border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition hover:bg-control-hover focus:border-accent"
        />
      </label>
      <label class="block min-w-40 flex-1 sm:flex-none">
        <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.reports_end_date()}</span>
        <input
          type="date"
          bind:value={endDate}
          min={startDate}
          class="mt-1.5 h-10 w-full rounded-(--radius-control) border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition hover:bg-control-hover focus:border-accent"
        />
      </label>
      <!-- Bucketing belongs to the series reports; spending ranks one range. -->
      {#if bucketedView}
        <label class="block min-w-36 flex-1 sm:flex-none">
          <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.reports_bucket()}</span>
          <select
            bind:value={bucket}
            class="mt-1.5 h-10 w-full rounded-(--radius-control) border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition hover:bg-control-hover focus:border-accent"
          >
            <option value="day">{m.reports_bucket_day()}</option>
            <option value="week">{m.reports_bucket_week()}</option>
            <option value="month">{m.reports_bucket_month()}</option>
            <option value="quarter">{m.reports_bucket_quarter()}</option>
            <option value="year">{m.reports_bucket_year()}</option>
          </select>
        </label>
      {/if}
      <button
        type="submit"
        class="h-10 rounded-(--radius-control) bg-foreground px-4 text-sm font-semibold text-background transition hover:opacity-90"
      >
        {m.reports_apply_filters()}
      </button>
    </form>

    <div class="mt-5 border-t border-border pt-5">
      <ReportFilterControls
        filters={reportFilters}
        dimensions={filterDimensions}
        accountScope={view === 'cashflow' ? 'cash-scope' : 'basis'}
        onChange={applyReportFilters}
      />
    </div>
  </Panel>

  <nav aria-label={m.reports_view_switch_legend()} class="mt-5 flex flex-wrap gap-2">
    {#each [{ value: 'net-worth' as const, label: m.reports_view_net_worth() }, { value: 'spending' as const, label: m.reports_view_spending() }, { value: 'cashflow' as const, label: m.reports_view_cashflow() }] as choice (choice.value)}
      <button
        type="button"
        aria-current={view === choice.value ? 'page' : undefined}
        onclick={() => setParam('view', choice.value)}
        class={`h-9 rounded-(--radius-control) border px-3 text-sm font-medium transition ${
          view === choice.value
            ? 'border-accent bg-accent text-accent-foreground'
            : 'border-border bg-control text-foreground hover:bg-control-hover'
        }`}
      >
        {choice.label}
      </button>
    {/each}
  </nav>
  </div>

  {#if view === 'cashflow'}
    <CashflowView options={cashflowOptions} {commodityLabel} />
  {:else if view === 'spending'}
    <SpendingView
      options={spendingOptions}
      {commodityLabel}
      onGroupByChange={(next) => setParam('group_by', next)}
      onModeChange={(next) => setParam('mode', next)}
    />
  {:else if netWorthQuery.isPending}
    <StatePanel title={m.reports_loading_title()} copy={m.reports_loading_copy()} />
  {:else if netWorthQuery.isError}
    <StatePanel title={m.reports_error_title()} copy={netWorthError.copy}>
      {#if netWorthError.retryable}
        <button
          type="button"
          class="rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
          onclick={() => netWorthQuery.refetch()}
        >
          {m.reports_retry()}
        </button>
      {/if}
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
        <div class="flex items-center gap-3">
          <p class="text-sm text-muted">{m.reports_posted_only()}</p>
          <button type="button" class="report-export" onclick={exportNetWorthCSV}>
            {m.reports_export_csv()}
          </button>
        </div>
      </div>

      {#if multiCommodity}
        <p class="mt-4 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
          {m.reports_multi_commodity_notice()}
        </p>
      {/if}

      <div class="mt-5 overflow-x-auto">
        <table class="w-full min-w-[34rem] border-collapse text-left text-sm">
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
                  {formattedTotal(row.normal_quantity_value, row.quantity_scale, row.commodity_id)}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      {#if netWorthColumns.length > 0}
        <BucketColumnChart
          columns={netWorthColumns}
          formatAmount={(value, scale) => formatQuantity(value, scale, locale)}
          caption={m.reports_net_worth_chart_caption({ commodity: commodityLabel(rows[0].commodity_id) })}
        />
      {/if}
    </Panel>
  {/if}
</div>
