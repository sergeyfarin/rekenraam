<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import Activity from '@lucide/svelte/icons/activity';
  import Coins from '@lucide/svelte/icons/coins';
  import Database from '@lucide/svelte/icons/database';
  import Languages from '@lucide/svelte/icons/languages';
  import Palette from '@lucide/svelte/icons/palette';
  import ShieldCheck from '@lucide/svelte/icons/shield-check';
  import Clock from '@lucide/svelte/icons/clock';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import packageInfo from '../../../../package.json';
  import { authSessionQueryOptions } from '$lib/api/auth';
  import { currentBookQueryOptions } from '$lib/api/books';
  import { currenciesQueryOptions } from '$lib/api/currencies';
  import { healthQueryOptions } from '$lib/api/health';
  import {
    saveUserPreferences,
    userPreferencesQueryOptions
  } from '$lib/api/settings';
  import { getAPIClientErrorMessage } from '$lib/api-error-messages';
  import Panel from '$lib/components/panel.svelte';
  import StatusBadge from '$lib/components/status-badge.svelte';
  import { m } from '$lib/paraglide/messages.js';

  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const healthQuery = createQuery(() => healthQueryOptions());
  const currentBookQuery = createQuery(() => currentBookQueryOptions());
  const currenciesQuery = createQuery(() => currenciesQueryOptions());
  const preferencesQuery = createQuery(() => userPreferencesQueryOptions());

  let selectedTimeZone = $state('');
  let preferencesMarker = $state('');
  let preferencesPending = $state(false);
  let preferencesError = $state<unknown>(undefined);
  let preferencesSuccess = $state('');

  const browserTimeZone = $derived(Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC');
  const timeZoneOptions = $derived.by(() => {
    if (typeof Intl.supportedValuesOf !== 'function') {
      return [selectedTimeZone || browserTimeZone || 'UTC'];
    }
    const zones = Intl.supportedValuesOf('timeZone');
    return zones.includes('UTC') ? zones : ['UTC', ...zones];
  });

  const defaultCurrency = $derived.by(() => {
    const defaultCurrencyID = currentBookQuery.data?.default_currency_commodity_id;
    return currenciesQuery.data?.currencies.find((currency) => currency.id === defaultCurrencyID);
  });

  const healthLabel = $derived.by(() => {
    if (healthQuery.isPending) {
      return m.settings_status_loading();
    }

    if (healthQuery.isError) {
      return getAPIClientErrorMessage(healthQuery.error);
    }

    return m.settings_health_ok({ status: healthQuery.data?.status ?? 'unknown' });
  });

  const bookLabel = $derived.by(() => {
    if (currentBookQuery.isPending) {
      return m.settings_status_loading();
    }

    if (currentBookQuery.isError) {
      return getAPIClientErrorMessage(currentBookQuery.error);
    }

    return currentBookQuery.data?.name ?? m.settings_status_unknown();
  });

  const defaultCurrencyLabel = $derived.by(() => {
    if (currentBookQuery.isPending || currenciesQuery.isPending) {
      return m.settings_status_loading();
    }

    if (currentBookQuery.isError) {
      return getAPIClientErrorMessage(currentBookQuery.error);
    }

    if (currenciesQuery.isError) {
      return getAPIClientErrorMessage(currenciesQuery.error);
    }

    return defaultCurrency
      ? m.settings_default_currency_value({ code: defaultCurrency.code, name: defaultCurrency.name })
      : m.settings_status_not_set();
  });

  const timeZoneLabel = $derived.by(() => {
    if (preferencesQuery.isPending) {
      return m.settings_status_loading();
    }

    if (preferencesQuery.isError) {
      return getAPIClientErrorMessage(preferencesQuery.error);
    }

    return preferencesQuery.data?.time_zone ?? browserTimeZone;
  });

  const summaryItems = $derived([
    {
      icon: Database,
      label: m.settings_database_health_label(),
      value: healthLabel,
      tone: healthQuery.isError ? 'danger' : 'positive'
    },
    {
      icon: ShieldCheck,
      label: m.settings_book_label(),
      value: bookLabel,
      tone: 'accent'
    },
    {
      icon: Coins,
      label: m.settings_default_currency_label(),
      value: defaultCurrencyLabel,
      tone: 'accent'
    },
    {
      icon: Clock,
      label: m.settings_time_zone_label(),
      value: timeZoneLabel,
      tone: preferencesQuery.isError ? 'danger' : 'accent'
    },
    {
      icon: Activity,
      label: m.settings_app_version_label(),
      value: packageInfo.version,
      tone: 'accent'
    },
    {
      icon: Database,
      label: m.settings_last_backup_label(),
      value: m.settings_last_backup_untracked(),
      tone: 'warning'
    }
  ] as const);

  const sectionLinks = [
    {
      href: '/app/settings/appearance',
      icon: Palette,
      title: m.settings_card_appearance_title(),
      copy: m.settings_card_appearance_copy()
    },
    {
      href: '/app/settings/language',
      icon: Languages,
      title: m.settings_card_language_title(),
      copy: m.settings_card_language_copy()
    },
    {
      href: '/app/settings/currencies',
      icon: Coins,
      title: m.settings_card_currencies_title(),
      copy: m.settings_card_currencies_copy()
    },
    {
      href: '/app/settings/data',
      icon: Database,
      title: m.settings_data_title(),
      copy: m.settings_data_subtitle()
    },
    {
      href: '/app/settings/trash',
      icon: Trash2,
      title: m.settings_card_trash_title(),
      copy: m.settings_card_trash_copy()
    }
  ];

  $effect(() => {
    const marker = [preferencesQuery.data?.id ?? 'missing', preferencesQuery.data?.updated_at ?? ''].join(':');
    if (preferencesMarker === marker || preferencesPending) return;
    preferencesMarker = marker;
    selectedTimeZone = preferencesQuery.data?.time_zone ?? browserTimeZone;
  });

  async function csrfToken() {
    if (sessionQuery.data?.csrf_token) return sessionQuery.data.csrf_token;
    const session = await sessionQuery.refetch();
    const token = session.data?.csrf_token;
    if (!token) throw new Error('CSRF token is unavailable');
    return token;
  }

  async function handleSavePreferences(event: SubmitEvent) {
    event.preventDefault();
    preferencesPending = true;
    preferencesError = undefined;
    preferencesSuccess = '';

    try {
      await saveUserPreferences(
        {
          time_zone: selectedTimeZone,
          change_reason: 'saved workspace preferences'
        },
        await csrfToken()
      );
      preferencesSuccess = m.settings_preferences_saved();
      await preferencesQuery.refetch();
    } catch (error) {
      preferencesError = error;
    } finally {
      preferencesPending = false;
    }
  }
</script>

<section class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]">
  <Panel>
    <h2 class="text-lg font-semibold tracking-tight text-balance">{m.settings_overview_title()}</h2>
    <p class="mt-3 max-w-3xl text-sm leading-6 text-muted">{m.settings_overview_copy()}</p>

    <div class="mt-5 grid gap-px overflow-hidden rounded-(--radius-panel) border border-border bg-border sm:grid-cols-2">
      {#each summaryItems as item}
        {@const Icon = item.icon}
        <article class="bg-surface p-4">
          <div class="flex items-start justify-between gap-3">
            <div class="flex min-w-0 items-center gap-2">
              <Icon size={18} class="shrink-0 text-muted" aria-hidden="true" />
              <p class="text-sm font-semibold text-foreground">{item.label}</p>
            </div>
            <StatusBadge tone={item.tone}>{item.value}</StatusBadge>
          </div>
        </article>
      {/each}
    </div>

    <form class="mt-6 grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end" onsubmit={handleSavePreferences}>
      <label class="block text-sm font-semibold text-foreground">
        {m.settings_time_zone_label()}
        <select
          bind:value={selectedTimeZone}
          class="mt-2 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
          disabled={preferencesPending || preferencesQuery.isPending}
        >
          {#each timeZoneOptions as timeZone}
            <option value={timeZone}>{timeZone}</option>
          {/each}
        </select>
      </label>
      <button
        type="submit"
        class="rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
        disabled={preferencesPending || selectedTimeZone === ''}
      >
        {preferencesPending ? m.settings_preferences_saving() : m.settings_preferences_save()}
      </button>
    </form>
    {#if preferencesSuccess}
      <p class="mt-3 text-sm font-semibold text-positive">{preferencesSuccess}</p>
    {/if}
    {#if preferencesError}
      <p class="mt-3 text-sm font-semibold text-danger">{getAPIClientErrorMessage(preferencesError)}</p>
    {/if}
  </Panel>

  <Panel variant="toolbar">
    <h2 class="text-lg font-semibold tracking-tight text-balance">{m.settings_recommended_title()}</h2>
    <div class="mt-5 space-y-3">
      {#each sectionLinks as link}
        {@const Icon = link.icon}
        <a
          href={link.href}
          class="block rounded-(--radius-panel) border border-border bg-surface p-4 transition hover:bg-row-hover"
        >
          <div class="flex items-center gap-2">
            <Icon size={18} class="text-accent" aria-hidden="true" />
            <p class="text-sm font-semibold text-foreground">{link.title}</p>
          </div>
          <p class="mt-2 text-sm leading-6 text-muted">{link.copy}</p>
        </a>
      {/each}
    </div>
  </Panel>
</section>
