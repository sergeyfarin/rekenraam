<script lang="ts">
  import { browser } from '$app/environment';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { createQuery } from '@tanstack/svelte-query';
  import { LayoutDashboard, LogOut, RefreshCw, Tags, WalletCards } from '@lucide/svelte';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import PageHeader from '$lib/components/page-header.svelte';
  import Panel from '$lib/components/panel.svelte';
  import StatusBadge from '$lib/components/status-badge.svelte';
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

  const installStateLabel = $derived.by(() => {
    switch (setupQuery.data?.install_state) {
      case 'fresh':
        return m.app_shell_install_state_fresh();
      case 'configured':
        return m.app_shell_install_state_configured();
      case 'recovery_required':
        return m.app_shell_install_state_recovery_required();
      default:
        return setupQuery.data?.install_state ?? 'unknown';
    }
  });

  const isOverviewRoute = $derived($page.url.pathname === '/app');
  const isAccountsRoute = $derived($page.url.pathname.startsWith('/app/accounts'));
  const isCategoriesRoute = $derived($page.url.pathname.startsWith('/app/categories'));

  const headerTitle = $derived(
    isAccountsRoute ? m.accounts_title() : isCategoriesRoute ? m.categories_title() : m.app_shell_header_title()
  );
  const headerCopy = $derived(
    isAccountsRoute
      ? m.accounts_shell_copy()
      : isCategoriesRoute
        ? m.categories_shell_copy()
        : m.app_shell_header_copy()
  );

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
      const refreshedSession = await sessionQuery.refetch();

      if (!refreshedSession.data?.authenticated) {
        await goto('/');
        return;
      }

      if (!refreshedSession.data.csrf_token) {
        throw new APIClientError({ status: 403, code: 'CSRF_INVALID' });
      }

      await logout(refreshedSession.data.csrf_token);
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
  <main class="min-h-screen px-4 py-14 sm:px-6 lg:px-8">
    <div class="mx-auto max-w-3xl">
      <StatusBadge>{m.app_shell_badge()}</StatusBadge>
      <h1 class="mt-4 text-2xl font-semibold tracking-tight text-balance">{m.app_shell_loading_title()}</h1>
      <p class="mt-3 text-sm leading-6 text-muted">{m.app_shell_loading_copy()}</p>
    </div>
  </main>
{:else if shellState === 'error'}
  <main class="min-h-screen px-4 py-14 sm:px-6 lg:px-8">
    <div class="mx-auto max-w-3xl">
      <Panel>
        <StatusBadge tone="danger">{m.app_shell_badge()}</StatusBadge>
        <h1 class="mt-4 text-2xl font-semibold tracking-tight text-balance">{m.app_shell_error_title()}</h1>
        <p class="mt-3 text-sm leading-6 text-muted">{m.app_shell_error_copy()}</p>

        <div class="mt-6 space-y-4">
          <APIFormError error={shellError} id="app-shell-error" />
          <button
            type="button"
            class="inline-flex items-center gap-2 rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90"
            onclick={refreshShell}
          >
            <RefreshCw size={16} aria-hidden="true" />
            {m.app_shell_retry()}
          </button>
        </div>
      </Panel>
    </div>
  </main>
{:else if !shouldRedirectHome}
  <main class="min-h-screen lg:grid lg:grid-cols-[16rem_minmax(0,1fr)]">
    <aside class="border-b border-border bg-sidebar px-4 py-4 lg:min-h-screen lg:border-b-0 lg:border-r lg:px-4 lg:py-5">
      <div class="flex items-center justify-between gap-3 lg:block">
        <div>
          <p class="text-xs font-semibold uppercase tracking-[0.14em] text-muted">{m.app_shell_badge()}</p>
          <h1 class="mt-1 text-xl font-semibold tracking-tight text-balance">{m.app_name()}</h1>
        </div>

        <StatusBadge tone={setupQuery.data?.install_state === 'configured' ? 'positive' : 'warning'}>
          {installStateLabel}
        </StatusBadge>
      </div>

      <nav
        class="mt-5 flex gap-2 overflow-x-auto pb-1 lg:block lg:space-y-1 lg:overflow-visible lg:pb-0"
        aria-label={m.app_shell_nav_title()}
      >
        <a
          href="/app"
          aria-current={isOverviewRoute ? 'page' : undefined}
          class:bg-selected={isOverviewRoute}
          class:text-selected-foreground={isOverviewRoute}
          class:bg-transparent={!isOverviewRoute}
          class:text-foreground={!isOverviewRoute}
          class="flex min-w-fit items-center gap-2 rounded-(--radius-control) px-3 py-2 text-sm font-semibold transition hover:bg-control-hover"
        >
          <LayoutDashboard size={16} aria-hidden="true" />
          {m.app_shell_nav_overview()}
        </a>
        <a
          href="/app/accounts"
          aria-current={isAccountsRoute ? 'page' : undefined}
          class:bg-selected={isAccountsRoute}
          class:text-selected-foreground={isAccountsRoute}
          class:bg-transparent={!isAccountsRoute}
          class:text-foreground={!isAccountsRoute}
          class="flex min-w-fit items-center gap-2 rounded-(--radius-control) px-3 py-2 text-sm font-semibold transition hover:bg-control-hover"
        >
          <WalletCards size={16} aria-hidden="true" />
          {m.app_shell_nav_accounts()}
        </a>
        <a
          href="/app/categories"
          aria-current={isCategoriesRoute ? 'page' : undefined}
          class:bg-selected={isCategoriesRoute}
          class:text-selected-foreground={isCategoriesRoute}
          class:bg-transparent={!isCategoriesRoute}
          class:text-foreground={!isCategoriesRoute}
          class="flex min-w-fit items-center gap-2 rounded-(--radius-control) px-3 py-2 text-sm font-semibold transition hover:bg-control-hover"
        >
          <Tags size={16} aria-hidden="true" />
          {m.app_shell_nav_categories()}
        </a>
      </nav>

      <div class="mt-5 hidden rounded-(--radius-panel) border border-border bg-surface/70 p-3 lg:block">
        <p class="text-xs font-semibold uppercase tracking-[0.14em] text-muted">{m.app_shell_status_title()}</p>
        <div class="mt-3 space-y-2 text-sm leading-5">
          <p class="text-foreground">
            {m.app_shell_status_install_state({ state: installStateLabel })}
          </p>
          <p class="text-muted">
            {m.app_shell_status_session({ username: sessionQuery.data?.user?.username ?? '' })}
          </p>

          {#if nextStepLabel}
            <p class="text-muted">{nextStepLabel}</p>
          {/if}
        </div>
      </div>

      <div class="mt-5 space-y-3">
        <APIFormError error={logoutError} id="app-shell-logout-error" />
        <button
          type="button"
          class="inline-flex w-full items-center justify-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-60"
          onclick={handleLogout}
          disabled={logoutPending}
        >
          <LogOut size={16} aria-hidden="true" />
          {logoutPending ? m.app_shell_logout_pending() : m.app_shell_logout()}
        </button>
      </div>
    </aside>

    <section class="min-w-0">
      <PageHeader eyebrow={m.app_shell_header_eyebrow()} title={headerTitle} copy={headerCopy} />

      <div class="px-4 py-4 sm:px-6 lg:px-8">
        {@render children()}
      </div>
    </section>
  </main>
{/if}
