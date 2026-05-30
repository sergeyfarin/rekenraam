<script lang="ts">
  import { browser } from '$app/environment';
  import { goto } from '$app/navigation';
  import { createQuery } from '@tanstack/svelte-query';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import { authSessionQueryOptions, logout } from '$lib/api/auth';
  import { setupStatusQueryOptions } from '$lib/api/setup';
  import { APIClientError } from '$lib/api/client';
  import { m } from '$lib/paraglide/messages.js';

  let { children } = $props();

  const setupQuery = createQuery(() => setupStatusQueryOptions());
  const sessionQuery = createQuery(() => authSessionQueryOptions());

  let logoutError = $state<unknown>(undefined);
  let logoutPending = $state(false);

  const shellError = $derived(setupQuery.error ?? sessionQuery.error);

  const shellState = $derived.by<'loading' | 'error' | 'ready'>(() => {
    if (setupQuery.isPending || sessionQuery.isPending) {
      return 'loading';
    }

    if (setupQuery.isError || sessionQuery.isError) {
      return 'error';
    }

    if (setupQuery.data === undefined || sessionQuery.data === undefined) {
      return 'loading';
    }

    return 'ready';
  });

  const shouldRedirectHome = $derived.by(() => {
    if (shellState !== 'ready') {
      return false;
    }

    return setupQuery.data?.install_state !== 'configured' || !sessionQuery.data?.authenticated;
  });

  const nextStepLabel = $derived.by(() => {
    const nextStep = setupQuery.data?.current_step;

    if (!nextStep) {
      return null;
    }

    return m.install_gate_next_step({ step: nextStep.replaceAll('_', ' ') });
  });

  $effect(() => {
    if (browser && shouldRedirectHome) {
      void goto('/');
    }
  });

  async function refreshShell() {
    await Promise.all([setupQuery.refetch(), sessionQuery.refetch()]);
  }

  async function handleLogout() {
    logoutPending = true;
    logoutError = undefined;

    try {
      let csrfToken = sessionQuery.data?.csrf_token;

      if (!csrfToken) {
        const refreshedSession = await sessionQuery.refetch();
        csrfToken = refreshedSession.data?.csrf_token;
      }

      if (!csrfToken) {
        throw new APIClientError({ status: 403, code: 'CSRF_INVALID' });
      }

      await logout(csrfToken);
      await sessionQuery.refetch();
      await goto('/');
    } catch (error) {
      logoutError = error;
    } finally {
      logoutPending = false;
    }
  }
</script>

{#if shellState === 'loading'}
  <main class="min-h-screen px-6 py-16 sm:px-10">
    <section class="mx-auto max-w-3xl rounded-[2rem] border border-border/80 bg-surface/95 p-8 shadow-[var(--shadow-panel)] backdrop-blur">
      <span class="inline-flex rounded-full bg-surface-strong px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] text-muted">
        {m.app_shell_badge()}
      </span>
      <h1 class="mt-4 text-3xl font-semibold tracking-tight text-balance">{m.app_shell_loading_title()}</h1>
      <p class="mt-3 text-sm leading-6 text-muted">{m.app_shell_loading_copy()}</p>
    </section>
  </main>
{:else if shellState === 'error'}
  <main class="min-h-screen px-6 py-16 sm:px-10">
    <section class="mx-auto max-w-3xl rounded-[2rem] border border-border/80 bg-surface/95 p-8 shadow-[var(--shadow-panel)] backdrop-blur">
      <span class="inline-flex rounded-full bg-danger-soft px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] text-danger">
        {m.app_shell_badge()}
      </span>
      <h1 class="mt-4 text-3xl font-semibold tracking-tight text-balance">{m.app_shell_error_title()}</h1>
      <p class="mt-3 text-sm leading-6 text-muted">{m.app_shell_error_copy()}</p>

      <div class="mt-6 space-y-4">
        <APIFormError error={shellError} id="app-shell-error" />
        <button
          type="button"
          class="inline-flex items-center rounded-full bg-foreground px-5 py-3 text-sm font-semibold text-background transition hover:opacity-90"
          onclick={refreshShell}
        >
          {m.app_shell_retry()}
        </button>
      </div>
    </section>
  </main>
{:else if !shouldRedirectHome}
  <main class="min-h-screen px-6 py-10 sm:px-10 sm:py-12">
    <div class="mx-auto grid max-w-7xl gap-6 lg:grid-cols-[17rem_1fr]">
      <aside class="rounded-[2rem] border border-border/80 bg-surface/95 p-6 shadow-[var(--shadow-panel)] backdrop-blur">
        <div>
          <p class="text-xs font-semibold uppercase tracking-[0.2em] text-muted">{m.app_shell_badge()}</p>
          <h1 class="mt-3 text-3xl font-semibold tracking-tight text-balance">{m.app_name()}</h1>
        </div>

        <nav class="mt-8 space-y-3" aria-label={m.app_shell_nav_title()}>
          <p class="text-xs font-semibold uppercase tracking-[0.18em] text-muted">{m.app_shell_nav_title()}</p>
          <a
            href="/app"
            aria-current="page"
            class="flex items-center rounded-2xl border border-border bg-surface-strong/70 px-4 py-3 text-sm font-semibold text-foreground"
          >
            {m.app_shell_nav_overview()}
          </a>
          <div class="rounded-2xl border border-dashed border-border px-4 py-3 text-sm text-muted">
            {m.app_shell_nav_future()}
          </div>
        </nav>

        <div class="mt-8 rounded-[1.75rem] border border-border bg-surface-strong/60 p-4">
          <p class="text-xs font-semibold uppercase tracking-[0.18em] text-muted">{m.app_shell_status_title()}</p>
          <p class="mt-3 text-sm leading-6 text-foreground">{m.app_shell_status_setup()}</p>
          <p class="mt-2 text-sm leading-6 text-muted">
            {m.app_shell_status_session({ username: sessionQuery.data?.user?.username ?? '' })}
          </p>

          {#if nextStepLabel}
            <p class="mt-2 text-sm leading-6 text-muted">{nextStepLabel}</p>
          {/if}
        </div>

        <div class="mt-8 space-y-4">
          <APIFormError error={logoutError} id="app-shell-logout-error" />
          <button
            type="button"
            class="inline-flex w-full items-center justify-center rounded-full border border-border bg-surface px-5 py-3 text-sm font-semibold text-foreground transition hover:bg-surface-strong disabled:cursor-not-allowed disabled:opacity-60"
            onclick={handleLogout}
            disabled={logoutPending}
          >
            {logoutPending ? m.install_gate_logout_pending() : m.install_gate_logout()}
          </button>
        </div>
      </aside>

      <section class="space-y-6">
        <header class="rounded-[2rem] border border-border/80 bg-surface/95 p-6 shadow-[var(--shadow-panel)] backdrop-blur sm:p-8">
          <p class="text-xs font-semibold uppercase tracking-[0.2em] text-muted">{m.app_shell_header_eyebrow()}</p>
          <h2 class="mt-3 text-4xl font-semibold tracking-tight text-balance">{m.app_shell_header_title()}</h2>
          <p class="mt-3 max-w-3xl text-sm leading-6 text-muted">{m.app_shell_header_copy()}</p>
        </header>

        {@render children()}
      </section>
    </div>
  </main>
{/if}