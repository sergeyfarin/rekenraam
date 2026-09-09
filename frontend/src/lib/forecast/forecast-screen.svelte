<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import ChevronUp from '@lucide/svelte/icons/chevron-up';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import { parseISO } from 'date-fns';
  import {
    forecastBalancesQueryOptions,
    forecastEventsQueryKey,
    type ForecastBalancesResponse,
    type ForecastQuery
  } from '$lib/api/forecast';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import Panel from '$lib/components/panel.svelte';
  import StatePanel from '$lib/components/state-panel.svelte';
  import StatusBadge from '$lib/components/status-badge.svelte';
  import { formatQuantity } from '$lib/money/format';
  import { getLocale } from '$lib/paraglide/runtime.js';
  import { m } from '$lib/paraglide/messages.js';
  import ForecastChart from './forecast-chart.svelte';
  import ForecastEvents from './forecast-events.svelte';
  import ForecastLearning from './forecast-learning.svelte';
  import {
    forecastHasMovements,
    forecastLearnedSeriesFor,
    parseForecastFilters,
    writeForecastFilters,
    type ForecastFilters,
    type ForecastLearningPattern,
    type ForecastSeries,
    type ForecastSpendingModel
  } from './forecast-model';

  type Quantity = { quantity_value: string; quantity_scale: number };
  type DisplaySeries = {
    key: string;
    label: string;
    series: ForecastSeries;
    detailAccountID?: number;
    detailCommodityID?: number;
  };

  const locale = $derived(getLocale());
  const dateFormatter = $derived(new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'short', day: 'numeric' }));
  const dateTimeFormatter = $derived(new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }));
  const parsed = $derived(parseForecastFilters($page.url.searchParams));
  const active = $derived(parsed.filters);
  const activeQuery = $derived<ForecastQuery>({
    horizonDays: active.horizonDays,
    accountIDs: active.accountIDs,
    includeDescendants: active.includeDescendants,
    reportingCurrencyID: active.reportingCurrencyID ?? undefined,
    fxMethod: active.reportingCurrencyID === null ? undefined : 'constant_as_of',
    spendingModel: active.spendingModel,
    historyCompleteFrom: active.historyCompleteFrom ?? undefined,
    expenseCategoryIDs: active.expenseCategoryIDs,
    expensePatterns: active.expensePatterns
  });
  const query = createQuery(() => ({
    ...forecastBalancesQueryOptions(activeQuery),
    enabled: parsed.valid,
    refetchOnMount: 'always' as const,
    refetchOnWindowFocus: true
  }));

  let horizonDays = $state(90);
  let accountIDs = $state<number[]>([]);
  let includeDescendants = $state(true);
  let reportingCurrencyID = $state<number | null>(null);
  let spendingModel = $state<ForecastSpendingModel>('off');
  let historyCompleteFrom = $state('');
  let expenseCategoryIDs = $state<number[]>([]);
  let expensePatterns = $state<Record<number, ForecastLearningPattern>>({});
  let syncedURL = $state('');
  let selectedSeriesKey = $state('');
  let openDetailKey = $state('');
  let observedBasis = $state('');
  let detailNotice = $state('');
  const queryClient = useQueryClient();
  const data = $derived(query.data);

  $effect(() => {
    const signature = $page.url.search;
    if (signature === syncedURL) return;
    horizonDays = active.horizonDays;
    accountIDs = [...active.accountIDs];
    includeDescendants = active.includeDescendants;
    reportingCurrencyID = active.reportingCurrencyID;
    spendingModel = active.spendingModel;
    historyCompleteFrom = active.historyCompleteFrom ?? '';
    expenseCategoryIDs = [...active.expenseCategoryIDs];
    expensePatterns = { ...active.expensePatterns };
    openDetailKey = '';
    detailNotice = '';
    syncedURL = signature;
  });

  $effect(() => {
    const nextBasis = data?.basis_token;
    if (!nextBasis || nextBasis === observedBasis) return;
    if (observedBasis && openDetailKey) {
      openDetailKey = '';
      detailNotice = m.forecast_event_basis_changed();
      queryClient.removeQueries({ queryKey: forecastEventsQueryKey });
    }
    observedBasis = nextBasis;
  });

  const historyValid = $derived(
    spendingModel === 'off' || (/^\d{4}-\d{2}-\d{2}$/.test(historyCompleteFrom) && !Number.isNaN(Date.parse(historyCompleteFrom)))
  );
  const formValid = $derived(Number.isInteger(horizonDays) && horizonDays >= 1 && horizonDays <= 366 && historyValid);
  const learned = $derived(data?.learned_spending ?? null);
  const learningOptions = $derived(learned?.category_options ?? []);
  const hasMovements = $derived(data ? forecastHasMovements(data.totals) : false);
  const displaySeries = $derived.by<DisplaySeries[]>(() => {
    if (!data) return [];
    const rows: DisplaySeries[] = data.totals.map((series) => ({
      key: `total:${series.commodity_id}`,
      label: m.forecast_series_total({ commodity: series.commodity_code }),
      series,
      detailCommodityID: series.commodity_id
    }));
    if (data.converted) rows.unshift({ key: 'converted', label: m.forecast_series_converted({ commodity: data.converted.commodity_code }), series: data.converted });
    for (const series of data.series) {
      const account = data.scope.accounts.find((row) => row.id === series.account_id);
      rows.push({ key: `account:${series.account_id}:${series.commodity_id}`, label: m.forecast_series_account({ account: accountName(account), commodity: series.commodity_code }), series, detailAccountID: series.account_id, detailCommodityID: series.commodity_id });
    }
    return rows;
  });
  const selectedDisplay = $derived(displaySeries.find((row) => row.key === selectedSeriesKey) ?? displaySeries[0]);
  const selectedLearnedSeries = $derived(
    selectedDisplay
      ? forecastLearnedSeriesFor(learned, selectedDisplay.detailAccountID, selectedDisplay.series.commodity_id)
      : undefined
  );

  function accountName(account: ForecastBalancesResponse['scope']['account_options'][number] | undefined): string {
    if (!account) return m.forecast_unknown_account();
    return account.name?.trim() || account.code?.trim() || m.forecast_account_number({ id: account.id });
  }

  function formatDate(value: string): string {
    return dateFormatter.format(parseISO(value));
  }

  function formatComputedAt(value: string): string {
    return dateTimeFormatter.format(new Date(value));
  }

  function formatAmount(quantity: Quantity, commodity: string): string {
    return `${commodity} ${formatQuantity(quantity.quantity_value, quantity.quantity_scale, locale)}`;
  }

  function formatSelectedAmount(value: string, scale: number): string {
    return formatAmount({ quantity_value: value, quantity_scale: scale }, selectedDisplay?.series.commodity_code ?? '');
  }

  function formatEventAmount(value: string, scale: number, commodity: string): string {
    return formatAmount({ quantity_value: value, quantity_scale: scale }, commodity);
  }

  function selectSeries(key: string) {
    selectedSeriesKey = key;
    openDetailKey = '';
    detailNotice = '';
  }

  function endPoint(series: ForecastSeries) {
    return series.points.at(-1);
  }

  function setAccount(id: number, selected: boolean) {
    const next = new Set(accountIDs);
    if (selected) next.add(id); else next.delete(id);
    accountIDs = [...next].sort((a, b) => a - b);
  }

  function applyFilters() {
    if (!formValid) return;
    openDetailKey = '';
    queryClient.removeQueries({ queryKey: forecastEventsQueryKey });
    const filters: ForecastFilters = {
      horizonDays, accountIDs, includeDescendants, reportingCurrencyID,
      spendingModel, historyCompleteFrom: historyCompleteFrom || null, expenseCategoryIDs, expensePatterns
    };
    const params = writeForecastFilters(filters);
    void goto(`/app/forecast?${params.toString()}`, { keepFocus: true, noScroll: true });
  }

  function resetFilters() {
    openDetailKey = '';
    queryClient.removeQueries({ queryKey: forecastEventsQueryKey });
    void goto('/app/forecast', { keepFocus: true, noScroll: true });
  }

  function toggleDetails(date: string) {
    if (!selectedDisplay || query.isFetching) return;
    const key = `${selectedDisplay.key}:${date}`;
    openDetailKey = openDetailKey === key ? '' : key;
    detailNotice = '';
  }

  async function recoverChangedBasis() {
    openDetailKey = '';
    queryClient.removeQueries({ queryKey: forecastEventsQueryKey });
    detailNotice = m.forecast_event_basis_changed();
    await query.refetch();
  }

  function refreshForecast() {
    void query.refetch();
  }

  function setSpendingModel(value: boolean) {
    spendingModel = value ? 'adaptive_v1' : 'off';
    if (!value) {
      // Turning estimates off drops every model option, so the request goes
      // back to being exactly the core forecast.
      historyCompleteFrom = '';
      expenseCategoryIDs = [];
      expensePatterns = {};
    }
  }

  function setExpenseCategory(id: number, selected: boolean) {
    const next = new Set(expenseCategoryIDs);
    if (selected) next.add(id); else next.delete(id);
    expenseCategoryIDs = [...next].sort((a, b) => a - b);
    if (!selected) {
      const { [id]: _removed, ...rest } = expensePatterns;
      expensePatterns = rest;
    }
  }

  function setExpensePattern(id: number, pattern: ForecastLearningPattern) {
    expensePatterns = { ...expensePatterns, [id]: pattern };
  }

  function diagnosticLabel(code: ForecastBalancesResponse['diagnostics'][number]['code']): string {
    switch (code) {
      case 'carried_forward': return m.forecast_diagnostic_carried();
      case 'blocked_occurrence': return m.forecast_diagnostic_blocked();
      case 'invalid_draft': return m.forecast_diagnostic_invalid_draft();
      case 'invalid_template_occurrence': return m.forecast_diagnostic_invalid_template();
      case 'broken_occurrence_link': return m.forecast_diagnostic_broken_link();
    }
  }

  function accountStatusLabel(status: 'active' | 'closed' | 'archived'): string {
    if (status === 'closed') return m.account_status_closed();
    if (status === 'archived') return m.account_status_archived();
    return m.account_status_active();
  }

  function usePreset(days: number) {
    horizonDays = days;
  }
</script>

{#if !parsed.valid}
  <StatePanel title={m.forecast_invalid_filters_title()} copy={m.forecast_invalid_filters_copy()}>
    <button type="button" class="rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background" onclick={resetFilters}>{m.forecast_reset()}</button>
  </StatePanel>
{:else if query.isPending}
  <StatePanel title={m.forecast_loading_title()} copy={m.forecast_loading_copy()} />
{:else if query.isError}
  <StatePanel title={m.forecast_error_title()} copy={m.forecast_error_copy()}>
    <APIFormError error={query.error} id="forecast-error" />
    <div class="flex flex-wrap gap-2">
      <button type="button" class="rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background" onclick={refreshForecast}>{m.forecast_retry()}</button>
      <button type="button" class="rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground" onclick={resetFilters}>{m.forecast_reset()}</button>
    </div>
  </StatePanel>
{:else if data}
  <div class="space-y-5">
    <Panel variant="toolbar">
      <form onsubmit={(event) => { event.preventDefault(); applyFilters(); }} aria-label={m.forecast_filters()}>
        <div class="grid gap-5 xl:grid-cols-[minmax(12rem,0.7fr)_minmax(16rem,1.3fr)_minmax(12rem,0.7fr)]">
          <fieldset>
            <legend class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.forecast_horizon()}</legend>
            <div class="mt-2 flex flex-wrap gap-2">
              {#each [30, 90, 180, 365] as days}
                <button type="button" class="rounded-(--radius-control) border border-border px-3 py-2 text-sm font-semibold" class:bg-selected={horizonDays === days} class:text-selected-foreground={horizonDays === days} class:bg-control={horizonDays !== days} onclick={() => usePreset(days)}>{m.forecast_days({ count: days })}</button>
              {/each}
            </div>
            <label class="mt-3 block text-sm text-foreground">
              <span>{m.forecast_custom_horizon()}</span>
              <input type="number" min="1" max="366" step="1" bind:value={horizonDays} aria-invalid={!formValid} class="mt-1.5 h-10 w-full rounded-(--radius-control) border border-border bg-control px-3 text-foreground outline-none focus:border-accent" />
            </label>
            {#if !formValid}<p class="mt-1 text-xs text-danger">{m.forecast_horizon_error()}</p>{/if}
          </fieldset>

          <fieldset>
            <legend class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.forecast_accounts()}</legend>
            <p class="mt-1 text-xs leading-5 text-muted">{accountIDs.length === 0 ? m.forecast_accounts_default() : m.forecast_accounts_selected({ count: accountIDs.length })}</p>
            <div class="mt-2 max-h-40 space-y-2 overflow-y-auto rounded-(--radius-control) border border-border bg-control p-3">
              {#each data.scope.account_options as account (account.id)}
                <label class="flex items-start gap-2 text-sm text-foreground">
                  <input type="checkbox" class="mt-0.5 size-4 accent-[var(--color-accent)]" checked={accountIDs.includes(account.id)} onchange={(event) => setAccount(account.id, event.currentTarget.checked)} />
                  <span>{accountName(account)}{#if account.status !== 'active'} <span class="text-muted">({accountStatusLabel(account.status)})</span>{/if}</span>
                </label>
              {:else}
                <p class="text-sm text-muted">{m.forecast_accounts_empty()}</p>
              {/each}
            </div>
            {#if accountIDs.length > 0}
              <label class="mt-3 flex items-center gap-2 text-sm text-foreground"><input type="checkbox" class="size-4 accent-[var(--color-accent)]" bind:checked={includeDescendants} />{m.forecast_include_descendants()}</label>
            {/if}
          </fieldset>

          <label class="block text-sm text-foreground">
            <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.forecast_reporting_currency()}</span>
            <select bind:value={reportingCurrencyID} class="mt-2 h-10 w-full rounded-(--radius-control) border border-border bg-control px-3 text-foreground outline-none focus:border-accent">
              <option value={null}>{m.forecast_reporting_none()}</option>
              {#each data.currency_options as currency (currency.id)}
                <option value={currency.id}>{currency.code}</option>
              {/each}
            </select>
            <span class="mt-2 block text-xs leading-5 text-muted">{m.forecast_reporting_help()}</span>
          </label>
        </div>
        <div class="mt-5 flex flex-wrap items-center gap-2">
          <button type="submit" disabled={!formValid} class="rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background disabled:opacity-50">{m.forecast_apply()}</button>
          <button type="button" class="rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground" onclick={resetFilters}>{m.forecast_reset()}</button>
          <button type="button" class="ml-auto inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground disabled:opacity-60" disabled={query.isFetching} onclick={refreshForecast}><RefreshCw size={16} aria-hidden="true" />{query.isFetching ? m.forecast_updating() : m.forecast_refresh()}</button>
        </div>
      </form>
    </Panel>

    <div class="flex flex-wrap items-center gap-2 text-sm text-muted" aria-live="polite">
      <StatusBadge tone={data.assumptions.complete ? 'positive' : 'warning'}>{data.assumptions.complete ? m.forecast_assumptions_complete() : m.forecast_assumptions_incomplete()}</StatusBadge>
      <span>{m.forecast_basis_metadata({ asOf: formatDate(data.as_of_date), computedAt: formatComputedAt(data.computed_at), zone: data.time_zone })}</span>
    </div>

    {#if detailNotice}
      <p role="status" class="rounded-(--radius-control) border border-warning/40 bg-warning-soft px-4 py-3 text-sm text-warning">{detailNotice}</p>
    {/if}

    {#if data.scope.resolved_account_ids.length === 0}
      <StatePanel title={m.forecast_no_accounts_title()} copy={m.forecast_no_accounts_copy()}>
        <div class="flex flex-wrap gap-3"><a href="/app/accounts" class="font-semibold text-accent underline underline-offset-2">{m.forecast_go_accounts()}</a><button type="button" class="font-semibold text-accent underline underline-offset-2" onclick={resetFilters}>{m.forecast_reset()}</button></div>
      </StatePanel>
    {:else}
      {#if data.assumptions.carried_forward_event_count > 0 || data.assumptions.excluded_event_count > 0}
        <Panel variant="subtle">
          <h2 class="font-semibold text-foreground">{m.forecast_assumptions_title()}</h2>
          <p class="mt-2 text-sm leading-6 text-muted">{m.forecast_assumptions_notice({ carried: data.assumptions.carried_forward_event_count, excluded: data.assumptions.excluded_event_count })}</p>
          {#if data.diagnostics.length > 0}
            <ul class="mt-3 list-disc space-y-1 pl-5 text-sm text-muted">
              {#each data.diagnostics as diagnostic}
                <li>{diagnosticLabel(diagnostic.code)} · {m.forecast_events_count({ count: diagnostic.event_count })}</li>
              {/each}
            </ul>
          {/if}
          <a href="/app/recurring" class="mt-3 inline-block font-semibold text-accent underline underline-offset-2">{m.forecast_review_recurring()}</a>
        </Panel>
      {/if}

      {#if data.valuation}
        <Panel variant="subtle">
          <h2 class="font-semibold text-foreground">{m.forecast_fx_title({ currency: data.valuation.reporting_currency_code })}</h2>
          {#if data.valuation.complete}
            <p class="mt-2 text-sm leading-6 text-muted">{m.forecast_fx_complete({ date: formatDate(data.valuation.as_of_date), count: data.valuation.used_rates.length })}</p>
            {#if data.valuation.used_rates.length > 0}
              <ul class="mt-3 space-y-2 text-sm text-muted">
                {#each data.valuation.used_rates as rate (rate.observation_id)}
                  <li class="flex flex-wrap items-center gap-2">
                    <span>{m.forecast_fx_rate({
                      source: data.currency_options.find((row) => row.id === rate.base_commodity_id)?.code ?? String(rate.base_commodity_id),
                      quote: data.valuation.reporting_currency_code,
                      price: formatQuantity(rate.price_value, rate.price_scale, locale),
                      base: formatQuantity(rate.base_quantity_value, rate.base_quantity_scale, locale),
                      date: formatDate(rate.valuation_date)
                    })}</span>
                    {#if rate.stale}<StatusBadge tone="warning">{m.forecast_fx_stale()}</StatusBadge>{/if}
                    {#if rate.is_derived}<StatusBadge tone="warning">{m.forecast_fx_derived()}</StatusBadge>{/if}
                  </li>
                {/each}
              </ul>
            {/if}
          {:else}
            <p class="mt-2 text-sm leading-6 text-warning">{m.forecast_fx_incomplete()}</p>
            <ul class="mt-2 list-disc pl-5 text-sm text-muted">
              {#each data.valuation.gaps as gap}<li>{m.forecast_fx_gap({ commodity: data.currency_options.find((row) => row.id === gap.commodity_id)?.code ?? String(gap.commodity_id), date: gap.nearest_observation_date ? formatDate(gap.nearest_observation_date) : m.forecast_fx_no_rate() })}</li>{/each}
            </ul>
          {/if}
          <p class="mt-2 text-xs leading-5 text-muted">{m.forecast_fx_policy()}</p>
        </Panel>
      {/if}

      {#if !hasMovements}
        <StatePanel title={m.forecast_no_movement_title()} copy={m.forecast_no_movement_copy()} />
      {/if}

      <section aria-labelledby="forecast-summary-title">
        <h2 id="forecast-summary-title" class="text-lg font-semibold text-foreground">{m.forecast_summary_title()}</h2>
        <div class="mt-3 grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {#each data.totals as series (series.commodity_id)}
            {@const final = endPoint(series)}
            <Panel padding="sm" variant="subtle">
              <h3 class="font-semibold text-foreground">{m.forecast_series_total({ commodity: series.commodity_code })}</h3>
              <dl class="mt-3 grid grid-cols-2 gap-x-3 gap-y-2 text-sm">
                <dt class="text-muted">{m.forecast_opening()}</dt><dd class="text-right font-medium tabular-nums">{formatAmount(series.opening_balance, series.commodity_code)}</dd>
                <dt class="text-muted">{m.forecast_end_recorded()}</dt><dd class="text-right font-medium tabular-nums">{final ? formatAmount(final.recorded_balance, series.commodity_code) : '—'}</dd>
                <dt class="text-muted">{m.forecast_end_projected()}</dt><dd class="text-right font-medium tabular-nums">{final ? formatAmount(final.projected_balance, series.commodity_code) : '—'}</dd>
                <dt class="text-muted">{m.forecast_minimum()}</dt><dd class="text-right font-medium tabular-nums">{formatAmount(series.minimum_balance, series.commodity_code)} · {formatDate(series.minimum_date)}</dd>
              </dl>
            </Panel>
          {/each}
          {#if data.converted}
            {@const final = endPoint(data.converted)}
            <Panel padding="sm" variant="subtle">
              <h3 class="font-semibold text-foreground">{m.forecast_series_converted({ commodity: data.converted.commodity_code })}</h3>
              <dl class="mt-3 grid grid-cols-2 gap-x-3 gap-y-2 text-sm">
                <dt class="text-muted">{m.forecast_opening()}</dt><dd class="text-right font-medium tabular-nums">{formatAmount(data.converted.opening_balance, data.converted.commodity_code)}</dd>
                <dt class="text-muted">{m.forecast_end_recorded()}</dt><dd class="text-right font-medium tabular-nums">{final ? formatAmount(final.recorded_balance, data.converted.commodity_code) : '—'}</dd>
                <dt class="text-muted">{m.forecast_end_projected()}</dt><dd class="text-right font-medium tabular-nums">{final ? formatAmount(final.projected_balance, data.converted.commodity_code) : '—'}</dd>
                <dt class="text-muted">{m.forecast_minimum()}</dt><dd class="text-right font-medium tabular-nums">{formatAmount(data.converted.minimum_balance, data.converted.commodity_code)} · {formatDate(data.converted.minimum_date)}</dd>
              </dl>
            </Panel>
          {/if}
          {#each data.series as series (series.account_id + ':' + series.commodity_id)}
            {@const final = endPoint(series)}
            <Panel padding="sm">
              <h3 class="font-semibold text-foreground">{accountName(data.scope.accounts.find((account) => account.id === series.account_id))} · {series.commodity_code}</h3>
              <dl class="mt-3 grid grid-cols-2 gap-x-3 gap-y-2 text-sm">
                <dt class="text-muted">{m.forecast_opening()}</dt><dd class="text-right font-medium tabular-nums">{formatAmount(series.opening_balance, series.commodity_code)}</dd>
                <dt class="text-muted">{m.forecast_end_recorded()}</dt><dd class="text-right font-medium tabular-nums">{final ? formatAmount(final.recorded_balance, series.commodity_code) : '—'}</dd>
                <dt class="text-muted">{m.forecast_end_projected()}</dt><dd class="text-right font-medium tabular-nums">{final ? formatAmount(final.projected_balance, series.commodity_code) : '—'}</dd>
                <dt class="text-muted">{m.forecast_minimum()}</dt><dd class="text-right font-medium tabular-nums">{formatAmount(series.minimum_balance, series.commodity_code)} · {formatDate(series.minimum_date)}</dd>
              </dl>
              {#if series.first_negative_date}<p class="mt-3 text-xs font-semibold text-danger">{m.forecast_first_negative({ date: formatDate(series.first_negative_date) })}</p>{/if}
              {#if data.scope.accounts.find((account) => account.id === series.account_id)?.account_class === 'liability'}<p class="mt-3 text-xs text-muted">{m.forecast_liability_note()}</p>{/if}
            </Panel>
          {/each}
        </div>
      </section>

      {#if selectedDisplay}
        <Panel>
          <div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
            <div><h2 class="text-lg font-semibold text-foreground">{m.forecast_daily_title()}</h2><p class="mt-1 text-sm text-muted">{m.forecast_daily_copy()}</p></div>
            <label class="text-sm text-foreground"><span class="block text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.forecast_series()}</span><select value={selectedDisplay.key} onchange={(event) => selectSeries(event.currentTarget.value)} class="mt-1.5 h-10 max-w-full rounded-(--radius-control) border border-border bg-control px-3 text-foreground">{#each displaySeries as row (row.key)}<option value={row.key}>{row.label}</option>{/each}</select></label>
          </div>
          <ForecastChart series={selectedDisplay.series} estimated={selectedLearnedSeries} {formatDate} formatAmount={formatSelectedAmount} />
          <!-- svelte-ignore a11y_no_noninteractive_tabindex (keyboard users must be able to scroll the bounded table region) -->
          <div class="mt-5 overflow-x-auto" role="region" tabindex="0" aria-label={m.forecast_table_scroll_label()}>
            <table class="w-full min-w-[60rem] border-collapse text-left text-sm">
              <caption class="sr-only">{m.forecast_table_caption({ series: selectedDisplay.label })}</caption>
              <thead><tr class="border-b border-border text-xs uppercase tracking-[0.08em] text-muted"><th class="px-3 py-2">{m.forecast_date()}</th><th class="px-3 py-2 text-right">{m.forecast_posted_delta()}</th><th class="px-3 py-2 text-right">{m.forecast_draft_delta()}</th><th class="px-3 py-2 text-right">{m.forecast_template_delta()}</th><th class="px-3 py-2 text-right">{m.forecast_recorded_only()}</th><th class="px-3 py-2 text-right">{m.forecast_with_recurring()}</th>{#if selectedLearnedSeries}<th class="px-3 py-2 text-right">{m.forecast_with_estimates()}</th>{/if}</tr></thead>
              <tbody>
                {#each selectedDisplay.series.points as point (point.date)}
                  {@const detailKey = `${selectedDisplay.key}:${point.date}`}
                  {@const detailOpen = openDetailKey === detailKey}
                  <tr class="border-b border-border/70">
                    <th scope="row" class="whitespace-nowrap px-3 py-2 font-medium">
                      <button
                        type="button"
                        class="inline-flex min-h-10 items-center gap-2 rounded-(--radius-control) px-2 text-left font-semibold text-accent hover:bg-control disabled:text-muted"
                        aria-expanded={detailOpen}
                        aria-controls={`forecast-detail-${point.date}`}
                        disabled={query.isFetching}
                        onclick={() => toggleDetails(point.date)}
                      >
                        {#if detailOpen}<ChevronUp size={16} aria-hidden="true" />{:else}<ChevronDown size={16} aria-hidden="true" />{/if}
                        {formatDate(point.date)}
                      </button>
                    </th>
                    <td class="px-3 py-2 text-right tabular-nums">{formatSelectedAmount(point.posted_delta.quantity_value, point.posted_delta.quantity_scale)}</td>
                    <td class="px-3 py-2 text-right tabular-nums">{formatSelectedAmount(point.draft_delta.quantity_value, point.draft_delta.quantity_scale)}</td>
                    <td class="px-3 py-2 text-right tabular-nums">{formatSelectedAmount(point.template_delta.quantity_value, point.template_delta.quantity_scale)}</td>
                    <td class="px-3 py-2 text-right tabular-nums">{formatSelectedAmount(point.recorded_balance.quantity_value, point.recorded_balance.quantity_scale)}</td>
                    <td class="px-3 py-2 text-right font-semibold tabular-nums">{formatSelectedAmount(point.projected_balance.quantity_value, point.projected_balance.quantity_scale)}</td>
                    {#if selectedLearnedSeries}
                      {@const estimate = selectedLearnedSeries.points.find((row) => row.date === point.date)}
                      <td class="px-3 py-2 text-right tabular-nums text-muted">
                        {estimate ? formatSelectedAmount(estimate.projected_balance.quantity_value, estimate.projected_balance.quantity_scale) : '—'}
                      </td>
                    {/if}
                  </tr>
                  {#if detailOpen}
                    <tr id={`forecast-detail-${point.date}`} class="border-b border-border">
                      <td colspan="6" class="p-3">
                        <ForecastEvents
                          date={point.date}
                          basisToken={data.basis_token}
                          filters={activeQuery}
                          detailAccountID={selectedDisplay.detailAccountID}
                          detailCommodityID={selectedDisplay.detailCommodityID}
                          accounts={data.scope.accounts}
                          disabled={query.isFetching}
                          {formatDate}
                          formatAmount={formatEventAmount}
                          onBasisChanged={recoverChangedBasis}
                          onClose={() => (openDetailKey = '')}
                        />
                      </td>
                    </tr>
                  {/if}
                {/each}
              </tbody>
            </table>
          </div>
          <ForecastLearning
            {learned}
            enabled={spendingModel === 'adaptive_v1'}
            {historyCompleteFrom}
            historyError={!historyValid}
            {expenseCategoryIDs}
            {expensePatterns}
            options={learningOptions}
            formatAmount={formatSelectedAmount}
            {formatDate}
            onToggle={setSpendingModel}
            onHistoryChange={(value) => (historyCompleteFrom = value)}
            onCategoryToggle={setExpenseCategory}
            onPatternChange={setExpensePattern}
          />
        </Panel>
      {/if}
    {/if}
  </div>
{/if}
