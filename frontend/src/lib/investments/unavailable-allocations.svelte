<script lang="ts">
  import { m } from '#lib/paraglide/messages.js';

  // Lot quantities restored for lots not held on the current date (#521
  // review): named, never dropped, and the preview waits until the owner
  // removes them or changes the date back.
  let { onRemove }: { onRemove: () => void } = $props();
</script>

<div role="alert" class="space-y-2 rounded-(--radius-control) border border-border bg-surface px-3 py-2 text-sm text-foreground">
  <p>{m.investments_activity_lots_unavailable()}</p>
  <button type="button" onclick={onRemove}
    class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover">
    {m.investments_activity_lots_unavailable_remove()}
  </button>
</div>
