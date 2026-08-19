<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import { hasNegativeColumn, type SeriesColumn } from './report-series';

  let {
    columns,
    formatAmount,
    caption
  }: {
    columns: SeriesColumn[];
    formatAmount: (value: string, scale: number) => string;
    caption: string;
  } = $props();

  const COLUMN_WIDTH = 64;
  const BAR_WIDTH = 26;
  const PLOT_HEIGHT = 132;
  const LABEL_HEIGHT = 26;

  const signed = $derived(hasNegativeColumn(columns));
  // With no negative column the baseline sits on the floor and the full height
  // is available; once anything drops below it, the two halves share the plot so
  // a loss reads as the mirror of an equal gain.
  const baselineY = $derived(signed ? PLOT_HEIGHT / 2 : PLOT_HEIGHT);
  const halfHeight = $derived(signed ? PLOT_HEIGHT / 2 : PLOT_HEIGHT);
  const width = $derived(Math.max(columns.length * COLUMN_WIDTH, COLUMN_WIDTH));
  const height = PLOT_HEIGHT + LABEL_HEIGHT;

  let hoveredKey = $state<string | null>(null);

  /** Keeps a hairline for a non-zero value so a tiny column stays visible. */
  function barHeight(ratio: number): number {
    const magnitude = Math.abs(ratio) * halfHeight;
    if (magnitude === 0) return 0;
    return Math.max(magnitude, 2);
  }
</script>

<!--
  A single-series signed column chart: one accent hue throughout, so no legend
  and no categorical palette. Direction is carried by which side of the baseline
  a column sits on, which is a cue that survives without colour; the hatch
  repeats it for negative columns. The table above stays the source of truth.
-->
<figure class="mt-5 mb-0">
  <figcaption class="text-xs text-muted">{caption}</figcaption>
  <div class="mt-2 overflow-x-auto">
    <svg
      viewBox={`0 0 ${width} ${height}`}
      {width}
      {height}
      role="img"
      aria-label={caption}
      class="max-w-full"
    >
      <line
        x1="0"
        y1={baselineY}
        x2={width}
        y2={baselineY}
        stroke="var(--color-border)"
        stroke-width="1"
      />
      {#each columns as column, index (column.key)}
        {@const x = index * COLUMN_WIDTH}
        {@const barX = x + (COLUMN_WIDTH - BAR_WIDTH) / 2}
        {@const size = barHeight(column.ratio)}
        {@const barY = column.negative ? baselineY : baselineY - size}
        <g
          onmouseenter={() => (hoveredKey = column.key)}
          onmouseleave={() => (hoveredKey = null)}
          role="presentation"
        >
          <rect x={x} y="0" width={COLUMN_WIDTH} height={height} fill="transparent" />
          {#if size > 0}
            <rect
              x={barX}
              y={barY}
              width={BAR_WIDTH}
              height={size}
              rx="3"
              fill="var(--color-accent)"
              opacity={hoveredKey === null || hoveredKey === column.key ? 1 : 0.55}
            />
            {#if column.negative}
              <rect
                x={barX}
                y={barY}
                width={BAR_WIDTH}
                height={size}
                rx="3"
                fill="url(#bucket-column-negative-hatch)"
              />
            {/if}
          {/if}
          <text
            x={x + COLUMN_WIDTH / 2}
            y={PLOT_HEIGHT + LABEL_HEIGHT / 2}
            text-anchor="middle"
            dominant-baseline="middle"
            class="fill-muted text-[10px]"
          >{column.label}</text>
          {#if hoveredKey === column.key}
            <title>{formatAmount(column.quantityValue, column.quantityScale)}</title>
          {/if}
        </g>
      {/each}
      <defs>
        <pattern
          id="bucket-column-negative-hatch"
          width="6"
          height="6"
          patternTransform="rotate(45)"
          patternUnits="userSpaceOnUse"
        >
          <line x1="0" y1="0" x2="0" y2="6" stroke="var(--color-surface)" stroke-width="2" />
        </pattern>
      </defs>
    </svg>
  </div>
  {#if signed}
    <p class="mt-2 text-xs text-muted">{m.reports_chart_negative_note()}</p>
  {/if}
</figure>
