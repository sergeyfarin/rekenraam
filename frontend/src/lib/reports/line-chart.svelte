<script lang="ts">
  import { linePath, type ChartPoint } from './report-chart';

  let { points, title } = $props<{ points: ChartPoint[]; title: string }>();

  const width = 720;
  const height = 160;
  const path = $derived(linePath(points, width, height));
</script>

<!--
  A summary of the table below it, never a substitute. The SVG is aria-hidden
  and the surrounding figure carries a text description, so a keyboard or
  screen-reader user loses nothing by not seeing it — every value it plots is
  in the table, exactly.
-->
{#if path}
  <figure class="mt-5">
    <figcaption class="sr-only">{title}</figcaption>
    <svg
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
      role="presentation"
      aria-hidden="true"
      class="h-40 w-full text-accent"
    >
      <polyline points={path} fill="none" stroke="currentColor" stroke-width="2" vector-effect="non-scaling-stroke" />
    </svg>
  </figure>
{/if}
