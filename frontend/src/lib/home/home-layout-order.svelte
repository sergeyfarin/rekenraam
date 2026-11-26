<script lang="ts" generics="Item extends string">
  import GripVertical from '@lucide/svelte/icons/grip-vertical';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import ChevronUp from '@lucide/svelte/icons/chevron-up';
  import { tick, type Snippet } from 'svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { focusHomeControl } from './layout-model.ts';

  let { id, items, onMove, children }: {
    id: string; items: readonly Item[]; onMove: (from: number, to: number) => void; children: Snippet<[Item]>;
  } = $props();
  let dragging = $state<number | null>(null);

  async function move(from: number, to: number) {
    if (to < 0 || to >= items.length || from === to) return;
    const item = items[from];
    onMove(from, to);
    await tick();
    focusHomeControl(`${id}-${item}`);
  }

  function startDrag(event: DragEvent, index: number) {
    dragging = index;
    event.dataTransfer?.setData('text/plain', id);
  }

  function allowDrop(event: DragEvent) {
    if (dragging !== null) event.preventDefault();
  }

  function drop(event: DragEvent, index: number) {
    if (dragging === null) return;
    event.preventDefault();
    const from = dragging;
    dragging = null;
    void move(from, index);
  }
</script>

<ul class="grid gap-2">
  {#each items as item, index (item)}
    <li id="{id}-{item}" tabindex="-1" ondragover={allowDrop} ondrop={(event) => drop(event, index)}
      class="flex min-w-0 items-center gap-1 rounded-(--radius-control) border border-border p-1 focus-visible:outline-2 focus-visible:outline-accent">
      <span aria-hidden="true" draggable="true" ondragstart={(event) => startDrag(event, index)} ondragend={() => (dragging = null)}
        class="hidden cursor-grab touch-none p-1 text-muted lg:[@media(pointer:fine)]:block"><GripVertical size={16} /></span>
      <div class="min-w-0 flex-1">{@render children(item)}</div>
      <div class="flex shrink-0">
        <button type="button" aria-label={m.home_layout_move_up()} disabled={index === 0} onclick={() => move(index, index - 1)}
          class="grid size-11 place-items-center rounded-(--radius-control) hover:bg-control-hover disabled:opacity-40"><ChevronUp size={17} aria-hidden="true" /></button>
        <button type="button" aria-label={m.home_layout_move_down()} disabled={index === items.length - 1} onclick={() => move(index, index + 1)}
          class="grid size-11 place-items-center rounded-(--radius-control) hover:bg-control-hover disabled:opacity-40"><ChevronDown size={17} aria-hidden="true" /></button>
      </div>
    </li>
  {/each}
</ul>
