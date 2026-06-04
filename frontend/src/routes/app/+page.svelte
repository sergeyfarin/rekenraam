<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { Activity, CheckCircle2, GitBranch, Route, ShieldCheck } from '@lucide/svelte';
  import { healthQueryOptions } from '$lib/api/health';
  import { getAPIClientErrorMessage } from '$lib/api-error-messages';
  import Panel from '$lib/components/panel.svelte';
  import StatusBadge from '$lib/components/status-badge.svelte';
  import { m } from '$lib/paraglide/messages.js';

  const healthQuery = createQuery(() => healthQueryOptions());

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
    if (healthQuery.isPending) {
      return m.home_health_state_loading();
    }

    if (healthQuery.isError) {
      return m.home_health_state_error();
    }

    return m.home_health_state_success();
  });
</script>

<section class="grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(20rem,0.85fr)]">
  <Panel>
    <h2 class="text-lg font-semibold tracking-tight text-balance">{m.app_shell_overview_title()}</h2>
    <p class="mt-3 max-w-3xl text-sm leading-6 text-muted">{m.app_shell_overview_copy()}</p>

    <div class="mt-5 divide-y divide-border overflow-hidden rounded-[var(--radius-panel)] border border-border">
      <article class="grid gap-3 bg-surface px-4 py-3 sm:grid-cols-[1.5rem_minmax(10rem,0.35fr)_1fr] sm:items-start">
        <ShieldCheck size={18} class="mt-0.5 text-accent" aria-hidden="true" />
        <p class="text-sm font-semibold text-foreground">{m.app_shell_card_auth_title()}</p>
        <p class="text-sm leading-6 text-muted">{m.app_shell_card_auth_copy()}</p>
      </article>

      <article class="grid gap-3 bg-surface px-4 py-3 sm:grid-cols-[1.5rem_minmax(10rem,0.35fr)_1fr] sm:items-start">
        <CheckCircle2 size={18} class="mt-0.5 text-positive" aria-hidden="true" />
        <p class="text-sm font-semibold text-foreground">{m.app_shell_card_setup_title()}</p>
        <p class="text-sm leading-6 text-muted">{m.app_shell_card_setup_copy()}</p>
      </article>

      <article class="grid gap-3 bg-surface px-4 py-3 sm:grid-cols-[1.5rem_minmax(10rem,0.35fr)_1fr] sm:items-start">
        <Route size={18} class="mt-0.5 text-muted" aria-hidden="true" />
        <p class="text-sm font-semibold text-foreground">{m.app_shell_card_routing_title()}</p>
        <p class="text-sm leading-6 text-muted">{m.app_shell_card_routing_copy()}</p>
      </article>
    </div>
  </Panel>

  <Panel variant="toolbar">
    <div class="flex items-center justify-between gap-4">
      <div class="flex items-center gap-2">
        <Activity size={18} class="text-muted" aria-hidden="true" />
        <p class="text-sm font-semibold text-foreground">{m.app_shell_health_title()}</p>
      </div>
      <StatusBadge tone={healthQuery.isError ? 'danger' : 'accent'}>{healthStateLabel}</StatusBadge>
    </div>

    <p class="mt-4 text-sm leading-6 text-muted">{healthMessage}</p>
  </Panel>
</section>

<section class="mt-4">
  <Panel>
    <div class="flex items-center gap-2">
      <GitBranch size={18} class="text-muted" aria-hidden="true" />
      <h2 class="text-lg font-semibold tracking-tight text-balance">{m.app_shell_next_title()}</h2>
    </div>

    <div class="mt-5 grid gap-px overflow-hidden rounded-[var(--radius-panel)] border border-border bg-border md:grid-cols-2">
      <article class="bg-surface p-4">
        <p class="text-sm font-semibold text-foreground">{m.app_shell_next_books_title()}</p>
        <p class="mt-2 text-sm leading-6 text-muted">{m.app_shell_next_books_copy()}</p>
      </article>

      <article class="bg-surface p-4">
        <p class="text-sm font-semibold text-foreground">{m.app_shell_next_transactions_title()}</p>
        <p class="mt-2 text-sm leading-6 text-muted">{m.app_shell_next_transactions_copy()}</p>
      </article>

      <article class="bg-surface p-4">
        <p class="text-sm font-semibold text-foreground">{m.app_shell_next_reports_title()}</p>
        <p class="mt-2 text-sm leading-6 text-muted">{m.app_shell_next_reports_copy()}</p>
      </article>

      <article class="bg-surface p-4">
        <p class="text-sm font-semibold text-foreground">{m.app_shell_next_imports_title()}</p>
        <p class="mt-2 text-sm leading-6 text-muted">{m.app_shell_next_imports_copy()}</p>
      </article>
    </div>
  </Panel>
</section>
