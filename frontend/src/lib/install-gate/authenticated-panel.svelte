<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';

  let {
    username,
    redirecting
  }: {
    username: string;
    redirecting: boolean;
  } = $props();
</script>

<div class="space-y-4">
  <div class="rounded-[1.75rem] border border-border bg-surface-strong/60 p-5">
    <p class="text-sm font-semibold uppercase tracking-[0.18em] text-muted">
      {m.install_gate_authenticated_signed_in({ username })}
    </p>
    <p class="mt-3 text-sm leading-6 text-muted">
      {redirecting ? m.install_gate_authenticated_redirect() : m.install_gate_authenticated_copy()}
    </p>
  </div>

  <a
    href="/app"
    class="inline-flex w-full items-center justify-center rounded-full border border-border bg-surface px-5 py-3 text-sm font-semibold text-foreground transition hover:bg-surface-strong"
  >
    {m.install_gate_open_app()}
  </a>
</div>
