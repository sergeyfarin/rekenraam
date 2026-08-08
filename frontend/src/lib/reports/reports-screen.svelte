<script lang="ts">
  import { browser } from '$app/environment';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { createQuery } from '@tanstack/svelte-query';
  import { parseISO } from 'date-fns';
  import Panel from '$lib/components/panel.svelte';
  import { currenciesQueryOptions } from '$lib/api/currencies';
  import { getLocale } from '$lib/paraglide/runtime.js';
  import { m } from '$lib/paraglide/messages.js';
  import CashflowReport from './cashflow-report.svelte';
  import NetWorthReport from './net-worth-report.svelte';
  import SpendingReport from './spending-report.svelte';
  import {
    defaultFilters,
    parseReportFilters,
    reportFiltersToHref,
    type ReportFilters,
    type ReportView
  } from './report-filters';

  const locale = $derived(getLocale());
  const dateFormatter = $derived(
    new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'short', day: 'numeric' })
  );

  // The URL is the source of truth for filter state; the form fields below are
  // a draft of the next URL, not the state the report is reading.
  const activeFilters = $derived.by(() => parseReportFilters($page.url.searchParams));
  const initialFilters = defaultFilters();

  let draft = $state<ReportFilters>({ ...initialFilters });

  $effect(() => {
    if (activeFilters) {
      draft = { ...activeFilters };
    }
  });

  $effect(() => {
    // A missing or malformed range redirects to the resolved default preset
    // rather than reporting over a range the user never chose. replaceState
    // keeps the bad URL out of the back-button history.
    if (browser && activeFilters === null) {
      void goto(reportFiltersToHref(defaultFilters()), {
        replaceState: true,
        keepFocus: true,
        noScroll: true
      });
    }
  });

  const currenciesQuery = createQuery(() => currenciesQueryOptions());
  const currencyCodeByID = $derived.by(() => {
    const codes = new Map<number, string>();
    for (const currency of currenciesQuery.data?.currencies ?? []) {
      codes.set(currency.id, currency.code);
    }
    return codes;
  });

  function commodityLabel(commodityID: number): string {
    return currencyCodeByID.get(commodityID) ?? m.reports_commodity_unknown({ commodityId: commodityID });
  }

  function formatDate(date: string): string {
    return dateFormatter.format(parseISO(date));
  }

  function formatRange(start: string, end: string): string {
    return start === end
      ? formatDate(start)
      : m.reports_date_range({ start: formatDate(start), end: formatDate(end) });
  }

  function viewLabel(view: ReportView): string {
    switch (view) {
      case 'net-worth':
        return m.reports_view_net_worth();
      case 'spending':
        return m.reports_view_spending();
      case 'cashflow':
        return m.reports_view_cashflow();
    }
  }

  // Switching view keeps every other filter, so a date range chosen on one
  // report carries to the next instead of silently resetting.
  function viewHref(view: ReportView): string {
    return reportFiltersToHref({ ...(activeFilters ?? initialFilters), view });
  }

  function applyFilters() {
    void goto(reportFiltersToHref(draft), { keepFocus: true, noScroll: true });
  }

  const activeView = $derived(activeFilters?.view ?? initialFilters.view);
  const controlClass =
    'mt-1.5 h-10 w-full rounded-(--radius-control) border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition hover:bg-control-hover focus:border-accent';
  const labelClass = 'text-xs font-semibold uppercase tracking-[0.12em] text-muted';
</script>

<div class="space-y-5">
  <nav aria-label={m.reports_view_nav_label()} class="print:hidden">
    <ul class="flex flex-wrap gap-2">
      {#each ['net-worth', 'spending', 'cashflow'] as const as view (view)}
        <li>
          <a
            href={viewHref(view)}
            aria-current={activeView === view ? 'page' : undefined}
            class="inline-flex h-10 items-center rounded-(--radius-control) border px-4 text-sm font-semibold transition {activeView ===
            view
              ? 'border-foreground bg-foreground text-background'
              : 'border-border bg-control text-foreground hover:bg-control-hover'}"
          >
            {viewLabel(view)}
          </a>
        </li>
      {/each}
    </ul>
  </nav>

  <!--
    The filter form is interactive and means nothing on paper, but the filters
    it holds do — so the print sheet below restates the active range as text
    rather than dropping that context from the printed document.
  -->
  <p class="hidden text-sm text-muted print:block">
    {m.reports_print_filters({
      range: formatRange(
        (activeFilters ?? initialFilters).startDate,
        (activeFilters ?? initialFilters).endDate
      ),
      basis: m.reports_posted_only()
    })}
  </p>

  <div class="print:hidden">
  <Panel>
    <form
      class="flex flex-wrap items-end gap-3"
      onsubmit={(event) => {
        event.preventDefault();
        applyFilters();
      }}
    >
      <label class="block min-w-40 flex-1 sm:flex-none">
        <span class={labelClass}>{m.reports_start_date()}</span>
        <input type="date" bind:value={draft.startDate} max={draft.endDate} class={controlClass} />
      </label>
      <label class="block min-w-40 flex-1 sm:flex-none">
        <span class={labelClass}>{m.reports_end_date()}</span>
        <input type="date" bind:value={draft.endDate} min={draft.startDate} class={controlClass} />
      </label>

      {#if activeView === 'net-worth' || activeView === 'cashflow'}
        <label class="block min-w-36 flex-1 sm:flex-none">
          <span class={labelClass}>{m.reports_bucket()}</span>
          <select bind:value={draft.bucket} class={controlClass}>
            <option value="day">{m.reports_bucket_day()}</option>
            <option value="week">{m.reports_bucket_week()}</option>
            <option value="month">{m.reports_bucket_month()}</option>
            <option value="quarter">{m.reports_bucket_quarter()}</option>
            <option value="year">{m.reports_bucket_year()}</option>
          </select>
        </label>
      {/if}

      {#if activeView === 'spending'}
        <label class="block min-w-36 flex-1 sm:flex-none">
          <span class={labelClass}>{m.reports_group_by()}</span>
          <select bind:value={draft.groupBy} class={controlClass}>
            <option value="category">{m.reports_group_by_category()}</option>
            <option value="payee">{m.reports_group_by_payee()}</option>
          </select>
        </label>
        <label class="block min-w-36 flex-1 sm:flex-none">
          <span class={labelClass}>{m.reports_direction()}</span>
          <select bind:value={draft.direction} class={controlClass}>
            <option value="expense">{m.reports_direction_expense()}</option>
            <option value="income">{m.reports_direction_income()}</option>
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
  </Panel>
  </div>

  {#if activeFilters}
    {#if activeView === 'net-worth'}
      <NetWorthReport filters={activeFilters} {locale} {commodityLabel} {formatRange} />
    {:else if activeView === 'spending'}
      <SpendingReport filters={activeFilters} {locale} {commodityLabel} />
    {:else if activeView === 'cashflow'}
      <CashflowReport filters={activeFilters} {locale} {commodityLabel} {formatRange} />
    {/if}
  {/if}
</div>
