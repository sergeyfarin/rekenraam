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
  let bookCode = $state('personal');
  let bookName = $state('');
  let currencyDefaultCode = $state('USD');
  let additionalCurrencyCodes = $state<string[]>([]);
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
    if (healthQuery.isPending) {
      return 'loading';
    }

    if (healthQuery.isError) {
      return 'error';
    }

    return 'success';
  });

  const healthMessage = $derived.by(() => {
    if (healthQuery.isPending) {
      return m.home_health_checking();
    }

    if (healthQuery.isError) {
      return getAPIClientErrorMessage(healthQuery.error);
    }

    return m.home_health_status({ status: healthQuery.data?.status ?? 'unknown' });
  });

  const healthStateLabel = $derived.by(() => {
    if (healthState === 'loading') {
      return m.home_health_state_loading();
    }

    if (healthState === 'error') {
      return m.home_health_state_error();
    }

    return m.home_health_state_success();
  });

  const currencyDisplayNames = $derived.by(() => {
    if (!browser || typeof Intl.DisplayNames === 'undefined') {
      return null;
    }

    return new Intl.DisplayNames(undefined, { type: 'currency' });
  });

  const localizedCurrencyCatalog = $derived.by(() =>
    (currencyCatalogQuery.data?.currencies ?? []).map((currency) => {
      const name = currencyDisplayNames?.of(currency.code) ?? currency.english_name;

      return {
        ...currency,
        name,
        label: `${currency.code} - ${name}`
      };
    })
  );

  const currencyNamesByCode = $derived.by(
    () => new Map(localizedCurrencyCatalog.map((currency) => [currency.code, currency.name]))
  );

  const installGateError = $derived(setupQuery.error ?? sessionQuery.error ?? currencyCatalogQuery.error);

  const pageState = $derived.by<
    'loading' | 'error' | 'fresh' | 'login' | 'book_setup' | 'currency_setup' | 'authenticated' | 'recovery_required'
  >(() => {
    if (setupQuery.isPending || sessionQuery.isPending) {
      return 'loading';
    }

    if (setupQuery.isError || sessionQuery.isError) {
      return 'error';
    }

    if (setupQuery.data === undefined || sessionQuery.data === undefined) {
      return 'loading';
    }

    if (setupQuery.data.install_state === 'fresh') {
      return 'fresh';
    }

    if (setupQuery.data.install_state === 'recovery_required') {
      return 'recovery_required';
    }

    if (!sessionQuery.data.authenticated) {
      return 'login';
    }

    if (setupQuery.data.current_step === 'book') {
      return 'book_setup';
    }

    if (setupQuery.data.current_step === 'currencies') {
      return 'currency_setup';
    }

    return 'authenticated';
  });

  const stateBadge = $derived.by(() => {
    switch (pageState) {
      case 'fresh':
        return m.install_gate_state_fresh();
      case 'login':
        return m.install_gate_state_configured();
      case 'book_setup':
        return m.install_gate_state_book();
      case 'currency_setup':
        return m.install_gate_state_currencies();
      case 'authenticated':
        return m.install_gate_state_authenticated();
      case 'recovery_required':
        return m.install_gate_state_recovery();
      case 'error':
        return m.install_gate_error_badge();
      default:
        return m.install_gate_loading_badge();
    }
  });

  const completedSteps = $derived.by(() => setupQuery.data?.steps.filter((step) => step.status === 'completed').length ?? 0);

  const nextStepLabel = $derived.by(() => {
    const nextStep = setupQuery.data?.current_step;

    if (!nextStep) {
      return null;
    }

    return m.install_gate_next_step({ step: nextStep.replaceAll('_', ' ') });
  });

  $effect(() => {
    if (!browser || pageState !== 'authenticated') {
      redirectingToApp = false;
      return;
    }

    redirectingToApp = true;
    void goto('/app');
  });

  $effect(() => {
    if (localizedCurrencyCatalog.length === 0) {
      return;
    }

    if (!localizedCurrencyCatalog.some((currency) => currency.code === currencyDefaultCode)) {
      currencyDefaultCode = localizedCurrencyCatalog[0]?.code ?? 'USD';
    }
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
        password: ownerPassword
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
      await login({
        username: loginUsername,
        password: loginPassword
      });

      loginPassword = '';
      await refreshInstallGate();
    } catch (error) {
      loginError = error;
    } finally {
      loginPending = false;
    }
  }

  async function handleCreateBook(event: SubmitEvent) {
    event.preventDefault();

    bookPending = true;
    bookError = undefined;

    try {
      await createBook(
        {
          code: bookCode,
          name: bookName
        },
        sessionQuery.data?.csrf_token ?? ''
      );

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
      const currencyCodes = Array.from(new Set([currencyDefaultCode, ...additionalCurrencyCodes])).filter(Boolean);

      await completeCurrencySetup(
        {
          default_currency_code: currencyDefaultCode,
          currencies: currencyCodes.map((code) => ({
            code,
            name: currencyNamesByCode.get(code) ?? code
          }))
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
    <div class="space-y-6">
      <p class="text-sm font-medium uppercase tracking-[0.24em] text-muted">{m.home_hero_eyebrow()}</p>
      <h1 class="max-w-2xl text-5xl font-semibold tracking-tight text-balance sm:text-6xl">
        {m.app_name()}
      </h1>
      <p class="max-w-xl text-base leading-7 text-muted sm:text-lg">
        {m.home_hero_foundation_copy()}
      </p>

      <div class="grid gap-4 sm:grid-cols-2">
        <div class="rounded-[1.75rem] border border-border/80 bg-surface/90 p-5 shadow-[var(--shadow-panel)] backdrop-blur">
          <p class="text-xs font-semibold uppercase tracking-[0.2em] text-muted">
            {m.install_gate_progress_label()}
          </p>
          <p class="mt-3 text-3xl font-semibold text-foreground">
            {completedSteps} / {setupQuery.data?.steps.length ?? 0}
          </p>

          {#if nextStepLabel}
            <p class="mt-2 text-sm leading-6 text-muted">{nextStepLabel}</p>
          {/if}

          <p class="mt-3 text-sm leading-6 text-muted">{m.install_gate_non_blocking_note()}</p>
        </div>

        <div class="rounded-[1.75rem] border border-border/80 bg-surface/90 p-5 shadow-[var(--shadow-panel)] backdrop-blur">
          <div class="flex items-center justify-between gap-4">
            <div>
              <p class="text-sm font-medium text-muted">{m.home_backend_handshake()}</p>
              <p class="text-xs uppercase tracking-[0.2em] text-muted">{m.home_foundation_check()}</p>
            </div>
            <span
              class:text-danger={healthState === 'error'}
              class:bg-danger-soft={healthState === 'error'}
              class:text-accent={healthState !== 'error'}
              class:status-accent-soft={healthState !== 'error'}
              class="rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em]"
            >
              {healthStateLabel}
            </span>
          </div>

          <p class="mt-4 text-sm leading-6 text-muted">{healthMessage}</p>
        </div>
      </div>
    </div>

    <div class="rounded-[2rem] border border-border/80 bg-surface/95 p-6 shadow-[var(--shadow-panel)] backdrop-blur sm:p-8">
      <div class="mb-6 flex items-center justify-between gap-4">
        <div class="space-y-2">
          <span class="inline-flex rounded-full bg-surface-strong px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] text-muted">
            {stateBadge}
          </span>

          {#if pageState === 'fresh'}
            <h2 class="text-3xl font-semibold tracking-tight text-balance">{m.install_gate_fresh_title()}</h2>
            <p class="text-sm leading-6 text-muted">{m.install_gate_fresh_copy()}</p>
          {:else if pageState === 'login'}
            <h2 class="text-3xl font-semibold tracking-tight text-balance">{m.install_gate_login_title()}</h2>
            <p class="text-sm leading-6 text-muted">{m.install_gate_login_copy()}</p>
          {:else if pageState === 'book_setup'}
            <h2 class="text-3xl font-semibold tracking-tight text-balance">{m.install_gate_book_title()}</h2>
            <p class="text-sm leading-6 text-muted">{m.install_gate_book_copy()}</p>
          {:else if pageState === 'currency_setup'}
            <h2 class="text-3xl font-semibold tracking-tight text-balance">{m.install_gate_currencies_title()}</h2>
            <p class="text-sm leading-6 text-muted">{m.install_gate_currencies_copy()}</p>
          {:else if pageState === 'authenticated'}
            <h2 class="text-3xl font-semibold tracking-tight text-balance">{m.install_gate_authenticated_title()}</h2>
            <p class="text-sm leading-6 text-muted">{m.install_gate_authenticated_copy()}</p>
          {:else if pageState === 'recovery_required'}
            <h2 class="text-3xl font-semibold tracking-tight text-balance">{m.install_gate_recovery_title()}</h2>
            <p class="text-sm leading-6 text-muted">{m.install_gate_recovery_copy()}</p>
          {:else if pageState === 'error'}
            <h2 class="text-3xl font-semibold tracking-tight text-balance">{m.install_gate_error_title()}</h2>
            <p class="text-sm leading-6 text-muted">{m.install_gate_error_copy()}</p>
          {:else}
            <h2 class="text-3xl font-semibold tracking-tight text-balance">{m.install_gate_loading_title()}</h2>
            <p class="text-sm leading-6 text-muted">{m.install_gate_loading_copy()}</p>
          {/if}
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
        <form class="space-y-4" onsubmit={handleCreateOwner}>
          <APIFormError error={ownerError} id="owner-form-error" />

          <label class="block space-y-2">
            <span class="text-sm font-medium text-foreground">{m.install_gate_username_label()}</span>
            <input
              bind:value={ownerUsername}
              name="username"
              autocomplete="username"
              class="w-full rounded-2xl border border-border bg-surface-strong/40 px-4 py-3 text-base text-foreground placeholder:text-muted"
              required
            />
          </label>

          <label class="block space-y-2">
            <span class="text-sm font-medium text-foreground">{m.install_gate_password_label()}</span>
            <input
              bind:value={ownerPassword}
              name="password"
              type="password"
              autocomplete="new-password"
              class="w-full rounded-2xl border border-border bg-surface-strong/40 px-4 py-3 text-base text-foreground placeholder:text-muted"
              required
            />
          </label>

          <button
            type="submit"
            class="inline-flex w-full items-center justify-center rounded-full bg-foreground px-5 py-3 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
            disabled={ownerPending}
          >
            {ownerPending ? m.install_gate_owner_submit_pending() : m.install_gate_owner_submit()}
          </button>
        </form>
      {:else if pageState === 'login'}
        <form class="space-y-4" onsubmit={handleLogin}>
          <APIFormError error={loginError} id="login-form-error" />

          <label class="block space-y-2">
            <span class="text-sm font-medium text-foreground">{m.install_gate_username_label()}</span>
            <input
              bind:value={loginUsername}
              name="username"
              autocomplete="username"
              class="w-full rounded-2xl border border-border bg-surface-strong/40 px-4 py-3 text-base text-foreground placeholder:text-muted"
              required
            />
          </label>

          <label class="block space-y-2">
            <span class="text-sm font-medium text-foreground">{m.install_gate_password_label()}</span>
            <input
              bind:value={loginPassword}
              name="password"
              type="password"
              autocomplete="current-password"
              class="w-full rounded-2xl border border-border bg-surface-strong/40 px-4 py-3 text-base text-foreground placeholder:text-muted"
              required
            />
          </label>

          <button
            type="submit"
            class="inline-flex w-full items-center justify-center rounded-full bg-foreground px-5 py-3 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
            disabled={loginPending}
          >
            {loginPending ? m.install_gate_login_submit_pending() : m.install_gate_login_submit()}
          </button>
        </form>
      {:else if pageState === 'book_setup'}
        <form class="space-y-4" onsubmit={handleCreateBook}>
          <APIFormError error={bookError} id="book-form-error" />

          <label class="block space-y-2">
            <span class="text-sm font-medium text-foreground">{m.install_gate_book_name_label()}</span>
            <input
              bind:value={bookName}
              name="book-name"
              autocomplete="off"
              class="w-full rounded-2xl border border-border bg-surface-strong/40 px-4 py-3 text-base text-foreground placeholder:text-muted"
              required
            />
          </label>

          <label class="block space-y-2">
            <span class="text-sm font-medium text-foreground">{m.install_gate_book_code_label()}</span>
            <input
              bind:value={bookCode}
              name="book-code"
              autocomplete="off"
              class="w-full rounded-2xl border border-border bg-surface-strong/40 px-4 py-3 text-base text-foreground placeholder:text-muted"
            />
          </label>

          <button
            type="submit"
            class="inline-flex w-full items-center justify-center rounded-full bg-foreground px-5 py-3 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
            disabled={bookPending}
          >
            {bookPending ? m.install_gate_book_submit_pending() : m.install_gate_book_submit()}
          </button>
        </form>
      {:else if pageState === 'currency_setup'}
        <form class="space-y-4" onsubmit={handleCompleteCurrencySetup}>
          <APIFormError error={currencyError ?? currencyCatalogQuery.error} id="currency-form-error" />

          {#if currencyCatalogQuery.isPending}
            <div class="rounded-[1.75rem] border border-border bg-surface-strong/60 p-5 text-sm leading-6 text-muted">
              {m.install_gate_currencies_loading()}
            </div>
          {:else if localizedCurrencyCatalog.length === 0}
            <div class="rounded-[1.75rem] border border-border bg-surface-strong/60 p-5 text-sm leading-6 text-muted">
              {m.install_gate_currencies_empty()}
            </div>
          {:else}
            <label class="block space-y-2">
              <span class="text-sm font-medium text-foreground">{m.install_gate_default_currency_label()}</span>
              <select
                bind:value={currencyDefaultCode}
                name="default-currency"
                class="w-full rounded-2xl border border-border bg-surface-strong/40 px-4 py-3 text-base text-foreground"
                required
              >
                {#each localizedCurrencyCatalog as currency}
                  <option value={currency.code}>{currency.label}</option>
                {/each}
              </select>
            </label>

            <label class="block space-y-2">
              <span class="text-sm font-medium text-foreground">{m.install_gate_additional_currencies_label()}</span>
              <select
                bind:value={additionalCurrencyCodes}
                name="additional-currencies"
                class="min-h-48 w-full rounded-2xl border border-border bg-surface-strong/40 px-4 py-3 text-base text-foreground"
                multiple
              >
                {#each localizedCurrencyCatalog as currency}
                  <option value={currency.code}>{currency.label}</option>
                {/each}
              </select>
            </label>

            <button
              type="submit"
              class="inline-flex w-full items-center justify-center rounded-full bg-foreground px-5 py-3 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
              disabled={currencyPending || currencyCatalogQuery.isPending}
            >
              {currencyPending ? m.install_gate_currencies_submit_pending() : m.install_gate_currencies_submit()}
            </button>
          {/if}
        </form>
      {:else if pageState === 'authenticated'}
        <div class="space-y-4">
          <div class="rounded-[1.75rem] border border-border bg-surface-strong/60 p-5">
            <p class="text-sm font-semibold uppercase tracking-[0.18em] text-muted">
              {m.install_gate_authenticated_signed_in({ username: sessionQuery.data?.user?.username ?? '' })}
            </p>
            <p class="mt-3 text-sm leading-6 text-muted">
              {redirectingToApp ? m.install_gate_authenticated_redirect() : m.install_gate_authenticated_copy()}
            </p>
          </div>

          <a
            href="/app"
            class="inline-flex w-full items-center justify-center rounded-full border border-border bg-surface px-5 py-3 text-sm font-semibold text-foreground transition hover:bg-surface-strong"
          >
            {m.install_gate_open_app()}
          </a>
        </div>
      {:else}
        <div class="space-y-4">
          <div class="rounded-[1.75rem] border border-danger/25 bg-danger-soft p-5 text-sm leading-6 text-danger shadow-[var(--shadow-panel)]">
            <p class="font-semibold text-foreground">{m.install_gate_recovery_command_label()}</p>
            <p class="mt-2 font-mono text-xs leading-6 sm:text-sm">
              printf '%s\n' 'new-password' | DATABASE_URL=file:backend/var/dev.sqlite go run ./backend/cmd/rekenraam recover-owner --password-stdin
            </p>
          </div>

          <p class="text-sm leading-6 text-muted">{m.install_gate_recovery_backup_note()}</p>
        </div>
      {/if}
    </div>
  </section>
</main>
