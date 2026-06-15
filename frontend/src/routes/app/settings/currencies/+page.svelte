<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import { parseISO } from 'date-fns';
  import { currentBookQueryOptions } from '$lib/api/books';
  import { currenciesQueryOptions, type CurrencyResponse } from '$lib/api/currencies';
  import { getLatestPriceObservation, type PriceObservationResponse } from '$lib/api/pricing';
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

  let rateDirection = $state<RateDirection>('currency_default');

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

  const activeCurrencies = $derived.by(() =>
    [...(currenciesQuery.data?.currencies.filter((currency) => currency.status === 'active') ?? [])].sort((left, right) =>
      left.code.localeCompare(right.code)
    )
  );

  const defaultCurrency = $derived.by(() => {
    const defaultCurrencyID = currentBookQuery.data?.default_currency_commodity_id;
    return activeCurrencies.find((currency) => currency.id === defaultCurrencyID);
  });

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
    currenciesQuery.error ?? currentBookQuery.error ?? (ratesQueryEnabled ? latestRatesQuery.error : null)
  );
  const isLoading = $derived(
    currenciesQuery.isPending || currentBookQuery.isPending || (ratesQueryEnabled && latestRatesQuery.isPending)
  );
  const isError = $derived(
    currenciesQuery.isError || currentBookQuery.isError || (ratesQueryEnabled && latestRatesQuery.isError)
  );

  const directionOptions: { value: RateDirection; label: string }[] = [
    { value: 'currency_default', label: m.currencies_rate_direction_currency_default() },
    { value: 'default_currency', label: m.currencies_rate_direction_default_currency() },
    { value: 'both', label: m.currencies_rate_direction_both() }
  ];

  async function retry() {
    await Promise.all([
      currenciesQuery.refetch(),
      currentBookQuery.refetch(),
      ratesQueryEnabled ? latestRatesQuery.refetch() : Promise.resolve()
    ]);
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

  function formatDate(value: string): string {
    return dateFormatter.format(parseISO(value));
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
        <table class="w-full min-w-[48rem] text-left text-sm">
          <thead class="border-b border-border bg-toolbar text-xs font-semibold uppercase tracking-[0.12em] text-muted">
            <tr>
              <th class="px-4 py-3">{m.currencies_table_currency()}</th>
              <th class="px-4 py-3">{m.currencies_table_precision()}</th>
              <th class="px-4 py-3">{m.currencies_table_rate()}</th>
              <th class="px-4 py-3">{m.currencies_table_valuation_date()}</th>
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
                    <div class="space-y-2">
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
                        </div>
                      {/if}
                    </div>
                  {:else}
                    <span class="text-muted">{m.currencies_rate_missing_default()}</span>
                  {/if}
                </td>
                <td class="px-4 py-3 align-top text-muted">
                  {#if defaultCurrency && defaultCurrency.id === currency.id}
                    {m.currencies_rate_identity_date()}
                  {:else if defaultCurrency}
                    <div class="space-y-2">
                      {#if rateDirection === 'currency_default' || rateDirection === 'both'}
                        {@const observation = rateObservation(currency, defaultCurrency)}
                        <p>{observation ? formatDate(observation.valuation_date) : m.currencies_rate_missing_date()}</p>
                      {/if}

                      {#if rateDirection === 'default_currency' || rateDirection === 'both'}
                        {@const observation = rateObservation(defaultCurrency, currency)}
                        <p>{observation ? formatDate(observation.valuation_date) : m.currencies_rate_missing_date()}</p>
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
