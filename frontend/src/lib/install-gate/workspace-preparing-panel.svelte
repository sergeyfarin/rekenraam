<script lang="ts">
  import APIFormError from '$lib/components/api-form-error.svelte';
  import { m } from '$lib/paraglide/messages.js';

  let {
    error,
    pending,
    onretry
  }: {
    error: unknown;
    pending: boolean;
    onretry: () => void;
  } = $props();
</script>

<div class="space-y-4">
  <APIFormError {error} id="workspace-form-error" />

  <div class="rounded-[1.75rem] border border-border bg-surface-strong/60 p-5 text-sm leading-6 text-muted">
    {error ? m.install_gate_preparing_error_copy() : m.install_gate_preparing_copy()}
  </div>

  {#if error}
    <button
      type="button"
      class="inline-flex w-full items-center justify-center rounded-full bg-foreground px-5 py-3 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
      onclick={onretry}
      disabled={pending}
    >
      {pending ? m.install_gate_preparing_submit_pending() : m.install_gate_preparing_submit()}
    </button>
  {/if}
</div>
