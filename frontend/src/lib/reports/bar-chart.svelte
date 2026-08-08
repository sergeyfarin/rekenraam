<script lang="ts">
  import { chartBars, type ChartPoint } from './report-chart';

  let { points, title } = $props<{ points: ChartPoint[]; title: string }>();

  const bars = $derived(chartBars(points));
</script>

<!--
  A summary of the table below it. Marked aria-hidden because every figure it
  shows is already in the table, in exact form and with its commodity.

  A negative bar (a category that nets an inflow after refunds) is distinguished
  by a dashed outline as well as by colour, since colour alone must never be the
  only cue on a financial state.
-->
{#if bars.length > 0}
  <figure class="mt-5" aria-hidden="true">
    <figcaption class="sr-only">{title}</figcaption>
    <ul class="space-y-1.5">
      {#each bars as bar (bar.label)}
        <li class="flex items-center gap-3">
          <span class="w-40 shrink-0 truncate text-xs text-muted">{bar.label}</span>
          <span class="h-3 flex-1 rounded-full bg-surface-strong">
            <span
              class="block h-3 rounded-full {bar.negative
                ? 'border border-dashed border-danger bg-transparent'
                : 'bg-accent'}"
              style={`width: ${(bar.fraction * 100).toFixed(2)}%`}
            ></span>
          </span>
        </li>
      {/each}
    </ul>
  </figure>
{/if}
