<script lang="ts">
  import type { AllocationResponse } from '#lib/api/allocation.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { allocationAreas, allocationColor, allocationIDs, calendarFraction, savedCalendarDay } from './allocation-model.ts';

  let { data, label, percent, showMarkers, onRevision, formatDay, formatSaved }: {
    data: AllocationResponse;
    label: (id: number) => string;
    percent: (units: number) => string;
    showMarkers: boolean;
    onRevision: (id: number) => void;
    formatDay: (date: string) => string;
    formatSaved: (timestamp: string) => string;
  } = $props();
  const ids = $derived(allocationIDs(data.points, data.current.targets));
  const areas = $derived(allocationAreas(data.points, ids));
  const markers = $derived(data.revisions.map(r => ({ ...r, day: savedCalendarDay(r.saved_at, data.time_zone) })).filter(r => r.day >= data.from && r.day <= data.to));
  const path = (run: ReturnType<typeof allocationAreas>[number]['segments'][number]) => [
    ...run.map(p => `${p.x * 100},${(1 - p.upper) * 40}`),
    ...run.toReversed().map(p => `${p.x * 100},${(1 - p.lower) * 40}`)
  ].join(' ');
</script>

<figure class="space-y-3" data-testid="allocation-chart">
  <figcaption class="text-sm font-semibold text-foreground">{m.allocation_over_time()}</figcaption>
  <p class="text-sm text-muted">{m.allocation_chart_help()} {data.interval === 'day' ? m.returns_chart_interval_day() : data.interval === 'week' ? m.returns_chart_interval_week() : m.returns_chart_interval_month()}</p>
  <div class="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-3">
    <div class="min-w-0">
      <div class="mb-1 flex min-h-8 items-end justify-between text-xs text-muted" aria-hidden="true"><span>{m.allocation_actual()}</span><span>{percent(1000000)}</span></div>
      <svg viewBox="0 0 100 40" preserveAspectRatio="none" class="block h-56 w-full overflow-hidden rounded-(--radius-control) border border-border bg-surface-strong" role="img" aria-label={m.allocation_over_time()}>
        <title>{m.allocation_chart_accessible()}</title>
        {#each areas as area, index (area.id)}
          {#each area.segments as run, segment (segment)}
            {#if run.length === 1}
              <rect x={Math.min(99, Math.max(0, run[0].x * 100 - 0.5))} y={(1 - run[0].upper) * 40} width="1" height={(run[0].upper - run[0].lower) * 40} fill={allocationColor(index)} />
            {:else}
              <polygon points={path(run)} fill={allocationColor(index)} stroke="var(--color-surface)" stroke-width="0.15" />
            {/if}
          {/each}
        {/each}
        {#if showMarkers}
          {#each markers as revision (revision.id)}
            {@const x = calendarFraction(revision.day, data.from, data.to) * 100}
            <line x1={x} x2={x} y1="0" y2="40" stroke="var(--color-foreground)" stroke-dasharray="1 1" stroke-width="0.4"><title>{m.allocation_saved_change({ date: formatSaved(revision.saved_at) })}</title></line>
          {/each}
        {/if}
      </svg>
      <div class="mt-1 flex justify-between gap-2 text-xs text-muted"><span>{formatDay(data.from)}</span><span>{formatDay(data.to)}</span></div>
    </div>
    {#if data.current.targets.length}
      <div class="w-16 text-center text-xs text-muted">
        <span class="mb-1 flex min-h-8 items-end justify-center">{m.allocation_current_target()}</span>
        <svg viewBox="0 0 10 40" preserveAspectRatio="none" class="mx-auto block h-56 w-8 rounded-(--radius-control) border border-border" role="img" aria-label={m.allocation_current_target()}>
          {#each ids as id, index (id)}
            {@const target = data.current.targets.find(t => t.commodity_id === id)?.units ?? 0}
            {@const before = ids.slice(0, index).reduce((sum, key) => sum + (data.current.targets.find(t => t.commodity_id === key)?.units ?? 0), 0)}
            <rect x="0" y={(1 - (before + target) / 1000000) * 40} width="10" height={target / 1000000 * 40} fill={allocationColor(index)}><title>{label(id)}: {percent(target)}</title></rect>
          {/each}
        </svg>
      </div>
    {/if}
  </div>
  <ul class="grid gap-2 text-sm sm:grid-cols-2" aria-label={m.allocation_current_target()}>
    {#each ids as id, index (id)}
      <li class="flex min-w-0 items-start gap-2"><span class="mt-1 size-3 shrink-0 rounded-sm" style:background={allocationColor(index)} aria-hidden="true"></span><span class="wrap-anywhere text-foreground">{label(id)}{#if data.current.targets.length}: {percent(data.current.targets.find(t => t.commodity_id === id)?.units ?? 0)}{/if}</span></li>
    {/each}
  </ul>
  {#if data.points.some(p => p.chart_reason)}<p role="status" class="text-sm text-warning">{m.allocation_gaps()}</p>{/if}
  {#if showMarkers && markers.length}
    <ul class="flex flex-wrap gap-2">
      {#each markers as revision (revision.id)}<li><button type="button" class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 text-sm text-foreground" onclick={() => onRevision(revision.id)}>{m.allocation_saved_change({ date: formatSaved(revision.saved_at) })}</button></li>{/each}
    </ul>
  {/if}
</figure>
