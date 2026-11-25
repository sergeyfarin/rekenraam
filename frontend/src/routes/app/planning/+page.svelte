<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { planningOverviewQueryOptions } from '#lib/api/section-overviews.ts';
  import { formatCalendarDate } from '#lib/dates/calendar-date.ts';
  import { useOwnerToday } from '#lib/dates/owner-date-context.ts';
  import { maskFinancialText } from '#lib/money/format.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import OverviewAmount from '#lib/section-overviews/overview-amount.svelte';
  import OverviewCard from '#lib/section-overviews/overview-card.svelte';

  const ownerToday = useOwnerToday();
  const query = createQuery(() => planningOverviewQueryOptions(ownerToday()));
  const overview = $derived(query.data);
  const locale = $derived(getLocale());
  const monthFormatter = $derived(new Intl.DateTimeFormat(locale, { month: 'long', year: 'numeric', timeZone: 'UTC' }));
  const integer = $derived(new Intl.NumberFormat(locale));
  const percent = $derived(new Intl.NumberFormat(locale, { style: 'percent', maximumFractionDigits: 1 }));

  function pace(basisPoints: string | number): string {
    return maskFinancialText(percent.format(`${basisPoints}e-4` as `${number}`));
  }

  function budgetHref(startDate?: string, currencyID?: number): string {
    if (!startDate) return '/app/planning/budgets';
    const params = new URLSearchParams({ month: startDate.slice(0, 7) });
    if (currencyID !== undefined) params.set('currency_id', String(currencyID));
    return `/app/planning/budgets?${params}`;
  }
</script>

{#if query.isPending}
  <p role="status" class="text-sm text-muted">{m.overview_loading()}</p>
{:else if !query.isError && overview}
  {@const budget = overview.budget.summary}
  {@const due = overview.recurring.summary}
  {@const forecast = overview.forecast.summary}
  <div class="grid min-w-0 gap-4 sm:grid-cols-2 xl:grid-cols-3" data-testid="planning-overview">
    <OverviewCard title={m.budgets_title()} href={budgetHref(budget?.start_date, budget?.totals[0]?.commodity_id)}
      failed={overview.budget.status === 'failed'} retry={() => query.refetch()}>
      {#if budget}
        <p>{monthFormatter.format(new Date(`${budget.start_date}T00:00:00Z`))}</p>
        {#if budget.totals.length === 0}
          <p>{m.overview_budget_empty()}</p>
        {:else}
          <ul class="space-y-3">
            {#each budget.totals as row (row.commodity_id)}
              <li>
                <a href={budgetHref(budget.start_date, row.commodity_id)} class="inline-flex min-h-11 flex-wrap items-center gap-x-2 underline-offset-4 hover:underline">
                  <OverviewAmount amount={row.remaining} />
                  <span>{m.overview_budget_left()}</span>
                </a>
                <p>{m.overview_budget_spent({ share: pace(row.spent_basis_points) })}</p>
              </li>
            {/each}
          </ul>
          <p>{m.overview_budget_elapsed({ share: pace(budget.elapsed_basis_points) })}</p>
        {/if}
      {/if}
    </OverviewCard>
    <OverviewCard title={m.recurring_title()} href="/app/planning/recurring" failed={overview.recurring.status === 'failed'} retry={() => query.refetch()}>
      {#if due}
        {#if due.draft_count === 0 && due.blocked_count === 0}
          <p>{m.overview_due_empty()}</p>
        {:else}
          <dl class="space-y-2">
            <div class="flex flex-wrap items-baseline justify-between gap-x-3">
              <dt>{m.overview_due_drafts()}</dt>
              <dd class="font-semibold tabular-nums text-foreground">{maskFinancialText(integer.format(due.draft_count))}</dd>
            </div>
            <div class="flex flex-wrap items-baseline justify-between gap-x-3">
              <dt>{m.recurring_blocked()}</dt>
              <dd class="font-semibold tabular-nums text-foreground">{maskFinancialText(integer.format(due.blocked_count))}</dd>
            </div>
          </dl>
        {/if}
      {/if}
    </OverviewCard>
    <OverviewCard title={m.forecast_nav()} href={forecast ? `/app/planning/forecast?horizon_days=${forecast.horizon_days}` : '/app/planning/forecast'}
      failed={overview.forecast.status === 'failed'} retry={() => query.refetch()}>
      {#if forecast}
        <p>{m.overview_forecast_estimate({ days: integer.format(forecast.horizon_days), date: formatCalendarDate(forecast.through_date, locale) })}</p>
        {#if forecast.minimums.length === 0}
          <p>{m.overview_report_empty()}</p>
        {:else}
          <ul class="space-y-2">
            {#each forecast.minimums as row (row.amount.commodity_code)}
              <li class="flex flex-wrap items-baseline justify-between gap-x-3">
                <span>{formatCalendarDate(row.date, locale)}</span>
                <OverviewAmount amount={row.amount} />
              </li>
            {/each}
          </ul>
        {/if}
        {#if !forecast.complete}<p>{m.overview_forecast_incomplete()}</p>{/if}
      {/if}
    </OverviewCard>
  </div>
{:else}
  <p role="alert" class="text-sm text-muted">{m.overview_load_failed()}</p>
  <button type="button" class="min-h-11 text-sm font-medium underline" onclick={() => query.refetch()}>{m.reports_retry()}</button>
{/if}
