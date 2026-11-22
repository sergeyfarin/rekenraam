<script module lang="ts">
  import type { ReconciliationSessionResponse } from '#lib/api/reconciliation.ts';

  export type CloseFailure = { session: ReconciliationSessionResponse; reason: string; error: unknown };
  export type StartOutcome =
    | { state: 'closed'; session: ReconciliationSessionResponse }
    | (CloseFailure & { state: 'checking' | 'unknown' | 'open' });
</script>

<script lang="ts">
  import Panel from '#lib/components/panel.svelte';
  import { formatCalendarDate } from '#lib/dates/calendar-date.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';

  let { outcome, accountLabel, startPending, onRetry, onReturn }: {
    outcome: StartOutcome;
    accountLabel: string;
    startPending: boolean;
    onRetry: () => void;
    onReturn: () => void;
  } = $props();

  const statement = $derived({ account: accountLabel, date: formatCalendarDate(outcome.session.statement_date, getLocale()) });
  const actionClass = 'mt-3 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-60';
</script>

<Panel>
  {#if outcome.state === 'closed'}
    <p role="status" class="text-sm font-semibold text-positive">
      {outcome.session.status === 'finished' ? m.reconcile_start_finished(statement) : m.reconcile_start_discarded(statement)}
    </p>
  {:else if outcome.state === 'checking'}
    <p role="status" class="text-sm text-muted">{m.reconcile_start_checking_result(statement)}</p>
  {:else if outcome.state === 'unknown'}
    <p role="alert" class="text-sm text-danger">{m.reconcile_start_result_unknown(statement)}</p>
    <button type="button" class={actionClass} onclick={onRetry}>{m.reconcile_start_retry_status()}</button>
  {:else}
    <p role="alert" class="text-sm text-danger">{m.reconcile_start_result_open(statement)}</p>
    <button type="button" class={actionClass} disabled={startPending} onclick={onReturn}>{m.reconcile_start_return_to_session()}</button>
  {/if}
</Panel>
