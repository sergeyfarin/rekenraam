<script lang="ts">
  import Check from '@lucide/svelte/icons/check';
  import Languages from '@lucide/svelte/icons/languages';
  import { m } from '$lib/paraglide/messages.js';
  import { getLocale, locales, setLocale, type Locale } from '$lib/paraglide/runtime.js';
  import { localeAutonym } from '$lib/settings/locales';
  import Panel from '$lib/components/panel.svelte';
  import StatusBadge from '$lib/components/status-badge.svelte';

  const currentLocale = getLocale();

  function selectLocale(locale: Locale) {
    if (locale === currentLocale) {
      return;
    }

    // Reloads the document: every message was already resolved during render,
    // so switching in place would leave the previous locale on screen.
    setLocale(locale);
  }
</script>

<section class="grid gap-4">
  <Panel>
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <StatusBadge tone="accent">{localeAutonym(currentLocale)}</StatusBadge>
        <h2 class="mt-3 text-lg font-semibold tracking-tight text-balance">{m.language_title()}</h2>
        <p class="mt-2 max-w-prose text-sm text-muted">{m.language_copy()}</p>
      </div>

      <Languages size={18} class="text-muted" aria-hidden="true" />
    </div>

    <fieldset class="mt-6">
      <legend class="text-sm font-semibold text-foreground">{m.language_select_legend()}</legend>
      <div class="mt-3 grid gap-2 sm:grid-cols-2">
        {#each locales as locale (locale)}
          <button
            type="button"
            lang={locale}
            aria-pressed={locale === currentLocale}
            class:border-accent={locale === currentLocale}
            class:bg-control-hover={locale === currentLocale}
            class="flex items-center justify-between gap-3 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-left text-sm font-semibold text-foreground transition hover:bg-control-hover"
            onclick={() => selectLocale(locale)}
          >
            <span class="min-w-0 truncate">{localeAutonym(locale)}</span>
            {#if locale === currentLocale}
              <Check size={16} class="shrink-0 text-accent" aria-hidden="true" />
            {/if}
          </button>
        {/each}
      </div>
    </fieldset>
  </Panel>

  {#if currentLocale !== 'en'}
    <Panel variant="toolbar">
      <h3 class="text-sm font-semibold text-foreground">{m.language_translation_progress_title()}</h3>
      <p class="mt-2 max-w-prose text-sm text-muted">{m.language_translation_progress_copy()}</p>
    </Panel>
  {/if}
</section>
