<script lang="ts">
  import { tick } from 'svelte';
  import type { HomeLayoutState } from '#lib/api/home.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import HomeItemActions from './home-item-actions.svelte';
  import { focusHomeControl, moveHomeItem, wideWidgets } from './layout-model.ts';
  import { useHomePersonalization } from './personalization.svelte.ts';
  import type { WidgetView } from './widgets.ts';

  let { widget, state, explore = false }: { widget: WidgetView; state: HomeLayoutState; explore?: boolean } = $props();
  const personalization = useHomePersonalization();
  const index = $derived(state.layout.widgets.indexOf(widget.id));
  const hasSettings = $derived(wideWidgets.includes(widget.id));
  const triggerID = $derived(`${explore ? 'explore' : 'home'}-actions-${widget.id}`);

  async function arrange(action: 'add' | 'top' | 'remove' | 'move', offset = 0) {
    const ids = state.layout.widgets;
    const to = index + offset;
    const widgets = action === 'add' ? [...ids, widget.id]
      : action === 'top' ? [widget.id, ...ids]
      : action === 'remove' ? ids.filter(id => id !== widget.id) : moveHomeItem(ids, index, to);
    const result = await personalization.save(state, {
      ...state.layout, widgets,
      explore_open: action === 'remove' ? true : state.layout.explore_open
    });
    if (result.status !== 'saved') return;
    await tick();
    focusHomeControl(`${action === 'remove' ? 'explore' : 'home'}-actions-${widget.id}`);
    if (action === 'move') {
      const format = new Intl.NumberFormat(getLocale());
      personalization.announce(m.home_layout_moved({ item: widget.title, position: format.format(to + 1), total: format.format(widgets.length) }));
    }
  }

  const actionClass = 'flex min-h-11 w-full items-center rounded-(--radius-control) px-2 text-left hover:bg-control-hover disabled:opacity-50';
</script>

<HomeItemActions id={triggerID} label={widget.title}>
  {#snippet children(close)}
    {#if explore}
      <button type="button" class={actionClass} disabled={personalization.pending} onclick={() => { close(); void arrange('add'); }}>{m.home_layout_add_widget()}</button>
      <button type="button" class={actionClass} disabled={personalization.pending} onclick={() => { close(); void arrange('top'); }}>{m.home_layout_add_top()}</button>
    {:else}
      <button type="button" class={actionClass} disabled={personalization.pending || index <= 0} onclick={() => { close(); void arrange('move', -1); }}>{m.home_layout_move_up()}</button>
      <button type="button" class={actionClass} disabled={personalization.pending || index < 0 || index === state.layout.widgets.length - 1} onclick={() => { close(); void arrange('move', 1); }}>{m.home_layout_move_down()}</button>
      <button type="button" class={actionClass} disabled={personalization.pending} onclick={() => { close(); void arrange('remove'); }}>{m.home_layout_remove()}</button>
    {/if}
    {#if hasSettings}
      <button type="button" class={actionClass} disabled={personalization.pending} onclick={() => { close(); personalization.open(widget.id); }}>{m.home_layout_settings()}</button>
    {/if}
    <a href={widget.open.href} class={actionClass}>{widget.open.text}</a>
  {/snippet}
</HomeItemActions>

{#if explore}
  <button type="button" disabled={personalization.pending} onclick={() => arrange('add')}
    class="min-h-11 rounded-(--radius-control) px-1 text-left font-medium text-accent underline-offset-4 hover:underline disabled:opacity-50">{m.home_layout_add_widget()}</button>
{/if}
