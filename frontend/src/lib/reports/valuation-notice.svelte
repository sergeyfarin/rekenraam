<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import { valuationNotice, type Valuation } from './reporting-currency';

  let {
    valuation,
    commodityLabel
  }: {
    valuation: Valuation | undefined;
    /** The screen's own naming, so a notice names a commodity the way the table does. */
    commodityLabel: (commodityID: number) => string;
  } = $props();

  const notice = $derived(valuationNotice(valuation));
  const separator = $derived(m.reports_filter_summary_separator());
</script>

<!--
  Printed as well as shown. A restated figure that leaned on a stale rate, or a
  total that quietly omits a commodity, is exactly the provenance a page read
  away from the screen has no other way to recover.
-->
{#if notice.kind === 'stale'}
  <p class="mt-4 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
    {m.reports_valuation_stale({ dates: notice.dates.join(separator) })}
  </p>
{:else if notice.kind === 'incomplete'}
  <p class="mt-4 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
    {m.reports_valuation_incomplete({
      commodities: notice.commodityIDs.map((id) => commodityLabel(id)).join(separator)
    })}
  </p>
{/if}
