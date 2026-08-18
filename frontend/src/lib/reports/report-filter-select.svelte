<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import { toggleID, type FilterOption } from './report-filters';

  let {
    label,
    options,
    selected,
    onChange,
    pending = false,
    error = false,
    onRetry,
    emptyCopy
  }: {
    label: string;
    options: FilterOption[];
    selected: number[];
    onChange: (ids: number[]) => void;
    pending?: boolean;
    error?: boolean;
    onRetry?: () => void;
    /** Shown when the dimension has nothing to select yet — no accounts, no payees. */
    emptyCopy: string;
  } = $props();

  let search = $state('');

  // A short list is faster to scan than to search; the box only earns its space
  // once scanning stops being realistic.
  const searchable = $derived(options.length > 8);
  const visibleOptions = $derived.by(() => {
    const needle = search.trim().toLocaleLowerCase();
    if (!searchable || needle === '') {
      return options;
    }
    return options.filter((option) => option.label.toLocaleLowerCase().includes(needle));
  });

  const selectedCount = $derived(selected.length);
  const controlClass =
    'h-10 w-full rounded-(--radius-control) border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted hover:bg-control-hover focus:border-accent';
</script>

<!-- self-start keeps a closed control at its natural height instead of
     stretching it to match an expanded sibling in the same grid row. -->
<details class="group self-start rounded-(--radius-control) border border-border bg-control">
  <summary
    class="flex h-10 cursor-pointer list-none items-center justify-between gap-2 rounded-(--radius-control) px-3 text-sm font-medium text-foreground transition hover:bg-control-hover focus-visible:outline-none"
  >
    <span>{label}</span>
    <span class="flex items-center gap-2">
      {#if selectedCount > 0}
        <!-- The count is a non-color cue that this dimension is narrowing the report. -->
        <span class="rounded-full bg-accent px-2 py-0.5 text-xs font-semibold text-accent-foreground">
          {selectedCount}
        </span>
      {/if}
      <span aria-hidden="true" class="text-muted transition group-open:rotate-180">▾</span>
    </span>
  </summary>

  <div class="border-t border-border px-3 py-3">
    {#if pending}
      <p class="text-sm text-muted">{m.reports_filter_options_loading()}</p>
    {:else if error}
      <p class="text-sm text-foreground">{m.reports_filter_options_error()}</p>
      {#if onRetry}
        <button
          type="button"
          onclick={() => onRetry?.()}
          class="mt-2 rounded-(--radius-control) border border-border bg-control px-3 py-1.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
        >
          {m.reports_retry()}
        </button>
      {/if}
    {:else if options.length === 0}
      <p class="text-sm text-muted">{emptyCopy}</p>
    {:else}
      {#if searchable}
        <label class="block">
          <span class="sr-only">{m.reports_filter_search({ dimension: label })}</span>
          <input
            type="search"
            bind:value={search}
            placeholder={m.reports_filter_search_placeholder()}
            class={controlClass}
          />
        </label>
      {/if}

      <fieldset class="mt-3 max-h-56 space-y-1 overflow-y-auto">
        <legend class="sr-only">{label}</legend>
        {#each visibleOptions as option (option.id)}
          <label class="flex cursor-pointer items-center gap-2 rounded-(--radius-control) px-1.5 py-1 text-sm text-foreground transition hover:bg-control-hover">
            <input
              type="checkbox"
              class="size-4 accent-[var(--color-accent)]"
              checked={selected.includes(option.id)}
              onchange={() => onChange(toggleID(selected, option.id))}
            />
            <span class="min-w-0 truncate">{option.label}</span>
          </label>
        {/each}
        {#if visibleOptions.length === 0}
          <p class="px-1.5 py-1 text-sm text-muted">{m.reports_filter_search_empty()}</p>
        {/if}
      </fieldset>

      {#if selectedCount > 0}
        <button
          type="button"
          onclick={() => onChange([])}
          class="mt-3 text-sm font-semibold text-accent underline underline-offset-2 transition hover:opacity-80"
        >
          {m.reports_filter_clear_dimension()}
        </button>
      {/if}
    {/if}
  </div>
</details>

<style>
  /* Safari still paints a disclosure triangle without this. */
  summary::-webkit-details-marker {
    display: none;
  }
</style>
