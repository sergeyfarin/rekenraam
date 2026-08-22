<script lang="ts">
  import APIFormError from '$lib/components/api-form-error.svelte';
  import { m } from '$lib/paraglide/messages.js';

  let {
    code = $bindable(''),
    error,
    pending,
    onsubmit,
    oncancel
  }: {
    code: string;
    error: unknown;
    pending: boolean;
    onsubmit: (event: SubmitEvent) => void;
    oncancel: () => void;
  } = $props();
</script>

<form class="space-y-4" {onsubmit}>
  <APIFormError {error} id="login-mfa-form-error" />

  <p class="text-sm text-muted">{m.install_gate_mfa_hint()}</p>

  <label class="block space-y-2">
    <span class="text-sm font-medium text-foreground">{m.install_gate_mfa_code_label()}</span>
    <input
      bind:value={code}
      name="one-time-code"
      inputmode="text"
      autocomplete="one-time-code"
      autocapitalize="characters"
      spellcheck="false"
      class="w-full rounded-2xl border border-border bg-surface-strong/40 px-4 py-3 text-base tracking-[0.2em] text-foreground placeholder:text-muted placeholder:tracking-normal"
      placeholder="000000"
      required
    />
  </label>

  <button
    type="submit"
    class="inline-flex w-full items-center justify-center rounded-full bg-foreground px-5 py-3 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
    disabled={pending}
  >
    {pending ? m.install_gate_mfa_submit_pending() : m.install_gate_mfa_submit()}
  </button>

  <button
    type="button"
    class="inline-flex w-full items-center justify-center rounded-full px-5 py-2 text-sm font-semibold text-muted transition hover:text-foreground"
    onclick={oncancel}
  >
    {m.install_gate_mfa_cancel()}
  </button>
</form>
