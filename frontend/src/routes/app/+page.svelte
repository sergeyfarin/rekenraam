<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { healthQueryOptions } from '$lib/api/health';
  import { getAPIClientErrorMessage } from '$lib/api-error-messages';
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

<section class="grid gap-6 xl:grid-cols-[1.1fr_0.9fr]">
  <div class="rounded-[2rem] border border-border/80 bg-surface/95 p-6 shadow-[var(--shadow-panel)] backdrop-blur sm:p-8">
    <h3 class="text-2xl font-semibold tracking-tight text-balance">{m.app_shell_overview_title()}</h3>
    <p class="mt-3 max-w-3xl text-sm leading-6 text-muted">{m.app_shell_overview_copy()}</p>

    <div class="mt-6 grid gap-4 sm:grid-cols-3">
      <article class="rounded-[1.75rem] border border-border bg-surface-strong/60 p-4">
        <p class="text-sm font-semibold text-foreground">{m.app_shell_card_auth_title()}</p>
        <p class="mt-2 text-sm leading-6 text-muted">{m.app_shell_card_auth_copy()}</p>
      </article>

      <article class="rounded-[1.75rem] border border-border bg-surface-strong/60 p-4">
        <p class="text-sm font-semibold text-foreground">{m.app_shell_card_setup_title()}</p>
        <p class="mt-2 text-sm leading-6 text-muted">{m.app_shell_card_setup_copy()}</p>
      </article>

      <article class="rounded-[1.75rem] border border-border bg-surface-strong/60 p-4">
        <p class="text-sm font-semibold text-foreground">{m.app_shell_card_routing_title()}</p>
        <p class="mt-2 text-sm leading-6 text-muted">{m.app_shell_card_routing_copy()}</p>
      </article>
    </div>
  </div>

  <div class="rounded-[2rem] border border-border/80 bg-surface/95 p-6 shadow-[var(--shadow-panel)] backdrop-blur sm:p-8">
    <div class="flex items-center justify-between gap-4">
      <p class="text-sm font-semibold text-foreground">{m.app_shell_health_title()}</p>
      <span
        class:text-danger={healthQuery.isError}
        class:bg-danger-soft={healthQuery.isError}
        class:text-accent={!healthQuery.isError}
        class="rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em]"
        style:background-color={healthQuery.isError ? 'var(--color-danger-soft)' : 'color-mix(in oklab, var(--color-accent) 12%, transparent)'}
      >
        {healthStateLabel}
      </span>
    </div>

    <p class="mt-4 text-sm leading-6 text-muted">{healthMessage}</p>
  </div>
</section>

<section class="mt-6 rounded-[2rem] border border-border/80 bg-surface/95 p-6 shadow-[var(--shadow-panel)] backdrop-blur sm:p-8">
  <h3 class="text-2xl font-semibold tracking-tight text-balance">{m.app_shell_next_title()}</h3>

  <div class="mt-6 grid gap-4 md:grid-cols-2">
    <article class="rounded-[1.75rem] border border-border bg-surface-strong/60 p-4">
      <p class="text-sm font-semibold text-foreground">{m.app_shell_next_books_title()}</p>
      <p class="mt-2 text-sm leading-6 text-muted">{m.app_shell_next_books_copy()}</p>
    </article>

    <article class="rounded-[1.75rem] border border-border bg-surface-strong/60 p-4">
      <p class="text-sm font-semibold text-foreground">{m.app_shell_next_transactions_title()}</p>
      <p class="mt-2 text-sm leading-6 text-muted">{m.app_shell_next_transactions_copy()}</p>
    </article>

    <article class="rounded-[1.75rem] border border-border bg-surface-strong/60 p-4">
      <p class="text-sm font-semibold text-foreground">{m.app_shell_next_reports_title()}</p>
      <p class="mt-2 text-sm leading-6 text-muted">{m.app_shell_next_reports_copy()}</p>
    </article>

    <article class="rounded-[1.75rem] border border-border bg-surface-strong/60 p-4">
      <p class="text-sm font-semibold text-foreground">{m.app_shell_next_imports_title()}</p>
      <p class="mt-2 text-sm leading-6 text-muted">{m.app_shell_next_imports_copy()}</p>
    </article>
  </div>
</section>
