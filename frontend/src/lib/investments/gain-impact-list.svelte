<script lang="ts">
  import { m } from '#lib/paraglide/messages.js';
  import type { GainImpactRow } from '#lib/investments/gain-impact.ts';

  // The replay gain disclosure (T-114/T-126), shared by every investment
  // confirmation so a changed gain reads the same wherever it is reviewed.
  let {
    rows,
    refreshed = false,
    showHeading = false,
    labelledBy
  }: {
    rows: GainImpactRow[];
    refreshed?: boolean;
    showHeading?: boolean;
    labelledBy?: string;
  } = $props();

  const headingID = $props.id();

  function gainLabel(row: GainImpactRow): string {
    const unknown = m.investments_gain_impact_unknown();
    const values = {
      date: row.date, before: row.before ?? unknown, after: row.after ?? unknown,
      basisBefore: row.basisBefore ?? unknown, basisAfter: row.basisAfter ?? unknown, currency: row.currency
    };
    switch (row.kind) {
      case 'removed':
        return m.investments_gain_impact_removed(values);
      case 'replaced':
        return m.investments_gain_impact_replaced(values);
      default:
        return m.investments_gain_impact_revised(values);
    }
  }
</script>

<section class="px-4 py-3" aria-labelledby={showHeading ? headingID : labelledBy}>
  {#if showHeading}
    <h4 id={headingID} class="text-sm font-semibold text-foreground">{m.investments_gain_impact_title()}</h4>
  {/if}
  <p class="mt-1 text-xs leading-5 text-muted">{m.investments_gain_impact_copy()}</p>
  {#if refreshed}
    <p class="mt-2 text-xs font-semibold leading-5 text-warning" role="status">{m.investments_gain_impact_refreshed()}</p>
  {/if}
  <ul class="mt-2 divide-y divide-border text-sm">
    {#each rows as row (row.key)}
      <li class="py-2 font-mono text-foreground">{gainLabel(row)}</li>
    {/each}
  </ul>
</section>
