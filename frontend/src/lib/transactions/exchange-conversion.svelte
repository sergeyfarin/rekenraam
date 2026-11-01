<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { transactionQueryOptions, type TransactionResponse } from '#lib/api/transactions.ts';
  import { formatQuantity, formatRatio, joinCommodityAmount } from '#lib/money/format.ts';
  import { coefficientSign, negateCoefficient } from '#lib/money/amount.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';

  // A currency exchange against the market (#481): the rate the two exact
  // amounts imply, and, when a market rate covers the day, what the deal cost
  // or gained at it. Read on demand, never stored or booked; a market rate
  // recorded later shows on the next read.
  let { transaction }: { transaction: TransactionResponse } = $props();

  const query = createQuery(() => transactionQueryOptions(transaction.id));
  const locale = $derived(getLocale());
  const conversion = $derived(query.data?.exchange_conversion);

  const codes = $derived(
    new Map(
      (transaction.journal_entries ?? []).flatMap((e) => e.postings ?? []).map((p) => [p.commodity_id, p.commodity_code] as const)
    )
  );
  const code = (id: number) => codes.get(id) ?? '';
  const rate = (value: string, scale: number) => formatRatio({ value, scale }, { value: '1', scale: 0 }, 6, locale);
  const magnitude = (value: string) => (coefficientSign(value) < 0 ? negateCoefficient(value) : value);
</script>

<div class="rounded-[var(--radius-control)] border border-border bg-surface px-3 py-2 text-sm" aria-live="polite">
  <p class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.transactions_exchange_title()}</p>
  {#if query.isPending}
    <p class="mt-1 text-muted">{m.recurring_loading()}</p>
  {:else if query.isError}
    <APIFormError error={query.error} />
    <button type="button" class="mt-1 min-h-10 text-xs font-medium text-accent underline-offset-2 hover:underline" onclick={() => query.refetch()}>{m.recurring_refresh()}</button>
  {:else if conversion}
    {@const from = code(conversion.sent.commodity_id)}
    {@const to = code(conversion.received.commodity_id)}
    <p class="mt-1 tabular-nums text-foreground">
      {m.transactions_transfer_rate({ rate: rate(conversion.implied_rate.quantity_value, conversion.implied_rate.quantity_scale), from, to })}
    </p>
    {#if conversion.market}
      {@const market = conversion.market}
      <p class="mt-1 tabular-nums text-muted">
        {m.transactions_exchange_market({ date: market.observation_date, rate: rate(market.rate.quantity_value, market.rate.quantity_scale), from, to })}
      </p>
      {#if market.stale}
        <p class="text-xs text-muted">{m.transactions_exchange_market_stale({ date: market.observation_date })}</p>
      {/if}
      {@const sign = coefficientSign(market.cost.quantity_value)}
      {@const amount = joinCommodityAmount(from, formatQuantity(magnitude(market.cost.quantity_value), market.cost.quantity_scale, locale))}
      {@const percent = formatQuantity(magnitude(market.cost_percent), 2, locale)}
      <p class="mt-1 font-medium tabular-nums text-foreground">
        {sign > 0
          ? m.transactions_exchange_cost_below({ amount, percent })
          : sign < 0
            ? m.transactions_exchange_cost_above({ amount, percent })
            : m.transactions_exchange_cost_equal()}
      </p>
    {:else}
      <p class="mt-1 text-muted">{m.transactions_exchange_no_market({ date: conversion.conversion_date })}</p>
    {/if}
    <p class="mt-1 text-xs text-muted">{m.transactions_exchange_not_booked()}</p>
  {/if}
</div>
