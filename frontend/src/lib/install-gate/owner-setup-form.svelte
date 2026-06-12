<script lang="ts">
  import APIFormError from '$lib/components/api-form-error.svelte';
  import { m } from '$lib/paraglide/messages.js';

  let {
    username = $bindable(''),
    password = $bindable(''),
    error,
    pending,
    onsubmit
  }: {
    username: string;
    password: string;
    error: unknown;
    pending: boolean;
    onsubmit: (event: SubmitEvent) => void;
  } = $props();
</script>

<form class="space-y-4" {onsubmit}>
  <APIFormError {error} id="owner-form-error" />

  <label class="block space-y-2">
    <span class="text-sm font-medium text-foreground">{m.install_gate_username_label()}</span>
    <input
      bind:value={username}
      name="username"
      autocomplete="username"
      class="w-full rounded-2xl border border-border bg-surface-strong/40 px-4 py-3 text-base text-foreground placeholder:text-muted"
      required
      maxlength="64"
    />
  </label>

  <label class="block space-y-2">
    <span class="text-sm font-medium text-foreground">{m.install_gate_password_label()}</span>
    <input
      bind:value={password}
      name="password"
      type="password"
      autocomplete="new-password"
      class="w-full rounded-2xl border border-border bg-surface-strong/40 px-4 py-3 text-base text-foreground placeholder:text-muted"
      required
      minlength="12"
      maxlength="1024"
    />
    <span class="block text-xs leading-5 text-muted">{m.install_gate_password_hint()}</span>
  </label>

  <button
    type="submit"
    class="inline-flex w-full items-center justify-center rounded-full bg-foreground px-5 py-3 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
    disabled={pending}
  >
    {pending ? m.install_gate_owner_submit_pending() : m.install_gate_owner_submit()}
  </button>
</form>
