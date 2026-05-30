<script lang="ts">
  import { MoonStar, SunMedium } from '@lucide/svelte';
  import { m } from '$lib/paraglide/messages.js';
  import { themeState, toggleTheme } from '$lib/theme.svelte';

  const currentThemeLabel = $derived(themeState.name === 'light' ? m.theme_name_light() : m.theme_name_dark());
  const switchThemeLabel = $derived(themeState.name === 'light' ? m.theme_switch_to_dark() : m.theme_switch_to_light());
</script>

<button
  type="button"
  class="inline-flex items-center gap-3 rounded-full border border-border/80 bg-surface/90 px-3 py-2 text-left shadow-[var(--shadow-panel)] backdrop-blur transition-transform duration-150 hover:-translate-y-0.5"
  aria-label={switchThemeLabel}
  title={switchThemeLabel}
  onclick={toggleTheme}
>
  <span class="flex size-10 items-center justify-center rounded-full bg-surface-strong text-accent">
    {#if themeState.name === 'light'}
      <SunMedium class="size-4" aria-hidden="true" />
    {:else}
      <MoonStar class="size-4" aria-hidden="true" />
    {/if}
  </span>

  <span class="min-w-0">
    <span class="block text-[0.65rem] font-semibold uppercase tracking-[0.22em] text-muted">
      {m.theme_label()}
    </span>
    <span class="block text-sm font-medium text-foreground">
      {currentThemeLabel}
    </span>
  </span>
</button>