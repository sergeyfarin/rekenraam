<script lang="ts">
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { useHomePersonalization } from './personalization.svelte.ts';

  let { allowUndo = true }: { allowUndo?: boolean } = $props();
  const personalization = useHomePersonalization();
</script>

{#if personalization.conflict}
  <p role="alert" class="my-2 rounded-(--radius-panel) border border-warning bg-surface p-3 text-sm text-foreground">{m.home_layout_conflict()}</p>
{/if}
<APIFormError error={personalization.error} />
{#if allowUndo && personalization.hasUndo}
  <div class="my-2 flex flex-wrap items-center gap-3 rounded-(--radius-panel) border border-border bg-surface px-3 text-sm">
    <span role="status">{m.home_layout_saved()}</span>
    <button type="button" disabled={personalization.pending} onclick={personalization.undo}
      class="min-h-11 px-2 font-semibold text-accent underline-offset-4 hover:underline">{m.home_layout_undo()}</button>
  </div>
{/if}
<p aria-live="polite" class="sr-only">{personalization.announcement}</p>
