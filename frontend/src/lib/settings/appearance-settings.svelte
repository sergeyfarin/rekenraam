<script lang="ts">
  import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
  import SwatchBook from '@lucide/svelte/icons/swatch-book';
  import { m } from '$lib/paraglide/messages.js';
  import {
    resetAppearance,
    setAccentColor,
    setBaseColor,
    themeState,
    type AccentColorName,
    type BaseColorName
  } from '$lib/theme.svelte';
  import Panel from '$lib/components/panel.svelte';
  import StatusBadge from '$lib/components/status-badge.svelte';

  type BaseOption = {
    name: BaseColorName;
    label: string;
    swatch: string;
  };

  type AccentOption = {
    name: AccentColorName;
    label: string;
    swatch: string;
  };

  const baseOptions: BaseOption[] = [
    { name: 'olive', label: m.appearance_base_olive(), swatch: 'oklch(0.76 0.06 108)' },
    { name: 'slate', label: m.appearance_base_slate(), swatch: 'oklch(0.72 0.04 250)' },
    { name: 'zinc', label: m.appearance_base_zinc(), swatch: 'oklch(0.74 0.025 286)' }
  ];

  const accentOptions: AccentOption[] = [
    { name: 'emerald', label: m.appearance_accent_emerald(), swatch: 'oklch(0.56 0.14 154)' },
    { name: 'blue', label: m.appearance_accent_blue(), swatch: 'oklch(0.58 0.15 245)' },
    { name: 'violet', label: m.appearance_accent_violet(), swatch: 'oklch(0.57 0.15 300)' },
    { name: 'rose', label: m.appearance_accent_rose(), swatch: 'oklch(0.56 0.16 18)' },
    { name: 'amber', label: m.appearance_accent_amber(), swatch: 'oklch(0.62 0.14 78)' }
  ];
</script>

<section class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]">
  <Panel>
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <StatusBadge tone="accent">{m.appearance_preset_name()}</StatusBadge>
        <h2 class="mt-3 text-lg font-semibold tracking-tight text-balance">{m.appearance_title()}</h2>
      </div>

      <button
        type="button"
        class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
        onclick={resetAppearance}
      >
        <RotateCcw size={16} aria-hidden="true" />
        {m.appearance_reset()}
      </button>
    </div>

    <div class="mt-6 grid gap-6 md:grid-cols-2">
      <fieldset>
        <legend class="text-sm font-semibold text-foreground">{m.appearance_base_label()}</legend>
        <div class="mt-3 grid gap-2">
          {#each baseOptions as option}
            <button
              type="button"
              aria-pressed={themeState.baseColor === option.name}
              class:border-accent={themeState.baseColor === option.name}
              class:bg-control-hover={themeState.baseColor === option.name}
              class="flex items-center justify-between gap-3 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-left text-sm font-semibold text-foreground transition hover:bg-control-hover"
              onclick={() => setBaseColor(option.name)}
            >
              <span class="flex min-w-0 items-center gap-3">
                <span
                  class="size-5 shrink-0 rounded-full border border-border"
                  style={`background: ${option.swatch}`}
                  aria-hidden="true"
                ></span>
                <span>{option.label}</span>
              </span>
            </button>
          {/each}
        </div>
      </fieldset>

      <fieldset>
        <legend class="text-sm font-semibold text-foreground">{m.appearance_accent_label()}</legend>
        <div class="mt-3 grid gap-2">
          {#each accentOptions as option}
            <button
              type="button"
              aria-pressed={themeState.accentColor === option.name}
              class:border-accent={themeState.accentColor === option.name}
              class:bg-control-hover={themeState.accentColor === option.name}
              class="flex items-center justify-between gap-3 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-left text-sm font-semibold text-foreground transition hover:bg-control-hover"
              onclick={() => setAccentColor(option.name)}
            >
              <span class="flex min-w-0 items-center gap-3">
                <span
                  class="size-5 shrink-0 rounded-full border border-border"
                  style={`background: ${option.swatch}`}
                  aria-hidden="true"
                ></span>
                <span>{option.label}</span>
              </span>
            </button>
          {/each}
        </div>
      </fieldset>
    </div>
  </Panel>

  <Panel variant="toolbar">
    <div class="flex items-center gap-2">
      <SwatchBook size={18} class="text-muted" aria-hidden="true" />
      <h2 class="text-lg font-semibold tracking-tight text-balance">{m.appearance_preview_title()}</h2>
    </div>

    <div class="mt-5 space-y-3">
      <div class="rounded-(--radius-panel) border border-border bg-surface p-4">
        <p class="text-sm font-semibold text-foreground">{m.appearance_preview_surface()}</p>
        <div class="mt-3 flex flex-wrap gap-2">
          <span class="rounded-(--radius-control) bg-accent px-2.5 py-1 text-xs font-semibold text-accent-foreground">
            {m.appearance_preview_accent()}
          </span>
          <span class="rounded-(--radius-control) bg-selected px-2.5 py-1 text-xs font-semibold text-selected-foreground">
            {m.appearance_preview_selected()}
          </span>
        </div>
      </div>

      <div class="grid grid-cols-5 overflow-hidden rounded-(--radius-panel) border border-border">
        <div class="h-12 bg-chart-1" aria-label={m.appearance_chart_1()}></div>
        <div class="h-12 bg-chart-2" aria-label={m.appearance_chart_2()}></div>
        <div class="h-12 bg-chart-3" aria-label={m.appearance_chart_3()}></div>
        <div class="h-12 bg-chart-4" aria-label={m.appearance_chart_4()}></div>
        <div class="h-12 bg-chart-5" aria-label={m.appearance_chart_5()}></div>
      </div>
    </div>
  </Panel>
</section>
