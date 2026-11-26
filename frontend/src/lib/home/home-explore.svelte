<script lang="ts">
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import type { HomeLayoutState } from '#lib/api/home.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { exploreTitles, type ExploreGroup } from './explore.ts';
  import { useHomePersonalization } from './personalization.svelte.ts';
  import HomeWidgetCard from './home-widget-card.svelte';

  type Props = {
    state: HomeLayoutState | undefined;
    groups: ExploreGroup[] | undefined;
    /** The Home read failed: the briefing above says so. */
    failed: boolean;
    onRetry: () => void;
  };
  let { state, groups, failed, onRetry }: Props = $props();

  const personalization = useHomePersonalization();
  const open = $derived(state?.layout.explore_open ?? true);

  function toggle() {
    if (state) void personalization.save(state, { ...state.layout, explore_open: !open });
  }
</script>

<section aria-labelledby="home-explore-title" class="mt-12" data-testid="home-explore">
  <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
    <div class="min-w-0">
      <h2 id="home-explore-title" class="text-base font-semibold">{m.home_explore_title()}</h2>
      <p class="text-sm text-muted">{m.home_explore_subtitle()}</p>
    </div>
    <button
      disabled={!state || personalization.pending}
      type="button"
      aria-expanded={open}
      aria-controls="home-explore-body"
      onclick={toggle}
      class="inline-flex min-h-11 items-center gap-1 rounded-(--radius-control) px-2 text-sm font-medium text-accent hover:bg-control-hover"
    >
      {open ? m.home_explore_collapse() : m.home_explore_expand()}<ChevronDown size={16} aria-hidden="true" class={open ? 'rotate-180' : ''} />
    </button>
  </div>
  <div id="home-explore-body" hidden={!open}>
    {#if open && groups && state}
      {#each groups as group (group.id)}
        <section aria-labelledby="home-explore-{group.id}" class="mt-6">
          <h3 id="home-explore-{group.id}" class="text-sm font-semibold text-muted">{group.title}</h3>
          <div class="mt-3 grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
            {#each group.widgets as widget (widget.id)}
              <HomeWidgetCard {widget} {state} {onRetry} explore />
            {/each}
          </div>
        </section>
      {/each}
    {:else if open && !failed}
      {#each exploreTitles() as group (group.id)}
        <div aria-hidden="true" class="mt-6">
          <p class="text-sm font-semibold text-muted">{group.title}</p>
          <div class="mt-3 grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
            {#each group.widgets as placeholder (placeholder.id)}
              <div class="flex min-h-52 min-w-0 flex-col rounded-(--radius-panel) border border-dashed border-muted bg-surface px-4 pt-3 pb-2 text-sm">
                <p class="text-sm font-semibold text-muted">{placeholder.title}</p>
                <span class="mt-2 block h-6 w-32 rounded-(--radius-control) bg-surface-strong"></span>
              </div>
            {/each}
          </div>
        </div>
      {/each}
    {/if}
  </div>
</section>
