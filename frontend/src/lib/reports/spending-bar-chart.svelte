<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import type { SpendingBar } from './spending';

  let {
    bars,
    formatAmount,
    caption
  }: {
    bars: SpendingBar[];
    formatAmount: (value: string, scale: number) => string;
    caption: string;
  } = $props();

  const ROW_HEIGHT = 34;
  const BAR_HEIGHT = 18;
  const LABEL_WIDTH = 148;
  const TRACK_START = LABEL_WIDTH + 12;
  const TRACK_WIDTH = 260;
  // Values live in their own right-hand column so they line up and can never be
  // clipped by the widest bar.
  const VALUE_X = TRACK_START + TRACK_WIDTH + 8;
  const VALUE_GUTTER = 116;
  const VIEW_WIDTH = VALUE_X + VALUE_GUTTER;
  const MAX_LABEL_CHARS = 22;

  const height = $derived(Math.max(bars.length * ROW_HEIGHT, ROW_HEIGHT));
  let hoveredKey = $state<string | null>(null);

  function barWidth(ratio: number): number {
    // Keep a hairline for non-zero magnitudes so a tiny value stays visible.
    if (ratio <= 0) return 0;
    return Math.max(ratio * TRACK_WIDTH, 2);
  }

  function truncate(label: string): string {
    return label.length > MAX_LABEL_CHARS ? `${label.slice(0, MAX_LABEL_CHARS - 1)}…` : label;
  }
</script>

<!--
  A single-series magnitude chart: one accent hue for every bar, so no legend is
  needed (the heading names the series) and no categorical palette is involved.
  The table above is the accessible source of truth; this summarizes it.
-->
<figure class="mt-5 mb-0">
  <figcaption class="text-xs text-muted">{caption}</figcaption>
  <div class="mt-2 overflow-x-auto">
    <svg
      viewBox={`0 0 ${VIEW_WIDTH} ${height}`}
      width={VIEW_WIDTH}
      height={height}
      role="img"
      aria-label={caption}
      class="max-w-full"
    >
      <!-- Recessive baseline the bars are anchored to. -->
      <line
        x1={TRACK_START}
        y1="0"
        x2={TRACK_START}
        y2={height}
        stroke="var(--color-border)"
        stroke-width="1"
      />
      {#each bars as bar, index (bar.key)}
        {@const y = index * ROW_HEIGHT}
        {@const barY = y + (ROW_HEIGHT - BAR_HEIGHT) / 2}
        {@const width = barWidth(bar.ratio)}
        <g
          onmouseenter={() => (hoveredKey = bar.key)}
          onmouseleave={() => (hoveredKey = null)}
          role="presentation"
        >
          <!-- Hit target larger than the mark. -->
          <rect x="0" y={y} width={VIEW_WIDTH} height={ROW_HEIGHT} fill="transparent" />
          <text
            x={LABEL_WIDTH}
            y={y + ROW_HEIGHT / 2}
            text-anchor="end"
            dominant-baseline="middle"
            class="fill-muted text-[11px]"
          >{truncate(bar.label)}</text>
          {#if width > 0}
            <rect
              x={TRACK_START}
              y={barY}
              width={width}
              height={BAR_HEIGHT}
              rx="4"
              fill="var(--color-accent)"
              opacity={hoveredKey === null || hoveredKey === bar.key ? 1 : 0.55}
            />
            {#if bar.negative}
              <!--
                Non-colour cue for a refund-dominated row: a hatch overlay, so the
                sign is never carried by colour alone.
              -->
              <rect
                x={TRACK_START}
                y={barY}
                width={width}
                height={BAR_HEIGHT}
                rx="4"
                fill="url(#spending-negative-hatch)"
              />
            {/if}
          {/if}
          <text
            x={VALUE_X}
            y={y + ROW_HEIGHT / 2}
            dominant-baseline="middle"
            class="fill-foreground text-[11px] tabular-nums"
          >{formatAmount(bar.quantityValue, bar.quantityScale)}</text>
        </g>
      {/each}
      <defs>
        <pattern
          id="spending-negative-hatch"
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
  {#if bars.some((bar) => bar.negative)}
    <p class="mt-2 text-xs text-muted">{m.reports_spending_negative_note()}</p>
  {/if}
</figure>
