<script lang="ts">
  // Names a short position in text, never by colour alone (#174). Long is the
  // ordinary case and shows nothing.
  import { m } from '#lib/paraglide/messages.js';

  let { side }: { side: string | undefined } = $props();
</script>

{#if side === 'short'}
  <span class="ml-2 inline-flex items-center rounded-full border border-border px-2 py-0.5 text-xs font-medium text-foreground"
    data-testid="position-side-short">{m.investments_position_side_short()}</span>
{/if}
