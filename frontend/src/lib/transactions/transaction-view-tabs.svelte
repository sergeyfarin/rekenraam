<script lang="ts">
  import type { ReadonlyURLSearchParams } from '$app/state';
  import { m } from '#lib/paraglide/messages.js';
  import {
    transactionViewHref,
    type TransactionView
  } from './transaction-view.ts';

  let {
    view,
    params
  }: {
    view: TransactionView;
    params: ReadonlyURLSearchParams;
  } = $props();

  const views = [
    { id: 'all', label: m.transactions_view_all },
    { id: 'drafts', label: m.transactions_view_drafts },
    { id: 'deleted', label: m.transactions_view_deleted }
  ] as const;
</script>

<!-- The segmented view control from the design (#446). These are links, not
     tabs: each view is a real URL, so Back, copied links and reload all work. -->
<nav
  aria-label={m.transactions_views_label()}
  class="flex w-max max-w-full gap-1 overflow-x-auto rounded-(--radius-panel) border border-border bg-surface p-1"
>
  {#each views as item (item.id)}
    <a
      href={transactionViewHref(item.id, params)}
      aria-current={view === item.id ? 'page' : undefined}
      data-testid={`transactions-view-${item.id}`}
      class="flex min-h-9 shrink-0 items-center rounded-(--radius-control) px-3 text-sm font-medium text-muted transition hover:text-foreground aria-[current=page]:bg-surface-strong aria-[current=page]:text-foreground"
    >
      {item.label()}
    </a>
  {/each}
</nav>
