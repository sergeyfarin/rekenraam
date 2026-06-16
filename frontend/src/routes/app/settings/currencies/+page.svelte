<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import AlertTriangle from '@lucide/svelte/icons/alert-triangle';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import Save from '@lucide/svelte/icons/save';
  import { parseISO } from 'date-fns';
  import { authSessionQueryOptions } from '$lib/api/auth';
  import { currentBookQueryOptions } from '$lib/api/books';
  import { currenciesQueryOptions, type CurrencyResponse } from '$lib/api/currencies';
  import { APIClientError } from '$lib/api/client';
  import {
    createPricingSourceAssignment,
    getLatestPriceObservation,
    pricingPolicyQueryOptions,
    pricingRefreshRunsQueryOptions,
    pricingSourceAssignmentsQueryOptions,
    pricingSourceHealthQueryOptions,
    pricingSourcesQueryOptions,
    runPricingRefresh,
    savePricingPolicy,
    updatePricingSourceAssignment,
    type MarketDataSourceResponse,
    type PriceObservationResponse,
    type PricingSourceAssignmentResponse
  } from '$lib/api/pricing';
  import { getAPIClientErrorMessage } from '$lib/api-error-messages';
  import Panel from '$lib/components/panel.svelte';
  import StatePanel from '$lib/components/state-panel.svelte';
  import StatusBadge from '$lib/components/status-badge.svelte';
  import { m } from '$lib/paraglide/messages.js';

  type RateDirection = 'currency_default' | 'default_currency' | 'both';
  type RatePair = {
    base: CurrencyResponse;
    quote: CurrencyResponse;
  };

  const currenciesQuery = createQuery(() => currenciesQueryOptions());
  const currentBookQuery = createQuery(() => currentBookQueryOptions());
  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const sourcesQuery = createQuery(() => pricingSourcesQueryOptions());
  const policyQuery = createQuery(() => pricingPolicyQueryOptions());
  const assignmentsQuery = createQuery(() => pricingSourceAssignmentsQueryOptions());
  const refreshRunsQuery = createQuery(() => pricingRefreshRunsQueryOptions());
  const sourceHealthQuery = createQuery(() => pricingSourceHealthQueryOptions());

  let rateDirection = $state<RateDirection>('currency_default');
  let policyBaseCommodityID = $state('');
  let policyDefaultSourceID = $state('');
  let refreshEnabled = $state(false);
  let refreshHourUTC = $state(4);
  let refreshMinuteUTC = $state(0);
  let triangulationMaxHops = $state(1);
  let initializedPolicyMarker = $state('');
  let policySaving = $state(false);
  let refreshRunning = $state(false);
  let assignmentSavingKey = $state('');
  let actionError = $state<unknown>(undefined);
  let actionMessage = $state('');

  const currencyDisplayNames = new Intl.DisplayNames(undefined, { type: 'currency' });
  const numberFormatter = new Intl.NumberFormat(undefined, {
    maximumFractionDigits: 8,
    minimumFractionDigits: 0
  });
  const dateFormatter = new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric'
  });
  const dateTimeFormatter = new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    timeZoneName: 'short'
  });

  const activeCurrencies = $derived.by(() =>
    [...(currenciesQuery.data?.currencies.filter((currency) => currency.status === 'active') ?? [])].sort((left, right) =>
      left.code.localeCompare(right.code)
    )
  );

  const currencyByID = $derived.by(() => new Map(activeCurrencies.map((currency) => [currency.id, currency])));

  const defaultCurrency = $derived.by(() => {
    const defaultCurrencyID = currentBookQuery.data?.default_currency_commodity_id;
    return activeCurrencies.find((currency) => currency.id === defaultCurrencyID);
  });

  const providerSources = $derived.by(() =>
    (sourcesQuery.data?.sources ?? []).filter((source) => source.kind === 'provider' && source.status === 'active')
  );

  const providerByID = $derived.by(() => new Map(providerSources.map((source) => [source.id, source])));

  const ratePairs = $derived.by<RatePair[]>(() => {
    if (!defaultCurrency) {
      return [];
    }

    return activeCurrencies
      .filter((currency) => currency.id !== defaultCurrency.id)
      .flatMap((currency) => [
        { base: currency, quote: defaultCurrency },
        { base: defaultCurrency, quote: currency }
      ]);
  });

  const ratesQueryEnabled = $derived(ratePairs.length > 0);

  const latestRatesQuery = createQuery(() => ({
    queryKey: [
      'api',
      'pricing',
      'latest-currency-rates',
      defaultCurrency?.id ?? null,
      ratePairs.map((pair) => rateKey(pair.base.id, pair.quote.id)).join(',')
    ],
    enabled: ratesQueryEnabled,
    queryFn: async () => {
      const entries = await Promise.all(
        ratePairs.map(async (pair) => [
          rateKey(pair.base.id, pair.quote.id),
          await getLatestPriceObservation(pair.base.id, pair.quote.id).catch(() => null)
        ] as const)
      );

      return new Map(entries);
    },
    staleTime: 30_000
  }));

  const pageError = $derived(
    currenciesQuery.error ??
      currentBookQuery.error ??
      sessionQuery.error ??
      sourcesQuery.error ??
      policyQuery.error ??
      assignmentsQuery.error ??
      refreshRunsQuery.error ??
      sourceHealthQuery.error ??
      (ratesQueryEnabled ? latestRatesQuery.error : null)
  );
  const isLoading = $derived(
    currenciesQuery.isPending ||
      currentBookQuery.isPending ||
      sessionQuery.isPending ||
      sourcesQuery.isPending ||
      policyQuery.isPending ||
      assignmentsQuery.isPending ||
      refreshRunsQuery.isPending ||
      sourceHealthQuery.isPending ||
      (ratesQueryEnabled && latestRatesQuery.isPending)
  );
  const isError = $derived(
    currenciesQuery.isError ||
      currentBookQuery.isError ||
      sessionQuery.isError ||
      sourcesQuery.isError ||
      policyQuery.isError ||
      assignmentsQuery.isError ||
      refreshRunsQuery.isError ||
      sourceHealthQuery.isError ||
      (ratesQueryEnabled && latestRatesQuery.isError)
  );

  const latestRun = $derived(refreshRunsQuery.data?.runs[0]);

  const directionOptions: { value: RateDirection; label: string }[] = [
    { value: 'currency_default', label: m.currencies_rate_direction_currency_default() },
    { value: 'default_currency', label: m.currencies_rate_direction_default_currency() },
    { value: 'both', label: m.currencies_rate_direction_both() }
  ];

  $effect(() => {
    const marker = [
      policyQuery.data?.id ?? 'missing',
      policyQuery.data?.updated_at ?? '',
      defaultCurrency?.id ?? '',
      providerSources.map((source) => source.id).join(',')
    ].join(':');

    if (initializedPolicyMarker === marker || policySaving) {
      return;
    }

    initializedPolicyMarker = marker;
    policyBaseCommodityID = policyQuery.data?.base_commodity_id
      ? String(policyQuery.data.base_commodity_id)
      : defaultCurrency
        ? String(defaultCurrency.id)
        : '';
    policyDefaultSourceID = policyQuery.data?.default_source_id
      ? String(policyQuery.data.default_source_id)
      : providerSources[0]
        ? String(providerSources[0].id)
        : '';
    refreshEnabled = policyQuery.data?.refresh_enabled ?? false;
    refreshHourUTC = policyQuery.data?.refresh_hour_utc ?? 4;
    refreshMinuteUTC = policyQuery.data?.refresh_minute_utc ?? 0;
    triangulationMaxHops = policyQuery.data?.triangulation_max_hops ?? 1;
  });

  async function retry() {
    await Promise.all([
      currenciesQuery.refetch(),
      currentBookQuery.refetch(),
      sessionQuery.refetch(),
      sourcesQuery.refetch(),
      policyQuery.refetch(),
      assignmentsQuery.refetch(),
      refreshRunsQuery.refetch(),
      sourceHealthQuery.refetch(),
      ratesQueryEnabled ? latestRatesQuery.refetch() : Promise.resolve()
    ]);
  }

  async function refreshAfterMutation() {
    await Promise.all([
      policyQuery.refetch(),
      assignmentsQuery.refetch(),
      refreshRunsQuery.refetch(),
      sourceHealthQuery.refetch(),
      ratesQueryEnabled ? latestRatesQuery.refetch() : Promise.resolve()
    ]);
  }

  async function csrfToken(): Promise<string> {
    if (sessionQuery.data?.csrf_token) {
      return sessionQuery.data.csrf_token;
    }

    const refreshedSession = await sessionQuery.refetch();
    const token = refreshedSession.data?.csrf_token;
    if (!token) {
      throw new APIClientError({ status: 403, code: 'CSRF_INVALID' });
    }
    return token;
  }

  async function handleSavePolicy() {
    policySaving = true;
    actionError = undefined;
    actionMessage = '';

    try {
      await savePricingPolicy(
        {
          base_commodity_id: numberOrUndefined(policyBaseCommodityID),
          default_source_id: numberOrUndefined(policyDefaultSourceID),
          refresh_enabled: refreshEnabled,
          refresh_hour_utc: refreshHourUTC,
          refresh_minute_utc: refreshMinuteUTC,
          max_backfill_days: policyQuery.data?.max_backfill_days ?? 370,
          staleness_max_days: policyQuery.data?.staleness_max_days ?? 3,
          triangulation_max_hops: triangulationMaxHops,
          rounding_mode: policyQuery.data?.rounding_mode ?? 'half_up',
          prefer_official_fx: policyQuery.data?.prefer_official_fx ?? true,
          weekend_policy: policyQuery.data?.weekend_policy ?? 'skip',
          change_reason: 'saved currency exchange rate settings'
        },
        await csrfToken()
      );
      actionMessage = m.currencies_policy_saved();
      await refreshAfterMutation();
    } catch (error) {
      actionError = error;
    } finally {
      policySaving = false;
    }
  }

  async function handleRunRefresh() {
    refreshRunning = true;
    actionError = undefined;
    actionMessage = '';

    try {
      const run = await runPricingRefresh({ trigger: 'manual' }, await csrfToken());
      actionMessage = m.currencies_refresh_finished({ status: run.status, count: run.items_total });
      await refreshAfterMutation();
    } catch (error) {
      actionError = error;
    } finally {
      refreshRunning = false;
    }
  }

  async function handleAssignmentChange(pair: RatePair, selectedSourceID: string) {
    const key = rateKey(pair.base.id, pair.quote.id);
    assignmentSavingKey = key;
    actionError = undefined;
    actionMessage = '';

    try {
      const existing = assignmentFor(pair.base.id, pair.quote.id);
      const token = await csrfToken();
      if (selectedSourceID === '') {
        if (existing) {
          await updatePricingSourceAssignment(
            existing.id,
            assignmentRequest(pair, existing.source_id, 'archived'),
            token
          );
        }
      } else if (existing) {
        await updatePricingSourceAssignment(existing.id, assignmentRequest(pair, Number(selectedSourceID), 'active'), token);
      } else {
        await createPricingSourceAssignment(assignmentRequest(pair, Number(selectedSourceID), 'active'), token);
      }
      actionMessage = m.currencies_assignment_saved();
      await refreshAfterMutation();
    } catch (error) {
      actionError = error;
    } finally {
      assignmentSavingKey = '';
    }
  }

  function assignmentRequest(pair: RatePair, sourceID: number, status: 'active' | 'archived') {
    return {
      commodity_id: pair.base.id,
      quote_commodity_id: pair.quote.id,
      source_id: sourceID,
      priority: 10,
      status,
      effective_from: todayISO(),
      change_reason: 'saved currency exchange rate provider'
    };
  }

  function rateKey(baseCommodityID: number, quoteCommodityID: number): string {
    return `${baseCommodityID}:${quoteCommodityID}`;
  }

  function currencyName(currency: CurrencyResponse): string {
    return currencyDisplayNames.of(currency.code) ?? currency.name;
  }

  function rateObservation(base: CurrencyResponse, quote: CurrencyResponse): PriceObservationResponse | null | undefined {
    if (base.id === quote.id) {
      return null;
    }

    return latestRatesQuery.data?.get(rateKey(base.id, quote.id));
  }

  function assignmentFor(baseCommodityID: number, quoteCommodityID: number): PricingSourceAssignmentResponse | undefined {
    return assignmentsQuery.data?.assignments.find(
      (assignment) =>
        assignment.status === 'active' &&
        assignment.commodity_id === baseCommodityID &&
        assignment.quote_commodity_id === quoteCommodityID
    );
  }

  function sourceForObservation(observation: PriceObservationResponse | null | undefined): MarketDataSourceResponse | undefined {
    return observation?.source_id ? providerByID.get(observation.source_id) : undefined;
  }

  function healthFor(baseCommodityID: number, quoteCommodityID: number) {
    return sourceHealthQuery.data?.sources.find(
      (health) => health.commodity_id === baseCommodityID && health.quote_commodity_id === quoteCommodityID
    );
  }

  function formatDate(value: string): string {
    return dateFormatter.format(parseISO(value));
  }

  function formatDateTime(value: string): string {
    return dateTimeFormatter.format(parseISO(value));
  }

  function formatRate(price: PriceObservationResponse): string {
    const decimalText = scaledRatioToDecimalText(
      BigInt(price.price_value),
      price.price_scale,
      BigInt(price.base_quantity_value),
      price.base_quantity_scale,
      8
    );

    return numberFormatter.format(Number(decimalText));
  }

  function numberOrUndefined(value: string): number | undefined {
    return value === '' ? undefined : Number(value);
  }

  function todayISO(): string {
    return new Date().toISOString().slice(0, 10);
  }

  function scaledRatioToDecimalText(
    priceValue: bigint,
    priceScale: number,
    baseQuantityValue: bigint,
    baseQuantityScale: number,
    maxDecimals: number
  ): string {
    if (baseQuantityValue === 0n) {
      return '0';
    }

    const numerator = priceValue * pow10(baseQuantityScale);
    const denominator = baseQuantityValue * pow10(priceScale);
    const integer = numerator / denominator;
    let remainder = numerator % denominator;
    let decimals = '';

    for (let index = 0; index < maxDecimals && remainder !== 0n; index += 1) {
      remainder *= 10n;
      decimals += String(remainder / denominator);
      remainder %= denominator;
    }

    decimals = decimals.replace(/0+$/, '');
    return decimals ? `${integer}.${decimals}` : String(integer);
  }

  function pow10(exponent: number): bigint {
    return 10n ** BigInt(exponent);
  }
</script>

{#if isLoading}
  <StatePanel title={m.currencies_loading_title()} copy={m.currencies_loading_copy()} />
{:else if isError}
  <StatePanel title={m.currencies_error_title()} copy={m.currencies_error_copy()}>
    <p class="text-sm leading-6 text-muted">{getAPIClientErrorMessage(pageError)}</p>
    <button
      type="button"
      class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      onclick={retry}
    >
      <RefreshCw size={16} aria-hidden="true" />
      {m.currencies_retry()}
    </button>
  </StatePanel>
{:else if activeCurrencies.length === 0}
  <StatePanel title={m.currencies_empty_title()} copy={m.currencies_empty_copy()} />
{:else}
  <section class="space-y-4">
    <Panel>
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <StatusBadge tone="accent">{m.currencies_badge()}</StatusBadge>
          <h2 class="mt-3 text-lg font-semibold tracking-tight text-balance">{m.currencies_title()}</h2>
          <p class="mt-2 max-w-3xl text-sm leading-6 text-muted">{m.currencies_copy()}</p>
        </div>

        {#if defaultCurrency}
          <div class="rounded-(--radius-panel) border border-border bg-toolbar px-4 py-3 text-sm">
            <p class="font-semibold text-foreground">{m.currencies_default_label()}</p>
            <p class="mt-1 text-muted">
              {m.currencies_default_value({ code: defaultCurrency.code, name: currencyName(defaultCurrency) })}
            </p>
          </div>
        {/if}
      </div>

      <div class="mt-6 grid gap-4 lg:grid-cols-2">
        <label class="block text-sm font-semibold text-foreground">
          {m.currencies_policy_base_label()}
          <select
            bind:value={policyBaseCommodityID}
            class="mt-2 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
          >
            {#each activeCurrencies as currency}
              <option value={String(currency.id)}>{currency.code} - {currencyName(currency)}</option>
            {/each}
          </select>
        </label>

        <label class="block text-sm font-semibold text-foreground">
          {m.currencies_policy_provider_label()}
          <select
            bind:value={policyDefaultSourceID}
            class="mt-2 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
          >
            {#each providerSources as source}
              <option value={String(source.id)}>{source.name}</option>
            {/each}
          </select>
        </label>

        <label class="flex items-center gap-3 text-sm font-semibold text-foreground">
          <input type="checkbox" bind:checked={refreshEnabled} class="size-4 accent-[var(--color-accent)]" />
          {m.currencies_policy_refresh_enabled()}
        </label>

        <div class="grid grid-cols-3 gap-3">
          <label class="block text-sm font-semibold text-foreground">
            {m.currencies_policy_hour_label()}
            <input
              type="number"
              min="0"
              max="23"
              bind:value={refreshHourUTC}
              class="mt-2 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
            />
          </label>

          <label class="block text-sm font-semibold text-foreground">
            {m.currencies_policy_minute_label()}
            <input
              type="number"
              min="0"
              max="59"
              bind:value={refreshMinuteUTC}
              class="mt-2 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
            />
          </label>

          <label class="block text-sm font-semibold text-foreground">
            {m.currencies_policy_hops_label()}
            <input
              type="number"
              min="0"
              max="4"
              bind:value={triangulationMaxHops}
              class="mt-2 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
            />
          </label>
        </div>
      </div>

      <div class="mt-5 flex flex-wrap items-center gap-3">
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
          disabled={policySaving || providerSources.length === 0}
          onclick={handleSavePolicy}
        >
          <Save size={16} aria-hidden="true" />
          {policySaving ? m.currencies_policy_saving() : m.currencies_policy_save()}
        </button>

        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-60"
          disabled={refreshRunning || providerSources.length === 0}
          onclick={handleRunRefresh}
        >
          <RefreshCw size={16} aria-hidden="true" />
          {refreshRunning ? m.currencies_refresh_running() : m.currencies_refresh_now()}
        </button>

        {#if latestRun}
          <p class="text-sm text-muted">
            {m.currencies_last_run({
              status: latestRun.status,
              time: latestRun.finished_at ? formatDateTime(latestRun.finished_at) : formatDateTime(latestRun.started_at)
            })}
          </p>
        {/if}
      </div>

      {#if actionMessage}
        <p class="mt-4 text-sm font-semibold text-positive">{actionMessage}</p>
      {/if}

      {#if actionError}
        <p class="mt-4 text-sm font-semibold text-danger">{getAPIClientErrorMessage(actionError)}</p>
      {/if}

      <div class="mt-6">
        <fieldset>
          <legend class="text-sm font-semibold text-foreground">{m.currencies_rate_direction_label()}</legend>
          <div class="mt-3 flex flex-wrap gap-2">
            {#each directionOptions as option}
              <button
                type="button"
                aria-pressed={rateDirection === option.value}
                class:bg-selected={rateDirection === option.value}
                class:text-selected-foreground={rateDirection === option.value}
                class:bg-control={rateDirection !== option.value}
                class:text-foreground={rateDirection !== option.value}
                class="rounded-(--radius-control) border border-border px-3 py-2 text-sm font-semibold transition hover:bg-control-hover"
                onclick={() => (rateDirection = option.value)}
              >
                {option.label}
              </button>
            {/each}
          </div>
        </fieldset>
      </div>
    </Panel>

    <Panel padding="none">
      <div class="overflow-x-auto">
        <table class="w-full min-w-[68rem] text-left text-sm">
          <thead class="border-b border-border bg-toolbar text-xs font-semibold uppercase tracking-[0.12em] text-muted">
            <tr>
              <th class="px-4 py-3">{m.currencies_table_currency()}</th>
              <th class="px-4 py-3">{m.currencies_table_precision()}</th>
              <th class="px-4 py-3">{m.currencies_table_rate()}</th>
              <th class="px-4 py-3">{m.currencies_table_provider()}</th>
              <th class="px-4 py-3">{m.currencies_table_last_update()}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border">
            {#each activeCurrencies as currency}
              <tr class="bg-surface">
                <td class="px-4 py-3 align-top">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="font-semibold text-foreground">{currency.code}</span>
                    {#if defaultCurrency?.id === currency.id}
                      <StatusBadge tone="positive">{m.currencies_default_badge()}</StatusBadge>
                    {/if}
                  </div>
                  <p class="mt-1 text-sm text-muted">{currencyName(currency)}</p>
                </td>
                <td class="px-4 py-3 align-top text-muted">
                  {m.currencies_precision_value({
                    standard: currency.standard_scale,
                    maximum: currency.max_quantity_scale
                  })}
                </td>
                <td class="px-4 py-3 align-top">
                  {#if defaultCurrency && defaultCurrency.id === currency.id}
                    <p class="font-semibold text-foreground">
                      {m.currencies_identity_rate({ code: currency.code })}
                    </p>
                  {:else if defaultCurrency}
                    <div class="space-y-3">
                      {#if rateDirection === 'currency_default' || rateDirection === 'both'}
                        {@const observation = rateObservation(currency, defaultCurrency)}
                        <div>
                          <p class="font-semibold text-foreground">
                            {currency.code}/{defaultCurrency.code}
                          </p>
                          <p class="text-muted">
                            {observation
                              ? m.currencies_rate_value({
                                  rate: formatRate(observation),
                                  quote: defaultCurrency.code,
                                  base: currency.code
                                })
                              : m.currencies_rate_missing()}
                          </p>
                          {#if observation?.is_derived}
                            <p class="mt-1 inline-flex items-start gap-1 text-xs font-semibold text-warning">
                              <AlertTriangle size={14} aria-hidden="true" />
                              {m.currencies_derived_warning()}
                            </p>
                          {/if}
                        </div>
                      {/if}

                      {#if rateDirection === 'default_currency' || rateDirection === 'both'}
                        {@const observation = rateObservation(defaultCurrency, currency)}
                        <div>
                          <p class="font-semibold text-foreground">
                            {defaultCurrency.code}/{currency.code}
                          </p>
                          <p class="text-muted">
                            {observation
                              ? m.currencies_rate_value({
                                  rate: formatRate(observation),
                                  quote: currency.code,
                                  base: defaultCurrency.code
                                })
                              : m.currencies_rate_missing()}
                          </p>
                          {#if observation?.is_derived}
                            <p class="mt-1 inline-flex items-start gap-1 text-xs font-semibold text-warning">
                              <AlertTriangle size={14} aria-hidden="true" />
                              {m.currencies_derived_warning()}
                            </p>
                          {/if}
                        </div>
                      {/if}
                    </div>
                  {:else}
                    <span class="text-muted">{m.currencies_rate_missing_default()}</span>
                  {/if}
                </td>
                <td class="px-4 py-3 align-top">
                  {#if defaultCurrency && defaultCurrency.id !== currency.id}
                    <div class="space-y-3">
                      {#if rateDirection === 'currency_default' || rateDirection === 'both'}
                        {@const pair = { base: currency, quote: defaultCurrency }}
                        {@const observation = rateObservation(currency, defaultCurrency)}
                        <div>
                          <p class="mb-1 text-xs font-semibold text-muted">{currency.code}/{defaultCurrency.code}</p>
                          <select
                            value={assignmentFor(currency.id, defaultCurrency.id)?.source_id
                              ? String(assignmentFor(currency.id, defaultCurrency.id)?.source_id)
                              : ''}
                            disabled={assignmentSavingKey === rateKey(currency.id, defaultCurrency.id)}
                            class="w-full rounded-(--radius-control) border border-border bg-control px-2 py-1.5 text-sm text-foreground disabled:opacity-60"
                            onchange={(event) => handleAssignmentChange(pair, event.currentTarget.value)}
                          >
                            <option value="">{m.currencies_provider_global()}</option>
                            {#each providerSources as source}
                              <option value={String(source.id)}>{source.name}</option>
                            {/each}
                          </select>
                          <p class="mt-1 text-xs text-muted">
                            {sourceForObservation(observation)?.name ?? m.currencies_provider_unknown()}
                          </p>
                        </div>
                      {/if}

                      {#if rateDirection === 'default_currency' || rateDirection === 'both'}
                        {@const pair = { base: defaultCurrency, quote: currency }}
                        {@const observation = rateObservation(defaultCurrency, currency)}
                        <div>
                          <p class="mb-1 text-xs font-semibold text-muted">{defaultCurrency.code}/{currency.code}</p>
                          <select
                            value={assignmentFor(defaultCurrency.id, currency.id)?.source_id
                              ? String(assignmentFor(defaultCurrency.id, currency.id)?.source_id)
                              : ''}
                            disabled={assignmentSavingKey === rateKey(defaultCurrency.id, currency.id)}
                            class="w-full rounded-(--radius-control) border border-border bg-control px-2 py-1.5 text-sm text-foreground disabled:opacity-60"
                            onchange={(event) => handleAssignmentChange(pair, event.currentTarget.value)}
                          >
                            <option value="">{m.currencies_provider_global()}</option>
                            {#each providerSources as source}
                              <option value={String(source.id)}>{source.name}</option>
                            {/each}
                          </select>
                          <p class="mt-1 text-xs text-muted">
                            {sourceForObservation(observation)?.name ?? m.currencies_provider_unknown()}
                          </p>
                        </div>
                      {/if}
                    </div>
                  {:else}
                    <span class="text-muted">{m.currencies_provider_identity()}</span>
                  {/if}
                </td>
                <td class="px-4 py-3 align-top text-muted">
                  {#if defaultCurrency && defaultCurrency.id === currency.id}
                    {m.currencies_rate_identity_date()}
                  {:else if defaultCurrency}
                    <div class="space-y-3">
                      {#if rateDirection === 'currency_default' || rateDirection === 'both'}
                        {@const observation = rateObservation(currency, defaultCurrency)}
                        {@const health = healthFor(currency.id, defaultCurrency.id)}
                        <div>
                          <p>{observation ? formatDate(observation.valuation_date) : m.currencies_rate_missing_date()}</p>
                          <p class="text-xs">
                            {health?.last_attempt_at
                              ? m.currencies_last_attempt({ time: formatDateTime(health.last_attempt_at) })
                              : m.currencies_last_attempt_missing()}
                          </p>
                        </div>
                      {/if}

                      {#if rateDirection === 'default_currency' || rateDirection === 'both'}
                        {@const observation = rateObservation(defaultCurrency, currency)}
                        {@const health = healthFor(defaultCurrency.id, currency.id)}
                        <div>
                          <p>{observation ? formatDate(observation.valuation_date) : m.currencies_rate_missing_date()}</p>
                          <p class="text-xs">
                            {health?.last_attempt_at
                              ? m.currencies_last_attempt({ time: formatDateTime(health.last_attempt_at) })
                              : m.currencies_last_attempt_missing()}
                          </p>
                        </div>
                      {/if}
                    </div>
                  {:else}
                    {m.currencies_rate_missing_date()}
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </Panel>
  </section>
{/if}
