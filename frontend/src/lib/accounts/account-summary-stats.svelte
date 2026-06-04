<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';

  let { activeCount, closedCount, visibleCount } = $props<{
    activeCount: number;
    closedCount: number;
    visibleCount: number;
  }>();
</script>

<div class="grid grid-cols-3 overflow-hidden rounded-[var(--radius-panel)] border border-border bg-border sm:min-w-80">
  <div class="bg-surface px-3 py-2.5">
    <p class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.accounts_stat_active()}</p>
    <p class="mt-1 text-xl font-semibold text-foreground">{activeCount}</p>
  </div>
  <div class="bg-surface px-3 py-2.5">
    <p class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.accounts_stat_closed()}</p>
    <p class="mt-1 text-xl font-semibold text-foreground">{closedCount}</p>
  </div>
  <div class="bg-surface px-3 py-2.5">
    <p class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.accounts_stat_visible()}</p>
    <p class="mt-1 text-xl font-semibold text-foreground">{visibleCount}</p>
  </div>
</div>
