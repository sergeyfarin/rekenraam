<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { m } from '$lib/paraglide/messages.js';
  import { getLocale } from '$lib/paraglide/runtime.js';
  import Panel from '$lib/components/panel.svelte';
  import { formatQuantity } from '$lib/money/format';
  import {
    reconciliationCheckpointsQueryOptions,
    type ReconciliationCheckpointResponse
  } from '$lib/api/reconciliation';

  let {
    accountID,
    commodityID
  }: {
    accountID: number;
    commodityID?: number;
  } = $props();

  const locale = $derived(getLocale());

  const checkpointsQuery = createQuery(() =>
    reconciliationCheckpointsQueryOptions(accountID, commodityID)
  );

  // Only active checkpoints represent a current reconciled-through point.
  const active = $derived(
    (checkpointsQuery.data?.checkpoints ?? [])
      .filter((c: ReconciliationCheckpointResponse) => c.status === 'active')
      .sort((a, b) => b.statement_date.localeCompare(a.statement_date))
  );
</script>

<Panel>
  <h3 class="text-sm font-semibold text-foreground">{m.reconcile_checkpoints_title()}</h3>

  {#if checkpointsQuery.isSuccess && active.length === 0}
    <p class="mt-2 text-sm text-muted">{m.reconcile_checkpoints_empty()}</p>
  {:else if active.length > 0}
    <table class="mt-3 w-full text-sm">
      <thead>
        <tr class="border-b border-border text-left text-xs font-semibold uppercase tracking-widest text-muted">
          <th class="py-2">{m.reconcile_checkpoints_col_date()}</th>
          <th class="py-2 text-right">{m.reconcile_checkpoints_col_balance()}</th>
        </tr>
      </thead>
      <tbody>
        {#each active as checkpoint (checkpoint.id)}
          <tr class="border-b border-border/50 last:border-0">
            <td class="py-2 whitespace-nowrap text-foreground">{checkpoint.statement_date}</td>
            <td class="py-2 text-right tabular-nums text-foreground">
              {formatQuantity(
                checkpoint.statement_balance.quantity_value,
                checkpoint.statement_balance.quantity_scale,
                locale
              )}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</Panel>
