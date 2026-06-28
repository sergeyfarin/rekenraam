<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { m } from '$lib/paraglide/messages.js';
  import { getLocale } from '$lib/paraglide/runtime.js';
  import Panel from '$lib/components/panel.svelte';
  import StatePanel from '$lib/components/state-panel.svelte';
  import StatusBadge from '$lib/components/status-badge.svelte';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import { formatQuantity } from '$lib/transactions/transaction-labels';
  import {
    reconciliationSessionQueryOptions,
    reconciliationQueryKey,
    updateReconciliationSelection,
    finishReconciliation,
    voidReconciliation,
    type ReconciliationSessionResponse
  } from '$lib/api/reconciliation';

  let {
    reconciliationID,
    accountLabel,
    csrfToken,
    initialSession,
    onFinished,
    onVoided
  }: {
    reconciliationID: number;
    accountLabel: string;
    csrfToken: string | undefined;
    initialSession: ReconciliationSessionResponse;
    onFinished: (session: ReconciliationSessionResponse) => void;
    onVoided: () => void;
  } = $props();

  const locale = $derived(getLocale());

  const sessionQuery = createQuery(() => ({
    ...reconciliationSessionQueryOptions(reconciliationID),
    initialData: initialSession
  }));
  const queryClient = useQueryClient();

  // Server-authoritative snapshot: the latest mutation response wins; otherwise the
  // query (seeded with initialData) provides it. Never compute the difference locally.
  let mutated = $state<ReconciliationSessionResponse | undefined>(undefined);
  const session = $derived(mutated ?? sessionQuery.data ?? initialSession);

  let selectionPending = $state(false);
  let finishPending = $state(false);
  let voidPending = $state(false);
  let actionError = $state<unknown>(undefined);
  let reason = $state('');

  const selectedIDs = $derived(new Set(session.selected_postings.map((p) => p.posting_id)));
  const isBalanced = $derived(session.difference.quantity_value === '0');

  function selectionAfterToggle(togglePostingID: number, nowSelected: boolean): number[] {
    // Despite the field name, the API matches these against candidate posting_id.
    const next = new Set(selectedIDs);
    if (nowSelected) next.add(togglePostingID);
    else next.delete(togglePostingID);
    return session.candidates.filter((c) => next.has(c.posting_id)).map((c) => c.posting_id);
  }

  async function toggle(postingID: number, nowSelected: boolean) {
    if (!csrfToken || selectionPending) return;
    selectionPending = true;
    actionError = undefined;
    try {
      mutated = await updateReconciliationSelection(
        reconciliationID,
        { posting_version_ids: selectionAfterToggle(postingID, nowSelected) },
        csrfToken
      );
    } catch (error) {
      actionError = error;
    } finally {
      selectionPending = false;
    }
  }

  async function handleFinish() {
    if (!csrfToken || !isBalanced) return;
    if (reason.trim() === '') {
      actionError = new Error(m.reconcile_finish_needs_reason());
      return;
    }
    finishPending = true;
    actionError = undefined;
    try {
      const finished = await finishReconciliation(reconciliationID, { change_reason: reason.trim() }, csrfToken);
      await queryClient.invalidateQueries({ queryKey: reconciliationQueryKey });
      onFinished(finished);
    } catch (error) {
      actionError = error;
    } finally {
      finishPending = false;
    }
  }

  async function handleVoid() {
    if (!csrfToken) return;
    voidPending = true;
    actionError = undefined;
    try {
      await voidReconciliation(reconciliationID, { change_reason: reason.trim() || 'discarded' }, csrfToken);
      await queryClient.invalidateQueries({ queryKey: reconciliationQueryKey });
      onVoided();
    } catch (error) {
      actionError = error;
      voidPending = false;
    }
  }

  function detailFor(payee?: string, description?: string): string {
    return [payee, description].filter(Boolean).join(' — ') || '—';
  }

  const inputClass =
    'mt-1.5 h-10 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted hover:bg-control-hover focus:border-accent disabled:cursor-not-allowed disabled:opacity-70';
  const labelClass = 'block text-xs font-semibold uppercase tracking-[0.12em] text-muted';
</script>

{#if sessionQuery.isError}
  <StatePanel title={m.reconcile_session_error_title()} copy={m.reconcile_session_error_copy()}>
    <button
      type="button"
      class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      onclick={() => sessionQuery.refetch()}
    >
      {m.reconcile_session_retry()}
    </button>
  </StatePanel>
{:else}
  <div class="lg:grid lg:grid-cols-[minmax(0,1fr)_22rem] lg:gap-6 xl:grid-cols-[minmax(0,1fr)_26rem]">
    <!-- Candidate postings -->
    <div class="min-w-0">
      <div class="mb-4">
        <h2 class="text-sm font-semibold text-foreground">{m.reconcile_session_title()}</h2>
        <p class="mt-1 text-sm leading-6 text-muted">{m.reconcile_session_copy()}</p>
        <p class="mt-2 text-xs text-muted">
          {m.reconcile_session_account()}: <span class="font-semibold text-foreground">{accountLabel}</span>
          · {m.reconcile_session_statement_date()}: <span class="font-semibold text-foreground">{session.statement_date}</span>
        </p>
      </div>

      {#if session.candidates.length === 0}
        <StatePanel title={m.reconcile_session_empty_title()} copy={m.reconcile_session_empty_copy()} />
      {:else}
        <Panel>
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead>
                <tr class="border-b border-border text-left text-xs font-semibold uppercase tracking-widest text-muted">
                  <th class="px-3 py-2">{m.reconcile_session_col_clear()}</th>
                  <th class="px-3 py-2">{m.reconcile_session_col_date()}</th>
                  <th class="px-3 py-2">{m.reconcile_session_col_detail()}</th>
                  <th class="px-3 py-2 text-right">{m.reconcile_session_col_amount()}</th>
                </tr>
              </thead>
              <tbody>
                {#each session.candidates as posting (posting.posting_id)}
                  {@const checked = selectedIDs.has(posting.posting_id)}
                  <tr class="border-b border-border/50 last:border-0">
                    <td class="px-3 py-3">
                      <input
                        type="checkbox"
                        class="h-4 w-4 accent-accent"
                        {checked}
                        disabled={selectionPending || !csrfToken}
                        aria-label={detailFor(posting.payee_name, posting.description)}
                        onchange={(e) => toggle(posting.posting_id, e.currentTarget.checked)}
                      />
                    </td>
                    <td class="px-3 py-3 whitespace-nowrap text-muted">{posting.entry_date}</td>
                    <td class="px-3 py-3 text-foreground">{detailFor(posting.payee_name, posting.description)}</td>
                    <td class="px-3 py-3 text-right tabular-nums text-foreground">
                      {formatQuantity(posting.quantity_value, posting.quantity_scale, locale)}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </Panel>
      {/if}
    </div>

    <!-- Summary + actions -->
    <aside class="mt-6 lg:mt-0">
      <div class="lg:sticky lg:top-4">
        <Panel>
          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <span class="text-sm text-muted">{m.reconcile_summary_statement_balance()}</span>
              <span class="tabular-nums text-foreground">
                {formatQuantity(session.statement_balance.quantity_value, session.statement_balance.quantity_scale, locale)}
              </span>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-sm text-muted">{m.reconcile_summary_starting_balance()}</span>
              <span class="tabular-nums text-foreground">
                {formatQuantity(session.starting_balance.quantity_value, session.starting_balance.quantity_scale, locale)}
              </span>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-sm text-muted">{m.reconcile_summary_cleared_balance()}</span>
              <span class="tabular-nums text-foreground">
                {formatQuantity(session.selected_balance.quantity_value, session.selected_balance.quantity_scale, locale)}
              </span>
            </div>

            <div class="border-t border-border pt-3">
              <div class="flex items-center justify-between">
                <span class="text-sm font-semibold text-foreground">{m.reconcile_summary_difference()}</span>
                <span class="tabular-nums font-semibold text-foreground">
                  {formatQuantity(session.difference.quantity_value, session.difference.quantity_scale, locale)}
                </span>
              </div>
              <div class="mt-2">
                <StatusBadge tone={isBalanced ? 'positive' : 'warning'}>
                  {isBalanced ? m.reconcile_summary_balanced() : m.reconcile_summary_unbalanced()}
                </StatusBadge>
              </div>
            </div>

            <div class="border-t border-border pt-3">
              <label class={labelClass} for="reconcile-reason">{m.reconcile_reason_label()}</label>
              <input
                id="reconcile-reason"
                type="text"
                class={inputClass}
                placeholder={m.reconcile_reason_placeholder()}
                bind:value={reason}
                disabled={finishPending || voidPending}
              />
            </div>

            <APIFormError error={actionError} id="reconcile-action-error" />

            <div class="flex flex-col gap-2">
              <button
                type="button"
                class="inline-flex items-center justify-center gap-2 rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
                disabled={!isBalanced || finishPending || voidPending || !csrfToken}
                onclick={handleFinish}
              >
                {finishPending ? m.reconcile_finishing() : m.reconcile_finish()}
              </button>
              {#if !isBalanced}
                <p class="text-xs text-muted">{m.reconcile_finish_hint()}</p>
              {/if}
              <button
                type="button"
                class="inline-flex items-center justify-center gap-2 rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-muted transition hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-60"
                disabled={finishPending || voidPending || !csrfToken}
                onclick={handleVoid}
              >
                {voidPending ? m.reconcile_voiding() : m.reconcile_void()}
              </button>
            </div>
          </div>
        </Panel>
      </div>
    </aside>
  </div>
{/if}
