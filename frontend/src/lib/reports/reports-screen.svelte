<script lang="ts">
  import { browser } from '$app/environment';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { createQuery } from '@tanstack/svelte-query';
  import { parseISO } from 'date-fns';
  import Panel from '$lib/components/panel.svelte';
  import StatePanel from '$lib/components/state-panel.svelte';
  import { currenciesQueryOptions } from '$lib/api/currencies';
  import { netWorthSeriesQueryOptions, type NetWorthSeriesOptions } from '$lib/api/ledger';
  import { formatQuantity } from '$lib/transactions/transaction-labels';
  import { getLocale } from '$lib/paraglide/runtime.js';
  import { m } from '$lib/paraglide/messages.js';
  import { hasMultipleCommodities, netWorthRows } from './net-worth';

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
    const endDate = todayISO();
    return { startDate: `${endDate.slice(0, 4)}-01-01`, endDate, bucket: 'month' };
  }

  function parseFilters(): NetWorthSeriesOptions | null {
    const search = $page.url.searchParams;
    const startDate = search.get('start_date') ?? '';
    const endDate = search.get('end_date') ?? '';
    const bucket = search.get('bucket');
    if (!/^\d{4}-\d{2}-\d{2}$/.test(startDate) || !/^\d{4}-\d{2}-\d{2}$/.test(endDate)) {
      return null;
    }
    if (bucket !== 'day' && bucket !== 'week' && bucket !== 'month' && bucket !== 'quarter' && bucket !== 'year') {
      return null;
    }
    return { startDate, endDate, bucket };
  }

  const activeFilters = $derived.by(parseFilters);
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

  $effect(() => {
    if (browser && activeFilters === null) {
      const defaults = defaultFilters();
      void goto(`/app/reports?start_date=${defaults.startDate}&end_date=${defaults.endDate}&bucket=${defaults.bucket}`, {
        replaceState: true,
        keepFocus: true,
        noScroll: true
      });
    }
  });

  const netWorthQuery = createQuery(() => ({
    ...netWorthSeriesQueryOptions(activeFilters ?? initialFilters),
    enabled: activeFilters !== null
  }));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

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
    const params = new URLSearchParams({ start_date: startDate, end_date: endDate, bucket });
    void goto(`/app/reports?${params.toString()}`, { keepFocus: true, noScroll: true });
  }
</script>

<div class="space-y-5">
  <Panel>
    <form class="flex flex-wrap items-end gap-3" onsubmit={(event) => { event.preventDefault(); applyFilters(); }}>
      <label class="block min-w-40 flex-1 sm:flex-none">
        <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.reports_start_date()}</span>
        <input
          type="date"
          bind:value={startDate}
          class="mt-1.5 h-10 w-full rounded-(--radius-control) border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition hover:bg-control-hover focus:border-accent"
        />
      </label>
      <label class="block min-w-40 flex-1 sm:flex-none">
        <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.reports_end_date()}</span>
        <input
          type="date"
          bind:value={endDate}
          class="mt-1.5 h-10 w-full rounded-(--radius-control) border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition hover:bg-control-hover focus:border-accent"
        />
      </label>
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
      <button
        type="submit"
        class="h-10 rounded-(--radius-control) bg-foreground px-4 text-sm font-semibold text-background transition hover:opacity-90"
      >
        {m.reports_apply_filters()}
      </button>
    </form>
  </Panel>

  {#if netWorthQuery.isPending}
    <StatePanel title={m.reports_loading_title()} copy={m.reports_loading_copy()} />
  {:else if netWorthQuery.isError}
    <StatePanel title={m.reports_error_title()} copy={m.reports_error_copy()}>
      <button
        type="button"
        class="rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
        onclick={() => netWorthQuery.refetch()}
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
    </Panel>
  {/if}
</div>
