<script lang="ts">
  import type { components } from '#lib/api/schema.js';
  import { formatQuantity } from '#lib/money/format.ts';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { m } from '#lib/paraglide/messages.js';
  let { value, commodities }: {
    value: components['schemas']['BalanceQuantity'] | null;
    commodities: components['schemas']['HomeReportingCurrency'][];
  } = $props();
  const commodity = $derived(commodities.find(c => c.commodity_id === value?.commodity_id));
</script>
{#if value}<span class="wrap-anywhere font-mono tabular-nums">{formatQuantity(value.normal_quantity_value, value.quantity_scale, getLocale())} {commodity?.code}</span>
{:else}<span class="text-sm text-muted">{m.home_trend_unavailable()}</span>{/if}
