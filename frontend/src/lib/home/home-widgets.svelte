<script lang="ts">
  import type { Home } from '#lib/api/home.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import HomeWidgetCard from './home-widget-card.svelte';
  import { homeWidgets, widgetTitles } from './widgets.ts';

  type Props = {
    home: Home | undefined;
    /** The Home read failed: the briefing above says so; cards keep their places. */
    failed: boolean;
    onRetry: () => void;
  };
  let { home, failed, onRetry }: Props = $props();

  const locale = $derived(getLocale());
  const widgets = $derived(home ? homeWidgets(home, locale) : null);

  const cardClass = 'flex min-w-0 flex-col rounded-(--radius-panel) border border-border bg-surface px-4 pt-3 pb-2 text-sm';
</script>

<!-- My Home has no visible heading: the widgets follow the shortcuts (#478). -->
{#if home && !home.consistent}
  <!-- The summaries are separate reads; never mix two states of the book silently. -->
  <p role="status" class="mt-8 text-sm text-muted" data-testid="home-widgets-changing">{m.home_widgets_changing()}</p>
{/if}
<section aria-label={m.home_widgets_label()} class="{home && !home.consistent ? 'mt-3' : 'mt-8'} grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3" data-testid="home-widgets">
  {#if widgets}
    {#each widgets as widget (widget.id)}
      <HomeWidgetCard {widget} {onRetry} />
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
