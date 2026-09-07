<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import { forecastChartPoints, type ForecastSeries } from './forecast-model';

  let {
    series,
    formatDate,
    formatAmount
  }: {
    series: ForecastSeries;
    formatDate: (date: string) => string;
    formatAmount: (value: string, scale: number) => string;
  } = $props();

  const points = $derived(forecastChartPoints(series));
  const width = $derived(Math.max(720, points.length * 12));
  const plotHeight = 180;
  const left = 16;
  const right = 16;
  const top = 16;
  const bottom = 28;
  const x = (value: number) => left + value * (width - left - right);
  const y = (value: number) => top + value * plotHeight;
  const recordedPath = $derived(points.map((point) => `${x(point.x)},${y(point.recordedY)}`).join(' '));
  const projectedPath = $derived(points.map((point) => `${x(point.x)},${y(point.projectedY)}`).join(' '));
  const first = $derived(series.points[0]);
  const last = $derived(series.points.at(-1));
</script>

<figure class="mt-5">
  <figcaption class="text-sm font-semibold text-foreground">{m.forecast_chart_caption()}</figcaption>
  <div class="mt-2 flex flex-wrap gap-x-5 gap-y-2 text-xs text-muted" aria-hidden="true">
    <span class="inline-flex items-center gap-2"><span class="h-0.5 w-7 bg-accent"></span>{m.forecast_recorded_only()}</span>
    <span class="inline-flex items-center gap-2"><span class="w-7 border-t-2 border-dashed border-warning"></span>{m.forecast_with_recurring()}</span>
  </div>
  <!-- svelte-ignore a11y_no_noninteractive_tabindex (keyboard users must be able to scroll the bounded chart region) -->
  <div class="mt-3 overflow-x-auto rounded-(--radius-control) border border-border bg-surface-strong/45" role="region" tabindex="0" aria-label={m.forecast_chart_scroll_label()}>
    <svg viewBox={`0 0 ${width} ${plotHeight + top + bottom}`} {width} height={plotHeight + top + bottom} role="img" aria-label={m.forecast_chart_accessible_label()}>
      <line x1={left} y1={top + plotHeight} x2={width - right} y2={top + plotHeight} stroke="var(--color-border)" stroke-width="1" />
      {#if points.length > 0}
        <polyline points={recordedPath} fill="none" stroke="var(--color-accent)" stroke-width="2.5" stroke-linejoin="round" />
        <polyline points={projectedPath} fill="none" stroke="var(--color-warning)" stroke-width="2.5" stroke-dasharray="7 5" stroke-linejoin="round" />
      {/if}
      {#if first}
        <text x={left} y={top + plotHeight + 19} class="fill-muted text-[11px]">{formatDate(first.date)}</text>
      {/if}
      {#if last}
        <text x={width - right} y={top + plotHeight + 19} text-anchor="end" class="fill-muted text-[11px]">{formatDate(last.date)}</text>
      {/if}
    </svg>
  </div>
  {#if last}
    <p class="mt-2 text-xs leading-5 text-muted">
      {m.forecast_chart_end_values({
        recorded: formatAmount(last.recorded_balance.quantity_value, last.recorded_balance.quantity_scale),
        projected: formatAmount(last.projected_balance.quantity_value, last.projected_balance.quantity_scale)
      })}
    </p>
  {/if}
</figure>
