<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import { getFormErrorState } from '$lib/form-errors';

  type Props = {
    error?: unknown;
    id?: string;
  };

  let { error = undefined, id = undefined }: Props = $props();

  const formError = $derived(getFormErrorState(error));
</script>

{#if formError !== null}
  <div
    id={id}
    role="alert"
    aria-live="polite"
    class="rounded-2xl border border-danger/25 bg-danger-soft px-4 py-3 text-sm text-danger shadow-[var(--shadow-panel)]"
  >
    <p class="font-semibold text-foreground">{m.form_error_title()}</p>
    <p class="mt-1 leading-6">{formError.message}</p>

    {#if formError.requestId}
      <p class="mt-2 text-xs uppercase tracking-[0.16em] text-muted">
        {m.form_error_request_id({ requestId: formError.requestId })}
      </p>
    {/if}
  </div>
{/if}