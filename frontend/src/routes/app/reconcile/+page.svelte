<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import ArrowLeft from '@lucide/svelte/icons/arrow-left';
  import { m } from '$lib/paraglide/messages.js';
  import Panel from '$lib/components/panel.svelte';
  import { authSessionQueryOptions } from '$lib/api/auth';
  import { accountsQueryOptions, type AccountResponse } from '$lib/api/accounts';
  import type { ReconciliationSessionResponse } from '$lib/api/reconciliation';
  import ReconcileStartForm from '$lib/reconcile/reconcile-start-form.svelte';
  import ReconcileSession from '$lib/reconcile/reconcile-session.svelte';
  import ReconcileCheckpoints from '$lib/reconcile/reconcile-checkpoints.svelte';

  // ── Session + CSRF ────────────────────────────────────────────────
  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const csrfToken = $derived(sessionQuery.data?.csrf_token);

  // ── Accounts (for resolving the active account's label) ───────────
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));

  // ── View state machine ────────────────────────────────────────────
  type View =
    | { type: 'start' }
    | { type: 'session'; session: ReconciliationSessionResponse }
    | { type: 'finished'; session: ReconciliationSessionResponse };

  let view = $state<View>({ type: 'start' });

  const activeAccountID = $derived(
    view.type === 'start' ? undefined : view.session.account_id
  );

  function accountLabel(accountID: number): string {
    const account = (accountsQuery.data?.accounts ?? []).find(
      (a: AccountResponse) => a.id === accountID
    );
    return account?.name ?? account?.code ?? String(accountID);
  }

  function handleStarted(session: ReconciliationSessionResponse) {
    view = { type: 'session', session };
  }

  function handleFinished(session: ReconciliationSessionResponse) {
    view = { type: 'finished', session };
  }

  function handleVoided() {
    view = { type: 'start' };
  }

  function backToStart() {
    view = { type: 'start' };
  }
</script>

{#if view.type === 'start'}
  <div class="lg:grid lg:grid-cols-[minmax(0,1fr)_22rem] lg:gap-6 xl:grid-cols-[minmax(0,1fr)_26rem]">
    <div class="min-w-0">
      <Panel>
        <ReconcileStartForm {csrfToken} onStarted={handleStarted} />
      </Panel>
    </div>
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
    onFinished={handleFinished}
    onVoided={handleVoided}
  />

  {#if activeAccountID !== undefined}
    <div class="mt-6 max-w-md">
      <ReconcileCheckpoints accountID={activeAccountID} commodityID={view.session.commodity_id} />
    </div>
  {/if}
{:else if view.type === 'finished'}
  <div class="max-w-xl space-y-6">
    <Panel>
      <h2 class="text-sm font-semibold text-foreground">{m.reconcile_finished_title()}</h2>
      <p class="mt-2 text-sm leading-6 text-muted">{m.reconcile_finished_copy()}</p>
      <button
        type="button"
        class="mt-5 inline-flex items-center gap-2 rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90"
        onclick={backToStart}
      >
        {m.reconcile_finished_again()}
      </button>
    </Panel>

    <ReconcileCheckpoints accountID={view.session.account_id} commodityID={view.session.commodity_id} />
  </div>
{/if}
