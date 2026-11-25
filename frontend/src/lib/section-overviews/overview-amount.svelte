<script lang="ts">
  import type { OverviewAmount } from '#lib/api/section-overviews.ts';
  import { formatQuantity, joinCommodityAmount } from '#lib/money/format.ts';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { amountPrivacy } from '#lib/money/privacy.svelte.ts';

  let { amount }: { amount: OverviewAmount } = $props();
  const value = $derived(formatQuantity(amount.quantity_value, amount.quantity_scale, getLocale()));
</script>

<span class="wrap-anywhere font-semibold tabular-nums text-foreground" data-testid="overview-amount">
  {amountPrivacy.hidden ? value : joinCommodityAmount(amount.commodity_code, value)}
</span>
