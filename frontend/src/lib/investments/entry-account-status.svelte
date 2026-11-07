<script lang="ts">
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { m } from '#lib/paraglide/messages.js';
  let { pending, error, onRetry }: { pending: boolean; error?: unknown; onRetry: () => unknown } = $props();
</script>

{#if pending}
  <p role="status" class="text-sm text-muted">{m.accounts_loading_title()}</p>
{:else if error}
  <APIFormError {error} />
  <button type="button" onclick={() => { onRetry(); }} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">{m.accounts_retry()}</button>
{/if}
