<script lang="ts">
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale, locales, setLocale, type Locale } from '#lib/paraglide/runtime.js';
  import { localeAutonym } from '#lib/settings/locales.ts';
  import ThemeChoice from './theme-choice.svelte';

  const currentLocale = getLocale();

  function selectLocale(event: Event) {
    const locale = (event.currentTarget as HTMLSelectElement).value as Locale;
    // Reloads the document, as the language settings page does.
    if (locale !== currentLocale) setLocale(locale);
  }
</script>

<!-- Sign-in, setup and recovery: the name, language and theme, nothing that
     needs a session (#436). -->
<header data-print-hide class="mx-auto flex max-w-[100rem] flex-wrap items-center gap-3 px-4 py-3 sm:px-6 lg:px-8">
  <a href="/" class="mr-auto text-base font-semibold tracking-tight text-foreground">{m.app_name()}</a>

  <label class="flex items-center gap-2 text-sm text-muted">
    <span class="sr-only">{m.language_title()}</span>
    <select
      value={currentLocale}
      onchange={selectLocale}
      class="h-10 rounded-(--radius-control) border border-border bg-control px-2 text-sm text-foreground"
    >
      {#each locales as locale (locale)}
        <option value={locale} lang={locale}>{localeAutonym(locale)}</option>
      {/each}
    </select>
  </label>

  <ThemeChoice />
</header>
