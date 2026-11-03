<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import Activity from '@lucide/svelte/icons/activity';
  import Clock from '@lucide/svelte/icons/clock';
  import Coins from '@lucide/svelte/icons/coins';
  import Database from '@lucide/svelte/icons/database';
  import packageInfo from '../../../package.json';
  import { authSessionQueryOptions } from '#lib/api/auth.ts';
  import { currentBookQueryOptions } from '#lib/api/books.ts';
  import { currenciesQueryOptions } from '#lib/api/currencies.ts';
  import { backupStatusQueryOptions } from '#lib/api/maintenance.ts';
  import { saveUserPreferences, userPreferencesQueryOptions } from '#lib/api/settings.ts';
  import { getAPIClientErrorMessage } from '#lib/api-error-messages.ts';
  import Panel from '#lib/components/panel.svelte';
  import StatusBadge from '#lib/components/status-badge.svelte';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { m } from '#lib/paraglide/messages.js';

  // General is the facts page (#446): default currency, time zone, app version
  // and the last complete backup. Links to the focused settings pages live in
  // the side list and the settings menu, never as cards here.
  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const currentBookQuery = createQuery(() => currentBookQueryOptions());
  const currenciesQuery = createQuery(() => currenciesQueryOptions());
  const preferencesQuery = createQuery(() => userPreferencesQueryOptions());
  const backupQuery = createQuery(() => backupStatusQueryOptions());
  const locale = $derived(getLocale());

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

  const lastBackupLabel = $derived.by(() => {
    if (backupQuery.isPending) {
      return m.settings_status_loading();
    }
    if (backupQuery.isError) {
      return getAPIClientErrorMessage(backupQuery.error);
    }
    const finishedAt = backupQuery.data?.last_success?.finished_at;
    if (!finishedAt) {
      return m.settings_data_backups_never();
    }
    const parsed = new Date(finishedAt);
    return Number.isNaN(parsed.getTime())
      ? finishedAt
      : new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(parsed);
  });

  const summaryItems = $derived([
    {
      icon: Coins,
      label: m.settings_default_currency_label(),
      value: defaultCurrencyLabel,
      tone: currentBookQuery.isError || currenciesQuery.isError ? 'danger' : 'accent'
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
      value: lastBackupLabel,
      tone: backupQuery.isError ? 'danger' : backupQuery.data?.last_success ? 'positive' : 'warning'
    }
  ] as const);

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
      await Promise.all([preferencesQuery.refetch(), sessionQuery.refetch()]);
    } catch (error) {
      preferencesError = error;
    } finally {
      preferencesPending = false;
    }
  }
</script>

<section class="grid gap-4" data-testid="general-settings">
  <Panel>
    <h2 class="text-lg font-semibold tracking-tight text-balance">{m.settings_nav_general()}</h2>
    <p class="mt-3 max-w-3xl text-sm leading-6 text-muted">{m.settings_general_copy()}</p>

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
      <p role="status" class="mt-3 text-sm font-semibold text-positive">{preferencesSuccess}</p>
    {/if}
    {#if preferencesError}
      <p role="alert" class="mt-3 text-sm font-semibold text-danger">{getAPIClientErrorMessage(preferencesError)}</p>
    {/if}
  </Panel>
</section>
