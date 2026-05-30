<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { healthQueryOptions } from '$lib/api/health';
  import { getAPIClientErrorMessage } from '$lib/api-error-messages';
  import { m } from '$lib/paraglide/messages.js';

  const healthQuery = createQuery(() => healthQueryOptions());

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
</script>

<main class="min-h-screen px-6 py-16 sm:px-10">
  <section class="mx-auto grid max-w-4xl gap-8 lg:grid-cols-[1.25fr_0.9fr] lg:items-end">
    <div class="space-y-5">
      <p class="text-sm font-medium uppercase tracking-[0.24em] text-muted">{m.home_hero_eyebrow()}</p>
      <h1 class="max-w-2xl text-5xl font-semibold tracking-tight text-balance sm:text-6xl">
        {m.app_name()}
      </h1>
      <p class="max-w-xl text-base leading-7 text-muted sm:text-lg">
        {m.home_hero_foundation_copy()}
      </p>
    </div>

    <div class="rounded-[2rem] border border-border/80 bg-surface/95 p-6 shadow-[var(--shadow-panel)] backdrop-blur">
      <div class="mb-5 flex items-center justify-between gap-4">
        <div>
          <p class="text-sm font-medium text-muted">{m.home_backend_handshake()}</p>
          <p class="text-xs uppercase tracking-[0.2em] text-muted">{m.home_foundation_check()}</p>
        </div>
        <span
          class:text-danger={healthState === 'error'}
          class:bg-danger-soft={healthState === 'error'}
          class:text-accent={healthState !== 'error'}
          class="rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em]"
          style:background-color={healthState === 'error' ? 'var(--color-danger-soft)' : 'color-mix(in oklab, var(--color-accent) 12%, transparent)'}
        >
          {healthStateLabel}
        </span>
      </div>

      <div class="rounded-2xl border border-border bg-surface-strong/60 p-4">
        <p
          class:text-danger={healthState === 'error'}
          class:text-foreground={healthState !== 'error'}
          class="text-sm leading-6"
        >
          {healthMessage}
        </p>
      </div>
    </div>
  </section>
</main>
