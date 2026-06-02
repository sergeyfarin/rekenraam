<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';

  let {
    completedSteps,
    totalSteps,
    nextStepLabel,
    healthState,
    healthStateLabel,
    healthMessage
  }: {
    completedSteps: number;
    totalSteps: number;
    nextStepLabel: string | null;
    healthState: 'loading' | 'success' | 'error';
    healthStateLabel: string;
    healthMessage: string;
  } = $props();
</script>

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
        {completedSteps} / {totalSteps}
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
