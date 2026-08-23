<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import Panel from '$lib/components/panel.svelte';
  import StatePanel from '$lib/components/state-panel.svelte';
  import { m } from '$lib/paraglide/messages.js';
  import { getLocale } from '$lib/paraglide/runtime.js';
  import { formatQuantity } from '$lib/money/format';
  import { categoriesQueryOptions, type CategoryResponse } from '$lib/api/categories';
  import { categoryDisplayName } from '$lib/categories/category-labels';
  import {
    spendingQueryOptions,
    type SpendingGroupBy,
    type SpendingMode,
    type SpendingOptions
  } from '$lib/api/reports';
  import SpendingBarChart from './spending-bar-chart.svelte';
  import { csvFilename, downloadCSV, exactDecimal, toCSV, withReportContext } from './report-csv';
  import { reportErrorState } from './report-error';
  import {
    formatShare,
    isSingleCommodity,
    spendingBars,
    spendingDrillDownHref,
    spendingRows,
    type SpendingRow
  } from './spending';

  let {
    options,
    commodityLabel,
    reportContext,
    onGroupByChange,
    onModeChange
  }: {
    options: SpendingOptions;
    commodityLabel: (commodityID: number) => string;
    /** What the shell measured, restated for an export that outlives the screen. */
    reportContext: string[];
    /** Changes only group_by in the URL, preserving every compatible filter. */
    onGroupByChange: (groupBy: SpendingGroupBy) => void;
    onModeChange: (mode: SpendingMode) => void;
  } = $props();

  const locale = $derived(getLocale());
  const query = createQuery(() => spendingQueryOptions(options));
  // Retrying cannot undo an overflow or repair a rejected query, so the
  // stable code speaks for itself and the retry button stands down.
  const errorState = $derived(reportErrorState(query.error, m.reports_spending_error_copy()));

  /** The endpoint's exclusion policy — the one fact with no place on screen. */
  const exclusionContext = $derived(
    query.data && query.data.excluded_system_roles.length > 0
      ? [
          m.reports_context_line({
            label: m.reports_context_excluded(),
            values: query.data.excluded_system_roles.join(', ')
          })
        ]
      : []
  );

  // Built-in categories arrive as stable codes with no name; display text is
  // resolved here at render time through the canonical label helper, which also
  // honours a user rename. One request per page, never per row.
  const categoriesQuery = createQuery(() => categoriesQueryOptions({ includeArchived: true }));
  const categoriesByID = $derived.by(() => {
    const byID = new Map<number, CategoryResponse>();
    for (const category of categoriesQuery.data?.categories ?? []) {
      byID.set(category.id, category);
    }
    return byID;
  });

  const rows = $derived(query.data ? spendingRows(query.data) : []);
  const singleCommodity = $derived(query.data ? isSingleCommodity(query.data) : false);
  const groupBy = $derived(options.groupBy);

  /**
   * This report's own policy, beyond the shell's common context.
   *
   * The grouping policy is part of the answer — a parent category not including
   * its children changes what every row means — and the exclusion list is what
   * the endpoint left out. One list, so the printed page and the exported file
   * cannot come to disagree about what the report was.
   */
  const policyContext = $derived([
    ...(groupBy === 'category' ? [m.reports_spending_grouping_policy()] : []),
    ...exclusionContext
  ]);
  const mode = $derived(options.mode ?? 'spending');

  function rowLabel(row: SpendingRow): string {
    if (row.unattributed) {
      return groupBy === 'payee' ? m.reports_spending_no_payee() : m.reports_spending_no_category();
    }
    if (row.categoryID !== undefined) {
      const category = categoriesByID.get(row.categoryID);
      if (category) {
        return categoryDisplayName(category);
      }
    }
    return row.name || row.code || m.reports_spending_unnamed();
  }

  function drillDownHref(row: SpendingRow): string | undefined {
    return spendingDrillDownHref(row, groupBy, mode);
  }

  function exportCSV() {
    const header = [
      groupBy === 'payee' ? m.reports_column_payee() : m.reports_column_category(),
      m.reports_column_commodity(),
      mode === 'income' ? m.reports_column_income() : m.reports_column_spending(),
      m.reports_column_share()
    ];
    const body = rows.map((row) => [
      rowLabel(row),
      commodityLabel(row.commodityID),
      exactDecimal(row.quantityValue, row.quantityScale),
      // The share is exact basis points from the server; a percentage with a
      // locale separator would not parse, so it goes out as basis points.
      row.shareBasisPoints === undefined ? '' : String(row.shareBasisPoints)
    ]);
    downloadCSV(
      csvFilename(mode === 'income' ? 'income' : 'spending', options.startDate, options.endDate),
      toCSV(withReportContext([header, ...body], [...reportContext, ...policyContext]))
    );
  }

  function formatAmount(value: string, scale: number): string {
    return formatQuantity(value, scale, locale);
  }

  function formattedTotal(row: SpendingRow): string {
    return `${commodityLabel(row.commodityID)} ${formatAmount(row.quantityValue, row.quantityScale)}`;
  }

  // The chart only summarizes a single-commodity report: bars across unlike
  // commodities would be a comparison the ledger cannot justify.
  const bars = $derived(singleCommodity ? spendingBars(rows, rowLabel) : []);

  const dimensionHeader = $derived(
    groupBy === 'payee' ? m.reports_column_payee() : m.reports_column_category()
  );
  const amountHeader = $derived(
    mode === 'income' ? m.reports_column_income() : m.reports_column_spending()
  );

  // Responsive column priority: the amount must survive at every width. The
  // commodity code is already inside the amount cell, so that column is the
  // first to hide when a single commodity is in range; share hides next.
  const commodityCellClass = $derived(
    singleCommodity ? 'hidden min-[600px]:table-cell' : ''
  );
  const shareCellClass = 'hidden min-[460px]:table-cell';
</script>

<Panel>
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div>
      <h2 class="text-lg font-semibold tracking-tight text-foreground">
        {mode === 'income' ? m.reports_income_title() : m.reports_spending_title()}
      </h2>
      <p class="mt-1 text-sm text-muted">
        {mode === 'income' ? m.reports_income_copy() : m.reports_spending_copy()}
      </p>
    </div>
    <div class="flex items-center gap-3">
      <p class="text-sm text-muted">{m.reports_posted_only()}</p>
      {#if rows.length > 0}
        <button type="button" class="report-export" onclick={exportCSV}>{m.reports_export_csv()}</button>
      {/if}
    </div>
  </div>

  <div class="mt-4 flex flex-wrap gap-3">
    <fieldset class="flex flex-wrap items-center gap-2">
      <legend class="sr-only">{m.reports_spending_group_by_legend()}</legend>
      <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">
        {m.reports_spending_group_by_legend()}
      </span>
      {#each [{ value: 'category' as const, label: m.reports_column_category() }, { value: 'payee' as const, label: m.reports_column_payee() }] as choice (choice.value)}
        <button
          type="button"
          aria-pressed={groupBy === choice.value}
          onclick={() => onGroupByChange(choice.value)}
          class={`h-9 rounded-(--radius-control) border px-3 text-sm font-medium transition ${
            groupBy === choice.value
              ? 'border-accent bg-accent text-accent-foreground'
              : 'border-border bg-control text-foreground hover:bg-control-hover'
          }`}
        >
          {choice.label}
        </button>
      {/each}
    </fieldset>

    <fieldset class="flex flex-wrap items-center gap-2">
      <legend class="sr-only">{m.reports_spending_mode_legend()}</legend>
      <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">
        {m.reports_spending_mode_legend()}
      </span>
      {#each [{ value: 'spending' as const, label: m.reports_spending_mode_spending() }, { value: 'income' as const, label: m.reports_spending_mode_income() }] as choice (choice.value)}
        <button
          type="button"
          aria-pressed={mode === choice.value}
          onclick={() => onModeChange(choice.value)}
          class={`h-9 rounded-(--radius-control) border px-3 text-sm font-medium transition ${
            mode === choice.value
              ? 'border-accent bg-accent text-accent-foreground'
              : 'border-border bg-control text-foreground hover:bg-control-hover'
          }`}
        >
          {choice.label}
        </button>
      {/each}
    </fieldset>
  </div>

  {#if query.isPending}
    <div class="mt-5">
      <StatePanel title={m.reports_spending_loading_title()} copy={m.reports_spending_loading_copy()} />
    </div>
  {:else if query.isError}
    <div class="mt-5">
      <StatePanel title={m.reports_spending_error_title()} copy={errorState.copy}>
        {#if errorState.retryable}
          <button
            type="button"
            class="rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
            onclick={() => query.refetch()}
          >
            {m.reports_retry()}
          </button>
        {/if}
      </StatePanel>
    </div>
  {:else if rows.length === 0}
    <div class="mt-5">
      <StatePanel
        title={mode === 'income' ? m.reports_income_empty_title() : m.reports_spending_empty_title()}
        copy={m.reports_spending_empty_copy()}
      />
    </div>
  {:else}
    {#if !singleCommodity}
      <p class="mt-4 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        {m.reports_spending_multi_commodity_notice()}
      </p>
    {/if}

    <div class="mt-5 overflow-x-auto">
      <table class="w-full border-collapse text-left text-sm">
        <caption class="sr-only">
          {mode === 'income' ? m.reports_income_title() : m.reports_spending_title()}
        </caption>
        <thead>
          <tr class="border-b border-border text-xs uppercase tracking-[0.12em] text-muted">
            <th scope="col" class="px-3 py-3 font-semibold">{dimensionHeader}</th>
            <th scope="col" class={`px-3 py-3 font-semibold ${commodityCellClass}`}>
              {m.reports_column_commodity()}
            </th>
            <th scope="col" class="px-3 py-3 text-right font-semibold">{amountHeader}</th>
            <th scope="col" class={`px-3 py-3 text-right font-semibold ${shareCellClass}`}>
              {m.reports_column_share()}
            </th>
          </tr>
        </thead>
        <tbody>
          {#each rows as row (row.key)}
            <tr class="border-b border-border/70 last:border-b-0">
              <td class="px-3 py-3 text-foreground">
                <!-- Linked only when the transactions route can ask the same
                     question; otherwise the label stays plain text rather than
                     leading to a list that disagrees with the number. -->
                {#if drillDownHref(row)}
                  <a
                    href={drillDownHref(row)}
                    class="font-medium text-accent underline underline-offset-2 transition hover:opacity-80"
                  >
                    {rowLabel(row)}
                  </a>
                {:else}
                  {rowLabel(row)}
                {/if}
                {#if row.unattributed}
                  <span class="ml-1 text-xs text-muted">({m.reports_spending_unattributed_hint()})</span>
                {/if}
              </td>
              <td class={`px-3 py-3 font-medium text-foreground ${commodityCellClass}`}>
                {commodityLabel(row.commodityID)}
              </td>
              <td class="px-3 py-3 text-right font-semibold tabular-nums text-foreground">
                {formattedTotal(row)}
              </td>
              <td class={`px-3 py-3 text-right tabular-nums text-muted ${shareCellClass}`}>
                {formatShare(row.shareBasisPoints, locale) || m.reports_spending_share_unavailable()}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if singleCommodity && bars.length > 0}
      <SpendingBarChart
        {bars}
        {formatAmount}
        caption={m.reports_spending_chart_caption({
          commodity: commodityLabel(rows[0].commodityID)
        })}
      />
    {/if}

    <!-- The grouping policy note is about category depth; it says nothing about payees. -->
    {#if groupBy === 'category'}
      <p class="mt-4 text-xs text-muted">{m.reports_spending_grouping_policy()}</p>
    {/if}
    <!--
      The grouping policy is ordinary screen content and prints as it is. The
      exclusion policy has nowhere on screen it belongs, so it is added on paper
      — which is what makes the printed page carry the same facts as the export,
      without saying anything twice.
    -->
    <div class="hidden text-xs text-muted print:block">
      {#each exclusionContext as line (line)}
        <p>{line}</p>
      {/each}
    </div>
  {/if}
</Panel>
