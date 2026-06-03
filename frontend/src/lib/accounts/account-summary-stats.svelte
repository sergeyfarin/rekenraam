<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';

  let { activeCount, closedCount, visibleCount } = $props<{
    activeCount: number;
    closedCount: number;
    visibleCount: number;
  }>();
</script>

<div class="grid grid-cols-3 gap-2 sm:min-w-80">
  <div class="rounded-2xl border border-border bg-surface-strong/60 p-3">
    <p class="text-xs font-semibold uppercase tracking-[0.16em] text-muted">{m.accounts_stat_active()}</p>
    <p class="mt-2 text-2xl font-semibold text-foreground">{activeCount}</p>
  </div>
  <div class="rounded-2xl border border-border bg-surface-strong/60 p-3">
    <p class="text-xs font-semibold uppercase tracking-[0.16em] text-muted">{m.accounts_stat_closed()}</p>
    <p class="mt-2 text-2xl font-semibold text-foreground">{closedCount}</p>
  </div>
  <div class="rounded-2xl border border-border bg-surface-strong/60 p-3">
    <p class="text-xs font-semibold uppercase tracking-[0.16em] text-muted">{m.accounts_stat_visible()}</p>
    <p class="mt-2 text-2xl font-semibold text-foreground">{visibleCount}</p>
  </div>
</div>
