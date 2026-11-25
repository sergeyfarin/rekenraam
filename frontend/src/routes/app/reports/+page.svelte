<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { reportsOverviewQueryOptions, type OverviewReportCard } from '#lib/api/section-overviews.ts';
  import { formatCalendarDate } from '#lib/dates/calendar-date.ts';
  import { useOwnerToday } from '#lib/dates/owner-date-context.ts';
  import Panel from '#lib/components/panel.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { withReportingCurrency } from '#lib/reports/reporting-currency.ts';
  import OverviewAmount from '#lib/section-overviews/overview-amount.svelte';
  import OverviewCard from '#lib/section-overviews/overview-card.svelte';

  const ownerToday = useOwnerToday();
  const query = createQuery(() => reportsOverviewQueryOptions(ownerToday()));
  const overview = $derived(query.data);
  const locale = $derived(getLocale());
  const savedDate = $derived(new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }));
  const gainsHref = '/app/investments/gains';
  const returnsHref = '/app/investments/returns';

  function reportHref(view: 'spending' | 'cashflow' | 'net-worth', start: string, end: string, currency: number | null): string {
    const params = new URLSearchParams({ start_date: start, end_date: end, bucket: 'month' });
    return `/app/reports/${view}?${withReportingCurrency(params, currency)}`;
  }
</script>

{#snippet headline(card: OverviewReportCard, title: string, href: string, period: string, scope?: string)}
  <OverviewCard {title} {href} failed={card.status === 'failed'} retry={() => query.refetch()}>
    {#if card.summary}
      <p>{period}</p>
      {#if scope}<p>{scope}</p>{/if}
      {#if card.summary.amounts.length > 0}
        <ul class="space-y-1 text-lg">
          {#each card.summary.amounts as amount (amount.commodity_code)}
            <li><OverviewAmount {amount} /></li>
          {/each}
        </ul>
      {/if}
      {#if card.summary.state === 'empty'}
        <p>{m.overview_report_empty()}</p>
      {:else if card.summary.state === 'incomplete'}
        <p>{card.summary.amounts.length ? m.overview_report_native() : m.overview_net_worth_incomplete()}</p>
      {:else if card.summary.state === 'no_currency'}
        <p>{m.overview_net_worth_no_currency()}</p>
      {/if}
      {#if card.summary.stale}<p>{m.overview_report_stale()}</p>{/if}
    {/if}
  </OverviewCard>
{/snippet}

{#if query.isPending}
  <p role="status" class="text-sm text-muted">{m.overview_loading()}</p>
{:else if !query.isError && overview}
  {@const { as_of: asOf, month_start: start, reporting_currency_id: currency } = overview}
  {@const date = formatCalendarDate(asOf, locale)}
  {@const period = m.overview_period({ from: formatCalendarDate(start, locale), to: date })}
  <div class="space-y-4" data-testid="reports-overview">
    <div class="grid min-w-0 gap-4 sm:grid-cols-2 xl:grid-cols-3">
      {@render headline(overview.spending, m.reports_view_spending(), reportHref('spending', start, asOf, currency), period, m.overview_spending_scope())}
      {@render headline(overview.cashflow, m.reports_view_cashflow(), reportHref('cashflow', start, asOf, currency), period, m.overview_cashflow_scope())}
      {@render headline(overview.net_worth, m.reports_view_net_worth(), reportHref('net-worth', asOf, asOf, currency), date, m.overview_net_worth_scope())}
      <OverviewCard title={m.nav_in_section({ page: m.investments_tab_gains(), section: m.investments_nav() })} href={gainsHref} />
      <OverviewCard title={m.reports_export_title()} href="/app/reports/export" />
    </div>
    <Panel padding="sm">
      <h2 class="text-lg font-semibold text-foreground">{m.overview_saved_reports()}</h2>
      {#if overview.gains_runs.status === 'failed' || overview.returns_runs.status === 'failed'}
        <p role="alert" class="mt-2 text-sm text-muted">{m.overview_saved_failed()}</p>
        <button type="button" class="min-h-11 text-sm font-medium underline" onclick={() => query.refetch()}>{m.reports_retry()}</button>
      {:else if !overview.gains_runs.summary?.length && !overview.returns_runs.summary?.length}
        <p class="mt-2 text-sm text-muted">{m.overview_saved_empty()}</p>
      {/if}
      <ul class="mt-2 divide-y divide-border">
        {#each overview.gains_runs.summary ?? [] as run (run.id)}
          <li>
            <a href={`${gainsHref}?run_id=${run.id}#saved-gains`} class="flex min-h-11 flex-wrap items-center justify-between gap-x-3 text-sm underline-offset-4 hover:underline">
              <span>{run.kind === 'statement' ? m.valuation_statement_title() : m.investments_tab_gains()}</span>
              <time datetime={run.captured_at} class="text-muted">{savedDate.format(new Date(run.captured_at))}</time>
            </a>
          </li>
        {/each}
        {#each overview.returns_runs.summary ?? [] as run (run.id)}
          <li>
            <a href={`${returnsHref}?run_id=${run.id}#returns-runs-title`} class="flex min-h-11 flex-wrap items-center justify-between gap-x-3 text-sm underline-offset-4 hover:underline">
              <span>{m.nav_returns()}</span>
              <time datetime={run.captured_at} class="text-muted">{savedDate.format(new Date(run.captured_at))}</time>
            </a>
          </li>
        {/each}
      </ul>
      {#if overview.gains_runs.summary?.length || overview.returns_runs.summary?.length}
        <div class="mt-2 flex flex-wrap gap-x-4 text-sm">
          <a class="inline-flex min-h-11 items-center underline" href={`${gainsHref}#saved-gains`}>{m.valuation_runs_title()}</a>
          <a class="inline-flex min-h-11 items-center underline" href={`${returnsHref}#returns-runs-title`}>{m.returns_runs_title()}</a>
        </div>
      {/if}
    </Panel>
  </div>
{:else}
  <p role="alert" class="text-sm text-muted">{m.overview_load_failed()}</p>
  <button type="button" class="min-h-11 text-sm font-medium underline" onclick={() => query.refetch()}>{m.reports_retry()}</button>
{/if}
