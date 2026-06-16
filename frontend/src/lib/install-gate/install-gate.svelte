<script lang="ts">
  import { browser } from '$app/environment';
  import { goto } from '$app/navigation';
  import { createQuery } from '@tanstack/svelte-query';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import { authSessionQueryOptions, login } from '$lib/api/auth';
  import { completeCurrencySetup, currencyCatalogQueryOptions } from '$lib/api/currencies';
  import { healthQueryOptions } from '$lib/api/health';
  import { createBook, createOwner, setupStatusQueryOptions } from '$lib/api/setup';
  import { getAPIClientErrorMessage } from '$lib/api-error-messages';
  import { m } from '$lib/paraglide/messages.js';
  import AuthenticatedPanel from './authenticated-panel.svelte';
  import CurrencySetupForm from './currency-setup-form.svelte';
  import InstallGateHero from './install-gate-hero.svelte';
  import LoginForm from './login-form.svelte';
  import OwnerSetupForm from './owner-setup-form.svelte';
  import RecoveryPanel from './recovery-panel.svelte';
  import WorkspacePreparingPanel from './workspace-preparing-panel.svelte';
  import { localeCurrencyCode, localizedCurrencyCatalog, quickCurrencyCodes } from './currency-options';
  import { installGateStateCopy } from './install-gate-copy';
  import {
    resolveInstallGateState,
    shouldCreateDefaultBook,
    shouldRedirectToApp
  } from './install-gate-state';

  const healthQuery = createQuery(() => healthQueryOptions());
  const setupQuery = createQuery(() => setupStatusQueryOptions());
  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const currencyCatalogQuery = createQuery(() => ({
    ...currencyCatalogQueryOptions(),
    enabled: sessionQuery.data?.authenticated === true
  }));

  let ownerUsername = $state('');
  let ownerPassword = $state('');
  let loginUsername = $state('');
  let loginPassword = $state('');
  let currencyDefaultCode = $state('');
  let currencySearchCode = $state('');
  let ownerError = $state<unknown>(undefined);
  let loginError = $state<unknown>(undefined);
  let bookError = $state<unknown>(undefined);
  let currencyError = $state<unknown>(undefined);
  let ownerPending = $state(false);
  let loginPending = $state(false);
  let bookPending = $state(false);
  let currencyPending = $state(false);
  let redirectingToApp = $state(false);

  const healthState = $derived.by<'loading' | 'success' | 'error'>(() => {
    if (healthQuery.isPending) return 'loading';
    if (healthQuery.isError) return 'error';
    return 'success';
  });

  const healthMessage = $derived.by(() => {
    if (healthQuery.isPending) return m.home_health_checking();
    if (healthQuery.isError) return getAPIClientErrorMessage(healthQuery.error);
    return m.home_health_status({ status: healthQuery.data?.status ?? 'unknown' });
  });

  const healthStateLabel = $derived.by(() => {
    if (healthState === 'loading') return m.home_health_state_loading();
    if (healthState === 'error') return m.home_health_state_error();
    return m.home_health_state_success();
  });

  const browserTimeZone = $derived.by(() => {
    if (!browser) return 'UTC';
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  });

  const currencyDisplayNames = $derived.by(() => {
    if (!browser || typeof Intl.DisplayNames === 'undefined') return null;
    return new Intl.DisplayNames(undefined, { type: 'currency' });
  });

  const catalog = $derived(localizedCurrencyCatalog(currencyCatalogQuery.data?.currencies ?? [], currencyDisplayNames));
  const localeCode = $derived(localeCurrencyCode(browser ? navigator.languages : ['en-US']));
  const quickCodes = $derived(quickCurrencyCodes(catalog, localeCode));
  const installGateError = $derived(setupQuery.error ?? sessionQuery.error ?? currencyCatalogQuery.error);

  const pageState = $derived.by(() =>
    resolveInstallGateState({
      setupPending: setupQuery.isPending,
      setupError: setupQuery.isError,
      setup: setupQuery.data,
      sessionPending: sessionQuery.isPending,
      sessionError: sessionQuery.isError,
      session: sessionQuery.data
    })
  );

  const stateCopy = $derived(installGateStateCopy(pageState));

  const completedSteps = $derived(setupQuery.data?.steps.filter((step) => step.status === 'completed').length ?? 0);
  const totalSteps = $derived(setupQuery.data?.steps.length ?? 0);
  const nextStepLabel = $derived.by(() => {
    const nextStep = setupQuery.data?.current_step;

    if (!nextStep || nextStep === 'book') return null;
    return m.install_gate_next_step({ step: nextStep.replaceAll('_', ' ') });
  });

  $effect(() => {
    if (!shouldRedirectToApp(browser, pageState)) {
      redirectingToApp = false;
      return;
    }

    redirectingToApp = true;
    void goto('/app');
  });

  $effect(() => {
    if (catalog.length === 0) return;

    const catalogCodes = new Set(catalog.map((currency) => currency.code));
    const fallbackCode = [localeCode, 'USD', catalog[0]?.code].find((code) => code !== undefined && catalogCodes.has(code));
    if (!currencyDefaultCode || !catalogCodes.has(currencyDefaultCode)) {
      currencyDefaultCode = fallbackCode ?? '';
    }
  });

  $effect(() => {
    if (!shouldCreateDefaultBook({ browser, state: pageState, pending: bookPending, error: bookError })) return;
    void createDefaultBook();
  });

  async function refreshInstallGate() {
    const refreshes: Promise<unknown>[] = [setupQuery.refetch(), sessionQuery.refetch(), healthQuery.refetch()];

    if (sessionQuery.data?.authenticated === true) {
      refreshes.push(currencyCatalogQuery.refetch());
    }

    await Promise.all(refreshes);
  }

  async function handleCreateOwner(event: SubmitEvent) {
    event.preventDefault();
    ownerPending = true;
    ownerError = undefined;

    try {
      const result = await createOwner({
        username: ownerUsername,
        password: ownerPassword,
        time_zone: browserTimeZone
      });
      loginUsername = result.owner.username;
      ownerPassword = '';
      await refreshInstallGate();
    } catch (error) {
      ownerError = error;
    } finally {
      ownerPending = false;
    }
  }

  async function handleLogin(event: SubmitEvent) {
    event.preventDefault();
    loginPending = true;
    loginError = undefined;

    try {
      await login({ username: loginUsername, password: loginPassword });
      loginPassword = '';
      await refreshInstallGate();
    } catch (error) {
      loginError = error;
    } finally {
      loginPending = false;
    }
  }

  async function createDefaultBook() {
    bookPending = true;
    bookError = undefined;

    try {
      await createBook({ code: 'personal', name: 'personal' }, sessionQuery.data?.csrf_token ?? '');
      await refreshInstallGate();
    } catch (error) {
      bookError = error;
    } finally {
      bookPending = false;
    }
  }

  async function handleCompleteCurrencySetup(event: SubmitEvent) {
    event.preventDefault();
    currencyPending = true;
    currencyError = undefined;

    try {
      await completeCurrencySetup(
        {
          default_currency_code: currencyDefaultCode
        },
        sessionQuery.data?.csrf_token ?? ''
      );

      await refreshInstallGate();
    } catch (error) {
      currencyError = error;
    } finally {
      currencyPending = false;
    }
  }

</script>

<main class="min-h-screen px-6 py-16 sm:px-10">
  <section class="mx-auto grid max-w-6xl gap-8 lg:grid-cols-[1.1fr_0.9fr] lg:items-start">
    <InstallGateHero
      {completedSteps}
      {totalSteps}
      {nextStepLabel}
      {healthState}
      {healthStateLabel}
      {healthMessage}
    />

    <div class="rounded-[2rem] border border-border/80 bg-surface/95 p-6 shadow-[var(--shadow-panel)] backdrop-blur sm:p-8">
      <div class="mb-6 flex items-center justify-between gap-4">
        <div class="space-y-2">
          <span class="inline-flex rounded-full bg-surface-strong px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] text-muted">
            {stateCopy.badge}
          </span>

          <h2 class="text-3xl font-semibold tracking-tight text-balance">{stateCopy.title}</h2>
          <p class="text-sm leading-6 text-muted">{stateCopy.copy}</p>
        </div>

        <div class="hidden h-16 w-16 rounded-[1.5rem] border border-border bg-surface-strong/70 sm:block"></div>
      </div>

      {#if pageState === 'loading'}
        <div class="rounded-[1.75rem] border border-border bg-surface-strong/60 p-5 text-sm leading-6 text-muted">
          {m.install_gate_loading_copy()}
        </div>
      {:else if pageState === 'error'}
        <div class="space-y-4">
          <APIFormError error={installGateError} id="install-gate-error" />
          <button
            type="button"
            class="inline-flex items-center rounded-full bg-foreground px-5 py-3 text-sm font-semibold text-background transition hover:opacity-90"
            onclick={refreshInstallGate}
          >
            {m.install_gate_retry()}
          </button>
        </div>
      {:else if pageState === 'fresh'}
        <OwnerSetupForm
          bind:username={ownerUsername}
          bind:password={ownerPassword}
          error={ownerError}
          pending={ownerPending}
          onsubmit={handleCreateOwner}
        />
      {:else if pageState === 'login'}
        <LoginForm
          bind:username={loginUsername}
          bind:password={loginPassword}
          error={loginError}
          pending={loginPending}
          onsubmit={handleLogin}
        />
      {:else if pageState === 'book_setup'}
        <WorkspacePreparingPanel error={bookError} pending={bookPending} onretry={createDefaultBook} />
      {:else if pageState === 'currency_setup'}
        <CurrencySetupForm
          error={currencyError}
          catalogError={currencyCatalogQuery.error}
          catalogPending={currencyCatalogQuery.isPending}
          {catalog}
          quickCodes={quickCodes}
          bind:defaultCode={currencyDefaultCode}
          bind:searchCode={currencySearchCode}
          pending={currencyPending}
          onsubmit={handleCompleteCurrencySetup}
        />
      {:else if pageState === 'authenticated'}
        <AuthenticatedPanel username={sessionQuery.data?.user?.username ?? ''} redirecting={redirectingToApp} />
      {:else}
        <RecoveryPanel />
      {/if}
    </div>
  </section>
</main>
