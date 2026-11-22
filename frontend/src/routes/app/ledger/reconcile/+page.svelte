<script lang="ts">
  import { accountLabelByID } from '#lib/accounts/account-labels.ts';
  import { page } from '$app/state';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { onDestroy } from 'svelte';
  import ArrowLeft from '@lucide/svelte/icons/arrow-left';
  import { m } from '#lib/paraglide/messages.js';
  import Panel from '#lib/components/panel.svelte';
  import { authSessionQueryOptions } from '#lib/api/auth.ts';
  import { accountsQueryOptions } from '#lib/api/accounts.ts';
  import { reconciliationSessionQueryOptions, type ReconciliationSessionResponse } from '#lib/api/reconciliation.ts';
  import { invalidateClosedSessionReads } from '#lib/reconcile/invalidate.ts';
  import ReconcileStartForm from '#lib/reconcile/reconcile-start-form.svelte';
  import ReconcileStartOutcome, { type CloseFailure, type StartOutcome } from '#lib/reconcile/reconcile-start-outcome.svelte';
  import ReconcileSession from '#lib/reconcile/reconcile-session.svelte';
  import ReconcileCheckpoints from '#lib/reconcile/reconcile-checkpoints.svelte';
  import { parseReconcileRequest } from '#lib/reconcile/reconcile-link.ts';

  // ── Session + CSRF ────────────────────────────────────────────────
  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const csrfToken = $derived(sessionQuery.data?.csrf_token);

  // ── Accounts (for resolving the active account's label) ───────────
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));

  // `?account=` preselects an account, including a holding, which the
  // default picker leaves out. `&commodity=` (a register statement row's
  // link) reconciles that commodity instead of the account's default.
  const request = $derived(parseReconcileRequest(page.url.searchParams));
  const requestedAccountID = $derived(request.accountID);
  const requestedCommodityID = $derived(request.commodityID);

  // ── View state machine ────────────────────────────────────────────
  type View =
    | { type: 'start'; fromSessionID?: number }
    | { type: 'session'; session: ReconciliationSessionResponse; initialReason?: string; initialError?: unknown }
    | { type: 'finished'; session: ReconciliationSessionResponse };

  const queryClient = useQueryClient();
  let view = $state<View>({ type: 'start' });
  let startOutcome = $state.raw<StartOutcome | undefined>(undefined);
  let startPending = $state(false);
  let mounted = true;
  onDestroy(() => { mounted = false; });

  // The account picked in the start form, so its checkpoints can be shown
  // (and voided, #248) before a new reconciliation starts.
  let startSelection = $state<{ accountID: number; commodityID: number } | undefined>(undefined);

  function handleAccountChange(accountID: number | undefined, commodityID: number | undefined) {
    startSelection =
      accountID !== undefined && commodityID !== undefined ? { accountID, commodityID } : undefined;
  }

  const activeAccountID = $derived(
    view.type === 'start' ? undefined : view.session.account_id
  );

  function accountLabel(accountID: number): string {
    return accountLabelByID(accountsQuery.data?.accounts, accountID) ?? String(accountID);
  }

  function handleStarted(session: ReconciliationSessionResponse) {
    startOutcome = undefined;
    view = { type: 'session', session };
  }

  function isStartFor(sessionID: number) {
    return mounted && view.type === 'start' && view.fromSessionID === sessionID;
  }

  function handleClosed(session: ReconciliationSessionResponse) {
    if (isStartFor(session.id)) {
      startOutcome = { state: 'closed', session };
    } else if (view.type === 'session' && view.session.id === session.id) {
      view = session.status === 'finished' ? { type: 'finished', session } : { type: 'start' };
    }
  }

  async function checkStartOutcome(failure: CloseFailure) {
    const sessionID = failure.session.id;
    if (!isStartFor(sessionID)) return;
    startOutcome = { ...failure, state: 'checking' };
    const options = reconciliationSessionQueryOptions(sessionID);

    let saved: ReconciliationSessionResponse;
    try {
      // Read after the failed response. A cached or already-running read may predate the write.
      await queryClient.cancelQueries({ queryKey: options.queryKey, exact: true });
      saved = await queryClient.fetchQuery({ ...options, staleTime: 0, retry: false });
    } catch {
      if (isStartFor(sessionID)) startOutcome = { ...failure, state: 'unknown' };
      return;
    }

    if (saved.status === 'open') {
      if (isStartFor(sessionID)) startOutcome = { ...failure, session: saved, state: 'open' };
    } else {
      await invalidateClosedSessionReads(queryClient, saved);
      handleClosed(saved);
    }
  }

  function retryStatus() {
    if (startOutcome?.state === 'unknown') void checkStartOutcome(startOutcome);
  }

  function returnToSession() {
    if (startPending || startOutcome?.state !== 'open' || !isStartFor(startOutcome.session.id)) return;
    const { session, reason, error } = startOutcome;
    startOutcome = undefined;
    view = { type: 'session', session, initialReason: reason, initialError: error };
  }

  function backToStart() {
    view = { type: 'start', fromSessionID: view.type === 'session' ? view.session.id : undefined };
  }
</script>

{#if view.type === 'start'}
  <div class="lg:grid lg:grid-cols-[minmax(0,1fr)_22rem] lg:gap-6 xl:grid-cols-[minmax(0,1fr)_26rem]">
    <div class="min-w-0">
      {#if startOutcome}
        <div class="mb-4">
          <ReconcileStartOutcome outcome={startOutcome} accountLabel={accountLabel(startOutcome.session.account_id)} {startPending} onRetry={retryStatus} onReturn={returnToSession} />
        </div>
      {/if}
      <Panel>
        <ReconcileStartForm {csrfToken} {requestedAccountID} {requestedCommodityID} bind:pending={startPending} onStarted={handleStarted} onAccountChange={handleAccountChange} />
      </Panel>
    </div>
    {#if startSelection}
      <div class="mt-6 min-w-0 lg:mt-0">
        <ReconcileCheckpoints
          accountID={startSelection.accountID}
          commodityID={startSelection.commodityID}
          {csrfToken}
        />
      </div>
    {/if}
  </div>
{:else if view.type === 'session'}
  <div class="mb-4">
    <button
      type="button"
      class="inline-flex items-center gap-1.5 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      onclick={backToStart}
    >
      <ArrowLeft size={14} aria-hidden="true" />
      {m.reconcile_title()}
    </button>
  </div>

  <ReconcileSession
    reconciliationID={view.session.id}
    accountLabel={accountLabel(view.session.account_id)}
    {csrfToken}
    initialSession={view.session}
    initialReason={view.initialReason}
    initialError={view.initialError}
    onClosed={handleClosed}
    onCloseFailed={(session, reason, error) => { void checkStartOutcome({ session, reason, error }); }}
  />

  {#if activeAccountID !== undefined}
    <div class="mt-6 max-w-md">
      <ReconcileCheckpoints accountID={activeAccountID} commodityID={view.session.commodity_id} voidable={false} />
    </div>
  {/if}
{:else if view.type === 'finished'}
  <div class="max-w-xl space-y-6">
    <Panel>
      <h2 class="text-sm font-semibold text-foreground">{m.reconcile_finished_title()}</h2>
      <p class="mt-2 text-sm leading-6 text-muted">{m.reconcile_finished_copy()}</p>
      <button
        type="button"
        class="mt-5 inline-flex items-center gap-2 rounded-(--radius-control) bg-accent px-4 py-2.5 text-sm font-semibold text-accent-foreground transition hover:opacity-95"
        onclick={backToStart}
      >
        {m.reconcile_finished_again()}
      </button>
    </Panel>

    <ReconcileCheckpoints accountID={view.session.account_id} commodityID={view.session.commodity_id} {csrfToken} />
  </div>
{/if}
