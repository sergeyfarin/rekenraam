<script lang="ts">
  import type { Home } from '#lib/api/home.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { homeWidgets, widgetTitles, type WidgetView } from './widgets.ts';

  type Props = {
    home: Home | undefined;
    /** The Home read failed: the briefing above says so; cards keep their places. */
    failed: boolean;
    onRetry: () => void;
  };
  let { home, failed, onRetry }: Props = $props();

  const locale = $derived(getLocale());
  const widgets = $derived(home ? homeWidgets(home, locale) : null);

  // White space belongs around a widget, not inside it; a row of the grid
  // stretches its cards to one height.
  const cardClass = 'flex min-w-0 flex-col rounded-(--radius-panel) border border-border bg-surface px-4 pt-3 pb-2 text-sm';

  /** Polyline segments between known points; a missing value is a gap, never zero. */
  function segments(points: (number | null)[]): string[] {
    const step = points.length > 1 ? 100 / (points.length - 1) : 0;
    const result: string[] = [];
    let current: string[] = [];
    points.forEach((point, index) => {
      if (point === null) {
        if (current.length) result.push(current.join(' '));
        current = [];
        return;
      }
      current.push(`${(index * step).toFixed(2)},${(30 - point * 28).toFixed(2)}`);
    });
    if (current.length) result.push(current.join(' '));
    return result;
  }
</script>

{#snippet card(widget: WidgetView)}
  <article id="home-widget-{widget.id}" aria-labelledby="home-widget-{widget.id}-title" data-testid="home-widget-{widget.id}" data-state={widget.state} class="{cardClass} {widget.wide ? 'md:col-span-2' : ''}">
    <h2 id="home-widget-{widget.id}-title" class="text-sm font-semibold">{widget.title}</h2>
    {#if widget.state === 'failed'}
      <p role="alert" class="mt-2 text-muted">{widget.scope}</p>
      <p class="mt-2">
        <button type="button" onclick={onRetry} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 text-sm font-semibold">{m.attention_retry()}</button>
      </p>
    {:else}
      {#if widget.fact}
        {#if widget.factLabel}
          <p class="mt-1 text-xl font-semibold tabular-nums {widget.factSign && widget.factSign > 0 ? 'text-positive' : widget.factSign && widget.factSign < 0 ? 'text-danger' : ''}">
            <span aria-hidden="true">{widget.fact}</span><span class="sr-only">{widget.factLabel}</span>
          </p>
        {:else}
          <p class="mt-1 text-xl font-semibold tabular-nums break-words {widget.state === 'no_data' || widget.state === 'not_set_up' ? 'text-muted' : ''}">{widget.fact}</p>
        {/if}
      {/if}
      {#if widget.scope}<p class="text-xs text-muted">{widget.scope}</p>{/if}
      {#if widget.chart}
        <svg viewBox="0 0 100 32" preserveAspectRatio="none" role="img" aria-label={widget.chart.label} class="mt-2 block h-16 w-full">
          {#each segments(widget.chart.points) as points, index (index)}
            <polyline {points} fill="none" stroke="var(--color-chart-1)" stroke-width="2" stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke" />
          {/each}
        </svg>
      {/if}
      {#if widget.rows.length}
        <ul class="mt-2">
          {#each widget.rows as row (row.key)}
            <li class="border-b border-border py-1.5 last:border-b-0">
              <div class="flex items-baseline justify-between gap-3">
                <span class="min-w-0 flex-1 truncate">
                  {#if row.href}<a href={row.href} class="underline-offset-4 hover:underline">{row.label}</a>{:else}{row.label}{/if}
                  {#if row.detail}<span class="{row.over ? 'text-warning' : 'text-muted'}"> · {row.detail}</span>{/if}
                </span>
                <span class="max-w-[60%] text-right tabular-nums">{row.amount}</span>
              </div>
              {#if row.bar !== undefined}
                <span aria-hidden="true" class="mt-1 block h-1.5 rounded-full bg-surface-strong"><span class="block h-full rounded-full bg-chart-1" style:width="{Math.round(row.bar * 100)}%"></span></span>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
      {#if widget.more}<p class="mt-1 text-xs text-muted">{widget.more}</p>{/if}
      {#if widget.action}
        <p class="mt-3"><a href={widget.action.href} class="inline-flex min-h-11 items-center rounded-(--radius-control) border border-border bg-control px-3 text-sm font-semibold">{widget.action.text}</a></p>
      {/if}
    {/if}
    <p class="mt-auto flex justify-end pt-1">
      <a href={widget.open.href} class="inline-flex min-h-11 items-center font-medium text-accent underline-offset-4 hover:underline">{widget.open.text}&nbsp;→</a>
    </p>
  </article>
{/snippet}

<!-- My Home has no visible heading: the widgets follow the shortcuts (#478). -->
<section aria-label={m.home_widgets_label()} class="mt-8 grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3" data-testid="home-widgets">
  {#if widgets}
    {#each widgets as widget (widget.id)}
      {@render card(widget)}
    {/each}
  {:else if !failed}
    {#each widgetTitles() as placeholder (placeholder.id)}
      <!-- A placeholder holds about a card's height, so content arriving late
           moves little of the page below it. -->
      <div aria-hidden="true" class="{cardClass} min-h-52 {placeholder.wide ? 'md:col-span-2' : ''}">
        <p class="text-sm font-semibold text-muted">{placeholder.title}</p>
        <span class="mt-2 block h-6 w-32 rounded-(--radius-control) bg-surface-strong"></span>
      </div>
    {/each}
  {/if}
</section>
