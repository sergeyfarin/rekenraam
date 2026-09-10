<script lang="ts">
  import { createInfiniteQuery } from '@tanstack/svelte-query';
  import { APIClientError } from '$lib/api/client';
  import {
    forecastEventsInfiniteQueryOptions,
    type ForecastBalancesResponse,
    type ForecastEvent,
    type ForecastQuery
  } from '$lib/api/forecast';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import StatusBadge from '$lib/components/status-badge.svelte';
  import { m } from '$lib/paraglide/messages.js';

  let {
    date,
    basisToken,
    filters,
    detailAccountID,
    detailCommodityID,
    accounts,
    disabled = false,
    formatDate,
    formatAmount,
    onBasisChanged,
    onClose
  }: {
    date: string;
    basisToken: string;
    filters: ForecastQuery;
    detailAccountID?: number;
    detailCommodityID?: number;
    accounts: ForecastBalancesResponse['scope']['accounts'];
    disabled?: boolean;
    formatDate: (value: string) => string;
    formatAmount: (value: string, scale: number, commodity: string) => string;
    onBasisChanged: () => void | Promise<void>;
    onClose: () => void;
  } = $props();

  const events = createInfiniteQuery(() => forecastEventsInfiniteQueryOptions({
    ...filters,
    date,
    basisToken,
    detailAccountID,
    detailCommodityID,
    limit: 50
  }));
  const items = $derived(events.data?.pages.flatMap((page) => page.items) ?? []);
  const totalCount = $derived(events.data?.pages[0]?.total_count ?? 0);
  let conflictHandled = $state(false);

  $effect(() => {
    const staleError = events.error instanceof APIClientError && events.error.code === 'FORECAST_BASIS_CHANGED';
    const mismatchedPage = events.data?.pages.some((page) => page.basis_token !== basisToken) ?? false;
    if ((staleError || mismatchedPage) && !conflictHandled) {
      conflictHandled = true;
      void onBasisChanged();
    }
  });

  function accountName(id: number): string {
    const account = accounts.find((row) => row.id === id);
    return account?.name?.trim() || account?.code?.trim() || m.forecast_account_number({ id });
  }

  function sourceLabel(source: ForecastEvent['source']): string {
    if (source === 'posted') return m.forecast_event_source_posted();
    if (source === 'draft') return m.forecast_event_source_draft();
    return m.forecast_event_source_template();
  }

  function sourceTone(source: ForecastEvent['source']): 'positive' | 'warning' | 'accent' {
    if (source === 'posted') return 'positive';
    if (source === 'draft') return 'warning';
    return 'accent';
  }
</script>

<section class="rounded-(--radius-panel) border border-border bg-surface-strong/35 p-4" aria-labelledby={`forecast-events-${date}`}>
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div>
      <h3 id={`forecast-events-${date}`} class="font-semibold text-foreground">{m.forecast_event_title({ date: formatDate(date) })}</h3>
      {#if events.data}<p class="mt-1 text-xs text-muted">{m.forecast_event_count({ shown: items.length, total: totalCount })}</p>{/if}
    </div>
    <button type="button" class="min-h-10 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground" onclick={onClose}>{m.forecast_event_close()}</button>
  </div>

  {#if events.isPending}
    <p class="mt-4 text-sm text-muted" role="status">{m.forecast_event_loading()}</p>
  {:else if events.isError && !(events.error instanceof APIClientError && events.error.code === 'FORECAST_BASIS_CHANGED')}
    <div class="mt-4 space-y-3">
      <APIFormError error={events.error} />
      <button type="button" disabled={disabled} class="min-h-10 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground disabled:opacity-60" onclick={() => events.refetch()}>{m.forecast_event_retry()}</button>
    </div>
  {:else if events.data && items.length === 0}
    <p class="mt-4 text-sm leading-6 text-muted">{m.forecast_event_empty()}</p>
  {:else if items.length > 0}
    <ol class="mt-4 space-y-3">
      {#each items as event (event.key)}
        <li>
          <article class="rounded-(--radius-control) border border-border bg-surface p-3">
            <div class="flex flex-wrap items-start justify-between gap-3">
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <StatusBadge tone={sourceTone(event.source)}>{sourceLabel(event.source)}</StatusBadge>
                  {#if event.carried_forward}<StatusBadge tone="warning">{m.forecast_event_carried()}</StatusBadge>{/if}
                </div>
                <p class="mt-2 break-words font-medium text-foreground">{event.description || '—'}</p>
                {#if event.payee_name}<p class="mt-1 text-sm text-muted">{m.forecast_event_payee({ payee: event.payee_name })}</p>{/if}
              </div>
              <a href={event.source === 'posted' ? '/app/transactions' : '/app/recurring'} class="font-semibold text-accent underline underline-offset-2">
                {event.source === 'posted' ? m.forecast_event_review_transactions() : m.forecast_event_review_recurring()}
              </a>
            </div>

            <dl class="mt-3 grid gap-x-4 gap-y-1 text-sm sm:grid-cols-[max-content_1fr]">
              <dt class="text-muted">{m.forecast_event_original_date()}</dt><dd class="tabular-nums text-foreground">{formatDate(event.source_date)}</dd>
              {#if event.projected_date !== event.source_date}
                <dt class="text-muted">{m.forecast_event_assumed_date()}</dt><dd class="tabular-nums text-foreground">{formatDate(event.projected_date)}</dd>
              {/if}
              {#if event.occurrence_date && event.occurrence_date !== event.source_date}
                <dt class="text-muted">{m.forecast_event_occurrence_date()}</dt><dd class="tabular-nums text-foreground">{formatDate(event.occurrence_date)}</dd>
              {/if}
            </dl>

            <h4 class="mt-3 text-xs font-semibold uppercase tracking-[0.1em] text-muted">{m.forecast_event_amounts()}</h4>
            <ul class="mt-1 space-y-1 text-sm">
              {#each event.amounts as amount (amount.account_id + ':' + amount.commodity_id)}
                <li class="flex flex-wrap justify-between gap-x-4 gap-y-1">
                  <span class="text-muted">{accountName(amount.account_id)}</span>
                  <span class="font-medium tabular-nums text-foreground">{formatAmount(amount.quantity_value, amount.quantity_scale, amount.commodity_code)}</span>
                </li>
              {/each}
            </ul>
          </article>
        </li>
      {/each}
    </ol>
    {#if events.hasNextPage}
      <button type="button" disabled={disabled || events.isFetchingNextPage} class="mt-4 min-h-10 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground disabled:opacity-60" onclick={() => events.fetchNextPage()}>
        {events.isFetchingNextPage ? m.forecast_event_loading_more() : m.forecast_event_load_more()}
      </button>
    {/if}
  {/if}
</section>
